// Package shipmentapp berisi logika aplikasi pengiriman: pembuatan
// transaksional (shipment + item + paket), pembentukan AWB, transisi status,
// dan pembuatan label. Boundary transaksi ada di sini, bukan di handler.
package shipmentapp

import (
	"context"
	"log/slog"
	"strings"
	"time"

	"github.com/angkut/angkut/apps/api/internal/domain/audit"
	"github.com/angkut/angkut/apps/api/internal/domain/courier"
	"github.com/angkut/angkut/apps/api/internal/domain/order"
	"github.com/angkut/angkut/apps/api/internal/domain/pricing"
	"github.com/angkut/angkut/apps/api/internal/domain/shared"
	"github.com/angkut/angkut/apps/api/internal/domain/shipment"
	"github.com/angkut/angkut/apps/api/internal/infrastructure/database"
	"github.com/angkut/angkut/apps/internal/platform"
)

// Deps adalah dependensi service shipment.
type Deps struct {
	Tx        *database.TxManager
	Stores    *database.Stores
	Shipments shipment.Repository
	Couriers  courier.Repository
	Registry  *courier.Registry
	Orders    order.Repository
	Engine    *pricing.Engine
	Audit     audit.Recorder
	Log       *slog.Logger
}

// Service adalah service aplikasi shipment.
type Service struct {
	tx        *database.TxManager
	stores    *database.Stores
	shipments shipment.Repository
	couriers  courier.Repository
	registry  *courier.Registry
	orders    order.Repository
	engine    *pricing.Engine
	audit     audit.Recorder
	log       *slog.Logger
}

// New membuat service shipment.
func New(d Deps) *Service {
	log := d.Log
	if log == nil {
		log = slog.Default()
	}
	return &Service{
		tx: d.Tx, stores: d.Stores, shipments: d.Shipments,
		couriers: d.Couriers, registry: d.Registry, orders: d.Orders, engine: d.Engine,
		audit: d.Audit, log: log,
	}
}

// CreateItemInput adalah satu item isi paket.
type CreateItemInput struct {
	OrderItemID string
	VariantID   string
	Name        string
	Qty         int
}

// CreateInput adalah permintaan membuat pengiriman.
type CreateInput struct {
	OrderID              string
	StoreID              string
	WarehouseID          string
	CourierCode          string
	ServiceCode          string
	OriginAddressID      string
	DestinationAddressID string

	Weight      float64
	Length      *float64
	Width       *float64
	Height      *float64
	PackageType string

	IsCOD         bool
	CODAmount     int64
	DeclaredValue int64
	Insurance     bool
	Notes         string

	Items []CreateItemInput

	// IdempotencyKey berasal dari header Idempotency-Key; bila diisi, retry
	// dengan key sama mengembalikan shipment yang sudah ada.
	IdempotencyKey string
}

// Create membuat pengiriman secara transaksional. Bila IdempotencyKey diisi
// dan sudah pernah dipakai, shipment lama dikembalikan tanpa membuat baris
// baru — retry aman.
func (s *Service) Create(ctx context.Context, orgID, actorUserID, requestID string, in CreateInput) (*shipment.Shipment, error) {
	if s.engine == nil {
		return nil, shared.New("SHIPMENT_NOT_CONFIGURED", "Mesin harga belum dikonfigurasi.", shared.ErrConflict)
	}
	key := strings.TrimSpace(in.IdempotencyKey)
	if key != "" {
		if existing, ok, err := s.shipments.GetByIdempotencyKey(ctx, orgID, key); err != nil {
			return nil, err
		} else if ok {
			// Retry: kembalikan hasil yang sama tanpa efek samping baru.
			s.log.Info("shipment idempotent diputar ulang", "key", key, "shipment_id", existing.ID)
			return existing, nil
		}
	}

	// Resolusi kurir & layanan: kode bukan sekadar teks, harus ada di registry.
	cr, svc, err := s.resolveService(ctx, in.CourierCode, in.ServiceCode)
	if err != nil {
		return nil, err
	}

	// Harga dihitung server dari engine — bukan dikirim klien.
	wr := s.engine.Weights(in.Weight, dims(in), svc.VolumetricDivisor)
	if wr.ExceedsMax {
		return nil, shared.Validation(map[string]string{
			"weight": "Berat chargeable melebihi batas yang diizinkan.",
		})
	}

	now := time.Now().UTC()
	sh := &shipment.Shipment{
		ID:                   platform.NewUUID(),
		OrganizationID:       orgID,
		OrderID:              strings.TrimSpace(in.OrderID),
		StoreID:              strings.TrimSpace(in.StoreID),
		WarehouseID:          strings.TrimSpace(in.WarehouseID),
		CourierID:            cr.ID,
		CourierServiceID:     svc.ID,
		CourierCode:          cr.Code,
		ServiceCode:          svc.Code,
		Status:               shipment.StatusPendingPickup,
		OriginAddressID:      strings.TrimSpace(in.OriginAddressID),
		DestinationAddressID: strings.TrimSpace(in.DestinationAddressID),
		ActualWeight:         wr.Actual,
		VolumetricWeight:     wr.Volumetric,
		ChargeableWeight:     wr.Chargeable,
		Length:               in.Length,
		Width:                in.Width,
		Height:               in.Height,
		PackageType:          strings.TrimSpace(in.PackageType),
		DeclaredValue:        in.DeclaredValue,
		IsCOD:                svc.SupportsCOD && in.IsCOD,
		CODAmount:            in.CODAmount,
		EtaMinDays:           svc.EtaMinDays,
		EtaMaxDays:           svc.EtaMaxDays,
		Notes:                strings.TrimSpace(in.Notes),
		Version:              1,
		IdempotencyKey:       key,
		BookedAt:             &now,
		CreatedAt:            now,
		UpdatedAt:            now,
		Items:                make([]shipment.Item, 0, len(in.Items)),
	}
	for _, it := range in.Items {
		sh.Items = append(sh.Items, shipment.Item{
			OrderItemID:  strings.TrimSpace(it.OrderItemID),
			VariantID:    strings.TrimSpace(it.VariantID),
			NameSnapshot: strings.TrimSpace(it.Name),
			Qty:          it.Qty,
		})
	}

	// Harga dasar dari tarif kurir rute ini; markup & fee dari engine.
	base, err := s.basePrice(ctx, sh, in)
	if err != nil {
		return nil, err
	}
	quote := s.engine.Price(base, pricing.QuoteOptions{
		IsCOD:                    in.IsCOD,
		CODAmount:                in.CODAmount,
		Insured:                  in.Insurance,
		DeclaredValue:            in.DeclaredValue,
		ServiceSupportsCOD:       svc.SupportsCOD,
		ServiceSupportsInsurance: svc.SupportsInsurance,
	})
	sh.CostPrice = quote.BasePrice
	sh.ShippingPrice = quote.Total
	sh.CODFee = quote.CODFee
	sh.InsuranceAmount = quote.InsuranceFee

	// Validasi input lebih dulu: resi diterbitkan di dalam transaksi berikutnya,
	// jadi memeriksa awb di sini hanya akan menolak request yang sah.
	if err := sh.ValidateInput(); err != nil {
		return nil, err
	}
	if err := s.createWithAWB(ctx, sh, cr); err != nil {
		return nil, err
	}
	s.recordAudit(ctx, orgID, actorUserID, requestID, audit.ActionShipmentCreated, sh)
	s.log.Info("shipment dibuat", "shipment_id", sh.ID, "awb", sh.AWB, "org", orgID)
	return sh, nil
}

// createWithAWB menerbitkan resi lalu menyimpan dalam satu transaksi. Bentrok
// resi (request paralel) diulang beberapa kali.
func (s *Service) createWithAWB(ctx context.Context, sh *shipment.Shipment, cr *courier.Courier) error {
	const maxAttempts = 3
	day := sh.CreatedAt.Format("060102")

	var lastErr error
	for attempt := 0; attempt < maxAttempts; attempt++ {
		lastErr = s.tx.InTx(ctx, func(q database.Querier) error {
			tx := s.stores.WithTx(q)
			seq, err := tx.Shipments().NextAWBSequence(ctx, sh.OrganizationID, cr.Code, day)
			if err != nil {
				return err
			}
			sh.AWB = shipment.AWB{
				CourierCode: cr.Code,
				Year:        sh.CreatedAt.Year(),
				Month:       sh.CreatedAt.Month(),
				Day:         sh.CreatedAt.Day(),
				Seq:         seq,
			}.String()
			s.attachPackage(sh)
			return tx.Shipments().Create(ctx, sh)
		})
		if lastErr == nil {
			return nil
		}
		if !shared.Is(lastErr, shared.ErrConflict) {
			return lastErr
		}
		s.log.Warn("resi bentrok — mencoba lagi", "attempt", attempt+1)
	}
	return lastErr
}

// attachPackage menyusun satu paket label untuk shipment. Payload QR/barcode
// dibentuk di sini agar nilainya konsisten antara API dan dokumen cetak.
func (s *Service) attachPackage(sh *shipment.Shipment) {
	sh.Packages = []shipment.Package{{
		Sequence:     1,
		Weight:       sh.ActualWeight,
		Length:       sh.Length,
		Width:        sh.Width,
		Height:       sh.Height,
		QRPayload:    shipment.QRPayload(sh.ID, sh.AWB),
		BarcodeValue: shipment.BarcodeValue(sh.AWB),
	}}
}

// resolveService memastikan kode kurir & layanan benar-benar ada dan aktif.
func (s *Service) resolveService(ctx context.Context, courierCode, serviceCode string) (*courier.Courier, *courier.Service, error) {
	c, err := s.couriers.GetByCode(ctx, strings.TrimSpace(courierCode))
	if err != nil {
		return nil, nil, shipment.ErrCourierNotFound
	}
	if !c.IsActive {
		return nil, nil, shipment.ErrCourierNotFound
	}
	services, err := s.couriers.ListActiveServices(ctx, c.ID)
	if err != nil {
		return nil, nil, err
	}
	want := strings.TrimSpace(serviceCode)
	for i := range services {
		if strings.EqualFold(services[i].Code, want) {
			return c, &services[i], nil
		}
	}
	return nil, nil, shipment.ErrCourierNotFound
}

// basePrice mengembalikan tarif kurir sesungguhnya untuk rute, diambil dari
// provider yang sama dengan rate comparison sehingga harga shipment konsisten
// dengan yang dilihat pengguna di halaman banding tarif.
//
// Bila tarif tidak ditemukan (mis. rute tak dilayani), error dilaporkan —
// lebih baik gagal jelas daripada menagih 0.
func (s *Service) basePrice(ctx context.Context, sh *shipment.Shipment, in CreateInput) (int64, error) {
	if s.registry == nil {
		return 0, shared.New("SHIPMENT_NOT_CONFIGURED",
			"Registry kurir belum dikonfigurasi.", shared.ErrConflict)
	}
	provider := s.registry.ProviderFor(sh.CourierCode)
	if provider == nil {
		return 0, shipment.ErrCourierNotFound
	}

	// Origin & destination kota dibutuhkan provider untuk memetakan zona.
	origin, err := s.addressCity(ctx, sh.OrganizationID, in.OriginAddressID)
	if err != nil {
		return 0, err
	}
	dest, err := s.addressCity(ctx, sh.OrganizationID, in.DestinationAddressID)
	if err != nil {
		return 0, err
	}
	if origin == "" || dest == "" {
		return 0, shared.Validation(map[string]string{
			"destination_address_id": "Alamat asal & tujuan dengan kota yang valid wajib diisi untuk menghitung ongkir.",
		})
	}

	rates, err := provider.FetchRates(ctx, courier.RateQuery{
		OriginCityID:        origin,
		DestinationCityID:   dest,
		ActualWeightKG:      in.Weight,
		Dimensions:          dims(in),
		WeightRoundUpStepKG: s.engine.Config().WeightRoundUpStepKG,
	})
	if err != nil {
		return 0, err
	}
	for _, r := range rates {
		if strings.EqualFold(r.Service.Code, sh.ServiceCode) {
			return r.Price, nil
		}
	}
	return 0, shared.New("SHIPMENT_RATE_NOT_FOUND",
		"Kurir tidak melayani rute ini untuk layanan yang dipilih.", shared.ErrNotFound)
}

// addressCity mengambil kota sebuah alamat; organisasi lain tidak terlihat.
func (s *Service) addressCity(ctx context.Context, orgID, addressID string) (string, error) {
	if strings.TrimSpace(addressID) == "" {
		return "", nil
	}
	addr, err := s.stores.Addresses().GetByID(ctx, orgID, addressID)
	if err != nil {
		return "", shipment.ErrAddressNotFound
	}
	return addr.CityID, nil
}

// dims menyusun dimensi opsional (harus lengkap).
func dims(in CreateInput) *shared.Dimensions {
	if in.Length == nil || in.Width == nil || in.Height == nil {
		return nil
	}
	return &shared.Dimensions{Length: *in.Length, Width: *in.Width, Height: *in.Height}
}

// Transition memindahkan status pengiriman dengan menegakkan state machine.
func (s *Service) Transition(ctx context.Context, orgID, actorUserID, requestID, id string, target shipment.Status, reason string) (*shipment.Shipment, error) {
	sh, err := s.shipments.GetByID(ctx, orgID, id)
	if err != nil {
		return nil, err
	}
	from := sh.Status
	if err := sh.Transition(target); err != nil {
		return nil, err
	}
	if from == target {
		return sh, nil
	}
	if err := s.shipments.UpdateStatus(ctx, orgID, id, from, target, reason); err != nil {
		return nil, err
	}
	s.recordAudit(ctx, orgID, actorUserID, requestID, audit.ActionShipmentStatusChanged, sh)
	return sh, nil
}

// Cancel membatalkan pengiriman (status → cancelled).
func (s *Service) Cancel(ctx context.Context, orgID, actorUserID, requestID, id, reason string) (*shipment.Shipment, error) {
	sh, err := s.Transition(ctx, orgID, actorUserID, requestID, id, shipment.StatusCancelled, reason)
	if err != nil {
		return nil, err
	}
	s.recordAudit(ctx, orgID, actorUserID, requestID, audit.ActionShipmentCanceled, sh)
	return sh, nil
}

// Get mengambil satu pengiriman.
func (s *Service) Get(ctx context.Context, orgID, id string) (*shipment.Shipment, error) {
	return s.shipments.GetByID(ctx, orgID, id)
}

// List mengembalikan halaman pengiriman.
func (s *Service) List(ctx context.Context, orgID string, f shipment.ListFilter) ([]shipment.Shipment, int, error) {
	return s.shipments.List(ctx, orgID, f)
}

// BasePriceFor mengekspos harga dasar untuk pemanggil yang butuh (mis. label).
func (s *Service) Engine() *pricing.Engine { return s.engine }

// recordAudit mencatat peristiwa shipment; kegagalan audit tidak menggagalkan
// operasi bisnis.
func (s *Service) recordAudit(ctx context.Context, orgID, actorUserID, requestID, action string, sh *shipment.Shipment) {
	if s.audit == nil {
		return
	}
	err := s.audit.Record(ctx, audit.Entry{
		OrganizationID: orgID,
		ActorUserID:    actorUserID,
		Action:         action,
		EntityType:     "shipment",
		EntityID:       sh.ID,
		Changes: map[string]any{
			"awb":            sh.AWB,
			"status":         string(sh.Status),
			"courier":        sh.CourierCode,
			"shipping_price": sh.ShippingPrice,
		},
		RequestID: requestID,
	})
	if err != nil {
		s.log.Warn("audit shipment gagal", "shipment_id", sh.ID, "error", err)
	}
}
