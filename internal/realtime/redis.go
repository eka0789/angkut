// Package realtime — implementasi penerbit pesan ke Redis pub/sub.
package realtime

import (
	"context"
	"encoding/json"
	"time"

	"github.com/redis/go-redis/v9"
)

// RedisPublisher menerbitkan Message ke kanal Redis.
type RedisPublisher struct {
	rdb redis.UniversalClient
}

// NewRedisPublisher membuat penerbit. Bila rdb kosong, mengembalikan nil —
// worker/API tetap hidup tanpa Redis dengan log peringatan yang sebelumnya
// sudah diberikan di perakit.
//
// Parameter sengaja bertipe konkret *redis.Client, bukan redis.UniversalClient:
// klien kosong datang sebagai pointer nil, dan bila dibungkus interface nilai
// `== nil`-nya menjadi palsu (interface berisi pointer nil) sehingga cek di
// Publish lolos dan pemanggilan berikutnya panic di dalam driver.
func NewRedisPublisher(rdb *redis.Client) *RedisPublisher {
	if rdb == nil {
		return nil
	}
	return &RedisPublisher{rdb: rdb}
}

// Publish mem-serialize pesan ke JSON lalu menerbitkannya ke semua kanal.
//
// Konteks digabungkan dengan timeout internal agar pemanggil (mis. transaksi
// yang baru selesai) tidak bisa tergantung lama karena Redis macet.
func (p *RedisPublisher) Publish(msg Message) error {
	if p == nil || p.rdb == nil {
		return nil // realizasi dari perjanjian best-effort.
	}
	raw, err := json.Marshal(msg)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	for _, ch := range msg.Topics {
		if err := p.rdb.Publish(ctx, ch, raw).Err(); err != nil {
			return err
		}
	}
	return nil
}

// RedisHub menjembatani pub/sub Redis dengan klien lokal (koneksi SSE/WS).
//
// Dua sifat yang penting:
//  1. go-redis PubSub menyambungkan ulang otomatis saat koneksi putus.
//  2. PSubscribe memakai pola (angkut:track:*, angkut:org:*:shipments), jadi
//     kanal baru tidak perlu dinyatakan di depan.
type RedisHub struct {
	rdb redis.UniversalClient
}

// NewRedisHub membuat penyalur. Sama seperti NewRedisPublisher, parameternya
// bertipe konkret *redis.Client supaya klien kosong terdeteksi sebagai nil
// — bukan interface berisi pointer nil yang lolos cek dan panic saat dipakai.
func NewRedisHub(rdb *redis.Client) *RedisHub {
	if rdb == nil {
		return nil
	}
	return &RedisHub{rdb: rdb}
}

// Subscribe memulai langganan pola dan mengembalikan penyalur pesan.
//
// Pesan yang datang memuat SATU kanal karena Topics tidak ikut serialisasi —
// setiap kanal target diterbitkan tersendiri, dan sisi penerima menandai
// lalu lintasnya dengan kanal datangnya itu sendiri. Klien SSE/WS MEMILIH SATU
// subjek per koneksi di handler-nya sehingga pesan ganda (silang kanal) tidak
// mungkin.
func (h *RedisHub) Subscribe(ctx context.Context, patterns []string) (<-chan Message, func(), error) {
	if h == nil || h.rdb == nil {
		return nil, nil, nil // TIDAK ADA — pemanggil menurunkan mode.
	}
	sub := h.rdb.Subscribe(ctx, patterns...)
	out := make(chan Message, 16)
	go func() {
		defer close(out)
		defer sub.Close()
		for {
			received, err := sub.Receive(ctx)
			if err != nil {
				return // ctx dibatal — hub berhenti bersama server.
			}
			pm, ok := received.(redis.Message)
			if !ok {
				continue
			}
			var msg Message
			if jsonErr := json.Unmarshal([]byte(pm.Payload), &msg); jsonErr != nil {
				// Payload tidak dapat diuraikan tidak boleh MENGGAGALKAN
				// langganan lain — lewati.
				continue
			}
			msg.Topics = []string{pm.Channel}
			select {
			case out <- msg:
			case <-ctx.Done():
				return
			}
		}
	}()
	return out, func() { sub.Close() }, nil
}
