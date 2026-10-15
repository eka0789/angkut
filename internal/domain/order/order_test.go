package order

import (
	"fmt"
	"testing"

	"github.com/angkut/angkut/apps/internal/domain/shared"
)

func validOrder() *Order {
	return &Order{
		OrganizationID: "org-1",
		CustomerID:     "cust-1",
		OrderNumber:    "ORD-2026-000001",
		Status:         StatusDraft,
		PaymentType:    PaymentPrepaid,
		Items: []Item{
			{ProductNameSnapshot: "Sepatu", Qty: 2, UnitPrice: 150_000},
			{ProductNameSnapshot: "Kaos", Qty: 1, UnitPrice: 75_000},
		},
	}
}

func TestRecalculateSumsItemsDeterministically(t *testing.T) {
	o := validOrder()
	o.ShippingCost = 21_000
	o.Discount = 10_000
	o.Recalculate()

	// subtotal = 2×150000 + 1×75000 = 375000
	if o.Subtotal != 375_000 {
		t.Errorf("subtotal = %d, ingin 375000", o.Subtotal)
	}
	// total = 375000 - 10000 + 21000 = 386000
	if o.TotalAmount != 386_000 {
		t.Errorf("total = %d, ingin 386000", o.TotalAmount)
	}
	// Total item harus ikut dihitung, bukan dikirim klien.
	if o.Items[0].TotalPrice != 300_000 || o.Items[1].TotalPrice != 75_000 {
		t.Errorf("total item salah: %d, %d", o.Items[0].TotalPrice, o.Items[1].TotalPrice)
	}
}

func TestValidateRejectsEmptyItems(t *testing.T) {
	o := validOrder()
	o.Items = nil
	err := o.Validate()
	if err == nil {
		t.Fatal("order tanpa item harus ditolak")
	}
	var de *shared.Error
	if !shared.As(err, &de) {
		t.Fatalf("error harus shared.Error, dapat %T", err)
	}
	if de.Fields["items"] == "" {
		t.Errorf("error harus menyebut field items; dapat %+v", de.Fields)
	}
}

func TestValidateRejectsDiscountBeyondTotal(t *testing.T) {
	o := validOrder()
	o.Recalculate()
	o.Discount = o.Subtotal + o.ShippingCost + 1
	if err := o.Validate(); err == nil {
		t.Fatal("diskon melebihi subtotal+ongkir harus ditolak")
	}
}

func TestValidateAcceptsWellFormedOrder(t *testing.T) {
	o := validOrder()
	o.Recalculate()
	if err := o.Validate(); err != nil {
		t.Fatalf("order valid ditolak: %v", err)
	}
}

func TestStatusMachineAllowsHappyPath(t *testing.T) {
	path := []Status{StatusConfirmed, StatusProcessing, StatusShipped, StatusDelivered}

	o := validOrder()
	o.Recalculate()
	for _, target := range path {
		if err := o.Transition(target); err != nil {
			t.Fatalf("transisi ke %s gagal: %v", target, err)
		}
	}
	if o.Status != StatusDelivered {
		t.Errorf("status akhir = %s, ingin delivered", o.Status)
	}
}

func TestStatusMachineRejectsIllegalJumps(t *testing.T) {
	cases := []struct {
		name string
		from Status
		to   Status
	}{
		{"draft langsung delivered", StatusDraft, StatusDelivered},
		{"draft langsung shipped", StatusDraft, StatusShipped},
		{"confirmed langsung delivered", StatusConfirmed, StatusDelivered},
		{"draft kembali processing", StatusDraft, StatusProcessing},
		{"delivered kembali shipped", StatusDelivered, StatusShipped},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			o := validOrder()
			o.Status = tc.from
			if err := o.Transition(tc.to); err == nil {
				t.Fatalf("transisi %s → %s seharusnya ditolak", tc.from, tc.to)
			}
		})
	}
}

func TestStatusMachineTerminalStatesHaveNoExit(t *testing.T) {
	for _, terminal := range []Status{StatusCancelled, StatusReturned} {
		if got := terminal.AllowedTransitions(); len(got) != 0 {
			t.Errorf("%s harus terminal, dapat transisi %v", terminal, got)
		}
	}
}

func TestTransitionToSameStatusIsIdempotentNoop(t *testing.T) {
	o := validOrder()
	o.Status = StatusConfirmed
	if err := o.Transition(StatusConfirmed); err != nil {
		t.Fatalf("transisi ke status sama harus no-op, dapat %v", err)
	}
	if o.Status != StatusConfirmed {
		t.Errorf("status berubah tak terduga: %s", o.Status)
	}
}

func TestTransitionRejectsUnknownStatus(t *testing.T) {
	o := validOrder()
	if err := o.Transition(Status("melayang")); err == nil {
		t.Fatal("status tak dikenal harus ditolak")
	}
}

func TestItemValidateRejectsNonPositiveQty(t *testing.T) {
	it := Item{ProductNameSnapshot: "Sepatu", Qty: 0, UnitPrice: 1000}
	if err := it.Validate(); err == nil {
		t.Fatal("qty 0 harus ditolak")
	}
}

func TestStatusValidCoversMigrationCheckConstraint(t *testing.T) {
	// Nilai ini harus sama persis dengan CHECK di migrasi 0015 — test menjaga
	// keduanya tidak lepas sinkron.
	want := []string{"draft", "confirmed", "processing", "shipped", "delivered", "cancelled", "returned"}
	if len(AllStatuses) != len(want) {
		t.Fatalf("jumlah status = %d, ingin %d", len(AllStatuses), len(want))
	}
	for i, w := range want {
		if string(AllStatuses[i]) != w {
			t.Errorf("status[%d] = %q, ingin %q", i, AllStatuses[i], w)
		}
	}
}

func TestPaymentTypeValid(t *testing.T) {
	if !PaymentPrepaid.Valid() || !PaymentCOD.Valid() {
		t.Error("prepaid & cod harus valid")
	}
	if PaymentType("kredit").Valid() {
		t.Error("tipe pembayaran tak dikenal harus invalid")
	}
}

func TestOrderNumberFormatIsStable(t *testing.T) {
	// Format nomor order dipakai FE & pelanggan; jaga tetap stabil.
	got := FormatOrderNumber(2026, 123)
	if got != "ORD-2026-000123" {
		t.Errorf("FormatOrderNumber = %q, ingin ORD-2026-000123", got)
	}
	if got := FormatOrderNumber(2026, 1); got != fmt.Sprintf("ORD-2026-%06d", 1) {
		t.Errorf("padding nomor order salah: %q", got)
	}
}
