-- QA-IDN (BUG-071): a semester must end after it starts. NOT VALID keeps
-- existing bad rows (e.g. Semester 1 2027/2028: 2026-10-14 .. 2026-01-01 on
-- the demo server) readable; every new insert/update is checked. Fix those
-- rows, then run: ALTER TABLE terms VALIDATE CONSTRAINT terms_dates_check;
ALTER TABLE terms ADD CONSTRAINT terms_dates_check
    CHECK (start_date IS NULL OR end_date IS NULL OR end_date > start_date) NOT VALID;
