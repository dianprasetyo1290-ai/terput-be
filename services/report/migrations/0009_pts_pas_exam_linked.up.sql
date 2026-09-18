-- Bug "Nilai Tipe Ujian (PTS/PAS) Tidak Otomatis Tersinkron ke Kolom
-- Penilaian Formatif": the guru-side "+ Tambah Tipe Nilai" only recognized
-- the literal names "STS"/"SAS" as exam-linked, so a tipe named "PTS"/"PAS"
-- (or UTS/UAS) landed in the plain formatif bucket -- no exam auto-fill,
-- and its scores were pooled into Nilai Formatif instead of STS/SAS.
-- Re-bucket those existing rows. Only the tipe's bucket/exam_linked flag
-- changes; every saved formative_grades score is kept as-is.
UPDATE report_assessment_types
SET bucket = 'sts', exam_linked = true, updated_at = now()
WHERE bucket = 'formatif' AND lower(name) ~ '\m(pts|uts)\M';

UPDATE report_assessment_types
SET bucket = 'sas', exam_linked = true, updated_at = now()
WHERE bucket = 'formatif' AND lower(name) ~ '\m(pas|uas)\M';
