-- Fasilitas (menu 116–118): master ruang, booking, perintah kerja, perawatan
-- berkala. BR-FAC-02: satu-satunya tabel ruang di seluruh SEKOCI.

CREATE TABLE buildings (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    school_id UUID NOT NULL,
    code TEXT NOT NULL,
    name TEXT NOT NULL,
    address TEXT NULL,
    sort INT NOT NULL DEFAULT 0,
    deleted_at TIMESTAMPTZ NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX uq_buildings_code ON buildings(school_id, code) WHERE deleted_at IS NULL;

CREATE TABLE floors (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    school_id UUID NOT NULL,
    building_id UUID NOT NULL REFERENCES buildings(id),
    level INT NOT NULL,
    name TEXT NOT NULL,
    deleted_at TIMESTAMPTZ NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX uq_floors_level ON floors(building_id, level) WHERE deleted_at IS NULL;

CREATE TABLE rooms (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    school_id UUID NOT NULL,
    floor_id UUID NOT NULL REFERENCES floors(id),
    code TEXT NOT NULL,
    name TEXT NOT NULL,
    kind TEXT NOT NULL CHECK (kind IN ('kelas','lab','aula','lapangan','perpustakaan','ruang_serbaguna','kantin','kantor','uks','gudang','lainnya')),
    capacity INT NOT NULL CHECK (capacity >= 0),
    usage TEXT NOT NULL CHECK (usage IN ('bookable','fixed')),
    requires_approval BOOLEAN NOT NULL DEFAULT false,
    status TEXT NOT NULL DEFAULT 'active' CHECK (status IN ('active','maintenance','inactive')),
    available_from TIME NULL,
    available_to TIME NULL,
    notes TEXT NULL,
    deleted_at TIMESTAMPTZ NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CHECK (usage = 'bookable' OR requires_approval = false),
    CHECK ((available_from IS NULL) = (available_to IS NULL)),
    CHECK (available_from IS NULL OR available_to > available_from)
);
CREATE UNIQUE INDEX uq_rooms_code ON rooms(school_id, code) WHERE deleted_at IS NULL;
CREATE INDEX idx_rooms_school ON rooms(school_id, kind, status) WHERE deleted_at IS NULL;

CREATE TABLE room_facilities (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    school_id UUID NOT NULL,
    room_id UUID NOT NULL REFERENCES rooms(id),
    name TEXT NOT NULL,
    qty INT NOT NULL DEFAULT 1 CHECK (qty > 0),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_room_facilities_room ON room_facilities(room_id);

CREATE TABLE room_fixed_schedules (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    school_id UUID NOT NULL,
    room_id UUID NOT NULL REFERENCES rooms(id),
    weekday SMALLINT NOT NULL CHECK (weekday BETWEEN 1 AND 7),
    starts_time TIME NOT NULL,
    ends_time TIME NOT NULL,
    label TEXT NOT NULL,
    valid_from DATE NOT NULL,
    valid_to DATE NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CHECK (ends_time > starts_time),
    CHECK (valid_to IS NULL OR valid_to >= valid_from)
);
CREATE INDEX idx_room_fixed_schedules_room ON room_fixed_schedules(room_id);

CREATE TABLE room_bookings (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    school_id UUID NOT NULL,
    room_id UUID NOT NULL REFERENCES rooms(id),
    starts_at TIMESTAMPTZ NOT NULL,
    ends_at TIMESTAMPTZ NOT NULL,
    purpose TEXT NOT NULL,
    participants INT NOT NULL CHECK (participants > 0),
    requester_user_id UUID NOT NULL,
    requester_name TEXT NULL,
    source TEXT NOT NULL CHECK (source IN ('portal','internal')),
    source_service TEXT NULL,
    source_ref TEXT NULL,
    status TEXT NOT NULL CHECK (status IN ('pending_approval','confirmed','rejected','cancelled')),
    approval_id UUID NULL,
    cancel_reason TEXT NULL,
    cancelled_by UUID NULL,
    idempotency_key TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (school_id, idempotency_key),
    CHECK (ends_at > starts_at),
    -- BR-FAC-01 anti-race: dua booking aktif tidak boleh tumpang tindih.
    CONSTRAINT room_bookings_no_overlap EXCLUDE USING gist (
        room_id WITH =, tstzrange(starts_at, ends_at, '[)') WITH &&
    ) WHERE (status IN ('pending_approval','confirmed'))
);
CREATE INDEX idx_room_bookings_requester ON room_bookings(school_id, requester_user_id, starts_at DESC);
CREATE INDEX idx_room_bookings_room ON room_bookings(room_id, starts_at);

CREATE TABLE maintenance_schedules (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    school_id UUID NOT NULL,
    room_id UUID NULL REFERENCES rooms(id),
    asset_id UUID NULL,
    title TEXT NOT NULL,
    priority TEXT NOT NULL DEFAULT 'sedang' CHECK (priority IN ('rendah','sedang','tinggi','darurat')),
    interval_unit TEXT NOT NULL CHECK (interval_unit IN ('day','week','month')),
    interval_count INT NOT NULL CHECK (interval_count > 0),
    next_due_date DATE NOT NULL,
    default_assignee_user_id UUID NULL,
    default_assignee_name TEXT NULL,
    active BOOLEAN NOT NULL DEFAULT true,
    created_by UUID NOT NULL, -- pelapor WO preventif yang dibuat job
    deleted_at TIMESTAMPTZ NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CHECK (room_id IS NOT NULL OR asset_id IS NOT NULL)
);
CREATE INDEX idx_maintenance_due ON maintenance_schedules(next_due_date) WHERE active AND deleted_at IS NULL;

CREATE TABLE work_orders (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    school_id UUID NOT NULL,
    code TEXT NOT NULL,
    source TEXT NOT NULL CHECK (source IN ('report','preventive','ticket')),
    room_id UUID NULL REFERENCES rooms(id),
    asset_id UUID NULL,
    title TEXT NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    priority TEXT NOT NULL CHECK (priority IN ('rendah','sedang','tinggi','darurat')),
    status TEXT NOT NULL CHECK (status IN ('dilaporkan','ditugaskan','diproses','menunggu_material','selesai','ditutup','dibatalkan')),
    reporter_user_id UUID NOT NULL,
    reporter_name TEXT NULL,
    assignee_user_id UUID NULL,
    assignee_name TEXT NULL,
    completion_note TEXT NULL,
    completed_at TIMESTAMPTZ NULL,
    cancel_reason TEXT NULL,
    schedule_id UUID NULL REFERENCES maintenance_schedules(id),
    scheduled_for DATE NULL,
    ticket_ref TEXT NULL,
    callback_url TEXT NULL,
    purchase_request_ref TEXT NULL,
    idempotency_key TEXT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (school_id, code),
    UNIQUE (school_id, idempotency_key),
    UNIQUE (schedule_id, scheduled_for),
    CHECK (room_id IS NOT NULL OR asset_id IS NOT NULL)
);
CREATE INDEX idx_work_orders_school ON work_orders(school_id, status, created_at DESC);
CREATE INDEX idx_work_orders_assignee ON work_orders(school_id, assignee_user_id);
CREATE INDEX idx_work_orders_reporter ON work_orders(school_id, reporter_user_id);

CREATE TABLE work_order_photos (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    school_id UUID NOT NULL,
    work_order_id UUID NOT NULL REFERENCES work_orders(id),
    object_key TEXT NOT NULL,
    mime TEXT NOT NULL,
    size_bytes BIGINT NOT NULL,
    sha256 TEXT NOT NULL,
    uploaded_by UUID NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_work_order_photos_wo ON work_order_photos(work_order_id);

CREATE TABLE work_order_events (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    school_id UUID NOT NULL,
    work_order_id UUID NOT NULL REFERENCES work_orders(id),
    from_status TEXT NULL,
    to_status TEXT NOT NULL,
    note TEXT NULL,
    actor_user_id UUID NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_work_order_events_wo ON work_order_events(work_order_id, created_at);

CREATE TABLE work_order_costs (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    school_id UUID NOT NULL,
    work_order_id UUID NOT NULL REFERENCES work_orders(id),
    kind TEXT NOT NULL CHECK (kind IN ('material','jasa','lainnya')),
    description TEXT NOT NULL,
    amount_idr BIGINT NOT NULL CHECK (amount_idr >= 0),
    created_by UUID NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_work_order_costs_wo ON work_order_costs(work_order_id);
