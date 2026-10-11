-- Local seed for the admission database (Teratai fork): one OPEN period with a
-- registration wave, a track with quota, required documents and a fee scheme so
-- the public PPDB flow and admin queue have real data. Deterministic ids.
-- Local only.

BEGIN;

INSERT INTO admission_periods (id, school_id, name, academic_year, status, timezone, registration_open_at, registration_close_at)
VALUES ('aaaaaaaa-0000-0000-0000-000000000001', '11111111-1111-1111-1111-111111111111',
        'PPDB 2026/2027', '2026/2027', 'open', 'Asia/Jakarta', now() - interval '1 day', now() + interval '30 days')
ON CONFLICT DO NOTHING;

INSERT INTO admission_waves (id, school_id, period_id, name, registration_start, registration_end, payment_end, test_start, test_end, announcement_at, reregistration_end)
VALUES ('aaaaaaaa-0000-0000-0000-000000000002', '11111111-1111-1111-1111-111111111111',
        'aaaaaaaa-0000-0000-0000-000000000001', 'Gelombang 1',
        now() - interval '1 day', now() + interval '20 days', now() + interval '25 days',
        now() + interval '21 days', now() + interval '24 days', now() + interval '26 days', now() + interval '30 days')
ON CONFLICT DO NOTHING;

INSERT INTO admission_tracks (id, school_id, period_id, code, name, quota, description, is_active)
VALUES ('aaaaaaaa-0000-0000-0000-000000000003', '11111111-1111-1111-1111-111111111111',
        'aaaaaaaa-0000-0000-0000-000000000001', 'REG', 'Jalur Reguler', 60, 'Jalur pendaftaran reguler', true)
ON CONFLICT DO NOTHING;

INSERT INTO document_requirements (id, school_id, period_id, track_id, code, name, required, allowed_mimes, max_bytes, description)
VALUES ('aaaaaaaa-0000-0000-0000-000000000004', '11111111-1111-1111-1111-111111111111',
        'aaaaaaaa-0000-0000-0000-000000000001', NULL, 'AKTA', 'Akta Kelahiran', true,
        ARRAY['application/pdf','image/jpeg','image/png'], 5242880, 'Unggah akta kelahiran (PDF/JPG/PNG, maks 5MB)')
ON CONFLICT DO NOTHING;

INSERT INTO fee_schemes (id, school_id, period_id, track_id, name, revision, status, registration_fee_idr, reregistration_fee_idr, installment_count)
VALUES ('aaaaaaaa-0000-0000-0000-000000000005', '11111111-1111-1111-1111-111111111111',
        'aaaaaaaa-0000-0000-0000-000000000001', NULL, 'Biaya PPDB Reguler', 1, 'published', 150000, 2500000, 2)
ON CONFLICT DO NOTHING;

INSERT INTO fee_items (school_id, fee_scheme_id, code, name, amount_idr, discountable)
VALUES ('11111111-1111-1111-1111-111111111111', 'aaaaaaaa-0000-0000-0000-000000000005', 'FORM', 'Formulir Pendaftaran', 150000, false),
       ('11111111-1111-1111-1111-111111111111', 'aaaaaaaa-0000-0000-0000-000000000005', 'DAFTARULANG', 'Daftar Ulang', 2500000, true)
ON CONFLICT DO NOTHING;

INSERT INTO installment_plans (school_id, fee_scheme_id, sequence, description, amount_idr, due_at)
VALUES ('11111111-1111-1111-1111-111111111111', 'aaaaaaaa-0000-0000-0000-000000000005', 1, 'Termin 1', 1325000, now() + interval '14 days'),
       ('11111111-1111-1111-1111-111111111111', 'aaaaaaaa-0000-0000-0000-000000000005', 2, 'Termin 2', 1325000, now() + interval '28 days')
ON CONFLICT DO NOTHING;

COMMIT;
