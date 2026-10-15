-- 04_ops.sql
-- Seed facilities, inventory, fixed assets, library, transport for Teratai Putih Global School

DO $$
DECLARE
  v_school_id UUID := '11111111-1111-1111-1111-111111111111';
  v_admin_id  UUID := '22222222-2222-2222-2222-222222222222';
  v_siswa_id  UUID := '44444444-4444-4444-4444-444444444444';
  v_bldg_a    UUID;
  v_bldg_b    UUID;
  v_floor_1   UUID;
  v_floor_2   UUID;
  v_room_lab  UUID;
  v_room_perpus UUID;
  v_room_aula UUID;
  v_room_gudang UUID;
  v_wh_id     UUID;
  v_cat_atk   UUID;
  v_cat_elek  UUID;
  v_item_kertas UUID;
  v_item_spidol UUID;
  v_vendor_id UUID;
  v_asset_cat UUID;
  v_lib_cat   UUID;
  v_lib_title UUID;
  v_lib_mcat  UUID;
  v_route_id  UUID;
  v_veh_id    UUID;
  v_crew_id   UUID;
  v_model_id  UUID;
BEGIN
  -- 1. Buildings & Floors
  INSERT INTO buildings (school_id, code, name, address, sort)
  VALUES (v_school_id, 'BLDG-A', 'Gedung Utama (A)', 'Jl. Teratai Putih No. 1, Bekasi', 1)
  ON CONFLICT (school_id, code) WHERE deleted_at IS NULL DO NOTHING;
  SELECT id INTO v_bldg_a FROM buildings WHERE school_id = v_school_id AND code = 'BLDG-A' LIMIT 1;

  INSERT INTO buildings (school_id, code, name, address, sort)
  VALUES (v_school_id, 'BLDG-B', 'Gedung Laboratorium (B)', 'Jl. Teratai Putih No. 1, Bekasi', 2)
  ON CONFLICT (school_id, code) WHERE deleted_at IS NULL DO NOTHING;
  SELECT id INTO v_bldg_b FROM buildings WHERE school_id = v_school_id AND code = 'BLDG-B' LIMIT 1;

  IF v_bldg_a IS NOT NULL THEN
    INSERT INTO floors (school_id, building_id, level, name)
    VALUES (v_school_id, v_bldg_a, 1, 'Lantai 1')
    ON CONFLICT (building_id, level) WHERE deleted_at IS NULL DO NOTHING;
    SELECT id INTO v_floor_1 FROM floors WHERE building_id = v_bldg_a AND level = 1 LIMIT 1;

    INSERT INTO floors (school_id, building_id, level, name)
    VALUES (v_school_id, v_bldg_a, 2, 'Lantai 2')
    ON CONFLICT (building_id, level) WHERE deleted_at IS NULL DO NOTHING;
    SELECT id INTO v_floor_2 FROM floors WHERE building_id = v_bldg_a AND level = 2 LIMIT 1;
  END IF;

  -- 2. Rooms
  IF v_floor_1 IS NOT NULL THEN
    INSERT INTO rooms (school_id, floor_id, code, name, kind, usage, capacity, status)
    VALUES (v_school_id, v_floor_1, 'R-AULA', 'Aula Graha Teratai', 'aula', 'bookable', 300, 'active')
    ON CONFLICT DO NOTHING;
    SELECT id INTO v_room_aula FROM rooms WHERE school_id = v_school_id AND code = 'R-AULA' LIMIT 1;

    INSERT INTO rooms (school_id, floor_id, code, name, kind, usage, capacity, status)
    VALUES (v_school_id, v_floor_1, 'R-PERPUS', 'Perpustakaan Pusat TPGS', 'perpustakaan', 'fixed', 60, 'active')
    ON CONFLICT DO NOTHING;
    SELECT id INTO v_room_perpus FROM rooms WHERE school_id = v_school_id AND code = 'R-PERPUS' LIMIT 1;

    INSERT INTO rooms (school_id, floor_id, code, name, kind, usage, capacity, status)
    VALUES (v_school_id, v_floor_1, 'R-GUDANG', 'Gudang Logistik Pusat', 'gudang', 'fixed', 10, 'active')
    ON CONFLICT DO NOTHING;
    SELECT id INTO v_room_gudang FROM rooms WHERE school_id = v_school_id AND code = 'R-GUDANG' LIMIT 1;
  END IF;

  IF v_floor_2 IS NOT NULL THEN
    INSERT INTO rooms (school_id, floor_id, code, name, kind, usage, capacity, status)
    VALUES (v_school_id, v_floor_2, 'R-LABKOM-1', 'Lab Komputer 1 (Multimedia)', 'lab', 'bookable', 36, 'active')
    ON CONFLICT DO NOTHING;
    SELECT id INTO v_room_lab FROM rooms WHERE school_id = v_school_id AND code = 'R-LABKOM-1' LIMIT 1;
  END IF;

  -- 3. Warehouse
  INSERT INTO warehouses (school_id, code, name, room_id, status)
  VALUES (v_school_id, 'WH-MAIN', 'Gudang Utama Teratai', v_room_gudang, 'active')
  ON CONFLICT (school_id, code) WHERE deleted_at IS NULL DO NOTHING;
  SELECT id INTO v_wh_id FROM warehouses WHERE school_id = v_school_id AND code = 'WH-MAIN' LIMIT 1;

  -- 4. Inventory Categories & Items
  INSERT INTO item_categories (school_id, code, name, tracking)
  VALUES (v_school_id, 'CAT-ATK', 'Alat Tulis Kantor & Kertas', 'stock')
  ON CONFLICT (school_id, code) WHERE deleted_at IS NULL DO NOTHING;
  SELECT id INTO v_cat_atk FROM item_categories WHERE school_id = v_school_id AND code = 'CAT-ATK' LIMIT 1;

  IF v_cat_atk IS NOT NULL THEN
    INSERT INTO items (school_id, code, name, category_id, unit, min_stock, status)
    VALUES (v_school_id, 'ITM-HVS-A4', 'Kertas HVS A4 80gr Sinar Dunia', v_cat_atk, 'rim', 10, 'active')
    ON CONFLICT (school_id, code) WHERE deleted_at IS NULL DO NOTHING;
    SELECT id INTO v_item_kertas FROM items WHERE school_id = v_school_id AND code = 'ITM-HVS-A4' LIMIT 1;

    INSERT INTO items (school_id, code, name, category_id, unit, min_stock, status)
    VALUES (v_school_id, 'ITM-SPIDOL-BLK', 'Spidol Whiteboard Snowman Hitam', v_cat_atk, 'pcs', 24, 'active')
    ON CONFLICT (school_id, code) WHERE deleted_at IS NULL DO NOTHING;
    SELECT id INTO v_item_spidol FROM items WHERE school_id = v_school_id AND code = 'ITM-SPIDOL-BLK' LIMIT 1;

    IF v_wh_id IS NOT NULL THEN
      IF v_item_kertas IS NOT NULL THEN
        INSERT INTO stock_balances (school_id, warehouse_id, item_id, qty)
        VALUES (v_school_id, v_wh_id, v_item_kertas, 85)
        ON CONFLICT (warehouse_id, item_id) DO UPDATE SET qty = EXCLUDED.qty;
      END IF;
      IF v_item_spidol IS NOT NULL THEN
        INSERT INTO stock_balances (school_id, warehouse_id, item_id, qty)
        VALUES (v_school_id, v_wh_id, v_item_spidol, 150)
        ON CONFLICT (warehouse_id, item_id) DO UPDATE SET qty = EXCLUDED.qty;
      END IF;
    END IF;
  END IF;

  -- 5. Vendors
  INSERT INTO vendors (school_id, code, name, address, categories, payment_terms_days, status)
  VALUES (
    v_school_id, 'VND-001', 'CV Sarana Edukasi Nusantara', 'Jl. Ahmad Yani No. 45, Bekasi',
    ARRAY['ATK', 'Buku', 'Elektronik'], 30, 'active'
  )
  ON CONFLICT (school_id, code) WHERE deleted_at IS NULL DO NOTHING;
  SELECT id INTO v_vendor_id FROM vendors WHERE school_id = v_school_id AND code = 'VND-001' LIMIT 1;

  -- 6. Asset Categories & Fixed Assets
  INSERT INTO asset_categories (school_id, code, name, useful_life_months, dep_start)
  VALUES (v_school_id, 'AST-ELEK', 'Peralatan Elektronik & IT', 48, 'acquisition_month')
  ON CONFLICT (school_id, code) WHERE deleted_at IS NULL DO NOTHING;
  SELECT id INTO v_asset_cat FROM asset_categories WHERE school_id = v_school_id AND code = 'AST-ELEK' LIMIT 1;

  IF v_asset_cat IS NOT NULL THEN
    INSERT INTO assets (
      school_id, code, name, category_id, status, condition, acquisition_cost_idr,
      residual_idr, acquired_on, room_id, qr_token, source
    ) VALUES (
      v_school_id, 'AST-LTP-001', 'Laptop Lenovo ThinkPad Lab Multimedia',
      v_asset_cat, 'active', 'baik', 12500000, 1000000, '2026-01-15',
      v_room_lab, 'QR-TPGS-AST-001', 'manual'
    )
    ON CONFLICT (qr_token) DO NOTHING;

    INSERT INTO assets (
      school_id, code, name, category_id, status, condition, acquisition_cost_idr,
      residual_idr, acquired_on, room_id, qr_token, source
    ) VALUES (
      v_school_id, 'AST-PRJ-001', 'Proyektor Epson EB-X500 Aula',
      v_asset_cat, 'active', 'baik', 8500000, 500000, '2026-02-10',
      v_room_aula, 'QR-TPGS-AST-002', 'manual'
    )
    ON CONFLICT (qr_token) DO NOTHING;
  END IF;

  -- 7. Library
  INSERT INTO lib_collection_categories (school_id, code, name)
  VALUES (v_school_id, 'CAT-PELAJARAN', 'Buku Teks Pelajaran')
  ON CONFLICT (school_id, code) WHERE deleted_at IS NULL DO NOTHING;
  SELECT id INTO v_lib_cat FROM lib_collection_categories WHERE school_id = v_school_id AND code = 'CAT-PELAJARAN' LIMIT 1;

  IF v_lib_cat IS NOT NULL THEN
    INSERT INTO lib_titles (school_id, title, author, publisher, isbn, publish_year, category_id, replacement_price_idr)
    VALUES (
      v_school_id, 'Matematika SMP Kelas VII Kurikulum Merdeka',
      'Kemendikbudristek', 'Pusat Kurikulum dan Perbukuan', '978-602-244-883-9', 2026,
      v_lib_cat, 85000
    )
    ON CONFLICT DO NOTHING;
    SELECT id INTO v_lib_title FROM lib_titles WHERE school_id = v_school_id AND title = 'Matematika SMP Kelas VII Kurikulum Merdeka' LIMIT 1;

    IF v_lib_title IS NOT NULL THEN
      INSERT INTO lib_copies (school_id, title_id, barcode, shelf_location, status)
      VALUES (v_school_id, v_lib_title, 'BC-MTK-7-001', 'RAK-A-01', 'available')
      ON CONFLICT (school_id, barcode) WHERE deleted_at IS NULL DO NOTHING;
      INSERT INTO lib_copies (school_id, title_id, barcode, shelf_location, status)
      VALUES (v_school_id, v_lib_title, 'BC-MTK-7-002', 'RAK-A-01', 'available')
      ON CONFLICT (school_id, barcode) WHERE deleted_at IS NULL DO NOTHING;
    END IF;
  END IF;

  INSERT INTO lib_member_categories (school_id, name, applies_to, is_default, max_loans, loan_days)
  VALUES (v_school_id, 'Siswa Reguler', 'student', true, 3, 14)
  ON CONFLICT (school_id, applies_to) WHERE is_default AND deleted_at IS NULL DO NOTHING;
  SELECT id INTO v_lib_mcat FROM lib_member_categories WHERE school_id = v_school_id AND applies_to = 'student' LIMIT 1;

  IF v_lib_mcat IS NOT NULL THEN
    INSERT INTO lib_members (school_id, kind, user_id, category_id, member_code, status)
    VALUES (v_school_id, 'student', v_siswa_id, v_lib_mcat, 'LIB-SISWA-001', 'active')
    ON CONFLICT (school_id, user_id) DO NOTHING;
  END IF;

  -- 8. Transport
  INSERT INTO transport_routes (school_id, code, name, status)
  VALUES (v_school_id, 'RTE-01', 'Rute 01 - Summarecon / Bekasi Barat', 'active')
  ON CONFLICT (school_id, code) WHERE deleted_at IS NULL DO NOTHING;
  SELECT id INTO v_route_id FROM transport_routes WHERE school_id = v_school_id AND code = 'RTE-01' LIMIT 1;

  INSERT INTO transport_vehicles (school_id, plate_no, name, capacity, status)
  VALUES (v_school_id, 'B 7123 KAA', 'Bus Sekolah Isuzu Elf 01', 19, 'active')
  ON CONFLICT (school_id, plate_no) WHERE deleted_at IS NULL DO NOTHING;
  SELECT id INTO v_veh_id FROM transport_vehicles WHERE school_id = v_school_id AND plate_no = 'B 7123 KAA' LIMIT 1;

  INSERT INTO transport_crew (school_id, kind, external_name, phone, license_no)
  VALUES (v_school_id, 'driver', 'Pak Bambang Supir', '081234567890', 'SIM-B1-987654')
  ON CONFLICT DO NOTHING;
  SELECT id INTO v_crew_id FROM transport_crew WHERE school_id = v_school_id AND external_name = 'Pak Bambang Supir' LIMIT 1;

  -- 9. 3D Campus Models & Room Nodes
  IF v_bldg_a IS NOT NULL THEN
    INSERT INTO campus3d_models (
      school_id, building_id, version, object_key, mime, size_bytes, sha256,
      node_names, status, uploaded_by
    ) VALUES (
      v_school_id, v_bldg_a, 1, 'models/bldg_a_v1.glb', 'model/gltf-binary', 1048576, 'dummy-sha256-hash',
      ARRAY['Node_AulaUtama', 'Node_LabKomputer1'], 'draft', v_admin_id
    )
    ON CONFLICT (building_id, version) DO NOTHING
    RETURNING id INTO v_model_id;

    IF v_model_id IS NULL THEN
      SELECT id INTO v_model_id FROM campus3d_models WHERE building_id = v_bldg_a AND version = 1;
    END IF;

    IF v_model_id IS NOT NULL AND v_room_aula IS NOT NULL THEN
      INSERT INTO campus3d_room_nodes (school_id, model_id, room_id, node_name)
      VALUES (v_school_id, v_model_id, v_room_aula, 'Node_AulaUtama')
      ON CONFLICT (model_id, room_id) DO NOTHING;
    END IF;
  END IF;

END $$;
