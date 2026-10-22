package handler

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/angkut/angkut/apps/api/internal/interfaces/http/httpx"
	"github.com/angkut/angkut/apps/api/internal/interfaces/http/middleware"
	"github.com/angkut/angkut/apps/api/internal/interfaces/http/response"
	"github.com/angkut/angkut/apps/internal/application/bulkapp"
	"github.com/angkut/angkut/apps/internal/domain/bulk"
	"github.com/angkut/angkut/apps/internal/domain/shared"
)

// BulkService adalah antarmuka minimal service bulk untuk handler.
type BulkService interface {
	ValidateReport(ctx context.Context, in bulkapp.ValidateInput) (*bulkapp.Report, error)
	Submit(ctx context.Context, in bulkapp.SubmitInput) (*bulkapp.SubmitResult, error)
	Job(ctx context.Context, orgID, jobID string) (*bulkapp.JobStatus, error)
	CreateUpload(ctx context.Context, orgID, actorUserID, filename string, r io.Reader) (*bulk.Upload, error)
	GetUpload(ctx context.Context, orgID, id string) (*bulk.Upload, error)
	ConsumeUpload(ctx context.Context, orgID, id string) error
}

// BulkHandler menangani endpoint pengiriman massal (TASK-039).
type BulkHandler struct {
	svc BulkService
	// maxUploadBytes membatasi ukuran file unggahan (BULK_MAX_UPLOAD_MIB).
	maxUploadBytes int64
}

// NewBulkHandler membuat handler bulk.
func NewBulkHandler(svc BulkService, maxUploadMiB int) *BulkHandler {
	if maxUploadMiB <= 0 {
		maxUploadMiB = 4
	}
	return &BulkHandler{svc: svc, maxUploadBytes: int64(maxUploadMiB) << 20}
}

// bulkRequest adalah body validasi/submit: baris datang langsung ATAU
// dirujuk lewat upload_id (hasil POST /bulk/upload).
type bulkRequest struct {
	Rows        []bulk.Row `json:"rows"`
	UploadID    string     `json:"upload_id"`
	CourierCode string     `json:"default_courier_code"`
	ServiceCode string     `json:"default_service_code"`
	WarehouseID string     `json:"warehouse_id"`
}

// resolveRows memilih baris dari body: upload_id (bila ada) menang atas
// rows eksplisit; unggahan yang dipakai ditandai terkonsumsi.
func (h *BulkHandler) resolveRows(w http.ResponseWriter, r *http.Request, orgID string, req *bulkRequest) ([]bulk.Row, bool) {
	if strings.TrimSpace(req.UploadID) == "" {
		if len(req.Rows) == 0 {
			response.Fail(w, shared.Validation(map[string]string{
				"rows": "Isi baris (rows) atau upload_id wajib satu diisi.",
			}))
			return nil, false
		}
		return req.Rows, true
	}
	up, err := h.svc.GetUpload(r.Context(), orgID, req.UploadID)
	if err != nil {
		response.Fail(w, err)
		return nil, false
	}
	return up.Rows, true
}

// validateBulkRequest memeriksa field batch wajib sebelum operasi lanjut.
func validateBulkRequest(w http.ResponseWriter, req *bulkRequest) bool {
	fields := map[string]string{}
	if strings.TrimSpace(req.CourierCode) == "" {
		fields["default_courier_code"] = "Kurir default wajib diisi."
	}
	if strings.TrimSpace(req.ServiceCode) == "" {
		fields["default_service_code"] = "Layanan default wajib diisi."
	}
	if strings.TrimSpace(req.WarehouseID) == "" {
		fields["warehouse_id"] = "Gudang asal wajib dipilih."
	}
	if len(fields) > 0 {
		response.Fail(w, shared.Validation(fields))
		return false
	}
	return true
}

// Upload menangani POST /api/v1/shipments/bulk/upload (multipart "file", CSV).
//
// XLSX sengaja BELUM didukung parser (kebutuhan dependensi berat; CSV adalah
// format interchange universal dari Excel — "Save as CSV"). Bila file XLSX
// dikirim, pesannya eksplisit, bukan kegagalan misterius.
func (h *BulkHandler) Upload(w http.ResponseWriter, r *http.Request) {
	identity := middleware.IdentityFrom(r.Context())
	if identity == nil {
		response.Fail(w, unauthorized())
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, h.maxUploadBytes)
	if err := r.ParseMultipartForm(h.maxUploadBytes); err != nil {
		response.Fail(w, shared.Validation(map[string]string{
			"file": fmt.Sprintf("File terlalu besar atau rusak (maksimum %d MiB).", h.maxUploadBytes>>20),
		}))
		return
	}
	file, header, err := r.FormFile("file")
	if err != nil {
		response.Fail(w, shared.Validation(map[string]string{
			"file": "Berkas wajib dikirim dengan nama field \"file\".",
		}))
		return
	}
	defer file.Close()

	name := strings.ToLower(header.Filename)
	switch {
	case strings.HasSuffix(name, ".xlsx") || strings.HasSuffix(name, ".xls"):
		response.Fail(w, shared.Validation(map[string]string{
			"file": "Format Excel belum didukung — simpan file sebagai CSV lalu unggah ulang.",
		}))
		return
	case !strings.HasSuffix(name, ".csv"):
		response.Fail(w, shared.Validation(map[string]string{
			"file": "Format file harus .csv.",
		}))
		return
	}

	up, err := h.svc.CreateUpload(r.Context(), identity.OrganizationID, identity.UserID, header.Filename, file)
	if err != nil {
		response.Fail(w, err)
		return
	}
	response.OK(w, map[string]any{
		"upload_id": up.ID,
		"row_count": up.RowCount,
		"rows":      up.Rows,
	})
}

// Validate menangani POST /api/v1/shipments/bulk/validate — laporan per baris
// tanpa efek samping (dipakai FE sebelum submit).
func (h *BulkHandler) Validate(w http.ResponseWriter, r *http.Request) {
	identity := middleware.IdentityFrom(r.Context())
	if identity == nil {
		response.Fail(w, unauthorized())
		return
	}
	req, ok := httpx.Decode[bulkRequest](w, r)
	if !ok {
		return
	}
	rows, ok := h.resolveRows(w, r, identity.OrganizationID, &req)
	if !ok {
		return
	}
	if !validateBulkRequest(w, &req) {
		return
	}

	report, err := h.svc.ValidateReport(r.Context(), bulkapp.ValidateInput{
		OrganizationID: identity.OrganizationID,
		CourierCode:    req.CourierCode,
		ServiceCode:    req.ServiceCode,
		WarehouseID:    req.WarehouseID,
		Rows:           rows,
	})
	if err != nil {
		response.Fail(w, err)
		return
	}
	response.OK(w, report)
}

// Submit menangani POST /api/v1/shipments/bulk (Idempotency-Key wajib).
// Balas 202 { job_id, accepted, rejected } — eksekusi berjalan asinkron,
// progres dibaca lewat Status.
func (h *BulkHandler) Submit(w http.ResponseWriter, r *http.Request) {
	identity := middleware.IdentityFrom(r.Context())
	if identity == nil {
		response.Fail(w, unauthorized())
		return
	}
	req, ok := httpx.Decode[bulkRequest](w, r)
	if !ok {
		return
	}
	rows, ok := h.resolveRows(w, r, identity.OrganizationID, &req)
	if !ok {
		return
	}
	if !validateBulkRequest(w, &req) {
		return
	}

	res, err := h.svc.Submit(r.Context(), bulkapp.SubmitInput{
		OrganizationID: identity.OrganizationID,
		ActorUserID:    identity.UserID,
		RequestID:      requestID(r),
		IdempotencyKey: r.Header.Get("Idempotency-Key"),
		CourierCode:    req.CourierCode,
		ServiceCode:    req.ServiceCode,
		WarehouseID:    req.WarehouseID,
		Rows:           rows,
	})
	if err != nil {
		response.Fail(w, err)
		return
	}
	if req.UploadID != "" {
		if cerr := h.svc.ConsumeUpload(r.Context(), identity.OrganizationID, req.UploadID); cerr != nil {
			// Penandaan gagal bukan kegagalan submit — job sudah tercipta.
			_ = cerr
		}
	}
	response.Accepted(w, map[string]any{
		"job_id":   res.JobID,
		"accepted": res.Accepted,
		"rejected": res.Rejected,
		"replayed": res.Replayed,
	})
}

// Status menangani GET /api/v1/shipments/bulk/{job_id}: status, progres,
// dan hasil per baris (tenant-scoped).
func (h *BulkHandler) Status(w http.ResponseWriter, r *http.Request) {
	identity := middleware.IdentityFrom(r.Context())
	if identity == nil {
		response.Fail(w, unauthorized())
		return
	}
	job, err := h.svc.Job(r.Context(), identity.OrganizationID, chi.URLParam(r, "job_id"))
	if err != nil {
		response.Fail(w, err)
		return
	}
	response.OK(w, job)
}
