-- Migration 0003_service_desk.down.sql: hanya tabel sd_* (tabel 0002 tidak disentuh)
DROP TABLE IF EXISTS sd_ticket_events;
DROP TABLE IF EXISTS sd_tickets;
DROP TABLE IF EXISTS sd_ticket_sequences;
DROP TABLE IF EXISTS sd_escalation_rules;
DROP TABLE IF EXISTS sd_sla_policies;
DROP TABLE IF EXISTS sd_holidays;
DROP TABLE IF EXISTS sd_business_hours;
DROP TABLE IF EXISTS sd_categories;
DROP TABLE IF EXISTS sd_team_members;
DROP TABLE IF EXISTS sd_teams;
