-- Migration 0002_service_requests.down.sql: Revert Parent Service Requests

DROP TABLE IF EXISTS service_request_events CASCADE;
DROP TABLE IF EXISTS service_requests CASCADE;
DROP TABLE IF EXISTS service_request_categories CASCADE;
