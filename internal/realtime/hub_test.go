package realtime

import (
	"encoding/json"
	"testing"
	"time"
)

// broadcastTest adalah hub dengan goroutine Redis dimainkan — broadcast
// dipanggil langsung sehingga uji deterministik.
func broadcastTest(hub *Hub, t string, topics []string) {
	hub.broadcast(Message{Type: t, Topics: topics, Payload: map[string]any{"x": 1}, OccurredAt: time.Now()})
}

func TestHubDeliversOnlyToMatchingSubjects(t *testing.T) {
	hub := NewHub(t.Context(), nil, nil) // tanpa jembatan — broadcast dipanggil manual

	subTrack, cancelTrack := hub.Subscribe([]string{TrackChannel("RESI-1")})
	subOrg, cancelOrg := hub.Subscribe([]string{OrgShipmentsChannel("ORG-1")})
	subBoth, cancelBoth := hub.Subscribe([]string{TrackChannel("RESI-1"), OrgShipmentsChannel("ORG-1")})

	broadcastTest(hub, TypeTrackingUpdated, []string{TrackChannel("RESI-1")})
	time.Sleep(10 * time.Millisecond)

	if got := recv(subTrack.C); got != 1 {
		t.Errorf("langganan track menerima %d, ingin 1", got)
	}
	if got := recv(subOrg.C); got != 0 {
		t.Errorf("langganan org menerima %d, ingin 0 (resi milik jalur lain)", got)
	}
	if got := recv(subBoth.C); got != 1 {
		t.Errorf("langganan keduanya menerima %d, ingin 1", got)
	}

	cancelTrack()
	cancelOrg()
	cancelBoth()
}

func TestHubUnsubscribeStopsDeliveryAndFreesRegistry(t *testing.T) {
	hub := NewHub(t.Context(), nil, nil)
	sub, cancel := hub.Subscribe([]string{TrackChannel("R")})
	cancel()

	hub.mu.Lock()
	names := len(hub.subs)
	hub.mu.Unlock()
	if names != 0 {
		t.Fatalf("registry tidak kosong setelah unsubscribe: %d", names)
	}

	broadcastTest(hub, TypeTrackingUpdated, []string{TrackChannel("R")})
	time.Sleep(10 * time.Millisecond)
	if got := recv(sub.C); got != 0 {
		t.Errorf("pesan dikirim ke langganan yang sudah dibatalkan (%d)", got)
	}
}

func TestHubSlowClientIsDroppedNotBlockingOthers(t *testing.T) {
	// Kli lambat tidak boleh ikut menahan hub — itu penentu bahwa satu klien
	// bermasalah tidak merusak klien lain (kondisi transparan produk).
	hub := NewHub(t.Context(), nil, nil)
	slow, _ := hub.Subscribe([]string{TrackChannel("R")}) // tidak pernah dibaca
	fast, cancelFast := hub.Subscribe([]string{TrackChannel("R")})
	defer cancelFast()

	for i := 0; i < 1000; i++ {
		broadcastTest(hub, TypeTrackingUpdated, []string{TrackChannel("R")})
	}

	// fast harus terus menerima (buffer 16 sudah penuh tapi tetap berjalan);
	// kalau broadcast memblokir, test akan waktu-habis.
	select {
	case msg := <-fast.C:
		if msg.Type != TypeTrackingUpdated {
			t.Errorf("tipe = %q", msg.Type)
		}
	default:
		t.Error("klien simpan tidak menerima pesan apapun")
	}
	_ = slow
}

func TestMessageJSONExcludesTopics(t *testing.T) {
	// Topics adalah detail internal kanal; bila ikut terkirim, klien bisa
	// berpura-pura menerima pesan untuk seluruh kanal sekaligus. Dipakai di
	// sisi penerima: kanal pseudonym dari kanal yang datang.
	raw, err := json.Marshal(Message{Type: TypeTrackingUpdated, Topics: []string{TrackChannel("r")}, Payload: map[string]any{}})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var m Message
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(m.Topics) != 0 {
		t.Errorf("Topics tidak seharusnya ikut JSON: %v", m.Topics)
	}
}

func TestNopNotifier(t *testing.T) {
	var n Notifier = NopNotifier{}
	if err := n.Publish(Message{}); err != nil {
		t.Errorf("no-op tidak boleh error: %v", err)
	}
}

func recv(ch chan Message) int {
	n := 0
	for {
		select {
		case <-ch:
			n++
		default:
			return n
		}
	}
}
