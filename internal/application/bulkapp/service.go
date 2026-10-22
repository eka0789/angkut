// Package bulkapp berisi logika aplikasi pengiriman massal (bulk shipping,
// TASK-039): validasi per baris, submit job idempotent, dan prosesor yang
// mengeksekusi setiap baris SECARA INDEPENDEN — satu baris gagal tidak
// membatalkan baris lain (aturan "Multi-action bulk" di CODING_RULES).
//
// Kontrak eksekusi (CODING_RULES): submit memakai Idempotency-Key dan
// mengembalikan 202 { job_id }; progres dibaca lewat GET /shipments/bulk/{job_id}.
// Prosesor berjalan dalam goroutine proses API (bukan worker) karena broker
// RabbitMQ tidak tersedia di lingkungan pengembangan; unit kerjanya ada di
// service ini sehingga bisa dipindahkan ke worker tanpa mengubah logika.
package bulkapp

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"strings"
	"time"

	"github.com/angkut/angkut/apps/internal/application/shipmentapp"
	"github.com/angkut/angkut/apps/internal/domain/address"
	"github.com/angkut/angkut/apps/internal/domain/audit"
	"github.com/angkut/angkut/apps/internal/domain/bulk"
	"github.com/angkut/angkut/apps/internal/domain/courier"
	"github.com/angkut/angkut/apps/internal/domain/region"
	"github.com/angkut/angkut/apps/internal/domain/shared"
	"github.com/angkut/angkut/apps/internal/domain/shipment"
	"github.com/angkut/angkut/apps/internal/infrastructure/database"
	"github.com/angkut/angkut/apps/internal/platform"
)

// ShipmentCreator adalah kontrak minimal service shipment yang dipakai
// prosesor per baris (pembuatan transaksional + AWB + idempotency).
type ShipmentCreator interface {
	Create(ctx context.Context, orgID, actorUserID, requestID string, in shipmentapp.CreateInput) (*shipment.Shipment, error)
}

// Options membatasi perilaku bulk dari konfigurasi.
type Options struct {
	MaxRows     int
	MaxWeightKG float64
	ChunkSize   int
	// SyncProcess menonaktifkan goroutine latar: dipakai test yang ingin
	// eksekusi deterministik (memanggil Process langsung).
	SyncProcess bool
}

// Deps adalah dependensi service bulk.
type Deps struct {
	Tx       *database.TxManager
	Stores   *database.Stores
	Shipment ShipmentCreator
	Couriers courier.Repository
	Regions  region.Repository
	Options  Options
	Audit    audit.Recorder
	Log      *slog.Logger
}

// Service adalah service aplikasi pengiriman massal.
type Service struct {
	tx       *database.TxManager
	stores   *database.Stores
	ship     ShipmentCreator
	couriers courier.Repository
	regions  region.Repository
	opts     Options
	audit    audit.Recorder
	log      *slog.Logger
}

// New membuat service bulk.
func New(d Deps) *Service {
	log := d.Log
	if log == nil {
		log = slog.Default()
	}
	if d.Options.MaxRows <= 0 {
		d.Options.MaxRows = 2000
	}
	if d.Options.MaxWeightKG <= 0 {
		d.Options.MaxWeightKG = 100
	}
	if d.Options.ChunkSize <= 0 {
		d.Options.ChunkSize = 100
	}
	return &Service{
		tx: d.Tx, stores: d.Stores, ship: d.Shipment, couriers: d.Couriers,
		regions: d.Regions, opts: d.Options, audit: d.Audit, log: log,
	}
}

// ── Validasi ────────────────────────────────────────────────────────────

// ValidateInput adalah permintaan memvalidasi sekumpulan baris TANPA
// membuat efek samping (dipakai endpoint preview & oleh Submit).
type ValidateInput struct {
	OrganizationID string
	CourierCode    string
	ServiceCode    string
	WarehouseID    string
	Rows           []bulk.Row
}

// RowReport adalah hasil validasi satu baris untuk laporan pengirim.
type RowReport struct {
	RowNo  int          `json:"row_no"`
	Status string       `json:"status"` // valid | warning | error
	Issues []bulk.Issue `json:"issues,omitempty"`
	Row    bulk.Row     `json:"row"`
}

// Report adalah laporan validasi seluruh baris.
type Report struct {
	Total   int         `json:"total"`
	Valid   int         `json:"valid"`
	Warning int         `json:"warning"`
	Error   int         `json:"error"`
	Rows    []RowReport `json:"rows"`
}

// prepared adalah baris yang sudah dinormalisasi + wilayah ter-resolve.
type prepared struct {
	row        bulk.Row
	rowNo      int
	districtID string
	cityID     string
	provinceID string
	issues     []bulk.Issue
}

func (p prepared) hasError() bool {
	for _, is := range p.issues {
		if is.IsError() {
			return true
		}
	}
	return false
}

// ValidateReport memvalidasi baris tanpa efek samping.
func (s *Service) ValidateReport(ctx context.Context, in ValidateInput) (*Report, error) {
	prepared, _, err := s.prepare(ctx, in)
	if err != nil {
		return nil, err
	}
	rep := &Report{Total: len(prepared), Rows: make([]RowReport, 0, len(prepared))}
	for _, p := range prepared {
		rr := RowReport{RowNo: p.rowNo, Issues: p.issues, Row: p.row, Status: "valid"}
		switch {
		case p.hasError():
			rr.Status = "error"
			rep.Error++
		case len(p.issues) > 0:
			rr.Status = "warning"
			rep.Warning++
			rep.Valid++
		default:
			rep.Valid++
		}
		rep.Rows = append(rep.Rows, rr)
	}
	return rep, nil
}

// prepare menjalankan seluruh tahap validasi: normalisasi, aturan murni,
// duplikat, resolusi referensi (kurir/layanan/gudang/kode pos). Kesalahan
// batch-level (kurir tak dikenal, gudang tanpa alamat) = error request;
// kesalahan baris = issue pada baris itu saja.
func (s *Service) prepare(ctx context.Context, in ValidateInput) ([]prepared, *batchContext, error) {
	if len(in.Rows) == 0 {
		return nil, nil, shared.Validation(map[string]string{"rows": "Minimal satu baris wajib diisi."})
	}
	if len(in.Rows) > s.opts.MaxRows {
		return nil, nil, shared.Validation(map[string]string{
			"rows": fmt.Sprintf("Jumlah baris melebihi batas %d.", s.opts.MaxRows),
		})
	}

	bc, err := s.batchContext(ctx, in)
	if err != nil {
		return nil, nil, err
	}

	opts := bulk.Options{MaxWeightKG: s.opts.MaxWeightKG}
	rows := make([]bulk.Row, len(in.Rows))
	copy(rows, in.Rows)

	out := make([]prepared, 0, len(rows))
	for i := range rows {
		p := prepared{row: rows[i], rowNo: i + 1}

		// Normalisasi sebelum aturan lain: telepon & kode pos dibakukan
		// ("sumber import tidak dipercaya" — CODING_RULES).
		if phone, err := bulk.NormalizePhone(p.row.RecipientPhone); err == nil {
			p.row.RecipientPhone = phone
		}
		if postal, err := bulk.NormalizePostalCode(p.row.PostalCode); err == nil {
			p.row.PostalCode = postal
		}

		p.issues = bulk.ValidateRow(p.row, opts)

		// Referensi: kode pos harus terdaftar agar alamat valid di DB.
		if res, ok := bc.postal(p.row.PostalCode); !ok {
			p.issues = append(p.issues, bulk.Issue{
				Field: "postal_code", Message: "Kode pos tidak dikenal dalam data wilayah.",
			})
		} else {
			p.districtID, p.cityID, p.provinceID = res.DistrictID, res.CityID, res.ProvinceID
		}

		// Layanan mendukung COD?
		if p.row.CODAmount > 0 && !bc.service.SupportsCOD {
			p.issues = append(p.issues, bulk.Issue{
				Field: "cod_amount", Message: "Layanan kurir yang dipilih tidak mendukung COD.",
			})
		}
		// Asuransi tanpa dukungan layanan = peringatan, bukan penolakan:
		// paket tetap terkirim, hanya perlindungannya dilepas.
		if p.row.Insurance && !bc.service.SupportsInsurance {
			p.issues = append(p.issues, bulk.Issue{
				Field: "insurance", Message: "Layanan tidak mendukung asuransi — asuransi dilepas.",
				Severity: bulk.SeverityWarning,
			})
			p.row.Insurance = false
		}

		out = append(out, p)
	}

	// Duplikat antar-baris di file yang sama (kembar) — baris kedua+ ditolak.
	for i, issues := range bulk.FindDuplicates(rows) {
		if i < len(out) {
			out[i].issues = append(out[i].issues, issues...)
		}
	}
	return out, bc, nil
}

// batchContext menyiapkan referensi sekali per batch: kurir + layanan +
// gudang asal, plus cache resolusi kode pos (hindari N+1 query).
type batchContext struct {
	service      *courier.Service
	warehouseID  string
	originAddrID string
	postalCache  map[string]*region.PostalResolution
	regions      region.Repository
	ctx          context.Context
}

func (b *batchContext) postal(code string) (*region.PostalResolution, bool) {
	if code == "" {
		return nil, false
	}
	if res, ok := b.postalCache[code]; ok {
		return res, res != nil
	}
	res, err := b.regions.ResolvePostalCode(b.ctx, code)
	if err != nil {
		b.postalCache[code] = nil
		return nil, false
	}
	b.postalCache[code] = res
	return res, true
}

// batchContext membangun konteks batch: resolve kurir/layanan/gudang sekali.
func (s *Service) batchContext(ctx context.Context, in ValidateInput) (*batchContext, error) {
	cr, err := s.couriers.GetByCode(ctx, strings.TrimSpace(in.CourierCode))
	if err != nil || !cr.IsActive {
		return nil, shared.Validation(map[string]string{"courier_code": "Kurir tidak dikenal atau tidak aktif."})
	}
	services, err := s.couriers.ListActiveServices(ctx, cr.ID)
	if err != nil {
		return nil, err
	}
	var svc *courier.Service
	for i := range services {
		if services[i].Code == strings.TrimSpace(in.ServiceCode) {
			svc = &services[i]
			break
		}
	}
	if svc == nil {
		return nil, shared.Validation(map[string]string{"service_code": "Layanan kurir tidak dikenal atau tidak aktif."})
	}

	wh, err := s.stores.Warehouses().GetByID(ctx, in.OrganizationID, strings.TrimSpace(in.WarehouseID))
	if err != nil {
		return nil, shared.Validation(map[string]string{"warehouse_id": "Gudang asal tidak ditemukan."})
	}
	if !wh.IsActive {
		return nil, shared.Validation(map[string]string{"warehouse_id": "Gudang asal tidak aktif."})
	}
	if wh.AddressID == "" {
		return nil, shared.Validation(map[string]string{
			"warehouse_id": "Gudang asal belum memiliki alamat — lengkapi dulu sebelum pengiriman massal.",
		})
	}

	return &batchContext{
		service:      svc,
		warehouseID:  wh.ID,
		originAddrID: wh.AddressID,
		postalCache:  make(map[string]*region.PostalResolution, 64),
		regions:      s.regions,
		ctx:          ctx,
	}, nil
}

// ── Unggahan CSV ────────────────────────────────────────────────────────

// CreateUpload membaca CSV dari reader lalu menyimpan baris terparse sebagai
// unggahan. Parser membatasi jumlah baris (BULK_MAX_ROWS); file mentah tidak
// disimpan — hanya hasilnya, sehingga tidak ada berkas mentah yang beredar.
func (s *Service) CreateUpload(ctx context.Context, orgID, actorUserID, filename string, r io.Reader) (*bulk.Upload, error) {
	rows, err := bulk.ParseCSV(r, s.opts.MaxRows)
	if err != nil {
		return nil, shared.Validation(map[string]string{"file": err.Error()})
	}
	up := &bulk.Upload{
		ID:             platform.NewUUID(),
		OrganizationID: orgID,
		Filename:       strings.TrimSpace(filename),
		RowCount:       len(rows),
		Rows:           rows,
		CreatedBy:      actorUserID,
		CreatedAt:      time.Now().UTC(),
	}
	if err := s.stores.Bulk().CreateUpload(ctx, up); err != nil {
		return nil, err
	}
	s.log.Info("unggahan CSV dibuat", "upload_id", up.ID, "rows", up.RowCount, "org", orgID)
	return up, nil
}

// GetUpload mengambil unggahan milik organisasi.
func (s *Service) GetUpload(ctx context.Context, orgID, id string) (*bulk.Upload, error) {
	return s.stores.Bulk().GetUpload(ctx, orgID, id)
}

// ConsumeUpload menandai unggahan sudah dipakai submit (jejak audit; baris
// tetap bisa dibaca ulang — penandaan bukan penghapusan).
func (s *Service) ConsumeUpload(ctx context.Context, orgID, id string) error {
	return s.stores.Bulk().MarkUploadConsumed(ctx, orgID, id, time.Now().UTC())
}

// ── Submit ──────────────────────────────────────────────────────────────

// SubmitInput adalah permintaan membuat job massal.
type SubmitInput struct {
	OrganizationID string
	ActorUserID    string
	RequestID      string
	IdempotencyKey string

	CourierCode string
	ServiceCode string
	WarehouseID string
	Rows        []bulk.Row
}

// SubmitResult adalah hasil submit (202).
type SubmitResult struct {
	Job      *bulk.Job `json:"-"`
	JobID    string    `json:"job_id"`
	Accepted int       `json:"accepted"`
	Rejected int       `json:"rejected"`
	Replayed bool      `json:"replayed"`
}

// Submit memvalidasi baris lalu membuat job + barisnya dalam SATU transaksi,
// dan menindaklanjutinya lewat prosesor per baris. Idempotency-Key yang sama
// mengembalikan job pertama tanpa membuat job/baris baru.
func (s *Service) Submit(ctx context.Context, in SubmitInput) (*SubmitResult, error) {
	key := strings.TrimSpace(in.IdempotencyKey)
	if key != "" {
		if existing, err := s.stores.Bulk().GetJobByKey(ctx, in.OrganizationID, key); err == nil {
			s.log.Info("submit bulk idempotent diputar ulang", "key", key, "job_id", existing.ID)
			return &SubmitResult{
				Job: existing, JobID: existing.ID,
				Accepted: existing.AcceptedRows, Rejected: existing.RejectedRows,
				Replayed: true,
			}, nil
		} else if !shared.Is(err, shared.ErrNotFound) {
			return nil, err
		}
	}

	prepared, bc, err := s.prepare(ctx, ValidateInput{
		OrganizationID: in.OrganizationID,
		CourierCode:    in.CourierCode,
		ServiceCode:    in.ServiceCode,
		WarehouseID:    in.WarehouseID,
		Rows:           in.Rows,
	})
	if err != nil {
		return nil, err
	}

	now := time.Now().UTC()
	job := &bulk.Job{
		ID:                platform.NewUUID(),
		OrganizationID:    in.OrganizationID,
		IdempotencyKey:    key,
		Status:            bulk.JobPending,
		CourierCode:       strings.TrimSpace(in.CourierCode),
		ServiceCode:       strings.TrimSpace(in.ServiceCode),
		OriginWarehouseID: bc.warehouseID,
		TotalRows:         len(prepared),
		CreatedBy:         in.ActorUserID,
		CreatedAt:         now,
		UpdatedAt:         now,
	}

	results := make([]bulk.RowResult, 0, len(prepared))
	for _, p := range prepared {
		status := bulk.StatusValid
		if p.hasError() {
			status = bulk.StatusInvalid
			job.RejectedRows++
		} else {
			job.AcceptedRows++
		}
		results = append(results, bulk.RowResult{
			JobID: job.ID, RowNo: p.rowNo, Row: p.row,
			Status: status, Issues: p.issues, CreatedAt: now,
		})
	}

	err = s.tx.InTx(ctx, func(q database.Querier) error {
		return s.stores.WithTx(q).Bulk().CreateJob(ctx, job, results)
	})
	if err != nil {
		// Balapan dua submit dengan key sama: pemanggil kedua menemukan
		// index unik lalu memutar hasil pemenang (bukan error 500).
		if key != "" {
			if existing, gerr := s.stores.Bulk().GetJobByKey(ctx, in.OrganizationID, key); gerr == nil {
				s.log.Info("submit bulk balapan — memakai job pemenang", "key", key, "job_id", existing.ID)
				return &SubmitResult{
					Job: existing, JobID: existing.ID,
					Accepted: existing.AcceptedRows, Rejected: existing.RejectedRows,
					Replayed: true,
				}, nil
			}
		}
		return nil, err
	}

	if s.audit != nil {
		e := audit.New(audit.ActionBulkJobSubmitted, in.ActorUserID, in.OrganizationID).
			WithEntity("bulk_job", job.ID).
			WithChanges(nil, map[string]any{
				"job_id": job.ID, "total": job.TotalRows,
				"accepted": job.AcceptedRows, "rejected": job.RejectedRows,
				"courier": job.CourierCode, "service": job.ServiceCode,
			}).
			WithRequest("", "", in.RequestID)
		if aerr := s.audit.Record(ctx, e); aerr != nil {
			s.log.Warn("audit bulk submit gagal", "error", aerr)
		}
	}
	s.log.Info("job bulk dibuat",
		"job_id", job.ID, "org", job.OrganizationID,
		"total", job.TotalRows, "accepted", job.AcceptedRows, "rejected", job.RejectedRows)

	// Eksekusi asinkron: endpoint sudah mengirim 202; progres dibaca via GET.
	if !s.opts.SyncProcess {
		go s.processDetached(job.ID)
	}
	return &SubmitResult{
		Job: job, JobID: job.ID,
		Accepted: job.AcceptedRows, Rejected: job.RejectedRows,
	}, nil
}

// processDetached menjalankan prosesor dengan konteks sendiri; panic tidak
// boleh membunuh proses API dan harus tercatat sebagai kegagalan job.
func (s *Service) processDetached(jobID string) {
	defer func() {
		if rec := recover(); rec != nil {
			s.log.Error("prosesor bulk panic", "job_id", jobID, "panic", rec)
			ctx := context.Background()
			_ = s.stores.Bulk().FinishJob(ctx, jobID, bulk.JobFailed,
				"Prosesor berhenti tak terduga.", time.Now().UTC())
		}
	}()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
	defer cancel()
	if err := s.Process(ctx, jobID); err != nil {
		s.log.Error("proses bulk gagal", "job_id", jobID, "error", err)
	}
}

// JobStatus adalah tampilan progres job untuk klien.
type JobStatus struct {
	bulk.Job
	Rows []bulk.RowResult `json:"rows,omitempty"`
}

// Job mengambil status job beserta hasil per baris (tenant-scoped).
func (s *Service) Job(ctx context.Context, orgID, jobID string) (*JobStatus, error) {
	job, err := s.stores.Bulk().GetJob(ctx, orgID, jobID)
	if err != nil {
		return nil, err
	}
	rows, err := s.stores.Bulk().ListJobRows(ctx, orgID, jobID)
	if err != nil {
		return nil, err
	}
	return &JobStatus{Job: *job, Rows: rows}, nil
}

// ── Prosesor per baris ─────────────────────────────────────────────────

// Process mengeksekisi seluruh baris valid sebuah job. Setiap baris
// berdiri sendiri: kegagalannya dicatat pada baris itu lalu proses lanjut.
// Dipanggil goroutine latar dari Submit (atau langsung dari test).
func (s *Service) Process(ctx context.Context, jobID string) error {
	job, err := s.stores.Bulk().GetJobByID(ctx, jobID)
	if err != nil {
		return err
	}
	if job.Status == bulk.JobCompleted || job.Status == bulk.JobFailed {
		return nil // sudah pernah selesai — aman dipanggil ulang
	}
	if err := s.stores.Bulk().MarkProcessing(ctx, jobID, time.Now().UTC()); err != nil {
		return err
	}

	wh, err := s.stores.Warehouses().GetByID(ctx, job.OrganizationID, job.OriginWarehouseID)
	if err != nil {
		return s.finish(ctx, jobID, bulk.JobFailed, "Gudang asal job tidak ditemukan lagi.")
	}
	if wh.AddressID == "" {
		return s.finish(ctx, jobID, bulk.JobFailed, "Gudang asal kehilangan alamat.")
	}

	rows, err := s.stores.Bulk().ListValidRows(ctx, jobID)
	if err != nil {
		return err
	}

	bc := &batchContext{
		postalCache: make(map[string]*region.PostalResolution, 64),
		regions:     s.regions,
		ctx:         ctx,
	}
	addrCache := make(map[string]string, len(rows)) // kunci duplikat → alamat tercipta

	processed, created, failed := 0, 0, 0
	chunk := s.opts.ChunkSize
	for i, rr := range rows {
		if err := s.stores.Bulk().MarkRowCreating(ctx, jobID, rr.RowNo); err != nil {
			return err
		}
		sh, err := s.executeRow(ctx, job, wh.AddressID, rr, bc, addrCache)
		if err != nil {
			failed++
			s.log.Warn("baris bulk gagal",
				"job_id", jobID, "row_no", rr.RowNo, "error", err)
			if merr := s.stores.Bulk().MarkRowFailed(ctx, jobID, rr.RowNo, userMessage(err)); merr != nil {
				return merr
			}
		} else {
			created++
			if merr := s.stores.Bulk().MarkRowCreated(ctx, jobID, rr.RowNo, sh.ID, sh.AWB); merr != nil {
				return merr
			}
		}
		processed++

		// Publikasikan progres per chunk (aturan Chunking CODING_RULES).
		if (i+1)%chunk == 0 || i == len(rows)-1 {
			if perr := s.stores.Bulk().UpdateProgress(ctx, jobID, processed, created, failed); perr != nil {
				return perr
			}
		}
	}
	if err := s.stores.Bulk().UpdateProgress(ctx, jobID, processed, created, failed); err != nil {
		return err
	}

	summary := ""
	if failed > 0 {
		summary = fmt.Sprintf("%d dari %d baris gagal diproses.", failed, len(rows))
	}
	return s.finish(ctx, jobID, bulk.JobCompleted, summary)
}

func (s *Service) finish(ctx context.Context, jobID string, status bulk.JobStatus, summary string) error {
	return s.stores.Bulk().FinishJob(ctx, jobID, status, summary, time.Now().UTC())
}

// executeRow mengeksekusi SATU baris: alamat tujuan (dipakai-ulang bila
// kembar) lalu membuat shipment lewat service shipment (pricing, AWB,
// idempotency per baris tetap berlaku).
func (s *Service) executeRow(
	ctx context.Context,
	job *bulk.Job,
	originAddressID string,
	rr bulk.RowResult,
	bc *batchContext,
	addrCache map[string]string,
) (*shipment.Shipment, error) {
	row := rr.Row
	phone, err := bulk.NormalizePhone(row.RecipientPhone)
	if err != nil {
		return nil, shared.Validation(map[string]string{"recipient_phone": err.Error()})
	}
	row.RecipientPhone = phone
	postal, err := bulk.NormalizePostalCode(row.PostalCode)
	if err != nil {
		return nil, shared.Validation(map[string]string{"postal_code": err.Error()})
	}
	row.PostalCode = postal

	res, ok := bc.postal(row.PostalCode)
	if !ok {
		return nil, shared.Validation(map[string]string{"postal_code": "Kode pos tidak dikenal."})
	}

	// Alamat tujuan: per penerima. Kiriman kembar dalam satu job memakai
	// alamat yang sama — tidak membuat baris addresses ganda.
	key := bulk.DuplicateKey(row)
	addrID, exists := addrCache[key]
	if !exists {
		addr := &address.Address{
			ID:             platform.NewUUID(),
			OrganizationID: job.OrganizationID,
			Recipient:      row.RecipientName,
			Phone:          row.RecipientPhone,
			ProvinceID:     res.ProvinceID,
			CityID:         res.CityID,
			DistrictID:     res.DistrictID,
			PostalCode:     row.PostalCode,
			Street:         row.Street,
			Label:          "Bulk " + job.CreatedAt.Format("060102"),
			Status:         address.StatusActive,
			CreatedBy:      job.CreatedBy,
			CreatedAt:      time.Now().UTC(),
			UpdatedAt:      time.Now().UTC(),
		}
		if err := addr.Validate(); err != nil {
			return nil, err
		}
		if err := s.tx.InTx(ctx, func(q database.Querier) error {
			return s.stores.WithTx(q).Addresses().Create(ctx, addr)
		}); err != nil {
			return nil, fmt.Errorf("membuat alamat tujuan: %w", err)
		}
		addrID = addr.ID
		addrCache[key] = addrID
	}

	// Idempotency per baris: retry setelah crash di tengah job tidak membuat
	// pengiriman ganda (kunci unik org+key di tabel shipments).
	in := shipmentapp.CreateInput{
		WarehouseID:          job.OriginWarehouseID,
		CourierCode:          job.CourierCode,
		ServiceCode:          job.ServiceCode,
		OriginAddressID:      originAddressID,
		DestinationAddressID: addrID,
		Weight:               row.WeightKG,
		Length:               row.LengthCM,
		Width:                row.WidthCM,
		Height:               row.HeightCM,
		PackageType:          "paket",
		IsCOD:                row.CODAmount > 0,
		CODAmount:            row.CODAmount,
		DeclaredValue:        row.DeclaredValue,
		Insurance:            row.Insurance,
		Notes:                strings.TrimSpace(row.Notes),
		Items: []shipmentapp.CreateItemInput{{
			Name: "Kiriman " + row.RecipientName,
			Qty:  1,
		}},
		IdempotencyKey: fmt.Sprintf("bulk:%s:%d", job.ID, rr.RowNo),
	}
	return s.ship.Create(ctx, job.OrganizationID, job.CreatedBy, job.ID, in)
}

// userMessage mengekstrak pesan aman untuk ditampilkan per baris. Pesan
// internal (wrap) tidak pernah ikut — hanya pesan typed error.
func userMessage(err error) string {
	var se *shared.Error
	if errors.As(err, &se) {
		if len(se.Fields) > 0 {
			parts := make([]string, 0, len(se.Fields))
			for _, v := range se.Fields {
				parts = append(parts, v)
			}
			return strings.Join(parts, " ")
		}
		return se.Message
	}
	return "Gagal memproses baris (rincian ada di log server)."
}
