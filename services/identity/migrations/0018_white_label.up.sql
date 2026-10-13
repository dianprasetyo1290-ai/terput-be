-- 0018: White-label branding, PWA manifest, and App Store listings (T26, menu 159)

CREATE TABLE app_brandings (
    id                 uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    school_id          uuid NOT NULL UNIQUE REFERENCES schools(id) ON DELETE CASCADE,
    app_name           text NOT NULL,
    short_name         text NOT NULL CHECK (char_length(short_name) <= 12),
    theme_color        text NOT NULL CHECK (theme_color ~ '^#[0-9a-fA-F]{6}$'),
    background_color   text NOT NULL CHECK (background_color ~ '^#[0-9a-fA-F]{6}$'),
    icon_192           text NULL,
    icon_512           text NULL,
    icon_maskable      text NULL,
    splash             text NULL,
    domain             text NULL UNIQUE CHECK (domain = lower(domain)),
    included_portals   text[] NOT NULL DEFAULT '{parent,student,teacher}' CHECK (included_portals <@ ARRAY['parent','student','teacher']),
    privacy_policy_url text NULL,
    support_email      text NULL,
    updated_by         uuid NULL,
    created_at         timestamptz NOT NULL DEFAULT now(),
    updated_at         timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX app_brandings_domain_idx ON app_brandings (domain);

CREATE TABLE app_branding_versions (
    id            uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    school_id     uuid NOT NULL REFERENCES schools(id) ON DELETE CASCADE,
    version       int NOT NULL,
    snapshot      jsonb NOT NULL,
    icon_192      text NULL,
    icon_512      text NULL,
    icon_maskable text NULL,
    splash        text NULL,
    published_by  uuid NULL,
    published_at  timestamptz NOT NULL DEFAULT now(),
    UNIQUE (school_id, version)
);

CREATE INDEX app_branding_versions_school_idx ON app_branding_versions (school_id, version DESC);

CREATE TABLE app_store_listings (
    id                  uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    school_id           uuid NOT NULL REFERENCES schools(id) ON DELETE CASCADE,
    store               text NOT NULL CHECK (store IN ('google_play','app_store')),
    account_owner       text NOT NULL CHECK (account_owner IN ('client','fullcircle')),
    package_id          text NULL,
    sha256_fingerprints text[] NOT NULL DEFAULT '{}',
    status              text NOT NULL CHECK (status IN ('not_started','in_review','published','rejected')),
    listing_url         text NULL,
    note                text NULL,
    updated_by          uuid NULL,
    created_at          timestamptz NOT NULL DEFAULT now(),
    updated_at          timestamptz NOT NULL DEFAULT now(),
    UNIQUE (school_id, store)
);
