package handler

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/angkut/angkut/apps/api/internal/application/shipmentapp"
	"github.com/angkut/angkut/apps/api/internal/domain/shipment"
	"github.com/angkut/angkut/apps/api/internal/infrastructure/label"
	"github.com/angkut/angkut/apps/api/internal/interfaces/http/httpx"
	"github.com/angkut/angkut/apps/api/internal/interfaces/http/middleware"
	"github.com/angkut/angkut/apps/api/internal/interfaces/http/response"
	"github.com/go-chi/chi/v5"
)

// ShipmentService adalah antarmuka minimal service shipment untuk handler.
type ShipmentService interface {
	Create(ctx context.Context, orgID, actorUserID, requestID string, in shipmentapp.CreateInput) (*shipment.Shipment, error)
	Get(ctx context.Context, orgID, id string) (*shipment.Shipment, error)
	List(ctx context.Context, orgID string, f shipment.ListFilter) ([]shipment.Shipment, int, error)
	Transition(ctx context.Context, orgID, actorUserID, requestID, id string, target shipment.Status, reason string) (*shipment.Shipment, error)
	Cancel(ctx context.Context, orgID, actorUserID, requestID, id, reason string) (*shipment.Shipment, error)
}

// ShipmentHandler menangani endpoint pengiriman tenant-scoped.
type ShipmentHandler struct {
	svc ShipmentService
}

// NewShipmentHandler membuat handler pengiriman.
func NewShipmentHandler(svc ShipmentService) *ShipmentHandler {
	return &ShipmentHandler{svc: svc}
}

// RegisterRoutes mendaftarkan rute di /api/v1/shipments.
func (h *ShipmentHandler) RegisterRoutes(r chi.Router) {
	r.Get("/", h.List)
	r.Post("/", h.Create)
	r.Route("/{id}", func(one chi.Router) {
		one.Get("/", h.Get)
		one.Patch("/", h.Update)
		one.Post("/cancel", h.Cancel)
		one.Get("/label", h.Label)
		one.Get("/share-link", h.ShareLink)
	})
}

// List menangani GET /api/v1/shipments.
func (h *ShipmentHandler) List(w http.ResponseWriter, r *http.Request) {
	identity := middleware.IdentityFrom(r.Context())
	if identity == nil {
		response.Fail(w, unauthorized())
		return
	}
	q := r.URL.Query()
	f := shipment.ListFilter{
		Status:      shipment.Status(strings.TrimSpace(q.Get("status"))),
		Courier:     strings.TrimSpace(q.Get("courier")),
		StoreID:     strings.TrimSpace(q.Get("store_id")),
		WarehouseID: strings.TrimSpace(q.Get("warehouse_id")),
		OrderID:     strings.TrimSpace(q.Get("order_id")),
		Search:      strings.TrimSpace(q.Get("search")),
		From:        strings.TrimSpace(q.Get("from")),
		To:          strings.TrimSpace(q.Get("to")),
		Limit:       atoiDefault(q.Get("limit"), 20),
		Offset:      atoiDefault(q.Get("offset"), 0),
	}
	if f.Status != "" && !f.Status.Valid() {
		response.Fail(w, ValidationError(map[string]string{
			"status": "Status pengiriman tidak dikenal.",
		}))
		return
	}

	items, total, err := h.svc.List(r.Context(), identity.OrganizationID, f)
	if err != nil {
		response.Fail(w, err)
		return
	}
	response.List(w, items, map[string]any{
		"total":  total,
		"limit":  f.Limit,
		"offset": f.Offset,
	})
}

// Create menangani POST /api/v1/shipments.
// Header Idempotency-Key bersifat wajib untuk mutasi ini (lihat router).
func (h *ShipmentHandler) Create(w http.ResponseWriter, r *http.Request) {
	identity := middleware.IdentityFrom(r.Context())
	if identity == nil {
		response.Fail(w, unauthorized())
		return
	}
	req, ok := httpx.Decode[shipmentCreateRequest](w, r)
	if !ok {
		return
	}

	items := make([]shipmentapp.CreateItemInput, 0, len(req.Items))
	for _, it := range req.Items {
		items = append(items, shipmentapp.CreateItemInput{
			OrderItemID: it.OrderItemID,
			VariantID:   it.VariantID,
			Name:        it.Name,
			Qty:         it.Qty,
		})
	}

	sh, err := h.svc.Create(r.Context(), identity.OrganizationID, identity.UserID, requestID(r), shipmentapp.CreateInput{
		OrderID:              req.OrderID,
		StoreID:              req.StoreID,
		WarehouseID:          req.WarehouseID,
		CourierCode:          req.CourierCode,
		ServiceCode:          req.ServiceCode,
		OriginAddressID:      req.OriginAddressID,
		DestinationAddressID: req.DestinationAddressID,
		Weight:               req.Weight,
		Length:               req.Length,
		Width:                req.Width,
		Height:               req.Height,
		PackageType:          req.PackageType,
		IsCOD:                req.IsCOD,
		CODAmount:            req.CODAmount,
		DeclaredValue:        req.DeclaredValue,
		Insurance:            req.Insurance,
		Notes:                req.Notes,
		Items:                items,
		IdempotencyKey:       httpx.IdempotencyKey(r),
	})
	if err != nil {
		response.Fail(w, err)
		return
	}
	response.Created(w, shipmentPayload(sh))
}

// Get menangani GET /api/v1/shipments/{id}.
func (h *ShipmentHandler) Get(w http.ResponseWriter, r *http.Request) {
	identity := middleware.IdentityFrom(r.Context())
	if identity == nil {
		response.Fail(w, unauthorized())
		return
	}
	sh, err := h.svc.Get(r.Context(), identity.OrganizationID, chi.URLParam(r, "id"))
	if err != nil {
		response.Fail(w, err)
		return
	}
	response.OK(w, shipmentPayload(sh))
}

// Update menangani PATCH /api/v1/shipments/{id} — memindahkan status.
func (h *ShipmentHandler) Update(w http.ResponseWriter, r *http.Request) {
	identity := middleware.IdentityFrom(r.Context())
	if identity == nil {
		response.Fail(w, unauthorized())
		return
	}
	req, ok := httpx.Decode[shipmentUpdateRequest](w, r)
	if !ok {
		return
	}
	target := shipment.Status(strings.TrimSpace(req.Status))
	if !target.Valid() {
		response.Fail(w, ValidationError(map[string]string{
			"status": "Status pengiriman tidak dikenal.",
		}))
		return
	}
	sh, err := h.svc.Transition(r.Context(), identity.OrganizationID, identity.UserID, requestID(r),
		chi.URLParam(r, "id"), target, strings.TrimSpace(req.Reason))
	if err != nil {
		response.Fail(w, err)
		return
	}
	response.OK(w, shipmentPayload(sh))
}

// Cancel menangani POST /api/v1/shipments/{id}/cancel.
func (h *ShipmentHandler) Cancel(w http.ResponseWriter, r *http.Request) {
	identity := middleware.IdentityFrom(r.Context())
	if identity == nil {
		response.Fail(w, unauthorized())
		return
	}
	req, ok := httpx.Decode[shipmentCancelRequest](w, r)
	if !ok {
		return
	}
	sh, err := h.svc.Cancel(r.Context(), identity.OrganizationID, identity.UserID, requestID(r),
		chi.URLParam(r, "id"), strings.TrimSpace(req.Reason))
	if err != nil {
		response.Fail(w, err)
		return
	}
	response.OK(w, shipmentPayload(sh))
}

// Label menangani GET /api/v1/shipments/{id}/label?format=pdf|a4|thermal.
// Mengembalikan PDF inline (Content-Type application/pdf) sehingga FE dapat
// mengunduhnya atau menampilkannya langsung.
func (h *ShipmentHandler) Label(w http.ResponseWriter, r *http.Request) {
	identity := middleware.IdentityFrom(r.Context())
	if identity == nil {
		response.Fail(w, unauthorized())
		return
	}
	sh, err := h.svc.Get(r.Context(), identity.OrganizationID, chi.URLParam(r, "id"))
	if err != nil {
		response.Fail(w, err)
		return
	}

	format := strings.TrimSpace(r.URL.Query().Get("format"))
	if format == "" || format == "pdf" {
		format = "a4"
	}
	if format != "a4" && format != "thermal" {
		response.Fail(w, ValidationError(map[string]string{
			"format": "Format label harus a4 atau thermal.",
		}))
		return
	}

	cfg := label.DefaultConfig()
	cfg.Format = format
	gen := label.New(cfg)

	pdf, err := gen.Generate(label.Data{
		Shipment:  sh,
		Origin:    label.Party{Name: warehouseLabel(sh)},
		PrintedAt: time.Now().UTC(),
	})
	if err != nil {
		response.Fail(w, fmt.Errorf("membuat label: %w", err))
		return
	}

	w.Header().Set("Content-Type", "application/pdf")
	w.Header().Set("Content-Disposition",
		fmt.Sprintf("inline; filename=\"label-%s.pdf\"", sh.AWB))
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusOK)
	if _, err := w.Write(pdf); err != nil {
		// Respons sudah terkirim; hanya catat — tidak bisa mengubah status.
		return
	}
}

// ShareLink menangani GET /api/v1/shipments/{id}/share-link.
//
// Tautan publik pelacakan dibangun dari WEB_URL + resi. Endpoint publik
// pelacakan menyusul di Phase 4 (TASK-035); tautan tetap dilaporkan apa
// adanya dengan masa berlaku sehingga FE tidak menyimpan logika kadaluarsa.
func (h *ShipmentHandler) ShareLink(w http.ResponseWriter, r *http.Request) {
	identity := middleware.IdentityFrom(r.Context())
	if identity == nil {
		response.Fail(w, unauthorized())
		return
	}
	sh, err := h.svc.Get(r.Context(), identity.OrganizationID, chi.URLParam(r, "id"))
	if err != nil {
		response.Fail(w, err)
		return
	}
	// Masa berlaku 30 hari; nilai ini dipakai FE untuk menampilkan tenggat.
	expires := time.Now().UTC().Add(30 * 24 * time.Hour)
	response.OK(w, map[string]any{
		"awb":        sh.AWB,
		"url":        "/track/" + sh.AWB,
		"expires_at": expires.Format(time.RFC3339),
	})
}

// warehouseLabel mengembalikan nama tampilan asal (sementara: alamat asal).
func warehouseLabel(sh *shipment.Shipment) string {
	if sh.WarehouseID != "" {
		return "Gudang " + sh.WarehouseID[:min(8, len(sh.WarehouseID))]
	}
	return "Asal pengiriman"
}

// shipmentPayload menyusun bentuk respons sesuai kontrak: field ringkas +
// detail paket/label yang dibutuhkan FE.
func shipmentPayload(sh *shipment.Shipment) map[string]any {
	if sh == nil {
		return nil
	}
	var qr, barcode string
	if len(sh.Packages) > 0 {
		qr = sh.Packages[0].QRPayload
		barcode = sh.Packages[0].BarcodeValue
	}
	return map[string]any{
		"id":     sh.ID,
		"awb":    sh.AWB,
		"status": string(sh.Status),
		"courier": map[string]any{
			"code":    sh.CourierCode,
			"service": sh.ServiceCode,
		},
		"order_id":               sh.OrderID,
		"store_id":               sh.StoreID,
		"warehouse_id":           sh.WarehouseID,
		"origin_address_id":      sh.OriginAddressID,
		"destination_address_id": sh.DestinationAddressID,
		"package": map[string]any{
			"actual_weight":     sh.ActualWeight,
			"volumetric_weight": sh.VolumetricWeight,
			"chargeable_weight": sh.ChargeableWeight,
			"length":            sh.Length,
			"width":             sh.Width,
			"height":            sh.Height,
			"type":              sh.PackageType,
		},
		"pricing": map[string]any{
			"shipping_price":   sh.ShippingPrice,
			"cost_price":       sh.CostPrice,
			"insurance_amount": sh.InsuranceAmount,
			"cod_fee":          sh.CODFee,
			"is_cod":           sh.IsCOD,
			"cod_amount":       sh.CODAmount,
			"declared_value":   sh.DeclaredValue,
		},
		"eta_min_days":  sh.EtaMinDays,
		"eta_max_days":  sh.EtaMaxDays,
		"qr_payload":    qr,
		"barcode_value": barcode,
		"label_url":     "/api/v1/shipments/" + sh.ID + "/label",
		"items":         sh.Items,
		"packages":      sh.Packages,
		"notes":         sh.Notes,
		"created_at":    sh.CreatedAt,
		"updated_at":    sh.UpdatedAt,
	}
}

// ─── Request DTOs ─────────────────────────────────────────────────────────

type shipmentCreateRequest struct {
	OrderID              string   `json:"order_id"`
	StoreID              string   `json:"store_id"`
	WarehouseID          string   `json:"warehouse_id"`
	CourierCode          string   `json:"courier_code"`
	ServiceCode          string   `json:"service_code"`
	OriginAddressID      string   `json:"origin_address_id"`
	DestinationAddressID string   `json:"destination_address_id"`
	Weight               float64  `json:"weight"`
	Length               *float64 `json:"length"`
	Width                *float64 `json:"width"`
	Height               *float64 `json:"height"`
	PackageType          string   `json:"package_type"`
	IsCOD                bool     `json:"is_cod"`
	CODAmount            int64    `json:"cod_amount"`
	DeclaredValue        int64    `json:"declared_value"`
	Insurance            bool     `json:"insurance"`
	Notes                string   `json:"notes"`
	Items                []struct {
		OrderItemID string `json:"order_item_id"`
		VariantID   string `json:"variant_id"`
		Name        string `json:"name"`
		Qty         int    `json:"qty"`
	} `json:"items"`
}

type shipmentUpdateRequest struct {
	Status string `json:"status"`
	Reason string `json:"reason"`
}

type shipmentCancelRequest struct {
	Reason string `json:"reason"`
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
