#!/usr/bin/env python3
"""Migrates the evaluation domain (RPM / Data RPM, Penilaian Guru) from the
old Laravel/nusaedu prod schema into evaluation-service.

Requires id_maps_identity.json + id_maps_academic.json.

Structural notes:

- RPM (lesson_plans): old rows join through a `programs` table for
  school_id/academic_year_id/teacher_id (lesson_plans itself only has
  program_id + free-form JSON fields). Rows whose program_id doesn't
  resolve (dangling FK, found in real data) are skipped. The old
  learning_objective/activity_core/teacher_reflection fields don't map
  1:1 onto the new section_a/b/c framework (a different curriculum
  template), so they're preserved verbatim as a "legacy" key inside each
  section rather than force-fit into the new form fields — content isn't
  lost, but won't pre-fill the new RPM form's specific inputs.

- Penilaian Guru (teacher_evaluation_*): old teacher_evaluation_forms/
  items are GLOBAL (no school_id) reference/master data; the new schema
  scopes both per-school (UNIQUE(school_id, evaluator_type)). One copy of
  the 6 forms + their items is seeded per real school from the old master
  data, then periods/tasks/answers are migrated referencing each period's
  own school's copy.

- evaluator_type mapping: old headmaster/curriculum_vp/student_affairs_vp/
  student -> new kepala_sekolah/waka_kurikulum/waka_kesiswaan/siswa
  (peer/self pass through unchanged).

- teacher_evaluation_periods.status ('draft'|'published'|'closed') maps to
  the new boolean `published` (true for published or closed).
"""
import json
import uuid
import psycopg2
import psycopg2.extras
from psycopg2.extras import execute_values

SRC = dict(host="localhost", port=5499, user="nusaedu_user", password=os.environ.get("SRC_DB_PASSWORD", ""), dbname="nusaedu")
DST_ID = dict(host="localhost", port=5480, user="sekoci", password="change-me", dbname="skci_identity")
DST_AC = dict(host="localhost", port=5480, user="sekoci", password="change-me", dbname="skci_academic")
DST_EV = dict(host="localhost", port=5480, user="sekoci", password="change-me", dbname="skci_evaluation")

EVALUATOR_TYPE_MAP = {
    "headmaster": "kepala_sekolah", "curriculum_vp": "waka_kurikulum",
    "student_affairs_vp": "waka_kesiswaan", "student": "siswa",
    "peer": "peer", "self": "self",
}


def load_maps():
    with open("id_maps_identity.json") as f:
        idm = json.load(f)
    with open("id_maps_academic.json") as f:
        acm = json.load(f)
    return (
        {int(k): v for k, v in idm["schools"].items()},
        {int(k): v for k, v in idm["users"].items()},
        {int(k): v for k, v in acm["academic_years"].items()},
        {int(k): v for k, v in acm["classrooms"].items()},
    )


def main():
    src = psycopg2.connect(**SRC)
    dst_id = psycopg2.connect(**DST_ID)
    dst_ac = psycopg2.connect(**DST_AC)
    dst_ev = psycopg2.connect(**DST_EV)
    school_map, user_map, ay_map, classroom_map = load_maps()

    with dst_id.cursor(cursor_factory=psycopg2.extras.RealDictCursor) as c:
        c.execute("SELECT id FROM schools")
        real_schools = [str(r["id"]) for r in c.fetchall()]

    def map_user(old_id):
        return user_map.get(old_id) if old_id else None

    # ================= RPM (lesson_plans) =================
    with src.cursor(cursor_factory=psycopg2.extras.RealDictCursor) as sc:
        sc.execute("""
            SELECT lp.id, lp.group, lp.learning_objective, lp.activity_core, lp.teacher_reflection, lp.created_by,
                   p.school_id, p.academic_year_id, p.teacher_id, p.topic, p.week, p.month, p.semester
            FROM lesson_plans lp JOIN programs p ON p.id = lp.program_id
            WHERE lp.deleted_at IS NULL AND p.deleted_at IS NULL ORDER BY lp.id
        """)
        lp_rows = sc.fetchall()

    lp_insert = []
    skip_no_school = skip_no_teacher = 0
    for r in lp_rows:
        new_school = school_map.get(r["school_id"])
        new_ay = ay_map.get(r["academic_year_id"])
        if not new_school or not new_ay:
            skip_no_school += 1
            continue
        teacher = map_user(r["teacher_id"]) or map_user(r["created_by"])
        if not teacher:
            skip_no_teacher += 1
            continue
        title = r["topic"] or f"RPM Minggu {r['week']}" if r["week"] else (r["topic"] or "RPM")
        section_a = {"legacy_group": r["group"], "legacy_topic": r["topic"], "legacy_week": r["week"],
                     "legacy_month": r["month"], "legacy_semester": r["semester"]}
        section_b = {"legacy_learning_objective": r["learning_objective"]}
        section_c = {"legacy_activity_core": r["activity_core"], "legacy_teacher_reflection": r["teacher_reflection"]}
        lp_insert.append((
            str(uuid.uuid4()), new_school, new_ay, teacher, title,
            json.dumps(section_a), json.dumps(section_b), json.dumps(section_c), "final",
        ))

    with dst_ev.cursor() as c:
        execute_values(c, """INSERT INTO lesson_plans (id, school_id, academic_year_id, guru_user_id, title,
            section_a, section_b, section_c, status) VALUES %s""", lp_insert)
    dst_ev.commit()
    print(f"lesson_plans: {len(lp_insert)} (skipped: no_school/year={skip_no_school} no_teacher={skip_no_teacher})")

    # ================= Seed forms + items per real school (from old global master data) =================
    with src.cursor(cursor_factory=psycopg2.extras.RealDictCursor) as sc:
        sc.execute("SELECT id, code, title, evaluator_type, priority, weight FROM teacher_evaluation_forms")
        old_forms = sc.fetchall()
        sc.execute("""SELECT id, form_id, code, indicator, indicator_en, weight, score_4_label, score_3_label,
            score_2_label, score_1_label, sort_order FROM teacher_evaluation_items ORDER BY form_id, sort_order""")
        old_items = sc.fetchall()

    form_map = {}   # (school_uuid, old_form_id) -> new form uuid
    item_map = {}   # old_item_id -> new item uuid (item codes are globally unique in old schema, but map per new form anyway)
    form_insert, item_insert = [], []

    for school_uuid in real_schools:
        for f in old_forms:
            new_form_id = str(uuid.uuid4())
            form_map[(school_uuid, f["id"])] = new_form_id
            new_type = EVALUATOR_TYPE_MAP.get(f["evaluator_type"], f["evaluator_type"])
            form_insert.append((new_form_id, school_uuid, f["code"], f["title"], new_type, f["priority"], f["weight"]))

    for it in old_items:
        for school_uuid in real_schools:
            new_form_id = form_map.get((school_uuid, it["form_id"]))
            if not new_form_id:
                continue
            new_item_id = str(uuid.uuid4())
            item_map[(school_uuid, it["id"])] = new_item_id
            labels = {"4": it["score_4_label"], "3": it["score_3_label"], "2": it["score_2_label"], "1": it["score_1_label"]}
            item_insert.append((new_item_id, new_form_id, school_uuid, it["code"], it["indicator"], it["indicator_en"],
                                 it["weight"], 4, json.dumps(labels), it["sort_order"]))

    with dst_ev.cursor() as c:
        execute_values(c, """INSERT INTO teacher_evaluation_forms (id, school_id, code, title, evaluator_type,
            priority, weight) VALUES %s""", form_insert)
        execute_values(c, """INSERT INTO teacher_evaluation_items (id, form_id, school_id, code, indicator,
            indicator_en, weight, max_score, score_labels, sort_order) VALUES %s""", item_insert)
    dst_ev.commit()
    print(f"teacher_evaluation_forms: {len(form_insert)}, teacher_evaluation_items: {len(item_insert)}")

    # ================= teacher_evaluation_periods =================
    with src.cursor(cursor_factory=psycopg2.extras.RealDictCursor) as sc:
        sc.execute("""
            SELECT id, school_id, title, semester, starts_at, ends_at, status, headmaster_user_id,
                   curriculum_vp_user_id, student_affairs_vp_user_id, target_teacher_ids, peer_assignments,
                   student_assignments, created_by, updated_by
            FROM teacher_evaluation_periods WHERE deleted_at IS NULL ORDER BY id
        """)
        period_rows = sc.fetchall()

    # academic_year_id is NOT NULL on the new table; resolve via the school's current year (old data has no direct link)
    with dst_id.cursor() as c:
        c.execute("SELECT id, current_academic_year_id FROM schools")
        school_ay = {str(r[0]): str(r[1]) for r in c.fetchall() if r[1]}

    period_map = {}
    period_insert = []
    skip_no_school2 = 0
    for r in period_rows:
        new_school = school_map.get(r["school_id"])
        new_ay = school_ay.get(new_school) if new_school else None
        if not new_school or not new_ay:
            skip_no_school2 += 1
            continue
        new_id = str(uuid.uuid4())
        period_map[r["id"]] = (new_id, new_school)
        target_ids = [map_user(int(x)) for x in (r["target_teacher_ids"] or []) if map_user(int(x))]
        # old peer_assignments/student_assignments are structured objects (not flat id
        # arrays like the new schema's peer_evaluatee_ids/peer_evaluator_ids/
        # student_classroom_ids); the real per-evaluator assignments are preserved
        # faithfully via teacher_evaluation_tasks below regardless, so these summary
        # arrays are left empty rather than shoehorning mismatched old data into them.
        published = r["status"] in ("published", "closed")
        period_insert.append((
            new_id, new_school, new_ay, r["title"], r["semester"], r["starts_at"], r["ends_at"], published,
            map_user(r["headmaster_user_id"]), map_user(r["curriculum_vp_user_id"]), map_user(r["student_affairs_vp_user_id"]),
            json.dumps(target_ids), json.dumps([]), json.dumps([]), json.dumps([]), "",
            map_user(r["created_by"]), map_user(r["updated_by"]),
        ))

    with dst_ev.cursor() as c:
        execute_values(c, """INSERT INTO teacher_evaluation_periods (id, school_id, academic_year_id, title,
            semester, publish_start_date, publish_end_date, published, headmaster_user_id, curriculum_vp_user_id,
            student_affairs_vp_user_id, target_teacher_ids, peer_evaluatee_ids, peer_evaluator_ids,
            student_classroom_ids, lead_assessor_name, created_by, updated_by) VALUES %s""", period_insert)
    dst_ev.commit()
    print(f"teacher_evaluation_periods: {len(period_insert)} (skipped no_school={skip_no_school2})")

    # ================= teacher_evaluation_tasks + answers =================
    with src.cursor(cursor_factory=psycopg2.extras.RealDictCursor) as sc:
        sc.execute("""
            SELECT t.id, t.period_id, t.form_id, t.target_teacher_user_id, t.evaluator_user_id, t.evaluator_type,
                   t.sub_classroom_id, t.status, t.submitted_at, t.raw_score, p.school_id
            FROM teacher_evaluation_tasks t JOIN teacher_evaluation_periods p ON p.id = t.period_id
            WHERE p.deleted_at IS NULL ORDER BY t.id
        """)
        task_rows = sc.fetchall()

    task_map = {}
    task_insert = []
    skip = 0
    for r in task_rows:
        period_info = period_map.get(r["period_id"])
        if not period_info:
            skip += 1
            continue
        new_period_id, new_school = period_info
        new_form_id = form_map.get((new_school, r["form_id"]))
        target = map_user(r["target_teacher_user_id"])
        evaluator = map_user(r["evaluator_user_id"])
        if not new_form_id or not target or not evaluator:
            skip += 1
            continue
        new_id = str(uuid.uuid4())
        task_map[r["id"]] = new_id
        classroom_id = classroom_map.get(r["sub_classroom_id"]) if r["sub_classroom_id"] else None
        new_type = EVALUATOR_TYPE_MAP.get(r["evaluator_type"], r["evaluator_type"])
        task_insert.append((
            new_id, new_period_id, new_form_id, new_school, school_ay.get(new_school), target, evaluator,
            new_type, classroom_id, r["status"], r["submitted_at"], r["raw_score"],
        ))

    with dst_ev.cursor() as c:
        execute_values(c, """INSERT INTO teacher_evaluation_tasks (id, period_id, form_id, school_id,
            academic_year_id, target_teacher_user_id, evaluator_user_id, evaluator_type, classroom_id, status,
            submitted_at, raw_score) VALUES %s ON CONFLICT DO NOTHING""", task_insert, page_size=500)
    dst_ev.commit()
    print(f"teacher_evaluation_tasks: {len(task_insert)} (skipped={skip})")

    with src.cursor(cursor_factory=psycopg2.extras.RealDictCursor) as sc:
        sc.execute("""
            SELECT a.task_id, a.item_id, a.score, t.period_id, p.school_id
            FROM teacher_evaluation_answers a
            JOIN teacher_evaluation_tasks t ON t.id = a.task_id
            JOIN teacher_evaluation_periods p ON p.id = t.period_id
            WHERE p.deleted_at IS NULL
        """)
        ans_rows = sc.fetchall()

    ans_insert = []
    skip_ans = 0
    for r in ans_rows:
        new_task = task_map.get(r["task_id"])
        period_info = period_map.get(r["period_id"])
        if not new_task or not period_info:
            skip_ans += 1
            continue
        _, new_school = period_info
        new_item = item_map.get((new_school, r["item_id"]))
        if not new_item:
            skip_ans += 1
            continue
        ans_insert.append((str(uuid.uuid4()), new_task, new_item, new_school, r["score"]))

    with dst_ev.cursor() as c:
        execute_values(c, """INSERT INTO teacher_evaluation_answers (id, task_id, item_id, school_id, score)
            VALUES %s ON CONFLICT (task_id, item_id) DO NOTHING""", ans_insert, page_size=1000)
    dst_ev.commit()
    print(f"teacher_evaluation_answers: {len(ans_insert)} (skipped={skip_ans})")


if __name__ == "__main__":
    main()
