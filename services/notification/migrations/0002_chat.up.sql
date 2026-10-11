-- notification-service 0002: chat between the school (guru/admin) and the
-- portal wali/siswa. A thread is tenant-scoped; participants are the ONLY
-- users who may read/post in it. Read state is per (message, user) so a
-- receipt is explicit and idempotent.
CREATE TABLE chat_threads (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    school_id   UUID NOT NULL,
    subject     TEXT NOT NULL DEFAULT '',
    created_by  UUID NOT NULL,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    closed_at   TIMESTAMPTZ
);
CREATE INDEX idx_chat_threads_school ON chat_threads (school_id, created_at DESC);

CREATE TABLE chat_thread_participants (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    thread_id   UUID NOT NULL REFERENCES chat_threads(id) ON DELETE CASCADE,
    user_id     UUID NOT NULL,
    role        TEXT NOT NULL DEFAULT 'member',   -- owner | member
    joined_at   TIMESTAMPTZ NOT NULL DEFAULT now(),

    CONSTRAINT chat_thread_participants_unique UNIQUE (thread_id, user_id)
);
CREATE INDEX idx_chat_participants_user ON chat_thread_participants (user_id, thread_id);

CREATE TABLE chat_messages (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    thread_id       UUID NOT NULL REFERENCES chat_threads(id) ON DELETE CASCADE,
    sender_user_id  UUID NOT NULL,
    body            TEXT NOT NULL DEFAULT '',
    attachment_key  TEXT,                          -- private storage key, never a public URL
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_chat_messages_thread ON chat_messages (thread_id, created_at);

CREATE TABLE chat_message_reads (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    message_id  UUID NOT NULL REFERENCES chat_messages(id) ON DELETE CASCADE,
    user_id     UUID NOT NULL,
    read_at     TIMESTAMPTZ NOT NULL DEFAULT now(),

    CONSTRAINT chat_message_reads_unique UNIQUE (message_id, user_id)
);
CREATE INDEX idx_chat_message_reads_user ON chat_message_reads (user_id, message_id);
