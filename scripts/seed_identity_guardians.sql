-- ============================================================================
-- SYNC GUARDIANS INTO IDENTITY SERVICE & LINK TO ADMISSION
-- Sets up official accounts for parents (role: orang_tua) per PRD FR-16-006.
-- ============================================================================

\c teratai_identity;

DO $$
DECLARE
    v_school_id uuid := '11111111-1111-1111-1111-111111111111';
    v_hash text := '$2b$12$ufRxaUqTFcih90qxfg6G3eqgsuKUXInqgFIZc20VjhWsLXuTrTZAK'; -- Teratai123!
BEGIN
    -- 1. Nurul Akbar
    INSERT INTO users (id, name, email, phone, password_hash, password_changed, is_superadmin, created_at, updated_at)
    VALUES ('88880000-0000-0000-0000-000000000001', 'Nurul Akbar Tanjung', 'nurulakbartanjung@gmail.com', '081234567891', v_hash, false, false, now(), now())
    ON CONFLICT (email) DO UPDATE SET name = EXCLUDED.name, updated_at = now();

    INSERT INTO user_school_roles (user_id, school_id, role, status)
    SELECT id, v_school_id, 'orang_tua', 'active' FROM users WHERE email = 'nurulakbartanjung@gmail.com'
    ON CONFLICT (user_id, school_id, role) DO UPDATE SET status = 'active';

    -- 2. David Pambudi
    INSERT INTO users (id, name, email, phone, password_hash, password_changed, is_superadmin, created_at, updated_at)
    VALUES ('88880000-0000-0000-0000-000000000002', 'David Pambudi', 'pambudidavid03@gmail.com', '081234567892', v_hash, false, false, now(), now())
    ON CONFLICT (email) DO UPDATE SET name = EXCLUDED.name, updated_at = now();

    INSERT INTO user_school_roles (user_id, school_id, role, status)
    SELECT id, v_school_id, 'orang_tua', 'active' FROM users WHERE email = 'pambudidavid03@gmail.com'
    ON CONFLICT (user_id, school_id, role) DO UPDATE SET status = 'active';

    -- 3. Budi Santoso
    INSERT INTO users (id, name, email, phone, password_hash, password_changed, is_superadmin, created_at, updated_at)
    VALUES ('88880000-0000-0000-0000-000000000003', 'Budi Santoso', 'budi.wali@gmail.com', '081234567893', v_hash, false, false, now(), now())
    ON CONFLICT (email) DO UPDATE SET name = EXCLUDED.name, updated_at = now();

    INSERT INTO user_school_roles (user_id, school_id, role, status)
    SELECT id, v_school_id, 'orang_tua', 'active' FROM users WHERE email = 'budi.wali@gmail.com'
    ON CONFLICT (user_id, school_id, role) DO UPDATE SET status = 'active';

    -- 4. Ibu Ratna Sari
    INSERT INTO users (id, name, email, phone, password_hash, password_changed, is_superadmin, created_at, updated_at)
    VALUES ('88880000-0000-0000-0000-000000000004', 'Ibu Ratna Sari', 'ratna.sari@gmail.com', '081234567894', v_hash, false, false, now(), now())
    ON CONFLICT (email) DO UPDATE SET name = EXCLUDED.name, updated_at = now();

    INSERT INTO user_school_roles (user_id, school_id, role, status)
    SELECT id, v_school_id, 'orang_tua', 'active' FROM users WHERE email = 'ratna.sari@gmail.com'
    ON CONFLICT (user_id, school_id, role) DO UPDATE SET status = 'active';

    -- 5. Hendra Pratama
    INSERT INTO users (id, name, email, phone, password_hash, password_changed, is_superadmin, created_at, updated_at)
    VALUES ('88880000-0000-0000-0000-000000000005', 'Hendra Pratama', 'hendra.pratama@gmail.com', '081234567895', v_hash, false, false, now(), now())
    ON CONFLICT (email) DO UPDATE SET name = EXCLUDED.name, updated_at = now();

    INSERT INTO user_school_roles (user_id, school_id, role, status)
    SELECT id, v_school_id, 'orang_tua', 'active' FROM users WHERE email = 'hendra.pratama@gmail.com'
    ON CONFLICT (user_id, school_id, role) DO UPDATE SET status = 'active';

    -- 6. Siti Maryam
    INSERT INTO users (id, name, email, phone, password_hash, password_changed, is_superadmin, created_at, updated_at)
    VALUES ('88880000-0000-0000-0000-000000000006', 'Siti Maryam', 'siti.maryam@gmail.com', '081234567896', v_hash, false, false, now(), now())
    ON CONFLICT (email) DO UPDATE SET name = EXCLUDED.name, updated_at = now();

    INSERT INTO user_school_roles (user_id, school_id, role, status)
    SELECT id, v_school_id, 'orang_tua', 'active' FROM users WHERE email = 'siti.maryam@gmail.com'
    ON CONFLICT (user_id, school_id, role) DO UPDATE SET status = 'active';

    -- 7. Syamsul Bahri
    INSERT INTO users (id, name, email, phone, password_hash, password_changed, is_superadmin, created_at, updated_at)
    VALUES ('88880000-0000-0000-0000-000000000007', 'Syamsul Bahri', 'syamsul.bahri@gmail.com', '081234567897', v_hash, false, false, now(), now())
    ON CONFLICT (email) DO UPDATE SET name = EXCLUDED.name, updated_at = now();

    INSERT INTO user_school_roles (user_id, school_id, role, status)
    SELECT id, v_school_id, 'orang_tua', 'active' FROM users WHERE email = 'syamsul.bahri@gmail.com'
    ON CONFLICT (user_id, school_id, role) DO UPDATE SET status = 'active';

    -- 8. Dewi Rahmawati
    INSERT INTO users (id, name, email, phone, password_hash, password_changed, is_superadmin, created_at, updated_at)
    VALUES ('88880000-0000-0000-0000-000000000008', 'Dewi Rahmawati', 'dewi.rahmawati@gmail.com', '081234567898', v_hash, false, false, now(), now())
    ON CONFLICT (email) DO UPDATE SET name = EXCLUDED.name, updated_at = now();

    INSERT INTO user_school_roles (user_id, school_id, role, status)
    SELECT id, v_school_id, 'orang_tua', 'active' FROM users WHERE email = 'dewi.rahmawati@gmail.com'
    ON CONFLICT (user_id, school_id, role) DO UPDATE SET status = 'active';

END $$;

\c teratai_admission;

-- Update identity_user_id directly using deterministic IDs
UPDATE guardians SET identity_user_id = '88880000-0000-0000-0000-000000000001' WHERE id IN (SELECT guardian_id FROM guardian_contacts WHERE lower(normalized_value) = 'nurulakbartanjung@gmail.com');
UPDATE guardians SET identity_user_id = '88880000-0000-0000-0000-000000000002' WHERE id IN (SELECT guardian_id FROM guardian_contacts WHERE lower(normalized_value) = 'pambudidavid03@gmail.com');
UPDATE guardians SET identity_user_id = '88880000-0000-0000-0000-000000000003' WHERE id IN (SELECT guardian_id FROM guardian_contacts WHERE lower(normalized_value) = 'budi.wali@gmail.com');
UPDATE guardians SET identity_user_id = '88880000-0000-0000-0000-000000000004' WHERE id IN (SELECT guardian_id FROM guardian_contacts WHERE lower(normalized_value) = 'ratna.sari@gmail.com');
UPDATE guardians SET identity_user_id = '88880000-0000-0000-0000-000000000005' WHERE id IN (SELECT guardian_id FROM guardian_contacts WHERE lower(normalized_value) = 'hendra.pratama@gmail.com');
UPDATE guardians SET identity_user_id = '88880000-0000-0000-0000-000000000006' WHERE id IN (SELECT guardian_id FROM guardian_contacts WHERE lower(normalized_value) = 'siti.maryam@gmail.com');
UPDATE guardians SET identity_user_id = '88880000-0000-0000-0000-000000000007' WHERE id IN (SELECT guardian_id FROM guardian_contacts WHERE lower(normalized_value) = 'syamsul.bahri@gmail.com');
UPDATE guardians SET identity_user_id = '88880000-0000-0000-0000-000000000008' WHERE id IN (SELECT guardian_id FROM guardian_contacts WHERE lower(normalized_value) = 'dewi.rahmawati@gmail.com');
