// Package health menyediakan checker readiness untuk dependensi.
package health

import (
	"context"
	"net/http"
	"time"
)

// Pinger adalah apa pun yang bisa di-ping (pgx pool, redis client).
type Pinger interface {
	Ping(ctx context.Context) error
}

// Dependency membungkus Pinger menjadi handler.Checker.
type Dependency struct {
	name    string
	pinger  Pinger
	timeout time.Duration
}

// New membuat checker dengan timeout default 2 detik.
func New(name string, p Pinger) *Dependency {
	return &Dependency{name: name, pinger: p, timeout: 2 * time.Second}
}

// Name mengembalikan nama dependensi.
func (d *Dependency) Name() string { return d.name }

// Check menjalankan ping dengan timeout; nil berarti siap.
func (d *Dependency) Check(r *http.Request) error {
	if d.pinger == nil {
		return nil
	}
	ctx, cancel := context.WithTimeout(r.Context(), d.timeout)
	defer cancel()
	return d.pinger.Ping(ctx)
}
