-- Akun demo per peran untuk halaman Akses Demo (SPA /demo): satu akun untuk
-- setiap peran yang belum punya akun di seed-all-roles-teratai.sql atau
-- 20_identity_features.sql, supaya setiap modul PRD bisa didemokan.
-- Hash password disalin dari user demo admin@teratai.sch.id (tidak membuat
-- hash baru); dilewati bila user admin demo belum ada. Aditif dan idempoten.
INSERT INTO users (id, name, email, password_hash, password_changed, is_superadmin)
SELECT v.id::uuid, v.name, v.email, src.password_hash, true, false
FROM (VALUES
  ('a2600000-0000-0000-0000-000000000001', 'Akuntan Teratai',                 'akuntan@teratai.sch.id'),
  ('a2600000-0000-0000-0000-000000000002', 'Petugas Inventaris Teratai',      'inventaris@teratai.sch.id'),
  ('a2600000-0000-0000-0000-000000000003', 'Petugas Pengadaan Teratai',       'pengadaan@teratai.sch.id'),
  ('a2600000-0000-0000-0000-000000000004', 'Petugas Aset Teratai',            'aset@teratai.sch.id'),
  ('a2600000-0000-0000-0000-000000000005', 'Pustakawan Teratai',              'pustakawan@teratai.sch.id'),
  ('a2600000-0000-0000-0000-000000000006', 'Petugas Fasilitas Teratai',       'fasilitas@teratai.sch.id'),
  ('a2600000-0000-0000-0000-000000000007', 'Teknisi Teratai',                 'teknisi@teratai.sch.id'),
  ('a2600000-0000-0000-0000-000000000008', 'Petugas Transportasi Teratai',    'transport@teratai.sch.id'),
  ('a2600000-0000-0000-0000-000000000009', 'Admin Kantin Teratai',            'kantin@teratai.sch.id'),
  ('a2600000-0000-0000-0000-00000000000a', 'Operator Kantin Teratai',         'operator.kantin@teratai.sch.id'),
  ('a2600000-0000-0000-0000-00000000000b', 'Petugas UKS Teratai',             'uks@teratai.sch.id'),
  ('a2600000-0000-0000-0000-00000000000c', 'Konselor BK Teratai',             'konselor@teratai.sch.id'),
  ('a2600000-0000-0000-0000-00000000000d', 'Wakasek Kesiswaan Teratai',       'kesiswaan@teratai.sch.id'),
  ('a2600000-0000-0000-0000-00000000000e', 'Koordinator Event Teratai',       'event@teratai.sch.id'),
  ('a2600000-0000-0000-0000-00000000000f', 'Pembina Ekskul Teratai',          'ekskul@teratai.sch.id'),
  ('a2600000-0000-0000-0000-000000000010', 'Agen Service Desk Teratai',       'servicedesk@teratai.sch.id'),
  ('a2600000-0000-0000-0000-000000000011', 'Manajer Service Desk Teratai',    'servicedesk.manager@teratai.sch.id'),
  ('a2600000-0000-0000-0000-000000000012', 'Koordinator PKL Teratai',         'pkl@teratai.sch.id'),
  ('a2600000-0000-0000-0000-000000000013', 'Pembimbing PKL Teratai',          'pembimbing.pkl@teratai.sch.id'),
  ('a2600000-0000-0000-0000-000000000014', 'Pembimbing Industri (Mitra PKL)', 'pembimbing.industri@teratai.sch.id'),
  ('a2600000-0000-0000-0000-000000000015', 'Koordinator Mutu Teratai',        'mutu@teratai.sch.id'),
  ('a2600000-0000-0000-0000-000000000016', 'Operator Dapodik Teratai',        'dapodik@teratai.sch.id'),
  ('a2600000-0000-0000-0000-000000000017', 'Humas Teratai',                   'humas@teratai.sch.id'),
  ('a2600000-0000-0000-0000-000000000018', 'Analis BI Teratai',               'analis@teratai.sch.id')
) AS v(id, name, email)
JOIN users src ON src.email = 'admin@teratai.sch.id'
WHERE EXISTS (SELECT 1 FROM schools WHERE id = '11111111-1111-1111-1111-111111111111')
ON CONFLICT DO NOTHING;

INSERT INTO user_school_roles (user_id, school_id, role, status)
SELECT u.id, '11111111-1111-1111-1111-111111111111', v.role, 'active'
FROM (VALUES
  ('akuntan@teratai.sch.id',             'akuntan'),
  ('inventaris@teratai.sch.id',          'inventory_officer'),
  ('pengadaan@teratai.sch.id',           'procurement_officer'),
  ('aset@teratai.sch.id',                'asset_officer'),
  ('pustakawan@teratai.sch.id',          'pustakawan'),
  ('fasilitas@teratai.sch.id',           'facility_officer'),
  ('teknisi@teratai.sch.id',             'teknisi'),
  ('transport@teratai.sch.id',           'transport_officer'),
  ('kantin@teratai.sch.id',              'canteen_admin'),
  ('operator.kantin@teratai.sch.id',     'canteen_operator'),
  ('uks@teratai.sch.id',                 'petugas_uks'),
  ('konselor@teratai.sch.id',            'konselor'),
  ('kesiswaan@teratai.sch.id',           'kesiswaan'),
  ('event@teratai.sch.id',               'event_coordinator'),
  ('ekskul@teratai.sch.id',              'pembina_ekskul'),
  ('servicedesk@teratai.sch.id',         'servicedesk_agent'),
  ('servicedesk.manager@teratai.sch.id', 'servicedesk_manager'),
  ('pkl@teratai.sch.id',                 'koordinator_pkl'),
  ('pembimbing.pkl@teratai.sch.id',      'pembimbing_pkl'),
  ('pembimbing.industri@teratai.sch.id', 'pembimbing_industri'),
  ('mutu@teratai.sch.id',                'qa_coordinator'),
  ('dapodik@teratai.sch.id',             'operator_dapodik'),
  ('humas@teratai.sch.id',               'humas'),
  ('analis@teratai.sch.id',              'bi_analyst')
) AS v(email, role)
JOIN users u ON u.email = v.email
ON CONFLICT (user_id, school_id, role) DO NOTHING;
