-- E-Rapor client requirement: "belum terdapat capaian sts dan capaian sas"
-- (Deskripsi Rapor had no distinct STS/SAS achievement score -- only a
-- single blended computed_score across every exam type mapped to the
-- report card). Adds two more aggregates alongside computed_score, each
-- averaged only over exams whose ExamType.mapping_rapor names that specific
-- bucket (see classifyMapping in handler/report_card.go). Nullable/never
-- touched by override logic, same treatment as computed_score.
ALTER TABLE report_card_subjects ADD COLUMN sts_score NUMERIC(5,2);
ALTER TABLE report_card_subjects ADD COLUMN sas_score NUMERIC(5,2);
