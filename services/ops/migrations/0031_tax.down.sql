-- 0031_tax.down.sql

DROP TABLE IF EXISTS tax_export_templates CASCADE;
DROP TABLE IF EXISTS output_tax_invoices CASCADE;
DROP TABLE IF EXISTS input_vat CASCADE;
DROP TABLE IF EXISTS withholding_slips CASCADE;
DROP TABLE IF EXISTS withholding_tax_rates CASCADE;
DROP TABLE IF EXISTS withholding_tax_types CASCADE;
DROP TABLE IF EXISTS tax_settings CASCADE;
