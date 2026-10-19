package trackingapp_test

import (
	"log/slog"
	"sync"
	"testing"

	"github.com/angkut/angkut/apps/internal/application/trackingapp"
	"github.com/angkut/angkut/apps/internal/realtime"
)

// spyNotifier mencatat publish yang dilakukan pemanggil.
type spyNotifier struct {
	mu    sync.Mutex
	calls []realtime.Message
}

func (s *spyNotifier) Publish(msg realtime.Message) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.calls = append(s.calls, msg)
	return nil
}

// TestDepsTanpaNotifierTidakPanic melindungi janji port opsional: worker/API
// lama yang tidak menyuntik Notifier tetap hidup tanpa cabang khusus.
func TestDepsTanpaNotifierTidakPanic(t *testing.T) {
	d := trackingapp.Deps{Log: slog.Default()}
	_ = trackingapp.New(d)
}

// TestKontrakKanalRealtime melindungi bentuk kanal yang dipakai dua konsumen:
// SSE per-resi (angkut:track:*) dan WS per-organisasi (angkut:org:*:shipments).
// Bila bentuk kanal diubah tanpa sengaja, test ini menangkapnya sebelum klien.
func TestKontrakKanalRealtime(t *testing.T) {
	spy := &spyNotifier{}

	msg := realtime.Message{
		Type:   realtime.TypeTrackingUpdated,
		Topics: []string{realtime.TrackChannel("AWB-1"), realtime.OrgShipmentsChannel("O1")},
	}
	if err := spy.Publish(msg); err != nil {
		t.Fatalf("Publish: %v", err)
	}
	if len(spy.calls) != 1 {
		t.Fatalf("panggilan = %d, ingin 1", len(spy.calls))
	}
	topics := spy.calls[0].Topics
	if len(topics) != 2 {
		t.Fatalf("kanal = %v, ingin 2 (per-resi + per-org)", topics)
	}
	if topics[0] != "angkut:track:AWB-1" || topics[1] != "angkut:org:O1:shipments" {
		t.Errorf("kanal salah: %v", topics)
	}
}
