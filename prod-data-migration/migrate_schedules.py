#!/usr/bin/env python3
"""Backfills academic-service's schedule_periods + schedules (Jadwal
Pelajaran), which the earlier academic-domain migration missed entirely.
Required before attendance-domain migration, since attendance's
class_sessions.schedule_id references academic-service schedules.id.

Structural note: the old schema's schedule_periods is a school-wide shared
time-slot table (period number + start/end time + day, no classroom_id).
The new schema ties schedule_periods to a specific classroom (per-classroom
bell schedules). The real per-slot record in the old schema is `schedules`
(sub_classroom_subject_id + schedule_period_id + day), so target
schedule_periods rows are synthesized on the fly, one per unique
(classroom_id, day_of_week, period_no) tuple derived by joining old
`schedules` -> `schedule_periods` -> classroom_subjects id_map.
"""
import json
import psycopg2
import psycopg2.extras

SRC = dict(host="localhost", port=5499, user="nusaedu_user", password=os.environ.get("SRC_DB_PASSWORD", ""), dbname="nusaedu")
DST_AC = dict(host="localhost", port=5480, user="sekoci", password="change-me", dbname="skci_academic")


def load_id_maps():
    with open("id_maps_academic.json") as f:
        maps = json.load(f)
    return {k: {str(k2): v2 for k2, v2 in v.items()} for k, v in maps.items()}


def main():
    src = psycopg2.connect(**SRC)
    dst = psycopg2.connect(**DST_AC)
    maps = load_id_maps()
    cs_map = maps["classroom_subjects"]  # old sub_classroom_subjects.id (str) -> new classroom_subjects UUID

    with dst.cursor(cursor_factory=psycopg2.extras.RealDictCursor) as dc:
        dc.execute("SELECT id, school_id, academic_year_id, classroom_id, subject_id FROM classroom_subjects")
        cs_rows = {str(r["id"]): r for r in dc.fetchall()}

    with src.cursor(cursor_factory=psycopg2.extras.RealDictCursor) as sc:
        sc.execute("""
            SELECT s.id, s.sub_classroom_subject_id, s.schedule_period_id, s.day,
                   sp.period, sp.start_time, sp.end_time
            FROM schedules s
            JOIN schedule_periods sp ON sp.id = s.schedule_period_id
            WHERE s.deleted_at IS NULL
            ORDER BY s.id
        """)
        rows = sc.fetchall()

    print(f"source schedules (non-deleted): {len(rows)}")

    period_key_map = {}  # (classroom_id, day_of_week, period_no) -> new schedule_period UUID
    inserted_periods = 0
    inserted_schedules = 0
    skipped_no_cs = 0
    skipped_dup = 0

    with dst.cursor() as dw:
        for r in rows:
            cs_uuid = cs_map.get(str(r["sub_classroom_subject_id"]))
            if not cs_uuid or cs_uuid not in cs_rows:
                skipped_no_cs += 1
                continue
            cs = cs_rows[cs_uuid]
            classroom_id = cs["classroom_id"]
            academic_year_id = cs["academic_year_id"]
            school_id = cs["school_id"]
            day = r["day"] if r["day"] else 1
            period_no = r["period"]
            key = (classroom_id, day, period_no)

            sp_id = period_key_map.get(key)
            if sp_id is None:
                dw.execute(
                    """INSERT INTO schedule_periods (school_id, academic_year_id, classroom_id, day_of_week, period_no, start_time, end_time)
                       VALUES (%s,%s,%s,%s,%s,%s,%s)
                       ON CONFLICT (classroom_id, day_of_week, period_no) DO UPDATE SET updated_at = now()
                       RETURNING id""",
                    (school_id, academic_year_id, classroom_id, day, period_no,
                     str(r["start_time"]), str(r["end_time"])),
                )
                sp_id = dw.fetchone()[0]
                period_key_map[key] = sp_id
                inserted_periods += 1

            try:
                dw.execute(
                    """INSERT INTO schedules (school_id, academic_year_id, classroom_id, schedule_period_id, classroom_subject_id)
                       VALUES (%s,%s,%s,%s,%s)
                       ON CONFLICT (schedule_period_id) DO NOTHING""",
                    (school_id, academic_year_id, classroom_id, sp_id, cs_uuid),
                )
                if dw.rowcount:
                    inserted_schedules += 1
                else:
                    skipped_dup += 1
            except psycopg2.Error as e:
                dst.rollback()
                print(f"  error on old schedule id={r['id']}: {e}")
                continue

    dst.commit()
    print(f"schedule_periods created: {inserted_periods}")
    print(f"schedules created: {inserted_schedules}")
    print(f"skipped (no classroom_subject mapping): {skipped_no_cs}")
    print(f"skipped (duplicate schedule_period slot -> unique constraint): {skipped_dup}")

    # Save schedule id map for attendance migration: old schedules.id -> new schedules.id
    with src.cursor(cursor_factory=psycopg2.extras.RealDictCursor) as sc:
        sc.execute("""
            SELECT s.id AS old_id, s.sub_classroom_subject_id, s.schedule_period_id, s.day, sp.period
            FROM schedules s JOIN schedule_periods sp ON sp.id = s.schedule_period_id
            WHERE s.deleted_at IS NULL
        """)
        src_rows = sc.fetchall()

    with dst.cursor(cursor_factory=psycopg2.extras.RealDictCursor) as dc:
        dc.execute("SELECT id, classroom_id, schedule_period_id, classroom_subject_id FROM schedules")
        new_by_cs_period = {(r["classroom_subject_id"], r["schedule_period_id"]): r["id"] for r in dc.fetchall()}
        dc.execute("SELECT id, classroom_id, day_of_week, period_no FROM schedule_periods")
        sp_by_key = {(r["classroom_id"], r["day_of_week"], r["period_no"]): r["id"] for r in dc.fetchall()}

    schedule_id_map = {}
    for r in src_rows:
        cs_uuid = cs_map.get(str(r["sub_classroom_subject_id"]))
        if not cs_uuid or cs_uuid not in cs_rows:
            continue
        cs = cs_rows[cs_uuid]
        key = (cs["classroom_id"], r["day"] if r["day"] else 1, r["period"])
        sp_id = sp_by_key.get(key)
        if not sp_id:
            continue
        new_id = new_by_cs_period.get((cs_uuid, sp_id))
        if new_id:
            schedule_id_map[str(r["old_id"])] = str(new_id)

    maps["schedules"] = schedule_id_map
    with open("id_maps_academic.json", "w") as f:
        json.dump(maps, f, indent=2)
    print(f"schedule id_map saved: {len(schedule_id_map)} entries")


if __name__ == "__main__":
    main()
