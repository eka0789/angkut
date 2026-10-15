package consumer

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"

	"github.com/angkut/angkut/apps/internal/infrastructure/broker"
)

// fakeDeduper meniru tabel processed_events di memori.
type fakeDeduper struct {
	mu       sync.Mutex
	seen     map[string]bool
	calls    int
	failWith error
}

func newFakeDeduper() *fakeDeduper {
	return &fakeDeduper{seen: map[string]bool{}}
}

func (f *fakeDeduper) MarkProcessed(_ context.Context, eventID, _ string) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls++
	if f.failWith != nil {
		return false, f.failWith
	}
	if f.seen[eventID] {
		return false, nil
	}
	f.seen[eventID] = true
	return true, nil
}

func TestParseMessageExtractsHeadersAndPayload(t *testing.T) {
	d := amqp.Delivery{
		RoutingKey: "shipment.delivered",
		Body:       []byte(`{"shipment_id":"s1","status":"delivered"}`),
		Headers: amqp.Table{
			"event_id":   "evt-1",
			"event_type": "shipment.delivered",
			"attempt":    int32(2),
		},
	}
	msg := parseMessage(d)

	if msg.ID != "evt-1" {
		t.Errorf("id = %q, ingin evt-1", msg.ID)
	}
	if msg.Type != "shipment.delivered" {
		t.Errorf("type = %q", msg.Type)
	}
	if msg.Attempt != 2 {
		t.Errorf("attempt = %d, ingin 2", msg.Attempt)
	}
	if msg.Payload["shipment_id"] != "s1" {
		t.Errorf("payload tidak terurai: %+v", msg.Payload)
	}
}

func TestParseMessageFallsBackToMessageID(t *testing.T) {
	// Tanpa header event_id, identitas harus tetap diturunkan agar dedup jalan.
	d := amqp.Delivery{
		RoutingKey: "shipment.delivered",
		MessageId:  "m-9",
		Body:       []byte(`{}`),
	}
	if got := parseMessage(d).ID; got != "shipment.delivered:m-9" {
		t.Errorf("id fallback = %q", got)
	}
}

func TestParseMessageSurvivesInvalidJSON(t *testing.T) {
	// Body bukan JSON tidak boleh panic; payload kosong dan handler yang
	// memutuskan (biasanya Permanent).
	d := amqp.Delivery{RoutingKey: "x.y", Body: []byte(`bukan json`)}
	msg := parseMessage(d)
	if msg.Payload != nil {
		t.Errorf("payload seharusnya nil, dapat %+v", msg.Payload)
	}
	if string(msg.Body) != "bukan json" {
		t.Error("body mentah harus tetap tersedia")
	}
}

func TestParseMessageReadsTypeFromPayload(t *testing.T) {
	d := amqp.Delivery{
		RoutingKey: "x.y",
		Body:       []byte(`{"event_type":"order.created"}`),
	}
	if got := parseMessage(d).Type; got != "order.created" {
		t.Errorf("type dari payload = %q", got)
	}
}

func TestHeaderIntAcceptsNumericTypes(t *testing.T) {
	cases := []struct {
		in   any
		want int
	}{
		{int(3), 3},
		{int32(4), 4},
		{int64(5), 5},
		{float64(6), 6},
		{"7", 0}, // string bukan angka → 0, tidak menebak
		{nil, 0},
	}
	for _, tc := range cases {
		h := amqp.Table{"attempt": tc.in}
		if got := headerInt(h, "attempt"); got != tc.want {
			t.Errorf("headerInt(%v) = %d, ingin %d", tc.in, got, tc.want)
		}
	}
	if got := headerInt(nil, "attempt"); got != 0 {
		t.Errorf("headerInt(nil) = %d", got)
	}
}

func TestIsPermanent(t *testing.T) {
	if !IsPermanent(Permanent(errors.New("bentuk payload rusak"))) {
		t.Error("error yang dibungkus Permanent harus terdeteksi")
	}
	if IsPermanent(errors.New("gangguan sementara")) {
		t.Error("error biasa bukan permanen")
	}
	// errors.As harus bisa menembus pembungkusan berlapis.
	wrapped := errors.Join(errors.New("konteks"), Permanent(errors.New("inti")))
	if !IsPermanent(wrapped) {
		t.Error("Permanent di dalam rantai error harus terdeteksi")
	}
}

func TestDeduperContractBlocksSecondDelivery(t *testing.T) {
	// Kontrak inti "webhook duplikat diproses sekali": pemanggilan kedua
	// dengan event_id sama harus melaporkan sudah pernah diproses.
	d := newFakeDeduper()
	ctx := context.Background()

	first, err := d.MarkProcessed(ctx, "evt-1", "tracking-worker")
	if err != nil || !first {
		t.Fatalf("pertama kali: (%v,%v), ingin (true,nil)", first, err)
	}
	second, err := d.MarkProcessed(ctx, "evt-1", "tracking-worker")
	if err != nil {
		t.Fatalf("kedua kali: %v", err)
	}
	if second {
		t.Error("kedua kali harus melaporkan duplikat (false)")
	}
}

func TestDeduperFailurePropagates(t *testing.T) {
	// Bila dedup tidak bisa dipastikan, pemanggil HARUS tahu — bukan diam-diam
	// memproses (yang bisa berarti efek ganda).
	d := newFakeDeduper()
	d.failWith = errors.New("database tidak tersedia")

	if _, err := d.MarkProcessed(context.Background(), "evt-1", "w"); err == nil {
		t.Fatal("kegagalan dedup harus dilaporkan")
	}
}

func TestRetryDelayForMatchesBrokerSchedule(t *testing.T) {
	// Jeda yang dilaporkan consumer harus sama dengan jadwal broker.
	cfg := retryTestConfig()
	d, ok := RetryDelayFor(cfg, 1)
	if !ok || d <= 0 {
		t.Fatalf("attempt 1: (%v,%v)", d, ok)
	}
	if _, ok := RetryDelayFor(cfg, 99); ok {
		t.Error("attempt di luar jadwal harus (0,false) → DLQ")
	}
}

func TestPermanentErrorUnwraps(t *testing.T) {
	inner := errors.New("sebab asli")
	err := Permanent(inner)
	if !errors.Is(err, inner) {
		t.Error("PermanentError harus dapat di-unwrap ke sebab aslinya")
	}
}

// retryTestConfig menyusun konfigurasi broker untuk pengujian jadwal retry.
func retryTestConfig() broker.Config {
	return broker.Config{
		URL:         "amqp://localhost",
		RetryDelays: []time.Duration{30 * time.Second, 2 * time.Minute},
	}
}
