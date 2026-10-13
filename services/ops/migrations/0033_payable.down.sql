-- 0033_payable.down.sql

DROP TABLE IF EXISTS payable_journals CASCADE;
DROP TABLE IF EXISTS payment_schedules CASCADE;
DROP TABLE IF EXISTS vendor_invoice_lines CASCADE;
DROP TABLE IF EXISTS vendor_invoices CASCADE;
DROP TABLE IF EXISTS payable_settings CASCADE;
