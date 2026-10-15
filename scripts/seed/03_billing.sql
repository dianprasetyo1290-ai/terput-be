-- 03_billing.sql
-- Seed billing schemes, student bills, and items for Teratai Putih Global School

DO $$
DECLARE
  v_school_id UUID := '11111111-1111-1111-1111-111111111111';
  v_ay_id     UUID := '12121212-1212-1212-1212-121212121212';
  v_siswa     UUID := '44444444-4444-4444-4444-444444444444';
  v_scheme_id UUID;
  v_bill_juli UUID;
  v_bill_agust UUID;
  v_bill_sept UUID;
  v_bill_okt  UUID;
BEGIN
  -- 1. Billing Scheme: SPP Bulanan
  INSERT INTO billing_schemes (
    school_id, academic_year_id, name, status, installment, due_day, notes
  ) VALUES (
    v_school_id, v_ay_id, 'SPP Reguler SMP TPGS 2026/2027', 'active', true, 10, 'SPP bulanan termasuk fasilitas lab & ekskul'
  )
  ON CONFLICT DO NOTHING;
  SELECT id INTO v_scheme_id FROM billing_schemes WHERE school_id = v_school_id AND name = 'SPP Reguler SMP TPGS 2026/2027' LIMIT 1;

  IF v_scheme_id IS NOT NULL THEN
    -- Components
    INSERT INTO billing_scheme_components (school_id, scheme_id, code, name, kind, amount_idr, sort_order)
    VALUES (v_school_id, v_scheme_id, 'SPP', 'SPP Pokok Bulanan', 'monthly', 750000, 1)
    ON CONFLICT (scheme_id, code) DO NOTHING;

    -- 2. Bills for Budi Santoso
    -- Juli 2026 (Paid)
    INSERT INTO student_bills (
      school_id, student_user_id, scheme_id, period_year, period_month,
      base_idr, discount_idr, net_idr, status, due_at, discount_snapshot
    ) VALUES (
      v_school_id, v_siswa, v_scheme_id, 2026, 7,
      750000, 0, 750000, 'paid', '2026-07-10 23:59:59+07', '[]'::jsonb
    )
    ON CONFLICT (school_id, student_user_id, period_year, period_month, scheme_id) DO NOTHING
    RETURNING id INTO v_bill_juli;

    IF v_bill_juli IS NOT NULL THEN
      INSERT INTO bill_items (school_id, bill_id, name, component_code, amount_idr, discount_idr, net_idr)
      VALUES (v_school_id, v_bill_juli, 'SPP Pokok Juli 2026', 'SPP', 750000, 0, 750000)
      ON CONFLICT DO NOTHING;
    END IF;

    -- Agustus 2026 (Paid)
    INSERT INTO student_bills (
      school_id, student_user_id, scheme_id, period_year, period_month,
      base_idr, discount_idr, net_idr, status, due_at, discount_snapshot
    ) VALUES (
      v_school_id, v_siswa, v_scheme_id, 2026, 8,
      750000, 0, 750000, 'paid', '2026-08-10 23:59:59+07', '[]'::jsonb
    )
    ON CONFLICT (school_id, student_user_id, period_year, period_month, scheme_id) DO NOTHING
    RETURNING id INTO v_bill_agust;

    IF v_bill_agust IS NOT NULL THEN
      INSERT INTO bill_items (school_id, bill_id, name, component_code, amount_idr, discount_idr, net_idr)
      VALUES (v_school_id, v_bill_agust, 'SPP Pokok Agustus 2026', 'SPP', 750000, 0, 750000)
      ON CONFLICT DO NOTHING;
    END IF;

    -- September 2026 (Unpaid)
    INSERT INTO student_bills (
      school_id, student_user_id, scheme_id, period_year, period_month,
      base_idr, discount_idr, net_idr, status, due_at, discount_snapshot
    ) VALUES (
      v_school_id, v_siswa, v_scheme_id, 2026, 9,
      750000, 0, 750000, 'unpaid', '2026-09-10 23:59:59+07', '[]'::jsonb
    )
    ON CONFLICT (school_id, student_user_id, period_year, period_month, scheme_id) DO NOTHING
    RETURNING id INTO v_bill_sept;

    IF v_bill_sept IS NOT NULL THEN
      INSERT INTO bill_items (school_id, bill_id, name, component_code, amount_idr, discount_idr, net_idr)
      VALUES (v_school_id, v_bill_sept, 'SPP Pokok September 2026', 'SPP', 750000, 0, 750000)
      ON CONFLICT DO NOTHING;
    END IF;

    -- Oktober 2026 (Unpaid)
    INSERT INTO student_bills (
      school_id, student_user_id, scheme_id, period_year, period_month,
      base_idr, discount_idr, net_idr, status, due_at, discount_snapshot
    ) VALUES (
      v_school_id, v_siswa, v_scheme_id, 2026, 10,
      750000, 0, 750000, 'unpaid', '2026-10-10 23:59:59+07', '[]'::jsonb
    )
    ON CONFLICT (school_id, student_user_id, period_year, period_month, scheme_id) DO NOTHING
    RETURNING id INTO v_bill_okt;

    IF v_bill_okt IS NOT NULL THEN
      INSERT INTO bill_items (school_id, bill_id, name, component_code, amount_idr, discount_idr, net_idr)
      VALUES (v_school_id, v_bill_okt, 'SPP Pokok Oktober 2026', 'SPP', 750000, 0, 750000)
      ON CONFLICT DO NOTHING;
    END IF;

  END IF;

END $$;
