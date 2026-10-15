-- Multi-unit demo: Yayasan Teratai Putih menaungi tiga unit (SD, SMP, SMK)
-- untuk pemilih "Multi-Unit Workspace" di sidebar. Unit SMP adalah sekolah
-- demo yang sudah ada (1111...); SD dan SMK ditambahkan di sini beserta tahun
-- ajarannya. Akun pimpinan, admin, dan keuangan demo diberi peran yang sama
-- di SD dan SMK supaya bisa berpindah unit. Aditif dan idempoten.
INSERT INTO foundations (id, name, address)
SELECT 'a2700000-0000-0000-0000-000000000001', 'Yayasan Teratai Putih', 'Jakarta'
WHERE EXISTS (SELECT 1 FROM schools WHERE id = '11111111-1111-1111-1111-111111111111')
ON CONFLICT DO NOTHING;

UPDATE schools SET foundation_id = 'a2700000-0000-0000-0000-000000000001'
WHERE id = '11111111-1111-1111-1111-111111111111' AND foundation_id IS NULL
  AND EXISTS (SELECT 1 FROM foundations WHERE id = 'a2700000-0000-0000-0000-000000000001');

INSERT INTO schools (id, foundation_id, name, code, level, city, is_active)
SELECT v.id::uuid, 'a2700000-0000-0000-0000-000000000001', v.name, v.code, v.level, 'Jakarta', true
FROM (VALUES
  ('a2700000-0000-0000-0000-0000000000d1', 'SD Teratai Putih',  'TRT-SD',  'SD'),
  ('a2700000-0000-0000-0000-0000000000e1', 'SMK Teratai Putih', 'TRT-SMK', 'SMK')
) AS v(id, name, code, level)
WHERE EXISTS (SELECT 1 FROM foundations WHERE id = 'a2700000-0000-0000-0000-000000000001')
ON CONFLICT DO NOTHING;

INSERT INTO academic_years (id, school_id, label, is_current)
SELECT v.id::uuid, v.school_id::uuid, '2026/2027', true
FROM (VALUES
  ('a2700000-0000-0000-0000-0000000000d2', 'a2700000-0000-0000-0000-0000000000d1'),
  ('a2700000-0000-0000-0000-0000000000e2', 'a2700000-0000-0000-0000-0000000000e1')
) AS v(id, school_id)
JOIN schools s ON s.id = v.school_id::uuid
ON CONFLICT DO NOTHING;

UPDATE schools s SET current_academic_year_id = ay.id
FROM academic_years ay
WHERE ay.school_id = s.id AND ay.id IN ('a2700000-0000-0000-0000-0000000000d2', 'a2700000-0000-0000-0000-0000000000e2')
  AND s.current_academic_year_id IS NULL;

INSERT INTO user_school_roles (user_id, school_id, role, status)
SELECT u.id, s.id, v.role, 'active'
FROM (VALUES
  ('yayasan@teratai.sch.id',        'kepala_yayasan'),
  ('admin@teratai.sch.id',          'admin'),
  ('financemanager@teratai.sch.id', 'finance_manager'),
  ('akuntan@teratai.sch.id',        'akuntan'),
  -- Yayasan-level staff serving every unit, so each module's SD / SMK data
  -- can be demoed with the same account (switch unit in the sidebar).
  ('hradmin@teratai.sch.id',        'hr_admin'),
  ('hrmanager@teratai.sch.id',      'hr_manager'),
  ('payroll@teratai.sch.id',        'payroll_officer'),
  ('pustakawan@teratai.sch.id',     'pustakawan'),
  ('kantin@teratai.sch.id',         'canteen_admin'),
  ('operator.kantin@teratai.sch.id','canteen_operator'),
  ('uks@teratai.sch.id',            'petugas_uks'),
  ('konselor@teratai.sch.id',       'konselor'),
  ('kesiswaan@teratai.sch.id',      'kesiswaan'),
  ('event@teratai.sch.id',          'event_coordinator'),
  ('ekskul@teratai.sch.id',         'pembina_ekskul'),
  ('transport@teratai.sch.id',      'transport_officer'),
  ('inventaris@teratai.sch.id',     'inventory_officer'),
  ('pengadaan@teratai.sch.id',      'procurement_officer'),
  ('aset@teratai.sch.id',           'asset_officer'),
  ('fasilitas@teratai.sch.id',      'facility_officer'),
  ('teknisi@teratai.sch.id',        'teknisi'),
  -- The principal account approves in every unit (the Approval Center flows
  -- of SD / SMK route to kepala_sekolah like SMP's).
  ('kepsek@teratai.sch.id',         'kepala_sekolah'),
  ('mutu@teratai.sch.id',           'qa_coordinator'),
  ('dapodik@teratai.sch.id',        'operator_dapodik'),
  ('analis@teratai.sch.id',         'bi_analyst'),
  ('humas@teratai.sch.id',          'humas'),
  ('servicedesk@teratai.sch.id',    'servicedesk_agent'),
  ('servicedesk.manager@teratai.sch.id', 'servicedesk_manager'),
  ('recruiter@teratai.sch.id',      'recruiter'),
  ('training@teratai.sch.id',       'training_admin')
) AS v(email, role)
JOIN users u ON u.email = v.email
CROSS JOIN schools s
WHERE s.id IN ('a2700000-0000-0000-0000-0000000000d1', 'a2700000-0000-0000-0000-0000000000e1')
ON CONFLICT (user_id, school_id, role) DO NOTHING;

-- PKL is a vocational (SMK) programme: its coordinator and mentor work there.
INSERT INTO user_school_roles (user_id, school_id, role, status)
SELECT u.id, 'a2700000-0000-0000-0000-0000000000e1', v.role, 'active'
FROM (VALUES
  ('pkl@teratai.sch.id',                 'koordinator_pkl'),
  ('pembimbing.pkl@teratai.sch.id',      'pembimbing_pkl'),
  ('pembimbing.industri@teratai.sch.id', 'pembimbing_industri')
) AS v(email, role)
JOIN users u ON u.email = v.email
WHERE EXISTS (SELECT 1 FROM schools WHERE id = 'a2700000-0000-0000-0000-0000000000e1')
ON CONFLICT (user_id, school_id, role) DO NOTHING;

-- The modules of SD and SMK: the feature set of SMP (the demo has data for
-- every module in every unit). PKL is vocational, so SMK only; white-label
-- branding stays a property of the original school.
INSERT INTO school_features (school_id, feature_key, enabled, updated_at)
SELECT s.id, f.feature_key, true, now()
FROM school_features f
CROSS JOIN schools s
WHERE f.school_id = '11111111-1111-1111-1111-111111111111' AND f.enabled
  AND f.feature_key <> 'white_label'
  AND s.id IN ('a2700000-0000-0000-0000-0000000000d1', 'a2700000-0000-0000-0000-0000000000e1')
  AND NOT (f.feature_key = 'pkl' AND s.id = 'a2700000-0000-0000-0000-0000000000d1')
ON CONFLICT (school_id, feature_key) DO NOTHING;
