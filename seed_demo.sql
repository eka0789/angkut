-- Seed data demo realistis untuk organisasi eka0789@gmail.com (demo lokal).
-- Idempoten: insert di-guard keberadaan penanda (AWB/SKU/nomor/recepient).

DO $$
DECLARE
  v_org uuid;
  v_user uuid;
  v_courier_id uuid;
  v_svc_id uuid;
  v_ccode text;
  v_scode text;
  v_addr_origin uuid;
  v_wh1 uuid;
  v_wh2 uuid;
  v_store uuid;
  v_cust uuid;
  v_prod uuid;
  v_variant uuid;
  v_order uuid;
  v_ship uuid;
  v_txn uuid;
  v_i int;
  v_j int;
  v_city text;
  v_created timestamptz;
  v_delivered timestamptz;
  v_awb text;
  v_status text;
  v_ship_price bigint;
  v_cost bigint;
  v_cod bool;
  v_cod_amount bigint;
  v_cod_fee bigint;
  v_total bigint;
  v_qty int;
  v_pname text;
  v_price bigint;
  v_cust_names text[] := ARRAY['Budi Santoso','Siti Rahayu','Ahmad Fauzi','Dewi Lestari','Rizky Pratama',
    'Nur Aisyah','Hendra Wijaya','Maya Sari','Agus Setiawan','Intan Permata','Fajar Nugroho','Lina Marlina'];
  v_phones text[] := ARRAY['081234500001','081234500002','081234500003','081234500004','081234500005',
    '081234500006','081234500007','081234500008','081234500009','081234500010','081234500011','081234500012'];
  v_segments text[] := ARRAY['vip','regular','new','regular','vip','new','regular','vip','new','regular','regular','new'];
  v_city_names text[] := ARRAY['SEMARANG','JAKARTA PUSAT','BANDUNG','SURABAYA','YOGYAKARTA','TANGERANG SELATAN'];
  v_couriers text[] := ARRAY['jne','jnt','sicepat'];
  v_statuses text[] := ARRAY['delivered','delivered','delivered','delivered','delivered','delivered','delivered',
    'in_transit','in_transit','in_transit','out_for_delivery','out_for_delivery','pending_pickup',
    'delivered','delivered','delivered','delivered','delivered','delivered','delivered',
    'in_transit','in_transit','out_for_delivery','pending_pickup','failed','returned'];
  v_prod_names text[] := ARRAY['Kaos Polos Cotton','Hoodie Fleece','Tote Bag Canvas','Topi Trucker'];
  v_prod_skus text[] := ARRAY['KAOS','HOOD','TOTE','TOPI'];
  v_prod_prices bigint[] := ARRAY[85000, 185000, 65000, 55000];
BEGIN
  SELECT o.id, o.owner_user_id INTO v_org, v_user
    FROM organizations o JOIN users u ON u.id = o.owner_user_id
   WHERE u.email = 'eka0789@gmail.com' LIMIT 1;
  IF v_org IS NULL THEN
    RAISE EXCEPTION 'org demo tidak ditemukan';
  END IF;

  -- ── Alamat asal (Semarang) + 6 alamat tujuan (kota berbeda) ──
  IF NOT EXISTS (SELECT 1 FROM addresses WHERE organization_id = v_org AND recipient = 'Gudang EKA Logistics') THEN
    INSERT INTO addresses (id, organization_id, recipient, phone, province_id, city_id, district_id,
        street, status, created_by, created_at, updated_at)
    SELECT gen_random_uuid(), v_org, 'Gudang EKA Logistics', '0812000999', p.id, c.id, d.id,
        'Jl. Industri Raya No. 8, Kawasan Industri', 'active', v_user, now(), now()
      FROM postal_codes pc
      JOIN districts d ON d.id = pc.district_id
      JOIN cities c ON c.id = d.city_id
      JOIN provinces p ON p.id = c.province_id
     WHERE c.name = 'SEMARANG' LIMIT 1;
  END IF;
  SELECT id INTO v_addr_origin FROM addresses
   WHERE organization_id = v_org AND recipient = 'Gudang EKA Logistics' LIMIT 1;

  v_i := 0;
  FOREACH v_city IN ARRAY v_city_names LOOP
    IF NOT EXISTS (SELECT 1 FROM addresses WHERE organization_id = v_org AND recipient LIKE '%(Penerima)' AND street LIKE '% ' || v_city || ' %') THEN
      INSERT INTO addresses (id, organization_id, recipient, phone, province_id, city_id, district_id,
          street, status, created_by, created_at, updated_at)
      SELECT gen_random_uuid(), v_org, 'Penerima ' || initcap(v_city), '0812999000' || v_i, p.id, c.id, d.id,
          'Jl. ' || initcap(v_city) || ' Raya No. ' || v_i || ' — kota ' || v_city, 'active', v_user, now(), now()
        FROM postal_codes pc
        JOIN districts d ON d.id = pc.district_id
        JOIN cities c ON c.id = d.city_id
        JOIN provinces p ON p.id = c.province_id
       WHERE c.name = v_city LIMIT 1;
    END IF;
    v_i := v_i + 1;
  END LOOP;

  -- ── Gudang & toko ──
  IF NOT EXISTS (SELECT 1 FROM warehouses WHERE organization_id = v_org AND name = 'Gudang Pusat Semarang') THEN
    INSERT INTO warehouses (id, organization_id, name, address_id, is_default, is_active)
    VALUES (gen_random_uuid(), v_org, 'Gudang Pusat Semarang', v_addr_origin, true, true);
  END IF;
  SELECT id INTO v_wh1 FROM warehouses WHERE organization_id = v_org AND name = 'Gudang Pusat Semarang';

  IF NOT EXISTS (SELECT 1 FROM warehouses WHERE organization_id = v_org AND name = 'Gudang Cabang Jakarta') THEN
    INSERT INTO warehouses (id, organization_id, name, address_id, is_default, is_active)
    SELECT gen_random_uuid(), v_org, 'Gudang Cabang Jakarta', a.id, false, true
      FROM addresses a
     WHERE a.organization_id = v_org AND a.street LIKE '% JAKARTA PUSAT %' LIMIT 1;
  END IF;
  SELECT id INTO v_wh2 FROM warehouses WHERE organization_id = v_org AND name = 'Gudang Cabang Jakarta';

  IF NOT EXISTS (SELECT 1 FROM stores WHERE organization_id = v_org AND name = 'Toko Online EKA') THEN
    INSERT INTO stores (id, organization_id, name, channel, adapter_type, warehouse_id, created_at, updated_at)
    VALUES (gen_random_uuid(), v_org, 'Toko Online EKA', 'manual', 'manual', v_wh1, now(), now());
  END IF;
  SELECT id INTO v_store FROM stores WHERE organization_id = v_org AND name = 'Toko Online EKA';

  -- ── Pelanggan ──
  v_i := 1;
  WHILE v_i <= 12 LOOP
    IF NOT EXISTS (SELECT 1 FROM customers WHERE organization_id = v_org AND phone = v_phones[v_i]) THEN
      INSERT INTO customers (id, organization_id, name, phone, segment, segment_source, metrics, created_at, updated_at)
      VALUES (gen_random_uuid(), v_org, v_cust_names[v_i], v_phones[v_i], v_segments[v_i], 'auto',
        jsonb_build_object('orders_count', 0, 'total_spend', 0), now(), now());
    END IF;
    v_i := v_i + 1;
  END LOOP;

  -- ── Produk + varian ──
  v_i := 1;
  WHILE v_i <= 4 LOOP
    IF NOT EXISTS (SELECT 1 FROM products WHERE organization_id = v_org AND name = v_prod_names[v_i]) THEN
      INSERT INTO products (id, organization_id, name, category, weight_kg, is_active, created_by, created_at, updated_at)
      VALUES (gen_random_uuid(), v_org, v_prod_names[v_i], 'fashion', 0.3 * v_i, true, v_user, now(), now());
      SELECT id INTO v_prod FROM products WHERE organization_id = v_org AND name = v_prod_names[v_i];
      v_j := 1;
      WHILE v_j <= 2 LOOP
        INSERT INTO product_variants (id, organization_id, product_id, sku, name, low_stock_threshold, is_active, created_at, updated_at)
        VALUES (gen_random_uuid(), v_org, v_prod,
          v_prod_skus[v_i] || '-' || (ARRAY['M','L','XL'])[v_j], (ARRAY['Ukuran M','Ukuran L','Ukuran XL'])[v_j],
          10, true, now(), now());
        v_j := v_j + 1;
      END LOOP;
    END IF;
    v_i := v_i + 1;
  END LOOP;

  -- ── Stok dua gudang (beberapa low-stock utk banner) ──
  FOR v_variant IN SELECT id FROM product_variants WHERE organization_id = v_org ORDER BY sku LOOP
    v_j := v_j + 1;
    IF NOT EXISTS (SELECT 1 FROM inventory WHERE organization_id = v_org AND variant_id = v_variant AND warehouse_id = v_wh1) THEN
      INSERT INTO inventory (id, organization_id, variant_id, warehouse_id, qty_on_hand, qty_reserved, version)
      VALUES (gen_random_uuid(), v_org, v_variant, v_wh1,
        (ARRAY[120,45,30,8,60,2,0,25])[((v_j-1) % 8) + 1], 0, 1);
    END IF;
    IF NOT EXISTS (SELECT 1 FROM inventory_movements WHERE organization_id = v_org AND variant_id = v_variant AND warehouse_id = v_wh1) THEN
      INSERT INTO inventory_movements (id, organization_id, variant_id, warehouse_id, type, qty_delta, qty_after, reason, created_at)
      SELECT gen_random_uuid(), v_org, i.variant_id, i.warehouse_id, 'adjust', i.qty_on_hand, i.qty_on_hand,
        'stok awal demo', now()
        FROM inventory i
       WHERE i.organization_id = v_org AND i.variant_id = v_variant AND i.warehouse_id = v_wh1;
    END IF;
    IF v_j <= 2 AND NOT EXISTS (SELECT 1 FROM inventory WHERE organization_id = v_org AND variant_id = v_variant AND warehouse_id = v_wh2) THEN
      INSERT INTO inventory (id, organization_id, variant_id, warehouse_id, qty_on_hand, qty_reserved, version)
      VALUES (gen_random_uuid(), v_org, v_variant, v_wh2, (ARRAY[20,5])[v_j], 0, 1);
    END IF;
  END LOOP;
  v_j := 0;

  -- ── 26 order + shipment + tracking + COD + ledger ──
  v_i := 1;
  WHILE v_i <= 26 LOOP
    v_status := v_statuses[v_i];
    IF v_status = 'delivered' THEN
      v_created := now() - make_interval(days => (32 - v_i * 2));
      v_delivered := v_created + make_interval(days => (1 + (v_i % 3)), hours => 6);
    ELSIF v_status = 'in_transit' THEN
      v_created := now() - make_interval(days => (v_i - 13));
      v_delivered := NULL;
    ELSIF v_status = 'out_for_delivery' THEN
      v_created := now() - make_interval(days => (v_i - 18));
      v_delivered := NULL;
    ELSIF v_status = 'pending_pickup' THEN
      v_created := now() - make_interval(hours => (v_i - 21) * 10);
      v_delivered := NULL;
    ELSIF v_status = 'failed' THEN
      v_created := now() - make_interval(days => 2);
      v_delivered := NULL;
    ELSE
      v_created := now() - make_interval(days => 6);
      v_delivered := NULL;
    END IF;

    SELECT id INTO v_cust FROM customers WHERE organization_id = v_org ORDER BY created_at OFFSET (v_i % 12) LIMIT 1;
    SELECT id INTO v_order FROM orders WHERE organization_id = v_org AND notes = 'demo-' || v_i;

    IF v_order IS NULL THEN
      INSERT INTO orders (id, organization_id, store_id, customer_id, order_number, status, payment_type,
          subtotal, shipping_cost, discount, total_amount, shipping_address_id, notes, created_at, updated_at)
      VALUES (gen_random_uuid(), v_org, v_store, v_cust,
          'ORD-2026-' || lpad((100000 + v_i)::text, 6, '0'),
          CASE WHEN v_status = 'delivered' THEN 'delivered'
               WHEN v_status IN ('in_transit','out_for_delivery') THEN 'shipped'
               WHEN v_status = 'returned' THEN 'returned'
               WHEN v_status = 'failed' THEN 'shipped'
               ELSE 'confirmed' END,
          CASE WHEN v_i % 3 = 1 THEN 'cod' ELSE 'prepaid' END,
          0, 0, 0, 0,
          (SELECT id FROM addresses WHERE organization_id = v_org AND recipient LIKE '%(Penerima)' ORDER BY created_at OFFSET (v_i % 6) LIMIT 1),
          'demo-' || v_i, v_created, v_created)
      RETURNING id INTO v_order;

      v_j := 0;
      FOR v_variant IN SELECT id FROM product_variants WHERE organization_id = v_org ORDER BY sku OFFSET (v_i % 6) LIMIT 2
      LOOP
        v_j := v_j + 1;
        SELECT p.name, v_prod_prices[((v_j + v_i) % 4) + 1] INTO v_pname, v_price
          FROM product_variants pv JOIN products p ON p.id = pv.product_id
         WHERE pv.id = v_variant;
        v_qty := 1 + (v_i % 2);
        INSERT INTO order_items (id, organization_id, order_id, variant_id, product_name_snapshot,
            variant_name_snapshot, qty, unit_price, total_price, created_at)
        VALUES (gen_random_uuid(), v_org, v_order, v_variant, v_pname, 'Reguler', v_qty, v_price, v_price * v_qty, v_created);
      END LOOP;
      SELECT COALESCE(sum(total_price),0) INTO v_total FROM order_items WHERE order_id = v_order;
    ELSE
      SELECT COALESCE(sum(total_price),0) INTO v_total FROM order_items WHERE order_id = v_order;
    END IF;

    v_ccode := v_couriers[(v_i % 3) + 1];
    SELECT id, code INTO v_courier_id, v_ccode FROM couriers WHERE code = v_ccode;
    SELECT id, code INTO v_svc_id, v_scode FROM courier_services
     WHERE courier_id = v_courier_id ORDER BY code LIMIT 1;

    v_ship_price := 20000 + (v_i * 2500) % 70000;
    v_cost := v_ship_price / 2;
    v_cod := (v_i % 3 = 1);
    v_cod_amount := CASE WHEN v_cod THEN 150000 + (v_i * 50000) % 700000 ELSE 0 END;
    v_cod_fee := CASE WHEN v_cod THEN 2500 + (v_cod_amount % 4000) ELSE 0 END;
    v_awb := upper(v_ccode) || '26' || to_char(v_created, 'MMDD') || lpad((900000 + v_i)::text, 6, '0');

    IF NOT EXISTS (SELECT 1 FROM shipments WHERE organization_id = v_org AND awb = v_awb) THEN
      INSERT INTO shipments (id, organization_id, order_id, store_id, warehouse_id,
          courier_id, courier_service_id, courier_code, service_code, awb, status,
          origin_address_id, destination_address_id, actual_weight, volumetric_weight, chargeable_weight,
          length, width, height, package_type, declared_value, insurance_amount,
          shipping_price, cost_price, is_cod, cod_amount, cod_fee, eta_min_days, eta_max_days,
          booked_at, delivered_at, version, created_at, updated_at)
      VALUES (gen_random_uuid(), v_org, v_order, v_store, v_wh1,
          v_courier_id, v_svc_id, v_ccode, v_scode, v_awb, v_status,
          v_addr_origin, (SELECT shipping_address_id FROM orders WHERE id = v_order),
          0.4 + (v_i % 3) * 0.5, 0, 0.4 + (v_i % 3) * 0.5,
          25, 20, 8, 'box', CASE WHEN v_cod THEN v_cod_amount ELSE 100000 END, 0,
          v_ship_price, v_cost, v_cod, v_cod_amount, v_cod_fee, 1, 3,
          v_created, v_delivered, 1, v_created, v_created);
      SELECT id INTO v_ship FROM shipments WHERE organization_id = v_org AND awb = v_awb;

      UPDATE orders SET shipping_cost = v_ship_price,
          total_amount = (SELECT COALESCE(sum(total_price),0) FROM order_items WHERE order_id = v_order) + v_ship_price
        WHERE id = v_order;

      INSERT INTO shipment_items (id, organization_id, shipment_id, order_item_id, qty, name_snapshot, created_at)
      SELECT gen_random_uuid(), v_org, v_ship, oi.id, oi.qty, oi.product_name_snapshot, v_created
        FROM order_items oi WHERE oi.order_id = v_order;

      INSERT INTO shipment_packages (id, organization_id, shipment_id, sequence, weight, length, width, height,
          awb, qr_payload, barcode_value, created_at)
      VALUES (gen_random_uuid(), v_org, v_ship, 1, 0.4 + (v_i % 3) * 0.5, 25, 20, 8,
          v_awb, 'ANGKUT|' || v_ship || '|' || v_awb, upper(v_awb), v_created);

      INSERT INTO tracking_events (id, organization_id, shipment_id, status, description, location, occurred_at, source, created_at)
      VALUES (gen_random_uuid(), v_org, v_ship, 'created', 'Pengiriman dibuat', 'Semarang', v_created, 'system', v_created);
      IF v_status <> 'pending_pickup' THEN
        INSERT INTO tracking_events (id, organization_id, shipment_id, status, description, location, occurred_at, source, created_at)
        VALUES (gen_random_uuid(), v_org, v_ship, 'picked_up', 'Paket dijemput kurir', 'Semarang', v_created + interval '5 hours', 'webhook', v_created + interval '5 hours');
      END IF;
      IF v_status IN ('in_transit','out_for_delivery','delivered','failed','returned') THEN
        INSERT INTO tracking_events (id, organization_id, shipment_id, status, description, location, occurred_at, source, created_at)
        VALUES (gen_random_uuid(), v_org, v_ship, 'in_transit', 'Paket dalam perjalanan', v_city_names[(v_i % 6) + 1], v_created + interval '1 day', 'webhook', v_created + interval '1 day');
      END IF;
      IF v_status IN ('out_for_delivery','delivered') THEN
        INSERT INTO tracking_events (id, organization_id, shipment_id, status, description, location, occurred_at, source, created_at)
        VALUES (gen_random_uuid(), v_org, v_ship, 'out_for_delivery', 'Sedang diantar ke penerima', v_city_names[(v_i % 6) + 1], v_created + interval '2 days', 'webhook', v_created + interval '2 days');
      END IF;
      IF v_status = 'delivered' THEN
        INSERT INTO tracking_events (id, organization_id, shipment_id, status, description, location, occurred_at, source, created_at)
        VALUES (gen_random_uuid(), v_org, v_ship, 'delivered', 'Paket diterima', v_city_names[(v_i % 6) + 1], v_delivered, 'webhook', v_delivered);
      END IF;
      IF v_status = 'failed' THEN
        INSERT INTO tracking_events (id, organization_id, shipment_id, status, description, location, occurred_at, source, created_at)
        VALUES (gen_random_uuid(), v_org, v_ship, 'failed', 'Penerima tidak dapat dihubungi', v_city_names[(v_i % 6) + 1], v_created + interval '2 days', 'webhook', v_created + interval '2 days');
      END IF;
      IF v_status = 'returned' THEN
        INSERT INTO tracking_events (id, organization_id, shipment_id, status, description, location, occurred_at, source, created_at)
        VALUES (gen_random_uuid(), v_org, v_ship, 'returned', 'Paket dikembalikan ke pengirim', 'Semarang', v_created + interval '4 days', 'webhook', v_created + interval '4 days');
      END IF;

      IF v_cod THEN
        v_txn := gen_random_uuid();
        INSERT INTO cod_transactions (id, organization_id, shipment_id, courier_code, amount, fee, status, collected_at, created_at, updated_at)
        VALUES (v_txn, v_org, v_ship, v_ccode, v_cod_amount, v_cod_fee,
          CASE WHEN v_status = 'delivered' THEN 'collected' ELSE 'pending' END,
          CASE WHEN v_status = 'delivered' THEN v_delivered ELSE NULL END, v_created, v_created);
        IF v_status = 'delivered' THEN
          INSERT INTO ledger_entries (id, organization_id, entry_date, account, debit, credit, ref_type, ref_id, memo, created_by, created_at)
          VALUES (gen_random_uuid(), v_org, v_delivered::date, 'cod_receivable', v_cod_amount - v_cod_fee, 0, 'cod', v_ship, 'Piutang COD ' || v_awb, v_user, v_delivered);
          INSERT INTO ledger_entries (id, organization_id, entry_date, account, debit, credit, ref_type, ref_id, memo, created_by, created_at)
          VALUES (gen_random_uuid(), v_org, v_delivered::date, 'cod_fee', v_cod_fee, 0, 'cod', v_ship, 'Fee COD ' || v_awb, v_user, v_delivered);
          INSERT INTO ledger_entries (id, organization_id, entry_date, account, debit, credit, ref_type, ref_id, memo, created_by, created_at)
          VALUES (gen_random_uuid(), v_org, v_delivered::date, 'cod_collected', 0, v_cod_amount, 'cod', v_ship, 'Pendapatan COD ' || v_awb, v_user, v_delivered);
        END IF;
      END IF;

      IF v_status = 'delivered' THEN
        INSERT INTO ledger_entries (id, organization_id, entry_date, account, debit, credit, ref_type, ref_id, memo, created_by, created_at)
        VALUES (gen_random_uuid(), v_org, v_delivered::date, 'shipping_expense', v_cost, 0, 'shipment', v_ship, 'Ongkos kirim ' || v_awb, v_user, v_delivered);
      END IF;

      IF v_i % 3 <> 1 AND v_i <= 9 THEN
        INSERT INTO invoices (id, organization_id, number, order_id, amount, status, created_at, updated_at)
        VALUES (gen_random_uuid(), v_org, 'INV-2026-' || lpad((200000 + v_i)::text, 6, '0'), v_order, v_total + v_ship_price,
          CASE WHEN v_i % 2 = 0 THEN 'paid' ELSE 'issued' END, v_created, v_created)
        ON CONFLICT (organization_id, number) DO NOTHING;
      END IF;
    END IF;
    v_i := v_i + 1;
  END LOOP;

  -- ── Retur + kebijakan ──
  IF NOT EXISTS (SELECT 1 FROM return_policies WHERE organization_id = v_org) THEN
    INSERT INTO return_policies (id, organization_id, window_days, auto_approve_limit, conditions, updated_by, created_at, updated_at)
    VALUES (gen_random_uuid(), v_org, 7, 500000, '["Kemasan utuh","Bukan barang terlarang"]'::jsonb, v_user, now(), now());
  END IF;

  IF NOT EXISTS (SELECT 1 FROM returns WHERE organization_id = v_org) THEN
    DECLARE
      v_ret uuid;
      v_sid uuid;
    BEGIN
      SELECT s.id INTO v_sid FROM shipments s
       WHERE s.organization_id = v_org AND s.status = 'delivered' ORDER BY s.delivered_at DESC LIMIT 1;
      v_ret := gen_random_uuid();
      INSERT INTO returns (id, organization_id, shipment_id, type, reason, status, refund_amount, created_by, created_at, updated_at)
      VALUES (v_ret, v_org, v_sid, 'refund', 'Ukuran tidak sesuai', 'requested', 85000, v_user, now() - interval '2 days', now() - interval '2 days');
      INSERT INTO return_items (id, return_id, qty, condition, restocked)
      VALUES (gen_random_uuid(), v_ret, 1, 'unknown', false);

      SELECT s.id INTO v_sid FROM shipments s
       WHERE s.organization_id = v_org AND s.status = 'delivered' ORDER BY s.delivered_at DESC OFFSET 1 LIMIT 1;
      v_ret := gen_random_uuid();
      INSERT INTO returns (id, organization_id, shipment_id, type, reason, status, refund_amount, refund_method,
          decided_by, decided_at, created_by, created_at, updated_at)
      VALUES (v_ret, v_org, v_sid, 'refund', 'Barang rusak saat pengiriman', 'inspected', 120000, 'transfer',
          v_user, now() - interval '1 day', v_user, now() - interval '5 days', now() - interval '1 day');
      INSERT INTO return_items (id, return_id, qty, condition, restocked)
      VALUES (gen_random_uuid(), v_ret, 1, 'damaged', false);
      INSERT INTO ledger_entries (id, organization_id, entry_date, account, debit, credit, ref_type, ref_id, memo, created_by, created_at)
      VALUES (gen_random_uuid(), v_org, (now() - interval '1 day')::date, 'return_refund', 120000, 0, 'return', v_ret, 'Refund retur', v_user, now() - interval '1 day');
    END;
  END IF;

  -- ── Notifikasi ──
  IF NOT EXISTS (SELECT 1 FROM notifications WHERE organization_id = v_org) THEN
    INSERT INTO notifications (id, organization_id, user_id, event_key, channel, priority, title, body, created_at)
    VALUES (gen_random_uuid(), v_org, v_user, 'shipment.delivered', 'inapp', 'normal',
      'Pengiriman selesai', 'Salah satu paket telah diterima penerima.', now() - interval '3 hours');
    INSERT INTO notifications (id, organization_id, user_id, event_key, channel, priority, title, body, created_at)
    VALUES (gen_random_uuid(), v_org, v_user, 'shipment.failed', 'inapp', 'high',
      'Pengiriman gagal', 'Penerima tidak dapat dihubungi — jadwalkan ulang.', now() - interval '1 day');
    INSERT INTO notifications (id, organization_id, user_id, event_key, channel, priority, title, body, created_at)
    VALUES (gen_random_uuid(), v_org, v_user, 'bulk.job_completed', 'inapp', 'normal',
      'Pengiriman massal selesai', 'Job pengiriman massal selesai diproses.', now() - interval '2 days');
  END IF;

  RAISE NOTICE 'SEED DEMO SELESAI untuk org %', v_org;
END $$;
