package platform

import "time"

// nowUnixMilli dipisah agar mudah di-stub di test.
var nowUnixMilli = func() int64 { return time.Now().UnixMilli() }

// Clock memudahkan pengujian waktu (eta, SLA, TTL) tanpa time.Sleep.
type Clock interface {
	Now() time.Time
}

// RealClock memakai waktu sistem.
type RealClock struct{}

func (RealClock) Now() time.Time { return time.Now() }

// FixedClock dipakai di test.
type FixedClock struct{ T time.Time }

func (c FixedClock) Now() time.Time { return c.T }

// Timezone aplikasi: seluruh tampilan pengguna memakai waktu Indonesia.
// Penyimpanan tetap UTC di DB.
var DefaultLocation = func() *time.Location {
	loc, err := time.LoadLocation("Asia/Jakarta")
	if err != nil {
		return time.FixedZone("WIB", 7*60*60)
	}
	return loc
}()

// InAppLocation mengonversi waktu ke zona aplikasi.
func InAppLocation(t time.Time) time.Time { return t.In(DefaultLocation) }

// StartOfDay mengembalikan 00:00:00 waktu aplikasi untuk tanggal t.
func StartOfDay(t time.Time) time.Time {
	l := InAppLocation(t)
	return time.Date(l.Year(), l.Month(), l.Day(), 0, 0, 0, 0, l.Location())
}

// DaysBetween menghitung selisih hari kalender aplikasi (boleh negatif).
func DaysBetween(from, to time.Time) int {
	a, b := StartOfDay(from), StartOfDay(to)
	return int(b.Sub(a).Hours() / 24)
}

// AddDays menggeser waktu sejumlah hari kalender aplikasi.
func AddDays(t time.Time, days int) time.Time { return t.AddDate(0, 0, days) }
