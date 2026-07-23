#!/usr/bin/env python3
"""Migrates the attendance domain (Presensi Siswa) from the old
Laravel/nusaedu prod schema into attendance-service.

Requires id_maps_identity.json + id_maps_academic.json (including the
"schedules" key from migrate_schedules.py — run that first).

Structural notes:

- Old `schedule_attendances` is a flat (schedule_id, user_id, is_present)
  table — no session concept. Confirmed by role lookup that its user_id is
  overwhelmingly siswa (22 students vs 3 teachers in the whole table), so
  every row is treated as a student attendance mark. class_sessions rows
  are synthesized: one per unique (schedule_id, date) pair found in the
  data, with guru_id taken from the schedule's classroom_subject teacher
  (not from any check-in record — old data has no explicit teacher
  check-in event to migrate).

- is_present is a binary (present/absent) in the old schema; the new
  schema has 4 statuses (hadir/sakit/izin/tanpa_keterangan). Without a
  sakit/izin distinction in the source, false maps to
  'tanpa_keterangan' (the safest/most conservative default — treating an
  unmarked absence as unexcused rather than guessing sick/leave).

- schedule_learning_reports (Agenda Pembelajaran Harian, old model: one
  report per schedule_id ever, not per date) is applied to whichever
  class_session(s) were synthesized for that schedule_id.

- Old `attendances` (guru self check-in, no schedule linkage, 3 rows in
  real data) has no equivalent slot in the new per-schedule class_sessions
  model and is not migrated — negligible volume, no reasonable target.
"""
import json
from datetime import datetime
import uuid
import psycopg2
import psycopg2.extras
from psycopg2.extras import execute_values

SRC = dict(host="localhost", port=5499, user="nusaedu_user", password=os.environ.get("SRC_DB_PASSWORD", ""), dbname="nusaedu")
DST_AC = dict(host="localhost", port=5480, user="sekoci", password="change-me", dbname="skci_academic")
DST_AT = dict(host="localhost", port=5480, user="sekoci", password="change-me", dbname="skci_attendance")


def load_maps():
    with open("id_maps_identity.json") as f:
        idm = json.load(f)
    with open("id_maps_academic.json") as f:
        acm = json.load(f)
    return (
        {int(k): v for k, v in idm["users"].items()},
        {int(k): v for k, v in acm["schedules"].items()},
    )


def main():
    src = psycopg2.connect(**SRC)
    dst_ac = psycopg2.connect(**DST_AC)
    dst_at = psycopg2.connect(**DST_AT)
    user_map, schedule_map = load_maps()

    with dst_ac.cursor(cursor_factory=psycopg2.extras.RealDictCursor) as c:
        c.execute("""
            SELECT s.id, s.school_id, s.academic_year_id, s.classroom_id, s.schedule_period_id,
                   cs.subject_id, cs.teacher_user_id, sp.start_time, sp.end_time
            FROM schedules s
            JOIN classroom_subjects cs ON cs.id = s.classroom_subject_id
            JOIN schedule_periods sp ON sp.id = s.schedule_period_id
        """)
        sched_rows = {str(r["id"]): r for r in c.fetchall()}

    with src.cursor(cursor_factory=psycopg2.extras.RealDictCursor) as sc:
        sc.execute("""
            SELECT id, school_id, user_id, schedule_id, checked_in_at, is_present, notes, created_at
            FROM schedule_attendances WHERE deleted_at IS NULL ORDER BY schedule_id, checked_in_at NULLS LAST
        """)
        sa_rows = sc.fetchall()
        sc.execute("""
            SELECT schedule_id, pertemuan, pembahasan, materi_pertemuan, bentuk_kegiatan
            FROM schedule_learning_reports WHERE deleted_at IS NULL
        """)
        report_by_schedule = {r["schedule_id"]: r for r in sc.fetchall()}

    session_map = {}  # (new_schedule_id, date) -> new class_session id
    session_insert = []
    student_insert = []
    skip_no_schedule = skip_no_user = skip_no_teacher = 0

    for r in sa_rows:
        new_sched_id = schedule_map.get(r["schedule_id"])
        sched = sched_rows.get(new_sched_id) if new_sched_id else None
        if not sched:
            skip_no_schedule += 1
            continue
        new_student = user_map.get(r["user_id"])
        if not new_student:
            skip_no_user += 1
            continue
        teacher_id = str(sched["teacher_user_id"]) if sched["teacher_user_id"] else None
        if not teacher_id:
            skip_no_teacher += 1
            continue

        ts = r["checked_in_at"] or r["created_at"]
        session_date = ts.date() if ts else datetime.now().date()
        key = (new_sched_id, session_date)
        session_id = session_map.get(key)
        if session_id is None:
            session_id = str(uuid.uuid4())
            session_map[key] = session_id
            rpt = report_by_schedule.get(r["schedule_id"])
            session_insert.append((
                session_id, str(sched["school_id"]), str(sched["academic_year_id"]), None,
                new_sched_id, str(sched["classroom_id"]), str(sched["subject_id"]) if sched["subject_id"] else None,
                teacher_id, session_date, ts, str(sched["start_time"]), str(sched["end_time"]),
                rpt["pertemuan"] if rpt else None, rpt["pembahasan"] if rpt else None,
                rpt["materi_pertemuan"] if rpt else None,
                json.dumps(rpt["bentuk_kegiatan"]) if rpt and rpt["bentuk_kegiatan"] else "[]",
            ))

        status = "hadir" if r["is_present"] else "tanpa_keterangan"
        student_insert.append((
            str(uuid.uuid4()), session_id, str(sched["school_id"]), str(sched["academic_year_id"]), None,
            str(sched["classroom_id"]), new_student, status, r["notes"], teacher_id,
        ))

    with dst_at.cursor() as c:
        execute_values(c, """INSERT INTO class_sessions (id, school_id, academic_year_id, term_id, schedule_id,
            classroom_id, subject_id, guru_id, session_date, checked_in_at, slot_start_time, slot_end_time,
            pertemuan, pembahasan, materi_pertemuan, bentuk_kegiatan)
            VALUES %s ON CONFLICT (schedule_id, session_date) DO NOTHING""", session_insert)
        execute_values(c, """INSERT INTO student_attendances (id, class_session_id, school_id, academic_year_id,
            term_id, classroom_id, student_id, status, catatan, marked_by)
            VALUES %s ON CONFLICT (class_session_id, student_id) DO NOTHING""", student_insert)
    dst_at.commit()

    print(f"class_sessions: {len(session_insert)}")
    print(f"student_attendances: {len(student_insert)}")
    print(f"skipped: no_schedule={skip_no_schedule} no_user={skip_no_user} no_teacher={skip_no_teacher}")


if __name__ == "__main__":
    main()
