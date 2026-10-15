package event

import "context"

// Publisher menulis event ke outbox (dalam transaksi bisnis) dan relay yang
// membacanya. Keduanya dipisah karena dipakai oleh proses berbeda: API menulis,
// worker merelay.
type Publisher interface {
	// Publish menyimpan event ke outbox. Implementasi WAJIB dipanggil dari
	// dalam transaksi bisnis yang sama agar atomik (ADR-005).
	Publish(ctx context.Context, e Event) error
}

// Relay membaca outbox dan menerbitkannya ke broker.
type RelayRepository interface {
	// Claim mengambil sejumlah baris pending yang sudah waktunya (available_at),
	// paling lama dulu, dan menandainya sedang dikerjakan agar aman dipakai
	// beberapa replika relay sekaligus.
	Claim(ctx context.Context, limit int) ([]Record, error)
	// MarkPublished menandai baris berhasil diterbitkan.
	MarkPublished(ctx context.Context, id int64) error
	// MarkFailed mencatat kegagalan; retryAfter menentukan kapan boleh dicoba
	// lagi (backoff). attempts melewati batas → status dead.
	MarkFailed(ctx context.Context, id int64, reason string, retryAfter int) error
}