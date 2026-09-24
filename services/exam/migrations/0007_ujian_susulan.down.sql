DROP TABLE IF EXISTS exam_attempt_sittings;
ALTER TABLE exam_attempts
    DROP COLUMN IF EXISTS final_score,
    DROP COLUMN IF EXISTS final_acc_at,
    DROP COLUMN IF EXISTS final_acc_by;
