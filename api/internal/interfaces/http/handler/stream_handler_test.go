package handler_test

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/angkut/angkut/apps/api/internal/interfaces/http/handler"
	"github.com/angkut/angkut/apps/api/internal/interfaces/http/middleware"
	"github.com/angkut/angkut/apps/internal/application/trackingapp"
	"github.com/angkut/angkut/apps/internal/domain/organization"
	"github.com/angkut/angkut/apps/internal/domain/shared"
	"github.com/angkut/angkut/apps/internal/domain/shipment"
	"github.com/angkut/angkut/apps/internal/domain/tracking"
	"github.com/angkut/angkut/apps/internal/realtime"
)

// stubStreamService menyediakan timeline untuk uji SSE.
type stubStreamService struct{}

func (stubStreamService) Append(context.Context, string, string, string, trackingapp.AppendInput) (*trackingapp.AppendResult, error) {
	return nil, nil
}

func (stubStreamService) Timeline(context.Context, string, string) (*tracking.Timeline, *shipment.Shipment, error) {
	return stubTimelineSSE("SSE-1"), stubShipmentSSE(), nil
}

func (stubStreamService) TimelineByAWB(_ context.Context, _ string, awb string) (*tracking.Timeline, *shipment.Shipment, error) {
	if awb == "GAKADA" {
		return nil, nil, shared.NotFound("TRACKING_NOT_FOUND", "Pengiriman tidak ditemukan.")
	}
	return stubTimelineSSE(awb), stubShipmentSSE(), nil
}

func (stubStreamService) TimelinePublik(_ context.Context, awb string) (*tracking.Timeline, *shipment.Shipment, error) {
	return stubStreamService{}.TimelineByAWB(context.Background(), "", awb)
}

func (stubStreamService) List(context.Context, string, tracking.Filter) ([]tracking.Event, int, error) {
	return nil, 0, nil
}

func stubTimelineSSE(awb string) *tracking.Timeline {
	now := time.Now().UTC().Truncate(time.Second)
	return &tracking.Timeline{
		AWB:    awb,
		Status: string(shipment.StatusInTransit),
		Events: []tracking.Event{
			{Status: tracking.EventPickedUp, Location: "Semarang", OccurredAt: now},
		},
		LastLocation: "Cirebon",
	}
}

func stubShipmentSSE() *shipment.Shipment {
	return &shipment.Shipment{
		ID: "s-1", AWB: "SSE-1", CourierCode: "jne", ServiceCode: "REG",
		EtaMinDays: 1, EtaMaxDays: 2,
	}
}

// sseRouter menyusun router chi untuk endpoint stream dengan identitas uji.
func sseRouter(h *handler.StreamHandler) *chi.Mux {
	identity := &middleware.Identity{
		UserID:         "tu-1",
		OrganizationID: "torg-1",
		RoleCode:       organization.RoleStaff,
	}
	srv := chi.NewRouter()
	srv.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			next.ServeHTTP(w, r.WithContext(middleware.WithTestIdentity(r.Context(), identity)))
		})
	})
	srv.Get("/stream/{awb}", h.TrackSSE)
	return srv
}

// TestSSEUnknownAWBRejectsSebelumStream: 404 tidak dibungkus sebagai aliran
// 200 — EventSource menampilkan kegagalan yang jelas, bukan stream kosong.
func TestSSEUnknownAWBRejectsSebelumStream(t *testing.T) {
	hub := realtime.NewHub(context.Background(), nil, nil)
	h := handler.NewStreamHandler(stubStreamService{}, hub, nil)

	req := httptest.NewRequest(http.MethodGet, "/stream/GAKADA", nil)
	rec := httptest.NewRecorder()
	sseRouter(h).ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, ingin 404", rec.Code)
	}
}

// TestSSESnapshotKontrak: snapshot pertama berbentuk SSE event berisi JSON
// kontrak (awb, status, events, confidence) yang dipakai FE.
func TestSSESnapshotKontrak(t *testing.T) {
	hub := realtime.NewHub(context.Background(), nil, nil)
	h := handler.NewStreamHandler(stubStreamService{}, hub, nil)

	req := httptest.NewRequest(http.MethodGet, "/stream/SSE-1", nil)
	rec := httptest.NewRecorder()

	srv := sseRouter(h)
	done := make(chan struct{})
	go func() { srv.ServeHTTP(rec, req); close(done) }()

	buf := bufio.NewReader(rec.Body)
	var dataLine, eventLine string
	start := time.Now()
	for dataLine == "" && time.Since(start) < 2*time.Second {
		line, err := buf.ReadString(10) // 10 = '\n'
		if strings.HasPrefix(line, "event: ") {
			eventLine = strings.TrimSuffix(strings.TrimPrefix(line, "event: "), "\n")
		}
		if line != "" && strings.HasPrefix(line, "data: ") {
			dataLine = strings.TrimSuffix(strings.TrimPrefix(line, "data: "), "\n")
			break
		}
		if err != nil && !errors.Is(err, io.EOF) {
			t.Fatalf("membaca aliran: %v", err)
		}
		if err != nil && errors.Is(err, io.EOF) {
			// aliran belum menulis apa-apa — tunggu sejenak lalu cegah test
			// hang selamanya.
			select {
			case <-done:
			case <-time.After(200 * time.Millisecond):
			}
		}
	}
	if dataLine == "" {
		t.Fatal("tidak ada event data: di aliran (snapshot awal tidak terkirim)")
	}

	var payload map[string]any
	if err := json.Unmarshal([]byte(dataLine), &payload); err != nil {
		t.Fatalf("payload SSE bukan JSON: %v\n%s", err, dataLine)
	}
	if payload["awb"] != "SSE-1" || payload["shipment_id"] != "s-1" {
		t.Errorf("payload tidak memuat awb/shipment_id: %v", payload)
	}
	if _, ok := payload["events"].([]any); !ok {
		t.Errorf("events harus array: %T", payload["events"])
	}
	conf, ok := payload["confidence"].(map[string]any)
	if !ok || conf == nil {
		t.Errorf("confidence harus object: %T", payload["confidence"])
		return
	}
	if _, ok := conf["level"]; !ok {
		t.Error("confidence harus memuat level")
	}
	// Event line dibaca dari aliran (buffer sudah terpakai oleh bufio), bukan
	// dari rec.Body yang sudah habis — asersinya tetap sama ketatnya.
	if eventLine != "tracking_update" {
		t.Errorf("event SSE = %q, ingin tracking_update", eventLine)
	}
}

// TestWSSubjectsKontrak: pemetaan channels → subjek internal adalah kontrak
// yang bisa berubah diam-diam; dipertahankan oleh helper uji eksplisit.
func TestWSSubjectsKontrak(t *testing.T) {
	subjects := handler.WSChannelSubjects("shipments,track:AWB-9,jalur-tak-lain", "org-1")
	if len(subjects) != 2 {
		t.Fatalf("subjek = %v, ingin 2 (kanal tak dikenal diabaikan)", subjects)
	}
	if subjects[0] != "angkut:org:org-1:shipments" || subjects[1] != "angkut:track:AWB-9" {
		t.Errorf("subjek salah: %v", subjects)
	}
}
