-- Migration 0030_training.down.sql: Revert Training & Onboarding Schema

DROP TABLE IF EXISTS onboarding_tasks CASCADE;
DROP TABLE IF EXISTS onboarding_plans CASCADE;
DROP TABLE IF EXISTS onboarding_template_tasks CASCADE;
DROP TABLE IF EXISTS onboarding_templates CASCADE;
DROP TABLE IF EXISTS quiz_attempts CASCADE;
DROP TABLE IF EXISTS material_progress CASCADE;
DROP TABLE IF EXISTS training_assignments CASCADE;
DROP TABLE IF EXISTS mandatory_trainings CASCADE;
DROP TABLE IF EXISTS quiz_questions CASCADE;
DROP TABLE IF EXISTS course_sessions CASCADE;
DROP TABLE IF EXISTS course_materials CASCADE;
DROP TABLE IF EXISTS course_competencies CASCADE;
DROP TABLE IF EXISTS courses CASCADE;
DROP TABLE IF EXISTS competencies CASCADE;
DROP TABLE IF EXISTS training_categories CASCADE;
