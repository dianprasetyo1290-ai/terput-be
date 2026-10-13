package transport

import (
	"context"
	"database/sql"
	"os"
	"testing"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
)

func setupTransportTestDB(t *testing.T) (*sql.DB, func()) {
	t.Helper()
	dbURL := os.Getenv("TEST_DATABASE_URL")
	if dbURL == "" {
		t.Skip("skipping DB test: TEST_DATABASE_URL not set")
	}

	db, err := sql.Open("pgx", dbURL)
	if err != nil {
		t.Skip("skipping DB test: cannot open connection:", err)
	}
	if err := db.Ping(); err != nil {
		t.Skip("skipping DB test: cannot ping database:", err)
	}

	// Apply migrations
	for _, f := range []string{
		"../../migrations/0001_ops_base.up.sql",
		"../../migrations/0050_transport.up.sql",
	} {
		content, err := os.ReadFile(f)
		if err != nil {
			t.Fatalf("read migration %s: %v", f, err)
		}
		if _, err := db.Exec(string(content)); err != nil {
			t.Fatalf("exec migration %s: %v", f, err)
		}
	}

	cleanup := func() {
		_, _ = db.Exec(`
			DELETE FROM transport_charges;
			DELETE FROM transport_absence_notices;
			DELETE FROM transport_trip_events;
			DELETE FROM transport_trips;
			DELETE FROM transport_allocations;
			DELETE FROM transport_fares;
			DELETE FROM transport_route_assignments;
			DELETE FROM transport_crew;
			DELETE FROM transport_vehicle_documents;
			DELETE FROM transport_vehicles;
			DELETE FROM transport_stops;
			DELETE FROM transport_routes;
			DELETE FROM transport_zones;
			DELETE FROM transport_settings;
		`)
	}
	return db, cleanup
}

func TestTransportE2E(t *testing.T) {
	db, cleanup := setupTransportTestDB(t)
	defer cleanup()
	defer db.Close()

	ctx := context.Background()
	loc := time.UTC
	store := NewStore(db, loc)
	schoolID := "11111111-1111-1111-1111-111111111111"

	// 1. Setup zone, route & stops
	zone, err := store.CreateZone(ctx, schoolID, "Zona 1 - Bekasi")
	if err != nil {
		t.Fatalf("create zone: %v", err)
	}

	route, err := store.CreateRoute(ctx, schoolID, "RT-01", "Rute Antar-Jemput Timur")
	if err != nil {
		t.Fatalf("create route: %v", err)
	}

	stops := []Stop{
		{Seq: 1, Name: "Halte Perumahan A", ZoneID: &zone.ID},
		{Seq: 2, Name: "Halte Pasar Kranji", ZoneID: &zone.ID},
	}
	if err := store.SetStops(ctx, schoolID, route.ID, stops); err != nil {
		t.Fatalf("set stops: %v", err)
	}
	dbStops, err := store.ListStops(ctx, schoolID, route.ID)
	if err != nil || len(dbStops) != 2 {
		t.Fatalf("expected 2 stops, got %d (err: %v)", len(dbStops), err)
	}

	// 2. Setup vehicle (capacity 2 for testing capacity check) & crew
	vehicle, err := store.CreateVehicle(ctx, Vehicle{
		SchoolID: schoolID,
		PlateNo:  "B 1234 SKC",
		Name:     "Toyota HiAce 01",
		Capacity: 2,
		Status:   "active",
	})
	if err != nil {
		t.Fatalf("create vehicle: %v", err)
	}

	extName := "Pak Joko"
	driver, err := store.CreateCrew(ctx, Crew{
		SchoolID:     schoolID,
		Kind:         "driver",
		ExternalName: &extName,
	})
	if err != nil {
		t.Fatalf("create crew: %v", err)
	}

	today := time.Now().In(loc)
	_, err = store.AssignRoute(ctx, RouteAssignment{
		SchoolID:     schoolID,
		RouteID:      route.ID,
		VehicleID:    vehicle.ID,
		DriverCrewID: driver.ID,
		ValidFrom:    today.AddDate(0, -1, 0),
	})
	if err != nil {
		t.Fatalf("assign route: %v", err)
	}

	// 3. Test AC-121-01: Allocate 2 students (capacity 2) -> ok
	s1 := "student-001"
	s2 := "student-002"
	s3 := "student-003"
	err = store.CreateAllocations(ctx, schoolID, []string{s1, s2}, route.ID, dbStops[0].ID, today, "admin-1")
	if err != nil {
		t.Fatalf("allocate 2 students: %v", err)
	}

	// Allocate 3rd student -> must fail with ErrCapacityFull
	err = store.CreateAllocations(ctx, schoolID, []string{s3}, route.ID, dbStops[0].ID, today, "admin-1")
	if err == nil {
		t.Fatalf("AC-121-01: expected 3rd student allocation to fail due to capacity, but succeeded")
	}

	// 4. Test Trips & Manifest (AC-121-02)
	trips, err := store.GetOrCreateDailyTrips(ctx, schoolID, today)
	if err != nil || len(trips) < 2 {
		t.Fatalf("get or create daily trips: %v (len %d)", err, len(trips))
	}
	tripJemput := trips[0]

	manifest, err := store.GetTripManifest(ctx, schoolID, tripJemput.ID)
	if err != nil {
		t.Fatalf("get manifest: %v", err)
	}
	if len(manifest) != 2 {
		t.Fatalf("expected 2 students in manifest, got %d", len(manifest))
	}

	// Record board event
	ev, err := store.RecordTripEvent(ctx, TripEvent{
		SchoolID:      schoolID,
		TripID:        tripJemput.ID,
		StudentUserID: s1,
		StopID:        &dbStops[0].ID,
		Event:         "board",
		Source:        "scan",
		RecordedBy:    "driver-user",
		RecordedAt:    today,
		ClientEventID: "client-ev-1",
	})
	if err != nil {
		t.Fatalf("record board event: %v", err)
	}
	if ev.Event != "board" {
		t.Errorf("expected board event, got %s", ev.Event)
	}

	// Check updated manifest
	manifestUpdated, err := store.GetTripManifest(ctx, schoolID, tripJemput.ID)
	if err != nil {
		t.Fatalf("get manifest updated: %v", err)
	}
	if manifestUpdated[0].Status != "naik" {
		t.Errorf("expected status 'naik' for student 1, got '%s'", manifestUpdated[0].Status)
	}
}
