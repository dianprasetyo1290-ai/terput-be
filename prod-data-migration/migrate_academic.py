#!/usr/bin/env python3
"""Migrates the academic domain from the old Laravel/nusaedu prod schema
into identity-service (academic_years) and academic-service (terms,
classrooms, subjects, classroom_subjects, class_enrollments).

Requires id_maps_identity.json from migrate_identity.py (run that first).

Structural note: the old schema has a two-level classroom/sub_classroom
split (e.g. classroom "X IPA" -> sub_classrooms "X IPA 1", "X IPA 2" via a
`sequence` field); the new schema flattens this to one level. Each old
sub_classroom becomes one new `classrooms` row, named
"{classroom.name} {sub_classroom.sequence}".

Roster: the old schema puts the student's current classroom directly on
users.sub_classroom_id (not a separate pivot table) — that's the source
for class_enrollments here.

academic_years edge cases found in the real data: some schools have TWO
rows both marked is_current=true (data entry error upstream), one school
has NONE marked current. Resolved by picking the row with the latest
start_date as "the" current year per school in both cases.
"""
import json
import psycopg2
import psycopg2.extras

SRC = dict(host="localhost", port=5499, user="nusaedu_user", password=os.environ.get("SRC_DB_PASSWORD", ""), dbname="nusaedu")
DST_ID = dict(host="localhost", port=5480, user="sekoci", password="change-me", dbname="skci_identity")
DST_AC = dict(host="localhost", port=5480, user="sekoci", password="change-me", dbname="skci_academic")


def load_id_maps():
    with open("id_maps_identity.json") as f:
        maps = json.load(f)
    # JSON keys are strings; old ids are ints in the source DB
    return {
        "schools": {int(k): v for k, v in maps["schools"].items()},
        "users": {int(k): v for k, v in maps["users"].items()},
    }


def migrate_academic_years(src, dst_id, school_map):
    """Owned by identity-service. Returns ({old_academic_year_id: new_uuid},
    {old_school_id: new_current_academic_year_uuid}) and sets each school's
    current_academic_year_id."""
    id_map = {}
    current_ay_by_old_school = {}
    with src.cursor(cursor_factory=psycopg2.extras.RealDictCursor) as sc:
        sc.execute("""
            SELECT id, school_id, name, start_date, end_date, is_current
            FROM academic_years WHERE deleted_at IS NULL ORDER BY school_id, start_date
        """)
        rows = sc.fetchall()

    by_school = {}
    for r in rows:
        by_school.setdefault(r["school_id"], []).append(r)

    with dst_id.cursor() as dc:
        for old_school_id, years in by_school.items():
            new_school_id = school_map.get(old_school_id)
            if not new_school_id:
                continue
            # resolve "the" current year: prefer is_current=true rows, break
            # ties (or fill the "none marked" gap) with latest start_date
            current_candidates = [y for y in years if y["is_current"]] or years
            current_row = max(current_candidates, key=lambda y: y["start_date"])

            for y in years:
                dc.execute(
                    """INSERT INTO academic_years (school_id, label, is_current, created_at, updated_at)
                       VALUES (%s, %s, %s, now(), now())
                       ON CONFLICT (school_id, label) WHERE deleted_at IS NULL DO UPDATE SET label = EXCLUDED.label
                       RETURNING id""",
                    (new_school_id, y["name"], y["id"] == current_row["id"]),
                )
                id_map[y["id"]] = dc.fetchone()[0]

            dc.execute(
                "UPDATE schools SET current_academic_year_id = %s WHERE id = %s",
                (id_map[current_row["id"]], new_school_id),
            )
            current_ay_by_old_school[old_school_id] = id_map[current_row["id"]]
    dst_id.commit()
    print(f"academic_years: migrated {len(id_map)} across {len(by_school)} schools")
    return id_map, current_ay_by_old_school


def migrate_terms(src, dst_ac, school_map, ay_map):
    id_map = {}
    with src.cursor(cursor_factory=psycopg2.extras.RealDictCursor) as sc:
        sc.execute("""
            SELECT t.id, t.academic_year_id, t.name, t."order", t.start_date, t.end_date, ay.school_id
            FROM terms t JOIN academic_years ay ON ay.id = t.academic_year_id
            WHERE t.deleted_at IS NULL
        """)
        rows = sc.fetchall()

    with dst_ac.cursor() as dc:
        for r in rows:
            new_school_id = school_map.get(r["school_id"])
            new_ay_id = ay_map.get(r["academic_year_id"])
            if not new_school_id or not new_ay_id:
                continue
            dc.execute(
                """INSERT INTO terms (school_id, academic_year_id, name, "order", start_date, end_date, created_at, updated_at)
                   VALUES (%s,%s,%s,%s,%s,%s,now(),now())
                   ON CONFLICT (academic_year_id, "order") DO UPDATE SET name = EXCLUDED.name
                   RETURNING id""",
                (new_school_id, new_ay_id, r["name"], r["order"], r["start_date"], r["end_date"]),
            )
            id_map[r["id"]] = dc.fetchone()[0]
    dst_ac.commit()
    print(f"terms: migrated {len(id_map)}")
    return id_map


def migrate_classrooms(src, dst_ac, school_map, ay_map, user_map, current_ay_by_old_school):
    """Old sub_classrooms -> new classrooms (flattened). Returns
    {old_sub_classroom_id: new_classroom_uuid}.

    Real data has a fair number of classrooms whose parent `classrooms.
    academic_year_id` was simply never set upstream (confirmed: 14/26 real
    sub_classrooms across 4 of 6 schools) — rather than silently dropping
    over half the classrooms, these fall back to the school's current
    academic year."""
    id_map = {}
    with src.cursor(cursor_factory=psycopg2.extras.RealDictCursor) as sc:
        sc.execute("""
            SELECT sc.id, sc.school_id, sc.sequence, sc.kokurikuler_teacher_user_id,
                   c.name AS classroom_name, c.academic_year_id
            FROM sub_classrooms sc
            JOIN classrooms c ON c.id = sc.classroom_id
            WHERE sc.deleted_at IS NULL AND c.deleted_at IS NULL
        """)
        rows = sc.fetchall()

    skipped = 0
    fallback_used = 0
    with dst_ac.cursor() as dc:
        for r in rows:
            new_school_id = school_map.get(r["school_id"])
            new_ay_id = ay_map.get(r["academic_year_id"])
            if not new_ay_id:
                new_ay_id = current_ay_by_old_school.get(r["school_id"])
                if new_ay_id:
                    fallback_used += 1
            if not new_school_id or not new_ay_id:
                skipped += 1
                continue
            name = f"{r['classroom_name']} {r['sequence']}".strip()
            homeroom = user_map.get(r["kokurikuler_teacher_user_id"]) if r["kokurikuler_teacher_user_id"] else None
            dc.execute(
                """INSERT INTO classrooms (school_id, academic_year_id, name, homeroom_teacher_user_id, created_at, updated_at)
                   VALUES (%s,%s,%s,%s,now(),now()) RETURNING id""",
                (new_school_id, new_ay_id, name, homeroom),
            )
            id_map[r["id"]] = dc.fetchone()[0]
    dst_ac.commit()
    print(f"classrooms (from sub_classrooms): migrated {len(id_map)} ({fallback_used} via current-year fallback), skipped {skipped}")
    return id_map


def migrate_subjects(src, dst_ac, school_map):
    id_map = {}
    with src.cursor(cursor_factory=psycopg2.extras.RealDictCursor) as sc:
        sc.execute("SELECT id, school_id, name, description, subject_group FROM subjects WHERE deleted_at IS NULL")
        rows = sc.fetchall()
    with dst_ac.cursor() as dc:
        for r in rows:
            new_school_id = school_map.get(r["school_id"])
            if not new_school_id:
                continue
            dc.execute(
                """INSERT INTO subjects (school_id, name, description, subject_group, created_at, updated_at)
                   VALUES (%s,%s,%s,%s,now(),now()) RETURNING id""",
                (new_school_id, r["name"], r["description"], r["subject_group"]),
            )
            id_map[r["id"]] = dc.fetchone()[0]
    dst_ac.commit()
    print(f"subjects: migrated {len(id_map)}")
    return id_map


def migrate_classroom_subjects(src, dst_ac, school_map, ay_map, classroom_map, subject_map, user_map, current_ay_by_old_school):
    """81% of source rows have no academic_year_id set (same upstream data
    quality issue as classrooms) — same current-year fallback applies."""
    id_map = {}
    with src.cursor(cursor_factory=psycopg2.extras.RealDictCursor) as sc:
        sc.execute("""
            SELECT id, school_id, sub_classroom_id, subject_id, teacher_user_id, academic_year_id
            FROM sub_classroom_subjects WHERE deleted_at IS NULL
        """)
        rows = sc.fetchall()
    skipped = 0
    fallback_used = 0
    with dst_ac.cursor() as dc:
        for r in rows:
            new_school_id = school_map.get(r["school_id"])
            new_ay_id = ay_map.get(r["academic_year_id"])
            if not new_ay_id:
                new_ay_id = current_ay_by_old_school.get(r["school_id"])
                if new_ay_id:
                    fallback_used += 1
            new_classroom_id = classroom_map.get(r["sub_classroom_id"])
            new_subject_id = subject_map.get(r["subject_id"])
            if not all([new_school_id, new_ay_id, new_classroom_id, new_subject_id]):
                skipped += 1
                continue
            new_teacher_id = user_map.get(r["teacher_user_id"]) if r["teacher_user_id"] else None
            dc.execute(
                """INSERT INTO classroom_subjects (school_id, academic_year_id, classroom_id, subject_id, teacher_user_id, is_active, created_at, updated_at)
                   VALUES (%s,%s,%s,%s,%s,true,now(),now())
                   ON CONFLICT (classroom_id, subject_id) DO UPDATE SET teacher_user_id = EXCLUDED.teacher_user_id
                   RETURNING id""",
                (new_school_id, new_ay_id, new_classroom_id, new_subject_id, new_teacher_id),
            )
            id_map[r["id"]] = dc.fetchone()[0]
    dst_ac.commit()
    print(f"classroom_subjects: migrated {len(id_map)} ({fallback_used} via current-year fallback), skipped {skipped}")
    return id_map


def migrate_enrollments(src, dst_ac, school_map, ay_map, classroom_map, user_map, current_ay_by_old_school):
    """Roster source: users.sub_classroom_id + users.academic_year_id (18
    students have a classroom but no academic_year_id — same fallback)."""
    count = 0
    skipped = 0
    fallback_used = 0
    with src.cursor(cursor_factory=psycopg2.extras.RealDictCursor) as sc:
        sc.execute("""
            SELECT u.id AS user_id, u.sub_classroom_id, u.academic_year_id, sc.school_id
            FROM users u
            JOIN sub_classrooms sc ON sc.id = u.sub_classroom_id
            WHERE u.deleted_at IS NULL AND u.sub_classroom_id IS NOT NULL
        """)
        rows = sc.fetchall()

    with dst_ac.cursor() as dc:
        for r in rows:
            new_school_id = school_map.get(r["school_id"])
            new_ay_id = ay_map.get(r["academic_year_id"]) if r["academic_year_id"] else None
            if not new_ay_id:
                new_ay_id = current_ay_by_old_school.get(r["school_id"])
                if new_ay_id:
                    fallback_used += 1
            new_classroom_id = classroom_map.get(r["sub_classroom_id"])
            new_student_id = user_map.get(r["user_id"])
            if not all([new_school_id, new_ay_id, new_classroom_id, new_student_id]):
                skipped += 1
                continue
            dc.execute(
                """INSERT INTO class_enrollments (school_id, academic_year_id, classroom_id, student_user_id, status, enrolled_at, created_at, updated_at)
                   VALUES (%s,%s,%s,%s,'active',now(),now(),now())
                   ON CONFLICT (academic_year_id, student_user_id) DO UPDATE SET classroom_id = EXCLUDED.classroom_id
                   """,
                (new_school_id, new_ay_id, new_classroom_id, new_student_id),
            )
            count += 1
    dst_ac.commit()
    print(f"class_enrollments: migrated {count} ({fallback_used} via current-year fallback), skipped {skipped}")


def main():
    src = psycopg2.connect(**SRC)
    dst_id = psycopg2.connect(**DST_ID)
    dst_ac = psycopg2.connect(**DST_AC)
    try:
        id_maps = load_id_maps()
        school_map = id_maps["schools"]
        user_map = id_maps["users"]

        ay_map, current_ay_by_old_school = migrate_academic_years(src, dst_id, school_map)
        term_map = migrate_terms(src, dst_ac, school_map, ay_map)
        classroom_map = migrate_classrooms(src, dst_ac, school_map, ay_map, user_map, current_ay_by_old_school)
        subject_map = migrate_subjects(src, dst_ac, school_map)
        cs_map = migrate_classroom_subjects(src, dst_ac, school_map, ay_map, classroom_map, subject_map, user_map, current_ay_by_old_school)
        migrate_enrollments(src, dst_ac, school_map, ay_map, classroom_map, user_map, current_ay_by_old_school)

        with open("id_maps_academic.json", "w") as f:
            json.dump({
                "academic_years": ay_map, "terms": term_map,
                "classrooms": classroom_map, "subjects": subject_map,
                "classroom_subjects": cs_map,
            }, f)
        print("\nID maps saved to id_maps_academic.json")
    finally:
        src.close()
        dst_id.close()
        dst_ac.close()


if __name__ == "__main__":
    main()
