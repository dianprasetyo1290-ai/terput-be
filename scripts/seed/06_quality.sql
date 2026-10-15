-- 06_quality.sql
-- Seed QA instrument, standards, items, indicators, and evidence for Teratai Putih Global School

DO $$
DECLARE
  v_school_id UUID := '11111111-1111-1111-1111-111111111111';
  v_admin_id  UUID := '22222222-2222-2222-2222-222222222222';
  v_inst_id   UUID;
  v_std_1     UUID;
  v_std_2     UUID;
  v_item_1    UUID;
  v_item_2    UUID;
  v_ind_1     UUID;
  v_ind_2     UUID;
BEGIN
  -- 1. QA Instrument (Akreditasi Sekolah)
  INSERT INTO qa_instruments (
    school_id, code, name, issuer, version_label, score_min, score_max, status, effective_from
  ) VALUES (
    v_school_id, 'INST-BANSM-2026', 'Instrumen Akreditasi Satuan Pendidikan (IASP)',
    'BAN-S/M', 'v2026.1', 0, 100, 'active', '2026-01-01'
  )
  ON CONFLICT (school_id, code, version_label) DO NOTHING;
  SELECT id INTO v_inst_id FROM qa_instruments WHERE school_id = v_school_id AND code = 'INST-BANSM-2026' LIMIT 1;

  IF v_inst_id IS NOT NULL THEN
    -- 2. Standards
    INSERT INTO qa_standards (school_id, instrument_id, sort, code, name, weight_bp)
    VALUES (v_school_id, v_inst_id, 1, 'STD-MUTU-LULUSAN', 'Standar Mutu Lulusan', 3000)
    ON CONFLICT DO NOTHING;
    SELECT id INTO v_std_1 FROM qa_standards WHERE instrument_id = v_inst_id AND code = 'STD-MUTU-LULUSAN' LIMIT 1;

    INSERT INTO qa_standards (school_id, instrument_id, sort, code, name, weight_bp)
    VALUES (v_school_id, v_inst_id, 2, 'STD-PROSES-PEMBELAJARAN', 'Standar Proses Pembelajaran', 3500)
    ON CONFLICT DO NOTHING;
    SELECT id INTO v_std_2 FROM qa_standards WHERE instrument_id = v_inst_id AND code = 'STD-PROSES-PEMBELAJARAN' LIMIT 1;

    -- 3. Items
    IF v_std_1 IS NOT NULL THEN
      INSERT INTO qa_items (school_id, standard_id, sort, code, statement)
      VALUES (v_school_id, v_std_1, 1, 'BUTIR-01', 'Siswa menunjukkan perilaku disiplin dalam berbagai situasi.')
      ON CONFLICT DO NOTHING;
      SELECT id INTO v_item_1 FROM qa_items WHERE standard_id = v_std_1 AND code = 'BUTIR-01' LIMIT 1;

      IF v_item_1 IS NOT NULL THEN
        INSERT INTO qa_indicators (
          school_id, item_id, sort, code, name, weight_bp, unit, direction, source_kind, rubric
        ) VALUES (
          v_school_id, v_item_1, 1, 'IND-01-A', 'Tingkat Kepatuhan Tata Tertib & Kehadiran',
          1000, 'score', 'higher_better', 'manual',
          '[{"score": 4, "desc": "Sangat Baik (>=95%)"}, {"score": 3, "desc": "Baik (85-94%)"}, {"score": 2, "desc": "Cukup (75-84%)"}, {"score": 1, "desc": "Kurang (<75%)"}]'::jsonb
        )
        ON CONFLICT DO NOTHING;
      END IF;
    END IF;

    IF v_std_2 IS NOT NULL THEN
      INSERT INTO qa_items (school_id, standard_id, sort, code, statement)
      VALUES (v_school_id, v_std_2, 1, 'BUTIR-02', 'Guru menggunakan metode pembelajaran aktif yang memotivasi siswa.')
      ON CONFLICT DO NOTHING;
      SELECT id INTO v_item_2 FROM qa_items WHERE standard_id = v_std_2 AND code = 'BUTIR-02' LIMIT 1;

      IF v_item_2 IS NOT NULL THEN
        INSERT INTO qa_indicators (
          school_id, item_id, sort, code, name, weight_bp, unit, direction, source_kind, rubric
        ) VALUES (
          v_school_id, v_item_2, 1, 'IND-02-A', 'Penerapan Pembelajaran Berbasis Proyek (PjBL)',
          1000, 'score', 'higher_better', 'manual',
          '[{"score": 4, "desc": "PjBL diterapkan di >80% mata pelajaran"}, {"score": 3, "desc": "PjBL diterapkan di 60-79% mapel"}]'::jsonb
        )
        ON CONFLICT DO NOTHING;
      END IF;
    END IF;

    -- 4. Evidence
    INSERT INTO qa_evidence (school_id, kind, title, url, uploaded_by, status)
    VALUES (
      v_school_id,
      'link',
      'Dokumen Rekapitulasi Presensi dan Kedisiplinan Siswa TPGS 2026',
      'http://43.157.207.147/teratai/attendance',
      v_admin_id,
      'verified'
    )
    ON CONFLICT DO NOTHING;

    INSERT INTO qa_evidence (school_id, kind, title, url, uploaded_by, status)
    VALUES (
      v_school_id,
      'link',
      'Modul Ajar dan RPP Kurikulum Merdeka Fase D',
      'http://43.157.207.147/teratai/academic',
      v_admin_id,
      'verified'
    )
    ON CONFLICT DO NOTHING;

  END IF;

END $$;
