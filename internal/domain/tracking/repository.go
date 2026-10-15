package tracking

import (
	"context"
	"errors"
)

// ErrDuplicate menandai event yang sudah tercatat sebelumnya. Bukan kegagalan:
// pemanggil memperlakukannya sebagai sukses idempotent (webhook dikirim ulang).
var ErrDuplicate = errors.New("event tracking sudah tercatat")

// IsDuplicate melaporkan apakah error berarti event sudah ada.
func IsDuplicate(err error) bool { return errors.Is(err, ErrDuplicate) }

// Filter menyaring daftar event.
type Filter struct {
	ShipmentID string
	Status     EventStatus
	Source     Source
	From       string // RFC3339
	To         string // RFC3339
	Limit      int
	Offset     int
}

// Repository menyimpan event tracking tenant-scoped.
//
// Tabel `tracking_events` append-only: TIDAK ada Update/Delete di antarmuka ini
// dengan sengaja. Koreksi dilakukan dengan menambah event baru.
type Repository interface {
	// Append menyimpan satu event. Implementasi harus mengembalikan
	// ErrDuplicate bila kunci idempotensi alami sudah ada, agar pemanggil
	// dapat memperlakukannya sebagai sukses (sudah tercatat) bukan kegagalan.
	Append(ctx context.Context, e *Event) error
	// ListByShipment mengembalikan event satu pengiriman, terurut waktu naik.
	ListByShipment(ctx context.Context, orgID, shipmentID string) ([]Event, error)
	// List mengembalikan halaman event dengan filter.
	List(ctx context.Context, orgID string, f Filter) ([]Event, int, error)
}

// ShipmentReader adalah potongan repository shipment yang dibutuhkan tracking
// (menghindari impor langsung ke paket database).
type ShipmentReader interface {
	FindShipmentIDByAWB(ctx context.Context, orgID, awb string) (shipmentID string, err error)
}
