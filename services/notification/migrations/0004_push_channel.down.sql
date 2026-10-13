-- 0004 down: remove rows with channel 'push' BEFORE reverting CHECK constraints, then drop push_subscriptions

DELETE FROM notification_deliveries WHERE channel = 'push';
DELETE FROM notification_templates WHERE channel = 'push';
DELETE FROM notification_preferences WHERE channel = 'push';

ALTER TABLE notification_deliveries DROP CONSTRAINT IF EXISTS notification_deliveries_channel_check;
ALTER TABLE notification_deliveries ADD CONSTRAINT notification_deliveries_channel_check CHECK (channel IN ('in_app', 'email', 'whatsapp'));

ALTER TABLE notification_templates DROP CONSTRAINT IF EXISTS notification_templates_channel_check;
ALTER TABLE notification_templates ADD CONSTRAINT notification_templates_channel_check CHECK (channel IN ('in_app', 'email', 'whatsapp'));

ALTER TABLE notification_preferences DROP CONSTRAINT IF EXISTS notification_preferences_channel_check;
ALTER TABLE notification_preferences ADD CONSTRAINT notification_preferences_channel_check CHECK (channel IN ('in_app', 'email', 'whatsapp'));

DROP TABLE IF EXISTS push_subscriptions CASCADE;
