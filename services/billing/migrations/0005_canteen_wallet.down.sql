-- 0005 down: removes the canteen/wallet tables. Rows that only exist because
-- of this feature (wallet receipts, wallet/canteen journal outbox rows and
-- wallet refunds) are removed first so the T6 constraints can be restored;
-- T6 data is left untouched.

DELETE FROM gateway_journal_postings WHERE source <> 'gateway';
ALTER TABLE gateway_journal_postings DROP COLUMN IF EXISTS source;

DELETE FROM receipts WHERE source = 'wallet';
ALTER TABLE receipts DROP CONSTRAINT IF EXISTS receipts_source_check;
ALTER TABLE receipts ADD CONSTRAINT receipts_source_check CHECK (source IN ('gateway','cashier'));

DELETE FROM bill_refunds WHERE wallet_id IS NOT NULL;
DROP INDEX IF EXISTS bill_refunds_wallet_idx;
ALTER TABLE bill_refunds DROP COLUMN IF EXISTS wallet_id;

DROP TABLE IF EXISTS wallet_reconciliations;
DROP TABLE IF EXISTS wallet_accounts;
DROP TABLE IF EXISTS wallet_closures;
DROP TABLE IF EXISTS savings_requests;
DROP TABLE IF EXISTS wallet_topups;
DROP TABLE IF EXISTS student_cards;
ALTER TABLE IF EXISTS canteen_sales DROP CONSTRAINT IF EXISTS canteen_sales_settlement_fk;
DROP TABLE IF EXISTS canteen_settlements;
DROP TABLE IF EXISTS canteen_sale_items;
DROP TABLE IF EXISTS canteen_sales;
DROP TRIGGER IF EXISTS wallet_movements_append_only_trg ON wallet_movements;
DROP TABLE IF EXISTS wallet_movements;
DROP FUNCTION IF EXISTS wallet_movements_append_only();
DROP TABLE IF EXISTS wallets;
DROP TABLE IF EXISTS wallet_settings;
DROP TABLE IF EXISTS canteen_sale_sequences;
DROP TABLE IF EXISTS canteen_daily_stock;
DROP TABLE IF EXISTS canteen_menu_items;
DROP TABLE IF EXISTS canteen_tenant_operators;
DROP TRIGGER IF EXISTS canteen_scheme_no_overlap_trg ON canteen_tenant_schemes;
DROP TABLE IF EXISTS canteen_tenant_schemes;
DROP FUNCTION IF EXISTS canteen_scheme_no_overlap();
DROP TABLE IF EXISTS canteen_tenants;
