-- 0060: Campus 3D (T25, menu 155-158, BR-3D-01)
-- Models, GLB mesh nodes, and ID references only.
-- NO operational data (room names, capacity, status, bookings, work orders, prices) stored here.

CREATE TABLE campus3d_models (
    id           uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    school_id    uuid NOT NULL,
    building_id  uuid NOT NULL REFERENCES buildings(id) ON DELETE CASCADE,
    version      int NOT NULL,
    object_key   text NOT NULL,
    mime         text NOT NULL CHECK (mime IN ('model/gltf-binary')),
    size_bytes   bigint NOT NULL,
    sha256       text NOT NULL,
    node_names   text[] NOT NULL DEFAULT '{}',
    status       text NOT NULL CHECK (status IN ('draft','published','archived')),
    uploaded_by  uuid NOT NULL,
    published_by uuid NULL,
    published_at timestamptz NULL,
    created_at   timestamptz NOT NULL DEFAULT now(),
    updated_at   timestamptz NOT NULL DEFAULT now(),
    UNIQUE (building_id, version)
);

CREATE UNIQUE INDEX campus3d_one_published ON campus3d_models (building_id) WHERE status = 'published';
CREATE INDEX campus3d_models_school_idx ON campus3d_models (school_id, building_id);

CREATE TABLE campus3d_floor_nodes (
    id         uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    school_id  uuid NOT NULL,
    model_id   uuid NOT NULL REFERENCES campus3d_models(id) ON DELETE CASCADE,
    floor_id   uuid NOT NULL REFERENCES floors(id) ON DELETE CASCADE,
    node_name  text NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (model_id, floor_id),
    UNIQUE (model_id, node_name)
);

CREATE TABLE campus3d_room_nodes (
    id         uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    school_id  uuid NOT NULL,
    model_id   uuid NOT NULL REFERENCES campus3d_models(id) ON DELETE CASCADE,
    room_id    uuid NOT NULL REFERENCES rooms(id) ON DELETE CASCADE,
    node_name  text NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (model_id, room_id),
    UNIQUE (model_id, node_name)
);

CREATE TABLE campus3d_tenant_spots (
    id         uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    school_id  uuid NOT NULL,
    tenant_id  uuid NOT NULL,
    room_id    uuid NOT NULL REFERENCES rooms(id) ON DELETE CASCADE,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (school_id, tenant_id)
);

CREATE INDEX campus3d_tenant_spots_room_idx ON campus3d_tenant_spots (school_id, room_id);
