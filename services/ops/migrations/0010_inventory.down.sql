-- 0010_inventory.down.sql
DROP TABLE IF EXISTS inventory_settings;
DROP TABLE IF EXISTS stock_count_lines;
DROP TABLE IF EXISTS stock_counts;
DROP TRIGGER IF EXISTS stock_movements_immutable_trg ON stock_movements;
DROP FUNCTION IF EXISTS trg_stock_movements_immutable();
DROP TABLE IF EXISTS stock_movements;
DROP TABLE IF EXISTS stock_balances;
DROP TABLE IF EXISTS warehouses;
DROP TABLE IF EXISTS items;
DROP TABLE IF EXISTS item_categories;
