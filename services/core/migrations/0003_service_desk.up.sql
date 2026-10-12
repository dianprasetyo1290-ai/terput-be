-- Migration 0003_service_desk.up.sql: Service Desk (menu 137-138, BR-SD-01)
-- Tabel 0002 (menu 81) tidak diubah; tiket hanya mereferensikannya.

CREATE TABLE sd_teams (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    school_id UUID NOT NULL,
    name TEXT NOT NULL,
    manager_user_id UUID NULL, -- supervisor tim, target eskalasi team_manager (AC-138-01)
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    deleted_at TIMESTAMPTZ NULL
);
CREATE INDEX idx_sd_teams_school ON sd_teams (school_id) WHERE deleted_at IS NULL;

CREATE TABLE sd_team_members (
    team_id UUID NOT NULL REFERENCES sd_teams(id) ON DELETE CASCADE,
    user_id UUID NOT NULL,
    school_id UUID NOT NULL,
    PRIMARY KEY (team_id, user_id)
);
CREATE INDEX idx_sd_team_members_user ON sd_team_members (school_id, user_id);

CREATE TABLE sd_categories (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    school_id UUID NOT NULL,
    name TEXT NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    team_id UUID NOT NULL REFERENCES sd_teams(id),
    is_facility BOOLEAN NOT NULL DEFAULT false,
    form_schema JSONB NOT NULL DEFAULT '[]',
    reporter_kinds TEXT[] NOT NULL CHECK (
        cardinality(reporter_kinds) > 0
        AND reporter_kinds <@ ARRAY['orang_tua','siswa','pegawai']::text[]
    ),
    service_request_category_id UUID NULL UNIQUE REFERENCES service_request_categories(id),
    is_fallback BOOLEAN NOT NULL DEFAULT false,
    active BOOLEAN NOT NULL DEFAULT true,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    deleted_at TIMESTAMPTZ NULL
);
CREATE INDEX idx_sd_categories_school ON sd_categories (school_id) WHERE deleted_at IS NULL;
CREATE UNIQUE INDEX idx_sd_categories_fallback ON sd_categories (school_id) WHERE is_fallback AND deleted_at IS NULL;

-- Jam layanan per hari (1 = Senin ... 7 = Minggu). Tidak ada baris = hari tutup.
CREATE TABLE sd_business_hours (
    school_id UUID NOT NULL,
    weekday INT NOT NULL CHECK (weekday BETWEEN 1 AND 7),
    opens_at TIME NOT NULL,
    closes_at TIME NOT NULL,
    PRIMARY KEY (school_id, weekday),
    CHECK (closes_at > opens_at)
);

CREATE TABLE sd_holidays (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    school_id UUID NOT NULL,
    date DATE NOT NULL,
    name TEXT NOT NULL,
    UNIQUE (school_id, date)
);

CREATE TABLE sd_sla_policies (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    school_id UUID NOT NULL,
    category_id UUID NULL REFERENCES sd_categories(id),
    priority TEXT NOT NULL CHECK (priority IN ('low','normal','high','urgent')),
    response_value INT NOT NULL CHECK (response_value > 0),
    response_unit TEXT NOT NULL CHECK (response_unit IN ('business_hours','business_days')),
    resolution_value INT NOT NULL CHECK (resolution_value > 0),
    resolution_unit TEXT NOT NULL CHECK (resolution_unit IN ('business_hours','business_days')),
    warn_bp INT NOT NULL CHECK (warn_bp BETWEEN 1 AND 9999),
    effective_from DATE NOT NULL,
    effective_to DATE NULL CHECK (effective_to IS NULL OR effective_to >= effective_from),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    deleted_at TIMESTAMPTZ NULL
);
CREATE INDEX idx_sd_sla_policies_school ON sd_sla_policies (school_id) WHERE deleted_at IS NULL;

CREATE TABLE sd_escalation_rules (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    school_id UUID NOT NULL,
    policy_id UUID NOT NULL REFERENCES sd_sla_policies(id) ON DELETE CASCADE,
    trigger TEXT NOT NULL CHECK (trigger IN ('warning','breach')),
    level INT NOT NULL CHECK (level > 0),
    target_kind TEXT NOT NULL CHECK (target_kind IN ('team_manager','role','user')),
    target_role TEXT NULL,
    target_user_id UUID NULL,
    UNIQUE (policy_id, trigger, level),
    CHECK (target_kind <> 'role' OR target_role IS NOT NULL),
    CHECK (target_kind <> 'user' OR target_user_id IS NOT NULL)
);

CREATE TABLE sd_ticket_sequences (
    school_id UUID NOT NULL,
    year INT NOT NULL,
    last INT NOT NULL DEFAULT 0,
    PRIMARY KEY (school_id, year)
);

CREATE TABLE sd_tickets (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    school_id UUID NOT NULL,
    number TEXT NOT NULL,
    category_id UUID NOT NULL REFERENCES sd_categories(id),
    team_id UUID NOT NULL REFERENCES sd_teams(id),
    priority TEXT NOT NULL CHECK (priority IN ('low','normal','high','urgent')),
    subject TEXT NOT NULL,
    description TEXT NOT NULL,
    form_answers JSONB NOT NULL DEFAULT '{}',
    channel TEXT NOT NULL CHECK (channel IN ('parent_portal','student_portal','employee_portal','agent')),
    reporter_user_id UUID NOT NULL,
    student_user_id UUID NULL,
    service_request_id UUID NULL UNIQUE REFERENCES service_requests(id),
    status TEXT NOT NULL CHECK (status IN ('new','assigned','in_progress','resolved','closed','rejected')),
    assignee_user_id UUID NULL,
    sla_snapshot JSONB NULL, -- salinan policy + aturan eskalasi saat tiket dibuat (tidak retroaktif)
    response_due_at TIMESTAMPTZ NULL,
    resolution_due_at TIMESTAMPTZ NULL,
    first_responded_at TIMESTAMPTZ NULL,
    resolved_at TIMESTAMPTZ NULL,
    closed_at TIMESTAMPTZ NULL,
    response_breached_at TIMESTAMPTZ NULL,
    resolution_breached_at TIMESTAMPTZ NULL,
    escalation_level INT NOT NULL DEFAULT 0,
    work_order_id UUID NULL,
    work_order_number TEXT NULL,
    satisfaction_score SMALLINT NULL CHECK (satisfaction_score BETWEEN 1 AND 5),
    satisfaction_comment TEXT NULL,
    rated_at TIMESTAMPTZ NULL,
    idempotency_key TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (school_id, idempotency_key),
    UNIQUE (school_id, number)
);
CREATE INDEX idx_sd_tickets_status_due ON sd_tickets (school_id, status, resolution_due_at);
CREATE INDEX idx_sd_tickets_assignee ON sd_tickets (school_id, assignee_user_id);
CREATE INDEX idx_sd_tickets_reporter ON sd_tickets (school_id, reporter_user_id);

CREATE TABLE sd_ticket_events (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    school_id UUID NOT NULL,
    ticket_id UUID NOT NULL REFERENCES sd_tickets(id) ON DELETE CASCADE,
    kind TEXT NOT NULL CHECK (kind IN ('status','comment','assign','escalation','sla_breach','work_order','satisfaction')),
    from_status TEXT NULL,
    to_status TEXT NULL,
    body TEXT NULL,
    internal BOOLEAN NOT NULL DEFAULT false, -- komentar internal tidak tampil ke pelapor
    attachments JSONB NOT NULL DEFAULT '[]',
    actor_user_id UUID NULL,
    dedupe_key TEXT NULL,
    seq BIGSERIAL, -- stable timeline order for events sharing a timestamp
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (ticket_id, dedupe_key)
);
CREATE INDEX idx_sd_ticket_events_ticket ON sd_ticket_events (ticket_id, created_at, seq);
