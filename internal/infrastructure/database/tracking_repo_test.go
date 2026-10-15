package database_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/angkut/angkut/apps/internal/infrastructure/database"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/angkut/angkut/apps/internal/domain/event"
	"github.com/angkut/angkut/apps/internal/domain/tracking"
	"github.com/angkut/angkut/apps/internal/domain/webhook"
)

// seedTrackingFixtures menyiapkan org + kurir + layanan + shipment agar event
// tracking punya induk yang sah, lalu membersihkannya.
func seedTrackingFixtures(t *testing.T, pool *pgxpool.Pool) (orgID, shipmentID, awb string) {
	t.Helper()
	ctx := context.Background()

	var userID string
	if err := pool.QueryRow(ctx, `SELECT gen_random_uuid()::text`).Scan(&orgID); err != nil {
		t.Fatalf("gen org: %v", err)
	}
	if err := pool.QueryRow(ctx, `SELECT gen_random_uuid()::text`).Scan(&userID); err != nil {
		t.Fatalf("gen user: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO users (id, email, name, password_hash, status)
		VALUES ($1,$2,'Uji Track','x','active')`, userID, "trk-"+userID[:8]+"@angkut.test"); err != nil {
		t.Fatalf("seed user: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO organizations (id, name, slug, owner_user_id) VALUES ($1,$2,$3,$4)`,
		orgID, "Org Track", "trk-"+orgID[:8], userID); err != nil {
		t.Fatalf("seed org: %v", err)
	}

	shipmentID = uuid.NewString()
	awb = "TRK" + shipmentID[:6]
	if _, err := pool.Exec(ctx, `
		INSERT INTO shipments (
			id, organization_id, courier_code, service_code, awb, status,
			actual_weight, volumetric_weight, chargeable_weight,
			shipping_price, cost_price, version, created_at, updated_at
		) VALUES ($1,$2,'jne','REG',$3,'pending_pickup',1.5,0,1.5,21000,18500,1,now(),now())`,
		shipmentID, orgID, awb); err != nil {
		t.Fatalf("seed shipment: %v", err)
	}

	t.Cleanup(func() {
		_, _ = pool.Exec(ctx, `DELETE FROM tracking_events WHERE organization_id=$1`, orgID)
		_, _ = pool.Exec(ctx, `DELETE FROM webhook_events WHERE organization_id=$1`, orgID)
		_, _ = pool.Exec(ctx, `DELETE FROM webhook_events WHERE payload_hash LIKE 'uji-%'`)
		_, _ = pool.Exec(ctx, `DELETE FROM outbox_events WHERE organization_id=$1`, orgID)
		_, _ = pool.Exec(ctx, `DELETE FROM shipment_packages WHERE organization_id=$1`, orgID)
		_, _ = pool.Exec(ctx, `DELETE FROM shipment_items WHERE organization_id=$1`, orgID)
		_, _ = pool.Exec(ctx, `DELETE FROM shipments WHERE organization_id=$1`, orgID)
		_, _ = pool.Exec(ctx, `DELETE FROM organizations WHERE id=$1`, orgID)
		_, _ = pool.Exec(ctx, `DELETE FROM users WHERE id=$1`, userID)
	})
	return orgID, shipmentID, awb
}

func TestTrackingAppendAndListByShipment(t *testing.T) {
	pool, stores := orderTestDB(t)
	orgID, shipmentID, awb := seedTrackingFixtures(t, pool)
	repo := stores.Tracking()
	ctx := context.Background()

	now := time.Now().UTC().Truncate(time.Second)
	events := []*tracking.Event{
		{
			ID: uuid.NewString(), OrganizationID: orgID, ShipmentID: shipmentID,
			Status: tracking.EventPickedUp, Description: "Dijemput", Location: "Jakarta",
			OccurredAt: now, Source: tracking.SourceWebhook, CreatedAt: now,
		},
		{
			ID: uuid.NewString(), OrganizationID: orgID, ShipmentID: shipmentID,
			Status: tracking.EventInTransit, Location: "Cirebon",
			OccurredAt: now.Add(time.Hour), Source: tracking.SourceWebhook, CreatedAt: now,
		},
	}
	for _, e := range events {
		if err := repo.Append(ctx, e); err != nil {
			t.Fatalf("Append(%s): %v", e.Status, err)
		}
	}

	got, err := repo.ListByShipment(ctx, orgID, shipmentID)
	if err != nil {
		t.Fatalf("ListByShipment: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("jumlah event = %d, ingin 2", len(got))
	}
	// Terurut naik berdasarkan waktu.
	if got[0].Status != tracking.EventPickedUp || got[1].Status != tracking.EventInTransit {
		t.Errorf("urutan salah: %v, %v", got[0].Status, got[1].Status)
	}

	// Timeline dari event tersimpan.
	tl := tracking.BuildTimeline(awb, got)
	if tl.Status != string(tracking.EventInTransit) {
		t.Errorf("status timeline = %q", tl.Status)
	}
	if tl.LastLocation != "Cirebon" {
		t.Errorf("lokasi terakhir = %q", tl.LastLocation)
	}
}

func TestTrackingAppendRejectsNaturalKeyDuplicate(t *testing.T) {
	// Kunci alami (shipment_id, status, occurred_at, source) harus menolak
	// event yang sama persis — inilah pertahanan terhadap webhook berulang.
	pool, stores := orderTestDB(t)
	orgID, shipmentID, _ := seedTrackingFixtures(t, pool)
	repo := stores.Tracking()
	ctx := context.Background()

	occurred := time.Now().UTC().Truncate(time.Second)
	e := &tracking.Event{
		ID: uuid.NewString(), OrganizationID: orgID, ShipmentID: shipmentID,
		Status: tracking.EventDelivered, OccurredAt: occurred,
		Source: tracking.SourceWebhook, CreatedAt: occurred,
	}
	if err := repo.Append(ctx, e); err != nil {
		t.Fatalf("Append pertama: %v", err)
	}

	dup := &tracking.Event{
		ID: uuid.NewString(), OrganizationID: orgID, ShipmentID: shipmentID,
		Status: tracking.EventDelivered, OccurredAt: occurred,
		Source: tracking.SourceWebhook, CreatedAt: occurred,
	}
	err := repo.Append(ctx, dup)
	if err == nil {
		t.Fatal("event dengan kunci alami sama harus ditolak")
	}
	if !tracking.IsDuplicate(err) {
		t.Errorf("error = %v, ingin tracking.ErrDuplicate", err)
	}

	// Sumber berbeda untuk status & waktu sama dianggap kejadian berbeda.
	other := &tracking.Event{
		ID: uuid.NewString(), OrganizationID: orgID, ShipmentID: shipmentID,
		Status: tracking.EventDelivered, OccurredAt: occurred,
		Source: tracking.SourceManual, CreatedAt: occurred,
	}
	if err := repo.Append(ctx, other); err != nil {
		t.Fatalf("sumber berbeda seharusnya diterima: %v", err)
	}
}

func TestTrackingFindShipmentIDByAWBIsTenantScoped(t *testing.T) {
	pool, stores := orderTestDB(t)
	orgID, shipmentID, awb := seedTrackingFixtures(t, pool)
	ctx := context.Background()

	got, err := stores.Tracking().FindShipmentIDByAWB(ctx, orgID, awb)
	if err != nil {
		t.Fatalf("FindShipmentIDByAWB: %v", err)
	}
	if got != shipmentID {
		t.Errorf("id = %q, ingin %q", got, shipmentID)
	}

	// Organisasi lain tidak boleh menemukan resi ini.
	otherOrg := "01920000-0000-7000-8000-00000000ffff"
	if _, err := stores.Tracking().FindShipmentIDByAWB(ctx, otherOrg, awb); err == nil {
		t.Fatal("resi terlihat oleh organisasi lain — isolasi tenant bocor")
	}
}

func TestWebhookStoreDeduplicatesByEventID(t *testing.T) {
	// GATE Phase 4: webhook duplikat diproses sekali.
	pool, stores := orderTestDB(t)
	orgID, _, _ := seedTrackingFixtures(t, pool)
	repo := stores.Webhooks()
	ctx := context.Background()

	now := time.Now().UTC()
	mk := func(id string, eventID string) *webhook.Event {
		return &webhook.Event{
			ID: id, OrganizationID: orgID, Provider: "jne", EventID: eventID,
			EventType: "delivered", PayloadHash: "hash-" + eventID,
			SignatureValid: true, Payload: map[string]any{"awb": "X"},
			RawBody: []byte(`{"awb":"X"}`), Status: webhook.StatusReceived, CreatedAt: now,
		}
	}

	first := mk(uuid.NewString(), "evt-dup-1")
	if err := repo.Store(ctx, first); err != nil {
		t.Fatalf("Store pertama: %v", err)
	}
	// Event yang sama (event_id sama) dikirim ulang oleh provider.
	second := mk(uuid.NewString(), "evt-dup-1")
	err := repo.Store(ctx, second)
	if err == nil {
		t.Fatal("event_id duplikat harus ditolak unique index")
	}
	if !errors.Is(err, webhook.ErrDuplicate) {
		t.Errorf("error = %v, ingin webhook.ErrDuplicate", err)
	}
}

func TestWebhookStoreDeduplicatesByPayloadHashWithoutEventID(t *testing.T) {
	// Sebagian provider tidak mengirim event_id; isi payload jadi kunci kedua.
	pool, stores := orderTestDB(t)
	orgID, _, _ := seedTrackingFixtures(t, pool)
	repo := stores.Webhooks()
	ctx := context.Background()

	now := time.Now().UTC()
	body := []byte(`{"awb":"Y","status":"delivered"}`)
	mk := func() *webhook.Event {
		return &webhook.Event{
			ID: uuid.NewString(), OrganizationID: orgID, Provider: "sicepat",
			PayloadHash: "uji-" + webhook.HashPayload(body), SignatureValid: true,
			Payload: map[string]any{"awb": "Y"}, RawBody: body,
			Status: webhook.StatusReceived, CreatedAt: now,
		}
	}

	if err := repo.Store(ctx, mk()); err != nil {
		t.Fatalf("Store pertama: %v", err)
	}
	if err := repo.Store(ctx, mk()); !errors.Is(err, webhook.ErrDuplicate) {
		t.Fatalf("payload identik tanpa event_id harus ditolak duplikat, dapat %v", err)
	}
}

func TestWebhookClaimPendingSkipsInvalidSignature(t *testing.T) {
	// Event dengan signature tidak sah disimpan sebagai bukti, tetapi TIDAK
	// boleh pernah diproses.
	pool, stores := orderTestDB(t)
	orgID, _, _ := seedTrackingFixtures(t, pool)
	repo := stores.Webhooks()
	ctx := context.Background()

	now := time.Now().UTC()
	valid := &webhook.Event{
		ID: uuid.NewString(), OrganizationID: orgID, Provider: "jne",
		EventID: "evt-valid", PayloadHash: "uji-hash-valid", SignatureValid: true,
		Payload: map[string]any{"awb": "V"}, RawBody: []byte(`{"awb":"V"}`),
		Status: webhook.StatusReceived, CreatedAt: now,
	}
	invalid := &webhook.Event{
		ID: uuid.NewString(), OrganizationID: orgID, Provider: "jne",
		EventID: "evt-invalid", PayloadHash: "uji-hash-invalid", SignatureValid: false,
		SignatureError: "signature tidak cocok",
		Payload:        map[string]any{"awb": "I"}, RawBody: []byte(`{"awb":"I"}`),
		Status: webhook.StatusReceived, CreatedAt: now,
	}
	for _, e := range []*webhook.Event{valid, invalid} {
		if err := repo.Store(ctx, e); err != nil {
			t.Fatalf("Store: %v", err)
		}
	}

	pending, err := repo.ClaimPending(ctx, 10)
	if err != nil {
		t.Fatalf("ClaimPending: %v", err)
	}
	for _, e := range pending {
		if !e.SignatureValid {
			t.Errorf("event dengan signature tidak sah TIDAK boleh diklaim: %s", e.ID)
		}
		if e.ID == invalid.ID {
			t.Error("event signature-invalid ikut terklaim")
		}
	}
}

// claimUntilFound mengklaim berulang sampai baris dengan id tertentu ditemukan.
//
// Diperlukan karena Claim mengambil batch GLOBAL (bukan per organisasi),
// sehingga baris dari uji lain yang berjalan bersamaan dapat memenuhi batch.
func claimUntilFound(t *testing.T, repo *database.OutboxRepo, wantID int64) *event.Record {
	t.Helper()
	ctx := context.Background()
	for attempt := 0; attempt < 20; attempt++ {
		records, err := repo.Claim(ctx, 200)
		if err != nil {
			t.Fatalf("Claim: %v", err)
		}
		for i := range records {
			if records[i].ID == wantID {
				return &records[i]
			}
		}
		if len(records) == 0 {
			break
		}
		// Tandai terbit agar batch berikutnya tidak mengulang baris yang sama.
		for i := range records {
			_ = repo.MarkPublished(ctx, records[i].ID)
		}
	}
	return nil
}

func TestOutboxPublishAndClaimRoundTrip(t *testing.T) {
	pool, stores := orderTestDB(t)
	orgID, shipmentID, _ := seedTrackingFixtures(t, pool)
	repo := stores.Outbox()
	ctx := context.Background()

	now := time.Now().UTC()
	if err := repo.Publish(ctx, event.New(
		uuid.NewString(), event.ShipmentDelivered, shipmentID, orgID, now,
		map[string]any{"awb": "OUT-1", "status": "delivered"},
	)); err != nil {
		t.Fatalf("Publish: %v", err)
	}

	// Ambil id baris yang baru ditulis, lalu klaim sampai baris itu ditemukan.
	var wantID int64
	if err := pool.QueryRow(ctx, `
		SELECT id FROM outbox_events
		 WHERE aggregate_id=$1::uuid AND event_type='shipment.delivered'
		 ORDER BY id DESC LIMIT 1`, shipmentID).Scan(&wantID); err != nil {
		t.Fatalf("membaca id outbox: %v", err)
	}

	found := claimUntilFound(t, repo, wantID)
	if found == nil {
		t.Fatal("baris outbox yang baru ditulis tidak pernah terklaim")
	}
	if found.Type != event.ShipmentDelivered {
		t.Errorf("type = %q, ingin shipment.delivered", found.Type)
	}
	if found.RoutingKey() != string(event.ShipmentDelivered) {
		t.Errorf("routing key = %q", found.RoutingKey())
	}
	if found.Payload["awb"] != "OUT-1" {
		t.Errorf("payload tidak utuh: %+v", found.Payload)
	}
	// Header harus memuat event_id & occurred_at agar consumer bisa dedup.
	if found.Headers["event_id"] == nil {
		t.Error("header event_id wajib ada untuk dedup di consumer")
	}

	// Setelah ditandai published, tidak diklaim lagi.
	if err := repo.MarkPublished(ctx, found.ID); err != nil {
		t.Fatalf("MarkPublished: %v", err)
	}
	again, err := repo.Claim(ctx, 50)
	if err != nil {
		t.Fatalf("Claim kedua: %v", err)
	}
	for _, r := range again {
		if r.ID == found.ID {
			t.Error("baris yang sudah published tidak boleh diklaim lagi")
		}
	}
}

func TestOutboxClaimIsExclusiveAcrossWorkers(t *testing.T) {
	// Dua relay paralel tidak boleh menerbitkan event yang sama: ini yang
	// dijaga FOR UPDATE SKIP LOCKED.
	pool, stores := orderTestDB(t)
	orgID, shipmentID, _ := seedTrackingFixtures(t, pool)
	repo := stores.Outbox()
	ctx := context.Background()

	now := time.Now().UTC()
	for i := 0; i < 3; i++ {
		if err := repo.Publish(ctx, event.New(
			uuid.NewString(), event.ShipmentUpdated, shipmentID, orgID, now,
			map[string]any{"n": i},
		)); err != nil {
			t.Fatalf("Publish: %v", err)
		}
	}

	// Klaim pertama mengambil semua (batch besar); klaim kedua harus kosong
	// untuk organisasi ini karena barisnya sudah 'processing'.
	first, err := repo.Claim(ctx, 100)
	if err != nil {
		t.Fatalf("Claim pertama: %v", err)
	}
	second, err := repo.Claim(ctx, 100)
	if err != nil {
		t.Fatalf("Claim kedua: %v", err)
	}

	firstIDs := map[int64]bool{}
	for _, r := range first {
		firstIDs[r.ID] = true
	}
	for _, r := range second {
		if firstIDs[r.ID] {
			t.Errorf("baris %d diklaim dua kali — event akan diterbitkan ganda", r.ID)
		}
	}
}

func TestOutboxRecoverStaleClaims(t *testing.T) {
	// Relay yang mati meninggalkan baris 'processing'; baris itu harus bisa
	// dipulihkan agar event tidak macet selamanya.
	pool, stores := orderTestDB(t)
	orgID, shipmentID, _ := seedTrackingFixtures(t, pool)
	repo := stores.Outbox()
	ctx := context.Background()

	now := time.Now().UTC()
	if err := repo.Publish(ctx, event.New(
		uuid.NewString(), event.ShipmentUpdated, shipmentID, orgID, now,
		map[string]any{"x": 1},
	)); err != nil {
		t.Fatalf("Publish: %v", err)
	}
	claimed, err := repo.Claim(ctx, 100)
	if err != nil {
		t.Fatalf("Claim: %v", err)
	}
	if len(claimed) == 0 {
		t.Fatal("tidak ada baris terklaim")
	}

	// Pemulihan dengan ambang 0 detik: semua klaim dianggap basi.
	n, err := repo.RecoverStaleClaims(ctx, 0)
	if err != nil {
		t.Fatalf("RecoverStaleClaims: %v", err)
	}
	if n == 0 {
		t.Error("klaim basi tidak dipulihkan")
	}

	// Setelah dipulihkan, baris bisa diklaim lagi.
	reclaimed, err := repo.Claim(ctx, 100)
	if err != nil {
		t.Fatalf("Claim ulang: %v", err)
	}
	foundAgain := false
	for _, r := range reclaimed {
		if r.ID == claimed[0].ID {
			foundAgain = true
		}
	}
	if !foundAgain {
		t.Error("baris yang dipulihkan harus dapat diklaim lagi")
	}
}

func TestProcessedEventDedupIsDatabaseBacked(t *testing.T) {
	// Dedup lintas replika worker: PRIMARY KEY di database, bukan memori proses.
	pool, stores := orderTestDB(t)
	repo := stores.ProcessedEvents()
	ctx := context.Background()

	eventID := "uji-" + uuid.NewString()
	t.Cleanup(func() {
		_, _ = pool.Exec(ctx, `DELETE FROM processed_events WHERE event_id=$1`, eventID)
	})

	first, err := repo.MarkProcessed(ctx, eventID, "tracking-worker")
	if err != nil {
		t.Fatalf("MarkProcessed pertama: %v", err)
	}
	if !first {
		t.Error("pertama kali harus true")
	}
	second, err := repo.MarkProcessed(ctx, eventID, "tracking-worker")
	if err != nil {
		t.Fatalf("MarkProcessed kedua: %v", err)
	}
	if second {
		t.Error("kedua kali harus false (sudah diproses)")
	}
}
