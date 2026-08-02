ALTER TABLE schedules DROP CONSTRAINT schedules_period_subject_key;
ALTER TABLE schedules ADD CONSTRAINT schedules_schedule_period_id_key UNIQUE (schedule_period_id);
