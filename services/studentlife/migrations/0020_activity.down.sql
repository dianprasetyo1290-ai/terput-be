-- 0020_activity.down.sql
-- Rollback T21 Activity domain

DROP TABLE IF EXISTS act_achievements;
DROP TABLE IF EXISTS act_ecc_assessments;
DROP TABLE IF EXISTS act_ecc_attendance;
DROP TABLE IF EXISTS act_ecc_sessions;
DROP TABLE IF EXISTS act_ecc_members;
DROP TABLE IF EXISTS act_ecc_coaches;
DROP TABLE IF EXISTS act_extracurriculars;
DROP TABLE IF EXISTS act_ecc_periods;
DROP TABLE IF EXISTS act_event_registrations;
DROP TABLE IF EXISTS act_event_committee;
DROP TABLE IF EXISTS act_events;
DROP TABLE IF EXISTS act_event_kinds;
