package handler_test

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"

	"github.com/angkut/angkut/apps/api/internal/interfaces/http/handler"
	"github.com/angkut/angkut/apps/api/internal/interfaces/http/middleware"
	"github.com/angkut/angkut/apps/internal/application/shipmentapp"
	"github.com/angkut/angkut/apps/internal/domain/shipment"
)

// stubShipmentService menyediakan pengiriman untuk uji label massal.
type stubShipmentService struct {
	items []shipment.Shipment
}

func (s stubShipmentService) Create(context.Context, string, string, string, shipmentapp.CreateInput) (*shipment.Shipment, error) {
	return nil, nil
}

func (s stubShipmentService) Get(context.Context, string, string) (*shipment.Shipment, error) {
	if len(s.items) == 0 {
		return nil, nil
	}
	return &s.items[0], nil
}

func (s stubShipmentService) List(context.Context, string, shipment.ListFilter) ([]shipment.Shipment, int, error) {
	return s.items, len(s.items), nil
}

func (s stubShipmentService) GetByIDs(_ context.Context, _ string, ids []string) ([]shipment.Shipment, error) {
	want := make(map[string]bool, len(ids))
	for _, id := range ids {
		want[id] = true
	}
	var out []shipment.Shipment
	for _, it := range s.items {
		if want[it.ID] {
			out = append(out, it)
		}
	}
	return out, nil
}

func (s stubShipmentService) Transition(context.Context, string, string, string, string, shipment.Status, string) (*shipment.Shipment, error) {
	return nil, nil
}

func (s stubShipmentService) Cancel(context.Context, string, string, string, string, string) (*shipment.Shipment, error) {
	return nil, nil
}

// TestBulkLabelsZipKontrak: POST /shipments/labels/bulk mengembalikan ZIP yang
// benar-benar bisa dibaca, satu PDF per pengiriman, dan id milik organisasi
// lain tidak ikut tercetak.
func TestBulkLabelsZipKontrak(t *testing.T) {
	items := make([]shipment.Shipment, 0, 60)
	for i := 0; i < 60; i++ {
		items = append(items, shipment.Shipment{
			ID:          uuid.NewString(),
			AWB:         "JNE260101" + pad3(i),
			CourierCode: "jne", ServiceCode: "REG",
			ActualWeight: 1.5, ChargeableWeight: 1.5,
			EtaMinDays: 1, EtaMaxDays: 2,
		})
	}
	h := handler.NewShipmentHandler(stubShipmentService{items: items})

	body, _ := json.Marshal(map[string]any{
		"ids":    []string{items[0].ID, items[1].ID, uuid.NewString()},
		"format": "a4",
	})
	req := httptest.NewRequest(http.MethodPost, "/shipments/labels/bulk", bytes.NewReader(body))
	req = req.WithContext(middleware.WithTestIdentity(req.Context(), &middleware.Identity{
		UserID: "u-1", OrganizationID: "org-1",
	}))
	rec := httptest.NewRecorder()
	h.BulkLabels(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, ingin 200; body=%s", rec.Code, rec.Body.String())
	}
	zr, err := zip.NewReader(bytes.NewReader(rec.Body.Bytes()), int64(rec.Body.Len()))
	if err != nil {
		t.Fatalf("ZIP tidak valid: %v", err)
	}
	if len(zr.File) != 2 {
		t.Fatalf("entri ZIP = %d, ingin 2 (id asing tidak dicetak)", len(zr.File))
	}
	for _, f := range zr.File {
		if len(f.Name) < 4 || f.Name[len(f.Name)-4:] != ".pdf" {
			t.Errorf("entri %q bukan PDF", f.Name)
		}
		rc, err := f.Open()
		if err != nil {
			t.Fatalf("membuka %q: %v", f.Name, err)
		}
		buf := new(bytes.Buffer)
		if _, err := buf.ReadFrom(rc); err != nil {
			t.Fatalf("membaca %q: %v", f.Name, err)
		}
		rc.Close()
		if !bytes.HasPrefix(buf.Bytes(), []byte("%PDF")) {
			t.Errorf("isi %q bukan PDF", f.Name)
		}
	}
}

func pad3(n int) string {
	s := []byte("000")
	v := n
	for i := 2; i >= 0 && v > 0; i-- {
		s[i] = byte('0' + v%10)
		v /= 10
	}
	return string(s)
}
