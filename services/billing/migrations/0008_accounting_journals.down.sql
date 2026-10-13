DROP INDEX IF EXISTS gateway_journal_postings_failed_idx;
DROP INDEX IF EXISTS gateway_journal_postings_school_idx;
ALTER TABLE gateway_journal_postings
    DROP COLUMN IF EXISTS exception_raised_at,
    DROP COLUMN IF EXISTS posted_at,
    DROP COLUMN IF EXISTS source_refs,
    DROP COLUMN IF EXISTS kind,
    ALTER COLUMN source SET DEFAULT 'gateway';
UPDATE gateway_journal_postings SET source = 'gateway' WHERE source = 'payment_gateway';
DROP TABLE IF EXISTS fee_component_accounts;
DROP TABLE IF EXISTS billing_journal_settings;
