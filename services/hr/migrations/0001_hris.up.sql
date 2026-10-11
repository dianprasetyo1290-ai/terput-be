-- Extension requirements
CREATE EXTENSION IF NOT EXISTS pgcrypto;
CREATE EXTENSION IF NOT EXISTS btree_gist;

-- Master PTKP Statuses (code only, no rates)
CREATE TABLE ptkp_statuses (
    code TEXT PRIMARY KEY,
    label TEXT NOT NULL
);

INSERT INTO ptkp_statuses (code, label) VALUES
    ('TK/0', 'Tidak Kawin tanpa Tanggungan'),
    ('TK/1', 'Tidak Kawin 1 Tanggungan'),
    ('TK/2', 'Tidak Kawin 2 Tanggungan'),
    ('TK/3', 'Tidak Kawin 3 Tanggungan'),
    ('K/0', 'Kawin tanpa Tanggungan'),
    ('K/1', 'Kawin 1 Tanggungan'),
    ('K/2', 'Kawin 2 Tanggungan'),
    ('K/3', 'Kawin 3 Tanggungan');

-- Organisasi (84)
CREATE TABLE departments (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    school_id UUID NOT NULL,
    code TEXT NOT NULL,
    name TEXT NOT NULL,
    parent_id UUID NULL REFERENCES departments(id) ON DELETE SET NULL,
    active BOOLEAN NOT NULL DEFAULT true,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    deleted_at TIMESTAMPTZ NULL
);
CREATE UNIQUE INDEX idx_departments_school_code ON departments(school_id, code) WHERE deleted_at IS NULL;
CREATE INDEX idx_departments_school ON departments(school_id);

CREATE TABLE positions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    school_id UUID NOT NULL,
    code TEXT NOT NULL,
    name TEXT NOT NULL,
    level INT NOT NULL DEFAULT 1,
    job_description TEXT NULL,
    department_id UUID NULL REFERENCES departments(id) ON DELETE SET NULL,
    salary_min_idr BIGINT NULL,
    salary_max_idr BIGINT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    deleted_at TIMESTAMPTZ NULL,
    CONSTRAINT chk_positions_salary_range CHECK (salary_min_idr IS NULL OR salary_max_idr IS NULL OR salary_max_idr >= salary_min_idr),
    CONSTRAINT uq_positions_school_code UNIQUE (school_id, code)
);
CREATE INDEX idx_positions_school ON positions(school_id);

CREATE TABLE headcount_plans (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    school_id UUID NOT NULL,
    position_id UUID NOT NULL REFERENCES positions(id) ON DELETE CASCADE,
    department_id UUID NULL REFERENCES departments(id) ON DELETE SET NULL,
    planned INT NOT NULL CHECK (planned >= 0),
    effective_from DATE NOT NULL,
    effective_to DATE NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_headcount_plans_school ON headcount_plans(school_id, position_id);

CREATE TABLE org_change_log (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    school_id UUID NOT NULL,
    entity_type TEXT NOT NULL,
    entity_id UUID NOT NULL,
    before JSONB NULL,
    after JSONB NULL,
    effective_date DATE NOT NULL,
    actor_user_id UUID NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_org_change_log_school ON org_change_log(school_id, created_at DESC);

-- Pegawai (83)
CREATE TABLE employees (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    school_id UUID NOT NULL,
    user_id UUID NULL UNIQUE,
    employee_no TEXT NOT NULL,
    full_name TEXT NOT NULL,
    email TEXT NULL,
    phone_enc BYTEA NULL,
    nik_enc BYTEA NULL,
    nik_bidx TEXT NULL,
    npwp_enc BYTEA NULL,
    npwp_bidx TEXT NULL,
    ptkp_status TEXT NOT NULL REFERENCES ptkp_statuses(code),
    bank_code TEXT NULL,
    bank_account_enc BYTEA NULL,
    bank_account_name TEXT NULL,
    birth_date DATE NULL,
    gender TEXT NULL CHECK (gender IN ('L', 'P')),
    address_enc BYTEA NULL,
    hire_date DATE NOT NULL,
    exit_date DATE NULL,
    exit_reason TEXT NULL,
    status TEXT NOT NULL CHECK (status IN ('candidate', 'active', 'exited')),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    deleted_at TIMESTAMPTZ NULL,
    CONSTRAINT uq_employees_school_empno UNIQUE (school_id, employee_no)
);
CREATE UNIQUE INDEX idx_employees_school_nik_bidx ON employees(school_id, nik_bidx) WHERE nik_bidx IS NOT NULL;
CREATE INDEX idx_employees_school_status ON employees(school_id, status);

CREATE TABLE employment_assignments (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    school_id UUID NOT NULL,
    employee_id UUID NOT NULL REFERENCES employees(id) ON DELETE CASCADE,
    position_id UUID NOT NULL REFERENCES positions(id),
    department_id UUID NULL REFERENCES departments(id) ON DELETE SET NULL,
    supervisor_employee_id UUID NULL REFERENCES employees(id) ON DELETE SET NULL,
    employment_type TEXT NOT NULL CHECK (employment_type IN ('tetap', 'kontrak', 'honorer', 'bukan_pegawai')),
    grade TEXT NULL,
    base_salary_enc BYTEA NULL,
    contract_end DATE NULL,
    effective_from DATE NOT NULL,
    effective_to DATE NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    EXCLUDE USING gist (employee_id WITH =, daterange(effective_from, effective_to, '[)') WITH &&)
);
CREATE INDEX idx_assignments_emp_eff ON employment_assignments(employee_id, effective_from DESC);

-- Dokumen (85)
CREATE TABLE document_types (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    school_id UUID NOT NULL,
    code TEXT NOT NULL,
    name TEXT NOT NULL,
    has_expiry BOOLEAN NOT NULL DEFAULT false,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    deleted_at TIMESTAMPTZ NULL,
    CONSTRAINT uq_document_types_school_code UNIQUE (school_id, code)
);

CREATE TABLE required_documents (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    school_id UUID NOT NULL,
    employment_type TEXT NOT NULL CHECK (employment_type IN ('tetap', 'kontrak', 'honorer', 'bukan_pegawai')),
    document_type_id UUID NOT NULL REFERENCES document_types(id) ON DELETE CASCADE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT uq_required_docs_school_emp_type UNIQUE (school_id, employment_type, document_type_id)
);

CREATE TABLE employee_documents (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    school_id UUID NOT NULL,
    employee_id UUID NOT NULL REFERENCES employees(id) ON DELETE CASCADE,
    document_type_id UUID NOT NULL REFERENCES document_types(id),
    version INT NOT NULL DEFAULT 1,
    object_key TEXT NOT NULL,
    file_name TEXT NOT NULL,
    mime TEXT NOT NULL,
    size_bytes BIGINT NOT NULL,
    expires_on DATE NULL,
    uploaded_by UUID NULL,
    requested_by_hr BOOLEAN NOT NULL DEFAULT false,
    superseded_at TIMESTAMPTZ NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT uq_emp_docs_version UNIQUE (employee_id, document_type_id, version)
);
CREATE INDEX idx_emp_docs_lookup ON employee_documents(employee_id, document_type_id, version DESC);

CREATE TABLE document_requests (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    school_id UUID NOT NULL,
    employee_id UUID NOT NULL REFERENCES employees(id) ON DELETE CASCADE,
    document_type_id UUID NOT NULL REFERENCES document_types(id),
    note TEXT NULL,
    status TEXT NOT NULL DEFAULT 'open' CHECK (status IN ('open', 'fulfilled', 'cancelled')),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_doc_reqs_emp_status ON document_requests(employee_id, status);

-- Absensi, Cuti, Lembur (86/87)
CREATE TABLE work_schedules (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    school_id UUID NOT NULL,
    name TEXT NOT NULL,
    position_id UUID NULL REFERENCES positions(id) ON DELETE SET NULL,
    department_id UUID NULL REFERENCES departments(id) ON DELETE SET NULL,
    days SMALLINT[] NOT NULL,
    start_time TIME NOT NULL,
    end_time TIME NOT NULL,
    late_tolerance_minutes INT NOT NULL DEFAULT 0,
    effective_from DATE NOT NULL,
    effective_to DATE NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_schedules_school ON work_schedules(school_id);

CREATE TABLE holidays (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    school_id UUID NOT NULL,
    date DATE NOT NULL,
    name TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT uq_holidays_school_date UNIQUE (school_id, date)
);

CREATE TABLE attendance_import_profiles (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    school_id UUID NOT NULL,
    name TEXT NOT NULL,
    column_map JSONB NOT NULL DEFAULT '{}',
    date_format TEXT NOT NULL DEFAULT 'YYYY-MM-DD',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE attendance_import_batches (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    school_id UUID NOT NULL,
    file_name TEXT NOT NULL,
    column_map JSONB NOT NULL DEFAULT '{}',
    total INT NOT NULL DEFAULT 0,
    ok INT NOT NULL DEFAULT 0,
    failed INT NOT NULL DEFAULT 0,
    errors JSONB NOT NULL DEFAULT '[]',
    created_by UUID NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE employee_attendance (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    school_id UUID NOT NULL,
    employee_id UUID NOT NULL REFERENCES employees(id) ON DELETE CASCADE,
    work_date DATE NOT NULL,
    status TEXT NOT NULL CHECK (status IN ('hadir', 'terlambat', 'izin', 'sakit', 'cuti', 'dinas_luar', 'alpa')),
    check_in_at TIMESTAMPTZ NULL,
    check_out_at TIMESTAMPTZ NULL,
    lat NUMERIC(9,6) NULL,
    lng NUMERIC(9,6) NULL,
    source TEXT NOT NULL CHECK (source IN ('ess', 'manual', 'import', 'leave')),
    import_batch_id UUID NULL REFERENCES attendance_import_batches(id) ON DELETE SET NULL,
    note TEXT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT uq_attendance_emp_date UNIQUE (employee_id, work_date)
);
CREATE INDEX idx_attendance_school_date ON employee_attendance(school_id, work_date);

CREATE TABLE leave_types (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    school_id UUID NOT NULL,
    code TEXT NOT NULL,
    name TEXT NOT NULL,
    has_quota BOOLEAN NOT NULL DEFAULT true,
    annual_quota_days NUMERIC(5,1) NULL,
    accrual TEXT NOT NULL DEFAULT 'none' CHECK (accrual IN ('none', 'monthly')),
    carry_over_max_days NUMERIC(5,1) NOT NULL DEFAULT 0,
    requires_document BOOLEAN NOT NULL DEFAULT false,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    deleted_at TIMESTAMPTZ NULL,
    CONSTRAINT uq_leave_types_school_code UNIQUE (school_id, code)
);

CREATE TABLE leave_balances (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    school_id UUID NOT NULL,
    employee_id UUID NOT NULL REFERENCES employees(id) ON DELETE CASCADE,
    leave_type_id UUID NOT NULL REFERENCES leave_types(id) ON DELETE CASCADE,
    year INT NOT NULL,
    entitled NUMERIC(5,1) NOT NULL DEFAULT 0,
    carried NUMERIC(5,1) NOT NULL DEFAULT 0,
    used NUMERIC(5,1) NOT NULL DEFAULT 0 CHECK (used >= 0),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT uq_leave_balances_emp_type_year UNIQUE (employee_id, leave_type_id, year)
);

CREATE TABLE leave_requests (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    school_id UUID NOT NULL,
    employee_id UUID NOT NULL REFERENCES employees(id) ON DELETE CASCADE,
    leave_type_id UUID NOT NULL REFERENCES leave_types(id),
    start_date DATE NOT NULL,
    end_date DATE NOT NULL,
    days NUMERIC(5,1) NOT NULL CHECK (days > 0),
    reason TEXT NULL,
    attachment_key TEXT NULL,
    status TEXT NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'approved', 'rejected', 'cancelled')),
    approval_id UUID NULL,
    idempotency_key TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT uq_leave_req_idempotency UNIQUE (school_id, idempotency_key)
);
CREATE INDEX idx_leave_requests_emp ON leave_requests(employee_id, status);

CREATE TABLE overtime_requests (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    school_id UUID NOT NULL,
    employee_id UUID NOT NULL REFERENCES employees(id) ON DELETE CASCADE,
    work_date DATE NOT NULL,
    start_at TIMESTAMPTZ NOT NULL,
    end_at TIMESTAMPTZ NOT NULL,
    minutes INT NOT NULL CHECK (minutes > 0),
    reason TEXT NOT NULL,
    status TEXT NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'approved', 'rejected', 'cancelled')),
    approval_id UUID NULL,
    idempotency_key TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT uq_overtime_req_idempotency UNIQUE (school_id, idempotency_key)
);

CREATE TABLE attendance_corrections (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    school_id UUID NOT NULL,
    employee_id UUID NOT NULL REFERENCES employees(id) ON DELETE CASCADE,
    work_date DATE NOT NULL,
    before JSONB NOT NULL,
    after JSONB NOT NULL,
    reason TEXT NOT NULL,
    status TEXT NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'approved', 'rejected', 'cancelled')),
    approval_id UUID NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE attendance_periods (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    school_id UUID NOT NULL,
    period CHAR(7) NOT NULL,
    status TEXT NOT NULL DEFAULT 'open' CHECK (status IN ('open', 'sent', 'locked')),
    sent_at TIMESTAMPTZ NULL,
    sent_by UUID NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT uq_attendance_periods_school_period UNIQUE (school_id, period)
);

-- ESS Profile Change Requests (87)
CREATE TABLE profile_change_requests (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    school_id UUID NOT NULL,
    employee_id UUID NOT NULL REFERENCES employees(id) ON DELETE CASCADE,
    field TEXT NOT NULL CHECK (field IN ('address', 'phone', 'bank_account', 'ptkp_status')),
    new_value_enc BYTEA NOT NULL,
    status TEXT NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'approved', 'rejected', 'cancelled')),
    approval_id UUID NULL,
    applied_at TIMESTAMPTZ NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Settings per School
CREATE TABLE hr_settings (
    school_id UUID PRIMARY KEY,
    contract_reminder_days INT NOT NULL DEFAULT 30,
    document_reminder_days INT NOT NULL DEFAULT 30,
    geolocation_enabled BOOLEAN NOT NULL DEFAULT false,
    geofence JSONB NULL,
    timezone TEXT NOT NULL DEFAULT 'Asia/Jakarta',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Leave accrual ledger: one row per granted slice makes the monthly/yearly
-- accrual job idempotent per (employee, type, year, month). month 0 = the
-- yearly grant of a non-accruing quota type.
CREATE TABLE leave_accruals (
    school_id UUID NOT NULL,
    employee_id UUID NOT NULL REFERENCES employees(id) ON DELETE CASCADE,
    leave_type_id UUID NOT NULL REFERENCES leave_types(id) ON DELETE CASCADE,
    year INT NOT NULL,
    month INT NOT NULL CHECK (month BETWEEN 0 AND 12),
    days NUMERIC(5,1) NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (employee_id, leave_type_id, year, month)
);

-- Audit outbox: written in the same transaction as the change, forwarded to
-- core POST /internal/audit by the audit-forward job. before/after never hold
-- plaintext NIK/NPWP/rekening/gaji, only {"changed": [...]} markers.
CREATE TABLE audit_events (
    id BIGSERIAL PRIMARY KEY,
    school_id UUID NOT NULL,
    module TEXT NOT NULL,
    entity_type TEXT NOT NULL,
    entity_id TEXT NOT NULL,
    action TEXT NOT NULL,
    actor_user_id UUID NULL,
    actor_role TEXT NULL,
    before JSONB NULL,
    after JSONB NULL,
    reason TEXT NULL,
    correlation_id TEXT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    forwarded_at TIMESTAMPTZ NULL
);
CREATE INDEX idx_audit_events_pending ON audit_events(id) WHERE forwarded_at IS NULL;
CREATE INDEX idx_audit_events_entity ON audit_events(school_id, entity_type, entity_id, created_at DESC);
