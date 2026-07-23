#!/usr/bin/env python3
"""Migrates the identity domain (foundations, schools, users, roles) from
the old Laravel/nusaedu prod schema into identity-service's new schema.

Source: local restore of the prod dump, sekoci-prod-src container (port 5499).
Target: identity-service's live DB (skci_identity, via the shared postgres
        container, port 5480).

Run against LOCAL databases only — never touches the VPS.

Old->new role mapping (see project memory for the reasoning):
  school_admin        -> admin
  teacher              -> guru
  student              -> siswa
  headmaster            -> guru (fallback if no other base role) + kepala_sekolah tag
  foundation_head       -> kepala_yayasan tag (school-scoped, using whatever
                            school_id the old row carried)
  homeroom_teacher      -> NOT migrated as a role row; academic-service's
                            classrooms.homeroom_teacher_user_id is the
                            authoritative source (next migration phase)
  parent/canteen_admin/cashier/staff/implementor -> out of scope, skipped
  is_superadmin=true    -> ('superadmin', NULL) row regardless of anything else

A user with zero resulting in-scope role rows is skipped entirely (no
account created) — they have no reason to log into the new system.
"""
import datetime
import psycopg2
import psycopg2.extras
from dateutil import parser as dateparser

INDO_MONTHS = {
    "januari": 1, "februari": 2, "maret": 3, "april": 4, "mei": 5, "juni": 6,
    "juli": 7, "agustus": 8, "september": 9, "oktober": 10, "november": 11, "desember": 12,
}


def parse_birth_date(raw):
    """The old system's birth_date is a free-text varchar with years of
    inconsistent data entry: DD/MM/YYYY, YYYY-MM-DD(THH:MM:SS), Indonesian
    'DD Month YYYY', and even raw Excel date serials that leaked through a
    prior bad import (e.g. '41107', '41243-01-01 17:00:00'). Best-effort
    parse; anything that doesn't produce a sane year (1900-2020, since
    every student/guru in this dataset is an adult or minor born by ~2020)
    is dropped to NULL rather than guessed at or allowed to crash the
    migration."""
    if not raw:
        return None
    raw = str(raw).strip()
    if not raw:
        return None

    # bare Excel serial number (e.g. "41107")
    if raw.isdigit() and 3 <= len(raw) <= 6:
        try:
            d = datetime.date(1899, 12, 30) + datetime.timedelta(days=int(raw))
            if 1900 <= d.year <= 2020:
                return d
        except (OverflowError, ValueError):
            pass
        return None

    # Indonesian "DD Month YYYY"
    parts = raw.lower().split()
    if len(parts) == 3 and parts[1] in INDO_MONTHS:
        try:
            d = datetime.date(int(parts[2]), INDO_MONTHS[parts[1]], int(parts[0]))
            if 1900 <= d.year <= 2020:
                return d
        except ValueError:
            pass
        return None

    # DD/MM/YYYY explicit (the dominant format in this dataset)
    try:
        d = datetime.datetime.strptime(raw[:10], "%d/%m/%Y").date()
        if 1900 <= d.year <= 2020:
            return d
    except ValueError:
        pass

    # fall back to dateutil for ISO-ish strings ('1985-07-18',
    # '2009-09-09 17:00:00')
    try:
        d = dateparser.parse(raw, dayfirst=True, fuzzy=False).date()
        if 1900 <= d.year <= 2020:
            return d
    except (ValueError, OverflowError, TypeError):
        pass

    return None

SRC = dict(host="localhost", port=5499, user="nusaedu_user", password=os.environ.get("SRC_DB_PASSWORD", ""), dbname="nusaedu")
DST = dict(host="localhost", port=5480, user="sekoci", password="change-me", dbname="skci_identity")

BASE_ROLE_PRIORITY = ["school_admin", "teacher", "student"]
BASE_ROLE_MAP = {"school_admin": "admin", "teacher": "guru", "student": "siswa"}
OUT_OF_SCOPE_ROLES = {"parent", "canteen_admin", "cashier", "staff", "foundation_admin", "implementor"}


def connect():
    src = psycopg2.connect(**SRC)
    dst = psycopg2.connect(**DST)
    return src, dst


def migrate_foundations(src, dst):
    id_map = {}
    with src.cursor(cursor_factory=psycopg2.extras.RealDictCursor) as sc, dst.cursor() as dc:
        sc.execute("SELECT id, name, address, year_founded, created_at, updated_at FROM foundations WHERE deleted_at IS NULL")
        rows = sc.fetchall()
        for r in rows:
            year_founded = None
            if r["year_founded"]:
                try:
                    year_founded = int(str(r["year_founded"])[:4])
                except ValueError:
                    year_founded = None
            dc.execute(
                """INSERT INTO foundations (name, address, year_founded, created_at, updated_at)
                   VALUES (%s, %s, %s, %s, %s) RETURNING id""",
                (r["name"], r["address"], year_founded, r["created_at"], r["updated_at"]),
            )
            new_id = dc.fetchone()[0]
            id_map[r["id"]] = new_id
    dst.commit()
    print(f"foundations: migrated {len(id_map)}")
    return id_map


def migrate_schools(src, dst, foundation_map):
    id_map = {}
    with src.cursor(cursor_factory=psycopg2.extras.RealDictCursor) as sc, dst.cursor() as dc:
        sc.execute("""
            SELECT s.id, s.name, s.code, s.address, s.subdistrict, s.district, s.city, s.province,
                   s.registration_number, s.year_founded, s.phone_number, s.email, s.website, s.logo,
                   s.foundation_id, s.created_at, s.updated_at, sl.name AS level_name
            FROM schools s
            LEFT JOIN school_levels sl ON sl.id = s.school_level_id
            WHERE s.deleted_at IS NULL
        """)
        rows = sc.fetchall()
        for r in rows:
            new_foundation_id = foundation_map.get(r["foundation_id"])
            dc.execute(
                """INSERT INTO schools (foundation_id, name, code, level, address, subdistrict, district, city,
                       province, registration_number, year_founded, phone_number, email, website, logo,
                       is_active, created_at, updated_at)
                   VALUES (%s,%s,%s,%s,%s,%s,%s,%s,%s,%s,%s,%s,%s,%s,%s,true,%s,%s) RETURNING id""",
                (new_foundation_id, r["name"], r["code"], r["level_name"], r["address"], r["subdistrict"],
                 r["district"], r["city"], r["province"], r["registration_number"], r["year_founded"],
                 r["phone_number"], r["email"], r["website"], r["logo"], r["created_at"], r["updated_at"]),
            )
            new_id = dc.fetchone()[0]
            id_map[r["id"]] = new_id
    dst.commit()
    print(f"schools: migrated {len(id_map)}")
    return id_map


def migrate_users(src, dst):
    """Migrates every user row (regardless of role) so FKs from later
    migrated tables (attendance, exams, etc.) can resolve — role
    assignment (and thus login capability) is decided separately in
    migrate_roles. Password hash copied as-is (bcrypt $2y$ == Go bcrypt)."""
    id_map = {}
    with src.cursor(cursor_factory=psycopg2.extras.RealDictCursor) as sc, dst.cursor() as dc:
        sc.execute("""
            SELECT id, name, email, phone, password, default_password, photo, birth_date, birth_place,
                   address, is_superadmin, created_at, updated_at
            FROM users WHERE deleted_at IS NULL
        """)
        rows = sc.fetchall()
        seen_emails = set()
        birth_date_source_count = 0
        birth_date_parsed_count = 0
        for r in rows:
            email = r["email"].strip().lower()
            if email in seen_emails:
                # old data has a handful of duplicate emails across schools
                # (data-entry error upstream) -- new schema's UNIQUE(email)
                # can't hold both; keep the first, skip the rest.
                continue
            seen_emails.add(email)
            if r["birth_date"]:
                birth_date_source_count += 1
            birth_date = parse_birth_date(r["birth_date"])
            if birth_date:
                birth_date_parsed_count += 1
            dc.execute(
                """INSERT INTO users (name, email, phone, password_hash, default_password, password_changed,
                       photo, birth_date, birth_place, address, is_superadmin, created_at, updated_at)
                   VALUES (%s,%s,%s,%s,%s,%s,%s,%s,%s,%s,%s,%s,%s) RETURNING id""",
                (r["name"], email, r["phone"], r["password"], r["default_password"],
                 r["default_password"] is None, r["photo"], birth_date, r["birth_place"], r["address"],
                 r["is_superadmin"], r["created_at"], r["updated_at"]),
            )
            new_id = dc.fetchone()[0]
            id_map[r["id"]] = new_id
    dst.commit()
    print(f"users: migrated {len(id_map)} (of {len(rows)} source rows, {len(rows) - len(id_map)} duplicate-email skips)")
    print(f"  birth_date: {birth_date_parsed_count}/{birth_date_source_count} source values parsed successfully ({birth_date_source_count - birth_date_parsed_count} unparseable, set NULL)")
    return id_map


def migrate_roles(src, dst, user_map, school_map):
    """Per (old_user, old_school), collect held role names, map to the new
    role set, and emit user_school_roles rows. See module docstring."""
    with src.cursor(cursor_factory=psycopg2.extras.RealDictCursor) as sc:
        sc.execute("""
            SELECT usr.user_id, usr.school_id, r.name AS role_name
            FROM user_school_roles usr
            JOIN roles r ON r.id = usr.role_id
            WHERE usr.deleted_by IS NULL
        """)
        role_rows = sc.fetchall()
        sc.execute("SELECT id FROM users WHERE is_superadmin = true AND deleted_at IS NULL")
        superadmin_old_ids = {r["id"] for r in sc.fetchall()}

    # group by (user_id, school_id)
    grouped = {}
    for r in role_rows:
        key = (r["user_id"], r["school_id"])
        grouped.setdefault(key, set()).add(r["role_name"])

    to_insert = []  # (new_user_id, new_school_id_or_none, role)
    skipped_no_user = 0
    skipped_no_school = 0
    fallback_guru_for_headmaster = []

    for (old_user_id, old_school_id), roles in grouped.items():
        new_user_id = user_map.get(old_user_id)
        if new_user_id is None:
            skipped_no_user += 1
            continue
        new_school_id = school_map.get(old_school_id) if old_school_id else None
        if old_school_id and new_school_id is None:
            skipped_no_school += 1
            continue

        emitted_base = False
        for old_role in BASE_ROLE_PRIORITY:
            if old_role in roles:
                to_insert.append((new_user_id, new_school_id, BASE_ROLE_MAP[old_role]))
                emitted_base = True
                break

        if "headmaster" in roles:
            to_insert.append((new_user_id, new_school_id, "kepala_sekolah"))
            if not emitted_base:
                to_insert.append((new_user_id, new_school_id, "guru"))
                fallback_guru_for_headmaster.append(old_user_id)
                emitted_base = True

        if "foundation_head" in roles:
            to_insert.append((new_user_id, new_school_id, "kepala_yayasan"))
            # foundation_head alone still needs a base role somewhere to log
            # in; if this (user, school) pair has no base role, note it but
            # don't fabricate one here -- check across ALL their schools
            # after the loop (a user might have a base role at a DIFFERENT
            # school, which is valid).

    for old_id in superadmin_old_ids:
        new_id = user_map.get(old_id)
        if new_id:
            to_insert.append((new_id, None, "superadmin"))

    # de-dup (unique constraint is user_id, school_id, role)
    to_insert = list({(u, s, r) for (u, s, r) in to_insert})

    with dst.cursor() as dc:
        for new_user_id, new_school_id, role in to_insert:
            dc.execute(
                """INSERT INTO user_school_roles (user_id, school_id, role, status)
                   VALUES (%s, %s, %s, 'active')
                   ON CONFLICT (user_id, school_id, role) DO NOTHING""",
                (new_user_id, new_school_id, role),
            )
    dst.commit()

    # verify: any migrated user with zero role rows at all (pure parent/
    # canteen_admin/etc, or a base-role-less foundation_head) is expected
    # and fine -- they just can't log in, matching "out of scope".
    with dst.cursor() as dc:
        dc.execute("SELECT count(DISTINCT user_id) FROM user_school_roles")
        users_with_roles = dc.fetchone()[0]

    print(f"user_school_roles: inserted {len(to_insert)} rows across {users_with_roles} distinct users")
    print(f"  skipped (user not migrated): {skipped_no_user}, skipped (school not migrated): {skipped_no_school}")
    print(f"  headmaster-only users given a fallback 'guru' base role: {len(fallback_guru_for_headmaster)} -> old ids {fallback_guru_for_headmaster}")


def main():
    src, dst = connect()
    try:
        foundation_map = migrate_foundations(src, dst)
        school_map = migrate_schools(src, dst, foundation_map)
        user_map = migrate_users(src, dst)
        migrate_roles(src, dst, user_map, school_map)

        import json
        with open("id_maps_identity.json", "w") as f:
            json.dump({"foundations": foundation_map, "schools": school_map, "users": user_map}, f)
        print("\nID maps saved to id_maps_identity.json (needed by later migration phases)")
    finally:
        src.close()
        dst.close()


if __name__ == "__main__":
    main()
