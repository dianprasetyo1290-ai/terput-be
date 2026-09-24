-- Yadika13 items 6 & 7: ujian susulan (a re-sit).
--
-- Item 9's "Reset Ujian" puts a student back to belum_mulai so they can sit
-- the exam again. Item 6 says the score that counts must stay the FIRST one
-- the guru approved ("di-ACC"), no matter how many re-sits follow, and that
-- the earlier sittings must be kept as history.
--
-- Two pieces, deliberately no new "susulan exam" entity: a re-sit is the
-- same exam_attempts row starting over, so history is a snapshot table and
-- the approved score is three columns on the attempt itself.

-- The approved ("ACC") score. NULL = never approved, so exam_attempts.score
-- (the latest sitting) is what counts -- unchanged behaviour for every row
-- that exists today. Once set it wins over score until the guru ACCs again.
ALTER TABLE exam_attempts
    ADD COLUMN final_score  DOUBLE PRECISION,
    ADD COLUMN final_acc_at TIMESTAMPTZ,
    ADD COLUMN final_acc_by UUID;

-- One row per completed sitting that a Reset Ujian cleared away. answers is
-- a snapshot rather than a foreign key: exam_attempt_answers rows are
-- deleted by the reset (their unique key is per attempt, not per sitting),
-- and history here only ever needs to be read back, never re-scored.
CREATE TABLE exam_attempt_sittings (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    school_id     UUID NOT NULL,
    exam_id       UUID NOT NULL,
    user_id       UUID NOT NULL,
    attempt_id    UUID NOT NULL REFERENCES exam_attempts(id) ON DELETE CASCADE,
    sitting       INT  NOT NULL,                  -- 1 = the original sitting
    status        TEXT NOT NULL,                  -- status at the moment of reset
    score         DOUBLE PRECISION,
    correct_count INT,
    started_at    TIMESTAMPTZ,
    submitted_at  TIMESTAMPTZ,
    answers       JSONB NOT NULL DEFAULT '[]'::jsonb,
    reset_by      UUID,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (attempt_id, sitting)
);
CREATE INDEX idx_exam_attempt_sittings_exam_user ON exam_attempt_sittings(exam_id, user_id);
