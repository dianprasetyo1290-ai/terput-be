CREATE TABLE IF NOT EXISTS assistant_settings (
    school_id UUID PRIMARY KEY,
    log_retention_days INT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS help_articles (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    school_id UUID,
    title TEXT NOT NULL,
    body_md TEXT NOT NULL,
    tags TEXT[] NOT NULL DEFAULT '{}',
    route_name TEXT,
    status TEXT NOT NULL CHECK (status IN ('draft', 'published')),
    updated_by UUID,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS assistant_intents (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    school_id UUID,
    key TEXT NOT NULL,
    phrases TEXT[] NOT NULL,
    tool_key TEXT NOT NULL,
    roles TEXT[] NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS assistant_conversations (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    school_id UUID NOT NULL,
    user_id UUID NOT NULL,
    active_role TEXT NOT NULL,
    started_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    last_message_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS assistant_messages (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    school_id UUID NOT NULL,
    conversation_id UUID NOT NULL REFERENCES assistant_conversations(id) ON DELETE CASCADE,
    sender TEXT NOT NULL CHECK (sender IN ('user', 'assistant')),
    content TEXT NOT NULL,
    ai_generated BOOLEAN NOT NULL DEFAULT false,
    sources JSONB NOT NULL DEFAULT '[]'::jsonb,
    tool_calls JSONB NOT NULL DEFAULT '[]'::jsonb,
    provider TEXT,
    latency_ms INT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS assistant_actions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    school_id UUID NOT NULL,
    conversation_id UUID NOT NULL REFERENCES assistant_conversations(id) ON DELETE CASCADE,
    message_id UUID NOT NULL REFERENCES assistant_messages(id) ON DELETE CASCADE,
    action_key TEXT NOT NULL,
    payload JSONB NOT NULL,
    preview JSONB NOT NULL,
    status TEXT NOT NULL CHECK (status IN ('proposed', 'confirmed', 'executed', 'cancelled', 'failed', 'expired')),
    proposed_by UUID NOT NULL,
    confirmed_at TIMESTAMPTZ,
    executed_at TIMESTAMPTZ,
    result JSONB,
    expires_at TIMESTAMPTZ NOT NULL,
    idempotency_key TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(school_id, idempotency_key)
);

-- Seed Initial System Intents
INSERT INTO assistant_intents (school_id, key, phrases, tool_key, roles)
VALUES
    (NULL, 'help_search', ARRAY['panduan', 'bantuan', 'cara pakai', 'tutorial', 'manual', 'bagaimana cara', 'fitur apa'], 'help_search', ARRAY['admin', 'superadmin', 'kepala_sekolah', 'kepala_yayasan', 'finance_manager', 'bi_analyst', 'guru', 'walikelas', 'siswa', 'orang_tua', 'staf']),
    (NULL, 'metric_lookup', ARRAY['metrik', 'kamus metrik', 'rumus', 'definisi metrik', 'nilai metrik', 'angka kolektibilitas', 'angka kehadiran', 'jumlah siswa'], 'metric_lookup', ARRAY['admin', 'superadmin', 'kepala_sekolah', 'kepala_yayasan', 'finance_manager', 'bi_analyst']),
    (NULL, 'my_bills', ARRAY['tagihan saya', 'status spp saya', 'riwayat bayar', 'tunggakan saya', 'tagihan spp'], 'my_bills', ARRAY['siswa', 'orang_tua']),
    (NULL, 'child_bills', ARRAY['tagihan anak', 'spp anak saya', 'tunggakan anak'], 'child_bills', ARRAY['orang_tua']),
    (NULL, 'arrears_summary', ARRAY['ringkasan tunggakan', 'total tunggakan sekolah', 'aging tunggakan', 'daftar penunggak'], 'arrears_summary', ARRAY['admin', 'superadmin', 'finance_manager', 'kepala_sekolah', 'kepala_yayasan']),
    (NULL, 'draft_announcement', ARRAY['buat draf pengumuman', 'buat draf broadcast', 'buat surat edaran'], 'draft_announcement', ARRAY['admin', 'superadmin', 'kepala_sekolah']),
    (NULL, 'draft_message', ARRAY['susun pesan', 'draf pesan wali', 'tulis pesan'], 'draft_message', ARRAY['admin', 'superadmin', 'guru', 'walikelas', 'kepala_sekolah']),
    (NULL, 'summarize_report', ARRAY['ringkas laporan', 'ringkasan bulanan', 'analisis singkat'], 'summarize_report', ARRAY['admin', 'superadmin', 'kepala_sekolah', 'kepala_yayasan', 'bi_analyst', 'finance_manager']),
    (NULL, 'send_payment_reminders', ARRAY['ingatkan tunggakan', 'kirim pengingat spp', 'kirim notifikasi tagihan'], 'send_payment_reminders', ARRAY['admin', 'superadmin', 'finance_manager'])
ON CONFLICT DO NOTHING;
