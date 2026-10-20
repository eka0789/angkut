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
	"github.com/angkut/angkut/apps/internal/domain/shipment"
	"github.com/angkut/angkut/apps/internal/domain/tracking"
	"github.com/angkut/angkut/apps/internal/realtime"
)

// Batas streaming. Nilainya konservatif: klien diminta menyambung ulang lebih
// sering daripada membiarkan koneksi menggantung di proxy.
const (
	// sseHeartbeat mengirim ": ping" berkala untuk menjaga NAT/proxy tetap
	// menganggap koneksi terpakai.
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

// TrackSSE menangani GET /api/v1/tracking/{awb}/stream (EventSource).
//
// Snapshot dikirim lebih dulu sebagai event `tracking_update`; setelah itu
// pembaruan dari hub dialirkan dengan bentuk payload yang sama. Resi yang
// tidak ada ditolak 404 SEBELUM header 200 dikirim, supaya EventSource
// melaporkan kegagalan alih-alih stream kosong.
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

	tl, sh, err := h.svc.TimelineByAWB(r.Context(), identity.OrganizationID, awb)
	if err != nil {
		response.Fail(w, err)
		return
	}

	flusher := flusherOf(w)
	if flusher == nil {
		response.Fail(w, fmt.Errorf("streaming tidak didukung oleh server ini"))
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)

	if !writeSSE(w, flusher, trackingPayload(tl, sh)) {
		return
	}

	sub, cancel := h.hub.Subscribe([]string{realtime.TrackChannel(awb)})
	defer cancel()

	heartbeat := time.NewTicker(sseHeartbeat)
	defer heartbeat.Stop()
	life := time.NewTimer(sseMaxLife)
	defer life.Stop()

	for {
		select {
		case <-r.Context().Done():
			return
		case <-life.C:
			// Tutup rapi; EventSource akan menyambung ulang dan mendapat
			// snapshot baru — lebih baik daripada koneksi menggantung.
			return
		case <-heartbeat.C:
			if _, err := fmt.Fprint(w, ": ping\n\n"); err != nil {
				return
			}
			flusher.Flush()
		case msg, ok := <-sub.C:
			if !ok {
				return
			}
			if !writeSSE(w, flusher, msg.Payload) {
				return
			}
		}
	}
}

// flusherOf mencari http.Flusher di balik pembungkus ResponseWriter dengan
// berjalan lewat Unwrap() — sama seperti http.ResponseController.
//
// Diperlukan karena middleware (mis. pencatat akses) membungkus writer hanya
// dengan menyalin metode ResponseWriter, sehingga `w.(http.Flusher)` pada
// handler gagal walau server aslinya mendukung flush. Bila tidak ada flusher
// sama sekali, kembalikan nil agar handler menolak SEBELUM header 200 dikirim.
func flusherOf(w http.ResponseWriter) http.Flusher {
	for cur := w; cur != nil; {
		if f, ok := cur.(http.Flusher); ok {
			return f
		}
		u, ok := cur.(interface{ Unwrap() http.ResponseWriter })
		if !ok {
			return nil
		}
		cur = u.Unwrap()
	}
	return nil
}

// writeSSE menulis satu event dan melaporkan apakah koneksi masih dapat dipakai.
func writeSSE(w http.ResponseWriter, flusher http.Flusher, payload map[string]any) bool {
	body, err := json.Marshal(payload)
	if err != nil {
		return false
	}
	if _, err := fmt.Fprintf(w, "event: tracking_update\ndata: %s\n\n", body); err != nil {
		return false
	}
	flusher.Flush()
	return true
}

// WSChannelSubjects memetakan daftar `channels` dari klien ke subjek internal.
//
// Kanal tak dikenal diabaikan dengan sengaja: klien lama tidak boleh membuat
// koneksi gagal hanya karena server menambah jenis kanal baru.
func WSChannelSubjects(channels, orgID string) []string {
	subjects := make([]string, 0, len(channels))
	for _, raw := range strings.Split(channels, ",") {
		c := strings.TrimSpace(raw)
		switch {
		case c == "shipments":
			subjects = append(subjects, realtime.OrgShipmentsChannel(orgID))
		case strings.HasPrefix(c, "track:"):
			awb := strings.TrimSpace(strings.TrimPrefix(c, "track:"))
			if awb != "" {
				subjects = append(subjects, realtime.TrackChannel(awb))
			}
		}
	}
	return subjects
}

// OrgWS menangani GET /api/v1/ws?channels=shipments,track:AWB (WebSocket).
//
// Token sudah diverifikasi middleware AuthQuery; identitas dipakai untuk
// menurunkan kanal organisasi sehingga klien tidak bisa meminta kanal tenant
// lain lewat parameter.
func (h *StreamHandler) OrgWS(w http.ResponseWriter, r *http.Request) {
	identity := middleware.IdentityFrom(r.Context())
	if identity == nil {
		response.Fail(w, unauthorized())
		return
	}
	subjects := WSChannelSubjects(r.URL.Query().Get("channels"), identity.OrganizationID)
	if len(subjects) == 0 {
		response.Fail(w, ValidationError(map[string]string{
			"channels": "Pilih minimal satu kanal: shipments atau track:<awb>.",
		}))
		return
	}

	conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{
		// Origin sudah divalidasi CORS untuk permintaan biasa; WS dari browser
		// membawa Origin yang sama. Pemeriksaan di sini mengandalkan token.
		InsecureSkipVerify: false,
	})
	if err != nil {
		return
	}
	defer conn.Close(websocket.StatusNormalClosure, "")

	sub, cancel := h.hub.Subscribe(subjects)
	defer cancel()

	ctx := r.Context()
	ping := time.NewTicker(wsPingInterval)
	defer ping.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ping.C:
			if err := conn.Ping(ctx); err != nil {
				return
			}
		case msg, ok := <-sub.C:
			if !ok {
				return
			}
			data, err := json.Marshal(msg)
			if err != nil {
				continue
			}
			if err := conn.Write(ctx, websocket.MessageText, data); err != nil {
				return
			}
		}
	}
}

// pastikan context tetap diimpor untuk tanda tangan interface di paket ini.
var _ context.Context
