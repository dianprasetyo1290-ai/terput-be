-- 0011_procurement.down.sql
DROP VIEW IF EXISTS pr_open_commitment_v;
DROP VIEW IF EXISTS po_line_match_v;
DROP TABLE IF EXISTS vendor_evaluations;
DROP TABLE IF EXISTS goods_returns;
DROP TABLE IF EXISTS goods_receipt_lines;
DROP TABLE IF EXISTS goods_receipts;
DROP TABLE IF EXISTS purchase_order_lines;
DROP TABLE IF EXISTS purchase_orders;
DROP TABLE IF EXISTS purchase_request_lines;
DROP TABLE IF EXISTS purchase_requests;
DROP TABLE IF EXISTS vendor_accounting_sync;
DROP TABLE IF EXISTS vendor_bank_accounts;
DROP TABLE IF EXISTS vendor_documents;
DROP TABLE IF EXISTS vendor_contacts;
DROP TABLE IF EXISTS vendors;
