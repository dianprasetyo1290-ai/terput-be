package transport

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

type Store struct {
	db  *sql.DB
	loc *time.Location
}

func NewStore(db *sql.DB, loc *time.Location) *Store {
	if loc == nil {
		loc = time.UTC
	}
	return &Store{db: db, loc: loc}
}

// ---------------------------------------------------------------------
// Settings
// ---------------------------------------------------------------------

func (s *Store) GetSettings(ctx context.Context, schoolID string) (Settings, error) {
	var st Settings
	err := s.db.QueryRowContext(ctx, `
		SELECT school_id, prorata_policy, doc_reminder_days, notify_parent_on_trip,
		       component_code, bill_due_day, created_at, updated_at
		FROM transport_settings
		WHERE school_id = $1
	`, schoolID).Scan(
		&st.SchoolID, &st.ProrataPolicy, &st.DocReminderDays, &st.NotifyParentOnTrip,
		&st.ComponentCode, &st.BillDueDay, &st.CreatedAt, &st.UpdatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		// Initialize default settings
		now := time.Now().In(s.loc)
		st = Settings{
			SchoolID:           schoolID,
			ProrataPolicy:      "daily",
			DocReminderDays:    30,
			NotifyParentOnTrip: true,
			ComponentCode:      "TRANSPORT",
			CreatedAt:          now,
			UpdatedAt:          now,
		}
		_, insErr := s.db.ExecContext(ctx, `
			INSERT INTO transport_settings (school_id, prorata_policy, doc_reminder_days, notify_parent_on_trip, component_code, created_at, updated_at)
			VALUES ($1, $2, $3, $4, $5, $6, $7)
			ON CONFLICT (school_id) DO NOTHING
		`, st.SchoolID, st.ProrataPolicy, st.DocReminderDays, st.NotifyParentOnTrip, st.ComponentCode, now, now)
		if insErr != nil {
			return st, insErr
		}
		return st, nil
	}
	return st, err
}

func (s *Store) UpdateSettings(ctx context.Context, st Settings) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO transport_settings (school_id, prorata_policy, doc_reminder_days, notify_parent_on_trip, component_code, bill_due_day, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, now())
		ON CONFLICT (school_id) DO UPDATE SET
			prorata_policy = EXCLUDED.prorata_policy,
			doc_reminder_days = EXCLUDED.doc_reminder_days,
			notify_parent_on_trip = EXCLUDED.notify_parent_on_trip,
			component_code = EXCLUDED.component_code,
			bill_due_day = EXCLUDED.bill_due_day,
			updated_at = now()
	`, st.SchoolID, st.ProrataPolicy, st.DocReminderDays, st.NotifyParentOnTrip, st.ComponentCode, st.BillDueDay)
	return err
}

// ---------------------------------------------------------------------
// Zones
// ---------------------------------------------------------------------

func (s *Store) ListZones(ctx context.Context, schoolID string) ([]Zone, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, school_id, name, created_at, updated_at
		FROM transport_zones
		WHERE school_id = $1 AND deleted_at IS NULL
		ORDER BY name ASC
	`, schoolID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []Zone
	for rows.Next() {
		var z Zone
		if err := rows.Scan(&z.ID, &z.SchoolID, &z.Name, &z.CreatedAt, &z.UpdatedAt); err != nil {
			return nil, err
		}
		list = append(list, z)
	}
	return list, nil
}

func (s *Store) CreateZone(ctx context.Context, schoolID, name string) (Zone, error) {
	var z Zone
	err := s.db.QueryRowContext(ctx, `
		INSERT INTO transport_zones (school_id, name)
		VALUES ($1, $2)
		RETURNING id, school_id, name, created_at, updated_at
	`, schoolID, name).Scan(&z.ID, &z.SchoolID, &z.Name, &z.CreatedAt, &z.UpdatedAt)
	return z, err
}

func (s *Store) UpdateZone(ctx context.Context, schoolID, id, name string) (Zone, error) {
	var z Zone
	err := s.db.QueryRowContext(ctx, `
		UPDATE transport_zones
		SET name = $1, updated_at = now()
		WHERE school_id = $2 AND id = $3 AND deleted_at IS NULL
		RETURNING id, school_id, name, created_at, updated_at
	`, name, schoolID, id).Scan(&z.ID, &z.SchoolID, &z.Name, &z.CreatedAt, &z.UpdatedAt)
	return z, err
}

func (s *Store) DeleteZone(ctx context.Context, schoolID, id string) error {
	res, err := s.db.ExecContext(ctx, `
		UPDATE transport_zones
		SET deleted_at = now()
		WHERE school_id = $1 AND id = $2 AND deleted_at IS NULL
	`, schoolID, id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

// ---------------------------------------------------------------------
// Routes & Stops
// ---------------------------------------------------------------------

func (s *Store) ListRoutes(ctx context.Context, schoolID string) ([]Route, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, school_id, code, name, status, created_at, updated_at
		FROM transport_routes
		WHERE school_id = $1 AND deleted_at IS NULL
		ORDER BY code ASC
	`, schoolID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []Route
	for rows.Next() {
		var r Route
		if err := rows.Scan(&r.ID, &r.SchoolID, &r.Code, &r.Name, &r.Status, &r.CreatedAt, &r.UpdatedAt); err != nil {
			return nil, err
		}
		list = append(list, r)
	}
	return list, nil
}

func (s *Store) GetRoute(ctx context.Context, schoolID, id string) (Route, error) {
	var r Route
	err := s.db.QueryRowContext(ctx, `
		SELECT id, school_id, code, name, status, created_at, updated_at
		FROM transport_routes
		WHERE school_id = $1 AND id = $2 AND deleted_at IS NULL
	`, schoolID, id).Scan(&r.ID, &r.SchoolID, &r.Code, &r.Name, &r.Status, &r.CreatedAt, &r.UpdatedAt)
	if err != nil {
		return r, err
	}

	stops, err := s.ListStops(ctx, schoolID, id)
	if err != nil {
		return r, err
	}
	r.Stops = stops
	return r, nil
}

func (s *Store) CreateRoute(ctx context.Context, schoolID, code, name string) (Route, error) {
	var r Route
	err := s.db.QueryRowContext(ctx, `
		INSERT INTO transport_routes (school_id, code, name)
		VALUES ($1, $2, $3)
		RETURNING id, school_id, code, name, status, created_at, updated_at
	`, schoolID, code, name).Scan(&r.ID, &r.SchoolID, &r.Code, &r.Name, &r.Status, &r.CreatedAt, &r.UpdatedAt)
	return r, err
}

func (s *Store) UpdateRoute(ctx context.Context, schoolID, id, code, name, status string) (Route, error) {
	var r Route
	err := s.db.QueryRowContext(ctx, `
		UPDATE transport_routes
		SET code = $1, name = $2, status = $3, updated_at = now()
		WHERE school_id = $4 AND id = $5 AND deleted_at IS NULL
		RETURNING id, school_id, code, name, status, created_at, updated_at
	`, code, name, status, schoolID, id).Scan(&r.ID, &r.SchoolID, &r.Code, &r.Name, &r.Status, &r.CreatedAt, &r.UpdatedAt)
	return r, err
}

func (s *Store) DeleteRoute(ctx context.Context, schoolID, id string) error {
	res, err := s.db.ExecContext(ctx, `
		UPDATE transport_routes
		SET deleted_at = now()
		WHERE school_id = $1 AND id = $2 AND deleted_at IS NULL
	`, schoolID, id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func (s *Store) ListStops(ctx context.Context, schoolID, routeID string) ([]Stop, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT s.id, s.school_id, s.route_id, s.seq, s.name, s.address, s.zone_id,
		       z.name AS zone_name, s.pickup_time, s.dropoff_time, s.created_at, s.updated_at
		FROM transport_stops s
		LEFT JOIN transport_zones z ON z.id = s.zone_id
		WHERE s.school_id = $1 AND s.route_id = $2 AND s.deleted_at IS NULL
		ORDER BY s.seq ASC
	`, schoolID, routeID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var stops []Stop
	for rows.Next() {
		var st Stop
		if err := rows.Scan(&st.ID, &st.SchoolID, &st.RouteID, &st.Seq, &st.Name, &st.Address,
			&st.ZoneID, &st.ZoneName, &st.PickupTime, &st.DropoffTime, &st.CreatedAt, &st.UpdatedAt); err != nil {
			return nil, err
		}
		stops = append(stops, st)
	}
	return stops, nil
}

func (s *Store) SetStops(ctx context.Context, schoolID, routeID string, stops []Stop) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	// Soft delete existing stops
	if _, err := tx.ExecContext(ctx, `
		UPDATE transport_stops SET deleted_at = now()
		WHERE school_id = $1 AND route_id = $2 AND deleted_at IS NULL
	`, schoolID, routeID); err != nil {
		return err
	}

	for i, st := range stops {
		seq := i + 1
		if st.Seq > 0 {
			seq = st.Seq
		}
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO transport_stops (school_id, route_id, seq, name, address, zone_id, pickup_time, dropoff_time)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		`, schoolID, routeID, seq, st.Name, st.Address, st.ZoneID, st.PickupTime, st.DropoffTime); err != nil {
			return err
		}
	}

	return tx.Commit()
}

// ---------------------------------------------------------------------
// Vehicles & Documents
// ---------------------------------------------------------------------

func (s *Store) ListVehicles(ctx context.Context, schoolID string) ([]Vehicle, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, school_id, plate_no, name, capacity, status, created_at, updated_at
		FROM transport_vehicles
		WHERE school_id = $1 AND deleted_at IS NULL
		ORDER BY plate_no ASC
	`, schoolID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []Vehicle
	for rows.Next() {
		var v Vehicle
		if err := rows.Scan(&v.ID, &v.SchoolID, &v.PlateNo, &v.Name, &v.Capacity, &v.Status, &v.CreatedAt, &v.UpdatedAt); err != nil {
			return nil, err
		}
		list = append(list, v)
	}
	return list, nil
}

func (s *Store) CreateVehicle(ctx context.Context, v Vehicle) (Vehicle, error) {
	var res Vehicle
	err := s.db.QueryRowContext(ctx, `
		INSERT INTO transport_vehicles (school_id, plate_no, name, capacity, status)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING id, school_id, plate_no, name, capacity, status, created_at, updated_at
	`, v.SchoolID, v.PlateNo, v.Name, v.Capacity, v.Status).Scan(
		&res.ID, &res.SchoolID, &res.PlateNo, &res.Name, &res.Capacity, &res.Status, &res.CreatedAt, &res.UpdatedAt,
	)
	return res, err
}

func (s *Store) UpdateVehicle(ctx context.Context, v Vehicle) (Vehicle, error) {
	var res Vehicle
	err := s.db.QueryRowContext(ctx, `
		UPDATE transport_vehicles
		SET plate_no = $1, name = $2, capacity = $3, status = $4, updated_at = now()
		WHERE school_id = $5 AND id = $6 AND deleted_at IS NULL
		RETURNING id, school_id, plate_no, name, capacity, status, created_at, updated_at
	`, v.PlateNo, v.Name, v.Capacity, v.Status, v.SchoolID, v.ID).Scan(
		&res.ID, &res.SchoolID, &res.PlateNo, &res.Name, &res.Capacity, &res.Status, &res.CreatedAt, &res.UpdatedAt,
	)
	return res, err
}

func (s *Store) DeleteVehicle(ctx context.Context, schoolID, id string) error {
	res, err := s.db.ExecContext(ctx, `
		UPDATE transport_vehicles
		SET deleted_at = now()
		WHERE school_id = $1 AND id = $2 AND deleted_at IS NULL
	`, schoolID, id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func (s *Store) ListDocuments(ctx context.Context, schoolID, vehicleID string) ([]VehicleDocument, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, school_id, vehicle_id, kind, number, valid_until, object_key, created_at, updated_at
		FROM transport_vehicle_documents
		WHERE school_id = $1 AND vehicle_id = $2 AND deleted_at IS NULL
		ORDER BY valid_until ASC
	`, schoolID, vehicleID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []VehicleDocument
	for rows.Next() {
		var d VehicleDocument
		if err := rows.Scan(&d.ID, &d.SchoolID, &d.VehicleID, &d.Kind, &d.Number, &d.ValidUntil, &d.ObjectKey, &d.CreatedAt, &d.UpdatedAt); err != nil {
			return nil, err
		}
		list = append(list, d)
	}
	return list, nil
}

func (s *Store) AddDocument(ctx context.Context, d VehicleDocument) (VehicleDocument, error) {
	var res VehicleDocument
	err := s.db.QueryRowContext(ctx, `
		INSERT INTO transport_vehicle_documents (school_id, vehicle_id, kind, number, valid_until, object_key)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING id, school_id, vehicle_id, kind, number, valid_until, object_key, created_at, updated_at
	`, d.SchoolID, d.VehicleID, d.Kind, d.Number, d.ValidUntil, d.ObjectKey).Scan(
		&res.ID, &res.SchoolID, &res.VehicleID, &res.Kind, &res.Number, &res.ValidUntil, &res.ObjectKey, &res.CreatedAt, &res.UpdatedAt,
	)
	return res, err
}

func (s *Store) DeleteDocument(ctx context.Context, schoolID, id string) error {
	res, err := s.db.ExecContext(ctx, `
		UPDATE transport_vehicle_documents
		SET deleted_at = now()
		WHERE school_id = $1 AND id = $2 AND deleted_at IS NULL
	`, schoolID, id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

type ExpiringItem struct {
	Kind         string    `json:"kind"` // 'vehicle_doc' | 'crew_license'
	ID           string    `json:"id"`
	Title        string    `json:"title"`
	Number       string    `json:"number"`
	ValidUntil   time.Time `json:"valid_until"`
	DaysLeft     int       `json:"days_left"`
	VehiclePlate string    `json:"vehicle_plate,omitempty"`
	CrewName     string    `json:"crew_name,omitempty"`
}

func (s *Store) ListExpiring(ctx context.Context, schoolID string, withinDays int) ([]ExpiringItem, error) {
	today := time.Now().In(s.loc)
	var items []ExpiringItem

	// 1. Vehicle docs
	vRows, err := s.db.QueryContext(ctx, `
		SELECT d.id, d.kind, d.number, d.valid_until, v.plate_no
		FROM transport_vehicle_documents d
		JOIN transport_vehicles v ON v.id = d.vehicle_id
		WHERE d.school_id = $1 AND d.deleted_at IS NULL AND v.deleted_at IS NULL
	`, schoolID)
	if err == nil {
		defer vRows.Close()
		for vRows.Next() {
			var id, kind, number, plate string
			var validUntil time.Time
			if err := vRows.Scan(&id, &kind, &number, &validUntil, &plate); err == nil {
				due, daysLeft := DocReminderDue(validUntil, today, withinDays)
				if due {
					items = append(items, ExpiringItem{
						Kind:         "vehicle_doc",
						ID:           id,
						Title:        fmt.Sprintf("Dokumen %s (%s)", kind, plate),
						Number:       number,
						ValidUntil:   validUntil,
						DaysLeft:     daysLeft,
						VehiclePlate: plate,
					})
				}
			}
		}
	}

	// 2. Crew licenses
	cRows, err := s.db.QueryContext(ctx, `
		SELECT id, kind, license_no, license_valid_until, external_name
		FROM transport_crew
		WHERE school_id = $1 AND deleted_at IS NULL AND license_valid_until IS NOT NULL
	`, schoolID)
	if err == nil {
		defer cRows.Close()
		for cRows.Next() {
			var id, kind string
			var licNo, extName *string
			var validUntil *time.Time
			if err := cRows.Scan(&id, &kind, &licNo, &validUntil, &extName); err == nil && validUntil != nil {
				due, daysLeft := DocReminderDue(*validUntil, today, withinDays)
				if due {
					name := "Kru"
					if extName != nil {
						name = *extName
					}
					no := ""
					if licNo != nil {
						no = *licNo
					}
					items = append(items, ExpiringItem{
						Kind:       "crew_license",
						ID:         id,
						Title:      fmt.Sprintf("SIM %s (%s)", kind, name),
						Number:     no,
						ValidUntil: *validUntil,
						DaysLeft:   daysLeft,
						CrewName:   name,
					})
				}
			}
		}
	}

	return items, nil
}

// ---------------------------------------------------------------------
// Crew & Route Assignments
// ---------------------------------------------------------------------

func (s *Store) ListCrew(ctx context.Context, schoolID string) ([]Crew, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, school_id, kind, employee_id, user_id, external_name, phone,
		       license_no, license_valid_until, created_at, updated_at
		FROM transport_crew
		WHERE school_id = $1 AND deleted_at IS NULL
		ORDER BY kind, created_at DESC
	`, schoolID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []Crew
	for rows.Next() {
		var c Crew
		if err := rows.Scan(&c.ID, &c.SchoolID, &c.Kind, &c.EmployeeID, &c.UserID, &c.ExternalName,
			&c.Phone, &c.LicenseNo, &c.LicenseValidUntil, &c.CreatedAt, &c.UpdatedAt); err != nil {
			return nil, err
		}
		if c.ExternalName != nil {
			c.Name = *c.ExternalName
		}
		list = append(list, c)
	}
	return list, nil
}

func (s *Store) CreateCrew(ctx context.Context, c Crew) (Crew, error) {
	var res Crew
	err := s.db.QueryRowContext(ctx, `
		INSERT INTO transport_crew (school_id, kind, employee_id, user_id, external_name, phone, license_no, license_valid_until)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		RETURNING id, school_id, kind, employee_id, user_id, external_name, phone, license_no, license_valid_until, created_at, updated_at
	`, c.SchoolID, c.Kind, c.EmployeeID, c.UserID, c.ExternalName, c.Phone, c.LicenseNo, c.LicenseValidUntil).Scan(
		&res.ID, &res.SchoolID, &res.Kind, &res.EmployeeID, &res.UserID, &res.ExternalName,
		&res.Phone, &res.LicenseNo, &res.LicenseValidUntil, &res.CreatedAt, &res.UpdatedAt,
	)
	return res, err
}

func (s *Store) UpdateCrew(ctx context.Context, c Crew) (Crew, error) {
	var res Crew
	err := s.db.QueryRowContext(ctx, `
		UPDATE transport_crew
		SET kind = $1, employee_id = $2, user_id = $3, external_name = $4, phone = $5,
		    license_no = $6, license_valid_until = $7, updated_at = now()
		WHERE school_id = $8 AND id = $9 AND deleted_at IS NULL
		RETURNING id, school_id, kind, employee_id, user_id, external_name, phone, license_no, license_valid_until, created_at, updated_at
	`, c.Kind, c.EmployeeID, c.UserID, c.ExternalName, c.Phone, c.LicenseNo, c.LicenseValidUntil, c.SchoolID, c.ID).Scan(
		&res.ID, &res.SchoolID, &res.Kind, &res.EmployeeID, &res.UserID, &res.ExternalName,
		&res.Phone, &res.LicenseNo, &res.LicenseValidUntil, &res.CreatedAt, &res.UpdatedAt,
	)
	return res, err
}

func (s *Store) DeleteCrew(ctx context.Context, schoolID, id string) error {
	res, err := s.db.ExecContext(ctx, `
		UPDATE transport_crew
		SET deleted_at = now()
		WHERE school_id = $1 AND id = $2 AND deleted_at IS NULL
	`, schoolID, id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func (s *Store) AssignRoute(ctx context.Context, a RouteAssignment) (RouteAssignment, error) {
	var res RouteAssignment
	err := s.db.QueryRowContext(ctx, `
		INSERT INTO transport_route_assignments (school_id, route_id, vehicle_id, driver_crew_id, attendant_crew_id, valid_from, valid_to)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		RETURNING id, school_id, route_id, vehicle_id, driver_crew_id, attendant_crew_id, valid_from, valid_to, created_at, updated_at
	`, a.SchoolID, a.RouteID, a.VehicleID, a.DriverCrewID, a.AttendantCrewID, a.ValidFrom, a.ValidTo).Scan(
		&res.ID, &res.SchoolID, &res.RouteID, &res.VehicleID, &res.DriverCrewID, &res.AttendantCrewID,
		&res.ValidFrom, &res.ValidTo, &res.CreatedAt, &res.UpdatedAt,
	)
	return res, err
}

func (s *Store) GetActiveRouteAssignment(ctx context.Context, schoolID, routeID string, date time.Time) (*RouteAssignment, int, error) {
	var a RouteAssignment
	var capacity int
	err := s.db.QueryRowContext(ctx, `
		SELECT a.id, a.school_id, a.route_id, a.vehicle_id, a.driver_crew_id, a.attendant_crew_id,
		       a.valid_from, a.valid_to, a.created_at, a.updated_at, v.capacity
		FROM transport_route_assignments a
		JOIN transport_vehicles v ON v.id = a.vehicle_id
		WHERE a.school_id = $1 AND a.route_id = $2
		  AND a.valid_from <= $3 AND (a.valid_to IS NULL OR a.valid_to >= $3)
		ORDER BY a.valid_from DESC
		LIMIT 1
	`, schoolID, routeID, date).Scan(
		&a.ID, &a.SchoolID, &a.RouteID, &a.VehicleID, &a.DriverCrewID, &a.AttendantCrewID,
		&a.ValidFrom, &a.ValidTo, &a.CreatedAt, &a.UpdatedAt, &capacity,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, 0, nil
	}
	return &a, capacity, err
}

// ---------------------------------------------------------------------
// Fares
// ---------------------------------------------------------------------

func (s *Store) ListFares(ctx context.Context, schoolID string) ([]Fare, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, school_id, zone_id, route_id, monthly_amount_idr, effective_from, effective_to, created_at, updated_at
		FROM transport_fares
		WHERE school_id = $1 AND deleted_at IS NULL
		ORDER BY effective_from DESC
	`, schoolID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []Fare
	for rows.Next() {
		var f Fare
		if err := rows.Scan(&f.ID, &f.SchoolID, &f.ZoneID, &f.RouteID, &f.MonthlyAmountIDR,
			&f.EffectiveFrom, &f.EffectiveTo, &f.CreatedAt, &f.UpdatedAt); err != nil {
			return nil, err
		}
		list = append(list, f)
	}
	return list, nil
}

func (s *Store) CreateFare(ctx context.Context, f Fare) (Fare, error) {
	var res Fare
	err := s.db.QueryRowContext(ctx, `
		INSERT INTO transport_fares (school_id, zone_id, route_id, monthly_amount_idr, effective_from, effective_to)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING id, school_id, zone_id, route_id, monthly_amount_idr, effective_from, effective_to, created_at, updated_at
	`, f.SchoolID, f.ZoneID, f.RouteID, f.MonthlyAmountIDR, f.EffectiveFrom, f.EffectiveTo).Scan(
		&res.ID, &res.SchoolID, &res.ZoneID, &res.RouteID, &res.MonthlyAmountIDR,
		&res.EffectiveFrom, &res.EffectiveTo, &res.CreatedAt, &res.UpdatedAt,
	)
	return res, err
}

func (s *Store) DeleteFare(ctx context.Context, schoolID, id string) error {
	res, err := s.db.ExecContext(ctx, `
		UPDATE transport_fares
		SET deleted_at = now()
		WHERE school_id = $1 AND id = $2 AND deleted_at IS NULL
	`, schoolID, id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

// ---------------------------------------------------------------------
// Allocations (AC-121-01, BR-TRS-01, AC-119-01)
// ---------------------------------------------------------------------

func (s *Store) ListAllocations(ctx context.Context, schoolID, routeID, studentUserID string) ([]Allocation, error) {
	query := `
		SELECT a.id, a.school_id, a.student_user_id, a.route_id, r.code AS route_code, r.name AS route_name,
		       a.stop_id, st.name AS stop_name, a.start_date, a.end_date, a.end_reason, a.created_by,
		       a.created_at, a.updated_at
		FROM transport_allocations a
		JOIN transport_routes r ON r.id = a.route_id
		JOIN transport_stops st ON st.id = a.stop_id
		WHERE a.school_id = $1
	`
	var args []any
	args = append(args, schoolID)

	if routeID != "" {
		args = append(args, routeID)
		query += fmt.Sprintf(" AND a.route_id = $%d", len(args))
	}
	if studentUserID != "" {
		args = append(args, studentUserID)
		query += fmt.Sprintf(" AND a.student_user_id = $%d", len(args))
	}
	query += " ORDER BY a.start_date DESC"

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []Allocation
	for rows.Next() {
		var a Allocation
		if err := rows.Scan(&a.ID, &a.SchoolID, &a.StudentUserID, &a.RouteID, &a.RouteCode, &a.RouteName,
			&a.StopID, &a.StopName, &a.StartDate, &a.EndDate, &a.EndReason, &a.CreatedBy,
			&a.CreatedAt, &a.UpdatedAt); err != nil {
			return nil, err
		}
		list = append(list, a)
	}
	return list, nil
}

// CreateAllocations allocates a batch of students checking capacity (AC-121-01).
func (s *Store) CreateAllocations(ctx context.Context, schoolID string, studentUserIDs []string, routeID, stopID string, startDate time.Time, createdBy string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	// 1. Lock route row
	var routeExists bool
	if err := tx.QueryRowContext(ctx, `SELECT true FROM transport_routes WHERE school_id = $1 AND id = $2 AND deleted_at IS NULL FOR UPDATE`, schoolID, routeID).Scan(&routeExists); err != nil {
		return fmt.Errorf("rute tidak ditemukan: %w", err)
	}

	// 2. Check active vehicle assignment and its capacity
	var capacity int
	err = tx.QueryRowContext(ctx, `
		SELECT v.capacity
		FROM transport_route_assignments a
		JOIN transport_vehicles v ON v.id = a.vehicle_id
		WHERE a.school_id = $1 AND a.route_id = $2
		  AND a.valid_from <= $3 AND (a.valid_to IS NULL OR a.valid_to >= $3)
		ORDER BY a.valid_from DESC
		LIMIT 1
	`, schoolID, routeID, startDate).Scan(&capacity)
	if errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("belum ada penugasan kendaraan untuk rute ini pada tanggal %s", startDate.Format("2006-01-02"))
	} else if err != nil {
		return err
	}

	// 3. Count currently active allocations on this route
	var activeCount int
	if err := tx.QueryRowContext(ctx, `
		SELECT count(*)
		FROM transport_allocations
		WHERE school_id = $1 AND route_id = $2
		  AND start_date <= $3 AND (end_date IS NULL OR end_date >= $3)
	`, schoolID, routeID, startDate).Scan(&activeCount); err != nil {
		return err
	}

	// 4. Validate capacity (AC-121-01)
	if err := CanAllocate(capacity, activeCount, len(studentUserIDs)); err != nil {
		return err
	}

	// 5. Insert each student
	for _, sid := range studentUserIDs {
		// End any previous open allocation for this student
		// A failed statement aborts the whole transaction, so its error
		// must not be dropped (the next INSERT would only report 25P02).
		if _, err := tx.ExecContext(ctx, `
			UPDATE transport_allocations
			SET end_date = $1, end_reason = 'Ganti rute/alokasi baru', updated_at = now()
			WHERE school_id = $2 AND student_user_id = $3 AND end_date IS NULL
		`, startDate.AddDate(0, 0, -1), schoolID, sid); err != nil {
			return err
		}

		// Insert new allocation
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO transport_allocations (school_id, student_user_id, route_id, stop_id, start_date, created_by)
			VALUES ($1, $2, $3, $4, $5, $6)
		`, schoolID, sid, routeID, stopID, startDate, createdBy); err != nil {
			return err
		}
	}

	return tx.Commit()
}

func (s *Store) EndAllocation(ctx context.Context, schoolID, id string, endDate time.Time, reason string) error {
	res, err := s.db.ExecContext(ctx, `
		UPDATE transport_allocations
		SET end_date = $1, end_reason = $2, updated_at = now()
		WHERE school_id = $3 AND id = $4 AND (end_date IS NULL OR end_date > $1)
	`, endDate, reason, schoolID, id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func (s *Store) MoveAllocation(ctx context.Context, schoolID, id, newRouteID, newStopID string, effectiveDate time.Time, createdBy string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	var sid string
	var prevStartDate time.Time
	err = tx.QueryRowContext(ctx, `
		SELECT student_user_id, start_date
		FROM transport_allocations
		WHERE school_id = $1 AND id = $2 AND end_date IS NULL
		FOR UPDATE
	`, schoolID, id).Scan(&sid, &prevStartDate)
	if err != nil {
		return err
	}

	// End previous allocation
	endDate := effectiveDate.AddDate(0, 0, -1)
	if endDate.Before(prevStartDate) {
		endDate = prevStartDate
	}
	if _, err := tx.ExecContext(ctx, `
		UPDATE transport_allocations
		SET end_date = $1, end_reason = 'Pindah rute', updated_at = now()
		WHERE id = $2
	`, endDate, id); err != nil {
		return err
	}

	// Check new route capacity
	var capacity, activeCount int
	if err := tx.QueryRowContext(ctx, `
		SELECT v.capacity
		FROM transport_route_assignments a
		JOIN transport_vehicles v ON v.id = a.vehicle_id
		WHERE a.school_id = $1 AND a.route_id = $2
		  AND a.valid_from <= $3 AND (a.valid_to IS NULL OR a.valid_to >= $3)
		LIMIT 1
	`, schoolID, newRouteID, effectiveDate).Scan(&capacity); err != nil {
		return fmt.Errorf("kapasitas rute baru belum siap: %w", err)
	}

	if err := tx.QueryRowContext(ctx, `
		SELECT count(*)
		FROM transport_allocations
		WHERE school_id = $1 AND route_id = $2
		  AND start_date <= $3 AND (end_date IS NULL OR end_date >= $3)
	`, schoolID, newRouteID, effectiveDate).Scan(&activeCount); err != nil {
		return err
	}

	if err := CanAllocate(capacity, activeCount, 1); err != nil {
		return err
	}

	// Insert new allocation
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO transport_allocations (school_id, student_user_id, route_id, stop_id, start_date, created_by)
		VALUES ($1, $2, $3, $4, $5, $6)
	`, schoolID, sid, newRouteID, newStopID, effectiveDate, createdBy); err != nil {
		return err
	}

	return tx.Commit()
}

// ---------------------------------------------------------------------
// Trips & Attendance (AC-121-02, FR-121-002)
// ---------------------------------------------------------------------

func (s *Store) GetOrCreateDailyTrips(ctx context.Context, schoolID string, date time.Time) ([]Trip, error) {
	// 1. Query active assignments for this date
	rows, err := s.db.QueryContext(ctx, `
		SELECT a.route_id, a.vehicle_id, a.driver_crew_id, a.attendant_crew_id
		FROM transport_route_assignments a
		JOIN transport_routes r ON r.id = a.route_id
		WHERE a.school_id = $1 AND r.deleted_at IS NULL AND r.status = 'active'
		  AND a.valid_from <= $2 AND (a.valid_to IS NULL OR a.valid_to >= $2)
	`, schoolID, date)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	type assign struct {
		routeID   string
		vehicleID string
		driverID  string
		attID     *string
	}
	var assigns []assign
	for rows.Next() {
		var as assign
		if err := rows.Scan(&as.routeID, &as.vehicleID, &as.driverID, &as.attID); err == nil {
			assigns = append(assigns, as)
		}
	}
	rows.Close()

	// 2. Insert jemput and antar trips
	for _, as := range assigns {
		for _, dir := range []string{"jemput", "antar"} {
			_, _ = s.db.ExecContext(ctx, `
				INSERT INTO transport_trips (school_id, route_id, trip_date, direction, vehicle_id, driver_crew_id, attendant_crew_id)
				VALUES ($1, $2, $3, $4, $5, $6, $7)
				ON CONFLICT (route_id, trip_date, direction) DO NOTHING
			`, schoolID, as.routeID, date, dir, as.vehicleID, as.driverID, as.attID)
		}
	}

	return s.ListTrips(ctx, schoolID, date, "")
}

func (s *Store) ListTrips(ctx context.Context, schoolID string, date time.Time, routeID string) ([]Trip, error) {
	query := `
		SELECT t.id, t.school_id, t.route_id, r.name AS route_name, t.trip_date, t.direction,
		       t.vehicle_id, v.plate_no, t.driver_crew_id, COALESCE(cd.external_name, 'Driver') AS driver_name,
		       t.attendant_crew_id, ca.external_name AS attendant_name, t.status,
		       t.started_at, t.finished_at, t.created_at, t.updated_at
		FROM transport_trips t
		JOIN transport_routes r ON r.id = t.route_id
		JOIN transport_vehicles v ON v.id = t.vehicle_id
		LEFT JOIN transport_crew cd ON cd.id = t.driver_crew_id
		LEFT JOIN transport_crew ca ON ca.id = t.attendant_crew_id
		WHERE t.school_id = $1 AND t.trip_date = $2
	`
	var args []any
	args = append(args, schoolID, date)
	if routeID != "" {
		args = append(args, routeID)
		query += fmt.Sprintf(" AND t.route_id = $%d", len(args))
	}
	query += " ORDER BY r.code ASC, t.direction DESC"

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []Trip
	for rows.Next() {
		var tr Trip
		if err := rows.Scan(&tr.ID, &tr.SchoolID, &tr.RouteID, &tr.RouteName, &tr.TripDate, &tr.Direction,
			&tr.VehicleID, &tr.VehiclePlateNo, &tr.DriverCrewID, &tr.DriverName,
			&tr.AttendantCrewID, &tr.AttendantName, &tr.Status,
			&tr.StartedAt, &tr.FinishedAt, &tr.CreatedAt, &tr.UpdatedAt); err != nil {
			return nil, err
		}
		list = append(list, tr)
	}
	return list, nil
}

func (s *Store) GetTrip(ctx context.Context, schoolID, tripID string) (*Trip, error) {
	var tr Trip
	err := s.db.QueryRowContext(ctx, `
		SELECT t.id, t.school_id, t.route_id, r.name AS route_name, t.trip_date, t.direction,
		       t.vehicle_id, v.plate_no, t.driver_crew_id, COALESCE(cd.external_name, 'Driver') AS driver_name,
		       t.attendant_crew_id, ca.external_name AS attendant_name, t.status,
		       t.started_at, t.finished_at, t.created_at, t.updated_at
		FROM transport_trips t
		JOIN transport_routes r ON r.id = t.route_id
		JOIN transport_vehicles v ON v.id = t.vehicle_id
		LEFT JOIN transport_crew cd ON cd.id = t.driver_crew_id
		LEFT JOIN transport_crew ca ON ca.id = t.attendant_crew_id
		WHERE t.school_id = $1 AND t.id = $2
	`, schoolID, tripID).Scan(
		&tr.ID, &tr.SchoolID, &tr.RouteID, &tr.RouteName, &tr.TripDate, &tr.Direction,
		&tr.VehicleID, &tr.VehiclePlateNo, &tr.DriverCrewID, &tr.DriverName,
		&tr.AttendantCrewID, &tr.AttendantName, &tr.Status,
		&tr.StartedAt, &tr.FinishedAt, &tr.CreatedAt, &tr.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	return &tr, nil
}

func (s *Store) GetTripManifest(ctx context.Context, schoolID, tripID string) ([]ManifestEntry, error) {
	tr, err := s.GetTrip(ctx, schoolID, tripID)
	if err != nil {
		return nil, err
	}

	// 1. Get allocations active on trip date
	allocs, err := s.ListAllocations(ctx, schoolID, tr.RouteID, "")
	if err != nil {
		return nil, err
	}
	var activeAllocs []Allocation
	for _, a := range allocs {
		if !tr.TripDate.Before(a.StartDate) && (a.EndDate == nil || !tr.TripDate.After(*a.EndDate)) {
			activeAllocs = append(activeAllocs, a)
		}
	}

	// 2. Get trip events
	eRows, err := s.db.QueryContext(ctx, `
		SELECT id, school_id, trip_id, student_user_id, stop_id, event, source, recorded_by, recorded_at, client_event_id, created_at
		FROM transport_trip_events
		WHERE school_id = $1 AND trip_id = $2
	`, schoolID, tripID)
	if err != nil {
		return nil, err
	}
	defer eRows.Close()

	var events []TripEvent
	for eRows.Next() {
		var ev TripEvent
		if err := eRows.Scan(&ev.ID, &ev.SchoolID, &ev.TripID, &ev.StudentUserID, &ev.StopID,
			&ev.Event, &ev.Source, &ev.RecordedBy, &ev.RecordedAt, &ev.ClientEventID, &ev.CreatedAt); err == nil {
			events = append(events, ev)
		}
	}
	eRows.Close()

	// 3. Get absence notices for this date
	nRows, err := s.db.QueryContext(ctx, `
		SELECT id, school_id, student_user_id, notice_date, direction, reason, created_by, created_at, updated_at
		FROM transport_absence_notices
		WHERE school_id = $1 AND notice_date = $2 AND cancelled_at IS NULL
	`, schoolID, tr.TripDate)
	if err != nil {
		return nil, err
	}
	defer nRows.Close()

	var notices []AbsenceNotice
	for nRows.Next() {
		var an AbsenceNotice
		if err := nRows.Scan(&an.ID, &an.SchoolID, &an.StudentUserID, &an.NoticeDate, &an.Direction,
			&an.Reason, &an.CreatedBy, &an.CreatedAt, &an.UpdatedAt); err == nil {
			notices = append(notices, an)
		}
	}
	nRows.Close()

	return Manifest(activeAllocs, events, notices, tr.Direction), nil
}

func (s *Store) RecordTripEvent(ctx context.Context, ev TripEvent) (*TripEvent, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	// Query previous events for this trip & student
	rows, err := tx.QueryContext(ctx, `
		SELECT id, school_id, trip_id, student_user_id, stop_id, event, source, recorded_by, recorded_at, client_event_id, created_at
		FROM transport_trip_events
		WHERE school_id = $1 AND trip_id = $2 AND student_user_id = $3
		FOR UPDATE
	`, ev.SchoolID, ev.TripID, ev.StudentUserID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var prev []TripEvent
	for rows.Next() {
		var p TripEvent
		if err := rows.Scan(&p.ID, &p.SchoolID, &p.TripID, &p.StudentUserID, &p.StopID,
			&p.Event, &p.Source, &p.RecordedBy, &p.RecordedAt, &p.ClientEventID, &p.CreatedAt); err == nil {
			prev = append(prev, p)
		}
	}
	rows.Close()

	// Check idempotency & validation
	if err := ValidateTripEvent(prev, ev); err != nil {
		return nil, err
	}

	for _, p := range prev {
		if p.ClientEventID == ev.ClientEventID && p.Event == ev.Event {
			return &p, nil // idempotent return
		}
	}

	recordedAt := time.Now().In(s.loc)
	if !ev.RecordedAt.IsZero() {
		recordedAt = ev.RecordedAt
	}

	var res TripEvent
	err = tx.QueryRowContext(ctx, `
		INSERT INTO transport_trip_events (school_id, trip_id, student_user_id, stop_id, event, source, recorded_by, recorded_at, client_event_id)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		RETURNING id, school_id, trip_id, student_user_id, stop_id, event, source, recorded_by, recorded_at, client_event_id, created_at
	`, ev.SchoolID, ev.TripID, ev.StudentUserID, ev.StopID, ev.Event, ev.Source, ev.RecordedBy, recordedAt, ev.ClientEventID).Scan(
		&res.ID, &res.SchoolID, &res.TripID, &res.StudentUserID, &res.StopID,
		&res.Event, &res.Source, &res.RecordedBy, &res.RecordedAt, &res.ClientEventID, &res.CreatedAt,
	)
	if err != nil {
		return nil, err
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return &res, nil
}

func (s *Store) ChangeTripStatus(ctx context.Context, schoolID, tripID, status string) error {
	now := time.Now().In(s.loc)
	var q string
	switch status {
	case "running":
		q = `UPDATE transport_trips SET status = 'running', started_at = COALESCE(started_at, $1), updated_at = now() WHERE school_id = $2 AND id = $3`
	case "done":
		q = `UPDATE transport_trips SET status = 'done', finished_at = COALESCE(finished_at, $1), updated_at = now() WHERE school_id = $2 AND id = $3`
	default:
		q = `UPDATE transport_trips SET status = $1, updated_at = now() WHERE school_id = $2 AND id = $3`
		_, err := s.db.ExecContext(ctx, q, status, schoolID, tripID)
		return err
	}
	_, err := s.db.ExecContext(ctx, q, now, schoolID, tripID)
	return err
}

// ---------------------------------------------------------------------
// Absence Notices (FR-121-004)
// ---------------------------------------------------------------------

func (s *Store) CreateAbsenceNotice(ctx context.Context, an AbsenceNotice) (AbsenceNotice, error) {
	var res AbsenceNotice
	err := s.db.QueryRowContext(ctx, `
		INSERT INTO transport_absence_notices (school_id, student_user_id, notice_date, direction, reason, created_by)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING id, school_id, student_user_id, notice_date, direction, reason, created_by, created_at, updated_at
	`, an.SchoolID, an.StudentUserID, an.NoticeDate, an.Direction, an.Reason, an.CreatedBy).Scan(
		&res.ID, &res.SchoolID, &res.StudentUserID, &res.NoticeDate, &res.Direction,
		&res.Reason, &res.CreatedBy, &res.CreatedAt, &res.UpdatedAt,
	)
	return res, err
}

func (s *Store) DeleteAbsenceNotice(ctx context.Context, schoolID, id string) error {
	res, err := s.db.ExecContext(ctx, `
		UPDATE transport_absence_notices
		SET cancelled_at = now(), updated_at = now()
		WHERE school_id = $1 AND id = $2 AND cancelled_at IS NULL
	`, schoolID, id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func (s *Store) ListStudentAbsences(ctx context.Context, schoolID, studentUserID string) ([]AbsenceNotice, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, school_id, student_user_id, notice_date, direction, reason, created_by, created_at, updated_at
		FROM transport_absence_notices
		WHERE school_id = $1 AND student_user_id = $2 AND cancelled_at IS NULL
		ORDER BY notice_date DESC
	`, schoolID, studentUserID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []AbsenceNotice
	for rows.Next() {
		var an AbsenceNotice
		if err := rows.Scan(&an.ID, &an.SchoolID, &an.StudentUserID, &an.NoticeDate, &an.Direction,
			&an.Reason, &an.CreatedBy, &an.CreatedAt, &an.UpdatedAt); err != nil {
			return nil, err
		}
		list = append(list, an)
	}
	return list, nil
}

// ---------------------------------------------------------------------
// Billing & Charges (AC-122-01, FR-122-002/003)
// ---------------------------------------------------------------------

type StudentChargePreview struct {
	StudentUserID string       `json:"student_user_id"`
	StudentName   string       `json:"student_name,omitempty"`
	Items         []ChargeItem `json:"items"`
	TotalAmount   int64        `json:"total_amount_idr"`
	HasFareError  bool         `json:"has_fare_error"`
	ErrorMessage  string       `json:"error_message,omitempty"`
}

func (s *Store) PreviewBilling(ctx context.Context, schoolID string, year, month int) ([]StudentChargePreview, error) {
	st, err := s.GetSettings(ctx, schoolID)
	if err != nil {
		return nil, err
	}

	fares, err := s.ListFares(ctx, schoolID)
	if err != nil {
		return nil, err
	}

	// Query distinct students with active allocations overlapping with this month
	daysInMonth := DaysInMonth(year, month)
	firstOfMonth := time.Date(year, time.Month(month), 1, 0, 0, 0, 0, time.UTC)
	lastOfMonth := time.Date(year, time.Month(month), daysInMonth, 23, 59, 59, 0, time.UTC)

	rows, err := s.db.QueryContext(ctx, `
		SELECT a.student_user_id, a.id, a.route_id, r.name AS route_name, st.zone_id, a.start_date, a.end_date
		FROM transport_allocations a
		JOIN transport_routes r ON r.id = a.route_id
		JOIN transport_stops st ON st.id = a.stop_id
		WHERE a.school_id = $1
		  AND a.start_date <= $2 AND (a.end_date IS NULL OR a.end_date >= $3)
		ORDER BY a.student_user_id, a.start_date ASC
	`, schoolID, lastOfMonth, firstOfMonth)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	studentSegments := make(map[string][]AllocationSegment)
	for rows.Next() {
		var uid, allocID, routeID, routeName string
		var zoneID *string
		var startDate time.Time
		var endDate *time.Time
		if err := rows.Scan(&uid, &allocID, &routeID, &routeName, &zoneID, &startDate, &endDate); err == nil {
			studentSegments[uid] = append(studentSegments[uid], AllocationSegment{
				AllocationID: allocID,
				RouteID:      routeID,
				RouteName:    routeName,
				ZoneID:       zoneID,
				StartDate:    startDate,
				EndDate:      endDate,
			})
		}
	}
	rows.Close()

	var previews []StudentChargePreview
	for uid, segments := range studentSegments {
		items, total, calcErr := ComputeCharge(segments, fares, st.ProrataPolicy, year, month)
		p := StudentChargePreview{
			StudentUserID: uid,
			Items:         items,
			TotalAmount:   total,
		}
		if calcErr != nil {
			p.HasFareError = true
			p.ErrorMessage = calcErr.Error()
		}
		previews = append(previews, p)
	}

	return previews, nil
}

func (s *Store) SaveCharge(ctx context.Context, c Charge) (*Charge, error) {
	var res Charge
	err := s.db.QueryRowContext(ctx, `
		INSERT INTO transport_charges (school_id, student_user_id, period_year, period_month, items, amount_idr, version, bill_id, status, last_error)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
		ON CONFLICT (school_id, student_user_id, period_year, period_month) DO UPDATE SET
			items = EXCLUDED.items,
			amount_idr = EXCLUDED.amount_idr,
			version = transport_charges.version + 1,
			bill_id = EXCLUDED.bill_id,
			status = EXCLUDED.status,
			last_error = EXCLUDED.last_error,
			updated_at = now()
		RETURNING id, school_id, student_user_id, period_year, period_month, items, amount_idr, version, bill_id, status, last_error, created_at, updated_at
	`, c.SchoolID, c.StudentUserID, c.PeriodYear, c.PeriodMonth, c.Items, c.AmountIDR, c.Version, c.BillID, c.Status, c.LastError).Scan(
		&res.ID, &res.SchoolID, &res.StudentUserID, &res.PeriodYear, &res.PeriodMonth,
		&res.Items, &res.AmountIDR, &res.Version, &res.BillID, &res.Status, &res.LastError,
		&res.CreatedAt, &res.UpdatedAt,
	)
	return &res, err
}

func (s *Store) UpdateChargeStatus(ctx context.Context, schoolID, id, status, billID, lastError string) error {
	var bID *string
	if billID != "" {
		bID = &billID
	}
	var lErr *string
	if lastError != "" {
		lErr = &lastError
	}
	_, err := s.db.ExecContext(ctx, `
		UPDATE transport_charges
		SET status = $1, bill_id = COALESCE($2, bill_id), last_error = $3, updated_at = now()
		WHERE school_id = $4 AND id = $5
	`, status, bID, lErr, schoolID, id)
	return err
}

func (s *Store) ListCharges(ctx context.Context, schoolID string, year, month int, status string) ([]Charge, error) {
	query := `
		SELECT id, school_id, student_user_id, period_year, period_month, items, amount_idr, version, bill_id, status, last_error, created_at, updated_at
		FROM transport_charges
		WHERE school_id = $1
	`
	var args []any
	args = append(args, schoolID)

	if year > 0 {
		args = append(args, year)
		query += fmt.Sprintf(" AND period_year = $%d", len(args))
	}
	if month > 0 {
		args = append(args, month)
		query += fmt.Sprintf(" AND period_month = $%d", len(args))
	}
	if status != "" {
		args = append(args, status)
		query += fmt.Sprintf(" AND status = $%d", len(args))
	}
	query += " ORDER BY created_at DESC"

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []Charge
	for rows.Next() {
		var c Charge
		if err := rows.Scan(&c.ID, &c.SchoolID, &c.StudentUserID, &c.PeriodYear, &c.PeriodMonth,
			&c.Items, &c.AmountIDR, &c.Version, &c.BillID, &c.Status, &c.LastError,
			&c.CreatedAt, &c.UpdatedAt); err != nil {
			return nil, err
		}
		list = append(list, c)
	}
	return list, nil
}

func (s *Store) GetCharge(ctx context.Context, schoolID, id string) (*Charge, error) {
	var c Charge
	err := s.db.QueryRowContext(ctx, `
		SELECT id, school_id, student_user_id, period_year, period_month, items, amount_idr, version, bill_id, status, last_error, created_at, updated_at
		FROM transport_charges
		WHERE school_id = $1 AND id = $2
	`, schoolID, id).Scan(
		&c.ID, &c.SchoolID, &c.StudentUserID, &c.PeriodYear, &c.PeriodMonth,
		&c.Items, &c.AmountIDR, &c.Version, &c.BillID, &c.Status, &c.LastError,
		&c.CreatedAt, &c.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	return &c, nil
}
