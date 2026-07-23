DROP INDEX IF EXISTS idx_announcements_published;
ALTER TABLE announcements
    DROP COLUMN IF EXISTS jenis,
    DROP COLUMN IF EXISTS documentation_photo_urls,
    DROP COLUMN IF EXISTS thumbnail_url,
    DROP COLUMN IF EXISTS url,
    DROP COLUMN IF EXISTS location,
    DROP COLUMN IF EXISTS published,
    DROP COLUMN IF EXISTS type;
