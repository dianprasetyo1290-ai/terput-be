-- notification-service 0003 (T13, PRD menu 147–148): marketing consent,
-- segmented campaigns with their own recipient ledger, and the teacher–parent
-- extension of the 0002 chat. Nothing from 0001/0002 is dropped or rewritten.

-- Promotional consent (BR-COM-01, OQ-43). No row = no consent = NOT sent.
-- subject_key is 'user:<uuid>' | 'phone:<sha256>' | 'email:<sha256>' so a PPDB
-- applicant without an identity account can still consent / opt out.
CREATE TABLE marketing_consents (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    school_id   UUID NOT NULL,
    subject_key TEXT NOT NULL,
    user_id     UUID,
    channel     TEXT NOT NULL CHECK (channel IN ('whatsapp','email','in_app')),
    status      TEXT NOT NULL CHECK (status IN ('opted_in','opted_out')),
    source      TEXT NOT NULL CHECK (source IN ('preferences','ppdb_form','unsubscribe_link','wa_keyword','admin_import')),
    changed_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT marketing_consents_unique UNIQUE (school_id, subject_key, channel)
);
CREATE INDEX idx_marketing_consents_user ON marketing_consents (school_id, user_id);

-- Append-only consent history.
CREATE TABLE marketing_consent_events (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    school_id     UUID NOT NULL,
    subject_key   TEXT NOT NULL,
    channel       TEXT NOT NULL CHECK (channel IN ('whatsapp','email','in_app')),
    status        TEXT NOT NULL CHECK (status IN ('opted_in','opted_out')),
    source        TEXT NOT NULL CHECK (source IN ('preferences','ppdb_form','unsubscribe_link','wa_keyword','admin_import')),
    actor_user_id UUID,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_marketing_consent_events_subject ON marketing_consent_events (school_id, subject_key, created_at DESC);

-- Saved recipient segments (FR-147-001). filter: grade_levels, classroom_ids,
-- period_id, statuses.
CREATE TABLE campaign_segments (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    school_id  UUID NOT NULL,
    name       TEXT NOT NULL,
    kind       TEXT NOT NULL CHECK (kind IN ('parents','ppdb_applicants','alumni_parents')),
    filter     JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_by UUID NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    deleted_at TIMESTAMPTZ
);
CREATE INDEX idx_campaign_segments_school ON campaign_segments (school_id) WHERE deleted_at IS NULL;

-- WhatsApp templates mirrored from the provider (FR-147-002). Only
-- approval_status='approved' may be sent.
CREATE TABLE wa_templates (
    id                   UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    school_id            UUID NOT NULL,
    name                 TEXT NOT NULL,
    provider_template_id TEXT NOT NULL,
    language             TEXT NOT NULL,
    body_preview         TEXT NOT NULL DEFAULT '',
    variables            JSONB NOT NULL DEFAULT '[]'::jsonb,
    approval_status      TEXT NOT NULL DEFAULT 'pending' CHECK (approval_status IN ('pending','approved','rejected')),
    created_at           TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at           TIMESTAMPTZ NOT NULL DEFAULT now(),
    deleted_at           TIMESTAMPTZ
);
CREATE INDEX idx_wa_templates_school ON wa_templates (school_id) WHERE deleted_at IS NULL;

CREATE TABLE campaigns (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    school_id    UUID NOT NULL,
    name         TEXT NOT NULL,
    segment_id   UUID NOT NULL REFERENCES campaign_segments(id),
    channel      TEXT NOT NULL CHECK (channel IN ('whatsapp','email','in_app')),
    template_id  UUID REFERENCES wa_templates(id),
    template_vars JSONB NOT NULL DEFAULT '{}'::jsonb,
    subject      TEXT,
    body         TEXT,
    scheduled_at TIMESTAMPTZ,
    status       TEXT NOT NULL DEFAULT 'draft' CHECK (status IN ('draft','scheduled','sending','sent','cancelled','failed')),
    last_error   TEXT,
    created_by   UUID NOT NULL,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    sent_at      TIMESTAMPTZ,
    CONSTRAINT campaigns_wa_template CHECK (channel <> 'whatsapp' OR template_id IS NOT NULL)
);
CREATE INDEX idx_campaigns_school ON campaigns (school_id, created_at DESC);
CREATE INDEX idx_campaigns_due ON campaigns (status, scheduled_at) WHERE status = 'scheduled';

-- Campaign recipient ledger (NFR-ADV-06). Contacts are stored masked + hashed
-- only; the raw contact is re-read at send time. UNIQUE makes a re-run of the
-- dispatch job idempotent.
CREATE TABLE campaign_recipients (
    id                  UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    school_id           UUID NOT NULL,
    campaign_id         UUID NOT NULL REFERENCES campaigns(id),
    subject_key         TEXT NOT NULL,
    user_id             UUID,
    display_name        TEXT NOT NULL DEFAULT '',
    contact_masked      TEXT NOT NULL DEFAULT '',
    contact_hash        TEXT NOT NULL DEFAULT '',
    status              TEXT NOT NULL CHECK (status IN ('queued','excluded','sent','delivered','read','replied','failed')),
    exclude_reason      TEXT CHECK (exclude_reason IN ('no_consent','opted_out','no_contact','quota')),
    provider_message_id TEXT,
    notification_id     UUID,
    last_error          TEXT,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    sent_at             TIMESTAMPTZ,
    delivered_at        TIMESTAMPTZ,
    read_at             TIMESTAMPTZ,
    replied_at          TIMESTAMPTZ,
    CONSTRAINT campaign_recipients_unique UNIQUE (campaign_id, subject_key)
);
CREATE INDEX idx_campaign_recipients_status ON campaign_recipients (campaign_id, status);
CREATE INDEX idx_campaign_recipients_provider ON campaign_recipients (provider_message_id) WHERE provider_message_id IS NOT NULL;

-- monthly_wa_quota NULL = no quota (no built-in number).
CREATE TABLE campaign_settings (
    school_id        UUID PRIMARY KEY,
    monthly_wa_quota INTEGER CHECK (monthly_wa_quota IS NULL OR monthly_wa_quota >= 0),
    updated_at       TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Teacher–parent chat (FR-148-*): additive extension of 0002.
ALTER TABLE chat_threads
    ADD COLUMN kind TEXT NOT NULL DEFAULT 'general' CHECK (kind IN ('general','teacher_parent')),
    ADD COLUMN student_user_id UUID,
    ADD COLUMN teacher_user_id UUID,
    ADD COLUMN guardian_user_id UUID,
    ADD COLUMN archived_at TIMESTAMPTZ,
    ADD CONSTRAINT chat_threads_teacher_parent_trio CHECK (
        kind = 'general' OR (student_user_id IS NOT NULL AND teacher_user_id IS NOT NULL AND guardian_user_id IS NOT NULL));
CREATE UNIQUE INDEX uq_chat_threads_teacher_parent_active
    ON chat_threads (school_id, teacher_user_id, guardian_user_id, student_user_id)
    WHERE kind = 'teacher_parent' AND closed_at IS NULL;

ALTER TABLE chat_messages
    ADD COLUMN attachment_name TEXT,
    ADD COLUMN attachment_mime TEXT,
    ADD COLUMN attachment_size BIGINT,
    ADD COLUMN outside_service_hours BOOLEAN NOT NULL DEFAULT false,
    ADD COLUMN school_id UUID;
UPDATE chat_messages m SET school_id = t.school_id FROM chat_threads t WHERE t.id = m.thread_id AND m.school_id IS NULL;
CREATE INDEX idx_chat_messages_school ON chat_messages (school_id, created_at);

-- Service hours NULL = no service-hours notice. 5 MB default follows FR-16-002.
CREATE TABLE chat_settings (
    school_id            UUID PRIMARY KEY,
    service_start        TIME,
    service_end          TIME,
    service_days         INTEGER[],
    max_attachment_bytes BIGINT NOT NULL DEFAULT 5242880 CHECK (max_attachment_bytes > 0),
    updated_at           TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT chat_settings_hours_pair CHECK ((service_start IS NULL) = (service_end IS NULL))
);

CREATE TABLE chat_message_reports (
    id               UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    school_id        UUID NOT NULL,
    message_id       UUID NOT NULL REFERENCES chat_messages(id),
    reporter_user_id UUID NOT NULL,
    reason           TEXT NOT NULL,
    status           TEXT NOT NULL DEFAULT 'open' CHECK (status IN ('open','reviewed','dismissed')),
    reviewed_by      UUID,
    review_note      TEXT,
    reviewed_at      TIMESTAMPTZ,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT chat_message_reports_unique UNIQUE (message_id, reporter_user_id)
);
CREATE INDEX idx_chat_message_reports_school ON chat_message_reports (school_id, status, created_at DESC);
