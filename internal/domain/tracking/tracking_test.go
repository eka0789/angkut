package tracking

import (
	"testing"
	"time"

	"github.com/angkut/angkut/apps/internal/domain/shared"
	"github.com/angkut/angkut/apps/internal/domain/shipment"
)

func ts(days int) time.Time {
	return time.Date(2026, 1, 1, 8, 0, 0, 0, time.UTC).AddDate(0, 0, days)
}

func TestValidateAcceptsWellFormedEvent(t *testing.T) {
	lat, lng := -6.9, 107.6
	e := &Event{
		ShipmentID: "ship-1",
		Status:     EventInTransit,
		OccurredAt: ts(1),
		Source:     SourceWebhook,
		Latitude:   &lat,
		Longitude:  &lng,
	}
	if err := e.Validate(); err != nil {
		t.Fatalf("event valid ditolak: %v", err)
	}
}

func TestValidateRejectsUnpairedCoordinates(t *testing.T) {
	lat := -6.9
	e := &Event{ShipmentID: "s", Status: EventInTransit, OccurredAt: ts(0), Source: SourceManual, Latitude: &lat}
	err := e.Validate()
	if err == nil {
		t.Fatal("koordinat tidak berpasangan harus ditolak")
	}
	var de *shared.Error
	if !shared.As(err, &de) || de.Fields["coordinates"] == "" {
		t.Errorf("error harus menyebut coordinates; dapat %v", err)
	}
}

func TestValidateRejectsOutOfRangeCoordinates(t *testing.T) {
	bad := 200.0
	e := &Event{
		ShipmentID: "s", Status: EventInTransit, OccurredAt: ts(0), Source: SourceManual,
		Latitude: &bad, Longitude: &bad,
	}
	if err := e.Validate(); err == nil {
		t.Fatal("koordinat di luar rentang harus ditolak")
	}
}

func TestValidateRejectsUnknownStatusAndSource(t *testing.T) {
	e := &Event{ShipmentID: "s", Status: EventStatus("melayang"), OccurredAt: ts(0), Source: SourceSystem}
	if err := e.Validate(); err == nil {
		t.Fatal("status tak dikenal harus ditolak")
	}
	e = &Event{ShipmentID: "s", Status: EventInTransit, OccurredAt: ts(0), Source: Source("telepati")}
	if err := e.Validate(); err == nil {
		t.Fatal("sumber tak dikenal harus ditolak")
	}
}

func TestValidateRequiresOccurredAt(t *testing.T) {
	e := &Event{ShipmentID: "s", Status: EventInTransit, Source: SourceSystem}
	if err := e.Validate(); err == nil {
		t.Fatal("occurred_at kosong harus ditolak")
	}
}

func TestShipmentStatusForMapping(t *testing.T) {
	cases := []struct {
		event EventStatus
		want  shipment.Status
		ok    bool
	}{
		{EventPickedUp, shipment.StatusPickedUp, true},
		{EventInTransit, shipment.StatusInTransit, true},
		{EventOutForDelivery, shipment.StatusOutForDelivery, true},
		{EventDelivered, shipment.StatusDelivered, true},
		{EventFailed, shipment.StatusFailed, true},
		{EventReturned, shipment.StatusReturned, true},
		// Event informasi tidak memindahkan status pengiriman.
		{EventCreated, "", false},
		{EventSorting, "", false},
		{EventPickupRequested, "", false},
	}
	for _, tc := range cases {
		got, ok := ShipmentStatusFor(tc.event)
		if ok != tc.ok || got != tc.want {
			t.Errorf("ShipmentStatusFor(%s) = (%q,%v), ingin (%q,%v)", tc.event, got, ok, tc.want, tc.ok)
		}
		if tc.event.PromotesShipment() != tc.ok {
			t.Errorf("PromotesShipment(%s) = %v, ingin %v", tc.event, !tc.ok, tc.ok)
		}
	}
}

func TestBuildTimelineOrdersChronologically(t *testing.T) {
	events := []Event{
		{Status: EventInTransit, OccurredAt: ts(2), Location: "Cirebon"},
		{Status: EventPickedUp, OccurredAt: ts(1), Location: "Semarang"},
		{Status: EventOutForDelivery, OccurredAt: ts(3), Location: "Bandung"},
	}
	tl := BuildTimeline("JNE1", events)

	if len(tl.Events) != 3 {
		t.Fatalf("jumlah event = %d", len(tl.Events))
	}
	// Terlama dulu — urutan yang dibaca manusia.
	if tl.Events[0].Status != EventPickedUp || tl.Events[2].Status != EventOutForDelivery {
		t.Errorf("urutan salah: %v, %v", tl.Events[0].Status, tl.Events[2].Status)
	}
	// Status timeline = event terakhir, bukan yang pertama.
	if tl.Status != string(EventOutForDelivery) {
		t.Errorf("status = %q, ingin out_for_delivery", tl.Status)
	}
	if tl.LastLocation != "Bandung" {
		t.Errorf("lokasi terakhir = %q, ingin Bandung", tl.LastLocation)
	}
}

func TestBuildTimelineIsDeterministicForEqualTimestamps(t *testing.T) {
	// Waktu identik harus menghasilkan urutan yang SAMA setiap kali —
	// timeline adalah bukti, bukan tampilan acak.
	mk := func() []Event {
		return []Event{
			{Status: EventOutForDelivery, OccurredAt: ts(1)},
			{Status: EventPickedUp, OccurredAt: ts(1)},
			{Status: EventInTransit, OccurredAt: ts(1)},
		}
	}
	first := BuildTimeline("A", mk())
	for i := 0; i < 5; i++ {
		again := BuildTimeline("A", mk())
		for j := range again.Events {
			if again.Events[j].Status != first.Events[j].Status {
				t.Fatalf("urutan tidak stabil pada percobaan %d: %v vs %v",
					i, again.Events[j].Status, first.Events[j].Status)
			}
		}
	}
	// Urutan mengikuti status kanonik: picked_up sebelum in_transit sebelum out_for_delivery.
	if first.Events[0].Status != EventPickedUp {
		t.Errorf("event pertama = %q, ingin picked_up", first.Events[0].Status)
	}
}

func TestBuildTimelineUsesMostRecentCoordinates(t *testing.T) {
	lat1, lng1 := -6.9, 107.6
	lat2, lng2 := -6.5, 107.0
	events := []Event{
		{Status: EventPickedUp, OccurredAt: ts(1), Latitude: &lat1, Longitude: &lng1},
		{Status: EventInTransit, OccurredAt: ts(2), Latitude: &lat2, Longitude: &lng2},
		{Status: EventOutForDelivery, OccurredAt: ts(3)}, // tanpa koordinat
	}
	tl := BuildTimeline("A", events)

	if tl.LastLat == nil || *tl.LastLat != lat2 {
		t.Errorf("last_lat = %v, ingin %v (event koordinat terbaru)", tl.LastLat, lat2)
	}
}

func TestBuildTimelineEmpty(t *testing.T) {
	tl := BuildTimeline("A", nil)
	if len(tl.Events) != 0 {
		t.Errorf("event harus kosong, dapat %d", len(tl.Events))
	}
	if tl.AWB != "A" {
		t.Errorf("awb = %q", tl.AWB)
	}
}

func TestConfidenceOnTrackWithinEta(t *testing.T) {
	booked := ts(0)
	sh := &shipment.Shipment{
		Status:     shipment.StatusInTransit,
		BookedAt:   &booked,
		EtaMaxDays: 3,
	}
	c := EvaluateConfidence(sh, booked.AddDate(0, 0, 2))
	if c.Level != ConfidenceOnTrack {
		t.Errorf("level = %q, ingin on_track", c.Level)
	}
}

func TestConfidenceDelayedPastEta(t *testing.T) {
	booked := ts(0)
	sh := &shipment.Shipment{
		Status:     shipment.StatusInTransit,
		BookedAt:   &booked,
		EtaMaxDays: 3,
	}
	c := EvaluateConfidence(sh, booked.AddDate(0, 0, 5))
	if c.Level != ConfidenceDelayed {
		t.Fatalf("level = %q, ingin delayed", c.Level)
	}
	if c.Reason == "" {
		t.Error("confidence delayed wajib menyertakan alasan")
	}
}

func TestConfidenceDeliveredIsOnTrack(t *testing.T) {
	booked := ts(0)
	sh := &shipment.Shipment{
		Status:     shipment.StatusDelivered,
		BookedAt:   &booked,
		EtaMaxDays: 3,
	}
	// Walau dihitung jauh melewati ETA, paket yang sudah terkirim tidak "terlambat".
	if c := EvaluateConfidence(sh, booked.AddDate(0, 0, 30)); c.Level != ConfidenceOnTrack {
		t.Errorf("level = %q, ingin on_track", c.Level)
	}
}

func TestConfidenceUnknownWithoutBooking(t *testing.T) {
	sh := &shipment.Shipment{Status: shipment.StatusInTransit}
	if c := EvaluateConfidence(sh, ts(5)); c.Level != ConfidenceUnknown {
		t.Errorf("level = %q, ingin unknown tanpa booked_at", c.Level)
	}
	if c := EvaluateConfidence(nil, ts(5)); c.Level != ConfidenceUnknown {
		t.Errorf("level = %q, ingin unknown untuk nil", c.Level)
	}
}

func TestEventStatusesMatchMigrationCheck(t *testing.T) {
	want := []string{"created", "pickup_requested", "picked_up", "sorting",
		"in_transit", "out_for_delivery", "delivered", "failed", "returned"}
	if len(AllEventStatuses) != len(want) {
		t.Fatalf("jumlah status = %d, ingin %d", len(AllEventStatuses), len(want))
	}
	for i, w := range want {
		if string(AllEventStatuses[i]) != w {
			t.Errorf("status[%d] = %q, ingin %q", i, AllEventStatuses[i], w)
		}
	}
}

func TestDescriptionForCoversAllStatuses(t *testing.T) {
	for _, s := range AllEventStatuses {
		if d := DescriptionFor(s); d == "" || d == string(s) {
			t.Errorf("deskripsi bawaan untuk %q tidak tersedia: %q", s, d)
		}
	}
}
