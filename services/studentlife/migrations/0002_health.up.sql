-- 0002_health.up.sql
-- Menu 127 (profil kesehatan) + 128 (kunjungan UKS). Kolom *_enc = AES-256-GCM
-- dengan AAD "<tabel>:<school_id>:<id baris>" (ciphertext tidak bisa dipindah
-- ke baris lain). Tidak ada seed data kesehatan; obat UKS diisi dari UI.

CREATE TABLE health_profiles (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    school_id     UUID NOT NULL,
    student_id    UUID NOT NULL,
    profile_enc   BYTEA NOT NULL,
    alert_enc     BYTEA NULL,
    emergency_enc BYTEA NULL,
    verified_by   UUID NULL,
    verified_at   TIMESTAMPTZ NULL,
    version       INT NOT NULL DEFAULT 1,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (school_id, student_id)
);

CREATE TABLE health_profile_changes (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    school_id       UUID NOT NULL,
    student_id      UUID NOT NULL,
    profile_id      UUID NULL REFERENCES health_profiles(id),
    submitted_by    UUID NOT NULL,
    proposed_enc    BYTEA NOT NULL,
    base_version    INT NOT NULL,
    status          TEXT NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'approved', 'rejected', 'superseded')),
    approval_id     UUID NULL,
    decided_at      TIMESTAMPTZ NULL,
    idempotency_key TEXT NOT NULL,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (school_id, idempotency_key)
);
CREATE INDEX idx_health_profile_changes_status ON health_profile_changes(school_id, status, created_at DESC);

CREATE TABLE uks_medicines (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    school_id   UUID NOT NULL,
    name        TEXT NOT NULL,
    unit        TEXT NOT NULL,
    ops_item_id UUID NULL,
    active      BOOLEAN NOT NULL DEFAULT true,
    deleted_at  TIMESTAMPTZ NULL,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX uq_uks_medicines_name ON uks_medicines(school_id, lower(name)) WHERE deleted_at IS NULL;

CREATE TABLE clinic_visits (
    id                 UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    school_id          UUID NOT NULL,
    student_id         UUID NOT NULL,
    visited_at         TIMESTAMPTZ NOT NULL,
    left_at            TIMESTAMPTZ NULL,
    details_enc        BYTEA NOT NULL,
    outcome            TEXT NOT NULL CHECK (outcome IN ('back_to_class', 'sent_home', 'referred')),
    referred_to_enc    BYTEA NULL,
    handled_by         UUID NOT NULL,
    attendance_sync    TEXT NOT NULL CHECK (attendance_sync IN ('not_needed', 'pending', 'done', 'skipped', 'failed')),
    attendance_attempts INT NOT NULL DEFAULT 0,
    attendance_error   TEXT NULL,
    parent_notified_at TIMESTAMPTZ NULL,
    idempotency_key    TEXT NOT NULL,
    created_at         TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at         TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (school_id, idempotency_key)
);
CREATE INDEX idx_clinic_visits_school ON clinic_visits(school_id, visited_at DESC);
CREATE INDEX idx_clinic_visits_student ON clinic_visits(school_id, student_id, visited_at DESC);
CREATE INDEX idx_clinic_visits_sync ON clinic_visits(attendance_sync) WHERE attendance_sync = 'pending';

CREATE TABLE clinic_visit_addenda (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    school_id  UUID NOT NULL,
    visit_id   UUID NOT NULL REFERENCES clinic_visits(id),
    note_enc   BYTEA NOT NULL,
    created_by UUID NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_clinic_visit_addenda_visit ON clinic_visit_addenda(visit_id, created_at);

CREATE TABLE clinic_visit_medications (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    school_id   UUID NOT NULL,
    visit_id    UUID NOT NULL REFERENCES clinic_visits(id),
    medicine_id UUID NOT NULL REFERENCES uks_medicines(id),
    qty         NUMERIC(10,2) NOT NULL CHECK (qty > 0),
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_clinic_visit_medications_visit ON clinic_visit_medications(visit_id);

-- FR-128-003: pengeluaran stok obat dikirim job ke inventory (T15).
CREATE TABLE inventory_issue_outbox (
    id                  UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    school_id           UUID NOT NULL,
    visit_medication_id UUID NOT NULL UNIQUE REFERENCES clinic_visit_medications(id),
    ops_item_id         UUID NULL,
    qty                 NUMERIC(10,2) NOT NULL,
    status              TEXT NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'sent', 'skipped', 'failed')),
    attempts            INT NOT NULL DEFAULT 0,
    last_error          TEXT NULL,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_inventory_issue_outbox_pending ON inventory_issue_outbox(status) WHERE status = 'pending';
