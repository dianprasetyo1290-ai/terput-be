DROP TABLE IF EXISTS chat_message_reports;
DROP TABLE IF EXISTS chat_settings;

DROP INDEX IF EXISTS idx_chat_messages_school;
ALTER TABLE chat_messages
    DROP COLUMN IF EXISTS school_id,
    DROP COLUMN IF EXISTS outside_service_hours,
    DROP COLUMN IF EXISTS attachment_size,
    DROP COLUMN IF EXISTS attachment_mime,
    DROP COLUMN IF EXISTS attachment_name;

DROP INDEX IF EXISTS uq_chat_threads_teacher_parent_active;
ALTER TABLE chat_threads
    DROP CONSTRAINT IF EXISTS chat_threads_teacher_parent_trio,
    DROP COLUMN IF EXISTS archived_at,
    DROP COLUMN IF EXISTS guardian_user_id,
    DROP COLUMN IF EXISTS teacher_user_id,
    DROP COLUMN IF EXISTS student_user_id,
    DROP COLUMN IF EXISTS kind;

DROP TABLE IF EXISTS campaign_settings;
DROP TABLE IF EXISTS campaign_recipients;
DROP TABLE IF EXISTS campaigns;
DROP TABLE IF EXISTS wa_templates;
DROP TABLE IF EXISTS campaign_segments;
DROP TABLE IF EXISTS marketing_consent_events;
DROP TABLE IF EXISTS marketing_consents;
