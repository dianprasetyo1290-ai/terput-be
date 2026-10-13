-- 0004: Push notification channel & WebPush subscriptions (T26, menu 159, FR-159-004)

-- 1. Extend channel CHECK constraints across preferences, templates, and deliveries
ALTER TABLE notification_preferences DROP CONSTRAINT IF EXISTS notification_preferences_channel_check;
ALTER TABLE notification_preferences ADD CONSTRAINT notification_preferences_channel_check CHECK (channel IN ('in_app', 'email', 'whatsapp', 'push'));

ALTER TABLE notification_templates DROP CONSTRAINT IF EXISTS notification_templates_channel_check;
ALTER TABLE notification_templates ADD CONSTRAINT notification_templates_channel_check CHECK (channel IN ('in_app', 'email', 'whatsapp', 'push'));

ALTER TABLE notification_deliveries DROP CONSTRAINT IF EXISTS notification_deliveries_channel_check;
ALTER TABLE notification_deliveries ADD CONSTRAINT notification_deliveries_channel_check CHECK (channel IN ('in_app', 'email', 'whatsapp', 'push'));

-- 2. Create push_subscriptions table
CREATE TABLE push_subscriptions (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    school_id       UUID NOT NULL,
    user_id         UUID NOT NULL,
    endpoint        TEXT NOT NULL UNIQUE,
    p256dh          TEXT NOT NULL,
    auth            TEXT NOT NULL,
    user_agent      TEXT NULL,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    last_success_at TIMESTAMPTZ NULL,
    revoked_at      TIMESTAMPTZ NULL
);

CREATE INDEX idx_push_subscriptions_active ON push_subscriptions (school_id, user_id) WHERE revoked_at IS NULL;
