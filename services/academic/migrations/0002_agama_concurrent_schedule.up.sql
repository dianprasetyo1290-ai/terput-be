-- Jadwal Pelajaran: allow more than one subject in the same classroom+day+
-- period slot, but ONLY for Agama variants (a classroom physically holds
-- students of different religions who split into parallel Agama sessions at
-- the same hour; every other subject must still be exclusive per slot — a
-- school must not be able to double-book Matematika at the same time).
--
-- The old `UNIQUE (schedule_period_id)` constraint made a period slot hold
-- at most one (subject, guru) pairing, full stop. Replaced with
-- `UNIQUE (schedule_period_id, classroom_subject_id)` (still blocks a literal
-- duplicate insert of the same subject into the same slot); the "only Agama
-- variants may share a slot, nothing else may" rule is enforced in
-- internal/handler/schedule.go (checkScheduleSlotConflict), reusing the same
-- case-insensitive Agama substring match as classroom_subject.go's
-- normalizeGroup — kept in Go rather than a DB CHECK because "is this
-- subject an Agama variant" depends on a join to subjects.subject_group, not
-- data available to a single-table constraint.
ALTER TABLE schedules DROP CONSTRAINT schedules_schedule_period_id_key;
ALTER TABLE schedules ADD CONSTRAINT schedules_period_subject_key UNIQUE (schedule_period_id, classroom_subject_id);
