-- Generalize the CBT runner for academic students and admission applicants.
-- Existing rows remain academic/student and all legacy columns/routes continue to work.
ALTER TABLE exams
    ADD COLUMN audience TEXT NOT NULL DEFAULT 'academic'
        CHECK (audience IN ('academic', 'admission')),
    ADD COLUMN admission_context JSONB;

ALTER TABLE exams
    ALTER COLUMN academic_year_id DROP NOT NULL,
    ALTER COLUMN term_id DROP NOT NULL,
    ALTER COLUMN exam_type_id DROP NOT NULL,
    ALTER COLUMN classroom_id DROP NOT NULL,
    ALTER COLUMN subject_id DROP NOT NULL,
    ALTER COLUMN teacher_id DROP NOT NULL;

CREATE TABLE exam_participants (
    id                 UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    school_id          UUID NOT NULL,
    exam_id            UUID NOT NULL REFERENCES exams(id) ON DELETE CASCADE,
    participant_type   TEXT NOT NULL CHECK (participant_type IN ('student','applicant')),
    participant_id     UUID NOT NULL,
    display_name       TEXT,
    external_reference TEXT,
    created_at         TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at         TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (exam_id, participant_type, participant_id),
    UNIQUE (exam_id, participant_type, external_reference),
    CHECK (external_reference IS NULL OR external_reference <> '')
);
CREATE INDEX idx_exam_participants_exam ON exam_participants(exam_id, participant_type);
CREATE UNIQUE INDEX idx_exams_admission_external_ref
    ON exams (school_id, (admission_context->>'external_reference'))
    WHERE audience = 'admission';

-- Backfill the typed reference before making the legacy academic columns nullable.
ALTER TABLE exam_attempts
    ADD COLUMN participant_type TEXT NOT NULL DEFAULT 'student'
        CHECK (participant_type IN ('student','applicant')),
    ADD COLUMN participant_id UUID;
UPDATE exam_attempts SET participant_id = user_id WHERE participant_id IS NULL;
ALTER TABLE exam_attempts ALTER COLUMN participant_id SET NOT NULL;

INSERT INTO exam_participants (school_id, exam_id, participant_type, participant_id)
SELECT school_id, exam_id, 'student', user_id
FROM exam_attempts
ON CONFLICT (exam_id, participant_type, participant_id) DO NOTHING;

ALTER TABLE exam_attempts DROP CONSTRAINT exam_attempts_exam_id_user_id_key;
ALTER TABLE exam_attempts
    ALTER COLUMN academic_year_id DROP NOT NULL,
    ALTER COLUMN user_id DROP NOT NULL,
    ALTER COLUMN classroom_id DROP NOT NULL,
    ADD CONSTRAINT exam_attempts_exam_participant_key
        UNIQUE (exam_id, participant_type, participant_id),
    ADD CONSTRAINT exam_attempts_typed_legacy_ref_check CHECK (
        (participant_type = 'student' AND user_id = participant_id)
        OR (participant_type = 'applicant' AND user_id IS NULL)
    );
CREATE INDEX idx_exam_attempts_participant ON exam_attempts(participant_type, participant_id);

-- Admission participants use signed, short-lived JWTs. Persisting the jti hash
-- allows explicit revocation and ensures only tokens issued by this service work.
CREATE TABLE participant_access_tokens (
    id               UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    token_hash       TEXT NOT NULL UNIQUE,
    school_id        UUID NOT NULL,
    exam_id          UUID NOT NULL REFERENCES exams(id) ON DELETE CASCADE,
    participant_type TEXT NOT NULL CHECK (participant_type = 'applicant'),
    participant_id   UUID NOT NULL,
    expires_at       TIMESTAMPTZ NOT NULL,
    revoked_at       TIMESTAMPTZ,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    FOREIGN KEY (exam_id, participant_type, participant_id)
        REFERENCES exam_participants(exam_id, participant_type, participant_id) ON DELETE CASCADE
);
CREATE INDEX idx_participant_access_tokens_expiry ON participant_access_tokens(expires_at) WHERE revoked_at IS NULL;

-- Admission rows must not pretend to have academic ownership/context. Academic
-- rows retain the original required-field invariant.
ALTER TABLE exams ADD CONSTRAINT exams_audience_context_check CHECK (
    (audience = 'academic' AND academic_year_id IS NOT NULL AND term_id IS NOT NULL
        AND exam_type_id IS NOT NULL AND classroom_id IS NOT NULL
        AND subject_id IS NOT NULL AND teacher_id IS NOT NULL)
    OR
    (audience = 'admission' AND academic_year_id IS NULL AND term_id IS NULL
        AND classroom_id IS NULL AND subject_id IS NULL)
);
