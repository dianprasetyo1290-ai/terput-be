-- 01_identity_features.sql
-- Enables all 32 PRD features, roles, and white-label branding for Teratai Putih Global School

DO $$
DECLARE
  v_school_id UUID := '11111111-1111-1111-1111-111111111111';
  v_admin_id  UUID := '22222222-2222-2222-2222-222222222222';
  v_features  TEXT[] := ARRAY[
    'accounting', 'hris', 'payroll', 'ats', 'training', 'payment_gateway', 'student_bills', 'service_requests',
    'facility', 'health', 'service_desk', 'canteen', 'wallet', 'campaign', 'teacher_parent_chat', 'ppdb_marketing',
    'payment_multi_provider', 'payment_autodebit', 'inventory', 'procurement', 'asset', 'budget', 'tax_nonpayroll',
    'payable', 'library', 'transport', 'counseling', 'discipline', 'event', 'extracurricular', 'achievement',
    'pkl', 'quality', 'gov_reporting', 'bi', 'ai_assistant', 'predictive', 'campus3d', 'white_label'
  ];
  v_f TEXT;
  v_roles TEXT[] := ARRAY[
    'admin', 'inventory_officer', 'procurement_officer', 'asset_officer', 'pustakawan',
    'facility_officer', 'teknisi', 'transport_officer', 'canteen_admin', 'canteen_operator',
    'petugas_uks', 'konselor', 'kesiswaan', 'event_coordinator', 'pembina_ekskul',
    'servicedesk_manager', 'servicedesk_agent', 'koordinator_pkl', 'pembimbing_pkl',
    'qa_coordinator', 'operator_dapodik', 'humas', 'bi_analyst'
  ];
  v_r TEXT;
BEGIN
  -- 1. Enable all 32 features
  FOREACH v_f IN ARRAY v_features LOOP
    INSERT INTO school_features (school_id, feature_key, enabled, updated_at)
    VALUES (v_school_id, v_f, true, NOW())
    ON CONFLICT (school_id, feature_key) DO UPDATE SET enabled = true, updated_at = NOW();
  END LOOP;

  -- 2. Grant roles to admin@teratai.sch.id
  FOREACH v_r IN ARRAY v_roles LOOP
    INSERT INTO user_school_roles (user_id, school_id, role, status)
    VALUES (v_admin_id, v_school_id, v_r, 'active')
    ON CONFLICT (user_id, school_id, role) DO UPDATE SET status = 'active';
  END LOOP;

  -- 3. White-label app branding
  INSERT INTO app_brandings (
    school_id, app_name, short_name, theme_color, background_color,
    included_portals, privacy_policy_url, support_email, updated_by
  ) VALUES (
    v_school_id,
    'Teratai Putih Smart School',
    'TPGS App',
    '#1e3a8a',
    '#ffffff',
    ARRAY['parent', 'student', 'teacher'],
    'https://terataiputih.sch.id/privacy',
    'bantuan@teratai.sch.id',
    v_admin_id
  )
  ON CONFLICT (school_id) DO UPDATE SET
    app_name = EXCLUDED.app_name,
    short_name = EXCLUDED.short_name,
    theme_color = EXCLUDED.theme_color,
    background_color = EXCLUDED.background_color,
    updated_at = NOW();

  INSERT INTO app_branding_versions (
    school_id, version, snapshot, published_by, published_at
  ) VALUES (
    v_school_id,
    1,
    jsonb_build_object(
      'app_name', 'Teratai Putih Smart School',
      'short_name', 'TPGS App',
      'theme_color', '#1e3a8a',
      'background_color', '#ffffff',
      'included_portals', '["parent", "student", "teacher"]'::jsonb
    ),
    v_admin_id,
    NOW()
  )
  ON CONFLICT (school_id, version) DO NOTHING;

END $$;
