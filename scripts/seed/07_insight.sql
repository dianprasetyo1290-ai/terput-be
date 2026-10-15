-- 07_insight.sql
-- Seed sample metric snapshots for BI Dashboard

DO $$
DECLARE
  v_school_id UUID := '11111111-1111-1111-1111-111111111111';
BEGIN
  -- Attendance Rate: 94.5% (9450 bp)
  INSERT INTO metric_snapshots (
    school_id, metric_key, period_kind, period_start, period_end, group_by, dim_value,
    numerator, denominator, value, source_as_of, computed_at
  ) VALUES (
    v_school_id, 'attendance_rate', 'month', '2026-09-01', '2026-09-30', '', '',
    1890, 2000, 9450, NOW(), NOW()
  )
  ON CONFLICT (school_id, metric_key, period_kind, period_start, group_by, dim_value)
  DO NOTHING;

  -- Collection Rate: 88.0% (8800 bp)
  INSERT INTO metric_snapshots (
    school_id, metric_key, period_kind, period_start, period_end, group_by, dim_value,
    numerator, denominator, value, source_as_of, computed_at
  ) VALUES (
    v_school_id, 'collection_rate', 'month', '2026-09-01', '2026-09-30', '', '',
    440, 500, 8800, NOW(), NOW()
  )
  ON CONFLICT (school_id, metric_key, period_kind, period_start, group_by, dim_value)
  DO NOTHING;

  -- Active Students: 485
  INSERT INTO metric_snapshots (
    school_id, metric_key, period_kind, period_start, period_end, group_by, dim_value,
    value, source_as_of, computed_at
  ) VALUES (
    v_school_id, 'active_students', 'month', '2026-09-01', '2026-09-30', '', '',
    485, NOW(), NOW()
  )
  ON CONFLICT (school_id, metric_key, period_kind, period_start, group_by, dim_value)
  DO NOTHING;

  -- Active Teachers: 38
  INSERT INTO metric_snapshots (
    school_id, metric_key, period_kind, period_start, period_end, group_by, dim_value,
    value, source_as_of, computed_at
  ) VALUES (
    v_school_id, 'active_teachers', 'month', '2026-09-01', '2026-09-30', '', '',
    38, NOW(), NOW()
  )
  ON CONFLICT (school_id, metric_key, period_kind, period_start, group_by, dim_value)
  DO NOTHING;

  -- Verified Achievements: 12
  INSERT INTO metric_snapshots (
    school_id, metric_key, period_kind, period_start, period_end, group_by, dim_value,
    value, source_as_of, computed_at
  ) VALUES (
    v_school_id, 'verified_achievements', 'month', '2026-09-01', '2026-09-30', '', '',
    12, NOW(), NOW()
  )
  ON CONFLICT (school_id, metric_key, period_kind, period_start, group_by, dim_value)
  DO NOTHING;

END $$;
