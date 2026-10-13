-- 0050_transport.down.sql: Revert Transportasi (T19)

DROP TABLE IF EXISTS transport_charges CASCADE;
DROP TABLE IF EXISTS transport_absence_notices CASCADE;
DROP TABLE IF EXISTS transport_trip_events CASCADE;
DROP TABLE IF EXISTS transport_trips CASCADE;
DROP TABLE IF EXISTS transport_allocations CASCADE;
DROP TABLE IF EXISTS transport_fares CASCADE;
DROP TABLE IF EXISTS transport_route_assignments CASCADE;
DROP TABLE IF EXISTS transport_crew CASCADE;
DROP TABLE IF EXISTS transport_vehicle_documents CASCADE;
DROP TABLE IF EXISTS transport_vehicles CASCADE;
DROP TABLE IF EXISTS transport_stops CASCADE;
DROP TABLE IF EXISTS transport_routes CASCADE;
DROP TABLE IF EXISTS transport_zones CASCADE;
DROP TABLE IF EXISTS transport_settings CASCADE;
