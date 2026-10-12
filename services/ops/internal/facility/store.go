package facility

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgconn"

	"skci/ops/internal/store"
)

// ValidationError carries an Indonesian message that is safe to show the
// user (answered with 422).
type ValidationError struct{ Msg string }

func (e *ValidationError) Error() string { return e.Msg }

func invalid(msg string) error { return &ValidationError{Msg: msg} }

var (
	ErrNotFound    = errors.New("data tidak ditemukan")
	ErrDuplicate   = errors.New("kode sudah dipakai")
	ErrHasChildren = errors.New("masih memiliki data turunan")
)

// Store holds the facility SQL. Its read methods (ListRooms, GetRoom,
// ListBuildings, BusyIntervals, ActiveWorkOrders) are the in-process
// contract for other ops packages (T16, T25): use them instead of querying
// facility tables directly (BR-FAC-02).
type Store struct {
	DB  *sql.DB
	Loc *time.Location
}

func NewStore(db *sql.DB, loc *time.Location) *Store { return &Store{DB: db, Loc: loc} }

func pgCode(err error) string {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		return pgErr.Code
	}
	return ""
}

func one(err error) error {
	if errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	}
	return err
}

func affected(res sql.Result, err error) error {
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// inTx runs fn in a transaction, committing only when fn succeeds.
func (s *Store) inTx(ctx context.Context, fn func(tx *sql.Tx) error) error {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	if err := fn(tx); err != nil {
		_ = tx.Rollback()
		return err
	}
	return tx.Commit()
}

// Actor identifies who performs a mutation (for audit + events).
type Actor struct {
	UserID string
	Role   string
	Name   string
}

func (a Actor) audit(schoolID, entityType, entityID, action string, before, after any, reason string) store.AuditEntry {
	e := store.AuditEntry{SchoolID: schoolID, Module: "facility", EntityType: entityType, EntityID: entityID,
		Action: action, Before: before, After: after}
	if a.UserID != "" {
		e.ActorUserID = &a.UserID
	}
	if a.Role != "" {
		e.ActorRole = &a.Role
	}
	if reason != "" {
		e.Reason = &reason
	}
	return e
}

// ---------------------------------------------------------------------------
// Buildings & floors
// ---------------------------------------------------------------------------

type Building struct {
	ID      string     `json:"id"`
	Code    string     `json:"code"`
	Name    string     `json:"name"`
	Address *string    `json:"address"`
	Sort    int        `json:"sort"`
	Floors  []RefFloor `json:"floors"`
}

type BuildingInput struct {
	Code    string  `json:"code"`
	Name    string  `json:"name"`
	Address *string `json:"address"`
	Sort    int     `json:"sort"`
}

type FloorInput struct {
	BuildingID string `json:"building_id"`
	Level      int    `json:"level"`
	Name       string `json:"name"`
}

// ListBuildings returns the school's buildings with their floors.
func (s *Store) ListBuildings(ctx context.Context, schoolID string) ([]Building, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT id, code, name, address, sort FROM buildings
		WHERE school_id=$1 AND deleted_at IS NULL ORDER BY sort, name`, schoolID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Building{}
	idx := map[string]int{}
	for rows.Next() {
		var b Building
		if err := rows.Scan(&b.ID, &b.Code, &b.Name, &b.Address, &b.Sort); err != nil {
			return nil, err
		}
		b.Floors = []RefFloor{}
		idx[b.ID] = len(out)
		out = append(out, b)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	frows, err := s.DB.QueryContext(ctx, `SELECT building_id, id, level, name FROM floors
		WHERE school_id=$1 AND deleted_at IS NULL ORDER BY level`, schoolID)
	if err != nil {
		return nil, err
	}
	defer frows.Close()
	for frows.Next() {
		var bid string
		var f RefFloor
		if err := frows.Scan(&bid, &f.ID, &f.Level, &f.Name); err != nil {
			return nil, err
		}
		if i, ok := idx[bid]; ok {
			out[i].Floors = append(out[i].Floors, f)
		}
	}
	return out, frows.Err()
}

func (s *Store) CreateBuilding(ctx context.Context, schoolID string, in BuildingInput, a Actor) (string, error) {
	var id string
	err := s.inTx(ctx, func(tx *sql.Tx) error {
		err := tx.QueryRowContext(ctx, `INSERT INTO buildings (school_id, code, name, address, sort)
			VALUES ($1,$2,$3,$4,$5) RETURNING id`, schoolID, in.Code, in.Name, in.Address, in.Sort).Scan(&id)
		if pgCode(err) == "23505" {
			return ErrDuplicate
		}
		if err != nil {
			return err
		}
		return store.Audit(ctx, tx, a.audit(schoolID, "building", id, "create", nil, in, ""))
	})
	return id, err
}

func (s *Store) UpdateBuilding(ctx context.Context, schoolID, id string, in BuildingInput, a Actor) error {
	return s.inTx(ctx, func(tx *sql.Tx) error {
		var before BuildingInput
		if err := one(tx.QueryRowContext(ctx, `SELECT code, name, address, sort FROM buildings
			WHERE id=$1 AND school_id=$2 AND deleted_at IS NULL FOR UPDATE`, id, schoolID).
			Scan(&before.Code, &before.Name, &before.Address, &before.Sort)); err != nil {
			return err
		}
		_, err := tx.ExecContext(ctx, `UPDATE buildings SET code=$3, name=$4, address=$5, sort=$6, updated_at=now()
			WHERE id=$1 AND school_id=$2`, id, schoolID, in.Code, in.Name, in.Address, in.Sort)
		if pgCode(err) == "23505" {
			return ErrDuplicate
		}
		if err != nil {
			return err
		}
		return store.Audit(ctx, tx, a.audit(schoolID, "building", id, "update", before, in, ""))
	})
}

func (s *Store) DeleteBuilding(ctx context.Context, schoolID, id string, a Actor) error {
	return s.inTx(ctx, func(tx *sql.Tx) error {
		var n int
		if err := tx.QueryRowContext(ctx, `SELECT count(*) FROM floors WHERE building_id=$1 AND school_id=$2 AND deleted_at IS NULL`,
			id, schoolID).Scan(&n); err != nil {
			return err
		}
		if n > 0 {
			return ErrHasChildren
		}
		if err := affected(tx.ExecContext(ctx, `UPDATE buildings SET deleted_at=now(), updated_at=now()
			WHERE id=$1 AND school_id=$2 AND deleted_at IS NULL`, id, schoolID)); err != nil {
			return err
		}
		return store.Audit(ctx, tx, a.audit(schoolID, "building", id, "delete", nil, nil, ""))
	})
}

func (s *Store) CreateFloor(ctx context.Context, schoolID string, in FloorInput, a Actor) (string, error) {
	var id string
	err := s.inTx(ctx, func(tx *sql.Tx) error {
		var ok bool
		if err := tx.QueryRowContext(ctx, `SELECT EXISTS (SELECT 1 FROM buildings WHERE id=$1 AND school_id=$2 AND deleted_at IS NULL)`,
			in.BuildingID, schoolID).Scan(&ok); err != nil {
			return err
		}
		if !ok {
			return ErrNotFound
		}
		err := tx.QueryRowContext(ctx, `INSERT INTO floors (school_id, building_id, level, name)
			VALUES ($1,$2,$3,$4) RETURNING id`, schoolID, in.BuildingID, in.Level, in.Name).Scan(&id)
		if pgCode(err) == "23505" {
			return ErrDuplicate
		}
		if err != nil {
			return err
		}
		return store.Audit(ctx, tx, a.audit(schoolID, "floor", id, "create", nil, in, ""))
	})
	return id, err
}

func (s *Store) UpdateFloor(ctx context.Context, schoolID, id string, in FloorInput, a Actor) error {
	return s.inTx(ctx, func(tx *sql.Tx) error {
		var before FloorInput
		if err := one(tx.QueryRowContext(ctx, `SELECT building_id, level, name FROM floors
			WHERE id=$1 AND school_id=$2 AND deleted_at IS NULL FOR UPDATE`, id, schoolID).
			Scan(&before.BuildingID, &before.Level, &before.Name)); err != nil {
			return err
		}
		_, err := tx.ExecContext(ctx, `UPDATE floors SET level=$3, name=$4, updated_at=now() WHERE id=$1 AND school_id=$2`,
			id, schoolID, in.Level, in.Name)
		if pgCode(err) == "23505" {
			return ErrDuplicate
		}
		if err != nil {
			return err
		}
		in.BuildingID = before.BuildingID
		return store.Audit(ctx, tx, a.audit(schoolID, "floor", id, "update", before, in, ""))
	})
}

func (s *Store) DeleteFloor(ctx context.Context, schoolID, id string, a Actor) error {
	return s.inTx(ctx, func(tx *sql.Tx) error {
		var n int
		if err := tx.QueryRowContext(ctx, `SELECT count(*) FROM rooms WHERE floor_id=$1 AND school_id=$2 AND deleted_at IS NULL`,
			id, schoolID).Scan(&n); err != nil {
			return err
		}
		if n > 0 {
			return ErrHasChildren
		}
		if err := affected(tx.ExecContext(ctx, `UPDATE floors SET deleted_at=now(), updated_at=now()
			WHERE id=$1 AND school_id=$2 AND deleted_at IS NULL`, id, schoolID)); err != nil {
			return err
		}
		return store.Audit(ctx, tx, a.audit(schoolID, "floor", id, "delete", nil, nil, ""))
	})
}

// ---------------------------------------------------------------------------
// Rooms
// ---------------------------------------------------------------------------

// RoomFilter narrows ListRooms. Zero values mean "no filter"; Limit 0 = all.
type RoomFilter struct {
	Kind        string
	Status      string
	Bookable    bool
	IDs         []string
	Q           string
	MinCapacity int
	Limit       int
	Offset      int
}

type RoomInput struct {
	FloorID          string  `json:"floor_id"`
	Code             string  `json:"code"`
	Name             string  `json:"name"`
	Kind             string  `json:"kind"`
	Capacity         int     `json:"capacity"`
	Usage            string  `json:"usage"`
	RequiresApproval bool    `json:"requires_approval"`
	Status           string  `json:"status"`
	AvailableFrom    *string `json:"available_from"`
	AvailableTo      *string `json:"available_to"`
	Notes            *string `json:"notes"`
}

const roomSelect = `SELECT r.id, r.floor_id, r.code, r.name, r.kind, r.capacity, r.usage, r.requires_approval, r.status,
	to_char(r.available_from, 'HH24:MI'), to_char(r.available_to, 'HH24:MI'), r.notes,
	b.id, b.name, f.id, f.level, f.name
	FROM rooms r JOIN floors f ON f.id = r.floor_id JOIN buildings b ON b.id = f.building_id`

func scanRoom(sc interface{ Scan(...any) error }) (Room, error) {
	var r Room
	err := sc.Scan(&r.ID, &r.FloorID, &r.Code, &r.Name, &r.Kind, &r.Capacity, &r.Usage, &r.RequiresApproval, &r.Status,
		&r.AvailableFrom, &r.AvailableTo, &r.Notes, &r.Building.ID, &r.Building.Name, &r.Floor.ID, &r.Floor.Level, &r.Floor.Name)
	r.Facilities = []Facility{}
	return r, err
}

// ListRooms returns the school's non-deleted rooms matching f plus the total
// count before pagination. Bookable=true keeps only Bookable rooms
// (AC-116-01, FR-116-002).
func (s *Store) ListRooms(ctx context.Context, schoolID string, f RoomFilter) ([]Room, int, error) {
	where := []string{"r.school_id = $1", "r.deleted_at IS NULL"}
	args := []any{schoolID}
	add := func(cond string, v any) {
		args = append(args, v)
		where = append(where, fmt.Sprintf(cond, len(args)))
	}
	if f.Kind != "" {
		add("r.kind = $%d", f.Kind)
	}
	if f.Status != "" {
		add("r.status = $%d", f.Status)
	}
	if f.Bookable {
		where = append(where, "r.usage = 'bookable'", "r.status = 'active'")
	}
	if len(f.IDs) > 0 {
		add("r.id = ANY($%d::uuid[])", f.IDs)
	}
	if f.Q != "" {
		add("(r.name ILIKE '%%' || $%[1]d || '%%' OR r.code ILIKE '%%' || $%[1]d || '%%')", f.Q)
	}
	if f.MinCapacity > 0 {
		add("r.capacity >= $%d", f.MinCapacity)
	}
	cond := strings.Join(where, " AND ")
	var total int
	if err := s.DB.QueryRowContext(ctx, `SELECT count(*) FROM rooms r WHERE `+cond, args...).Scan(&total); err != nil {
		return nil, 0, err
	}
	q := roomSelect + ` WHERE ` + cond + ` ORDER BY b.sort, b.name, f.level, r.code`
	if f.Limit > 0 {
		q += fmt.Sprintf(" LIMIT %d OFFSET %d", f.Limit, f.Offset)
	}
	rows, err := s.DB.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	out := []Room{}
	for rows.Next() {
		r, err := scanRoom(rows)
		if err != nil {
			return nil, 0, err
		}
		out = append(out, r)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}
	return out, total, s.attachFacilities(ctx, out)
}

func (s *Store) attachFacilities(ctx context.Context, rooms []Room) error {
	if len(rooms) == 0 {
		return nil
	}
	ids := make([]string, len(rooms))
	idx := map[string]int{}
	for i, r := range rooms {
		ids[i] = r.ID
		idx[r.ID] = i
	}
	rows, err := s.DB.QueryContext(ctx, `SELECT room_id, id, name, qty FROM room_facilities
		WHERE room_id = ANY($1::uuid[]) ORDER BY name`, ids)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var rid string
		var f Facility
		if err := rows.Scan(&rid, &f.ID, &f.Name, &f.Qty); err != nil {
			return err
		}
		rooms[idx[rid]].Facilities = append(rooms[idx[rid]].Facilities, f)
	}
	return rows.Err()
}

// GetRoom returns one non-deleted room of the school (ErrNotFound otherwise).
func (s *Store) GetRoom(ctx context.Context, schoolID, id string) (Room, error) {
	r, err := scanRoom(s.DB.QueryRowContext(ctx, roomSelect+` WHERE r.id=$1 AND r.school_id=$2 AND r.deleted_at IS NULL`, id, schoolID))
	if err != nil {
		return Room{}, one(err)
	}
	rooms := []Room{r}
	if err := s.attachFacilities(ctx, rooms); err != nil {
		return Room{}, err
	}
	return rooms[0], nil
}

func (s *Store) floorInSchool(ctx context.Context, q store.Q, schoolID, floorID string) error {
	var ok bool
	if err := q.QueryRowContext(ctx, `SELECT EXISTS (SELECT 1 FROM floors WHERE id=$1 AND school_id=$2 AND deleted_at IS NULL)`,
		floorID, schoolID).Scan(&ok); err != nil {
		return err
	}
	if !ok {
		return ErrNotFound
	}
	return nil
}

func roomWriteErr(err error) error {
	switch pgCode(err) {
	case "23505":
		return ErrDuplicate
	case "23514":
		return invalid("Isian ruang tidak valid (jam pakai atau persetujuan hanya untuk ruang yang dapat dipesan)")
	}
	return err
}

func (s *Store) CreateRoom(ctx context.Context, schoolID string, in RoomInput, a Actor) (string, error) {
	var id string
	err := s.inTx(ctx, func(tx *sql.Tx) error {
		if err := s.floorInSchool(ctx, tx, schoolID, in.FloorID); err != nil {
			return err
		}
		err := tx.QueryRowContext(ctx, `INSERT INTO rooms (school_id, floor_id, code, name, kind, capacity, usage,
			requires_approval, status, available_from, available_to, notes)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10::time,$11::time,$12) RETURNING id`,
			schoolID, in.FloorID, in.Code, in.Name, in.Kind, in.Capacity, in.Usage, in.RequiresApproval, in.Status,
			in.AvailableFrom, in.AvailableTo, in.Notes).Scan(&id)
		if err != nil {
			return roomWriteErr(err)
		}
		return store.Audit(ctx, tx, a.audit(schoolID, "room", id, "create", nil, in, ""))
	})
	return id, err
}

func (s *Store) UpdateRoom(ctx context.Context, schoolID, id string, in RoomInput, a Actor) error {
	return s.inTx(ctx, func(tx *sql.Tx) error {
		before, err := scanRoom(tx.QueryRowContext(ctx, roomSelect+` WHERE r.id=$1 AND r.school_id=$2 AND r.deleted_at IS NULL FOR UPDATE OF r`, id, schoolID))
		if err != nil {
			return one(err)
		}
		if err := s.floorInSchool(ctx, tx, schoolID, in.FloorID); err != nil {
			return err
		}
		_, err = tx.ExecContext(ctx, `UPDATE rooms SET floor_id=$3, code=$4, name=$5, kind=$6, capacity=$7, usage=$8,
			requires_approval=$9, status=$10, available_from=$11::time, available_to=$12::time, notes=$13, updated_at=now()
			WHERE id=$1 AND school_id=$2`, id, schoolID, in.FloorID, in.Code, in.Name, in.Kind, in.Capacity, in.Usage,
			in.RequiresApproval, in.Status, in.AvailableFrom, in.AvailableTo, in.Notes)
		if err != nil {
			return roomWriteErr(err)
		}
		return store.Audit(ctx, tx, a.audit(schoolID, "room", id, "update", before, in, ""))
	})
}

func (s *Store) DeleteRoom(ctx context.Context, schoolID, id string, a Actor) error {
	return s.inTx(ctx, func(tx *sql.Tx) error {
		if err := affected(tx.ExecContext(ctx, `UPDATE rooms SET deleted_at=now(), updated_at=now()
			WHERE id=$1 AND school_id=$2 AND deleted_at IS NULL`, id, schoolID)); err != nil {
			return err
		}
		return store.Audit(ctx, tx, a.audit(schoolID, "room", id, "delete", nil, nil, ""))
	})
}

// ReplaceFacilities sets the room's supporting facilities to exactly list.
func (s *Store) ReplaceFacilities(ctx context.Context, schoolID, roomID string, list []Facility, a Actor) error {
	return s.inTx(ctx, func(tx *sql.Tx) error {
		if _, err := s.lockRoom(ctx, tx, schoolID, roomID); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM room_facilities WHERE room_id=$1 AND school_id=$2`, roomID, schoolID); err != nil {
			return err
		}
		for _, f := range list {
			if _, err := tx.ExecContext(ctx, `INSERT INTO room_facilities (school_id, room_id, name, qty) VALUES ($1,$2,$3,$4)`,
				schoolID, roomID, f.Name, f.Qty); err != nil {
				return err
			}
		}
		return store.Audit(ctx, tx, a.audit(schoolID, "room", roomID, "set_facilities", nil, list, ""))
	})
}

// lockRoom row-locks a room so concurrent bookings on it serialise.
func (s *Store) lockRoom(ctx context.Context, tx *sql.Tx, schoolID, roomID string) (Room, error) {
	r, err := scanRoom(tx.QueryRowContext(ctx, roomSelect+` WHERE r.id=$1 AND r.school_id=$2 AND r.deleted_at IS NULL FOR UPDATE OF r`, roomID, schoolID))
	return r, one(err)
}

// ListFixedSchedules returns the fixed schedules of the given rooms.
func (s *Store) ListFixedSchedules(ctx context.Context, q store.Q, schoolID string, roomIDs []string) ([]FixedSchedule, error) {
	rows, err := q.QueryContext(ctx, `SELECT id, room_id, weekday, to_char(starts_time,'HH24:MI'), to_char(ends_time,'HH24:MI'),
		label, to_char(valid_from,'YYYY-MM-DD'), to_char(valid_to,'YYYY-MM-DD')
		FROM room_fixed_schedules WHERE school_id=$1 AND room_id = ANY($2::uuid[]) ORDER BY weekday, starts_time`, schoolID, roomIDs)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []FixedSchedule{}
	for rows.Next() {
		var f FixedSchedule
		if err := rows.Scan(&f.ID, &f.RoomID, &f.Weekday, &f.StartsTime, &f.EndsTime, &f.Label, &f.ValidFrom, &f.ValidTo); err != nil {
			return nil, err
		}
		out = append(out, f)
	}
	return out, rows.Err()
}

func (s *Store) CreateFixedSchedule(ctx context.Context, schoolID string, in FixedSchedule, a Actor) (string, error) {
	var id string
	err := s.inTx(ctx, func(tx *sql.Tx) error {
		room, err := s.lockRoom(ctx, tx, schoolID, in.RoomID)
		if err != nil {
			return err
		}
		if room.Usage != UsageBookable {
			return invalid("Jadwal tetap hanya untuk ruang yang dapat dipesan; ruang penggunaan tetap sudah tidak bisa dipesan")
		}
		err = tx.QueryRowContext(ctx, `INSERT INTO room_fixed_schedules (school_id, room_id, weekday, starts_time, ends_time, label, valid_from, valid_to)
			VALUES ($1,$2,$3,$4::time,$5::time,$6,$7::date,$8::date) RETURNING id`,
			schoolID, in.RoomID, in.Weekday, in.StartsTime, in.EndsTime, in.Label, in.ValidFrom, in.ValidTo).Scan(&id)
		if pgCode(err) == "23514" {
			return invalid("Jadwal tetap tidak valid: periksa hari, jam, dan masa berlaku")
		}
		if err != nil {
			return err
		}
		return store.Audit(ctx, tx, a.audit(schoolID, "room_fixed_schedule", id, "create", nil, in, ""))
	})
	return id, err
}

func (s *Store) DeleteFixedSchedule(ctx context.Context, schoolID, roomID, id string, a Actor) error {
	return s.inTx(ctx, func(tx *sql.Tx) error {
		if err := affected(tx.ExecContext(ctx, `DELETE FROM room_fixed_schedules WHERE id=$1 AND room_id=$2 AND school_id=$3`,
			id, roomID, schoolID)); err != nil {
			return err
		}
		return store.Audit(ctx, tx, a.audit(schoolID, "room_fixed_schedule", id, "delete", nil, nil, ""))
	})
}

// ---------------------------------------------------------------------------
// Availability & bookings
// ---------------------------------------------------------------------------

// BusyIntervals returns, per room, every occupied period overlapping
// [from, to): active bookings (confirmed + pending_approval) and the fixed
// schedules expanded day by day in the school zone.
func (s *Store) BusyIntervals(ctx context.Context, q store.Q, schoolID string, roomIDs []string, from, to time.Time) (map[string][]BusySlot, error) {
	out := map[string][]BusySlot{}
	for _, id := range roomIDs {
		out[id] = []BusySlot{}
	}
	if len(roomIDs) == 0 {
		return out, nil
	}
	rows, err := q.QueryContext(ctx, `SELECT id, room_id, starts_at, ends_at, purpose, requester_name, status
		FROM room_bookings WHERE school_id=$1 AND room_id = ANY($2::uuid[])
		AND status IN ('pending_approval','confirmed') AND starts_at < $4 AND ends_at > $3
		ORDER BY starts_at`, schoolID, roomIDs, from, to)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var id, roomID, purpose, status string
		var name *string
		var b BusySlot
		if err := rows.Scan(&id, &roomID, &b.Start, &b.End, &purpose, &name, &status); err != nil {
			return nil, err
		}
		b.Kind, b.Label, b.BookedByName, b.BookingID = BusyBooking, purpose, name, &id
		if status == BookingPending {
			b.Label = purpose + " (menunggu persetujuan)"
		}
		out[roomID] = append(out[roomID], b)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	fixed, err := s.ListFixedSchedules(ctx, q, schoolID, roomIDs)
	if err != nil {
		return nil, err
	}
	byRoom := map[string][]FixedSchedule{}
	for _, f := range fixed {
		byRoom[f.RoomID] = append(byRoom[f.RoomID], f)
	}
	localFrom := from.In(s.Loc)
	day := time.Date(localFrom.Year(), localFrom.Month(), localFrom.Day(), 0, 0, 0, 0, s.Loc)
	for ; day.Before(to); day = day.AddDate(0, 0, 1) {
		for roomID, list := range byRoom {
			for _, slot := range ExpandFixed(list, day, s.Loc) {
				if Overlaps(slot.Interval, Interval{Start: from, End: to}) {
					out[roomID] = append(out[roomID], slot)
				}
			}
		}
	}
	return out, nil
}

type Booking struct {
	ID              string    `json:"id"`
	RoomID          string    `json:"room_id"`
	RoomName        string    `json:"room_name"`
	BuildingName    string    `json:"building_name"`
	StartsAt        time.Time `json:"starts_at"`
	EndsAt          time.Time `json:"ends_at"`
	Purpose         string    `json:"purpose"`
	Participants    int       `json:"participants"`
	RequesterUserID string    `json:"requester_user_id"`
	RequesterName   *string   `json:"requester_name"`
	Source          string    `json:"source"`
	SourceService   *string   `json:"source_service"`
	SourceRef       *string   `json:"source_ref"`
	Status          string    `json:"status"`
	ApprovalID      *string   `json:"approval_id"`
	CancelReason    *string   `json:"cancel_reason"`
	CreatedAt       time.Time `json:"created_at"`
}

const bookingSelect = `SELECT bk.id, bk.room_id, r.name, b.name, bk.starts_at, bk.ends_at, bk.purpose, bk.participants,
	bk.requester_user_id, bk.requester_name, bk.source, bk.source_service, bk.source_ref, bk.status, bk.approval_id,
	bk.cancel_reason, bk.created_at
	FROM room_bookings bk JOIN rooms r ON r.id = bk.room_id JOIN floors f ON f.id = r.floor_id JOIN buildings b ON b.id = f.building_id`

func scanBooking(sc interface{ Scan(...any) error }) (Booking, error) {
	var b Booking
	err := sc.Scan(&b.ID, &b.RoomID, &b.RoomName, &b.BuildingName, &b.StartsAt, &b.EndsAt, &b.Purpose, &b.Participants,
		&b.RequesterUserID, &b.RequesterName, &b.Source, &b.SourceService, &b.SourceRef, &b.Status, &b.ApprovalID,
		&b.CancelReason, &b.CreatedAt)
	return b, err
}

func (s *Store) GetBooking(ctx context.Context, schoolID, id string) (Booking, error) {
	b, err := scanBooking(s.DB.QueryRowContext(ctx, bookingSelect+` WHERE bk.id=$1 AND bk.school_id=$2`, id, schoolID))
	return b, one(err)
}

func (s *Store) bookingByKey(ctx context.Context, q store.Q, schoolID, key string) (Booking, error) {
	b, err := scanBooking(q.QueryRowContext(ctx, bookingSelect+` WHERE bk.school_id=$1 AND bk.idempotency_key=$2`, schoolID, key))
	return b, one(err)
}

type BookingFilter struct {
	RequesterUserID string
	RoomID          string
	Status          string
	From, To        *time.Time
	Limit, Offset   int
}

func (s *Store) ListBookings(ctx context.Context, schoolID string, f BookingFilter) ([]Booking, int, error) {
	where := []string{"bk.school_id = $1"}
	args := []any{schoolID}
	add := func(cond string, v any) {
		args = append(args, v)
		where = append(where, fmt.Sprintf(cond, len(args)))
	}
	if f.RequesterUserID != "" {
		add("bk.requester_user_id = $%d", f.RequesterUserID)
	}
	if f.RoomID != "" {
		add("bk.room_id = $%d", f.RoomID)
	}
	if f.Status != "" {
		add("bk.status = $%d", f.Status)
	}
	if f.From != nil {
		add("bk.ends_at > $%d", *f.From)
	}
	if f.To != nil {
		add("bk.starts_at < $%d", *f.To)
	}
	cond := strings.Join(where, " AND ")
	var total int
	if err := s.DB.QueryRowContext(ctx, `SELECT count(*) FROM room_bookings bk WHERE `+cond, args...).Scan(&total); err != nil {
		return nil, 0, err
	}
	rows, err := s.DB.QueryContext(ctx, bookingSelect+` WHERE `+cond+fmt.Sprintf(` ORDER BY bk.starts_at DESC LIMIT %d OFFSET %d`, f.Limit, f.Offset), args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	out := []Booking{}
	for rows.Next() {
		b, err := scanBooking(rows)
		if err != nil {
			return nil, 0, err
		}
		out = append(out, b)
	}
	return out, total, rows.Err()
}

// BookingInput is a validated booking request.
type BookingInput struct {
	SchoolID        string
	RoomID          string
	Period          Interval
	Purpose         string
	Participants    int
	RequesterUserID string
	RequesterName   string
	Source          string
	SourceService   *string
	SourceRef       *string
	IdempotencyKey  string
}

// BookingRejection is returned by CreateBooking when BR-FAC-01 rejects the
// request. Conflict is set for overlaps; Nearest lists alternatives.
type BookingRejection struct {
	Err      error
	Conflict *BusySlot
	Nearest  []Interval
}

func (r *BookingRejection) Error() string { return r.Err.Error() }
func (r *BookingRejection) Unwrap() error { return r.Err }

// ApprovalOpener opens the core approval for a pending booking inside the
// booking transaction; an error aborts the booking (no auto-approve).
type ApprovalOpener func(ctx context.Context, b Booking, room Room) (approvalID string, err error)

// CreateBooking applies CheckBooking under a row lock on the room and
// inserts the booking. The exclusion constraint is the last line of defence
// against races. Returns (booking, created). A repeated idempotency key
// returns the existing booking with created=false.
func (s *Store) CreateBooking(ctx context.Context, in BookingInput, openApproval ApprovalOpener, a Actor) (Booking, bool, error) {
	if existing, err := s.bookingByKey(ctx, s.DB, in.SchoolID, in.IdempotencyKey); err == nil {
		return existing, false, nil
	} else if !errors.Is(err, ErrNotFound) {
		return Booking{}, false, err
	}
	var out Booking
	err := s.inTx(ctx, func(tx *sql.Tx) error {
		room, err := s.lockRoom(ctx, tx, in.SchoolID, in.RoomID)
		if err != nil {
			return err
		}
		window := Window(room, in.Period.Start.In(s.Loc))
		busyMap, err := s.BusyIntervals(ctx, tx, in.SchoolID, []string{room.ID}, window.Start, window.End)
		if err != nil {
			return err
		}
		busy := busyMap[room.ID]
		conflict, err := CheckBooking(room, in.Period, in.Participants, busy, s.Loc)
		if err != nil {
			rej := &BookingRejection{Err: err, Conflict: conflict}
			if errors.Is(err, ErrConflict) {
				rej.Nearest = nearestFor(busy, in.Period, window)
			}
			return rej
		}
		status := BookingConfirmed
		if NeedsApproval(room) {
			status = BookingPending
		}
		var id string
		err = tx.QueryRowContext(ctx, `INSERT INTO room_bookings (school_id, room_id, starts_at, ends_at, purpose, participants,
			requester_user_id, requester_name, source, source_service, source_ref, status, idempotency_key)
			VALUES ($1,$2,$3,$4,$5,$6,$7,NULLIF($8,''),$9,$10,$11,$12,$13) RETURNING id`,
			in.SchoolID, in.RoomID, in.Period.Start, in.Period.End, in.Purpose, in.Participants, in.RequesterUserID,
			in.RequesterName, in.Source, in.SourceService, in.SourceRef, status, in.IdempotencyKey).Scan(&id)
		switch pgCode(err) {
		case "23P01":
			return &BookingRejection{Err: ErrConflict, Nearest: nearestFor(busy, in.Period, window)}
		case "23505":
			return errIdempotentRace
		}
		if err != nil {
			return err
		}
		b, err := scanBooking(tx.QueryRowContext(ctx, bookingSelect+` WHERE bk.id=$1`, id))
		if err != nil {
			return err
		}
		if status == BookingPending {
			approvalID, err := openApproval(ctx, b, room)
			if err != nil {
				return err
			}
			if _, err := tx.ExecContext(ctx, `UPDATE room_bookings SET approval_id=$2 WHERE id=$1`, id, approvalID); err != nil {
				return err
			}
			b.ApprovalID = &approvalID
		}
		out = b
		return store.Audit(ctx, tx, a.audit(in.SchoolID, "room_booking", id, "create", nil, b, ""))
	})
	if errors.Is(err, errIdempotentRace) {
		existing, err := s.bookingByKey(ctx, s.DB, in.SchoolID, in.IdempotencyKey)
		return existing, false, err
	}
	return out, err == nil, err
}

var errIdempotentRace = errors.New("idempotency key inserted concurrently")

func nearestFor(busy []BusySlot, req, window Interval) []Interval {
	ivs := make([]Interval, len(busy))
	for i, b := range busy {
		ivs[i] = b.Interval
	}
	return NearestFreeSlots(ivs, req, window, 2)
}

// CancelBooking cancels a pending/confirmed booking. Returns the booking as
// it was before (callers withdraw its core approval when it was pending).
func (s *Store) CancelBooking(ctx context.Context, schoolID, id, reason string, a Actor) (Booking, error) {
	var before Booking
	err := s.inTx(ctx, func(tx *sql.Tx) error {
		b, err := scanBooking(tx.QueryRowContext(ctx, bookingSelect+` WHERE bk.id=$1 AND bk.school_id=$2 FOR UPDATE OF bk`, id, schoolID))
		if err != nil {
			return one(err)
		}
		if b.Status != BookingPending && b.Status != BookingConfirmed {
			return invalid("Pemesanan ini sudah tidak aktif")
		}
		before = b
		if _, err := tx.ExecContext(ctx, `UPDATE room_bookings SET status='cancelled', cancel_reason=$2, cancelled_by=NULLIF($3,'')::uuid,
			updated_at=now() WHERE id=$1`, id, reason, a.UserID); err != nil {
			return err
		}
		return store.Audit(ctx, tx, a.audit(schoolID, "room_booking", id, "cancel",
			map[string]string{"status": b.Status}, map[string]string{"status": BookingCancelled}, reason))
	})
	return before, err
}

// ApplyApprovalDecision applies core's callback idempotently: only a
// pending booking moves (approved → confirmed, rejected → rejected).
// changed=false means the decision was already applied (or the booking is
// no longer pending) and nothing was written.
func (s *Store) ApplyApprovalDecision(ctx context.Context, bookingID, approvalID, decision string) (Booking, bool, error) {
	to := BookingRejected
	if decision == "approved" {
		to = BookingConfirmed
	}
	var out Booking
	changed := false
	err := s.inTx(ctx, func(tx *sql.Tx) error {
		var schoolID string
		res, err := tx.ExecContext(ctx, `UPDATE room_bookings SET status=$2, updated_at=now()
			WHERE id=$1 AND status='pending_approval' AND (approval_id IS NULL OR approval_id::text=$3)`, bookingID, to, approvalID)
		if err != nil {
			return err
		}
		n, _ := res.RowsAffected()
		changed = n > 0
		if err := one(tx.QueryRowContext(ctx, `SELECT school_id FROM room_bookings WHERE id=$1`, bookingID).Scan(&schoolID)); err != nil {
			return err
		}
		out, err = scanBooking(tx.QueryRowContext(ctx, bookingSelect+` WHERE bk.id=$1`, bookingID))
		if err != nil || !changed {
			return err
		}
		return store.Audit(ctx, tx, Actor{Role: "core"}.audit(schoolID, "room_booking", bookingID, "approval_"+decision,
			map[string]string{"status": BookingPending}, map[string]string{"status": to}, ""))
	})
	return out, changed, err
}

// ---------------------------------------------------------------------------
// Work orders
// ---------------------------------------------------------------------------

type WorkOrder struct {
	ID                 string     `json:"id"`
	Code               string     `json:"code"`
	Source             string     `json:"source"`
	RoomID             *string    `json:"room_id"`
	RoomName           *string    `json:"room_name"`
	BuildingName       *string    `json:"building_name"`
	AssetID            *string    `json:"asset_id"`
	Title              string     `json:"title"`
	Description        string     `json:"description"`
	Priority           string     `json:"priority"`
	Status             string     `json:"status"`
	ReporterUserID     string     `json:"reporter_user_id"`
	ReporterName       *string    `json:"reporter_name"`
	AssigneeUserID     *string    `json:"assignee_user_id"`
	AssigneeName       *string    `json:"assignee_name"`
	CompletionNote     *string    `json:"completion_note"`
	CompletedAt        *time.Time `json:"completed_at"`
	CancelReason       *string    `json:"cancel_reason"`
	ScheduleID         *string    `json:"schedule_id"`
	ScheduledFor       *string    `json:"scheduled_for"`
	TicketRef          *string    `json:"ticket_ref"`
	CallbackURL        *string    `json:"-"`
	PurchaseRequestRef *string    `json:"purchase_request_ref"`
	CreatedAt          time.Time  `json:"created_at"`
	UpdatedAt          time.Time  `json:"updated_at"`
	SchoolID           string     `json:"-"`

	Photos    []Photo   `json:"photos,omitempty"`
	Events    []WOEvent `json:"events,omitempty"`
	Costs     []Cost    `json:"costs,omitempty"`
	TotalCost *int64    `json:"total_cost_idr,omitempty"`
}

type Photo struct {
	ID        string    `json:"id"`
	MIME      string    `json:"mime"`
	SizeBytes int64     `json:"size_bytes"`
	SHA256    string    `json:"sha256"`
	ObjectKey string    `json:"-"`
	CreatedAt time.Time `json:"created_at"`
}

type WOEvent struct {
	FromStatus  *string   `json:"from_status"`
	ToStatus    string    `json:"to_status"`
	Note        *string   `json:"note"`
	ActorUserID *string   `json:"actor_user_id"`
	CreatedAt   time.Time `json:"created_at"`
}

const woSelect = `SELECT w.id, w.code, w.source, w.room_id, r.name, b.name, w.asset_id, w.title, w.description, w.priority,
	w.status, w.reporter_user_id, w.reporter_name, w.assignee_user_id, w.assignee_name, w.completion_note, w.completed_at,
	w.cancel_reason, w.schedule_id, to_char(w.scheduled_for,'YYYY-MM-DD'), w.ticket_ref, w.callback_url,
	w.purchase_request_ref, w.created_at, w.updated_at, w.school_id
	FROM work_orders w LEFT JOIN rooms r ON r.id = w.room_id LEFT JOIN floors f ON f.id = r.floor_id
	LEFT JOIN buildings b ON b.id = f.building_id`

func scanWO(sc interface{ Scan(...any) error }) (WorkOrder, error) {
	var w WorkOrder
	err := sc.Scan(&w.ID, &w.Code, &w.Source, &w.RoomID, &w.RoomName, &w.BuildingName, &w.AssetID, &w.Title, &w.Description,
		&w.Priority, &w.Status, &w.ReporterUserID, &w.ReporterName, &w.AssigneeUserID, &w.AssigneeName, &w.CompletionNote,
		&w.CompletedAt, &w.CancelReason, &w.ScheduleID, &w.ScheduledFor, &w.TicketRef, &w.CallbackURL,
		&w.PurchaseRequestRef, &w.CreatedAt, &w.UpdatedAt, &w.SchoolID)
	return w, err
}

// WOInput is a validated new work order.
type WOInput struct {
	SchoolID       string
	Source         string
	RoomID         *string
	AssetID        *string
	Title          string
	Description    string
	Priority       string
	ReporterUserID string
	ReporterName   string
	AssigneeUserID *string
	AssigneeName   *string
	ScheduleID     *string
	ScheduledFor   *string
	TicketRef      *string
	CallbackURL    *string
	IdempotencyKey *string
}

// insertWO allocates the WO code and inserts the row + first event inside tx.
// Returns created=false when the schedule/idempotency key already exists.
func (s *Store) insertWO(ctx context.Context, tx *sql.Tx, in WOInput, a Actor) (string, bool, error) {
	if in.RoomID != nil {
		var ok bool
		if err := tx.QueryRowContext(ctx, `SELECT EXISTS (SELECT 1 FROM rooms WHERE id=$1 AND school_id=$2 AND deleted_at IS NULL)`,
			*in.RoomID, in.SchoolID).Scan(&ok); err != nil {
			return "", false, err
		}
		if !ok {
			return "", false, ErrNotFound
		}
	}
	status := WODilaporkan
	if in.AssigneeUserID != nil {
		status = WODitugaskan
	}
	code, err := store.NextDocNumber(ctx, tx, in.SchoolID, "WO", time.Now().In(s.Loc).Year())
	if err != nil {
		return "", false, err
	}
	var id string
	err = tx.QueryRowContext(ctx, `INSERT INTO work_orders (school_id, code, source, room_id, asset_id, title, description,
		priority, status, reporter_user_id, reporter_name, assignee_user_id, assignee_name, schedule_id, scheduled_for,
		ticket_ref, callback_url, idempotency_key)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,NULLIF($11,''),$12,$13,$14,$15::date,$16,$17,$18)
		ON CONFLICT DO NOTHING RETURNING id`,
		in.SchoolID, code, in.Source, in.RoomID, in.AssetID, in.Title, in.Description, in.Priority, status,
		in.ReporterUserID, in.ReporterName, in.AssigneeUserID, in.AssigneeName, in.ScheduleID, in.ScheduledFor,
		in.TicketRef, in.CallbackURL, in.IdempotencyKey).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	if err := addEvent(ctx, tx, in.SchoolID, id, nil, status, nil, a.UserID); err != nil {
		return "", false, err
	}
	after := map[string]any{"code": code, "source": in.Source, "room_id": in.RoomID, "asset_id": in.AssetID,
		"title": in.Title, "priority": in.Priority, "status": status}
	return id, true, store.Audit(ctx, tx, a.audit(in.SchoolID, "work_order", id, "create", nil, after, ""))
}

func addEvent(ctx context.Context, q store.Q, schoolID, woID string, from *string, to string, note *string, actorID string) error {
	_, err := q.ExecContext(ctx, `INSERT INTO work_order_events (school_id, work_order_id, from_status, to_status, note, actor_user_id)
		VALUES ($1,$2,$3,$4,$5,NULLIF($6,'')::uuid)`, schoolID, woID, from, to, note, actorID)
	return err
}

// PhotoInput is an already-stored photo object to attach.
type PhotoInput struct {
	ObjectKey string
	MIME      string
	Size      int64
	SHA256    string
}

// CreateWorkOrder inserts a WO with its photos. A repeated idempotency key
// returns the existing WO with created=false.
func (s *Store) CreateWorkOrder(ctx context.Context, in WOInput, photos []PhotoInput, a Actor) (WorkOrder, bool, error) {
	var id string
	created := false
	err := s.inTx(ctx, func(tx *sql.Tx) error {
		var err error
		id, created, err = s.insertWO(ctx, tx, in, a)
		if err != nil || !created {
			return err
		}
		for _, p := range photos {
			if _, err := tx.ExecContext(ctx, `INSERT INTO work_order_photos (school_id, work_order_id, object_key, mime, size_bytes, sha256, uploaded_by)
				VALUES ($1,$2,$3,$4,$5,$6,$7)`, in.SchoolID, id, p.ObjectKey, p.MIME, p.Size, p.SHA256, a.UserID); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return WorkOrder{}, false, err
	}
	if !created {
		if in.IdempotencyKey == nil {
			return WorkOrder{}, false, errors.New("perintah kerja duplikat")
		}
		w, err := scanWO(s.DB.QueryRowContext(ctx, woSelect+` WHERE w.school_id=$1 AND w.idempotency_key=$2`, in.SchoolID, *in.IdempotencyKey))
		return w, false, one(err)
	}
	w, err := s.GetWorkOrder(ctx, in.SchoolID, id)
	return w, true, err
}

// GetWorkOrder returns one WO with photos, events, costs and total.
func (s *Store) GetWorkOrder(ctx context.Context, schoolID, id string) (WorkOrder, error) {
	w, err := scanWO(s.DB.QueryRowContext(ctx, woSelect+` WHERE w.id=$1 AND w.school_id=$2`, id, schoolID))
	if err != nil {
		return WorkOrder{}, one(err)
	}
	w.Photos, w.Events, w.Costs = []Photo{}, []WOEvent{}, []Cost{}
	prow, err := s.DB.QueryContext(ctx, `SELECT id, mime, size_bytes, sha256, object_key, created_at FROM work_order_photos
		WHERE work_order_id=$1 ORDER BY created_at`, id)
	if err != nil {
		return w, err
	}
	defer prow.Close()
	for prow.Next() {
		var p Photo
		if err := prow.Scan(&p.ID, &p.MIME, &p.SizeBytes, &p.SHA256, &p.ObjectKey, &p.CreatedAt); err != nil {
			return w, err
		}
		w.Photos = append(w.Photos, p)
	}
	erow, err := s.DB.QueryContext(ctx, `SELECT from_status, to_status, note, actor_user_id::text, created_at FROM work_order_events
		WHERE work_order_id=$1 ORDER BY created_at, id`, id)
	if err != nil {
		return w, err
	}
	defer erow.Close()
	for erow.Next() {
		var e WOEvent
		if err := erow.Scan(&e.FromStatus, &e.ToStatus, &e.Note, &e.ActorUserID, &e.CreatedAt); err != nil {
			return w, err
		}
		w.Events = append(w.Events, e)
	}
	if w.Costs, err = s.listCosts(ctx, id); err != nil {
		return w, err
	}
	total := WOTotalCost(w.Costs)
	w.TotalCost = &total
	return w, nil
}

func (s *Store) listCosts(ctx context.Context, woID string) ([]Cost, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT id, kind, description, amount_idr, created_by, created_at FROM work_order_costs
		WHERE work_order_id=$1 ORDER BY created_at`, woID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Cost{}
	for rows.Next() {
		var c Cost
		if err := rows.Scan(&c.ID, &c.Kind, &c.Description, &c.AmountIDR, &c.CreatedBy, &c.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// GetPhoto returns one photo of a WO in the school.
func (s *Store) GetPhoto(ctx context.Context, schoolID, woID, photoID string) (Photo, error) {
	var p Photo
	err := s.DB.QueryRowContext(ctx, `SELECT id, mime, size_bytes, sha256, object_key, created_at FROM work_order_photos
		WHERE id=$1 AND work_order_id=$2 AND school_id=$3`, photoID, woID, schoolID).
		Scan(&p.ID, &p.MIME, &p.SizeBytes, &p.SHA256, &p.ObjectKey, &p.CreatedAt)
	return p, one(err)
}

type WOFilter struct {
	Status         string
	ActiveOnly     bool
	Priority       string
	RoomID         string
	RoomIDs        []string
	AssigneeUserID string
	ReporterUserID string
	TicketRef      string
	Limit, Offset  int
}

func (s *Store) ListWorkOrders(ctx context.Context, schoolID string, f WOFilter) ([]WorkOrder, int, error) {
	where := []string{"w.school_id = $1"}
	args := []any{schoolID}
	add := func(cond string, v any) {
		args = append(args, v)
		where = append(where, fmt.Sprintf(cond, len(args)))
	}
	if f.ActiveOnly {
		where = append(where, "w.status NOT IN ('selesai','ditutup','dibatalkan')")
	} else if f.Status != "" {
		add("w.status = $%d", f.Status)
	}
	if f.Priority != "" {
		add("w.priority = $%d", f.Priority)
	}
	if f.RoomID != "" {
		add("w.room_id = $%d", f.RoomID)
	}
	if len(f.RoomIDs) > 0 {
		add("w.room_id = ANY($%d::uuid[])", f.RoomIDs)
	}
	if f.AssigneeUserID != "" {
		add("w.assignee_user_id = $%d", f.AssigneeUserID)
	}
	if f.ReporterUserID != "" {
		add("w.reporter_user_id = $%d", f.ReporterUserID)
	}
	if f.TicketRef != "" {
		add("w.ticket_ref = $%d", f.TicketRef)
	}
	cond := strings.Join(where, " AND ")
	var total int
	if err := s.DB.QueryRowContext(ctx, `SELECT count(*) FROM work_orders w WHERE `+cond, args...).Scan(&total); err != nil {
		return nil, 0, err
	}
	q := woSelect + ` WHERE ` + cond + ` ORDER BY CASE w.priority WHEN 'darurat' THEN 0 WHEN 'tinggi' THEN 1 WHEN 'sedang' THEN 2 ELSE 3 END, w.created_at DESC`
	if f.Limit > 0 {
		q += fmt.Sprintf(" LIMIT %d OFFSET %d", f.Limit, f.Offset)
	}
	rows, err := s.DB.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	out := []WorkOrder{}
	for rows.Next() {
		w, err := scanWO(rows)
		if err != nil {
			return nil, 0, err
		}
		out = append(out, w)
	}
	return out, total, rows.Err()
}

// ActiveWorkOrders is the in-process read for T25's room status overlay.
func (s *Store) ActiveWorkOrders(ctx context.Context, schoolID string, roomIDs []string) ([]WorkOrder, error) {
	list, _, err := s.ListWorkOrders(ctx, schoolID, WOFilter{ActiveOnly: true, RoomIDs: roomIDs})
	return list, err
}

// WOChange describes a status change; Apply may set extra columns.
type WOChange struct {
	To             string
	Note           *string
	AssigneeUserID *string
	AssigneeName   *string
	PurchaseRef    *string
	CancelReason   *string
	// Allowed decides, given the current row, whether the change is allowed
	// (wraps CanTransition with the caller's relation to the WO).
	Allowed func(w WorkOrder) bool
}

// ErrTransition is returned when CanTransition rejects a change.
var ErrTransition = errors.New("Perubahan status perintah kerja tidak diizinkan")

// ErrForbidden is returned when the caller has no relation to the WO.
var ErrForbidden = errors.New("Anda tidak berhak mengubah perintah kerja ini")

// TransitionWO locks the WO, validates the change and writes the new status,
// the event row and the audit row atomically. Returns (before, after).
func (s *Store) TransitionWO(ctx context.Context, schoolID, id string, ch WOChange, a Actor) (WorkOrder, WorkOrder, error) {
	var before, after WorkOrder
	err := s.inTx(ctx, func(tx *sql.Tx) error {
		w, err := scanWO(tx.QueryRowContext(ctx, woSelect+` WHERE w.id=$1 AND w.school_id=$2 FOR UPDATE OF w`, id, schoolID))
		if err != nil {
			return one(err)
		}
		before = w
		if !ch.Allowed(w) {
			return ErrTransition
		}
		_, err = tx.ExecContext(ctx, `UPDATE work_orders SET status=$2,
			assignee_user_id = COALESCE($3, assignee_user_id), assignee_name = COALESCE($4, assignee_name),
			purchase_request_ref = COALESCE($5, purchase_request_ref), cancel_reason = COALESCE($6, cancel_reason),
			completion_note = CASE WHEN $2 = 'selesai' THEN $7 ELSE completion_note END,
			completed_at = CASE WHEN $2 = 'selesai' THEN now() ELSE completed_at END,
			updated_at = now() WHERE id=$1`,
			id, ch.To, ch.AssigneeUserID, ch.AssigneeName, ch.PurchaseRef, ch.CancelReason, ch.Note)
		if err != nil {
			return err
		}
		from := w.Status
		if err := addEvent(ctx, tx, schoolID, id, &from, ch.To, ch.Note, a.UserID); err != nil {
			return err
		}
		after, err = scanWO(tx.QueryRowContext(ctx, woSelect+` WHERE w.id=$1`, id))
		if err != nil {
			return err
		}
		reason := ""
		if ch.CancelReason != nil {
			reason = *ch.CancelReason
		}
		return store.Audit(ctx, tx, a.audit(schoolID, "work_order", id, "status_"+ch.To,
			map[string]any{"status": from, "assignee_user_id": w.AssigneeUserID},
			map[string]any{"status": ch.To, "assignee_user_id": after.AssigneeUserID, "purchase_request_ref": after.PurchaseRequestRef}, reason))
	})
	return before, after, err
}

// AddCost appends a cost line (FR-118-004) while the WO is still open.
func (s *Store) AddCost(ctx context.Context, schoolID, woID string, c Cost, allowed func(WorkOrder) bool, a Actor) (string, error) {
	var id string
	err := s.inTx(ctx, func(tx *sql.Tx) error {
		w, err := scanWO(tx.QueryRowContext(ctx, woSelect+` WHERE w.id=$1 AND w.school_id=$2 FOR UPDATE OF w`, woID, schoolID))
		if err != nil {
			return one(err)
		}
		if !allowed(w) {
			return ErrForbidden
		}
		if w.Status == WODitutup || w.Status == WODibatalkan {
			return ErrTransition
		}
		if err := tx.QueryRowContext(ctx, `INSERT INTO work_order_costs (school_id, work_order_id, kind, description, amount_idr, created_by)
			VALUES ($1,$2,$3,$4,$5,$6) RETURNING id`, schoolID, woID, c.Kind, c.Description, c.AmountIDR, a.UserID).Scan(&id); err != nil {
			return err
		}
		return store.Audit(ctx, tx, a.audit(schoolID, "work_order_cost", id, "create", nil, c, ""))
	})
	return id, err
}

func (s *Store) DeleteCost(ctx context.Context, schoolID, woID, costID string, allowed func(WorkOrder) bool, a Actor) error {
	return s.inTx(ctx, func(tx *sql.Tx) error {
		w, err := scanWO(tx.QueryRowContext(ctx, woSelect+` WHERE w.id=$1 AND w.school_id=$2 FOR UPDATE OF w`, woID, schoolID))
		if err != nil {
			return one(err)
		}
		if !allowed(w) {
			return ErrForbidden
		}
		if w.Status == WODitutup || w.Status == WODibatalkan {
			return ErrTransition
		}
		var before Cost
		if err := one(tx.QueryRowContext(ctx, `DELETE FROM work_order_costs WHERE id=$1 AND work_order_id=$2 AND school_id=$3
			RETURNING kind, description, amount_idr`, costID, woID, schoolID).Scan(&before.Kind, &before.Description, &before.AmountIDR)); err != nil {
			return err
		}
		return store.Audit(ctx, tx, a.audit(schoolID, "work_order_cost", costID, "delete", before, nil, ""))
	})
}

// ---------------------------------------------------------------------------
// Maintenance schedules (FR-118-003)
// ---------------------------------------------------------------------------

const scheduleSelect = `SELECT m.id, m.room_id, m.asset_id, r.name, m.title, m.priority, m.interval_unit, m.interval_count,
	to_char(m.next_due_date,'YYYY-MM-DD'), m.default_assignee_user_id, m.default_assignee_name, m.active, m.deleted_at, m.school_id, m.created_by
	FROM maintenance_schedules m LEFT JOIN rooms r ON r.id = m.room_id`

func scanSchedule(sc interface{ Scan(...any) error }) (Schedule, error) {
	var m Schedule
	err := sc.Scan(&m.ID, &m.RoomID, &m.AssetID, &m.RoomName, &m.Title, &m.Priority, &m.IntervalUnit, &m.IntervalCount,
		&m.NextDueDate, &m.DefaultAssigneeUserID, &m.DefaultAssigneeName, &m.Active, &m.DeletedAt, &m.SchoolID, &m.CreatedBy)
	return m, err
}

func (s *Store) ListSchedules(ctx context.Context, schoolID string) ([]Schedule, error) {
	rows, err := s.DB.QueryContext(ctx, scheduleSelect+` WHERE m.school_id=$1 AND m.deleted_at IS NULL ORDER BY m.next_due_date`, schoolID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Schedule{}
	for rows.Next() {
		m, err := scanSchedule(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

func (s *Store) SaveSchedule(ctx context.Context, schoolID, id string, m Schedule, a Actor) (string, error) {
	err := s.inTx(ctx, func(tx *sql.Tx) error {
		if m.RoomID != nil {
			var ok bool
			if err := tx.QueryRowContext(ctx, `SELECT EXISTS (SELECT 1 FROM rooms WHERE id=$1 AND school_id=$2 AND deleted_at IS NULL)`,
				*m.RoomID, schoolID).Scan(&ok); err != nil {
				return err
			}
			if !ok {
				return ErrNotFound
			}
		}
		var before any
		if id == "" {
			if err := tx.QueryRowContext(ctx, `INSERT INTO maintenance_schedules (school_id, room_id, asset_id, title, priority,
				interval_unit, interval_count, next_due_date, default_assignee_user_id, default_assignee_name, active, created_by)
				VALUES ($1,$2,$3,$4,$5,$6,$7,$8::date,$9,$10,$11,$12) RETURNING id`, schoolID, m.RoomID, m.AssetID, m.Title, m.Priority,
				m.IntervalUnit, m.IntervalCount, m.NextDueDate, m.DefaultAssigneeUserID, m.DefaultAssigneeName, m.Active, a.UserID).Scan(&id); err != nil {
				return err
			}
		} else {
			old, err := scanSchedule(tx.QueryRowContext(ctx, scheduleSelect+` WHERE m.id=$1 AND m.school_id=$2 AND m.deleted_at IS NULL FOR UPDATE OF m`, id, schoolID))
			if err != nil {
				return one(err)
			}
			before = old
			if _, err := tx.ExecContext(ctx, `UPDATE maintenance_schedules SET room_id=$3, asset_id=$4, title=$5, priority=$6,
				interval_unit=$7, interval_count=$8, next_due_date=$9::date, default_assignee_user_id=$10, default_assignee_name=$11,
				active=$12, updated_at=now() WHERE id=$1 AND school_id=$2`, id, schoolID, m.RoomID, m.AssetID, m.Title, m.Priority,
				m.IntervalUnit, m.IntervalCount, m.NextDueDate, m.DefaultAssigneeUserID, m.DefaultAssigneeName, m.Active); err != nil {
				return err
			}
		}
		action := "update"
		if before == nil {
			action = "create"
		}
		return store.Audit(ctx, tx, a.audit(schoolID, "maintenance_schedule", id, action, before, m, ""))
	})
	return id, err
}

func (s *Store) DeleteSchedule(ctx context.Context, schoolID, id string, a Actor) error {
	return s.inTx(ctx, func(tx *sql.Tx) error {
		if err := affected(tx.ExecContext(ctx, `UPDATE maintenance_schedules SET deleted_at=now(), active=false, updated_at=now()
			WHERE id=$1 AND school_id=$2 AND deleted_at IS NULL`, id, schoolID)); err != nil {
			return err
		}
		return store.Audit(ctx, tx, a.audit(schoolID, "maintenance_schedule", id, "delete", nil, nil, ""))
	})
}

// GeneratedWO is a preventive WO created by RunDueSchedules.
type GeneratedWO struct {
	SchoolID       string
	WorkOrderID    string
	Title          string
	AssigneeUserID *string
}

// RunDueSchedules is the preventive-maintenance job body (AC-118-01): for
// each due schedule it creates one WO for next_due_date (ON CONFLICT
// (schedule_id, scheduled_for) DO NOTHING) and advances next_due_date by one
// period, both in one transaction per schedule. Running it twice on the same
// day therefore yields a single WO.
func (s *Store) RunDueSchedules(ctx context.Context, today string) ([]GeneratedWO, error) {
	rows, err := s.DB.QueryContext(ctx, scheduleSelect+` WHERE m.active AND m.deleted_at IS NULL AND m.next_due_date <= $1::date`, today)
	if err != nil {
		return nil, err
	}
	var candidates []Schedule
	for rows.Next() {
		m, err := scanSchedule(rows)
		if err != nil {
			rows.Close()
			return nil, err
		}
		candidates = append(candidates, m)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	out := []GeneratedWO{}
	for _, m := range DueSchedules(candidates, today) {
		var gen *GeneratedWO
		err := s.inTx(ctx, func(tx *sql.Tx) error {
			var due string
			// Re-read under lock: another runner may have advanced it.
			if err := tx.QueryRowContext(ctx, `SELECT to_char(next_due_date,'YYYY-MM-DD') FROM maintenance_schedules
				WHERE id=$1 AND active AND deleted_at IS NULL AND next_due_date <= $2::date FOR UPDATE`, m.ID, today).Scan(&due); err != nil {
				if errors.Is(err, sql.ErrNoRows) {
					return nil
				}
				return err
			}
			scheduleID := m.ID
			id, created, err := s.insertWO(ctx, tx, WOInput{
				SchoolID: m.SchoolID, Source: SourcePreventive, RoomID: m.RoomID, AssetID: m.AssetID,
				Title: m.Title, Description: "Perawatan berkala " + due, Priority: m.Priority,
				ReporterUserID: m.CreatedBy, AssigneeUserID: m.DefaultAssigneeUserID, AssigneeName: m.DefaultAssigneeName,
				ScheduleID: &scheduleID, ScheduledFor: &due,
			}, Actor{Role: "system"})
			if err != nil {
				return err
			}
			dueDate, _ := time.Parse("2006-01-02", due)
			next := NextDue(dueDate, m.IntervalUnit, m.IntervalCount).Format("2006-01-02")
			if _, err := tx.ExecContext(ctx, `UPDATE maintenance_schedules SET next_due_date=$2::date, updated_at=now() WHERE id=$1`, m.ID, next); err != nil {
				return err
			}
			if created {
				gen = &GeneratedWO{SchoolID: m.SchoolID, WorkOrderID: id, Title: m.Title, AssigneeUserID: m.DefaultAssigneeUserID}
			}
			return nil
		})
		if err != nil {
			return out, err
		}
		if gen != nil {
			out = append(out, *gen)
		}
	}
	return out, nil
}
