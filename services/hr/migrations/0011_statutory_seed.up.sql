-- 0011_statutory_seed.up.sql: Parameter Statutori Pajak & BPJS Nasional
-- PERLU VALIDASI KONSULTAN PAJAK (OQ-19/20)

-- BPJS Kesehatan & Ketenagakerjaan
INSERT INTO statutory_bpjs (program, employer_bp, employee_bp, wage_cap_idr, effective_from, source_note) VALUES
('kesehatan', 400, 100, 12000000, '2024-01-01', 'Perpres 59/2024'),
('jht', 370, 200, NULL, '2015-07-01', 'PP 46/2015'),
('jp', 200, 100, 10042300, '2024-03-01', 'Permenaker No 1 Tahun 2024'),
('jkk', 24, 0, NULL, '2015-07-01', 'PP 44/2015 Tingkat Risiko Rendah'),
('jkm', 30, 0, NULL, '2015-07-01', 'PP 44/2015')
ON CONFLICT DO NOTHING;

-- PTKP (Penghasilan Tidak Kena Pajak)
INSERT INTO statutory_ptkp (status_code, annual_idr, effective_from, source_note) VALUES
('TK/0', 54000000, '2016-01-01', 'PMK 101/PMK.010/2016'),
('TK/1', 58500000, '2016-01-01', 'PMK 101/PMK.010/2016'),
('TK/2', 63000000, '2016-01-01', 'PMK 101/PMK.010/2016'),
('TK/3', 67500000, '2016-01-01', 'PMK 101/PMK.010/2016'),
('K/0', 58500000, '2016-01-01', 'PMK 101/PMK.010/2016'),
('K/1', 63000000, '2016-01-01', 'PMK 101/PMK.010/2016'),
('K/2', 67500000, '2016-01-01', 'PMK 101/PMK.010/2016'),
('K/3', 72000000, '2016-01-01', 'PMK 101/PMK.010/2016')
ON CONFLICT DO NOTHING;

-- TER Categories Mapping
INSERT INTO statutory_ter_categories (category, ptkp_status_code, effective_from) VALUES
('A', 'TK/0', '2024-01-01'),
('A', 'TK/1', '2024-01-01'),
('A', 'K/0', '2024-01-01'),
('B', 'TK/2', '2024-01-01'),
('B', 'TK/3', '2024-01-01'),
('B', 'K/1', '2024-01-01'),
('B', 'K/2', '2024-01-01'),
('C', 'K/3', '2024-01-01')
ON CONFLICT DO NOTHING;

-- TER Brackets Sample (PMK 168/2023)
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
('A', 10050001, 10350000, 225, '2024-01-01')
ON CONFLICT DO NOTHING;

-- Progressive Brackets (UU HPP / Pasal 17 UU PPh)
INSERT INTO statutory_progressive (lower_idr, upper_idr, rate_bp, effective_from) VALUES
(0, 60000000, 500, '2022-01-01'),
(60000001, 250000000, 1500, '2022-01-01'),
(250000001, 500000000, 2500, '2022-01-01'),
(500000001, 5000000000, 3000, '2022-01-01'),
(5000000001, NULL, 3500, '2022-01-01')
ON CONFLICT DO NOTHING;

-- Biaya Jabatan
INSERT INTO statutory_misc (key, value, effective_from) VALUES
('biaya_jabatan_bp', 500, '2008-01-01'),
('biaya_jabatan_max_annual_idr', 6000000, '2008-01-01')
ON CONFLICT DO NOTHING;
