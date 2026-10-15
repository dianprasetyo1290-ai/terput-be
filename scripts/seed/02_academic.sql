-- 02_academic.sql
-- Seed classrooms, subjects, classroom subjects, and enrollments for Teratai Putih Global School

DO $$
DECLARE
  v_school_id UUID := '11111111-1111-1111-1111-111111111111';
  v_ay_id     UUID := '12121212-1212-1212-1212-121212121212';
  v_walikelas UUID := '88888888-8888-8888-8888-888888888888';
  v_guru      UUID := '77777777-7777-7777-7777-777777777777';
  v_siswa     UUID := '44444444-4444-4444-4444-444444444444';
  v_class_7a  UUID;
  v_class_7b  UUID;
  v_class_8a  UUID;
  v_class_9a  UUID;
  v_sub_mtk   UUID;
  v_sub_indo  UUID;
  v_sub_ing   UUID;
  v_sub_ipa   UUID;
  v_sub_ips   UUID;
  v_sub_info  UUID;
BEGIN
  -- 1. Classrooms
  INSERT INTO classrooms (school_id, academic_year_id, name, grade_level, homeroom_teacher_user_id, capacity)
  VALUES (v_school_id, v_ay_id, 'Kelas 7A', '7', v_walikelas, 32)
  ON CONFLICT DO NOTHING;
  SELECT id INTO v_class_7a FROM classrooms WHERE school_id = v_school_id AND academic_year_id = v_ay_id AND name = 'Kelas 7A' LIMIT 1;

  INSERT INTO classrooms (school_id, academic_year_id, name, grade_level, homeroom_teacher_user_id, capacity)
  VALUES (v_school_id, v_ay_id, 'Kelas 7B', '7', v_guru, 32)
  ON CONFLICT DO NOTHING;
  SELECT id INTO v_class_7b FROM classrooms WHERE school_id = v_school_id AND academic_year_id = v_ay_id AND name = 'Kelas 7B' LIMIT 1;

  INSERT INTO classrooms (school_id, academic_year_id, name, grade_level, homeroom_teacher_user_id, capacity)
  VALUES (v_school_id, v_ay_id, 'Kelas 8A', '8', v_guru, 32)
  ON CONFLICT DO NOTHING;
  SELECT id INTO v_class_8a FROM classrooms WHERE school_id = v_school_id AND academic_year_id = v_ay_id AND name = 'Kelas 8A' LIMIT 1;

  INSERT INTO classrooms (school_id, academic_year_id, name, grade_level, homeroom_teacher_user_id, capacity)
  VALUES (v_school_id, v_ay_id, 'Kelas 9A', '9', v_guru, 32)
  ON CONFLICT DO NOTHING;
  SELECT id INTO v_class_9a FROM classrooms WHERE school_id = v_school_id AND academic_year_id = v_ay_id AND name = 'Kelas 9A' LIMIT 1;

  -- 2. Subjects
  INSERT INTO subjects (school_id, name, description, kelompok)
  VALUES (v_school_id, 'Matematika', 'Matematika Fase D Kurikulum Merdeka', 'umum')
  ON CONFLICT DO NOTHING;
  SELECT id INTO v_sub_mtk FROM subjects WHERE school_id = v_school_id AND name = 'Matematika' LIMIT 1;

  INSERT INTO subjects (school_id, name, description, kelompok)
  VALUES (v_school_id, 'Bahasa Indonesia', 'Bahasa dan Sastra Indonesia', 'umum')
  ON CONFLICT DO NOTHING;
  SELECT id INTO v_sub_indo FROM subjects WHERE school_id = v_school_id AND name = 'Bahasa Indonesia' LIMIT 1;

  INSERT INTO subjects (school_id, name, description, kelompok)
  VALUES (v_school_id, 'Bahasa Inggris', 'English Communication', 'umum')
  ON CONFLICT DO NOTHING;
  SELECT id INTO v_sub_ing FROM subjects WHERE school_id = v_school_id AND name = 'Bahasa Inggris' LIMIT 1;

  INSERT INTO subjects (school_id, name, description, kelompok)
  VALUES (v_school_id, 'Ilmu Pengetahuan Alam (IPA)', 'Fisika, Biologi, Kimia Terpadu', 'umum')
  ON CONFLICT DO NOTHING;
  SELECT id INTO v_sub_ipa FROM subjects WHERE school_id = v_school_id AND name = 'Ilmu Pengetahuan Alam (IPA)' LIMIT 1;

  INSERT INTO subjects (school_id, name, description, kelompok)
  VALUES (v_school_id, 'Informatika', 'Coding, Algoritma & Literasi Digital', 'umum')
  ON CONFLICT DO NOTHING;
  SELECT id INTO v_sub_info FROM subjects WHERE school_id = v_school_id AND name = 'Informatika' LIMIT 1;

  -- 3. Classroom Subjects
  IF v_class_7a IS NOT NULL THEN
    IF v_sub_mtk IS NOT NULL THEN
      INSERT INTO classroom_subjects (school_id, academic_year_id, classroom_id, subject_id, teacher_user_id, hours_per_week)
      VALUES (v_school_id, v_ay_id, v_class_7a, v_sub_mtk, v_guru, 4)
      ON CONFLICT (classroom_id, subject_id) DO NOTHING;
    END IF;
    IF v_sub_indo IS NOT NULL THEN
      INSERT INTO classroom_subjects (school_id, academic_year_id, classroom_id, subject_id, teacher_user_id, hours_per_week)
      VALUES (v_school_id, v_ay_id, v_class_7a, v_sub_indo, v_walikelas, 4)
      ON CONFLICT (classroom_id, subject_id) DO NOTHING;
    END IF;
    IF v_sub_ing IS NOT NULL THEN
      INSERT INTO classroom_subjects (school_id, academic_year_id, classroom_id, subject_id, teacher_user_id, hours_per_week)
      VALUES (v_school_id, v_ay_id, v_class_7a, v_sub_ing, v_guru, 3)
      ON CONFLICT (classroom_id, subject_id) DO NOTHING;
    END IF;
    IF v_sub_info IS NOT NULL THEN
      INSERT INTO classroom_subjects (school_id, academic_year_id, classroom_id, subject_id, teacher_user_id, hours_per_week)
      VALUES (v_school_id, v_ay_id, v_class_7a, v_sub_info, v_guru, 2)
      ON CONFLICT (classroom_id, subject_id) DO NOTHING;
    END IF;
  END IF;

  -- 4. Enroll Budi Santoso into Kelas 7A
  IF v_class_7a IS NOT NULL THEN
    INSERT INTO class_enrollments (school_id, academic_year_id, classroom_id, student_user_id, status)
    VALUES (v_school_id, v_ay_id, v_class_7a, v_siswa, 'active')
    ON CONFLICT (academic_year_id, student_user_id) DO UPDATE SET classroom_id = v_class_7a, status = 'active';
  END IF;

END $$;
