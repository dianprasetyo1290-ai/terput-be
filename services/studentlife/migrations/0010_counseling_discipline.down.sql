-- 0010_counseling_discipline.down.sql
DROP TABLE IF EXISTS discipline_appeals CASCADE;
DROP TABLE IF EXISTS discipline_actions CASCADE;
DROP TABLE IF EXISTS discipline_records CASCADE;
DROP TABLE IF EXISTS discipline_settings CASCADE;
DROP TABLE IF EXISTS discipline_thresholds CASCADE;
DROP TABLE IF EXISTS discipline_types CASCADE;

DROP TABLE IF EXISTS case_tasks CASCADE;
DROP TABLE IF EXISTS referrals CASCADE;
DROP TABLE IF EXISTS counseling_session_students CASCADE;
DROP TABLE IF EXISTS counseling_sessions CASCADE;
DROP TABLE IF EXISTS counseling_case_members CASCADE;
DROP TABLE IF EXISTS counseling_cases CASCADE;
DROP TABLE IF EXISTS counseling_settings CASCADE;
DROP TABLE IF EXISTS counseling_categories CASCADE;
