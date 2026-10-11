-- 0016: Track user and guardian deactivation timestamp for parent grace days calculation

ALTER TABLE user_school_roles ADD COLUMN IF NOT EXISTS deactivated_at TIMESTAMPTZ;
ALTER TABLE guardian_links ADD COLUMN IF NOT EXISTS deactivated_at TIMESTAMPTZ;
