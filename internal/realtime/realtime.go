// Package realtime menyediakan pengiriman acara "hidup" dari server ke klien.
//
// Arsitekturnya (ADR-005 + Realtime Tracking flow):
//
//	peristiwa menyimpan state (bukti) → publish ke Redis pub/sub
//	  → [hub per proses API] → klien (SSE) → klien (WS)
//
// Redis adalah rele antar proses: worker yang memperbarui tracking TIDAK perlu
// tahu klien mana yang online; hub di setiap proses API yang menerjemahkan
// pesan Redis ke koneksi klien yang sedang terbuka.
//
// Prinsip penting: PUBLISH BERSIFAT BEST-EFFORT. Pub/Sub Redis bukan antrean —
// pesan tidak lama bila tidak ada pelanggan. Bacaan sumber kebenaran selalu
// tersambung lewat REST (snapshot awal SSE/WS), sehingga kejadian yang hilang
// hanya merugikan kecepatan, tidak merusak data.
package realtime

import "time"

// Message adalah peristiwa yang disiarkan. Topics adalah daftar kanal Redis
// tempat pesan dipublikasikan; di sisi klien, Subjek dipakai untuk menyaring
// pesan yang diterima.
type Message struct {
	Type       string         `json:"type"` // "tracking.updated", "bulk.progress", ...
	Topics     []string       `json:"-"`    // kanal Redis (di dalam, bukan di klien)
	Payload    map[string]any `json:"payload"`
	OccurredAt time.Time      `json:"occurred_at"`
}

// Kanal Redis. PATOKAN akses baca-masuk? tidak — semua kanal mensyaratkan
// otentifikasi di sisi HTTP; Redis hanya di dalam jaringan internal.
const (
	// ChannelTrackPrefix + AWB: kanal per-resi (dipakai SSE /tracking/{awb}/stream).
	ChannelTrackPrefix = "angkut:track:"
	// ChannelOrgPrefix + orgID + ":shipments": kanal per-organisasi
	// (dipakai WS ?channels=shipments).
	ChannelOrgPrefix = "angkut:org:"
	// ChannelOrgSuffix menandai pesan pengiriman untuk organisasi tersebut.
	ChannelOrgSuffix = ":shipments"
)

// TrackChannel mengembalikan nama kanal Redis untuk satu resi.
func TrackChannel(awb string) string {
	return ChannelTrackPrefix + awb
}

// OrgShipmentsChannel mengembalikan nama kanal Redis untuk organisasi.
func OrgShipmentsChannel(orgID string) string {
	return ChannelOrgPrefix + orgID + ChannelOrgSuffix
}

// Pesan tipe yang dipahami klien saat ini.
const (
	// TypeTrackingUpdated berisi timeline terbaru untuk satu resi.
	TypeTrackingUpdated = "tracking.updated"
	// TypeBulkProgress berisi kemajuan pekerjaan massal untuk satu organisasi.
	TypeBulkProgress = "bulk.progress"
)

// Notifier adalah port untuk menyiarkan pesan. Implementasi no-op dipakai
// bila Redis tidak dikonfigurasi — kode pemanggil tidak perlu cabang khusus.
type Notifier interface {
	// Publish mengirim pesan ke semua kanal di Topics. Harus aman dipanggil
	// saat transaksi baru saja COMMIT (publish gagal tidak menasibiri state).
	Publish(msg Message) error
}

// NopNotifier menelan semua publish — dipakai saat realtime tidak aktif.
type NopNotifier struct{}

func (NopNotifier) Publish(Message) error { return nil }
