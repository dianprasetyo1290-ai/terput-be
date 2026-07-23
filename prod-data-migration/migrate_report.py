#!/usr/bin/env python3
"""Migrates the report domain (E-Rapor: report cards + subject scores)
from the old Laravel/nusaedu prod schema into report-service.

Requires id_maps_identity.json + id_maps_academic.json.

Structural note: the old `report_cards` table is already keyed at
(term, student, sub_classroom_subject) granularity — i.e. one row per
SUBJECT, matching the new schema's report_card_subjects, not its
report_cards header. Old rows are grouped by (term, student, classroom)
[classroom derived via the subject's classroom_subject] into one new
report_cards header row per group, with one report_card_subjects row per
old row in that group. class_teacher_note (a per-subject field in the old
schema) is taken from the first non-empty value in the group and promoted
to the new schema's per-header class_teacher_note.
"""
import json
import uuid
import psycopg2
import psycopg2.extras
from psycopg2.extras import execute_values

SRC = dict(host="localhost", port=5499, user="nusaedu_user", password=os.environ.get("SRC_DB_PASSWORD", ""), dbname="nusaedu")
DST_ID = dict(host="localhost", port=5480, user="sekoci", password="change-me", dbname="skci_identity")
DST_AC = dict(host="localhost", port=5480, user="sekoci", password="change-me", dbname="skci_academic")
DST_RP = dict(host="localhost", port=5480, user="sekoci", password="change-me", dbname="skci_report")


def load_maps():
    with open("id_maps_identity.json") as f:
        idm = json.load(f)
    with open("id_maps_academic.json") as f:
        acm = json.load(f)
    return (
        {int(k): v for k, v in idm["users"].items()},
        {int(k): v for k, v in acm["terms"].items()},
        {str(k): v for k, v in acm["classroom_subjects"].items()},
    )


def main():
    src = psycopg2.connect(**SRC)
    dst_id = psycopg2.connect(**DST_ID)
    dst_ac = psycopg2.connect(**DST_AC)
    dst_rp = psycopg2.connect(**DST_RP)
    user_map, term_map, cs_map = load_maps()

    with dst_id.cursor(cursor_factory=psycopg2.extras.RealDictCursor) as c:
        c.execute("SELECT id, name FROM users")
        student_names = {str(r["id"]): r["name"] for r in c.fetchall()}

    with dst_ac.cursor(cursor_factory=psycopg2.extras.RealDictCursor) as c:
        c.execute("SELECT id, school_id, academic_year_id, classroom_id, subject_id FROM classroom_subjects")
        cs_rows = {str(r["id"]): r for r in c.fetchall()}
        c.execute("SELECT id, name FROM subjects")
        subj_names = {str(r["id"]): r["name"] for r in c.fetchall()}
        c.execute("SELECT id, name FROM classrooms")
        classroom_names = {str(r["id"]): r["name"] for r in c.fetchall()}
        c.execute("SELECT id, name FROM terms")
        term_names = {str(r["id"]): r["name"] for r in c.fetchall()}

    with src.cursor(cursor_factory=psycopg2.extras.RealDictCursor) as sc:
        sc.execute("""
            SELECT id, school_id, term_id, student_user_id, sub_classroom_subject_id, final_score,
                   revision_score, grade, mid_exported_at, final_exported_at, class_teacher_note
            FROM report_cards WHERE deleted_at IS NULL ORDER BY term_id, student_user_id
        """)
        rc_rows = sc.fetchall()

    groups = {}  # (new_term, new_student, new_classroom) -> {rows: [...], school, ay}
    skip_no_term = skip_no_student = skip_no_cs = 0

    for r in rc_rows:
        new_term = term_map.get(r["term_id"])
        if not new_term:
            skip_no_term += 1
            continue
        new_student = user_map.get(r["student_user_id"])
        if not new_student:
            skip_no_student += 1
            continue
        cs_uuid = cs_map.get(str(r["sub_classroom_subject_id"])) if r["sub_classroom_subject_id"] else None
        cs = cs_rows.get(cs_uuid) if cs_uuid else None
        if not cs:
            skip_no_cs += 1
            continue
        key = (new_term, new_student, str(cs["classroom_id"]))
        g = groups.setdefault(key, {"school_id": str(cs["school_id"]), "academic_year_id": str(cs["academic_year_id"]), "rows": []})
        g["rows"].append((r, cs))

    rc_insert = []
    rcs_insert = []
    rc_id_by_key = {}

    for (new_term, new_student, new_classroom), g in groups.items():
        rc_id = str(uuid.uuid4())
        rc_id_by_key[(new_term, new_student, new_classroom)] = rc_id
        notes = [r["class_teacher_note"] for r, _ in g["rows"] if r["class_teacher_note"]]
        finalized = any(r["mid_exported_at"] or r["final_exported_at"] for r, _ in g["rows"])
        finalized_at = None
        for r, _ in g["rows"]:
            ts = r["final_exported_at"] or r["mid_exported_at"]
            if ts and (finalized_at is None or ts > finalized_at):
                finalized_at = ts
        rc_insert.append((
            rc_id, g["school_id"], g["academic_year_id"], new_term, new_classroom, new_student,
            "finalized" if finalized else "draft", finalized_at,
            notes[0] if notes else None,
            None, None, None, None,
            student_names.get(new_student), None, classroom_names.get(new_classroom), term_names.get(new_term),
        ))
        for r, cs in g["rows"]:
            score = r["revision_score"] if r["revision_score"] is not None else r["final_score"]
            rcs_insert.append((
                str(uuid.uuid4()), rc_id, g["school_id"], str(cs["subject_id"]),
                subj_names.get(str(cs["subject_id"])), score, r["final_score"] if r["revision_score"] is not None else None,
                1, r["grade"],
            ))

    with dst_rp.cursor() as c:
        execute_values(c, """INSERT INTO report_cards (id, school_id, academic_year_id, term_id, classroom_id,
            student_id, status, finalized_at, class_teacher_note, attendance_hadir, attendance_sakit,
            attendance_izin, attendance_tanpa_keterangan, student_name, student_nisn, classroom_name, term_name)
            VALUES %s ON CONFLICT (student_id, classroom_id, term_id) DO NOTHING""", rc_insert)
    dst_rp.commit()

    with dst_rp.cursor() as c:
        execute_values(c, """INSERT INTO report_card_subjects (id, report_card_id, school_id, subject_id,
            subject_name, computed_score, override_score, source_exam_count, grade_label)
            VALUES %s ON CONFLICT (report_card_id, subject_id) DO NOTHING""", rcs_insert)
    dst_rp.commit()

    print(f"report_cards: {len(rc_insert)} (grouped from {len(rc_rows)} old rows)")
    print(f"report_card_subjects: {len(rcs_insert)}")
    print(f"skipped: no_term={skip_no_term} no_student={skip_no_student} no_classroom_subject={skip_no_cs}")


if __name__ == "__main__":
    main()
