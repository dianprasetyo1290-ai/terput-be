ALTER TABLE class_sessions
    DROP CONSTRAINT IF EXISTS class_sessions_tugas_jenis_check,
    DROP COLUMN IF EXISTS substituted_by,
    DROP COLUMN IF EXISTS substituted_at,
    DROP COLUMN IF EXISTS original_guru_id,
    DROP COLUMN IF EXISTS tugas_jenis,
    DROP COLUMN IF EXISTS tugas_diberikan,
    DROP COLUMN IF EXISTS catatan,
    DROP COLUMN IF EXISTS checked_out_at;
