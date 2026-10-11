-- 0010_payroll.up.sql: Master data, inputs, runs, lines, payslips, journals

CREATE TABLE IF NOT EXISTS pay_components (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    school_id UUID NOT NULL,
    code TEXT NOT NULL,
    name TEXT NOT NULL,
    kind TEXT NOT NULL CHECK(kind IN ('earning', 'deduction')),
    nature TEXT NOT NULL CHECK(nature IN ('fixed', 'variable')),
    taxable BOOLEAN NOT NULL DEFAULT true,
    bpjs_base BOOLEAN NOT NULL DEFAULT false,
    calc TEXT NOT NULL CHECK(calc IN ('fixed_amount', 'per_hour', 'from_structure', 'from_input', 'overtime')),
    default_amount_idr BIGINT NULL,
    rate_per_unit_idr BIGINT NULL,
    max_input_idr BIGINT NULL,
    sort INT NOT NULL DEFAULT 0,
    status TEXT NOT NULL DEFAULT 'draft' CHECK(status IN ('draft', 'pending_approval', 'active', 'retired')),
    approval_id UUID NULL,
    deleted_at TIMESTAMPTZ NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_component_school_code UNIQUE(school_id, code)
);

CREATE TABLE IF NOT EXISTS pay_component_accounts (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    school_id UUID NOT NULL,
    component_id UUID NOT NULL REFERENCES pay_components(id),
    acc_code TEXT NOT NULL,
    liability_acc_code TEXT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_comp_account UNIQUE(school_id, component_id)
);

CREATE TABLE IF NOT EXISTS payroll_accounts (
    school_id UUID PRIMARY KEY,
    salary_payable_acc TEXT NOT NULL DEFAULT '2101',
    pph21_payable_acc TEXT NOT NULL DEFAULT '2102',
    bpjs_payable_acc TEXT NOT NULL DEFAULT '2103',
    bpjs_employer_expense_acc TEXT NOT NULL DEFAULT '5102',
    bank_acc TEXT NOT NULL DEFAULT '1101',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS salary_structures (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    school_id UUID NOT NULL,
    position_id UUID NULL,
    grade TEXT NULL,
    employment_type TEXT NULL,
    base_min_idr BIGINT NOT NULL,
    base_max_idr BIGINT NOT NULL,
    effective_from DATE NOT NULL,
    effective_to DATE NULL,
    status TEXT NOT NULL DEFAULT 'active',
    approval_id UUID NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS employee_components (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    school_id UUID NOT NULL,
    employee_id UUID NOT NULL,
    component_id UUID NOT NULL REFERENCES pay_components(id),
    amount_idr BIGINT NOT NULL,
    effective_from DATE NOT NULL,
    effective_to DATE NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS statutory_bpjs (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    school_id UUID NULL,
    program TEXT NOT NULL CHECK(program IN ('kesehatan', 'jht', 'jp', 'jkk', 'jkm')),
    employer_bp INT NOT NULL,
    employee_bp INT NOT NULL,
    wage_cap_idr BIGINT NULL,
    effective_from DATE NOT NULL,
    effective_to DATE NULL,
    source_note TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS statutory_ptkp (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    status_code TEXT NOT NULL,
    annual_idr BIGINT NOT NULL,
    effective_from DATE NOT NULL,
    effective_to DATE NULL,
    source_note TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS statutory_ter_categories (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    category TEXT NOT NULL CHECK(category IN ('A', 'B', 'C')),
    ptkp_status_code TEXT NOT NULL,
    effective_from DATE NOT NULL,
    effective_to DATE NULL
);

CREATE TABLE IF NOT EXISTS statutory_ter_brackets (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    category TEXT NOT NULL CHECK(category IN ('A', 'B', 'C')),
    lower_idr BIGINT NOT NULL,
    upper_idr BIGINT NULL,
    rate_bp INT NOT NULL,
    effective_from DATE NOT NULL,
    effective_to DATE NULL
);

CREATE TABLE IF NOT EXISTS statutory_progressive (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    lower_idr BIGINT NOT NULL,
    upper_idr BIGINT NULL,
    rate_bp INT NOT NULL,
    effective_from DATE NOT NULL,
    effective_to DATE NULL
);

CREATE TABLE IF NOT EXISTS statutory_misc (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    key TEXT NOT NULL CHECK(key IN ('biaya_jabatan_bp', 'biaya_jabatan_max_annual_idr')),
    value BIGINT NOT NULL,
    effective_from DATE NOT NULL,
    effective_to DATE NULL
);

CREATE TABLE IF NOT EXISTS payroll_calendars (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    school_id UUID NOT NULL,
    period CHAR(7) NOT NULL,
    cutoff_date DATE NOT NULL,
    pay_date DATE NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_calendar_school_period UNIQUE(school_id, period)
);

CREATE TABLE IF NOT EXISTS payroll_inputs (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    school_id UUID NOT NULL,
    period CHAR(7) NOT NULL,
    employee_id UUID NOT NULL,
    component_id UUID NOT NULL REFERENCES pay_components(id),
    quantity NUMERIC(10,2) NULL,
    amount_idr BIGINT NOT NULL,
    source TEXT NOT NULL CHECK(source IN ('manual', 'import', 'attendance')),
    locked BOOLEAN NOT NULL DEFAULT false,
    created_by UUID NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_input_period_emp_comp UNIQUE(school_id, period, employee_id, component_id)
);

CREATE TABLE IF NOT EXISTS payroll_runs (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    school_id UUID NOT NULL,
    period CHAR(7) NOT NULL,
    kind TEXT NOT NULL CHECK(kind IN ('regular', 'supplementary')),
    status TEXT NOT NULL DEFAULT 'draft' CHECK(status IN ('draft', 'calculated', 'pending_approval', 'approved', 'final', 'rejected', 'cancelled')),
    approval_id UUID NULL,
    calculated_at TIMESTAMPTZ NULL,
    finalized_at TIMESTAMPTZ NULL,
    finalized_by UUID NULL,
    params_snapshot JSONB NOT NULL DEFAULT '{}'::jsonb,
    totals JSONB NOT NULL DEFAULT '{}'::jsonb,
    idempotency_key TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_run_school_idempotency UNIQUE(school_id, idempotency_key)
);

CREATE UNIQUE INDEX IF NOT EXISTS one_regular_run ON payroll_runs(school_id, period)
WHERE kind = 'regular' AND status NOT IN ('rejected', 'cancelled');

CREATE TABLE IF NOT EXISTS payroll_lines (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    school_id UUID NOT NULL,
    run_id UUID NOT NULL REFERENCES payroll_runs(id),
    employee_id UUID NOT NULL,
    gross_idr BIGINT NOT NULL,
    taxable_idr BIGINT NOT NULL,
    bpjs_base_idr BIGINT NOT NULL,
    bpjs_employee_idr BIGINT NOT NULL,
    bpjs_employer_idr BIGINT NOT NULL,
    pph21_idr BIGINT NOT NULL,
    other_deductions_idr BIGINT NOT NULL DEFAULT 0,
    net_idr BIGINT NOT NULL,
    prev_net_idr BIGINT NULL,
    variance_bp INT NULL,
    flagged BOOLEAN NOT NULL DEFAULT false,
    details JSONB NOT NULL DEFAULT '[]'::jsonb,
    bank_snapshot_enc BYTEA NULL,
    payment_status TEXT NOT NULL DEFAULT 'unpaid' CHECK(payment_status IN ('unpaid', 'paid', 'failed')),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_lines_run_emp UNIQUE(run_id, employee_id)
);

CREATE TABLE IF NOT EXISTS payslips (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    school_id UUID NOT NULL,
    line_id UUID NOT NULL REFERENCES payroll_lines(id),
    object_key TEXT NOT NULL,
    sha256 TEXT NOT NULL,
    published_at TIMESTAMPTZ NULL,
    reissued_count INT NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_payslip_line UNIQUE(line_id)
);

CREATE TABLE IF NOT EXISTS tax_remittances (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    school_id UUID NOT NULL,
    period CHAR(7) NOT NULL,
    kind TEXT NOT NULL CHECK(kind IN ('pph21', 'bpjs_kesehatan', 'bpjs_tk')),
    amount_idr BIGINT NOT NULL,
    paid_on DATE NOT NULL,
    ref TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS payroll_journals (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    school_id UUID NOT NULL,
    run_id UUID NOT NULL REFERENCES payroll_runs(id),
    kind TEXT NOT NULL CHECK(kind IN ('accrual', 'payment')),
    reference TEXT NOT NULL UNIQUE,
    status TEXT NOT NULL DEFAULT 'pending' CHECK(status IN ('pending', 'posted', 'failed')),
    trans_no TEXT NULL,
    lines JSONB NOT NULL DEFAULT '[]'::jsonb,
    error TEXT NULL,
    attempts INT NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
