-- Kelola Informasi rebuild: split announcements into two content types
-- (Acara/Event vs Pengumuman) with their own extra fields, plus an explicit
-- published/draft state. Existing rows default to type='pengumuman' and
-- published=true so nothing already visible disappears after this migration
-- (the "message" column keeps doubling as Konten for events / Pesan for
-- pengumuman — same free-text semantics, no need for a second body column).
ALTER TABLE announcements
    ADD COLUMN type TEXT NOT NULL DEFAULT 'pengumuman' CHECK (type IN ('event', 'pengumuman')),
    ADD COLUMN published BOOLEAN NOT NULL DEFAULT true,
    ADD COLUMN location TEXT,                                    -- event only: Lokasi
    ADD COLUMN url TEXT,                                          -- event only: Url/Link
    ADD COLUMN thumbnail_url TEXT,                                -- event only: banner thumbnail
    ADD COLUMN documentation_photo_urls TEXT[] NOT NULL DEFAULT '{}', -- event only: dokumentasi gallery
    ADD COLUMN jenis TEXT CHECK (jenis IN ('umum', 'ujian'));     -- pengumuman only

CREATE INDEX idx_announcements_published ON announcements(published);
