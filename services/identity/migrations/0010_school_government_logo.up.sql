-- SEKOCI V10 item C: the rapor letterhead carries TWO logos -- Logo
-- Pemerintah (kiri atas) and Logo Sekolah (kanan atas). Per-school rather
-- than a hardcoded emblem: which government logo belongs on a rapor depends
-- on the school's own province/dinas, and this table is multi-tenant.
-- Same TEXT/data-URL storage as the existing `logo` column.
ALTER TABLE schools ADD COLUMN government_logo TEXT;
