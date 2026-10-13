-- 0050_transport.up.sql: Transportasi (T19)

CREATE TABLE IF NOT EXISTS transport_settings (
    school_id uuid PRIMARY KEY,
    prorata_policy text NOT NULL DEFAULT 'daily' CHECK (prorata_policy IN ('daily', 'full_month')),
    doc_reminder_days int NOT NULL DEFAULT 30 CHECK (doc_reminder_days > 0),
    notify_parent_on_trip bool NOT NULL DEFAULT true,
    component_code text NOT NULL DEFAULT 'TRANSPORT',
    bill_due_day int NULL CHECK (bill_due_day BETWEEN 1 AND 31),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS transport_zones (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    school_id uuid NOT NULL,
    name text NOT NULL,
    deleted_at timestamptz NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX IF NOT EXISTS transport_zones_school_name_idx ON transport_zones(school_id, name) WHERE deleted_at IS NULL;

CREATE TABLE IF NOT EXISTS transport_routes (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    school_id uuid NOT NULL,
    code text NOT NULL,
    name text NOT NULL,
    status text NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'inactive')),
    deleted_at timestamptz NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX IF NOT EXISTS transport_routes_school_code_idx ON transport_routes(school_id, code) WHERE deleted_at IS NULL;

CREATE TABLE IF NOT EXISTS transport_stops (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    school_id uuid NOT NULL,
    route_id uuid NOT NULL REFERENCES transport_routes(id) ON DELETE CASCADE,
    seq int NOT NULL CHECK (seq > 0),
    name text NOT NULL,
    address text NULL,
    zone_id uuid NULL REFERENCES transport_zones(id) ON DELETE SET NULL,
    pickup_time time NULL,
    dropoff_time time NULL,
    deleted_at timestamptz NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX IF NOT EXISTS transport_stops_route_seq_idx ON transport_stops(route_id, seq) WHERE deleted_at IS NULL;

CREATE TABLE IF NOT EXISTS transport_vehicles (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    school_id uuid NOT NULL,
    plate_no text NOT NULL,
    name text NOT NULL,
    capacity int NOT NULL CHECK (capacity > 0),
    status text NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'maintenance', 'inactive')),
    deleted_at timestamptz NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX IF NOT EXISTS transport_vehicles_school_plate_idx ON transport_vehicles(school_id, plate_no) WHERE deleted_at IS NULL;

CREATE TABLE IF NOT EXISTS transport_vehicle_documents (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    school_id uuid NOT NULL,
    vehicle_id uuid NOT NULL REFERENCES transport_vehicles(id) ON DELETE CASCADE,
    kind text NOT NULL CHECK (kind IN ('stnk', 'uji_kir', 'asuransi', 'lainnya')),
    number text NOT NULL,
    valid_until date NOT NULL,
    object_key text NULL,
    deleted_at timestamptz NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS transport_crew (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    school_id uuid NOT NULL,
    kind text NOT NULL CHECK (kind IN ('driver', 'attendant')),
    employee_id uuid NULL,
    user_id uuid NULL,
    external_name text NULL,
    phone text NULL,
    license_no text NULL,
    license_valid_until date NULL,
    deleted_at timestamptz NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    CHECK (employee_id IS NOT NULL OR external_name IS NOT NULL)
);

CREATE TABLE IF NOT EXISTS transport_route_assignments (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    school_id uuid NOT NULL,
    route_id uuid NOT NULL REFERENCES transport_routes(id) ON DELETE CASCADE,
    vehicle_id uuid NOT NULL REFERENCES transport_vehicles(id),
    driver_crew_id uuid NOT NULL REFERENCES transport_crew(id),
    attendant_crew_id uuid NULL REFERENCES transport_crew(id),
    valid_from date NOT NULL,
    valid_to date NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    CHECK (valid_to IS NULL OR valid_to >= valid_from)
);

CREATE TABLE IF NOT EXISTS transport_fares (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    school_id uuid NOT NULL,
    zone_id uuid NULL REFERENCES transport_zones(id) ON DELETE CASCADE,
    route_id uuid NULL REFERENCES transport_routes(id) ON DELETE CASCADE,
    monthly_amount_idr bigint NOT NULL CHECK (monthly_amount_idr >= 0),
    effective_from date NOT NULL,
    effective_to date NULL,
    deleted_at timestamptz NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    CHECK ((zone_id IS NULL) <> (route_id IS NULL))
);

CREATE TABLE IF NOT EXISTS transport_allocations (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    school_id uuid NOT NULL,
    student_user_id uuid NOT NULL,
    route_id uuid NOT NULL REFERENCES transport_routes(id),
    stop_id uuid NOT NULL REFERENCES transport_stops(id),
    start_date date NOT NULL,
    end_date date NULL,
    end_reason text NULL,
    created_by uuid NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    CHECK (end_date IS NULL OR end_date >= start_date)
);
CREATE UNIQUE INDEX IF NOT EXISTS transport_allocations_one_open ON transport_allocations(school_id, student_user_id) WHERE end_date IS NULL;

CREATE TABLE IF NOT EXISTS transport_trips (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    school_id uuid NOT NULL,
    route_id uuid NOT NULL REFERENCES transport_routes(id),
    trip_date date NOT NULL,
    direction text NOT NULL CHECK (direction IN ('jemput', 'antar')),
    vehicle_id uuid NOT NULL REFERENCES transport_vehicles(id),
    driver_crew_id uuid NOT NULL REFERENCES transport_crew(id),
    attendant_crew_id uuid NULL REFERENCES transport_crew(id),
    status text NOT NULL DEFAULT 'planned' CHECK (status IN ('planned', 'running', 'done')),
    started_at timestamptz NULL,
    finished_at timestamptz NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (route_id, trip_date, direction)
);

CREATE TABLE IF NOT EXISTS transport_trip_events (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    school_id uuid NOT NULL,
    trip_id uuid NOT NULL REFERENCES transport_trips(id) ON DELETE CASCADE,
    student_user_id uuid NOT NULL,
    stop_id uuid NULL REFERENCES transport_stops(id),
    event text NOT NULL CHECK (event IN ('board', 'alight', 'absent')),
    source text NOT NULL DEFAULT 'scan' CHECK (source IN ('scan', 'manual')),
    recorded_by uuid NOT NULL,
    recorded_at timestamptz NOT NULL DEFAULT now(),
    client_event_id text NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (trip_id, student_user_id, event),
    UNIQUE (school_id, client_event_id)
);

CREATE TABLE IF NOT EXISTS transport_absence_notices (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    school_id uuid NOT NULL,
    student_user_id uuid NOT NULL,
    notice_date date NOT NULL,
    direction text NOT NULL CHECK (direction IN ('jemput', 'antar', 'both')),
    reason text NULL,
    created_by uuid NOT NULL,
    cancelled_at timestamptz NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX IF NOT EXISTS transport_absence_notices_open ON transport_absence_notices(student_user_id, notice_date, direction) WHERE cancelled_at IS NULL;

CREATE TABLE IF NOT EXISTS transport_charges (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    school_id uuid NOT NULL,
    student_user_id uuid NOT NULL,
    period_year int NOT NULL,
    period_month int NOT NULL CHECK (period_month BETWEEN 1 AND 12),
    items jsonb NOT NULL,
    amount_idr bigint NOT NULL CHECK (amount_idr >= 0),
    version int NOT NULL DEFAULT 1,
    bill_id uuid NULL,
    status text NOT NULL CHECK (status IN ('issued', 'adjusted', 'void', 'failed')),
    last_error text NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (school_id, student_user_id, period_year, period_month)
);
