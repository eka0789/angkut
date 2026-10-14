package shipment

import (
	"strings"
	"testing"

	"github.com/angkut/angkut/apps/api/internal/domain/shared"
)

func validShipment() *Shipment {
	return &Shipment{
		OrganizationID:   "org-1",
		CourierCode:      "jne",
		ServiceCode:      "REG",
		AWB:              "JNE260101000001",
		Status:           StatusDraft,
		ActualWeight:     1.5,
		ChargeableWeight: 1.5,
		ShippingPrice:    21_000,
		CostPrice:        18_500,
		Items:            []Item{{NameSnapshot: "Sepatu", Qty: 1}},
		Packages:         []Package{{Sequence: 1, Weight: 1.5, QRPayload: "q", BarcodeValue: "b"}},
	}
}

func TestValidateAcceptsWellFormedShipment(t *testing.T) {
	if err := validShipment().Validate(); err != nil {
		t.Fatalf("shipment valid ditolak: %v", err)
	}
}

func TestValidateRejectsZeroWeight(t *testing.T) {
	s := validShipment()
	s.ActualWeight = 0
	err := s.Validate()
	if err == nil {
		t.Fatal("berat 0 harus ditolak")
	}
	var de *shared.Error
	if !shared.As(err, &de) || de.Fields["weight"] == "" {
		t.Errorf("error harus menyebut weight; dapat %v", err)
	}
}

func TestValidateCODInvariant(t *testing.T) {
	// COD tanpa nilai.
	s := validShipment()
	s.IsCOD = true
	s.CODAmount = 0
	if err := s.Validate(); err == nil {
		t.Fatal("COD tanpa nilai harus ditolak")
	}

	// Bukan COD tetapi ada nilai COD.
	s = validShipment()
	s.IsCOD = false
	s.CODAmount = 50_000
	if err := s.Validate(); err == nil {
		t.Fatal("nilai COD pada non-COD harus ditolak")
	}

	// COD dengan nilai: sah.
	s = validShipment()
	s.IsCOD = true
	s.CODAmount = 350_000
	if err := s.Validate(); err != nil {
		t.Fatalf("COD valid ditolak: %v", err)
	}
}

func TestShipmentStateMachineHappyPath(t *testing.T) {
	path := []Status{StatusPendingPickup, StatusPickedUp, StatusInTransit, StatusOutForDelivery, StatusDelivered}

	s := validShipment()
	for _, target := range path {
		if err := s.Transition(target); err != nil {
			t.Fatalf("transisi ke %s gagal: %v", target, err)
		}
	}
	if s.Status != StatusDelivered {
		t.Errorf("status akhir = %s, ingin delivered", s.Status)
	}
	if s.DeliveredAt == nil {
		t.Error("delivered_at harus terisi saat status delivered")
	}
	if s.BookedAt == nil {
		t.Error("booked_at harus terisi saat masuk pending_pickup")
	}
}

func TestShipmentStateMachineRejectsIllegalJump(t *testing.T) {
	cases := []struct{ from, to Status }{
		{StatusDraft, StatusDelivered},
		{StatusDraft, StatusInTransit},
		{StatusPendingPickup, StatusDelivered},
		{StatusDelivered, StatusInTransit}, // terminal tidak punya keluar
		{StatusCancelled, StatusPendingPickup},
	}
	for _, tc := range cases {
		s := validShipment()
		s.Status = tc.from
		if err := s.Transition(tc.to); err == nil {
			t.Errorf("transisi %s → %s seharusnya ditolak", tc.from, tc.to)
		}
	}
}

func TestShipmentFailedCanBeRetried(t *testing.T) {
	// Pengiriman gagal antar boleh dicoba lagi — bukan jalan buntu.
	s := validShipment()
	s.Status = StatusInTransit
	if err := s.Transition(StatusFailed); err != nil {
		t.Fatalf("in_transit → failed: %v", err)
	}
	if err := s.Transition(StatusPendingPickup); err != nil {
		t.Fatalf("failed → pending_pickup (retry) ditolak: %v", err)
	}
}

func TestTerminalStatusesHaveNoExit(t *testing.T) {
	for _, terminal := range []Status{StatusDelivered, StatusReturned, StatusCancelled} {
		if !terminal.IsTerminal() {
			t.Errorf("%s harus terminal", terminal)
		}
		if got := terminal.AllowedTransitions(); len(got) != 0 {
			t.Errorf("%s harus tanpa transisi, dapat %v", terminal, got)
		}
	}
}

func TestAWBFormat(t *testing.T) {
	got := AWB{CourierCode: "jne", Year: 2026, Month: 1, Day: 5, Seq: 42}.String()
	if got != "JNE260105000042" {
		t.Errorf("AWB = %q, ingin JNE260105000042", got)
	}
	// Kode kurir huruf besar, padding urutan 6 digit.
	got = AWB{CourierCode: "SiCepat", Year: 2026, Month: 12, Day: 31, Seq: 999999}.String()
	if got != "SICEPAT261231999999" {
		t.Errorf("AWB = %q, ingin SICEPAT261231999999", got)
	}
}

func TestQRPayloadShape(t *testing.T) {
	got := QRPayload("ship-1", "JNE260101000001")
	if got != "ANGKUT|ship-1|JNE260101000001" {
		t.Errorf("QRPayload = %q", got)
	}
	if strings.Count(got, "|") != 2 {
		t.Error("payload QR harus punya tepat 2 pemisah")
	}
}

func TestBarcodeValueNormalised(t *testing.T) {
	if got := BarcodeValue(" jne260101000001 "); got != "JNE260101000001" {
		t.Errorf("BarcodeValue = %q", got)
	}
}

func TestStatusValidCoversMigrationCheckConstraint(t *testing.T) {
	// Harus sama persis dengan CHECK di migrasi 0016.
	want := []string{"draft", "pending_pickup", "picked_up", "in_transit",
		"out_for_delivery", "delivered", "failed", "returned", "cancelled"}
	if len(AllStatuses) != len(want) {
		t.Fatalf("jumlah status = %d, ingin %d", len(AllStatuses), len(want))
	}
	for i, w := range want {
		if string(AllStatuses[i]) != w {
			t.Errorf("status[%d] = %q, ingin %q", i, AllStatuses[i], w)
		}
	}
}
