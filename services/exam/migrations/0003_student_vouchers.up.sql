-- Client spec (2026-07-24): a voucher is issued to a STUDENT, usable across
-- ANY exam within a date range — not one voucher per exam. Repurposes the
-- existing exam_tokens table rather than introducing a new one (same shape,
-- same admin-facing "Voucher" concept, just no longer exam-scoped): drops
-- exam_id/classroom_id (an exam_attempts row is now created lazily at entry
-- time, pulling classroom_id off the exam itself instead of the voucher),
-- adds a valid_from/valid_until window in place of a single expires_at.
ALTER TABLE exam_tokens DROP CONSTRAINT exam_tokens_exam_id_fkey;
ALTER TABLE exam_tokens DROP CONSTRAINT exam_tokens_exam_id_user_id_key;
DROP INDEX idx_exam_tokens_code;

ALTER TABLE exam_tokens DROP COLUMN exam_id;
ALTER TABLE exam_tokens DROP COLUMN classroom_id;
ALTER TABLE exam_tokens RENAME COLUMN expires_at TO valid_until;
ALTER TABLE exam_tokens ADD COLUMN valid_from DATE NOT NULL DEFAULT CURRENT_DATE;
ALTER TABLE exam_tokens ALTER COLUMN valid_until TYPE DATE USING valid_until::date;

ALTER TABLE exam_tokens ADD CONSTRAINT exam_tokens_school_user_key UNIQUE (school_id, user_id);
CREATE UNIQUE INDEX idx_exam_tokens_school_code ON exam_tokens(school_id, code);
