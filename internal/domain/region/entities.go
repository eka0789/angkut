// Package region berisi entitas wilayah Indonesia (provinsi, kota, kecamatan, desa, kode pos).
// Data ini bersifat reference/public — tidak memiliki tenant scoping.
package region

// Province merepresentasikan provinsi Indonesia (level 1).
type Province struct {
	ID        string `json:"id"`         // 2 digit: 31, 32, 33, ...
	Name      string `json:"name"`       // Nama provinsi
	CreatedAt string `json:"created_at"` // RFC3339
}

// City merepresentasikan kota/kabupaten (level 2).
type City struct {
	ID         string `json:"id"`          // 4 digit: 3171, 3201, 3371, ...
	ProvinceID string `json:"province_id"` // FK ke Province
	Name       string `json:"name"`        // Nama kota/kabupaten
	Type       string `json:"type"`        // "kota" atau "kabupaten"
	CreatedAt  string `json:"created_at"`  // RFC3339
}

// District merepresentasikan kecamatan (level 3).
type District struct {
	ID        string `json:"id"`         // 6 digit: 317101, 327301, ...
	CityID    string `json:"city_id"`    // FK ke City
	Name      string `json:"name"`       // Nama kecamatan
	CreatedAt string `json:"created_at"` // RFC3339
}

// Village merepresentasikan desa/kelurahan (level 4).
type Village struct {
	ID         string `json:"id"`          // 10 digit: 3171011001, ...
	DistrictID string `json:"district_id"` // FK ke District
	Name       string `json:"name"`        // Nama desa/kelurahan
	Type       string `json:"type"`        // "desa" atau "kelurahan"
	CreatedAt  string `json:"created_at"`  // RFC3339
}

// PostalCode merepresentasikan kode pos.
type PostalCode struct {
	Code       string `json:"code"`        // 5 digit: 50241, 40111, ...
	DistrictID string `json:"district_id"` // FK ke District
	CreatedAt  string `json:"created_at"`  // RFC3339
}

// PostalResolution adalah hasil resolve satu kode pos ke rantai wilayah
// penuh (kecamatan → kota → provinsi). Dipakai impor massal (bulk) yang hanya
// menerima kolom kode pos, tanpa id wilayah.
type PostalResolution struct {
	PostalCode string `json:"postal_code"`
	DistrictID string `json:"district_id"`
	CityID     string `json:"city_id"`
	ProvinceID string `json:"province_id"`
}

// ValidType melaporkan apakah tipe kota valid.
func ValidCityType(t string) bool {
	return t == "kota" || t == "kabupaten"
}

// ValidType melaporkan apakah tipe desa valid.
func ValidVillageType(t string) bool {
	return t == "desa" || t == "kelurahan"
}
