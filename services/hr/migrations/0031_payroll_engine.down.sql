-- 0031_payroll_engine.down.sql: back to the 0011 partial TER seed.
ALTER TABLE payroll_accounts DROP COLUMN IF EXISTS salary_expense_acc;
DROP INDEX IF EXISTS idx_tax_remittances_period;
DROP INDEX IF EXISTS idx_payroll_lines_employee;
DROP TRIGGER IF EXISTS trg_payroll_lines_guard_final ON payroll_lines;
DROP FUNCTION IF EXISTS payroll_lines_guard_final();
DELETE FROM statutory_ter_brackets WHERE effective_from = '2024-01-01';
INSERT INTO statutory_ter_brackets (category, lower_idr, upper_idr, rate_bp, effective_from) VALUES
('A', 0, 5400000, 0, '2024-01-01'),
('A', 5400001, 5650000, 25, '2024-01-01'),
('A', 5650001, 5950000, 50, '2024-01-01'),
('A', 5950001, 6300000, 75, '2024-01-01'),
('A', 6300001, 6750000, 100, '2024-01-01'),
('A', 6750001, 7500000, 125, '2024-01-01'),
('A', 7500001, 8550000, 150, '2024-01-01'),
('A', 8550001, 9650000, 175, '2024-01-01'),
('A', 9650001, 10050000, 200, '2024-01-01'),
('A', 10050001, 10350000, 225, '2024-01-01');
