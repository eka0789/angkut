package middleware

import (
	"net/http"
	"sync"
	"time"
)

// MemoryLimiter adalah token bucket in-memory. Cukup untuk dev & single node.
// Saat produksi multi-node, ganti dengan implementasi Redis tanpa mengubah
// pemanggil (kontrak Limiter tetap).
type MemoryLimiter struct {
	mu       sync.Mutex
	buckets  map[string]*bucket
	capacity int
	refill   time.Duration // waktu untuk mengisi 1 token
}

type bucket struct {
	tokens float64
	last   time.Time
}

// NewMemoryLimiter membuat limiter dengan batas perMenit (kapasitas = perMenit,
// refill merata sepanjang satu menit).
func NewMemoryLimiter(perMenit int) *MemoryLimiter {
	if perMenit <= 0 {
		perMenit = 60
	}
	return &MemoryLimiter{
		buckets:  make(map[string]*bucket),
		capacity: perMenit,
		refill:   time.Minute / time.Duration(perMenit),
	}
}

func (m *MemoryLimiter) Allow(key string, now time.Time) (bool, int, time.Duration) {
	m.mu.Lock()
	defer m.mu.Unlock()

	b, ok := m.buckets[key]
	if !ok {
		b = &bucket{tokens: float64(m.capacity), last: now}
		m.buckets[key] = b
	}
	// Isi ulang sesuai waktu yang berlalu.
	elapsed := now.Sub(b.last)
	if elapsed > 0 {
		b.tokens += elapsed.Seconds() / m.refill.Seconds()
		if b.tokens > float64(m.capacity) {
			b.tokens = float64(m.capacity)
		}
		b.last = now
	}
	if b.tokens >= 1 {
		b.tokens--
		return true, int(b.tokens), 0
	}
	need := 1 - b.tokens
	wait := time.Duration(need * m.refill.Seconds() * float64(time.Second))
	return false, 0, wait
}

// GC membuang bucket yang sudah penuh (jarang dipakai) untuk mencegah kebocoran
// memori pada kunci IP yang terus berganti.
func (m *MemoryLimiter) GC(now time.Time) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for k, b := range m.buckets {
		if now.Sub(b.last) > 10*time.Minute {
			delete(m.buckets, k)
		}
	}
}

var _ = http.StatusOK
