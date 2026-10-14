-- QA-IDN putaran 2 / QA-CORE BUG-089, BUG-153, BUG-154: pengumuman dengan
-- audiens wajib (peran ∩ unit ∩ kelas) dan jadwal terbit.
--
-- audience_all  : "semua peran" dipilih secara eksplisit. Sebelumnya
--                 target_roles kosong berarti semua peran tanpa pernah
--                 dipilih (BUG-153); baris lama dipertahankan perilakunya.
-- target_school_ids : pengumuman global superadmin (school_id NULL) bisa
--                 dibatasi ke beberapa unit; kosong = semua unit.
-- target_class_ids  : dibatasi ke kelas (academic.classrooms); hanya untuk
--                 pengumuman satu unit. target_class_names menyimpan nama
--                 kelas saat dipilih untuk tampilan.
-- publish_at    : waktu terbit terjadwal; terlihat bila published AND
--                 (publish_at IS NULL OR publish_at <= now()).
ALTER TABLE announcements
    ADD COLUMN audience_all       BOOLEAN NOT NULL DEFAULT false,
    ADD COLUMN target_school_ids  UUID[]  NOT NULL DEFAULT '{}',
    ADD COLUMN target_class_ids   UUID[]  NOT NULL DEFAULT '{}',
    ADD COLUMN target_class_names TEXT[]  NOT NULL DEFAULT '{}',
    ADD COLUMN publish_at         TIMESTAMPTZ;

UPDATE announcements SET audience_all = true WHERE cardinality(target_roles) = 0;

ALTER TABLE announcements
    ADD CONSTRAINT announcements_audience_required
        CHECK (audience_all OR cardinality(target_roles) > 0),
    ADD CONSTRAINT announcements_class_needs_unit
        CHECK (cardinality(target_class_ids) = 0 OR school_id IS NOT NULL);

CREATE INDEX idx_announcements_publish_at ON announcements(publish_at) WHERE published;
