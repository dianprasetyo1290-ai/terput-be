-- 0006: Class management & teacher assignment (PRD menus 39-40).
--
-- FR-39-001: classroom capacity (NULL = unlimited, kept nullable so every
--            pre-existing class keeps working without a backfill).
-- FR-39-003: class_enrollment_history -- append-only log of every change to
--            a student's class assignment. class_enrollments keeps only the
--            CURRENT row per (academic_year_id, student_user_id), so before
--            this table a transfer / re-enroll silently overwrote the old
--            classroom_id and the history was lost.
-- FR-40-002: classroom_subjects.hours_per_week (jam pelajaran per minggu) so
--            teacher workload can be summed per guru.

ALTER TABLE classrooms
    ADD COLUMN IF NOT EXISTS capacity INT CHECK (capacity IS NULL OR capacity > 0);

ALTER TABLE classroom_subjects
    ADD COLUMN IF NOT EXISTS hours_per_week SMALLINT NOT NULL DEFAULT 0 CHECK (hours_per_week >= 0 AND hours_per_week <= 60);

-- No FKs to classrooms on purpose: history must survive a classroom hard
-- delete (class_enrollments cascades) and the trigger below fires while that
-- cascade is in flight. Classroom names are resolved at read time (classrooms
-- are soft-deleted, so the row is normally still there).
CREATE TABLE IF NOT EXISTS class_enrollment_history (
    id                 UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    school_id          UUID NOT NULL,
    academic_year_id   UUID NOT NULL,
    student_user_id    UUID NOT NULL,
    from_classroom_id  UUID,
    to_classroom_id    UUID,
    from_status        TEXT,
    to_status          TEXT,
    action             TEXT NOT NULL CHECK (action IN (
                           'enroll', 'move', 'remove', 'status_change',
                           'transfer', 'promotion', 'admission',
                           'withdrawal', 'unit_transfer', 'classroom_deleted', 'student_deleted')),
    reason             TEXT,
    request_id         UUID,   -- student_status_requests.id when the change came from Menu 78
    actor_user_id      UUID,   -- identity-service user id of whoever made the change (NULL = system/internal)
    created_at         TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_ceh_student ON class_enrollment_history (school_id, student_user_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_ceh_from_classroom ON class_enrollment_history (from_classroom_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_ceh_to_classroom ON class_enrollment_history (to_classroom_id, created_at DESC);

-- The trigger guarantees EVERY writer of class_enrollments is logged (manual
-- add/remove, transfer apply, promotion, admission saga, identity's delete
-- cascade, classroom delete) without each call site having to remember.
-- Call sites that know more context set transaction-local settings first
-- (see store.setEnrollmentContext):
--   skci.enroll_action, skci.enroll_reason, skci.enroll_request_id, skci.enroll_actor
-- Without them the action is derived from the row change.
CREATE OR REPLACE FUNCTION log_class_enrollment_change() RETURNS trigger AS $$
DECLARE
    v_action  TEXT := NULLIF(current_setting('skci.enroll_action', true), '');
    v_reason  TEXT := NULLIF(current_setting('skci.enroll_reason', true), '');
    v_request UUID := NULLIF(current_setting('skci.enroll_request_id', true), '')::uuid;
    v_actor   UUID := NULLIF(current_setting('skci.enroll_actor', true), '')::uuid;
BEGIN
    IF TG_OP = 'INSERT' THEN
        INSERT INTO class_enrollment_history (school_id, academic_year_id, student_user_id, from_classroom_id, to_classroom_id, from_status, to_status, action, reason, request_id, actor_user_id)
        VALUES (NEW.school_id, NEW.academic_year_id, NEW.student_user_id, NULL, NEW.classroom_id, NULL, NEW.status,
                COALESCE(v_action, 'enroll'), v_reason, v_request, v_actor);
        RETURN NEW;
    ELSIF TG_OP = 'UPDATE' THEN
        IF NEW.classroom_id IS NOT DISTINCT FROM OLD.classroom_id AND NEW.status IS NOT DISTINCT FROM OLD.status THEN
            RETURN NEW; -- e.g. idempotent upsert replay: nothing changed, nothing to log
        END IF;
        INSERT INTO class_enrollment_history (school_id, academic_year_id, student_user_id, from_classroom_id, to_classroom_id, from_status, to_status, action, reason, request_id, actor_user_id)
        VALUES (NEW.school_id, NEW.academic_year_id, NEW.student_user_id, OLD.classroom_id, NEW.classroom_id, OLD.status, NEW.status,
                COALESCE(v_action, CASE WHEN NEW.classroom_id IS DISTINCT FROM OLD.classroom_id THEN 'move' ELSE 'status_change' END),
                v_reason, v_request, v_actor);
        RETURN NEW;
    ELSE
        INSERT INTO class_enrollment_history (school_id, academic_year_id, student_user_id, from_classroom_id, to_classroom_id, from_status, to_status, action, reason, request_id, actor_user_id)
        VALUES (OLD.school_id, OLD.academic_year_id, OLD.student_user_id, OLD.classroom_id, NULL, OLD.status, NULL,
                COALESCE(v_action, 'remove'), v_reason, v_request, v_actor);
        RETURN OLD;
    END IF;
END;
$$ LANGUAGE plpgsql;

DROP TRIGGER IF EXISTS trg_class_enrollment_history ON class_enrollments;
CREATE TRIGGER trg_class_enrollment_history
    AFTER INSERT OR UPDATE OR DELETE ON class_enrollments
    FOR EACH ROW EXECUTE FUNCTION log_class_enrollment_change();

-- Seed one 'enroll' row per existing enrollment so every current student has
-- a starting point in their history.
INSERT INTO class_enrollment_history (school_id, academic_year_id, student_user_id, from_classroom_id, to_classroom_id, from_status, to_status, action, reason, created_at)
SELECT school_id, academic_year_id, student_user_id, NULL, classroom_id, NULL, status, 'enroll', 'Data awal (sebelum riwayat dicatat)', enrolled_at
FROM class_enrollments;
