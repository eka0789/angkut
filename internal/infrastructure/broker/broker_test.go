package broker

import (
	"testing"
	"time"
)

func TestRetryDelaySchedule(t *testing.T) {
	cfg := DefaultConfig("amqp://localhost")

	// Tiga percobaan sesuai jadwal default.
	if d, ok := cfg.RetryDelay(1); !ok || d != 30*time.Second {
		t.Errorf("attempt 1 = (%v,%v), ingin (30s,true)", d, ok)
	}
	if d, ok := cfg.RetryDelay(2); !ok || d != 2*time.Minute {
		t.Errorf("attempt 2 = (%v,%v), ingin (2m,true)", d, ok)
	}
	if d, ok := cfg.RetryDelay(3); !ok || d != 10*time.Minute {
		t.Errorf("attempt 3 = (%v,%v), ingin (10m,true)", d, ok)
	}
	// Percobaan ke-4 tidak ada jadwalnya → harus ke DLQ.
	if _, ok := cfg.RetryDelay(4); ok {
		t.Error("attempt 4 harus kehabisan jadwal (menuju DLQ)")
	}
	if _, ok := cfg.RetryDelay(0); ok {
		t.Error("attempt 0 tidak valid")
	}
	if _, ok := cfg.RetryDelay(-1); ok {
		t.Error("attempt negatif tidak valid")
	}
}

func TestRetryDelayScheduleIsExhaustible(t *testing.T) {
	// Kontrak penting: jadwal retry HARUS berakhir, agar pesan bermasalah
	// tidak berputar selamanya dan akhirnya sampai ke DLQ.
	cfg := DefaultConfig("amqp://localhost")
	attempts := 0
	for i := 1; i <= 100; i++ {
		if _, ok := cfg.RetryDelay(i); !ok {
			break
		}
		attempts++
	}
	if attempts != len(cfg.RetryDelays) {
		t.Errorf("jumlah percobaan = %d, ingin %d", attempts, len(cfg.RetryDelays))
	}
	if attempts == 0 {
		t.Fatal("harus ada minimal satu percobaan ulang")
	}
}

func TestRetryQueueNameIsDistinctPerDelay(t *testing.T) {
	seen := map[string]bool{}
	for _, d := range []time.Duration{30 * time.Second, 2 * time.Minute, 10 * time.Minute} {
		name := RetryQueueName(d)
		if seen[name] {
			t.Errorf("nama queue duplikat: %s", name)
		}
		seen[name] = true
	}
	// Nama harus memuat jeda dalam ms agar operator dapat mengenalinya.
	if got := RetryQueueName(30 * time.Second); got != "angkut.events.retry.30000ms" {
		t.Errorf("nama queue retry = %q", got)
	}
	if got := RetryQueueName(2 * time.Minute); got != "angkut.events.retry.120000ms" {
		t.Errorf("nama queue retry = %q", got)
	}
}

func TestExchangeAndQueueNamesAreStable(t *testing.T) {
	// Nama-nama ini adalah kontrak operasional (dashboard, alert, runbook).
	// Perubahannya harus disengaja, bukan efek samping refactor.
	cases := map[string]string{
		ExchangeEvents: "angkut.events",
		ExchangeRetry:  "angkut.events.retry",
		ExchangeDLX:    "angkut.events.dlx",
		QueueDLQ:       "angkut.events.dlq",
	}
	for got, want := range cases {
		if got != want {
			t.Errorf("nama = %q, ingin %q", got, want)
		}
	}
}

func TestDialRejectsEmptyURL(t *testing.T) {
	if _, err := Dial(Config{URL: ""}); err == nil {
		t.Fatal("URL kosong harus ditolak, bukan mencoba menyambung")
	}
}
