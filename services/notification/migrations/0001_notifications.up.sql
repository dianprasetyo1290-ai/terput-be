-- notification-service 0001: notifications, per-user channel preferences,
-- reply templates and delivery bookkeeping for one tenant (school_id).
-- See ../../docs/CONVENTIONS.md for the multi-tenancy rule every table here
-- follows (school_id + query scoped to the caller's JWT school_id).
CREATE EXTENSION IF NOT EXISTS pgcrypto;

-- notifications: one row per message addressed to ONE user (the portal wali
-- receiver). `data` is free-form jsonb for the client (deep-link ids etc).
-- `idempotency_key` makes POST /internal/notifications safe to retry: the same
-- (school_id, idempotency_key) can never create a second row. It is nullable
-- because notifications created through other paths have no external key.
CREATE TABLE notifications (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    school_id       UUID NOT NULL,
    user_id         UUID NOT NULL,                       -- recipient (identity-service users.id)
    type            TEXT NOT NULL,                       -- e.g. billing.invoice.paid, admission.decision
    title           TEXT NOT NULL,
    body            TEXT NOT NULL DEFAULT '',
    data            JSONB NOT NULL DEFAULT '{}'::jsonb,
    idempotency_key TEXT,
    read_at         TIMESTAMPTZ,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),

    CONSTRAINT notifications_school_idempotency_unique UNIQUE (school_id, idempotency_key)
);

-- The tenant+user+read index the portal inbox / unread badge hits constantly.
CREATE INDEX idx_notifications_school_user_read ON notifications (school_id, user_id, read_at);
CREATE INDEX idx_notifications_school_user_created ON notifications (school_id, user_id, created_at DESC);

-- notification_preferences: per user, per channel on/off switch. A missing row
-- means "use the default" (in_app on, email/whatsapp off) so the client only
-- ever stores what the user actually changed.
CREATE TABLE notification_preferences (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    school_id   UUID NOT NULL,
    user_id     UUID NOT NULL,
    channel     TEXT NOT NULL CHECK (channel IN ('in_app', 'email', 'whatsapp')),
    enabled     BOOLEAN NOT NULL DEFAULT true,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now(),

    CONSTRAINT notification_preferences_unique UNIQUE (school_id, user_id, channel)
);
CREATE INDEX idx_notification_preferences_user ON notification_preferences (school_id, user_id);

-- notification_templates: versioned message bodies keyed by a stable `key`
-- (e.g. "admission.decision.accepted"). A new version is a new row so a send
-- can always be replayed against the exact text that produced it.
CREATE TABLE notification_templates (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    school_id   UUID NOT NULL,
    key         TEXT NOT NULL,
    version     INTEGER NOT NULL DEFAULT 1 CHECK (version > 0),
    channel     TEXT NOT NULL CHECK (channel IN ('in_app', 'email', 'whatsapp')),
    subject     TEXT NOT NULL DEFAULT '',
    body        TEXT NOT NULL,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now(),

    CONSTRAINT notification_templates_unique UNIQUE (school_id, key, version, channel)
);
CREATE INDEX idx_notification_templates_key ON notification_templates (school_id, key, channel, version DESC);

-- notification_deliveries: one row per (notification, channel) attempt ledger.
-- status is pending -> sent | failed; attempts counts provider calls so a retry
-- is observable and bounded. provider_message_id is the upstream receipt.
CREATE TABLE notification_deliveries (
    id                  UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    notification_id     UUID NOT NULL REFERENCES notifications(id) ON DELETE CASCADE,
    channel             TEXT NOT NULL CHECK (channel IN ('in_app', 'email', 'whatsapp')),
    status              TEXT NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'sent', 'failed')),
    attempts            INTEGER NOT NULL DEFAULT 0 CHECK (attempts >= 0),
    last_error          TEXT,
    provider_message_id TEXT,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT now(),

    CONSTRAINT notification_deliveries_unique UNIQUE (notification_id, channel)
);
CREATE INDEX idx_notification_deliveries_status ON notification_deliveries (status, created_at);
CREATE INDEX idx_notification_deliveries_notification ON notification_deliveries (notification_id);
