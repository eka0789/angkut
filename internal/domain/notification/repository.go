package notification

import "context"

// Filter menyaring daftar notifikasi milik satu pengguna.
type Filter struct {
	// Unread=true hanya mengembalikan yang belum dibaca.
	Unread bool
	// Priority membatasi prioritas tertentu (kosong = semua).
	Priority Priority
	// EventKey membatasi satu jenis event (kosong = semua).
	EventKey EventKey
	Limit    int
	Offset   int
}

// Repository menyimpan notifikasi & preferensi. Semua operasi baca notifikasi
// WAJIB memfilter user_id DAN organization_id — notifikasi adalah data pribadi
// di dalam tenant, dua lapis yang tidak boleh dilepas.
type Repository interface {
	// Create menyimpan satu notifikasi (in-app) untuk satu pengguna.
	Create(ctx context.Context, n *Notification) error
	// CreateMany menyimpan beberapa notifikasi sekaligus (satu kejadian untuk
	// banyak penerima) dalam satu panggilan.
	CreateMany(ctx context.Context, items []Notification) error
	// List mengembalikan halaman notifikasi + totalnya.
	List(ctx context.Context, orgID, userID string, f Filter) ([]Notification, int, error)
	// UnreadCount menghitung notifikasi belum dibaca.
	UnreadCount(ctx context.Context, orgID, userID string) (int, error)
	// MarkRead menandai satu notifikasi milik pengguna sebagai dibaca.
	// Mengembalikan NotFound bila bukan miliknya — bukan diam-diam sukses.
	MarkRead(ctx context.Context, orgID, userID, id string) error
	// MarkAllRead menandai semua notifikasi pengguna sebagai dibaca dan
	// mengembalikan jumlah yang berubah.
	MarkAllRead(ctx context.Context, orgID, userID string) (int, error)
}

// TemplateRepository menyimpan template pesan per organisasi.
type TemplateRepository interface {
	// List mengembalikan template yang sudah dikustomisasi organisasi.
	List(ctx context.Context, orgID string) ([]Template, error)
	// Upsert membuat/memperbarui template untuk (key, channel).
	Upsert(ctx context.Context, t *Template) error
}

// PreferenceRepository menyimpan preferensi per pengguna.
type PreferenceRepository interface {
	// List mengembalikan preferensi pengguna (hanya yang diset eksplisit).
	List(ctx context.Context, userID string) ([]Preference, error)
	// Upsert menyimpan preferensi (key, channel) pengguna.
	Upsert(ctx context.Context, p *Preference) error
}

// Recipient adalah satu penerima notifikasi in-app.
type Recipient struct {
	UserID string
	Email  string
}

// DispatchRepository adalah sisi baca untuk FAN-OUT notifikasi (dipakai saat
// kejadian bisnis terjadi): anggota organisasi + preferensi + template kustom.
// Semua query di sini berjalan DI DALAM transaksi kejadian bisnis.
type DispatchRepository interface {
	// InAppRecipients mengembalikan anggota aktif organisasi yang notifikasi
	// in-app-nya AKTIF untuk event ini. Default tanpa preferensi eksplisit =
	// aktif (organisasi satu-orang tetap menerima pembaruan).
	InAppRecipients(ctx context.Context, orgID string, key EventKey) ([]Recipient, error)
	// EmailRecipients mengembalikan anggota aktif yang OPT-IN email untuk
	// event ini. Default tanpa preferensi eksplisit = nonaktif (email mahal
	// dan mudah jadi spam; opt-in dilakukan secara sadar).
	EmailRecipients(ctx context.Context, orgID string, key EventKey) ([]Recipient, error)
	// CustomTemplate mengembalikan template kustom organisasi, bila ada.
	CustomTemplate(ctx context.Context, orgID string, key EventKey, ch Channel) (*Template, bool, error)
}
