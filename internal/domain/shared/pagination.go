package shared

// Pagination mendukung dua gaya: kursor (default, untuk feed besar) dan offset
// (untuk tabel admin). Kontraknya ada di planning/api-contract.md.
type Pagination struct {
	Limit  int    // 1..100, default 20
	Cursor string // opaque cursor (base64 dari keyset)
	Page   int    // hanya bila memakai mode offset
}

// DefaultLimit dan MaxLimit membatasi biaya query tanpa perlu tanya client.
const (
	DefaultLimit = 20
	MaxLimit     = 100
)

// Normalize membetulkan nilai di luar rentang tanpa menolak request.
func (p *Pagination) Normalize() {
	if p.Limit <= 0 {
		p.Limit = DefaultLimit
	}
	if p.Limit > MaxLimit {
		p.Limit = MaxLimit
	}
	if p.Cursor == "" && p.Page <= 0 {
		p.Page = 1
	}
}

// PageMeta adalah bagian meta pada envelope respons daftar.
type PageMeta struct {
	NextCursor string `json:"next_cursor,omitempty"`
	HasMore    bool   `json:"has_more"`
	Limit      int    `json:"limit"`
}

// OffsetMeta dipakai pada endpoint bercorak tabel (page/per_page/total).
type OffsetMeta struct {
	CurrentPage int `json:"current_page"`
	LastPage    int `json:"last_page"`
	PerPage     int `json:"per_page"`
	Total       int `json:"total"`
}
