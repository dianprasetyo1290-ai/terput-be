-- admission 0004 (T13, PRD menu 149): PPDB landing pages, visit and
-- application attribution, referral programs/codes and rewards issued
-- through the menu 09 promo engine (discount_vouchers). Additive only.

CREATE TABLE landing_pages (
    id            uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    school_id     uuid NOT NULL,
    period_id     uuid NOT NULL REFERENCES admission_periods(id),
    wave_id       uuid REFERENCES admission_waves(id),
    slug          text NOT NULL CHECK (slug ~ '^[a-z0-9]+(-[a-z0-9]+)*$'),
    title         text NOT NULL,
    template_code text NOT NULL CHECK (template_code IN ('hero_cta','program_grid','faq_testimonial')),
    content       jsonb NOT NULL DEFAULT '{}'::jsonb,   -- template sections, never raw HTML
    status        text NOT NULL DEFAULT 'draft' CHECK (status IN ('draft','published','archived')),
    published_at  timestamptz,
    created_by    text NOT NULL,
    created_at    timestamptz NOT NULL DEFAULT now(),
    updated_at    timestamptz NOT NULL DEFAULT now(),
    deleted_at    timestamptz,
    UNIQUE (school_id, slug)
);
CREATE INDEX landing_pages_school_idx ON landing_pages (school_id, status) WHERE deleted_at IS NULL;

CREATE TABLE referral_programs (
    id                uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    school_id         uuid NOT NULL,
    period_id         uuid NOT NULL REFERENCES admission_periods(id),  -- period the reward voucher is valid for
    name              text NOT NULL DEFAULT '',
    reward_kind       text NOT NULL CHECK (reward_kind IN ('fixed','percent')),
    reward_value      bigint NOT NULL CHECK (reward_value > 0),         -- rupiah (fixed) or basis points (percent)
    reward_cap_idr    bigint CHECK (reward_cap_idr IS NULL OR reward_cap_idr > 0),
    reward_valid_days integer NOT NULL CHECK (reward_valid_days > 0),
    priority          integer NOT NULL,
    active            boolean NOT NULL DEFAULT true,
    created_by        text NOT NULL,
    created_at        timestamptz NOT NULL DEFAULT now(),
    updated_at        timestamptz NOT NULL DEFAULT now(),
    CHECK (reward_kind <> 'percent' OR reward_value <= 10000)
);
CREATE INDEX referral_programs_active_idx ON referral_programs (school_id, active);

CREATE TABLE referral_codes (
    id            uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    school_id     uuid NOT NULL,
    code          text NOT NULL,
    owner_user_id uuid NOT NULL,   -- identity user (orang_tua)
    owner_kind    text NOT NULL CHECK (owner_kind IN ('parent','alumni')),
    status        text NOT NULL DEFAULT 'active' CHECK (status IN ('active','revoked')),
    created_at    timestamptz NOT NULL DEFAULT now(),
    revoked_at    timestamptz,
    UNIQUE (school_id, code),
    UNIQUE (school_id, owner_user_id)
);

-- One visit per visitor per page per day; visitor_hash is sha256 of a random
-- browser id (no raw IP).
CREATE TABLE landing_visits (
    id               uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    school_id        uuid NOT NULL,
    landing_page_id  uuid REFERENCES landing_pages(id),
    visitor_hash     text NOT NULL,
    utm_source       text,
    utm_medium       text,
    utm_campaign     text,
    referral_code_id uuid REFERENCES referral_codes(id),
    visited_on       date NOT NULL,
    created_at       timestamptz NOT NULL DEFAULT now(),
    UNIQUE (school_id, landing_page_id, visitor_hash, visited_on)
);
CREATE INDEX landing_visits_report_idx ON landing_visits (school_id, visited_on);

CREATE TABLE application_attributions (
    application_id   uuid PRIMARY KEY REFERENCES applications(id),
    school_id        uuid NOT NULL,
    landing_page_id  uuid REFERENCES landing_pages(id),
    referral_code_id uuid REFERENCES referral_codes(id),
    utm_source       text,
    utm_medium       text,
    utm_campaign     text,
    created_at       timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX application_attributions_school_idx ON application_attributions (school_id);

CREATE TABLE referral_rewards (
    id               uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    school_id        uuid NOT NULL,
    referral_code_id uuid NOT NULL REFERENCES referral_codes(id),
    application_id   uuid NOT NULL UNIQUE REFERENCES applications(id),
    program_id       uuid REFERENCES referral_programs(id),
    status           text NOT NULL DEFAULT 'pending_reregistration' CHECK (status IN ('pending_reregistration','issued','void')),
    void_reason      text CHECK (void_reason IN ('self_referral','application_withdrawn','no_active_program','code_revoked')),
    voucher_id       uuid REFERENCES discount_vouchers(id),
    voucher_code     text,          -- shown only to the code owner and admin
    issued_at        timestamptz,
    idempotency_key  text NOT NULL UNIQUE,
    created_at       timestamptz NOT NULL DEFAULT now(),
    updated_at       timestamptz NOT NULL DEFAULT now(),
    CHECK (status <> 'void' OR void_reason IS NOT NULL),
    CHECK (status <> 'issued' OR voucher_id IS NOT NULL)
);
CREATE INDEX referral_rewards_code_idx ON referral_rewards (school_id, referral_code_id, status);
