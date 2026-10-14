-- QA-IDN putaran 2: tata kelola akses dan data master.
--   * peran `akuntan` (Accounting, dipetakan QA-ACC di SSO Laravel)
--   * BUG-023  pengecualian SoD lewat Approval Center + pembersihan akun lama
--   * BUG-120  break-glass (akses darurat berbatas waktu)
--   * BUG-119  permintaan hak subjek data (UU PDP No. 27/2022)
--   * BUG-082  matriks izin per peran x modul yang bisa diatur
--   * BUG-073  data referensi (agama, jenis kelamin, pekerjaan orang tua, program)

-- ---------------------------------------------------------------------------
-- Peran akuntan
-- ---------------------------------------------------------------------------
ALTER TABLE user_school_roles DROP CONSTRAINT IF EXISTS user_school_roles_role_check;
ALTER TABLE user_school_roles ADD CONSTRAINT user_school_roles_role_check
    CHECK (role IN (
        'superadmin', 'admin', 'guru', 'siswa', 'orang_tua', 'walikelas', 'kepala_sekolah', 'kepala_yayasan', 'pengawas',
        'hr_admin', 'hr_manager', 'payroll_officer', 'recruiter', 'training_admin', 'auditor', 'finance_manager', 'pegawai',
        'inventory_officer', 'procurement_officer', 'asset_officer', 'pustakawan', 'facility_officer', 'teknisi',
        'transport_officer', 'pendamping_transport', 'pengemudi', 'canteen_admin', 'canteen_operator', 'petugas_uks',
        'konselor', 'kesiswaan', 'event_coordinator', 'pembina_ekskul', 'servicedesk_agent', 'servicedesk_manager',
        'koordinator_pkl', 'pembimbing_pkl', 'pembimbing_industri', 'qa_coordinator', 'operator_dapodik', 'humas', 'bi_analyst',
        'akuntan'
    ));

-- Akuntan menyusun jurnal; auditor memeriksanya dan payroll officer membuat
-- jurnal gaji yang disetujui keuangan.
INSERT INTO role_conflicts (role_a, role_b, reason) VALUES
    ('akuntan', 'auditor', 'Akuntan tidak boleh mengaudit pembukuan yang disusunnya'),
    ('akuntan', 'siswa', 'Akun siswa tidak boleh memegang peran keuangan')
ON CONFLICT (role_a, role_b) DO NOTHING;

-- ---------------------------------------------------------------------------
-- BUG-023: pengecualian SoD yang disetujui lewat core Approval Center
-- ---------------------------------------------------------------------------
CREATE TABLE role_conflict_exceptions (
    id                UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    school_id         UUID NOT NULL REFERENCES schools(id) ON DELETE CASCADE,
    user_id           UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    role              TEXT NOT NULL,
    conflicting_roles TEXT[] NOT NULL,
    reason            TEXT NOT NULL CHECK (length(btrim(reason)) >= 20),
    valid_until       DATE,
    status            TEXT NOT NULL DEFAULT 'pending'
                      CHECK (status IN ('pending', 'approved', 'rejected', 'revoked', 'failed')),
    approval_id       TEXT,
    status_message    TEXT,
    requested_by      UUID REFERENCES users(id) ON DELETE SET NULL,
    decided_by        TEXT,
    decision_comment  TEXT,
    decided_at        TIMESTAMPTZ,
    revoked_by        UUID REFERENCES users(id) ON DELETE SET NULL,
    revoked_at        TIMESTAMPTZ,
    created_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
    CHECK (cardinality(conflicting_roles) > 0)
);
-- One open request/approval per account+role.
CREATE UNIQUE INDEX uq_role_conflict_exceptions_open
    ON role_conflict_exceptions(user_id, role) WHERE status IN ('pending', 'approved');
CREATE INDEX idx_role_conflict_exceptions_school ON role_conflict_exceptions(school_id, status);

-- Every role row removed by the SoD clean-up, so it can be reviewed and
-- restored (down migration or by hand).
CREATE TABLE role_sod_cleanup_log (
    id                  UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_school_role_id UUID NOT NULL,
    user_id             UUID NOT NULL,
    school_id           UUID,
    role                TEXT NOT NULL,
    status              TEXT NOT NULL,
    student_status      TEXT,
    academic_year_id    UUID,
    deactivated_at      TIMESTAMPTZ,
    original_created_at TIMESTAMPTZ NOT NULL,
    removed_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    reason              TEXT NOT NULL,
    restored_at         TIMESTAMPTZ
);
CREATE INDEX idx_role_sod_cleanup_log_user ON role_sod_cleanup_log(user_id);

-- Keep priority when a forbidden pair has to be broken: a lower rank is
-- kept, a higher rank is removed first. Managerial roles survive, a stray
-- auditor/siswa/orang_tua tag on a staff account goes.
CREATE FUNCTION sod_keep_rank(p_role TEXT) RETURNS INT LANGUAGE sql IMMUTABLE AS $$
    SELECT CASE p_role
        WHEN 'superadmin' THEN 1
        WHEN 'admin' THEN 2
        WHEN 'kepala_yayasan' THEN 3
        WHEN 'kepala_sekolah' THEN 4
        WHEN 'finance_manager' THEN 5
        WHEN 'hr_manager' THEN 6
        WHEN 'hr_admin' THEN 7
        WHEN 'guru' THEN 10
        WHEN 'walikelas' THEN 11
        WHEN 'siswa' THEN 20
        WHEN 'orang_tua' THEN 60
        WHEN 'auditor' THEN 70
        ELSE 40
    END
$$;

-- Resolves every forbidden combination not covered by an approved exception
-- by repeatedly removing, per account, the role involved in most conflicts
-- (ties: higher sod_keep_rank first). Removed rows are logged and the
-- account's tokens are revoked so the old role claim dies at once.
-- Returns the number of role names removed.
CREATE FUNCTION sod_resolve_violations(p_reason TEXT) RETURNS INT LANGUAGE plpgsql AS $$
DECLARE
    v_user UUID;
    v_role TEXT;
    v_n    INT := 0;
BEGIN
    LOOP
        SELECT x.user_id, x.role INTO v_user, v_role
        FROM (
            SELECT h.user_id, h.role, count(*) AS c
            FROM (SELECT DISTINCT user_id, role FROM user_school_roles) h
            JOIN (SELECT DISTINCT user_id, role FROM user_school_roles) o
              ON o.user_id = h.user_id AND o.role <> h.role
            JOIN role_conflicts rc
              ON rc.role_a = LEAST(h.role, o.role) AND rc.role_b = GREATEST(h.role, o.role)
            JOIN users u ON u.id = h.user_id AND u.deleted_at IS NULL
            WHERE NOT EXISTS (
                SELECT 1 FROM role_conflict_exceptions e
                WHERE e.user_id = h.user_id AND e.status = 'approved'
                  AND (e.valid_until IS NULL OR e.valid_until >= CURRENT_DATE)
                  AND ((e.role = h.role AND o.role = ANY(e.conflicting_roles))
                    OR (e.role = o.role AND h.role = ANY(e.conflicting_roles))))
            GROUP BY h.user_id, h.role
        ) x
        ORDER BY x.c DESC, sod_keep_rank(x.role) DESC, x.user_id, x.role
        LIMIT 1;
        EXIT WHEN NOT FOUND;

        INSERT INTO role_sod_cleanup_log (user_school_role_id, user_id, school_id, role, status, student_status,
                                          academic_year_id, deactivated_at, original_created_at, reason)
        SELECT id, user_id, school_id, role, status, student_status, academic_year_id, deactivated_at, created_at, p_reason
        FROM user_school_roles WHERE user_id = v_user AND role = v_role;
        DELETE FROM user_school_roles WHERE user_id = v_user AND role = v_role;
        UPDATE users SET tokens_valid_after = now() WHERE id = v_user;
        UPDATE user_sessions SET revoked_at = now(), revoked_reason = 'sod_cleanup'
        WHERE user_id = v_user AND revoked_at IS NULL;
        v_n := v_n + 1;
    END LOOP;
    RETURN v_n;
END
$$;

-- One-time clean-up of accounts created before SoD existed (UAT: the
-- admin@teratai.sch.id account held 41 roles incl. admin + auditor +
-- finance_manager + payroll_officer + orang_tua + siswa).
SELECT sod_resolve_violations('Migrasi 0022: pembersihan kombinasi peran terlarang (BUG-023)');

-- ---------------------------------------------------------------------------
-- BUG-120: break-glass
-- ---------------------------------------------------------------------------
CREATE TABLE break_glass_grants (
    id                  UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    school_id           UUID NOT NULL REFERENCES schools(id) ON DELETE CASCADE,
    user_id             UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    role                TEXT NOT NULL,
    reason              TEXT NOT NULL CHECK (length(btrim(reason)) >= 20),
    incident_ref        TEXT,
    granted_by          UUID REFERENCES users(id) ON DELETE SET NULL,
    starts_at           TIMESTAMPTZ NOT NULL DEFAULT now(),
    expires_at          TIMESTAMPTZ NOT NULL,
    -- true when the grant inserted the user_school_roles row (so ending the
    -- grant removes exactly that row and never a role the user already had).
    role_row_created    BOOLEAN NOT NULL DEFAULT false,
    ended_at            TIMESTAMPTZ,
    end_reason          TEXT CHECK (end_reason IN ('expired', 'revoked')),
    ended_by            UUID REFERENCES users(id) ON DELETE SET NULL,
    summary             JSONB,
    summary_sent_at     TIMESTAMPTZ,
    summary_status      TEXT,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    CHECK (expires_at > starts_at),
    CHECK (expires_at <= starts_at + interval '24 hours')
);
CREATE UNIQUE INDEX uq_break_glass_active ON break_glass_grants(user_id, school_id, role) WHERE ended_at IS NULL;
CREATE INDEX idx_break_glass_due ON break_glass_grants(expires_at) WHERE ended_at IS NULL;

-- ---------------------------------------------------------------------------
-- BUG-119: hak subjek data (akses, koreksi, penghapusan)
-- ---------------------------------------------------------------------------
CREATE TABLE data_subject_requests (
    id               UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    school_id        UUID REFERENCES schools(id) ON DELETE SET NULL,
    subject_user_id  UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    request_type     TEXT NOT NULL CHECK (request_type IN ('access', 'correction', 'deletion')),
    details          TEXT NOT NULL CHECK (length(btrim(details)) >= 10),
    corrections      JSONB,
    status           TEXT NOT NULL DEFAULT 'submitted'
                     CHECK (status IN ('submitted', 'in_review', 'completed', 'rejected')),
    requested_by     UUID REFERENCES users(id) ON DELETE SET NULL,
    requested_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    -- UU PDP: pengendali menanggapi paling lambat 3 x 24 jam.
    due_at           TIMESTAMPTZ NOT NULL DEFAULT now() + interval '72 hours',
    handled_by       UUID REFERENCES users(id) ON DELETE SET NULL,
    handled_at       TIMESTAMPTZ,
    resolution       TEXT,
    export_payload   JSONB,
    updated_at       TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_dsr_school_status ON data_subject_requests(school_id, status, due_at);
CREATE INDEX idx_dsr_subject ON data_subject_requests(subject_user_id);
-- One open request per subject and type.
CREATE UNIQUE INDEX uq_dsr_open ON data_subject_requests(subject_user_id, request_type)
    WHERE status IN ('submitted', 'in_review');

ALTER TABLE users ADD COLUMN anonymized_at TIMESTAMPTZ;

-- ---------------------------------------------------------------------------
-- BUG-082: matriks izin per peran x modul (school_id NULL = semua unit)
-- ---------------------------------------------------------------------------
CREATE TABLE role_permissions (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    school_id    UUID REFERENCES schools(id) ON DELETE CASCADE,
    role         TEXT NOT NULL,
    module       TEXT NOT NULL,
    can_view     BOOLEAN NOT NULL DEFAULT false,
    can_create   BOOLEAN NOT NULL DEFAULT false,
    can_edit     BOOLEAN NOT NULL DEFAULT false,
    can_delete   BOOLEAN NOT NULL DEFAULT false,
    can_approve  BOOLEAN NOT NULL DEFAULT false,
    updated_by   UUID REFERENCES users(id) ON DELETE SET NULL,
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    CHECK (role <> 'superadmin')
);
CREATE UNIQUE INDEX uq_role_permissions
    ON role_permissions(COALESCE(school_id, '00000000-0000-0000-0000-000000000000'::uuid), role, module);

-- ---------------------------------------------------------------------------
-- BUG-073: data referensi
-- ---------------------------------------------------------------------------
CREATE TABLE reference_values (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    school_id   UUID REFERENCES schools(id) ON DELETE CASCADE,
    category    TEXT NOT NULL CHECK (category IN ('agama', 'jenis_kelamin', 'pekerjaan_ortu', 'program')),
    code        TEXT NOT NULL CHECK (code ~ '^[a-z0-9_]{1,40}$'),
    label       TEXT NOT NULL CHECK (length(btrim(label)) BETWEEN 1 AND 100),
    sort_order  INT NOT NULL DEFAULT 0,
    active      BOOLEAN NOT NULL DEFAULT true,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX uq_reference_values_code
    ON reference_values(COALESCE(school_id, '00000000-0000-0000-0000-000000000000'::uuid), category, code);
CREATE UNIQUE INDEX uq_reference_values_label
    ON reference_values(COALESCE(school_id, '00000000-0000-0000-0000-000000000000'::uuid), category, lower(label));

-- National references (Dapodik). Labels for agama match the Agama subject
-- variants academic uses for the subject matrix. Program/jurusan is unit
-- specific, so none is seeded.
INSERT INTO reference_values (category, code, label, sort_order) VALUES
    ('agama', 'islam', 'Islam', 1),
    ('agama', 'kristen', 'Kristen', 2),
    ('agama', 'katolik', 'Katolik', 3),
    ('agama', 'hindu', 'Hindu', 4),
    ('agama', 'buddha', 'Buddha', 5),
    ('agama', 'konghucu', 'Konghucu', 6),
    ('agama', 'kepercayaan', 'Kepercayaan kepada Tuhan YME', 7),
    ('jenis_kelamin', 'l', 'Laki-laki', 1),
    ('jenis_kelamin', 'p', 'Perempuan', 2),
    ('pekerjaan_ortu', 'tidak_bekerja', 'Tidak bekerja', 1),
    ('pekerjaan_ortu', 'nelayan', 'Nelayan', 2),
    ('pekerjaan_ortu', 'petani', 'Petani', 3),
    ('pekerjaan_ortu', 'peternak', 'Peternak', 4),
    ('pekerjaan_ortu', 'pns_tni_polri', 'PNS/TNI/Polri', 5),
    ('pekerjaan_ortu', 'karyawan_swasta', 'Karyawan Swasta', 6),
    ('pekerjaan_ortu', 'pedagang_kecil', 'Pedagang Kecil', 7),
    ('pekerjaan_ortu', 'pedagang_besar', 'Pedagang Besar', 8),
    ('pekerjaan_ortu', 'wiraswasta', 'Wiraswasta', 9),
    ('pekerjaan_ortu', 'wirausaha', 'Wirausaha', 10),
    ('pekerjaan_ortu', 'buruh', 'Buruh', 11),
    ('pekerjaan_ortu', 'pensiunan', 'Pensiunan', 12),
    ('pekerjaan_ortu', 'tki', 'Tenaga Kerja Indonesia', 13),
    ('pekerjaan_ortu', 'meninggal', 'Sudah Meninggal', 14),
    ('pekerjaan_ortu', 'lainnya', 'Lainnya', 99);
