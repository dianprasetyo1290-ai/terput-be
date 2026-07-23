-- exam-service schema (CBT / Tes Basis Komputer engine)
CREATE EXTENSION IF NOT EXISTS pgcrypto;

-- ==========================================================================
-- Kelola Tipe Ujian (admin CRUD; guru read-only)
-- ==========================================================================
CREATE TABLE exam_types (
    id                UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    school_id         UUID NOT NULL,
    academic_year_id  UUID NOT NULL,
    name              TEXT NOT NULL,              -- Nama Ujian
    mapping_rapor     TEXT NOT NULL,               -- Mapping Rapor: which report-card score bucket (report-service reads this)
    created_by        UUID NOT NULL,
    created_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
    deleted_at        TIMESTAMPTZ
);
CREATE INDEX idx_exam_types_school ON exam_types(school_id, academic_year_id) WHERE deleted_at IS NULL;

-- ==========================================================================
-- Bank Soal: folders/Kategori (admin creates) -> questions inside (guru adds,
-- guru only ever sees their own; admin sees all).
-- ==========================================================================
CREATE TABLE question_folders (
    id                UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    school_id         UUID NOT NULL,
    academic_year_id  UUID NOT NULL,               -- keeps old years from mixing with current
    name              TEXT NOT NULL,               -- e.g. "UJIAN SUMATIF AKHIR JENJANG 2026/2027 Kelas X"
    description       TEXT,
    created_by        UUID NOT NULL,
    created_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
    deleted_at        TIMESTAMPTZ
);
CREATE INDEX idx_question_folders_school_year ON question_folders(school_id, academic_year_id) WHERE deleted_at IS NULL;

-- question_type: pilihan_ganda (5 opsi a-e), benar_salah (2 opsi), essay (free
-- text, guru-defined acceptable opsi, manual override allowed), tebak_gambar
-- (image + guru-defined acceptable opsi, NO manual override allowed).
CREATE TABLE questions (
    id                     UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    school_id              UUID NOT NULL,
    academic_year_id       UUID NOT NULL,
    folder_id              UUID NOT NULL REFERENCES question_folders(id) ON DELETE CASCADE,
    created_by             UUID NOT NULL,          -- guru who authored it; used for guru-only-sees-own-soal scoping
    question_type          TEXT NOT NULL CHECK (question_type IN ('pilihan_ganda','benar_salah','essay','tebak_gambar')),
    content                TEXT NOT NULL,          -- soal stem
    image_url              TEXT,                   -- optional uploaded image on the stem (also holds the Tebak Gambar image)
    no_kd                  TEXT,                   -- No KD
    konten_kd               TEXT,                   -- Konten KD
    indikator_pencapaian   TEXT,                   -- Indikator Pencapaian
    level_kognitif          TEXT,                   -- Level Kognitif
    poin                   NUMERIC(6,2) NOT NULL DEFAULT 1.00, -- raw poin as entered by guru (pre score-normalization)
    is_published           BOOLEAN NOT NULL DEFAULT false,     -- reviewed/published by admin
    created_at             TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at             TIMESTAMPTZ NOT NULL DEFAULT now(),
    deleted_at              TIMESTAMPTZ
);
CREATE INDEX idx_questions_folder ON questions(folder_id) WHERE deleted_at IS NULL;
CREATE INDEX idx_questions_created_by ON questions(school_id, created_by) WHERE deleted_at IS NULL;
CREATE INDEX idx_questions_school_year ON questions(school_id, academic_year_id) WHERE deleted_at IS NULL;

-- Answer options for pilihan_ganda (exactly 5, a-e) and benar_salah (exactly 2).
CREATE TABLE answer_options (
    id                UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    question_id       UUID NOT NULL REFERENCES questions(id) ON DELETE CASCADE,
    content           TEXT,                        -- option text (option can have text and/or image)
    image_url         TEXT,
    is_correct        BOOLEAN NOT NULL DEFAULT false,
    order_index       INT NOT NULL,                -- 0=a,1=b,... never shuffled
    created_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (question_id, order_index)
);
CREATE INDEX idx_answer_options_question ON answer_options(question_id);

-- Guru-defined acceptable free-text answers for essay & tebak_gambar
-- (auto-grading matches student's typed answer case-insensitive/trimmed
-- against any row here).
CREATE TABLE essay_answer_options (
    id                UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    question_id       UUID NOT NULL REFERENCES questions(id) ON DELETE CASCADE,
    answer_text       TEXT NOT NULL,
    created_at        TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_essay_answer_options_question ON essay_answer_options(question_id);

-- ==========================================================================
-- Buat Ujian Baru
-- ==========================================================================
CREATE TABLE exams (
    id                UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    school_id         UUID NOT NULL,
    academic_year_id  UUID NOT NULL,
    term_id           UUID NOT NULL,               -- Semester/Tahun Ajaran, from academic-service's terms
    exam_type_id      UUID NOT NULL REFERENCES exam_types(id),
    title             TEXT NOT NULL,               -- Judul Ujian
    description       TEXT,                        -- Deskripsi/catatan (optional)
    classroom_id      UUID NOT NULL,               -- Mata Pelajaran block: kelas (from academic-service; trusted from JWT+payload, see README Assumptions)
    subject_id        UUID NOT NULL,               -- mapel (from academic-service)
    teacher_id        UUID NOT NULL,               -- guru who owns/teaches this exam (admin assigns explicitly; guru = self)
    start_at          TIMESTAMPTZ NOT NULL,        -- Tanggal Mulai
    end_at            TIMESTAMPTZ NOT NULL,        -- Tanggal Selesai
    is_shuffled       BOOLEAN NOT NULL DEFAULT false, -- Acak Soal
    is_published      BOOLEAN NOT NULL DEFAULT false, -- draft/publish lifecycle
    published_at      TIMESTAMPTZ,
    created_by        UUID NOT NULL,
    created_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
    deleted_at        TIMESTAMPTZ
);
CREATE INDEX idx_exams_school_year ON exams(school_id, academic_year_id) WHERE deleted_at IS NULL;
CREATE INDEX idx_exams_classroom ON exams(classroom_id) WHERE deleted_at IS NULL;
CREATE INDEX idx_exams_teacher ON exams(teacher_id) WHERE deleted_at IS NULL;

-- Join: which bank-soal questions belong to which exam, in what (natural,
-- pre-shuffle) order, at what final (post score-normalization) point value.
CREATE TABLE exam_questions (
    id                UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    exam_id           UUID NOT NULL REFERENCES exams(id) ON DELETE CASCADE,
    question_id       UUID NOT NULL REFERENCES questions(id) ON DELETE CASCADE,
    order_index       INT NOT NULL,                -- natural/base order (display numbering always follows this sequence 1..N)
    points            NUMERIC(6,2) NOT NULL,        -- scaled points for THIS exam so exam total == 100 on publish
    created_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (exam_id, question_id),
    UNIQUE (exam_id, order_index)
);
CREATE INDEX idx_exam_questions_exam ON exam_questions(exam_id);

-- ==========================================================================
-- Exam Token / Voucher
-- ==========================================================================
CREATE TABLE exam_tokens (
    id                  UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    school_id           UUID NOT NULL,
    academic_year_id    UUID NOT NULL,
    exam_id             UUID NOT NULL REFERENCES exams(id) ON DELETE CASCADE,
    user_id             UUID NOT NULL,             -- siswa
    classroom_id        UUID NOT NULL,             -- classroom this voucher was (bulk-)issued for; drives monitor's ?classroom_id= filter
    code                TEXT NOT NULL,
    expires_at          TIMESTAMPTZ NOT NULL,
    regenerated_count   INT NOT NULL DEFAULT 0,
    created_by          UUID NOT NULL,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (exam_id, user_id)
);
CREATE UNIQUE INDEX idx_exam_tokens_code ON exam_tokens(exam_id, code);

-- ==========================================================================
-- Exam Attempt state machine
--   belum_mulai -> aktif -> selesai
--   belum_mulai -> dilarang -> belum_mulai (Larang Ujian / Buka Larangan, admin only)
--   aktif -> blokir -> aktif (anti-cheat auto OR manual Blokir; Ulangi/Buka Blokir resumes, never wipes answers)
-- One row per (exam, student); created when a voucher is (bulk-)generated so
-- the live monitor roster can show "belum_mulai" before the student ever
-- opens the exam.
-- ==========================================================================
CREATE TABLE exam_attempts (
    id                  UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    school_id           UUID NOT NULL,
    academic_year_id    UUID NOT NULL,
    exam_id             UUID NOT NULL REFERENCES exams(id) ON DELETE CASCADE,
    user_id             UUID NOT NULL,             -- siswa
    classroom_id        UUID NOT NULL,             -- denormalized from the token, so /monitor can filter without a join
    token_id            UUID REFERENCES exam_tokens(id) ON DELETE SET NULL,
    status              TEXT NOT NULL DEFAULT 'belum_mulai'
                          CHECK (status IN ('belum_mulai','aktif','blokir','dilarang','selesai')),
    question_order      JSONB,                     -- ordered JSON array of exam_questions.id (as strings) for THIS student's attempt (shuffled per-attempt if exam.is_shuffled; displayed numbering 1..N always follows this array's order, never the underlying shuffle)
    current_position    INT NOT NULL DEFAULT 0,     -- 0-indexed pointer into question_order; resume point after unblock
    focus_lost_count    INT NOT NULL DEFAULT 0,
    started_at          TIMESTAMPTZ,
    submitted_at        TIMESTAMPTZ,
    blocked_at          TIMESTAMPTZ,
    blocked_reason       TEXT,
    blocked_by           UUID,
    banned_at           TIMESTAMPTZ,               -- "dilarang" (Larang Ujian)
    banned_reason        TEXT,                      -- required Alasan
    banned_by            UUID,
    score               NUMERIC(6,2),
    correct_count       INT,
    graded_at           TIMESTAMPTZ,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (exam_id, user_id)
);
CREATE INDEX idx_exam_attempts_exam_status ON exam_attempts(exam_id, status);
CREATE INDEX idx_exam_attempts_exam_classroom ON exam_attempts(exam_id, classroom_id);
CREATE INDEX idx_exam_attempts_user ON exam_attempts(user_id);

CREATE TABLE exam_attempt_answers (
    id                  UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    attempt_id          UUID NOT NULL REFERENCES exam_attempts(id) ON DELETE CASCADE,
    exam_question_id    UUID NOT NULL REFERENCES exam_questions(id) ON DELETE CASCADE,
    selected_option_id  UUID REFERENCES answer_options(id),  -- pilihan_ganda / benar_salah
    essay_answer        TEXT,                                -- essay / tebak_gambar typed answer
    is_correct          BOOLEAN,                              -- NULL until graded
    points_awarded      NUMERIC(6,2),
    manually_graded     BOOLEAN NOT NULL DEFAULT false,        -- true if a guru overrode auto-grading (essay only)
    answered_at         TIMESTAMPTZ NOT NULL DEFAULT now(),
    created_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (attempt_id, exam_question_id)
);
CREATE INDEX idx_exam_attempt_answers_attempt ON exam_attempt_answers(attempt_id);
