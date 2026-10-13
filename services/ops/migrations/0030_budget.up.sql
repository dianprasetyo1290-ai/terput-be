-- 0030_budget.up.sql: Anggaran / RAPBS & Realisasi (Menu 109)
-- Pengaturan anggaran, RAPBS tahunan, pos anggaran, cache realisasi buku besar, dan notifikasi serapan.

CREATE TABLE budget_settings (
    school_id UUID PRIMARY KEY,
    fiscal_year_start_month INT NOT NULL DEFAULT 7 CHECK (fiscal_year_start_month BETWEEN 1 AND 12),
    lock_rule TEXT NOT NULL DEFAULT 'warn' CHECK (lock_rule IN ('warn', 'hard_stop', 'approval_required')),
    tolerance_bp INT NOT NULL DEFAULT 0 CHECK (tolerance_bp >= 0),
    warn_threshold_bp INT NOT NULL DEFAULT 8000 CHECK (warn_threshold_bp >= 0),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE budgets (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    school_id UUID NOT NULL,
    code TEXT NOT NULL,
    name TEXT NOT NULL,
    fiscal_year TEXT NOT NULL,
    period_start DATE NOT NULL,
    period_end DATE NOT NULL,
    status TEXT NOT NULL DEFAULT 'draft' CHECK (status IN ('draft', 'pending_approval', 'approved', 'revised', 'closed')),
    total_planned_idr BIGINT NOT NULL DEFAULT 0 CHECK (total_planned_idr >= 0),
    approval_id UUID NULL,
    created_by UUID NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    deleted_at TIMESTAMPTZ NULL,
    CONSTRAINT uq_budgets_school_code UNIQUE (school_id, code)
);
CREATE INDEX idx_budgets_school_status ON budgets(school_id, status);

CREATE TABLE budget_lines (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    school_id UUID NOT NULL,
    budget_id UUID NOT NULL REFERENCES budgets(id) ON DELETE CASCADE,
    code TEXT NOT NULL,
    name TEXT NOT NULL,
    category TEXT NOT NULL CHECK (category IN ('operasional', 'investasi', 'personalia', 'pemeliharaan', 'lainnya')),
    acc_code TEXT NOT NULL,
    div_code TEXT NULL,
    amount_planned_idr BIGINT NOT NULL CHECK (amount_planned_idr >= 0),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT uq_budget_lines_budget_code UNIQUE (budget_id, code)
);
CREATE INDEX idx_budget_lines_budget ON budget_lines(budget_id);
CREATE INDEX idx_budget_lines_acc ON budget_lines(school_id, acc_code);

CREATE TABLE budget_actuals_cache (
    budget_line_id UUID PRIMARY KEY REFERENCES budget_lines(id) ON DELETE CASCADE,
    school_id UUID NOT NULL,
    actual_idr BIGINT NOT NULL DEFAULT 0 CHECK (actual_idr >= 0),
    last_synced_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE budget_alerts (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    school_id UUID NOT NULL,
    budget_line_id UUID NOT NULL REFERENCES budget_lines(id) ON DELETE CASCADE,
    threshold_bp INT NOT NULL,
    current_bp INT NOT NULL,
    message TEXT NOT NULL,
    acknowledged_at TIMESTAMPTZ NULL,
    acknowledged_by UUID NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_budget_alerts_school ON budget_alerts(school_id, acknowledged_at);
