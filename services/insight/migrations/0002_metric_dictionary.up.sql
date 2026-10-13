CREATE TABLE IF NOT EXISTS metric_definitions (
    key TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    formula TEXT NOT NULL,
    unit TEXT NOT NULL CHECK (unit IN ('percent_bp', 'idr', 'count', 'ratio_milli', 'days')),
    owner_service TEXT NOT NULL,
    data_owner TEXT NOT NULL,
    source_metric TEXT NOT NULL,
    allowed_group_by TEXT[] NOT NULL DEFAULT '{}',
    additive BOOLEAN NOT NULL,
    refresh_minutes INT,
    limitations TEXT,
    version INT NOT NULL DEFAULT 1,
    status TEXT NOT NULL CHECK (status IN ('active', 'deprecated'))
);

CREATE TABLE IF NOT EXISTS metric_school_params (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    school_id UUID NOT NULL,
    metric_key TEXT NOT NULL REFERENCES metric_definitions(key),
    params JSONB NOT NULL DEFAULT '{}'::jsonb,
    effective_from DATE NOT NULL,
    effective_to DATE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(school_id, metric_key, effective_from)
);

CREATE TABLE IF NOT EXISTS metric_snapshots (
    school_id UUID NOT NULL,
    metric_key TEXT NOT NULL REFERENCES metric_definitions(key),
    period_kind TEXT NOT NULL CHECK (period_kind IN ('day', 'month', 'term', 'academic_year')),
    period_start DATE NOT NULL,
    period_end DATE NOT NULL,
    group_by TEXT NOT NULL DEFAULT '',
    dim_value TEXT NOT NULL DEFAULT '',
    numerator BIGINT,
    denominator BIGINT,
    value BIGINT,
    source_as_of TIMESTAMPTZ NOT NULL,
    computed_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY(school_id, metric_key, period_kind, period_start, group_by, dim_value)
);

CREATE TABLE IF NOT EXISTS metric_refresh_runs (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    school_id UUID NOT NULL,
    metric_key TEXT NOT NULL REFERENCES metric_definitions(key),
    period_start DATE NOT NULL,
    status TEXT NOT NULL CHECK (status IN ('ok', 'failed')),
    error TEXT,
    started_at TIMESTAMPTZ NOT NULL,
    finished_at TIMESTAMPTZ NOT NULL
);

-- Seed Initial Product Metric Definitions (T24 §2)
INSERT INTO metric_definitions (key, name, description, formula, unit, owner_service, data_owner, source_metric, allowed_group_by, additive, refresh_minutes, limitations, version, status)
VALUES
    ('collection_rate', 'Tingkat Kolektibilitas SPP', 'Persentase pembayaran teralokasi dibanding tagihan jatuh tempo', 'sum(pembayaran_teralokasi) / sum(tagihan_jatuh_tempo) * 10000', 'percent_bp', 'billing', 'Finance / TU', 'collection_rate', ARRAY['month'], false, 60, 'Tidak mencakup tagihan yang belum jatuh tempo', 1, 'active'),
    ('outstanding_idr', 'Total Tunggakan SPP & Biaya', 'Total nominal tagihan jatuh tempo yang belum dibayarkan', 'sum(tagihan_jatuh_tempo - terbayar)', 'idr', 'billing', 'Finance / TU', 'outstanding_idr', ARRAY['aging_bucket'], true, 60, 'Hanya menghitung tagihan dengan status aktif bukan void', 1, 'active'),
    ('billed_idr', 'Total Nominal Diterbitkan', 'Total nominal tagihan yang diterbitkan pada periode tertentu', 'sum(net_idr)', 'idr', 'billing', 'Finance / TU', 'billed_idr', ARRAY['month'], true, 60, NULL, 1, 'active'),
    ('paid_idr', 'Total Penerimaan Kas', 'Total nominal pembayaran terverifikasi yang masuk', 'sum(allocated_idr)', 'idr', 'billing', 'Finance / Kasir', 'paid_idr', ARRAY['month'], true, 60, NULL, 1, 'active'),
    ('ppdb_conversion', 'Konversi Pendaftaran PPDB', 'Rasio calon siswa yang menyelesaikan daftar ulang dibanding total pendaftar', 'sum(daftar_ulang) / sum(total_pendaftar) * 10000', 'percent_bp', 'admission', 'Panitia PPDB', 'ppdb_conversion', ARRAY['wave'], false, 120, 'Hanya menghitung pendaftar pada gelombang yang sudah dimulai', 1, 'active'),
    ('ppdb_applicants', 'Jumlah Pendaftar PPDB', 'Total calon siswa yang mendaftar PPDB', 'count(pendaftar)', 'count', 'admission', 'Panitia PPDB', 'ppdb_applicants', ARRAY['wave', 'wave_day'], true, 120, NULL, 1, 'active'),
    ('attendance_rate', 'Tingkat Kehadiran Siswa', 'Persentase kehadiran siswa terhadap hari efektif pembelajaran', '(hadir + terlambat) / hari_efektif * 10000', 'percent_bp', 'attendance', 'Kesiswaan / Wali Kelas', 'attendance_rate', ARRAY['month', 'grade', 'class'], false, 60, 'Sakit dan izin yang disetujui tidak dihitung sebagai pelanggaran alpa', 1, 'active'),
    ('active_students', 'Jumlah Siswa Aktif', 'Total siswa dengan status terdaftar aktif di sekolah', 'count(siswa_aktif)', 'count', 'identity', 'Tata Usaha', 'active_students', '{}', true, 360, NULL, 1, 'active'),
    ('active_teachers', 'Jumlah Guru Aktif', 'Total guru yang aktif bertugas mengajar', 'count(guru_aktif)', 'count', 'identity', 'Kurikulum / SDM', 'active_teachers', '{}', true, 360, NULL, 1, 'active'),
    ('active_staff', 'Jumlah Tenaga Kependidikan', 'Total pegawai non-guru yang aktif bertugas', 'count(staf_aktif)', 'count', 'hr', 'SDM / HR', 'active_staff', '{}', true, 360, NULL, 1, 'active'),
    ('verified_achievements', 'Prestasi Siswa Terverifikasi', 'Jumlah pencapaian dan prestasi lomba siswa yang terverifikasi', 'count(prestasi_terverifikasi)', 'count', 'studentlife', 'Kesiswaan', 'verified_achievements', ARRAY['level'], true, 360, 'Hanya prestasi berstatus verified', 1, 'active')
ON CONFLICT (key) DO NOTHING;
