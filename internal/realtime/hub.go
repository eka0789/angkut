// Hub adalah penyalur pesan antar-klien dalam satu proses API.
//
// Alur: kanal Redis → Hub → semua koneksi lokal yang berlangganan subjek
// tersebut. Hub satu-satunya pemilik registry klien; handler SSE/WS hanya
// berlangganan dan berhenti, tidak menyentuh registry secara langsung.
package realtime

import (
	"context"
	"sync"
)

// Hub membagi pesan dari Redis ke koneksi lokal yang relevan.
type Hub struct {
	mu   sync.Mutex
	subs map[*Subscription]map[string]struct{}
}

// Subscription adalah satu klien terhubung.
type Subscription struct {
	C chan Message
}

// NewHub membuat hub dan (bila bridge != nil) menyalakan jembatan Redis.
//
// Bila Redis tidak tersedia, hub tetap hidup — hanya saja tidak ada pesan yang
// datang, dan SSE/WS tetap mengirim snapshot awal. Ini membuat mode dev tanpa
// Redis bekerja tanpa cabang khusus di handler.
func NewHub(ctx context.Context, bridge *RedisHub, patterns []string) *Hub {
	h := &Hub{subs: make(map[*Subscription]map[string]struct{})}
	if bridge == nil {
		return h
	}
	events, _, err := bridge.Subscribe(ctx, patterns)
	if err != nil || events == nil {
		return h
	}
	go func() {
		for msg := range events {
			h.broadcast(msg)
		}
	}()
	return h
}

// Subscribe mendaftarkan klien yang tertarik pada daftar subjek.
//
// Fungsi yang dikembalikan HARUS dipanggil saat koneksi tutup (biasanya
// defer di handler) agar registry tidak membocorkan memori.
func (h *Hub) Subscribe(subjects []string) (*Subscription, func()) {
	sub := &Subscription{C: make(chan Message, 16)}
	set := make(map[string]struct{}, len(subjects))
	for _, s := range subjects {
		set[s] = struct{}{}
	}
	h.mu.Lock()
	h.subs[sub] = set
	h.mu.Unlock()

	cancel := func() {
		h.mu.Lock()
		delete(h.subs, sub)
		h.mu.Unlock()
	}
	return sub, cancel
}

// broadcast mensampaikan pesan ke semua klien yang berlangganan salah satu
// kanal di Topics.
//
// Antrian penuh bukan kegagalan koneksi: pesan dibuang tanpa memblokir hub.
// Pub/Sub bukan antrean; kebenaran data tetap didapat klien dari snapshot REST
// saat tersambung, dan EventSource menyambung ulang otomatis saat jeda.
func (h *Hub) broadcast(msg Message) {
	if len(msg.Topics) == 0 {
		return
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	for sub, set := range h.subs {
		match := false
		for _, t := range msg.Topics {
			if _, ok := set[t]; ok {
				match = true
				break
			}
		}
		if !match {
			continue
		}
		select {
		case sub.C <- msg:
		default:
		}
	}
}

// Bridges menyatukan kebutuhan konfigurasi jembatan: pola yang dilangganan
// adalah semuanya (per-resi dan per-organisasi) sejak hub menyaring sisanya.
func Patterns() []string {
	return []string{ChannelTrackPrefix + "*", ChannelOrgPrefix + "*"}
}
