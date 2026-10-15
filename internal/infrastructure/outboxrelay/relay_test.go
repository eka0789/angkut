package outboxrelay

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/angkut/angkut/apps/internal/domain/event"
)

// fakeRepo menyimpan status baris outbox di memori agar perilaku relay dapat
// diperiksa tanpa database.
type fakeRepo struct {
	mu        sync.Mutex
	records   []event.Record
	published map[int64]bool
	failed    map[int64]string
	attempts  map[int64]int
	recovered int64
	claimErr  error
}

func newFakeRepo(records ...event.Record) *fakeRepo {
	return &fakeRepo{
		records:   records,
		published: map[int64]bool{},
		failed:    map[int64]string{},
		attempts:  map[int64]int{},
	}
}

func (f *fakeRepo) Claim(_ context.Context, limit int) ([]event.Record, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.claimErr != nil {
		return nil, f.claimErr
	}
	out := make([]event.Record, 0, limit)
	for _, r := range f.records {
		// Sudah terbit atau sudah gagal (menunggu backoff) → tidak diklaim lagi.
		if f.published[r.ID] {
			continue
		}
		if _, ok := f.failed[r.ID]; ok {
			continue
		}
		out = append(out, r)
		if len(out) == limit {
			break
		}
	}
	return out, nil
}

func (f *fakeRepo) MarkPublished(_ context.Context, id int64) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.published[id] = true
	delete(f.failed, id)
	return nil
}

func (f *fakeRepo) MarkFailed(_ context.Context, id int64, reason string, _ int) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.failed[id] = reason
	f.attempts[id]++
	return nil
}

func (f *fakeRepo) RecoverStaleClaims(context.Context, time.Duration) (int64, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.recovered, nil
}

// fakePublisher merekam apa yang diterbitkan dan dapat disuruh gagal.
type fakePublisher struct {
	mu       sync.Mutex
	routes   []string
	bodies   [][]byte
	headers  []map[string]any
	failWith error
}

func (f *fakePublisher) Publish(_ context.Context, routingKey string, body []byte, headers map[string]any) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.failWith != nil {
		return f.failWith
	}
	f.routes = append(f.routes, routingKey)
	f.bodies = append(f.bodies, body)
	f.headers = append(f.headers, headers)
	return nil
}

func sampleRecords(n int) []event.Record {
	out := make([]event.Record, 0, n)
	for i := 1; i <= n; i++ {
		out = append(out, event.Record{
			ID:            int64(i),
			AggregateType: "shipment",
			AggregateID:   "ship-1",
			Type:          event.ShipmentDelivered,
			Payload:       map[string]any{"awb": "JNE1", "n": i},
			CreatedAt:     time.Now().UTC(),
		})
	}
	return out
}

func TestDrainPublishesAndMarksPublished(t *testing.T) {
	repo := newFakeRepo(sampleRecords(3)...)
	pub := &fakePublisher{}
	relay := New(DefaultConfig(), repo, pub, nil)

	published, failed, err := relay.drain(context.Background())
	if err != nil {
		t.Fatalf("drain: %v", err)
	}
	if published != 3 || failed != 0 {
		t.Fatalf("terbit/gagal = %d/%d, ingin 3/0", published, failed)
	}
	if len(pub.routes) != 3 {
		t.Fatalf("publikasi = %d, ingin 3", len(pub.routes))
	}
	// Routing key harus sama dengan nama event (katalog routing).
	for _, r := range pub.routes {
		if r != string(event.ShipmentDelivered) {
			t.Errorf("routing key = %q, ingin %q", r, event.ShipmentDelivered)
		}
	}
	for id := range repo.published {
		if !repo.published[id] {
			t.Errorf("baris %d tidak ditandai published", id)
		}
	}
}

func TestDrainLeavesFailedRowsUnpublished(t *testing.T) {
	// Inti janji ADR-005: kegagalan TIDAK menandai baris sebagai terbit,
	// sehingga event tidak hilang.
	repo := newFakeRepo(sampleRecords(2)...)
	pub := &fakePublisher{failWith: errors.New("broker mati")}
	relay := New(DefaultConfig(), repo, pub, nil)

	published, failed, err := relay.drain(context.Background())
	if err != nil {
		t.Fatalf("drain: %v", err)
	}
	if published != 0 || failed != 2 {
		t.Fatalf("terbit/gagal = %d/%d, ingin 0/2", published, failed)
	}
	if len(repo.published) != 0 {
		t.Error("baris gagal TIDAK BOLEH ditandai published")
	}
	if len(repo.failed) != 2 {
		t.Errorf("baris gagal = %d, ingin 2", len(repo.failed))
	}
	for id, reason := range repo.failed {
		if reason == "" {
			t.Errorf("baris %d gagal tanpa alasan", id)
		}
	}
}

func TestDrainRetriesAfterFailureCleared(t *testing.T) {
	// Setelah backoff habis (baris tidak lagi ditandai failed), relay harus
	// mencoba lagi dan berhasil.
	repo := newFakeRepo(sampleRecords(1)...)
	pub := &fakePublisher{failWith: errors.New("broker mati")}
	relay := New(DefaultConfig(), repo, pub, nil)

	if _, failed, _ := relay.drain(context.Background()); failed != 1 {
		t.Fatal("percobaan pertama seharusnya gagal")
	}

	// Simulasi backoff selesai.
	repo.mu.Lock()
	repo.failed = map[int64]string{}
	repo.mu.Unlock()
	pub.mu.Lock()
	pub.failWith = nil
	pub.mu.Unlock()

	published, failed, err := relay.drain(context.Background())
	if err != nil {
		t.Fatalf("drain kedua: %v", err)
	}
	if published != 1 || failed != 0 {
		t.Fatalf("terbit/gagal = %d/%d, ingin 1/0", published, failed)
	}
}

func TestDrainHandlesMultipleBatches(t *testing.T) {
	cfg := DefaultConfig()
	cfg.BatchSize = 2
	repo := newFakeRepo(sampleRecords(5)...)
	pub := &fakePublisher{}
	relay := New(cfg, repo, pub, nil)

	published, failed, err := relay.drain(context.Background())
	if err != nil {
		t.Fatalf("drain: %v", err)
	}
	if published != 5 || failed != 0 {
		t.Fatalf("terbit/gagal = %d/%d, ingin 5/0", published, failed)
	}
}

func TestDrainStopsOnClaimError(t *testing.T) {
	repo := newFakeRepo(sampleRecords(1)...)
	repo.claimErr = errors.New("database tidak tersedia")
	pub := &fakePublisher{}
	relay := New(DefaultConfig(), repo, pub, nil)

	if _, _, err := relay.drain(context.Background()); err == nil {
		t.Fatal("error klaim harus diteruskan ke pemanggil")
	}
	if len(pub.routes) != 0 {
		t.Error("tidak boleh ada publikasi ketika klaim gagal")
	}
}

func TestDrainWithEmptyOutbox(t *testing.T) {
	repo := newFakeRepo()
	pub := &fakePublisher{}
	relay := New(DefaultConfig(), repo, pub, nil)

	published, failed, err := relay.drain(context.Background())
	if err != nil {
		t.Fatalf("drain: %v", err)
	}
	if published != 0 || failed != 0 {
		t.Errorf("outbox kosong: terbit/gagal = %d/%d", published, failed)
	}
}

func TestRunStopsOnContextCancel(t *testing.T) {
	repo := newFakeRepo(sampleRecords(1)...)
	pub := &fakePublisher{}
	cfg := DefaultConfig()
	cfg.Interval = 10 * time.Millisecond
	relay := New(cfg, repo, pub, nil)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- relay.Run(ctx) }()

	// Beri kesempatan satu putaran berjalan.
	time.Sleep(50 * time.Millisecond)
	cancel()

	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Errorf("error = %v, ingin context.Canceled", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("relay tidak berhenti setelah ctx dibatalkan")
	}
	if len(pub.routes) == 0 {
		t.Error("relay tidak menerbitkan apa pun sebelum berhenti")
	}
}

func TestPublishMarksFailedWhenPayloadUnencodable(t *testing.T) {
	// Payload berisi nilai yang tidak bisa disandikan JSON (channel) — relay
	// harus menandai gagal, bukan berputar selamanya.
	repo := newFakeRepo()
	pub := &fakePublisher{}
	relay := New(DefaultConfig(), repo, pub, nil)

	rec := event.Record{
		ID:      99,
		Type:    event.ShipmentDelivered,
		Payload: map[string]any{"bad": make(chan int)},
	}
	if err := relay.Publish(context.Background(), rec); err == nil {
		t.Fatal("payload tak tersandikan harus menghasilkan error")
	}
	if len(repo.failed) != 1 {
		t.Error("baris harus ditandai gagal agar tidak berputar selamanya")
	}
}

func TestPublishHeadersCarryAttemptCount(t *testing.T) {
	repo := newFakeRepo()
	pub := &fakePublisher{}
	relay := New(DefaultConfig(), repo, pub, nil)

	rec := sampleRecords(1)[0]
	rec.Attempts = 2
	rec.Headers = map[string]any{"event_id": "evt-1"}
	if err := relay.Publish(context.Background(), rec); err != nil {
		t.Fatalf("publish: %v", err)
	}
	if len(pub.headers) != 1 {
		t.Fatal("header tidak terkirim")
	}
	if got := pub.headers[0]["attempt"]; got != 2 {
		t.Errorf("header attempt = %v, ingin 2 (agar consumer tahu percobaan ke berapa)", got)
	}
	if pub.headers[0]["event_id"] != "evt-1" {
		t.Error("header asli dari outbox harus diteruskan")
	}
}
