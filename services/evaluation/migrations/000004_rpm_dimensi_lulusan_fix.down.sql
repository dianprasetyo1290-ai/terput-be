-- No-op: the up migration wipes acknowledged-wrong dimensi_lulusan data,
-- which cannot be un-wiped. Down just leaves the (already empty) column as
-- is, matching golang-migrate's convention that a down migration for a
-- data-only change need not resurrect deleted data.
SELECT 1;
