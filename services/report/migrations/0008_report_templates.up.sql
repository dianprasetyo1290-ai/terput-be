-- "Import Template" (Generate Rapor admin screen): lets a school replace
-- the 3 reference .xlsx files (Rapor Tengah Semester / Rapor Akhir Semester
-- / Rapor Akhir Semester Halaman 2) that define the printed rapor's visual
-- layout, per school. Scope cut (see services/report/README.md and the
-- `ponytail:` comment in handler/report_card.go's print-data assembly):
-- this table is upload/storage/replace ONLY -- the stored .xlsx is not
-- parsed or rendered from. Print output still comes from the hardcoded (but
-- dynamic-content, per-school) layout in the frontend print view. Building
-- a generic "render PDF from arbitrary uploaded template" engine was
-- explicitly out of scope (YAGNI) for this pass.
--
-- Stored as BYTEA directly in Postgres, not a filesystem path or object
-- store: no prior convention exists anywhere in this codebase for storing
-- raw uploaded files (services/evaluation's dimensi_lulusan import parses
-- an .xlsx into DB rows immediately and never persists the file itself),
-- and report-service's whole architecture is "just Postgres" -- adding a
-- filesystem volume or MinIO/S3 dependency for one admin-facing file-of-
-- record per school would be new infrastructure for a rarely-written,
-- small (<10MB) file. Reconsider if templates need to be served by a CDN
-- or exceed a few MB.
CREATE TABLE report_templates (
    id             UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    school_id      UUID NOT NULL,
    template_type  VARCHAR(30) NOT NULL CHECK (template_type IN ('tengah_semester', 'akhir_semester', 'akhir_semester_hal2')),
    file_name      TEXT NOT NULL,
    content_type   TEXT NOT NULL,
    file_size      INT NOT NULL,
    file_data      BYTEA NOT NULL,
    uploaded_by    UUID,
    uploaded_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (school_id, template_type)
);
