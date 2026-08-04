-- Add formatif_score to store purely the accumulated formative grades (tugas/harian)
-- independent from the final blended computed_score.
ALTER TABLE report_card_subjects ADD COLUMN formatif_score NUMERIC(5,2);
