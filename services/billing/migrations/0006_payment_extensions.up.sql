-- 0006: payment extensions (T14, menu 150-151): several payment providers
-- with per-method routing and failover, per-provider reconciliation,
-- autodebit mandates and retail-outlet payment codes. Every change to the
-- T6 tables is additive and backfilled with 'nicepay', so a school that
-- never configures anything behaves exactly as before.

-- Providers installed for this client (FR-150-001). Credentials are NOT
-- stored here: credential_ref is the env / secret-manager prefix
-- (e.g. 'NICEPAY' -> NICEPAY_*).
CREATE TABLE payment_providers (
    code                 text PRIMARY KEY CHECK (code ~ '^[a-z][a-z0-9_]{1,31}$'),
    adapter              text NOT NULL CHECK (adapter IN ('nicepay','sim')),
    label                text NOT NULL,
    enabled              boolean NOT NULL DEFAULT false,
    credential_ref       text NULL CHECK (credential_ref IS NULL OR credential_ref ~ '^[A-Z][A-Z0-9_]{1,31}$'),
    consecutive_failures integer NOT NULL DEFAULT 0 CHECK (consecutive_failures >= 0),
    last_failure_at      timestamptz NULL,
    created_at           timestamptz NOT NULL DEFAULT now(),
    updated_at           timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE payment_provider_capabilities (
    provider_code text NOT NULL REFERENCES payment_providers(code) ON DELETE CASCADE,
    capability    text NOT NULL CHECK (capability IN ('va','qris','ewallet','retail','autodebit')),
    PRIMARY KEY (provider_code, capability)
);

-- NICEPay is the provider T6 already used: enabled, same credentials.
-- Retail and autodebit are not seeded for it: the NICEPay SNAP client in
-- this repo has no such API (OQ-41). The sim adapter exists for dev/tests
-- only (refused in production) and starts disabled.
INSERT INTO payment_providers (code, adapter, label, enabled, credential_ref)
VALUES ('nicepay', 'nicepay', 'NICEPay', true, 'NICEPAY'),
       ('sim', 'sim', 'Simulator (non-produksi)', false, NULL);
INSERT INTO payment_provider_capabilities (provider_code, capability)
VALUES ('nicepay','va'), ('nicepay','qris'), ('nicepay','ewallet'),
       ('sim','va'), ('sim','qris'), ('sim','ewallet'), ('sim','retail'), ('sim','autodebit');

-- Retail outlet methods (FR-151-004) and the method a school charges
-- autodebit fees with (BR-PAY-02 per method).
ALTER TABLE payment_methods DROP CONSTRAINT payment_methods_kind_check;
ALTER TABLE payment_methods ADD CONSTRAINT payment_methods_kind_check
    CHECK (kind IN ('va','qris','ewallet','retail','autodebit'));

-- Provider-specific method code (bankCd / mitraCd / outlet code). The old
-- payment_methods.provider_code column stays and is the NICEPay code.
CREATE TABLE payment_method_provider_codes (
    method_id            uuid NOT NULL REFERENCES payment_methods(id) ON DELETE CASCADE,
    provider_code        text NOT NULL REFERENCES payment_providers(code),
    provider_method_code text NOT NULL,
    PRIMARY KEY (method_id, provider_code)
);
INSERT INTO payment_method_provider_codes (method_id, provider_code, provider_method_code)
SELECT id, 'nicepay', provider_code FROM payment_methods;

-- Routing per method kind / method and school (FR-150-002/003, BR-PAY-03).
CREATE TABLE payment_routing_rules (
    id                uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    school_id         uuid NOT NULL,
    method_kind       text NOT NULL CHECK (method_kind IN ('va','qris','ewallet','retail')),
    method_code       text NULL,
    primary_provider  text NOT NULL REFERENCES payment_providers(code),
    fallback_provider text NULL REFERENCES payment_providers(code),
    failover_enabled  boolean NOT NULL DEFAULT false,
    active            boolean NOT NULL DEFAULT true,
    created_by        uuid NULL,
    created_at        timestamptz NOT NULL DEFAULT now(),
    updated_at        timestamptz NOT NULL DEFAULT now(),
    CHECK (fallback_provider IS NULL OR fallback_provider <> primary_provider)
);
CREATE UNIQUE INDEX payment_routing_rules_active_uq
    ON payment_routing_rules (school_id, method_kind, COALESCE(method_code, '')) WHERE active;

-- T6 transactions: provider, failover marker, purpose (contract K1/K2 with
-- T12), autodebit mandate and retail pay code.
ALTER TABLE gateway_transactions
    ADD COLUMN provider      text NOT NULL DEFAULT 'nicepay',
    ADD COLUMN failover_from text NULL,
    ADD COLUMN purpose       text NOT NULL DEFAULT 'bills' CHECK (purpose IN ('bills','wallet_topup')),
    ADD COLUMN purpose_ref   uuid NULL,
    ADD COLUMN mandate_id    uuid NULL,
    ADD COLUMN pay_code      text NULL,
    ADD COLUMN retail_outlet text NULL,
    ADD CONSTRAINT gateway_transactions_purpose_ref_required CHECK (purpose = 'bills' OR purpose_ref IS NOT NULL);
ALTER TABLE gateway_transactions DROP CONSTRAINT gateway_transactions_provider_ref_key;
ALTER TABLE gateway_transactions ADD CONSTRAINT gateway_transactions_provider_ref_key UNIQUE (provider, provider_ref);
CREATE INDEX gateway_transactions_retail_pending_idx ON gateway_transactions (expires_at)
    WHERE status = 'pending' AND pay_code IS NOT NULL;

ALTER TABLE gateway_notifications ADD COLUMN provider text NOT NULL DEFAULT 'nicepay';
ALTER TABLE gateway_notifications DROP CONSTRAINT gateway_notifications_provider_ref_raw_hash_key;
ALTER TABLE gateway_notifications ADD CONSTRAINT gateway_notifications_provider_ref_raw_hash_key
    UNIQUE (provider, provider_ref, raw_hash);

-- Settlement batches were imported as 'NICEPay' by T6; normalise to the
-- provider code so reconciliation can group by provider (FR-150-004).
UPDATE settlement_batches b SET provider = 'nicepay'
 WHERE lower(b.provider) = 'nicepay' AND b.provider <> 'nicepay'
   AND NOT EXISTS (SELECT 1 FROM settlement_batches o
                    WHERE o.school_id = b.school_id AND o.provider = 'nicepay'
                      AND o.channel = b.channel AND o.settled_on = b.settled_on);

ALTER TABLE reconciliation_exceptions ADD COLUMN provider text NOT NULL DEFAULT 'nicepay';
ALTER TABLE reconciliation_exceptions DROP CONSTRAINT reconciliation_exceptions_school_id_kind_ref_key;
ALTER TABLE reconciliation_exceptions ADD CONSTRAINT reconciliation_exceptions_school_id_kind_ref_key
    UNIQUE (school_id, provider, kind, ref);

-- Autodebit (FR-151-001..003, BR-PAY-04). retry_interval_days and run_hour
-- have no default: a school must set them before mandates can be created.
CREATE TABLE autodebit_settings (
    school_id           uuid PRIMARY KEY,
    max_retries         integer NOT NULL DEFAULT 2 CHECK (max_retries >= 0),
    retry_interval_days integer NULL CHECK (retry_interval_days IS NULL OR retry_interval_days > 0),
    run_hour            smallint NULL CHECK (run_hour IS NULL OR run_hour BETWEEN 0 AND 23),
    updated_by          uuid NULL,
    created_at          timestamptz NOT NULL DEFAULT now(),
    updated_at          timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE autodebit_mandates (
    id                   uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    school_id            uuid NOT NULL,
    guardian_user_id     uuid NOT NULL,
    student_user_id      uuid NOT NULL,
    provider             text NOT NULL REFERENCES payment_providers(code),
    provider_mandate_ref text NULL,
    account_masked       text NULL,
    status               text NOT NULL DEFAULT 'pending_activation'
                         CHECK (status IN ('pending_activation','active','cancelled','failed')),
    max_amount_idr       bigint NULL CHECK (max_amount_idr IS NULL OR max_amount_idr > 0),
    redirect_url         text NULL,
    activated_at         timestamptz NULL,
    cancelled_at         timestamptz NULL,
    cancelled_by         uuid NULL,
    cancel_reason        text NULL,
    idempotency_key      text NOT NULL,
    created_at           timestamptz NOT NULL DEFAULT now(),
    updated_at           timestamptz NOT NULL DEFAULT now(),
    UNIQUE (school_id, idempotency_key),
    CHECK (status <> 'cancelled' OR cancelled_at IS NOT NULL)
);
CREATE INDEX autodebit_mandates_guardian_idx ON autodebit_mandates (school_id, guardian_user_id);
CREATE INDEX autodebit_mandates_status_idx ON autodebit_mandates (school_id, status);
CREATE UNIQUE INDEX autodebit_mandates_provider_ref_uq ON autodebit_mandates (provider, provider_mandate_ref)
    WHERE provider_mandate_ref IS NOT NULL;

-- "Jenis tagihan" = billing scheme. Ad-hoc bills (no scheme) are never debited.
CREATE TABLE autodebit_mandate_schemes (
    mandate_id uuid NOT NULL REFERENCES autodebit_mandates(id) ON DELETE CASCADE,
    scheme_id  uuid NOT NULL REFERENCES billing_schemes(id),
    PRIMARY KEY (mandate_id, scheme_id)
);

CREATE TABLE autodebit_attempts (
    id                     uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    school_id              uuid NOT NULL,
    mandate_id             uuid NOT NULL REFERENCES autodebit_mandates(id),
    bill_id                uuid NOT NULL REFERENCES student_bills(id),
    attempt_no             integer NOT NULL CHECK (attempt_no >= 1),
    scheduled_for          date NOT NULL,
    amount_idr             bigint NOT NULL CHECK (amount_idr > 0),
    status                 text NOT NULL DEFAULT 'scheduled'
                           CHECK (status IN ('scheduled','processing','success','failed','skipped')),
    skip_reason            text NULL,
    gateway_transaction_id uuid NULL REFERENCES gateway_transactions(id),
    failure_reason         text NULL,
    idempotency_key        text NOT NULL,
    created_at             timestamptz NOT NULL DEFAULT now(),
    updated_at             timestamptz NOT NULL DEFAULT now(),
    UNIQUE (school_id, idempotency_key),
    UNIQUE (mandate_id, bill_id, attempt_no)
);
CREATE INDEX autodebit_attempts_due_idx ON autodebit_attempts (school_id, scheduled_for) WHERE status = 'scheduled';
CREATE INDEX autodebit_attempts_status_idx ON autodebit_attempts (school_id, status, created_at DESC);
CREATE INDEX autodebit_attempts_tx_idx ON autodebit_attempts (gateway_transaction_id) WHERE gateway_transaction_id IS NOT NULL;
