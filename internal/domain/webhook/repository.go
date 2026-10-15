package webhook

import "context"

// IngestInput adalah permintaan menyimpan satu event masuk. RawBody adalah
// byte ASLI (signature dihitung atas ini, bukan atas payload hasil parse).
type IngestInput struct {
	Provider    string
	RawBody     []byte
	Payload     map[string]any
	ContentType string
	Headers     map[string]any
	SourceIP    string
}

// Result menjelaskan apa yang terjadi saat event diterima.
type Result struct {
	Event     *Event
	Duplicate bool // sudah pernah diterima — bukan kegagalan
}

// Repository menyimpan event webhook masuk.
type Repository interface {
	// Store menyimpan event; mengembalikan ErrDuplicate bila (provider,event_id)
	// atau (provider,payload_hash) sudah ada.
	Store(ctx context.Context, e *Event) error
	GetByID(ctx context.Context, id string) (*Event, error)
	// FindByProviderEventID mencari event dengan event_id provider tertentu.
	// Mengembalikan ErrNotFound bila belum ada — dipakai pra-cek dedup di luar
	// transaksi (setelah unique violation, transaksi Postgres tidak bisa commit).
	FindByProviderEventID(ctx context.Context, provider, eventID string) (*Event, error)
	// FindByProviderHash mencari event dari hash payload; dipakai bila provider
	// tidak mengirim event_id.
	FindByProviderHash(ctx context.Context, provider, payloadHash string) (*Event, error)
	// MarkProcessed/MarkFailed dipakai consumer setelah pemrosesan.
	MarkProcessed(ctx context.Context, id string) error
	MarkFailed(ctx context.Context, id string, reason string) error
	// MarkIgnored menandai event yang sengaja tidak diproses (mis. resi tidak
	// dikenal). Berbeda dari failed: tidak ada yang rusak, hanya tidak berlaku —
	// pemisahan ini penting agar operator tidak mengejar masalah yang tak ada.
	MarkIgnored(ctx context.Context, id, reason string) error
	// ClaimPending mengambil event yang belum diproses untuk diproses worker.
	ClaimPending(ctx context.Context, limit int) ([]Event, error)
	List(ctx context.Context, provider string, status Status, limit, offset int) ([]Event, int, error)
}

// Secrets menyimpan rahasia signature per provider (terenkripsi at-rest).
// Nilai mentah hanya dibuka di sini untuk keperluan verifikasi.
type Secrets interface {
	// SecretFor mengembalikan rahasia provider dalam bentuk terbuka.
	// Mengembalikan ErrSecretless bila provider belum punya rahasia.
	SecretFor(ctx context.Context, provider string) (secret string, algo Algo, header string, err error)
}
