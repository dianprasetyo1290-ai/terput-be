DROP INDEX idx_exam_tokens_school_code;
ALTER TABLE exam_tokens DROP CONSTRAINT exam_tokens_school_user_key;

ALTER TABLE exam_tokens ALTER COLUMN valid_until TYPE TIMESTAMPTZ USING valid_until::timestamptz;
ALTER TABLE exam_tokens DROP COLUMN valid_from;
ALTER TABLE exam_tokens RENAME COLUMN valid_until TO expires_at;
ALTER TABLE exam_tokens ADD COLUMN classroom_id UUID NOT NULL DEFAULT gen_random_uuid();
ALTER TABLE exam_tokens ADD COLUMN exam_id UUID NOT NULL DEFAULT gen_random_uuid();

ALTER TABLE exam_tokens ADD CONSTRAINT exam_tokens_exam_id_fkey FOREIGN KEY (exam_id) REFERENCES exams(id) ON DELETE CASCADE;
ALTER TABLE exam_tokens ADD CONSTRAINT exam_tokens_exam_id_user_id_key UNIQUE (exam_id, user_id);
CREATE UNIQUE INDEX idx_exam_tokens_code ON exam_tokens(exam_id, code);
