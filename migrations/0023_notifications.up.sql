-- 0022: notification (TASK-040).
--
-- Tiga tabel:
--   * notification_templates   — template pesan per organisasi (admin+ boleh
--                                mengubah). Placeholder ditulis {{variabel}}.
--   * notification_preferences — preferensi per pengguna per channel/event
--                                (sacramento: preferensi TIDAK per organisasi,
--                                sebab notifikasi bersifat pribadi).
--   * notifications            — notifikasi yang sudah dirender & disimpan
--                                (in-app). Satu baris per pengguna.
--
-- Notifikasi email TIDAK disimpan di sini: email dikirim lewat adapter dan
-- jejaknya ada di audit/outbox, bukan di tabel in-app.

CREATE TABLE notification_templates (
    id              uuid PRIMARY KEY,
    organization_id uuid NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    key             text NOT NULL,
    channel         text NOT NULL DEFAULT 'inapp'
        CHECK (channel IN ('inapp', 'email')),
    title_template  text NOT NULL,
    body_template   text NOT NULL,
    enabled         boolean NOT NULL DEFAULT true,
    updated_by      uuid REFERENCES users (id),
    created_at      timestamptz NOT NULL DEFAULT now(),
    updated_at      timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT uq_notification_template_key UNIQUE (organization_id, key, channel)
);

CREATE INDEX notification_templates_org_idx ON notification_templates (organization_id);

COMMENT ON TABLE notification_templates IS
    'Template pesan notifikasi per organisasi; {{variabel}} diisi saat render.';

CREATE TABLE notification_preferences (
    id          uuid PRIMARY KEY,
    user_id     uuid NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    event_key   text NOT NULL,
    channel     text NOT NULL
        CHECK (channel IN ('inapp', 'email')),
    enabled     boolean NOT NULL DEFAULT true,
    created_at  timestamptz NOT NULL DEFAULT now(),
    updated_at  timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT uq_notification_preference UNIQUE (user_id, event_key, channel)
);

CREATE INDEX notification_preferences_user_idx ON notification_preferences (user_id);

COMMENT ON TABLE notification_preferences IS
    'Preferensi notifikasi per pengguna (bukan per organisasi).';

CREATE TABLE notifications (
    id              uuid PRIMARY KEY,
    organization_id uuid NOT NULL REFERENCES organizations (id) ON DELETE CASCADE,
    user_id         uuid NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    event_key       text NOT NULL,
    channel         text NOT NULL DEFAULT 'inapp'
        CHECK (channel IN ('inapp', 'email')),
    priority        text NOT NULL DEFAULT 'normal'
        CHECK (priority IN ('low', 'normal', 'high')),
    title           text NOT NULL,
    body            text NOT NULL,
    data            jsonb NOT NULL DEFAULT '{}'::jsonb,
    read_at         timestamptz,
    created_at      timestamptz NOT NULL DEFAULT now()
);

-- Daftar notifikasi milik satu pengguna: terbaru dulu, belum dibaca dulu.
CREATE INDEX notifications_user_created_idx ON notifications (user_id, created_at DESC);
CREATE INDEX notifications_user_unread_idx ON notifications (user_id, created_at DESC)
    WHERE read_at IS NULL;

COMMENT ON TABLE notifications IS
    'Notifikasi in-app yang sudah dirender; satu baris per pengguna & kejadian.';
