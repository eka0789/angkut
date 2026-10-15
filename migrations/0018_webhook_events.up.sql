-- 0018 — Webhook ingress: event mentah dari kurir/store/payment (TASK-031).
--
-- Penyimpanan RAW lebih dulu, pemrosesan kemudian. Alasannya: begitu payload
-- masuk, kita tidak boleh bergantung pada bentuknya yang "benar" — kalau
-- parsing gagal karena kurir mengubah format, payload asli tetap tersimpan
-- dan bisa diproses ulang setelah parser diperbaiki.
--
-- Idempotensi: UNIQUE (provider, event_id) — kurir nyata mengirim ulang event
-- yang sama berkali-kali (retry karena timeout di sisi mereka).
-- Bila provider tidak memberi event_id, kolom `payload_hash` jadi kunci kedua.

CREATE TABLE webhook_events (
    id              uuid PRIMARY KEY,
    organization_id uuid REFERENCES organizations (id) ON DELETE SET NULL,

    provider        text NOT NULL,          -- jne | sicepat | shopify | midtrans | ...
    event_id        text,                   -- id dari provider (bila ada)
    event_type      text,                   -- mis. shipment.status_updated

    -- Hash payload mentah; dipakai sebagai kunci dedup ketika provider tidak
    -- mengirim event_id. Bukan untuk keamanan, murni identitas isi.
    payload_hash    text NOT NULL,

    signature_valid boolean NOT NULL DEFAULT false,
    signature_error text,

    raw_body        bytea NOT NULL,         -- byte asli; signature dihitung atas ini
    payload         jsonb NOT NULL DEFAULT '{}'::jsonb,
    content_type    text,
    headers         jsonb NOT NULL DEFAULT '{}'::jsonb,

    source_ip       inet,
    awb             text,                   -- hasil ekstraksi cepat untuk pencarian

    status          text NOT NULL DEFAULT 'received'
                    CHECK (status IN ('received', 'processed', 'failed', 'ignored')),
    attempts        int NOT NULL DEFAULT 0,
    last_error      text,
    processed_at    timestamptz,

    created_at      timestamptz NOT NULL DEFAULT now()
);

-- Dedup per provider: satu event_id hanya boleh sekali.
--
-- PENTING — index bersifat PARSIAL pada signature_valid: hanya event yang
-- signature-nya SAH yang boleh menempati slot dedup. Tanpa syarat itu, siapa
-- pun yang mengirim payload dengan signature salah lebih dulu akan "memakan"
-- event_id tersebut, sehingga kiriman ASLI dari provider ditolak sebagai
-- duplikat. Itu jalur denial-of-service yang murah.
CREATE UNIQUE INDEX uq_webhook_provider_event
    ON webhook_events (provider, event_id)
    WHERE event_id IS NOT NULL AND signature_valid;

-- Dedup isi bila provider tidak mengirim event_id (juga hanya untuk yang sah).
CREATE UNIQUE INDEX uq_webhook_provider_hash
    ON webhook_events (provider, payload_hash)
    WHERE event_id IS NULL AND signature_valid;

CREATE INDEX idx_webhook_events_status ON webhook_events (status, created_at);
CREATE INDEX idx_webhook_events_awb ON webhook_events (awb) WHERE awb IS NOT NULL;
CREATE INDEX idx_webhook_events_provider_time ON webhook_events (provider, created_at DESC);
-- Signature tidak sah adalah sinyal keamanan: harus mudah ditelusuri.
CREATE INDEX idx_webhook_events_invalid_sig
    ON webhook_events (provider, created_at DESC)
    WHERE NOT signature_valid;

COMMENT ON TABLE webhook_events IS
    'Event masuk dari pihak ketiga. Raw body SELALU disimpan sebelum parsing (traceability + replay).';

-- Rahasia signature per provider. Disimpan terenkripsi (cryptox AES-GCM);
-- nilai mentah tidak pernah dibaca ke domain — hanya dibuka adapter verifier.
CREATE TABLE webhook_secrets (
    id              uuid PRIMARY KEY,
    provider        text NOT NULL UNIQUE,
    secret_enc      bytea NOT NULL,
    -- Header tempat signature dikirim; berbeda antar provider.
    signature_header text NOT NULL DEFAULT 'X-Signature',
    algo            text NOT NULL DEFAULT 'hmac-sha256'
                    CHECK (algo IN ('hmac-sha256', 'hmac-sha1')),
    is_active       boolean NOT NULL DEFAULT true,
    created_at      timestamptz NOT NULL DEFAULT now(),
    updated_at      timestamptz NOT NULL DEFAULT now()
);

CREATE TRIGGER webhook_secrets_set_updated_at
    BEFORE UPDATE ON webhook_secrets
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

COMMENT ON TABLE webhook_secrets IS
    'Rahasia verifikasi signature per provider; tersimpan terenkripsi, tidak pernah di-log.';