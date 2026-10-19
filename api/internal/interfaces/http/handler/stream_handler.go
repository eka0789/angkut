// Package handler — endpoint streaming realtime (SSE untuk timeline tracking,
// WebSocket untuk kanal organisasi).
//
// Bentuk koneksi dipilih sesuai fungsinya: SSE cocok untuk aliran satu arah
// yang bisa dibuka browser tanpa dependensi (EventSource), WebSocket cocok
// untuk kanal majemuk berumur panjang. Keduanya berbagi prinsip yang sama:
// snapshot REST tetap sumber kebenaran; stream hanya mempercepat tampilan,
// tidak pernah menjadi sumber tunggal.
package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/coder/websocket"
	"github.com/go-chi/chi/v5"

	"github.com/angkut/angkut/apps/api/internal/interfaces/http/middleware"
	"github.com/angkut/angkut/apps/api/internal/interfaces/http/response"
	"github.com/angkut/angkut/apps/internal/domain/shared"
	"github.com/angkut/angkut/apps/internal/domain/shipment"
	"github.com/angkut/angkut/apps/internal/domain/tracking"
	"github.com/angkut/angkut/apps/internal/realtime"
)

// Batas streaming. Nilainya konservatif: klien diminta menyambung ulang lebih
// sering daripada membiarkan koneksi menggantung di proxy.
const (
	// sseHeartbeat mengirim ": ping" berkala untuk menjaga NAT/proxy tetap
	// anggap koneksi terpakai.
	sseHeartbeat = 20 * time.Second
	// sseMaxLife memotong koneksi SSE berkala; EventSource menyambung ulang
	// otomatis dan mendapat snapshot baru — jendela kebobolan proxy tidak ada.
	sseMaxLife = 10 * time.Minute
	// wsPingInterval menjaga klien browser dan NAT/proxy tidak mengekspirasi.
	wsPingInterval = 25 * time.Second
)

// StreamHandler menangani endpoint streaming realtime.
type StreamHandler struct {
	svc TrackingService
	hub *realtime.Hub
	log *slog.Logger
}

// NewStreamHandler membuat handler streaming. Hub opsional berisi jembatan
// Redis bila tersedia; tanpa Redis, stream tetap mengirim snapshot awal.
func NewStreamHandler(svc TrackingService, hub *realtime.Hub, log *slog.Logger) *StreamHandler {
	if log == nil {
		log = slog.Default()
	}
	return &StreamHandler{svc: svc, hub: hub, log: log}
}

// trackingPayload menyusun bentuk respons — HARUS sama persis dengan ByAWB
// agar klien tidak perlu menangani dua bentuk timeline.
func trackingPayload(tl *tracking.Timeline, sh *shipment.Shipment) map[string]any {
	return map[string]any{
		"awb":              tl.AWB,
		"status":           tl.Status,
		"courier":          map[string]any{"code": sh.CourierCode, "service": sh.ServiceCode},
		"events":           tl.Events,
		"last_location":    tl.LastLocation,
		"last_lat":         tl.LastLat,
		"last_lng":         tl.LastLng,
		"last_occurred_at": tl.OccurredAt,
		"eta_min_days":     sh.EtaMinDays,
		"eta_max_days":     sh.EtaMaxDays,
		"confidence":       tracking.EvaluateConfidence(sh, nowUTC()),
		"shipment_id":      sh.ID,
	}
}

// TrackSSE menangani GET /api/v1/tracking/{awb}/stream — EventSource.
//
// Pola:
//  1. Snapshot penuh dikirim SEKALI saat tersambung (recovery klien baru);
//  2. Setiap pesan realtime → snapshot TERBARU dibaca ulang dari database,
//     termasuk keyakinan ETA yang berubah (bukan hanya event barunya);
//  3. Umur koneksi dibatasi sseMaxLife; klien menyambung ulang otomatis.
func (h *StreamHandler) TrackSSE(w http.ResponseWriter, r *http.Request) {
	identity := middleware.IdentityFrom(r.Context())
	if identity == nil {
		response.Fail(w, unauthorized())
		return
	}
	awb := strings.TrimSpace(chi.URLParam(r, "awb"))
	if awb == "" {
		response.Fail(w, ValidationError(map[string]string{
			"awb": "Nomor resi wajib diisi.",
		}))
		return
	}

	flusher, ok := w.(http.Flusher)
	if !ok {
		response.Fail(w, shared.New(
			"SSE_UNSUPPORTED", "Streaming tidak didukung lingkungan server ini.",
			shared.ErrConflict))
		return
	}

	tl, sh, err := h.svc.TimelineByAWB(r.Context(), identity.OrganizationID, awb)
	if err != nil {
		// Ditulis SEBELUM header streaming: EventSource menampilkan error nyata
		// alih-alih aliran berumur panjang dengan 200 kosong.
		response.Fail(w, err)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	// X-Accel-Buffering: no — nginx tidak menampung aliran ini (kalau tidak,
	// pesan numpuk di buffer proxy dan klien menganggap stream diam).
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)

	writeSSEEvent(w, flusher, "tracking_update", trackingPayload(tl, sh))

	sub, cancel := h.hub.Subscribe([]string{realtime.TrackChannel(tl.AWB)})
	defer cancel()

	ctx := r.Context()
	deadline := time.Now().Add(sseMaxLife)
	ticker := time.NewTicker(sseHeartbeat)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			fmt.Fprint(w, ": ping\n\n")
			flusher.Flush()
		case <-sub.C:
			// Klien DALAM STREAM selalu dibarui dari sumber kebenaran.
			ntl, nsh, fetchErr := h.svc.TimelineByAWB(ctx, identity.OrganizationID, awb)
			if fetchErr != nil {
				h.log.Warn("stream tracking: snapshot ulang gagal",
					"awb", awb, "error", fetchErr)
				continue
			}
			writeSSEEvent(w, flusher, "tracking_update", trackingPayload(ntl, nsh))
		case <-ctx.Done():
			return
		default:
			if time.Now().After(deadline) {
				// Sisa umur habis secara PALANG SENGANGA: beri kabar explicit
				// agar klien yang "menyala tapi tidak begitu jarak" tidak
				// berharap stream hidup selamanya.
				fmt.Fprint(w, "event: reconnect\ndata: {}\n\n")
				flusher.Flush()
				return
			}
		}
	}
}

// OrgWS menangani GET /api/v1/ws?token=...&channels=shipments[,...].
//
// Kanal disaring dengan DAFTAR DIPERBOLEHKAN: kanal yang tidak dikenal cukup
// menghilangkan subjek TS — kunjungan permintaan tidak jadi entitas, tapi
// bila SEMUA kanal tidak valid, koneksi ditolak dengan 422 supaya klien tidak
// tersambung dengan tampilan "terhubung tapi mutiara tidak berpersion" yang
// sulit diperiksa.
func (h *StreamHandler) OrgWS(w http.ResponseWriter, r *http.Request) {
	identity := middleware.IdentityFrom(r.Context())
	if identity == nil {
		response.Fail(w, unauthorized())
		return
	}
	subjects := wsSubjects(r.URL.Query().Get("channels"), identity.OrganizationID)
	if len(subjects) == 0 {
		response.Fail(w, shared.Validation(map[string]string{
			"channels": "Harus diisi dari: shipments, command_center, track:<awb>.",
		}))
		return
	}

	conn, err := websocket.Accept(w, r, nil)
	if err != nil {
		return // Accept sudah menulis respons kesalahan sendiri.
	}
	defer conn.Close(websocket.StatusInternalError, "koneksi ditutup")

	sub, cancel := h.hub.Subscribe(subjects)
	defer cancel()

	ctx := r.Context()
	ping := time.NewTicker(wsPingInterval)
	defer ping.Stop()
	readerDone := make(chan struct{}, 1)
	go func() {
		// Pembaca klien hanya untuk drain + deteksi tutup — server tidak
		// mengharapkan isi pesan tertentu; coder/websocket otomatis membalas
		// ping protokol dari klien.
		for {
			if _, _, err := conn.Reader(ctx); err != nil {
				readerDone <- struct{}{}
				return
			}
		}
	}()
	defer func() { <-readerDone }() // menunggu goroutine pemaca selesai.

	for {
		select {
		case msg := <-sub.C:
			wctx, cancelW := context.WithTimeout(ctx, 5*time.Second)
			err := writeWSMessage(wctx, conn, msg)
			cancelW()
			if err != nil {
				return
			}
		case <-ping.C:
			pctx, cancelP := context.WithTimeout(ctx, 5*time.Second)
			perr := conn.Ping(pctx)
			cancelP()
			if perr != nil {
				return
			}
		case <-ctx.Done():
			return
		case <-readerDone:
			// Klien menutup koneksi — operasi normal, bukan kelainan.
			return
		}
	}
}

// wsSubjects mengubah parameter channels menjadi daftar subjek internal.
func wsSubjects(param, orgID string) []string {
	subjects := make([]string, 0, 2)
	for _, ch := range splitCSV(param) {
		switch ch {
		case "shipments", "command_center":
			// command_center alias shipments sampai modulnya dibangun — klien
			// yang diproduksi saat ini TIDAK boleh begitu mengandalkan detail.
			subjects = append(subjects, realtime.OrgShipmentsChannel(orgID))
		default:
			if awb, ok := trackChannelFromParam(ch); ok {
				subjects = append(subjects, realtime.TrackChannel(awb))
			}
		}
	}
	return dedupeStrings(subjects)
}

// trackChannelFromParam menerima "track:<awb>" dan mengembalikan AWB-nya.
func trackChannelFromParam(ch string) (string, bool) {
	const prefix = "track:"
	if !strings.HasPrefix(ch, prefix) {
		return "", false
	}
	awb := strings.TrimSpace(ch[len(prefix):])
	if awb == "" {
		return "", false
	}
	return awb, true
}

func splitCSV(s string) []string {
	parts := make([]string, 0, 4)
	for _, p := range strings.Split(s, ",") {
		if p = strings.TrimSpace(p); p != "" {
			parts = append(parts, p)
		}
	}
	return parts
}

func dedupeStrings(in []string) []string {
	seen := make(map[string]struct{}, len(in))
	out := make([]string, 0, len(in))
	for _, s := range in {
		if _, ok := seen[s]; ok {
			continue
		}
		seen[s] = struct{}{}
		out = append(out, s)
	}
	return out
}

// writeSSEEvent menulis satu event SSE lalu flush.
//
// Bila payload gagal diserialisasi (bisa terjadi bila payload cair berisi
// tipe asing), event dilewati — aliran tidak untuk dibunuh oleh satu pesan.
func writeSSEEvent(w http.ResponseWriter, flusher http.Flusher, event string, payload any) {
	raw, err := json.Marshal(payload)
	if err != nil {
		return
	}
	fmt.Fprintf(w, "id: %d\nevent: %s\ndata: %s\n\n", time.Now().UnixNano(), event, raw)
	flusher.Flush()
}

func writeWSMessage(ctx context.Context, conn *websocket.Conn, msg realtime.Message) error {
	raw, err := json.Marshal(map[string]any{"type": msg.Type, "data": msg.Payload})
	if err != nil {
		return err
	}
	return conn.Write(ctx, websocket.MessageText, raw)
}

// WSChannelSubjects memetakan daftar kanal klien ke subjek internal.
//
// diekspor khusus supaya kontraknya dijaga oleh test — bukan untuk dipakai
// handler lain (OrgWS memanggil fungsi ini juga).
func WSChannelSubjects(channelsParam, orgID string) []string {
	return wsSubjects(channelsParam, orgID)
}
