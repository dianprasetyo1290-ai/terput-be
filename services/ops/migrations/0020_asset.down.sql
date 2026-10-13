-- 0020_asset.down.sql: Revert asset tables and extensions

DROP TABLE IF EXISTS asset_depreciation_runs CASCADE;
DROP TABLE IF EXISTS asset_book_values CASCADE;
DROP TABLE IF EXISTS asset_accounting_events CASCADE;
DROP TABLE IF EXISTS asset_disposals CASCADE;
DROP TABLE IF EXISTS asset_movements CASCADE;
DROP TABLE IF EXISTS assets CASCADE;
DROP TABLE IF EXISTS asset_settings CASCADE;
ALTER TABLE item_categories DROP COLUMN IF EXISTS asset_category_id;
DROP TABLE IF EXISTS asset_categories CASCADE;
