-- 0020_activity.up.sql
-- T21: Event, Ekstrakurikuler & Prestasi Siswa
-- Menu PRD: 133, 134, 135, 136 (BR-EVT-01, BR-GEN-09, lampiran Anggaran, Ruang, Prestasi)

-- 1. Master Jenis Event (act_event_kinds)
CREATE TABLE act_event_kinds (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    school_id  UUID NOT NULL,
    name       TEXT NOT NULL,
    deleted_at TIMESTAMPTZ NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX uq_act_event_kinds_name ON act_event_kinds(school_id, name) WHERE deleted_at IS NULL;
CREATE INDEX idx_act_event_kinds_school ON act_event_kinds(school_id);

-- 2. Events (act_events)
CREATE TABLE act_events (
    id                      UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    school_id               UUID NOT NULL,
    name                    TEXT NOT NULL,
    kind_id                 UUID NOT NULL REFERENCES act_event_kinds(id),
    description             TEXT NULL,
    starts_at               TIMESTAMPTZ NOT NULL,
    ends_at                 TIMESTAMPTZ NOT NULL,
    room_id                 UUID NULL,
    booking_id              UUID NULL,
    location_text           TEXT NULL,
    target                  JSONB NOT NULL DEFAULT '{}'::jsonb, -- {grade_levels:[], classroom_ids:[], include_staff:bool}
    quota                   INT NULL CHECK (quota IS NULL OR quota > 0),
    fee_idr                 BIGINT NOT NULL DEFAULT 0 CHECK (fee_idr >= 0),
    requires_parent_consent BOOLEAN NOT NULL DEFAULT false,
    consent_text            TEXT NULL,
    budget_line_id          UUID NULL,
    budget_amount_idr       BIGINT NULL CHECK (budget_amount_idr IS NULL OR budget_amount_idr >= 0),
    registration_opens_at   TIMESTAMPTZ NULL,
    registration_closes_at  TIMESTAMPTZ NULL,
    status                  TEXT NOT NULL DEFAULT 'draft' CHECK (status IN ('draft','pending_approval','approved','rejected','published','cancelled','completed')),
    approval_id             UUID NULL,
    published_at            TIMESTAMPTZ NULL,
    created_by              UUID NOT NULL,
    deleted_at              TIMESTAMPTZ NULL,
    idempotency_key         TEXT NULL,
    created_at              TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at              TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT uq_act_events_idempotency UNIQUE(school_id, idempotency_key)
);
CREATE INDEX idx_act_events_school_status ON act_events(school_id, status);
CREATE INDEX idx_act_events_school_dates ON act_events(school_id, starts_at, ends_at);

-- 3. Event Committee / Panitia (act_event_committee)
CREATE TABLE act_event_committee (
    event_id   UUID NOT NULL REFERENCES act_events(id) ON DELETE CASCADE,
    user_id    UUID NOT NULL,
    school_id  UUID NOT NULL,
    role_label TEXT NOT NULL DEFAULT 'panitia',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY(event_id, user_id)
);
CREATE INDEX idx_act_event_committee_user ON act_event_committee(user_id);

-- 4. Event Registrations (act_event_registrations)
CREATE TABLE act_event_registrations (
    id                    UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    school_id             UUID NOT NULL,
    event_id              UUID NOT NULL REFERENCES act_events(id) ON DELETE CASCADE,
    student_user_id       UUID NOT NULL,
    registered_by         UUID NOT NULL,
    registered_via        TEXT NOT NULL CHECK (registered_via IN ('siswa','orang_tua','event_coordinator')),
    status                TEXT NOT NULL DEFAULT 'confirmed' CHECK (status IN ('pending_consent','confirmed','declined','cancelled')),
    consent_id            UUID NULL REFERENCES parent_consents(id),
    consent_text_snapshot TEXT NULL,
    bill_id               UUID NULL,
    attended_at           TIMESTAMPTZ NULL,
    attendance_by         UUID NULL,
    created_at            TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at            TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX uq_act_event_reg_active ON act_event_registrations(event_id, student_user_id) WHERE status <> 'cancelled';
CREATE INDEX idx_act_event_reg_student ON act_event_registrations(school_id, student_user_id);
CREATE INDEX idx_act_event_reg_event ON act_event_registrations(event_id, status);

-- 5. Periode Pendaftaran Ekskul (act_ecc_periods)
CREATE TABLE act_ecc_periods (
    id               UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    school_id        UUID NOT NULL,
    academic_year_id UUID NULL,
    name             TEXT NOT NULL,
    opens_at         TIMESTAMPTZ NOT NULL,
    closes_at        TIMESTAMPTZ NOT NULL,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at       TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_act_ecc_periods_school ON act_ecc_periods(school_id);

-- 6. Ekstrakurikuler (act_extracurriculars)
CREATE TABLE act_extracurriculars (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    school_id     UUID NOT NULL,
    name          TEXT NOT NULL,
    description   TEXT NULL,
    schedule      JSONB NOT NULL DEFAULT '[]'::jsonb, -- [{weekday:1,start:"15:00",end:"17:00"}]
    room_id       UUID NULL,
    location_text TEXT NULL,
    quota         INT NULL CHECK (quota IS NULL OR quota > 0),
    fee_idr       BIGINT NOT NULL DEFAULT 0 CHECK (fee_idr >= 0),
    full_policy   TEXT NOT NULL DEFAULT 'reject' CHECK (full_policy IN ('reject','waitlist')),
    active        BOOLEAN NOT NULL DEFAULT true,
    deleted_at    TIMESTAMPTZ NULL,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX uq_act_extracurriculars_name ON act_extracurriculars(school_id, name) WHERE deleted_at IS NULL;
CREATE INDEX idx_act_extracurriculars_school ON act_extracurriculars(school_id);

-- 7. Pembina / Pelatih Ekskul (act_ecc_coaches)
CREATE TABLE act_ecc_coaches (
    ecc_id     UUID NOT NULL REFERENCES act_extracurriculars(id) ON DELETE CASCADE,
    user_id    UUID NOT NULL,
    school_id  UUID NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY(ecc_id, user_id)
);
CREATE INDEX idx_act_ecc_coaches_user ON act_ecc_coaches(user_id);

-- 8. Anggota Ekskul (act_ecc_members)
CREATE TABLE act_ecc_members (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    school_id       UUID NOT NULL,
    ecc_id          UUID NOT NULL REFERENCES act_extracurriculars(id) ON DELETE CASCADE,
    period_id       UUID NOT NULL REFERENCES act_ecc_periods(id) ON DELETE CASCADE,
    student_user_id UUID NOT NULL,
    status          TEXT NOT NULL DEFAULT 'active' CHECK (status IN ('active','waitlisted','left','rejected')),
    waitlisted_at   TIMESTAMPTZ NULL,
    joined_at       TIMESTAMPTZ NULL,
    left_at         TIMESTAMPTZ NULL,
    bill_id         UUID NULL,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT uq_act_ecc_members UNIQUE(ecc_id, period_id, student_user_id)
);
CREATE INDEX idx_act_ecc_members_student ON act_ecc_members(school_id, student_user_id);
CREATE INDEX idx_act_ecc_members_status ON act_ecc_members(ecc_id, status);

-- 9. Sesi Pertemuan Ekskul (act_ecc_sessions)
CREATE TABLE act_ecc_sessions (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    school_id  UUID NOT NULL,
    ecc_id     UUID NOT NULL REFERENCES act_extracurriculars(id) ON DELETE CASCADE,
    held_on    DATE NOT NULL,
    note       TEXT NULL,
    created_by UUID NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT uq_act_ecc_sessions_date UNIQUE(ecc_id, held_on)
);
CREATE INDEX idx_act_ecc_sessions_ecc ON act_ecc_sessions(ecc_id, held_on);

-- 10. Presensi Sesi Ekskul (act_ecc_attendance)
CREATE TABLE act_ecc_attendance (
    session_id UUID NOT NULL REFERENCES act_ecc_sessions(id) ON DELETE CASCADE,
    member_id  UUID NOT NULL REFERENCES act_ecc_members(id) ON DELETE CASCADE,
    school_id  UUID NOT NULL,
    status     TEXT NOT NULL CHECK (status IN ('hadir','izin','sakit','alpa')),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY(session_id, member_id)
);

-- 11. Penilaian Ekskul per Periode (act_ecc_assessments)
CREATE TABLE act_ecc_assessments (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    school_id   UUID NOT NULL,
    ecc_id      UUID NOT NULL REFERENCES act_extracurriculars(id) ON DELETE CASCADE,
    period_id   UUID NOT NULL REFERENCES act_ecc_periods(id) ON DELETE CASCADE,
    member_id   UUID NOT NULL REFERENCES act_ecc_members(id) ON DELETE CASCADE,
    predicate   TEXT NOT NULL,
    description TEXT NULL,
    assessed_by UUID NOT NULL,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT uq_act_ecc_assessments UNIQUE(member_id, period_id)
);
CREATE INDEX idx_act_ecc_assessments_ecc ON act_ecc_assessments(ecc_id, period_id);

-- 12. Prestasi Siswa (act_achievements)
CREATE TABLE act_achievements (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    school_id       UUID NOT NULL,
    student_user_id UUID NOT NULL,
    activity_name   TEXT NOT NULL,
    organizer       TEXT NULL,
    level           TEXT NOT NULL CHECK (level IN ('sekolah','kecamatan','kabupaten_kota','provinsi','nasional','internasional')),
    rank_label      TEXT NOT NULL,
    achieved_on     DATE NOT NULL,
    evidence        JSONB NOT NULL DEFAULT '[]'::jsonb,
    event_id        UUID NULL REFERENCES act_events(id) ON DELETE SET NULL,
    ecc_id          UUID NULL REFERENCES act_extracurriculars(id) ON DELETE SET NULL,
    submitted_by    UUID NOT NULL,
    submitted_via   TEXT NOT NULL CHECK (submitted_via IN ('orang_tua','siswa','pembina_ekskul','kesiswaan')),
    status          TEXT NOT NULL DEFAULT 'pending' CHECK (status IN ('pending','verified','rejected')),
    verified_by     UUID NULL,
    verified_at     TIMESTAMPTZ NULL,
    reject_reason   TEXT NULL,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    deleted_at      TIMESTAMPTZ NULL,
    CONSTRAINT chk_act_achievement_maker_checker CHECK (verified_by IS NULL OR verified_by <> submitted_by)
);
CREATE INDEX idx_act_achievements_school_status ON act_achievements(school_id, status);
CREATE INDEX idx_act_achievements_student ON act_achievements(school_id, student_user_id);
CREATE INDEX idx_act_achievements_date ON act_achievements(school_id, achieved_on);
