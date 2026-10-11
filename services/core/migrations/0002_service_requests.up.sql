-- Migration 0002_service_requests.up.sql: Parent Service Requests (Menu 81)

CREATE TABLE IF NOT EXISTS service_request_categories (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    school_id UUID NOT NULL,
    name TEXT NOT NULL,
    description TEXT,
    handler_role TEXT NOT NULL,
    sla_hours INT NULL,
    requires_student BOOLEAN NOT NULL DEFAULT true,
    active BOOLEAN NOT NULL DEFAULT true,
    deleted_at TIMESTAMPTZ NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_service_request_categories_school ON service_request_categories (school_id);
CREATE INDEX IF NOT EXISTS idx_service_request_categories_active ON service_request_categories (school_id, active) WHERE deleted_at IS NULL;

CREATE TABLE IF NOT EXISTS service_requests (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    school_id UUID NOT NULL,
    category_id UUID NOT NULL REFERENCES service_request_categories(id),
    requester_user_id UUID NOT NULL,
    student_user_id UUID NULL,
    subject TEXT NOT NULL,
    description TEXT NOT NULL,
    attachments JSONB NOT NULL DEFAULT '[]',
    status TEXT NOT NULL CHECK (status IN ('submitted','accepted','in_progress','done','rejected')),
    assignee_user_id UUID NULL,
    result_attachments JSONB NOT NULL DEFAULT '[]',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_service_requests_school_status ON service_requests (school_id, status);
CREATE INDEX IF NOT EXISTS idx_service_requests_requester ON service_requests (school_id, requester_user_id);
CREATE INDEX IF NOT EXISTS idx_service_requests_student ON service_requests (school_id, student_user_id);

CREATE TABLE IF NOT EXISTS service_request_events (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    school_id UUID NOT NULL,
    request_id UUID NOT NULL REFERENCES service_requests(id) ON DELETE CASCADE,
    from_status TEXT NULL,
    to_status TEXT NOT NULL,
    note TEXT,
    actor_user_id UUID NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_service_request_events_req ON service_request_events (request_id, created_at);
