#!/usr/bin/env python3
"""Migrates the exam domain (Kelola Tipe Ujian, Bank Soal, Buat Ujian Baru,
attempts+answers) from the old Laravel/nusaedu prod schema into
exam-service.

Requires id_maps_identity.json and id_maps_academic.json (run
migrate_identity.py, migrate_academic.py, migrate_schedules.py first).

Structural notes (old -> new mismatches, and how they're resolved here):

- exam_types: 3 of the 6 old rows are "global" (school_id NULL, reused
  across schools); the new schema requires a school_id. One new row is
  created per (old_type_id, old_school_id) pair actually referenced by an
  exam, not one per old row.

- Bank Soal: the old schema has no question bank — exam_questions are
  created directly under one exam, never reused. The new schema requires
  every question to live in a question_folder. One folder per
  (school, academic_year) is created lazily ("Migrasi Ujian Lama"), and
  every old exam_question becomes its own new `questions` bank row inside
  it (not actually shared across exams, but structurally valid).

- classroom_id/subject_id: the old schema allows one exam to target
  multiple classroom+subject combos via exam_sub_classroom_subject
  (up to 78 in one outlier case); the new schema is exactly one
  classroom+subject per exam. Only the FIRST associated combo is used;
  extra associations are dropped (documented data loss, unavoidable given
  the new engine's single-classroom-per-exam design).

- teacher_id: old exams has no teacher column. Derived from the resolved
  classroom_subject's teacher_user_id, falling back to the exam's
  created_by, falling back to the school's first admin user.

- exam_attempts.classroom_id: old attempts have no classroom column
  either (new schema denormalizes it from the token at issue time, but
  tokens aren't migrated — they're ephemeral access codes with no
  historical value). Set to the parent exam's own classroom_id.

- exam_attempt_answers.is_correct: not stored directly in the old schema
  for multiple_choice; derived by checking whether selected_option_id's
  is_correct flag was true. For essay, left as NULL unless graded
  (points_awarded already reflects grading, manually_graded=true).
"""
import json
import uuid
import psycopg2
import psycopg2.extras
from psycopg2.extras import execute_values

SRC = dict(host="localhost", port=5499, user="nusaedu_user", password=os.environ.get("SRC_DB_PASSWORD", ""), dbname="nusaedu")
DST_ID = dict(host="localhost", port=5480, user="sekoci", password="change-me", dbname="skci_identity")
DST_AC = dict(host="localhost", port=5480, user="sekoci", password="change-me", dbname="skci_academic")
DST_EX = dict(host="localhost", port=5480, user="sekoci", password="change-me", dbname="skci_exam")


def load_maps():
    with open("id_maps_identity.json") as f:
        idm = json.load(f)
    with open("id_maps_academic.json") as f:
        acm = json.load(f)
    return (
        {int(k): v for k, v in idm["schools"].items()},
        {int(k): v for k, v in idm["users"].items()},
        {int(k): v for k, v in acm["terms"].items()},
        {str(k): v for k, v in acm["classroom_subjects"].items()},
    )


def school_admin_fallback(dst_id):
    """school_id (new uuid) -> some admin user id, for created_by/teacher_id fallbacks."""
    with dst_id.cursor() as c:
        c.execute("""
            SELECT usr.school_id, usr.user_id FROM user_school_roles usr
            WHERE usr.role = 'admin'
            ORDER BY usr.school_id, usr.user_id
        """)
        out = {}
        for school_id, user_id in c.fetchall():
            out.setdefault(str(school_id), str(user_id))
        return out


def main():
    src = psycopg2.connect(**SRC)
    dst_id = psycopg2.connect(**DST_ID)
    dst_ac = psycopg2.connect(**DST_AC)
    dst_ex = psycopg2.connect(**DST_EX)
    school_map, user_map, term_map, cs_map = load_maps()
    admin_fallback = school_admin_fallback(dst_id)

    with dst_ac.cursor(cursor_factory=psycopg2.extras.RealDictCursor) as c:
        c.execute("SELECT id, school_id, academic_year_id, classroom_id, subject_id, teacher_user_id FROM classroom_subjects")
        cs_rows = {str(r["id"]): r for r in c.fetchall()}
        c.execute("SELECT id, academic_year_id FROM terms")
        term_ay = {str(r["id"]): str(r["academic_year_id"]) for r in c.fetchall()}

    def map_user(old_id):
        if not old_id:
            return None
        return user_map.get(old_id)

    # ================= exam_types =================
    with src.cursor(cursor_factory=psycopg2.extras.RealDictCursor) as sc:
        sc.execute("SELECT id, name, grade_mapping, school_id, created_by FROM exam_types")
        et_rows = {r["id"]: r for r in sc.fetchall()}
        sc.execute("""
            SELECT DISTINCT e.exam_type_id, e.school_id
            FROM exams e WHERE e.deleted_at IS NULL AND e.exam_type_id IS NOT NULL AND e.school_id IS NOT NULL
        """)
        used_pairs = sc.fetchall()

    exam_type_map = {}  # (old_type_id, old_school_id) -> new uuid
    et_insert = []
    for pair in used_pairs:
        old_type_id, old_school_id = pair["exam_type_id"], pair["school_id"]
        et = et_rows.get(old_type_id)
        new_school = school_map.get(old_school_id)
        if not et or not new_school:
            continue
        created_by = None
        try:
            created_by = map_user(int(et["created_by"])) if et["created_by"] else None
        except (ValueError, TypeError):
            created_by = None
        created_by = created_by or admin_fallback.get(new_school)
        if not created_by:
            continue
        new_id = str(uuid.uuid4())
        exam_type_map[(old_type_id, old_school_id)] = new_id
        with dst_id.cursor() as c:
            c.execute("SELECT current_academic_year_id FROM schools WHERE id = %s", (new_school,))
            row = c.fetchone()
            ay_id = str(row[0]) if row and row[0] else None
        if not ay_id:
            continue
        et_insert.append((new_id, new_school, ay_id, et["name"], et["grade_mapping"] or "", created_by))

    with dst_ex.cursor() as c:
        execute_values(c, """INSERT INTO exam_types (id, school_id, academic_year_id, name, mapping_rapor, created_by)
            VALUES %s""", et_insert)
    dst_ex.commit()
    print(f"exam_types: {len(et_insert)}")

    # ================= question_folders (lazy, one per school+year) =================
    folder_map = {}  # (school_uuid, ay_uuid) -> folder id

    def get_folder(school_uuid, ay_uuid, created_by):
        key = (school_uuid, ay_uuid)
        if key in folder_map:
            return folder_map[key]
        fid = str(uuid.uuid4())
        with dst_ex.cursor() as c:
            c.execute("""INSERT INTO question_folders (id, school_id, academic_year_id, name, description, created_by)
                VALUES (%s,%s,%s,%s,%s,%s)""",
                (fid, school_uuid, ay_uuid, "Migrasi Ujian Lama", "Soal hasil migrasi dari sistem lama", created_by))
        folder_map[key] = fid
        return fid

    # ================= exams =================
    with src.cursor(cursor_factory=psycopg2.extras.RealDictCursor) as sc:
        sc.execute("""
            SELECT id, school_id, title, description, term_id, start_datetime, end_datetime,
                   is_published, is_shuffled, created_by, updated_at, exam_type_id
            FROM exams WHERE deleted_at IS NULL ORDER BY id
        """)
        exam_rows = sc.fetchall()
        sc.execute("""
            SELECT exam_id, sub_classroom_subject_id FROM exam_sub_classroom_subject ORDER BY exam_id, id
        """)
        exam_scs = {}
        for r in sc.fetchall():
            exam_scs.setdefault(r["exam_id"], r["sub_classroom_subject_id"])

    exam_map = {}  # old exam id -> (new_uuid, classroom_id, subject_id, school_id, ay_id)
    exam_insert = []
    skip_no_school = skip_no_term = skip_no_scs = skip_no_dates = skip_no_type = 0

    for r in exam_rows:
        new_school = school_map.get(r["school_id"])
        if not new_school:
            skip_no_school += 1
            continue
        new_term = term_map.get(r["term_id"]) if r["term_id"] else None
        if not new_term or new_term not in term_ay:
            skip_no_term += 1
            continue
        ay_id = term_ay[new_term]
        scs_old = exam_scs.get(r["id"])
        cs_uuid = cs_map.get(str(scs_old)) if scs_old else None
        cs = cs_rows.get(cs_uuid) if cs_uuid else None
        if not cs:
            skip_no_scs += 1
            continue
        if not r["start_datetime"] or not r["end_datetime"]:
            skip_no_dates += 1
            continue
        et_key = (r["exam_type_id"], r["school_id"])
        new_type = exam_type_map.get(et_key)
        if not new_type:
            skip_no_type += 1
            continue
        created_by = map_user(r["created_by"]) or cs.get("teacher_user_id") and str(cs["teacher_user_id"]) or admin_fallback.get(new_school)
        teacher_id = (str(cs["teacher_user_id"]) if cs.get("teacher_user_id") else None) or created_by
        if not created_by or not teacher_id:
            continue
        new_id = str(uuid.uuid4())
        exam_map[r["id"]] = (new_id, str(cs["classroom_id"]), str(cs["subject_id"]), new_school, ay_id)
        published_at = r["updated_at"] if r["is_published"] else None
        exam_insert.append((
            new_id, new_school, ay_id, new_term, new_type, r["title"], r["description"],
            str(cs["classroom_id"]), str(cs["subject_id"]), teacher_id,
            r["start_datetime"], r["end_datetime"], r["is_shuffled"], r["is_published"],
            published_at, created_by,
        ))

    with dst_ex.cursor() as c:
        execute_values(c, """INSERT INTO exams (id, school_id, academic_year_id, term_id, exam_type_id, title,
            description, classroom_id, subject_id, teacher_id, start_at, end_at, is_shuffled, is_published,
            published_at, created_by) VALUES %s""", exam_insert)
    dst_ex.commit()
    print(f"exams: {len(exam_insert)} (skipped: no_school={skip_no_school} no_term={skip_no_term} "
          f"no_classroom_subject={skip_no_scs} no_dates={skip_no_dates} no_type={skip_no_type})")

    # ================= questions + answer_options + essay_answer_options + exam_questions =================
    with src.cursor(cursor_factory=psycopg2.extras.RealDictCursor) as sc:
        sc.execute("""
            SELECT id, exam_id, question_type, content, answer_key_essay, points, order_index,
                   media_questions, kd_number, learning_outcome, competency_indicator, level_kognitif, created_by
            FROM exam_questions ORDER BY exam_id, order_index
        """)
        eq_rows = sc.fetchall()

    question_map = {}   # old exam_question id -> new questions.id (bank item)
    examq_map = {}       # old exam_question id -> new exam_questions.id (join row)
    q_insert, eq_insert = [], []
    skip_no_exam = 0

    TYPE_MAP = {"multiple_choice": "pilihan_ganda", "essay": "essay"}
    exam_order_counter = {}  # new_exam_id -> next order_index; old order_index isn't reliably unique/sequential per exam in source

    for r in eq_rows:
        exam_info = exam_map.get(r["exam_id"])
        if not exam_info:
            skip_no_exam += 1
            continue
        new_exam_id, _, _, school_uuid, ay_uuid = exam_info
        created_by = map_user(r["created_by"]) or admin_fallback.get(school_uuid)
        if not created_by:
            continue
        folder_id = get_folder(school_uuid, ay_uuid, created_by)
        qtype = TYPE_MAP.get(r["question_type"], "essay")
        new_q_id = str(uuid.uuid4())
        question_map[r["id"]] = new_q_id
        q_insert.append((
            new_q_id, school_uuid, ay_uuid, folder_id, created_by, qtype, r["content"],
            r["media_questions"], r["kd_number"], r["learning_outcome"], r["competency_indicator"],
            r["level_kognitif"], r["points"] or 1, True,
        ))
        new_eq_id = str(uuid.uuid4())
        examq_map[r["id"]] = new_eq_id
        order_idx = exam_order_counter.get(new_exam_id, 0)
        exam_order_counter[new_exam_id] = order_idx + 1
        eq_insert.append((new_eq_id, new_exam_id, new_q_id, order_idx, r["points"] or 1))

    with dst_ex.cursor() as c:
        execute_values(c, """INSERT INTO questions (id, school_id, academic_year_id, folder_id, created_by,
            question_type, content, image_url, no_kd, konten_kd, indikator_pencapaian, level_kognitif, poin,
            is_published) VALUES %s""", q_insert, page_size=1000)
        execute_values(c, """INSERT INTO exam_questions (id, exam_id, question_id, order_index, points)
            VALUES %s""", eq_insert, page_size=1000)
    dst_ex.commit()
    print(f"questions: {len(q_insert)}, exam_questions: {len(eq_insert)} (skipped no_exam={skip_no_exam})")

    # ================= answer_options (MC) + essay_answer_options =================
    with src.cursor(cursor_factory=psycopg2.extras.RealDictCursor) as sc:
        sc.execute("""
            SELECT id, exam_question_id, content, is_correct, order_index, media_answer_options
            FROM exam_question_has_answer_options ORDER BY exam_question_id, order_index
        """)
        opt_rows = sc.fetchall()

    option_map = {}  # old option id -> new answer_options.id
    opt_insert = []
    skip_no_q = 0
    for r in opt_rows:
        new_q = question_map.get(r["exam_question_id"])
        if not new_q:
            skip_no_q += 1
            continue
        new_opt_id = str(uuid.uuid4())
        option_map[r["id"]] = new_opt_id
        opt_insert.append((new_opt_id, new_q, r["content"], r["media_answer_options"], r["is_correct"], r["order_index"] or 0))

    with dst_ex.cursor() as c:
        execute_values(c, """INSERT INTO answer_options (id, question_id, content, image_url, is_correct, order_index)
            VALUES %s""", opt_insert, page_size=1000)
    dst_ex.commit()
    print(f"answer_options: {len(opt_insert)} (skipped no_question={skip_no_q})")

    essay_insert = []
    for r in eq_rows:
        new_q = question_map.get(r["id"])
        if new_q and r["answer_key_essay"]:
            essay_insert.append((str(uuid.uuid4()), new_q, r["answer_key_essay"]))
    with dst_ex.cursor() as c:
        execute_values(c, "INSERT INTO essay_answer_options (id, question_id, answer_text) VALUES %s", essay_insert)
    dst_ex.commit()
    print(f"essay_answer_options: {len(essay_insert)}")

    # ================= exam_attempts =================
    STATUS_MAP = {"banned": "dilarang", "blocked": "blokir", "completed": "selesai",
                  "graded": "selesai", "in_progress": "aktif"}
    with src.cursor(cursor_factory=psycopg2.extras.RealDictCursor) as sc:
        sc.execute("""
            SELECT id, exam_id, user_id, start_datetime, submit_datetime, graded_at, score, correct_count,
                   status, blocked_at, blocked_reason, blocked_by, banned_at, banned_reason, banned_by
            FROM exam_attempts ORDER BY id
        """)
        att_rows = sc.fetchall()

    attempt_map = {}
    att_insert = []
    skip_no_exam2 = skip_no_user = 0
    for r in att_rows:
        exam_info = exam_map.get(r["exam_id"])
        if not exam_info:
            skip_no_exam2 += 1
            continue
        new_exam_id, classroom_id, _, school_uuid, ay_uuid = exam_info
        new_user = map_user(r["user_id"])
        if not new_user:
            skip_no_user += 1
            continue
        new_id = str(uuid.uuid4())
        attempt_map[r["id"]] = new_id
        status = STATUS_MAP.get(r["status"], "selesai")
        att_insert.append((
            new_id, school_uuid, ay_uuid, new_exam_id, new_user, classroom_id, status,
            r["start_datetime"], r["submit_datetime"], r["blocked_at"], r["blocked_reason"],
            map_user(r["blocked_by"]), r["banned_at"], r["banned_reason"], map_user(r["banned_by"]),
            r["score"], r["correct_count"], r["graded_at"],
        ))

    with dst_ex.cursor() as c:
        execute_values(c, """INSERT INTO exam_attempts (id, school_id, academic_year_id, exam_id, user_id,
            classroom_id, status, started_at, submitted_at, blocked_at, blocked_reason, blocked_by,
            banned_at, banned_reason, banned_by, score, correct_count, graded_at) VALUES %s""",
            att_insert, page_size=500)
    dst_ex.commit()
    print(f"exam_attempts: {len(att_insert)} (skipped no_exam={skip_no_exam2} no_user={skip_no_user})")

    # ================= exam_attempt_answers =================
    with src.cursor(cursor_factory=psycopg2.extras.RealDictCursor) as sc:
        sc.execute("""
            SELECT exam_attempt_id, exam_question_id, selected_option_id, essay_answer, points_awarded, updated_at
            FROM exam_attempt_answers ORDER BY exam_attempt_id
        """)
        ans_rows = sc.fetchall()

    correct_option_ids = {v for k, v in option_map.items()}
    with dst_ex.cursor(cursor_factory=psycopg2.extras.RealDictCursor) as c:
        c.execute("SELECT id, is_correct FROM answer_options")
        opt_is_correct = {str(r["id"]): r["is_correct"] for r in c.fetchall()}

    ans_insert = []
    skip_no_att = skip_no_eq = 0
    for r in ans_rows:
        new_att = attempt_map.get(r["exam_attempt_id"])
        if not new_att:
            skip_no_att += 1
            continue
        new_eq = examq_map.get(r["exam_question_id"])
        if not new_eq:
            skip_no_eq += 1
            continue
        new_opt = option_map.get(r["selected_option_id"]) if r["selected_option_id"] else None
        is_correct = None
        manually_graded = False
        if new_opt:
            is_correct = opt_is_correct.get(new_opt)
        elif r["essay_answer"] is not None:
            manually_graded = r["points_awarded"] is not None
            if manually_graded:
                is_correct = (r["points_awarded"] or 0) > 0
        ans_insert.append((
            str(uuid.uuid4()), new_att, new_eq, new_opt, r["essay_answer"], is_correct,
            r["points_awarded"], manually_graded, r["updated_at"],
        ))

    with dst_ex.cursor() as c:
        execute_values(c, """INSERT INTO exam_attempt_answers (id, attempt_id, exam_question_id, selected_option_id,
            essay_answer, is_correct, points_awarded, manually_graded, answered_at)
            VALUES %s ON CONFLICT (attempt_id, exam_question_id) DO NOTHING""", ans_insert, page_size=1000)
    dst_ex.commit()
    print(f"exam_attempt_answers: {len(ans_insert)} (skipped no_attempt={skip_no_att} no_exam_question={skip_no_eq})")


if __name__ == "__main__":
    main()
