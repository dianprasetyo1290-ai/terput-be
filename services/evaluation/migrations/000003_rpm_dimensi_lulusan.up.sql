-- Client request: RPM gains "8 Dimensi Lulusan" (graduate-profile dimensions,
-- Kurikulum Merdeka's successor to the 6-dimension Profil Pelajar Pancasila),
-- filled via an Excel-template import rather than one-by-one form fields.
-- Modeled as freeform JSONB keyed by the 8 fixed dimension codes (see
-- internal/handler/dimensi_lulusan.go dimensiLulusanFields), same pattern as
-- section_a/b/c.
ALTER TABLE lesson_plans ADD COLUMN dimensi_lulusan JSONB NOT NULL DEFAULT '{}'::jsonb;
