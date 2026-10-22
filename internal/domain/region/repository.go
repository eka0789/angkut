package region

import "context"

// Repository mendefinisikan operasi baca referensi wilayah.
type Repository interface {
	ListProvinces(ctx context.Context) ([]Province, error)
	ListCities(ctx context.Context, provinceID string) ([]City, error)
	ListDistricts(ctx context.Context, cityID string) ([]District, error)
	ListVillages(ctx context.Context, districtID string) ([]Village, error)
	ListPostalCodes(ctx context.Context, districtID string) ([]PostalCode, error)

	GetProvinceByID(ctx context.Context, id string) (*Province, error)
	GetCityByID(ctx context.Context, id string) (*City, error)
	GetDistrictByID(ctx context.Context, id string) (*District, error)
	GetVillageByID(ctx context.Context, id string) (*Village, error)

	// ResolvePostalCode mengubah kode pos menjadi rantai wilayah penuh
	// (kecamatan → kota → provinsi). Kode tak dikenal → NotFound.
	ResolvePostalCode(ctx context.Context, code string) (*PostalResolution, error)
}
