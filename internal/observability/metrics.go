// Registry metrik Prometheus minimal tanpa dependensi eksternal, diekspos di
// /metrics oleh Registry.Handler. Bila nanti pindah ke
// prometheus/client_golang, hanya file ini yang berubah.
package observability

import (
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
)

// Registry menyimpan counter & gauge sederhana.
type Registry struct {
	mu       sync.RWMutex
	counters map[string]float64
	gauges   map[string]float64
}

// NewRegistry membuat registry metrik kosong.
func NewRegistry() *Registry {
	return &Registry{
		counters: make(map[string]float64),
		gauges:   make(map[string]float64),
	}
}

// Inc menambah counter bernama.
func (r *Registry) Inc(name string) { r.Add(name, 1) }

// Add menambah counter sebesar delta.
func (r *Registry) Add(name string, delta float64) {
	r.mu.Lock()
	r.counters[name] += delta
	r.mu.Unlock()
}

// Set menetapkan nilai gauge.
func (r *Registry) Set(name string, v float64) {
	r.mu.Lock()
	r.gauges[name] = v
	r.mu.Unlock()
}

// Handler mengekspos metrik dalam format text Prometheus.
func (r *Registry) Handler() http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")

		r.mu.RLock()
		counters := make(map[string]float64, len(r.counters))
		for k, v := range r.counters {
			counters[k] = v
		}
		gauges := make(map[string]float64, len(r.gauges))
		for k, v := range r.gauges {
			gauges[k] = v
		}
		r.mu.RUnlock()

		var b strings.Builder
		writeSection(&b, "counter", counters)
		writeSection(&b, "gauge", gauges)
		_, _ = w.Write([]byte(b.String()))
	}
}

func writeSection(b *strings.Builder, kind string, m map[string]float64) {
	if len(m) == 0 {
		return
	}
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		name := sanitize(k)
		fmt.Fprintf(b, "# TYPE %s %s\n%s %s\n", name, kind, name, strconv.FormatFloat(m[k], 'f', -1, 64))
	}
}

// sanitize menjadikan nama metrik valid menurut aturan Prometheus.
func sanitize(name string) string {
	out := make([]rune, 0, len(name))
	for i, r := range name {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r == '_':
			out = append(out, r)
		case r >= '0' && r <= '9':
			if i == 0 {
				out = append(out, '_')
			}
			out = append(out, r)
		default:
			out = append(out, '_')
		}
	}
	return string(out)
}