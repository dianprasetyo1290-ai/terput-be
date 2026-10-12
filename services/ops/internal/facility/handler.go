package facility

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"skci/ops/internal/client"
	"skci/ops/internal/common"
	"skci/ops/internal/storage"
)

// Handler serves the facility HTTP API (public JWT routes in this file,
// internal X-Internal-Key routes in internal.go).
type Handler struct {
	Store        *Store
	Client       *client.Client
	Storage      storage.Storage
	MaxUpload    int64
	CallbackBase string // OPS_PUBLIC_INTERNAL_URL

	// Hooks registered by later tasks (nil-safe, see hooks.go).
	Purchase PurchaseRequester
	Assets   AssetLookup
}

// Officers manage the facility master and work orders.
var Officers = []string{common.RoleFacilityOfficer, common.RoleAdmin, common.RoleSuperadmin}

// Readers may additionally read all bookings and work orders.
var Readers = append(slices.Clone(Officers), common.RoleKepalaSekolah)

// MaxPhotos caps the number of damage photos per report.
const MaxPhotos = 5

// PhotoMIMEs are the accepted damage-photo types.
var PhotoMIMEs = []string{"image/jpeg", "image/png", "image/webp"}

var uuidRe = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

func isUUID(s string) bool { return uuidRe.MatchString(s) }

func actorOf(c *common.Claims) Actor {
	return Actor{UserID: c.UserID, Role: c.ActiveRole, Name: c.Name}
}

func pageArgs(r *http.Request) (page, perPage, offset int) {
	page, perPage = common.Page(r)
	return page, perPage, (page - 1) * perPage
}

func writeList(w http.ResponseWriter, data any, page, perPage, total int) {
	common.WriteJSON(w, http.StatusOK, data, common.Meta{Page: page, PerPage: perPage, Total: total})
}

// writeStoreErr maps store errors to HTTP statuses.
func writeStoreErr(w http.ResponseWriter, err error, what string) {
	var rej *BookingRejection
	var httpErr *client.HTTPError
	var verr *ValidationError
	switch {
	case errors.As(err, &rej):
		writeRejection(w, rej)
	case errors.Is(err, ErrNotFound):
		common.WriteError(w, http.StatusNotFound, what+" tidak ditemukan")
	case errors.Is(err, ErrDuplicate):
		common.WriteError(w, http.StatusConflict, "Kode sudah dipakai di sekolah ini")
	case errors.Is(err, ErrHasChildren):
		common.WriteError(w, http.StatusConflict, what+" masih memiliki data di bawahnya; hapus data tersebut terlebih dahulu")
	case errors.Is(err, ErrTransition):
		common.WriteError(w, http.StatusUnprocessableEntity, ErrTransition.Error())
	case errors.Is(err, ErrForbidden):
		common.WriteError(w, http.StatusForbidden, ErrForbidden.Error())
	case errors.As(err, &httpErr):
		if httpErr.Status == http.StatusUnprocessableEntity {
			common.WriteError(w, http.StatusUnprocessableEntity, httpErr.Message())
			return
		}
		log.Printf("facility downstream error: %v", err)
		common.WriteError(w, http.StatusBadGateway, "Layanan terkait sedang tidak dapat dihubungi")
	case errors.As(err, &verr):
		common.WriteError(w, http.StatusUnprocessableEntity, verr.Msg)
	default:
		log.Printf("facility error: %v", err)
		common.WriteError(w, http.StatusInternalServerError, "Gagal memproses "+strings.ToLower(what))
	}
}

// rejectionBody is the 409/422 payload for a refused booking (AC-117-01).
type rejectionBody struct {
	Error        string     `json:"error"`
	Conflict     *BusySlot  `json:"conflict,omitempty"`
	NearestSlots []Interval `json:"nearest_slots"`
}

func writeRejection(w http.ResponseWriter, rej *BookingRejection) {
	status := http.StatusUnprocessableEntity
	msg := rej.Err.Error()
	if errors.Is(rej.Err, ErrConflict) {
		status = http.StatusConflict
		if rej.Conflict != nil {
			msg = "Bentrok dengan " + rej.Conflict.Label
		}
		if len(rej.Nearest) == 0 {
			msg += "; tidak ada slot kosong hari ini"
		}
	}
	nearest := rej.Nearest
	if nearest == nil {
		nearest = []Interval{}
	}
	common.WriteEnvelope(w, status, common.Envelope{
		Data:    rejectionBody{Error: msg, Conflict: rej.Conflict, NearestSlots: nearest},
		Message: msg,
		Errors:  map[string]string{"message": msg},
	})
}

func randomKey() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// ---------------------------------------------------------------------------
// Validation helpers
// ---------------------------------------------------------------------------

func validateRoom(in *RoomInput) error {
	in.Code, in.Name = strings.TrimSpace(in.Code), strings.TrimSpace(in.Name)
	if in.Status == "" {
		in.Status = RoomActive
	}
	switch {
	case !isUUID(in.FloorID):
		return errors.New("Lantai wajib dipilih")
	case in.Code == "" || in.Name == "":
		return errors.New("Kode dan nama ruang wajib diisi")
	case !slices.Contains(RoomKinds, in.Kind):
		return errors.New("Jenis ruang tidak valid")
	case !slices.Contains(RoomUsages, in.Usage):
		return errors.New("Penggunaan ruang tidak valid")
	case !slices.Contains(RoomStatuses, in.Status):
		return errors.New("Status ruang tidak valid")
	case in.Capacity < 0:
		return errors.New("Kapasitas tidak boleh negatif")
	case in.Usage == UsageFixed && in.RequiresApproval:
		return errors.New("Persetujuan hanya untuk ruang yang dapat dipesan")
	case (in.AvailableFrom == nil) != (in.AvailableTo == nil):
		return errors.New("Jam pakai mulai dan selesai harus diisi bersamaan")
	}
	if in.AvailableFrom != nil {
		from, err1 := ParseClock(*in.AvailableFrom)
		to, err2 := ParseClock(*in.AvailableTo)
		if err1 != nil || err2 != nil || to <= from {
			return errors.New("Jam pakai ruang tidak valid")
		}
	}
	return nil
}

func parseTime(s string) (time.Time, error) {
	t, err := time.Parse(time.RFC3339, strings.TrimSpace(s))
	if err != nil {
		return time.Time{}, errors.New("Format waktu tidak valid (RFC3339)")
	}
	return t, nil
}

func splitIDs(s string) ([]string, bool) {
	if strings.TrimSpace(s) == "" {
		return nil, true
	}
	var out []string
	for _, p := range strings.Split(s, ",") {
		p = strings.TrimSpace(p)
		if !isUUID(p) {
			return nil, false
		}
		out = append(out, p)
	}
	return out, true
}

// ---------------------------------------------------------------------------
// Buildings & floors (facility_officer)
// ---------------------------------------------------------------------------

func (h *Handler) ListBuildings(w http.ResponseWriter, r *http.Request) {
	list, err := h.Store.ListBuildings(r.Context(), common.EffectiveSchoolID(r))
	if err != nil {
		writeStoreErr(w, err, "Gedung")
		return
	}
	common.WriteJSON(w, http.StatusOK, list, nil)
}

func (h *Handler) decodeBuilding(w http.ResponseWriter, r *http.Request) (BuildingInput, bool) {
	var in BuildingInput
	if !common.Decode(w, r, &in) {
		return in, false
	}
	in.Code, in.Name = strings.TrimSpace(in.Code), strings.TrimSpace(in.Name)
	if in.Code == "" || in.Name == "" {
		common.WriteError(w, http.StatusUnprocessableEntity, "Kode dan nama gedung wajib diisi")
		return in, false
	}
	return in, true
}

func (h *Handler) CreateBuilding(w http.ResponseWriter, r *http.Request) {
	in, ok := h.decodeBuilding(w, r)
	if !ok {
		return
	}
	id, err := h.Store.CreateBuilding(r.Context(), common.EffectiveSchoolID(r), in, actorOf(common.FromCtx(r)))
	if err != nil {
		writeStoreErr(w, err, "Gedung")
		return
	}
	common.WriteJSON(w, http.StatusCreated, map[string]string{"id": id}, nil)
}

func (h *Handler) UpdateBuilding(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	in, ok := h.decodeBuilding(w, r)
	if !ok {
		return
	}
	if !isUUID(id) {
		common.WriteError(w, http.StatusNotFound, "Gedung tidak ditemukan")
		return
	}
	if err := h.Store.UpdateBuilding(r.Context(), common.EffectiveSchoolID(r), id, in, actorOf(common.FromCtx(r))); err != nil {
		writeStoreErr(w, err, "Gedung")
		return
	}
	common.WriteJSON(w, http.StatusOK, map[string]string{"id": id}, nil)
}

func (h *Handler) DeleteBuilding(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if !isUUID(id) {
		common.WriteError(w, http.StatusNotFound, "Gedung tidak ditemukan")
		return
	}
	if err := h.Store.DeleteBuilding(r.Context(), common.EffectiveSchoolID(r), id, actorOf(common.FromCtx(r))); err != nil {
		writeStoreErr(w, err, "Gedung")
		return
	}
	common.WriteJSON(w, http.StatusOK, map[string]string{"id": id}, nil)
}

func (h *Handler) decodeFloor(w http.ResponseWriter, r *http.Request) (FloorInput, bool) {
	var in FloorInput
	if !common.Decode(w, r, &in) {
		return in, false
	}
	in.Name = strings.TrimSpace(in.Name)
	if in.Name == "" {
		common.WriteError(w, http.StatusUnprocessableEntity, "Nama lantai wajib diisi")
		return in, false
	}
	return in, true
}

func (h *Handler) CreateFloor(w http.ResponseWriter, r *http.Request) {
	in, ok := h.decodeFloor(w, r)
	if !ok {
		return
	}
	if !isUUID(in.BuildingID) {
		common.WriteError(w, http.StatusUnprocessableEntity, "Gedung wajib dipilih")
		return
	}
	id, err := h.Store.CreateFloor(r.Context(), common.EffectiveSchoolID(r), in, actorOf(common.FromCtx(r)))
	if err != nil {
		writeStoreErr(w, err, "Lantai")
		return
	}
	common.WriteJSON(w, http.StatusCreated, map[string]string{"id": id}, nil)
}

func (h *Handler) UpdateFloor(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	in, ok := h.decodeFloor(w, r)
	if !ok {
		return
	}
	if !isUUID(id) {
		common.WriteError(w, http.StatusNotFound, "Lantai tidak ditemukan")
		return
	}
	if err := h.Store.UpdateFloor(r.Context(), common.EffectiveSchoolID(r), id, in, actorOf(common.FromCtx(r))); err != nil {
		writeStoreErr(w, err, "Lantai")
		return
	}
	common.WriteJSON(w, http.StatusOK, map[string]string{"id": id}, nil)
}

func (h *Handler) DeleteFloor(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if !isUUID(id) {
		common.WriteError(w, http.StatusNotFound, "Lantai tidak ditemukan")
		return
	}
	if err := h.Store.DeleteFloor(r.Context(), common.EffectiveSchoolID(r), id, actorOf(common.FromCtx(r))); err != nil {
		writeStoreErr(w, err, "Lantai")
		return
	}
	common.WriteJSON(w, http.StatusOK, map[string]string{"id": id}, nil)
}

// ---------------------------------------------------------------------------
// Rooms
// ---------------------------------------------------------------------------

// ListRooms: all staff. ?bookable=true keeps only Bookable rooms
// (AC-116-01) and, with from/to, drops rooms already busy in that period.
func (h *Handler) ListRooms(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	schoolID := common.EffectiveSchoolID(r)
	page, perPage, offset := pageArgs(r)
	minCap, _ := strconv.Atoi(q.Get("min_capacity"))
	f := RoomFilter{Kind: q.Get("kind"), Status: q.Get("status"), Bookable: q.Get("bookable") == "true",
		Q: strings.TrimSpace(q.Get("q")), MinCapacity: minCap, Limit: perPage, Offset: offset}

	var period *Interval
	if q.Get("from") != "" || q.Get("to") != "" {
		from, err1 := parseTime(q.Get("from"))
		to, err2 := parseTime(q.Get("to"))
		if err1 != nil || err2 != nil || !to.After(from) {
			common.WriteError(w, http.StatusBadRequest, "Rentang waktu tidak valid")
			return
		}
		period = &Interval{Start: from, End: to}
		f.Limit, f.Offset = 0, 0 // availability filter runs before pagination
	}
	rooms, total, err := h.Store.ListRooms(r.Context(), schoolID, f)
	if err != nil {
		writeStoreErr(w, err, "Ruang")
		return
	}
	if period != nil {
		rooms, err = h.freeRooms(r.Context(), schoolID, rooms, *period)
		if err != nil {
			writeStoreErr(w, err, "Ruang")
			return
		}
		total = len(rooms)
		end := min(offset+perPage, len(rooms))
		if offset >= len(rooms) {
			rooms = []Room{}
		} else {
			rooms = rooms[offset:end]
		}
	}
	writeList(w, rooms, page, perPage, total)
}

// freeRooms keeps rooms with no busy slot and within hours for period.
func (h *Handler) freeRooms(ctx context.Context, schoolID string, rooms []Room, period Interval) ([]Room, error) {
	ids := make([]string, len(rooms))
	for i, r := range rooms {
		ids[i] = r.ID
	}
	busy, err := h.Store.BusyIntervals(ctx, h.Store.DB, schoolID, ids, period.Start, period.End)
	if err != nil {
		return nil, err
	}
	out := []Room{}
	for _, room := range rooms {
		if _, err := CheckBooking(room, period, 1, busy[room.ID], h.Store.Loc); err == nil {
			out = append(out, room)
		}
	}
	return out, nil
}

func (h *Handler) GetRoom(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if !isUUID(id) {
		common.WriteError(w, http.StatusNotFound, "Ruang tidak ditemukan")
		return
	}
	room, err := h.Store.GetRoom(r.Context(), common.EffectiveSchoolID(r), id)
	if err != nil {
		writeStoreErr(w, err, "Ruang")
		return
	}
	common.WriteJSON(w, http.StatusOK, room, nil)
}

func (h *Handler) CreateRoom(w http.ResponseWriter, r *http.Request) {
	var in RoomInput
	if !common.Decode(w, r, &in) {
		return
	}
	if err := validateRoom(&in); err != nil {
		common.WriteError(w, http.StatusUnprocessableEntity, err.Error())
		return
	}
	id, err := h.Store.CreateRoom(r.Context(), common.EffectiveSchoolID(r), in, actorOf(common.FromCtx(r)))
	if err != nil {
		writeStoreErr(w, err, "Ruang")
		return
	}
	common.WriteJSON(w, http.StatusCreated, map[string]string{"id": id}, nil)
}

func (h *Handler) UpdateRoom(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	var in RoomInput
	if !common.Decode(w, r, &in) {
		return
	}
	if !isUUID(id) {
		common.WriteError(w, http.StatusNotFound, "Ruang tidak ditemukan")
		return
	}
	if err := validateRoom(&in); err != nil {
		common.WriteError(w, http.StatusUnprocessableEntity, err.Error())
		return
	}
	if err := h.Store.UpdateRoom(r.Context(), common.EffectiveSchoolID(r), id, in, actorOf(common.FromCtx(r))); err != nil {
		writeStoreErr(w, err, "Ruang")
		return
	}
	common.WriteJSON(w, http.StatusOK, map[string]string{"id": id}, nil)
}

func (h *Handler) DeleteRoom(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if !isUUID(id) {
		common.WriteError(w, http.StatusNotFound, "Ruang tidak ditemukan")
		return
	}
	if err := h.Store.DeleteRoom(r.Context(), common.EffectiveSchoolID(r), id, actorOf(common.FromCtx(r))); err != nil {
		writeStoreErr(w, err, "Ruang")
		return
	}
	common.WriteJSON(w, http.StatusOK, map[string]string{"id": id}, nil)
}

// SetFacilities replaces the room's supporting facilities.
func (h *Handler) SetFacilities(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	var body struct {
		Facilities []Facility `json:"facilities"`
	}
	if !common.Decode(w, r, &body) {
		return
	}
	if !isUUID(id) {
		common.WriteError(w, http.StatusNotFound, "Ruang tidak ditemukan")
		return
	}
	for i := range body.Facilities {
		body.Facilities[i].Name = strings.TrimSpace(body.Facilities[i].Name)
		if body.Facilities[i].Name == "" || body.Facilities[i].Qty <= 0 {
			common.WriteError(w, http.StatusUnprocessableEntity, "Nama fasilitas wajib diisi dan jumlah minimal 1")
			return
		}
	}
	if err := h.Store.ReplaceFacilities(r.Context(), common.EffectiveSchoolID(r), id, body.Facilities, actorOf(common.FromCtx(r))); err != nil {
		writeStoreErr(w, err, "Ruang")
		return
	}
	common.WriteJSON(w, http.StatusOK, body.Facilities, nil)
}

func (h *Handler) ListFixedSchedules(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if !isUUID(id) {
		common.WriteError(w, http.StatusNotFound, "Ruang tidak ditemukan")
		return
	}
	list, err := h.Store.ListFixedSchedules(r.Context(), h.Store.DB, common.EffectiveSchoolID(r), []string{id})
	if err != nil {
		writeStoreErr(w, err, "Jadwal tetap")
		return
	}
	common.WriteJSON(w, http.StatusOK, list, nil)
}

func (h *Handler) CreateFixedSchedule(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	var in FixedSchedule
	if !common.Decode(w, r, &in) {
		return
	}
	if !isUUID(id) {
		common.WriteError(w, http.StatusNotFound, "Ruang tidak ditemukan")
		return
	}
	in.RoomID, in.Label = id, strings.TrimSpace(in.Label)
	from, err1 := ParseClock(in.StartsTime)
	to, err2 := ParseClock(in.EndsTime)
	if in.Label == "" || in.Weekday < 1 || in.Weekday > 7 || err1 != nil || err2 != nil || to <= from {
		common.WriteError(w, http.StatusUnprocessableEntity, "Hari, jam mulai/selesai, dan keterangan jadwal tetap wajib valid")
		return
	}
	if _, err := time.Parse("2006-01-02", in.ValidFrom); err != nil {
		common.WriteError(w, http.StatusUnprocessableEntity, "Tanggal mulai berlaku wajib diisi")
		return
	}
	sid, err := h.Store.CreateFixedSchedule(r.Context(), common.EffectiveSchoolID(r), in, actorOf(common.FromCtx(r)))
	if err != nil {
		writeStoreErr(w, err, "Ruang")
		return
	}
	common.WriteJSON(w, http.StatusCreated, map[string]string{"id": sid}, nil)
}

func (h *Handler) DeleteFixedSchedule(w http.ResponseWriter, r *http.Request) {
	id, sid := chi.URLParam(r, "id"), chi.URLParam(r, "scheduleId")
	if !isUUID(id) || !isUUID(sid) {
		common.WriteError(w, http.StatusNotFound, "Jadwal tetap tidak ditemukan")
		return
	}
	if err := h.Store.DeleteFixedSchedule(r.Context(), common.EffectiveSchoolID(r), id, sid, actorOf(common.FromCtx(r))); err != nil {
		writeStoreErr(w, err, "Jadwal tetap")
		return
	}
	common.WriteJSON(w, http.StatusOK, map[string]string{"id": sid}, nil)
}

// RoomCalendar returns the room's busy slots in [from, to) (FR-117-004).
func (h *Handler) RoomCalendar(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	from, err1 := parseTime(r.URL.Query().Get("from"))
	to, err2 := parseTime(r.URL.Query().Get("to"))
	if err1 != nil || err2 != nil || !to.After(from) || to.Sub(from) > 62*24*time.Hour {
		common.WriteError(w, http.StatusBadRequest, "Rentang kalender tidak valid (maksimal 62 hari)")
		return
	}
	if !isUUID(id) {
		common.WriteError(w, http.StatusNotFound, "Ruang tidak ditemukan")
		return
	}
	schoolID := common.EffectiveSchoolID(r)
	room, err := h.Store.GetRoom(r.Context(), schoolID, id)
	if err != nil {
		writeStoreErr(w, err, "Ruang")
		return
	}
	busy, err := h.Store.BusyIntervals(r.Context(), h.Store.DB, schoolID, []string{id}, from, to)
	if err != nil {
		writeStoreErr(w, err, "Kalender")
		return
	}
	common.WriteJSON(w, http.StatusOK, map[string]any{"room": room, "busy": busy[id]}, nil)
}

// ---------------------------------------------------------------------------
// Bookings
// ---------------------------------------------------------------------------

type bookingBody struct {
	RoomID       string `json:"room_id"`
	StartsAt     string `json:"starts_at"`
	EndsAt       string `json:"ends_at"`
	Purpose      string `json:"purpose"`
	Participants int    `json:"participants"`
}

func (b bookingBody) validate() (Interval, error) {
	if !isUUID(b.RoomID) {
		return Interval{}, errors.New("Ruang wajib dipilih")
	}
	if strings.TrimSpace(b.Purpose) == "" {
		return Interval{}, errors.New("Tujuan pemesanan wajib diisi")
	}
	if b.Participants <= 0 {
		return Interval{}, errors.New("Jumlah peserta minimal 1")
	}
	start, err := parseTime(b.StartsAt)
	if err != nil {
		return Interval{}, err
	}
	end, err := parseTime(b.EndsAt)
	if err != nil {
		return Interval{}, err
	}
	if !end.After(start) {
		return Interval{}, ErrInvalidPeriod
	}
	if !start.After(time.Now()) {
		return Interval{}, errors.New("Waktu mulai sudah lewat")
	}
	return Interval{Start: start, End: end}, nil
}

// approvalOpener opens core's room_booking approval (FR-117-003).
func (h *Handler) approvalOpener(schoolID string) ApprovalOpener {
	return func(ctx context.Context, b Booking, room Room) (string, error) {
		local := b.StartsAt.In(h.Store.Loc)
		when := local.Format("02-01-2006 15:04") + "–" + b.EndsAt.In(h.Store.Loc).Format("15:04")
		return h.Client.CreateApproval(ctx, client.ApprovalRequest{
			SchoolID:      schoolID,
			RequestType:   "room_booking",
			SourceService: "ops",
			SourceID:      b.ID,
			Title:         fmt.Sprintf("Pemesanan %s %s", room.Name, when),
			Summary: map[string]any{"ruang": room.Name, "waktu": when, "tujuan": b.Purpose,
				"peserta": b.Participants},
			MakerUserID:    b.RequesterUserID,
			CallbackURL:    h.CallbackBase + "/internal/approval-callbacks",
			CorrelationID:  b.ID,
			IdempotencyKey: "room_booking:" + b.ID,
		})
	}
}

func (h *Handler) CreateBooking(w http.ResponseWriter, r *http.Request) {
	c := common.FromCtx(r)
	var body bookingBody
	if !common.Decode(w, r, &body) {
		return
	}
	period, err := body.validate()
	if err != nil {
		common.WriteError(w, http.StatusUnprocessableEntity, err.Error())
		return
	}
	schoolID := common.EffectiveSchoolID(r)
	key := strings.TrimSpace(r.Header.Get("Idempotency-Key"))
	if key == "" {
		key = randomKey()
	}
	b, created, err := h.Store.CreateBooking(r.Context(), BookingInput{
		SchoolID: schoolID, RoomID: body.RoomID, Period: period, Purpose: strings.TrimSpace(body.Purpose),
		Participants: body.Participants, RequesterUserID: c.UserID, RequesterName: c.Name, Source: "portal",
		IdempotencyKey: "portal:" + c.UserID + ":" + key,
	}, h.approvalOpener(schoolID), actorOf(c))
	if err != nil {
		writeStoreErr(w, err, "Ruang")
		return
	}
	status := http.StatusOK
	if created {
		status = http.StatusCreated
	}
	common.WriteJSON(w, status, b, nil)
}

func (h *Handler) MyBookings(w http.ResponseWriter, r *http.Request) {
	page, perPage, offset := pageArgs(r)
	list, total, err := h.Store.ListBookings(r.Context(), common.EffectiveSchoolID(r),
		BookingFilter{RequesterUserID: common.FromCtx(r).UserID, Status: r.URL.Query().Get("status"), Limit: perPage, Offset: offset})
	if err != nil {
		writeStoreErr(w, err, "Pemesanan")
		return
	}
	writeList(w, list, page, perPage, total)
}

func (h *Handler) ListBookings(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	page, perPage, offset := pageArgs(r)
	f := BookingFilter{Status: q.Get("status"), Limit: perPage, Offset: offset}
	if rid := q.Get("room_id"); rid != "" {
		if !isUUID(rid) {
			common.WriteError(w, http.StatusBadRequest, "room_id tidak valid")
			return
		}
		f.RoomID = rid
	}
	if v := q.Get("from"); v != "" {
		t, err := parseTime(v)
		if err != nil {
			common.WriteError(w, http.StatusBadRequest, err.Error())
			return
		}
		f.From = &t
	}
	if v := q.Get("to"); v != "" {
		t, err := parseTime(v)
		if err != nil {
			common.WriteError(w, http.StatusBadRequest, err.Error())
			return
		}
		f.To = &t
	}
	list, total, err := h.Store.ListBookings(r.Context(), common.EffectiveSchoolID(r), f)
	if err != nil {
		writeStoreErr(w, err, "Pemesanan")
		return
	}
	writeList(w, list, page, perPage, total)
}

// CancelBooking: the requester cancels their own booking; facility_officer
// (and admin/superadmin) may cancel any booking with a mandatory reason.
func (h *Handler) CancelBooking(w http.ResponseWriter, r *http.Request) {
	c := common.FromCtx(r)
	id := chi.URLParam(r, "id")
	var body struct {
		Reason string `json:"reason"`
	}
	if !common.Decode(w, r, &body) {
		return
	}
	body.Reason = strings.TrimSpace(body.Reason)
	if !isUUID(id) {
		common.WriteError(w, http.StatusNotFound, "Pemesanan tidak ditemukan")
		return
	}
	schoolID := common.EffectiveSchoolID(r)
	b, err := h.Store.GetBooking(r.Context(), schoolID, id)
	if err != nil {
		writeStoreErr(w, err, "Pemesanan")
		return
	}
	officer := common.HasActiveRole(c, Officers...)
	if b.RequesterUserID != c.UserID && !officer {
		common.WriteError(w, http.StatusForbidden, "Anda hanya dapat membatalkan pemesanan milik sendiri")
		return
	}
	if b.RequesterUserID != c.UserID && body.Reason == "" {
		common.WriteError(w, http.StatusUnprocessableEntity, "Alasan pembatalan wajib diisi")
		return
	}
	h.cancelBooking(w, r, schoolID, id, body.Reason, actorOf(c))
}

func (h *Handler) cancelBooking(w http.ResponseWriter, r *http.Request, schoolID, id, reason string, a Actor) {
	before, err := h.Store.CancelBooking(r.Context(), schoolID, id, reason, a)
	if err != nil {
		writeStoreErr(w, err, "Pemesanan")
		return
	}
	if before.Status == BookingPending && before.ApprovalID != nil {
		if err := h.Client.CancelApproval(r.Context(), *before.ApprovalID, a.UserID, reason); err != nil {
			log.Printf("facility: cancel approval %s: %v", *before.ApprovalID, err)
		}
	}
	common.WriteJSON(w, http.StatusOK, map[string]string{"id": id, "status": BookingCancelled}, nil)
}

// ApprovalDecision is core's callback body (T1 §3).
type ApprovalDecision struct {
	ApprovalID  string `json:"approval_id"`
	SourceID    string `json:"source_id"`
	RequestType string `json:"request_type"`
	Decision    string `json:"decision"`
	DecidedBy   string `json:"decided_by"`
	Comment     string `json:"comment"`
}

// ApplyApprovalDecision handles a room_booking callback idempotently and
// notifies the requester once per decision.
func (h *Handler) ApplyApprovalDecision(ctx context.Context, d ApprovalDecision) (Booking, error) {
	b, changed, err := h.Store.ApplyApprovalDecision(ctx, d.SourceID, d.ApprovalID, d.Decision)
	if err != nil || !changed {
		return b, err
	}
	schoolID, _ := h.bookingSchool(ctx, b.ID)
	title := "Pemesanan ruang disetujui"
	if b.Status == BookingRejected {
		title = "Pemesanan ruang ditolak"
	}
	h.notify(ctx, client.Notification{SchoolID: schoolID, UserID: b.RequesterUserID, Type: "facility_booking",
		Title: title, Body: fmt.Sprintf("%s, %s", b.RoomName, b.StartsAt.In(h.Store.Loc).Format("02-01-2006 15:04")),
		Data: map[string]any{"booking_id": b.ID, "status": b.Status}}, "booking:"+b.ID+":"+b.Status)
	return b, nil
}

func (h *Handler) bookingSchool(ctx context.Context, id string) (string, error) {
	var s string
	err := h.Store.DB.QueryRowContext(ctx, `SELECT school_id FROM room_bookings WHERE id=$1`, id).Scan(&s)
	return s, err
}

func (h *Handler) notify(ctx context.Context, n client.Notification, key string) {
	if err := h.Client.Notify(ctx, n, key); err != nil {
		log.Printf("facility: notify %s: %v", key, err)
	}
}

// ---------------------------------------------------------------------------
// Work orders
// ---------------------------------------------------------------------------

// Capabilities tells the UI which optional hooks are live (hide the
// purchase-request button and asset field when not).
func (h *Handler) Capabilities(w http.ResponseWriter, r *http.Request) {
	procurement := false
	if h.Purchase != nil {
		on, err := h.Client.FeatureEnabled(r.Context(), common.EffectiveSchoolID(r), "procurement")
		procurement = err == nil && on
	}
	common.WriteJSON(w, http.StatusOK, map[string]bool{"purchase_request": procurement, "asset": h.Assets != nil}, nil)
}

// resolveTarget validates room/asset of a new WO or schedule.
func (h *Handler) resolveTarget(ctx context.Context, schoolID string, roomID, assetID *string) (*string, error) {
	if assetID != nil && *assetID != "" {
		if h.Assets == nil {
			return roomID, invalid("Modul aset belum aktif")
		}
		if !isUUID(*assetID) {
			return roomID, invalid("Aset tidak valid")
		}
		assetRoom, ok, err := h.Assets.Exists(ctx, schoolID, *assetID)
		if err != nil {
			return roomID, err
		}
		if !ok {
			return roomID, invalid("Aset tidak ditemukan")
		}
		if (roomID == nil || *roomID == "") && assetRoom != "" {
			roomID = &assetRoom
		}
	}
	if roomID != nil && *roomID != "" && !isUUID(*roomID) {
		return roomID, invalid("Ruang tidak valid")
	}
	if (roomID == nil || *roomID == "") && (assetID == nil || *assetID == "") {
		return roomID, invalid("Ruang atau aset wajib dipilih")
	}
	return roomID, nil
}

func nilIfEmpty(s string) *string {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	return &s
}

// CreateWorkOrder accepts a damage report (FR-118-001) as multipart:
// room_id / asset_id, title, description, priority, photos[] (≤ MaxPhotos,
// each ≤ MAX_UPLOAD_BYTES, image/jpeg|png|webp, sha256 recorded).
func (h *Handler) CreateWorkOrder(w http.ResponseWriter, r *http.Request) {
	c := common.FromCtx(r)
	schoolID := common.EffectiveSchoolID(r)
	r.Body = http.MaxBytesReader(w, r.Body, h.MaxUpload*MaxPhotos+(1<<20))
	if err := r.ParseMultipartForm(4 << 20); err != nil {
		common.WriteError(w, http.StatusBadRequest, "Data laporan atau ukuran foto tidak valid")
		return
	}
	defer r.MultipartForm.RemoveAll()
	title := strings.TrimSpace(r.FormValue("title"))
	priority := r.FormValue("priority")
	if title == "" || !slices.Contains(Priorities, priority) {
		common.WriteError(w, http.StatusUnprocessableEntity, "Judul dan prioritas laporan wajib diisi")
		return
	}
	roomID, err := h.resolveTarget(r.Context(), schoolID, nilIfEmpty(r.FormValue("room_id")), nilIfEmpty(r.FormValue("asset_id")))
	if err != nil {
		writeStoreErr(w, err, "Ruang")
		return
	}
	files := r.MultipartForm.File["photos"]
	if len(files) > MaxPhotos {
		common.WriteError(w, http.StatusUnprocessableEntity, fmt.Sprintf("Maksimal %d foto", MaxPhotos))
		return
	}
	var photos []PhotoInput
	for _, fh := range files {
		if fh.Size <= 0 || fh.Size > h.MaxUpload {
			common.WriteError(w, http.StatusUnprocessableEntity, "Ukuran foto melebihi batas")
			return
		}
		f, err := fh.Open()
		if err != nil {
			common.WriteError(w, http.StatusBadRequest, "Foto tidak dapat dibaca")
			return
		}
		data, err := io.ReadAll(f)
		f.Close()
		if err != nil {
			common.WriteError(w, http.StatusBadRequest, "Foto tidak dapat dibaca")
			return
		}
		mime := http.DetectContentType(data)
		if !slices.Contains(PhotoMIMEs, mime) {
			common.WriteError(w, http.StatusUnprocessableEntity, "Foto harus berformat JPG, PNG, atau WebP")
			return
		}
		key := fmt.Sprintf("facility/%s/%s", schoolID, randomKey())
		obj, err := h.Storage.Put(r.Context(), key, mime, bytes.NewReader(data), int64(len(data)))
		if err != nil {
			log.Printf("facility: store photo: %v", err)
			common.WriteError(w, http.StatusInternalServerError, "Gagal menyimpan foto")
			return
		}
		photos = append(photos, PhotoInput{ObjectKey: obj.Key, MIME: obj.MIME, Size: obj.Size, SHA256: obj.Checksum})
	}
	wo, created, err := h.Store.CreateWorkOrder(r.Context(), WOInput{
		SchoolID: schoolID, Source: SourceReport, RoomID: roomID, AssetID: nilIfEmpty(r.FormValue("asset_id")),
		Title: title, Description: strings.TrimSpace(r.FormValue("description")), Priority: priority,
		ReporterUserID: c.UserID, ReporterName: c.Name, IdempotencyKey: nilIfEmpty(r.Header.Get("Idempotency-Key")),
	}, photos, actorOf(c))
	if err != nil || !created {
		for _, p := range photos {
			_ = h.Storage.Delete(r.Context(), p.ObjectKey)
		}
	}
	if err != nil {
		writeStoreErr(w, err, "Ruang")
		return
	}
	status := http.StatusOK
	if created {
		status = http.StatusCreated
	}
	common.WriteJSON(w, status, wo, nil)
}

func (h *Handler) MyWorkOrders(w http.ResponseWriter, r *http.Request) {
	page, perPage, offset := pageArgs(r)
	list, total, err := h.Store.ListWorkOrders(r.Context(), common.EffectiveSchoolID(r),
		WOFilter{ReporterUserID: common.FromCtx(r).UserID, Limit: perPage, Offset: offset})
	if err != nil {
		writeStoreErr(w, err, "Perintah kerja")
		return
	}
	writeList(w, list, page, perPage, total)
}

// ListWorkOrders: officers and kepala_sekolah see all; teknisi only the WOs
// assigned to them; any other staff only their own reports.
func (h *Handler) ListWorkOrders(w http.ResponseWriter, r *http.Request) {
	c := common.FromCtx(r)
	q := r.URL.Query()
	page, perPage, offset := pageArgs(r)
	f := WOFilter{Priority: q.Get("priority"), Limit: perPage, Offset: offset}
	if s := q.Get("status"); s == "active" {
		f.ActiveOnly = true
	} else {
		f.Status = s
	}
	if v := q.Get("room_id"); v != "" {
		if !isUUID(v) {
			common.WriteError(w, http.StatusBadRequest, "room_id tidak valid")
			return
		}
		f.RoomID = v
	}
	if v := q.Get("assignee"); v != "" {
		if !isUUID(v) {
			common.WriteError(w, http.StatusBadRequest, "assignee tidak valid")
			return
		}
		f.AssigneeUserID = v
	}
	switch {
	case common.HasActiveRole(c, Readers...):
	case c.ActiveRole == common.RoleTeknisi:
		f.AssigneeUserID = c.UserID
	default:
		f.ReporterUserID = c.UserID
	}
	list, total, err := h.Store.ListWorkOrders(r.Context(), common.EffectiveSchoolID(r), f)
	if err != nil {
		writeStoreErr(w, err, "Perintah kerja")
		return
	}
	writeList(w, list, page, perPage, total)
}

// relations returns the caller's relations to a WO (officer, assignee,
// reporter); kepala_sekolah reads but has no relation that mutates.
func relations(c *common.Claims, wo WorkOrder) []ActorKind {
	var out []ActorKind
	if common.HasActiveRole(c, Officers...) {
		out = append(out, ActorOfficer)
	}
	if wo.AssigneeUserID != nil && *wo.AssigneeUserID == c.UserID {
		out = append(out, ActorAssignee)
	}
	if wo.ReporterUserID == c.UserID {
		out = append(out, ActorReporter)
	}
	return out
}

func canRead(c *common.Claims, wo WorkOrder) bool {
	return common.HasActiveRole(c, Readers...) || len(relations(c, wo)) > 0
}

// loadWO fetches a WO the caller may read (404 for other schools, 403 for
// unrelated staff such as another teknisi).
func (h *Handler) loadWO(w http.ResponseWriter, r *http.Request) (WorkOrder, bool) {
	id := chi.URLParam(r, "id")
	if !isUUID(id) {
		common.WriteError(w, http.StatusNotFound, "Perintah kerja tidak ditemukan")
		return WorkOrder{}, false
	}
	wo, err := h.Store.GetWorkOrder(r.Context(), common.EffectiveSchoolID(r), id)
	if err != nil {
		writeStoreErr(w, err, "Perintah kerja")
		return WorkOrder{}, false
	}
	if !canRead(common.FromCtx(r), wo) {
		common.WriteError(w, http.StatusForbidden, "Anda tidak berhak melihat perintah kerja ini")
		return WorkOrder{}, false
	}
	return wo, true
}

func (h *Handler) GetWorkOrder(w http.ResponseWriter, r *http.Request) {
	if wo, ok := h.loadWO(w, r); ok {
		common.WriteJSON(w, http.StatusOK, wo, nil)
	}
}

// WorkOrderPhoto streams one photo through the authenticated API (no public
// URL).
func (h *Handler) WorkOrderPhoto(w http.ResponseWriter, r *http.Request) {
	wo, ok := h.loadWO(w, r)
	if !ok {
		return
	}
	pid := chi.URLParam(r, "photoId")
	if !isUUID(pid) {
		common.WriteError(w, http.StatusNotFound, "Foto tidak ditemukan")
		return
	}
	p, err := h.Store.GetPhoto(r.Context(), common.EffectiveSchoolID(r), wo.ID, pid)
	if err != nil {
		writeStoreErr(w, err, "Foto")
		return
	}
	f, err := h.Storage.Open(r.Context(), p.ObjectKey)
	if err != nil {
		common.WriteError(w, http.StatusNotFound, "Foto tidak ditemukan")
		return
	}
	defer f.Close()
	w.Header().Set("Content-Type", p.MIME)
	w.Header().Set("Cache-Control", "private, max-age=300")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	_, _ = io.Copy(w, f)
}

// transition runs a status change after checking the caller's relation:
// no relation → 403, relation but disallowed change → 422.
func (h *Handler) transition(w http.ResponseWriter, r *http.Request, wo WorkOrder, ch WOChange, mutateAllowed []ActorKind) (WorkOrder, bool) {
	c := common.FromCtx(r)
	rel := relations(c, wo)
	permitted := false
	for _, k := range rel {
		if slices.Contains(mutateAllowed, k) {
			permitted = true
		}
	}
	if !permitted {
		common.WriteError(w, http.StatusForbidden, ErrForbidden.Error())
		return WorkOrder{}, false
	}
	ch.Allowed = func(cur WorkOrder) bool { return CanTransition(cur.Status, ch.To, relations(c, cur)...) }
	_, after, err := h.Store.TransitionWO(r.Context(), common.EffectiveSchoolID(r), wo.ID, ch, actorOf(c))
	if err != nil {
		writeStoreErr(w, err, "Perintah kerja")
		return WorkOrder{}, false
	}
	h.afterTransition(r.Context(), after)
	return after, true
}

// afterTransition sends the notifications and the T11 ticket callback.
func (h *Handler) afterTransition(ctx context.Context, wo WorkOrder) {
	switch wo.Status {
	case WODitugaskan:
		if wo.AssigneeUserID != nil {
			h.notify(ctx, client.Notification{SchoolID: wo.SchoolID, UserID: *wo.AssigneeUserID, Type: "facility_work_order",
				Title: "Perintah kerja baru: " + wo.Code, Body: wo.Title, Data: map[string]any{"work_order_id": wo.ID}},
				"wo:"+wo.ID+":assigned:"+*wo.AssigneeUserID)
		}
	case WOSelesai:
		h.notify(ctx, client.Notification{SchoolID: wo.SchoolID, UserID: wo.ReporterUserID, Type: "facility_work_order",
			Title: "Laporan kerusakan selesai: " + wo.Code, Body: wo.Title, Data: map[string]any{"work_order_id": wo.ID}},
			"wo:"+wo.ID+":selesai")
	}
	h.sendCallback(ctx, wo)
}

// sendCallback posts the WO status to the ticket's callback_url (T9 §4.2);
// a failure is parked in core's exception queue (kind callback).
func (h *Handler) sendCallback(ctx context.Context, wo WorkOrder) {
	if wo.CallbackURL == nil || *wo.CallbackURL == "" {
		return
	}
	key := "wo:" + wo.ID + ":" + wo.Status
	body := map[string]any{"work_order_id": wo.ID, "ticket_ref": wo.TicketRef, "status": wo.Status, "completion_note": wo.CompletionNote}
	err := h.Client.PostCallback(ctx, *wo.CallbackURL, body, key)
	if err == nil {
		return
	}
	log.Printf("facility: callback %s: %v", key, err)
	if exErr := h.Client.PostException(ctx, client.Exception{SchoolID: wo.SchoolID, SourceService: "ops", Kind: "callback",
		Severity: "warning", CorrelationID: wo.ID, ErrorMessage: err.Error(),
		PayloadSummary: map[string]any{"work_order_id": wo.ID, "code": wo.Code, "status": wo.Status},
		MaxAttempts:    1, IdempotencyKey: "callback:" + key}); exErr != nil {
		log.Printf("facility: post exception %s: %v", key, exErr)
	}
}

// AssignWorkOrder (facility_officer): the assignee must hold the teknisi
// role in this school (checked with identity).
func (h *Handler) AssignWorkOrder(w http.ResponseWriter, r *http.Request) {
	var body struct {
		AssigneeUserID string `json:"assignee_user_id"`
	}
	if !common.Decode(w, r, &body) {
		return
	}
	wo, ok := h.loadWO(w, r)
	if !ok {
		return
	}
	tech, err := h.findTechnician(r.Context(), common.EffectiveSchoolID(r), body.AssigneeUserID)
	if err != nil {
		writeStoreErr(w, err, "Teknisi")
		return
	}
	if after, ok := h.transition(w, r, wo, WOChange{To: WODitugaskan, AssigneeUserID: &tech.ID, AssigneeName: &tech.Name},
		[]ActorKind{ActorOfficer}); ok {
		common.WriteJSON(w, http.StatusOK, after, nil)
	}
}

func (h *Handler) findTechnician(ctx context.Context, schoolID, userID string) (client.User, error) {
	if !isUUID(userID) {
		return client.User{}, invalid("Teknisi wajib dipilih")
	}
	users, err := h.Client.UsersWithRole(ctx, schoolID, common.RoleTeknisi)
	if err != nil {
		return client.User{}, err
	}
	for _, u := range users {
		if u.ID == userID {
			return u, nil
		}
	}
	return client.User{}, invalid("Pengguna tersebut bukan teknisi di sekolah ini")
}

// Technicians lists the school's teknisi accounts for the assign picker.
func (h *Handler) Technicians(w http.ResponseWriter, r *http.Request) {
	users, err := h.Client.UsersWithRole(r.Context(), common.EffectiveSchoolID(r), common.RoleTeknisi)
	if err != nil {
		writeStoreErr(w, err, "Teknisi")
		return
	}
	if users == nil {
		users = []client.User{}
	}
	common.WriteJSON(w, http.StatusOK, users, nil)
}

// ChangeWorkOrderStatus (teknisi penerima / facility_officer); selesai
// requires a completion note (FR-118-002).
func (h *Handler) ChangeWorkOrderStatus(w http.ResponseWriter, r *http.Request) {
	var body struct {
		To   string `json:"to"`
		Note string `json:"note"`
	}
	if !common.Decode(w, r, &body) {
		return
	}
	note := nilIfEmpty(body.Note)
	if body.To == WOSelesai && note == nil {
		common.WriteError(w, http.StatusUnprocessableEntity, "Catatan penyelesaian wajib diisi")
		return
	}
	if body.To != WODiproses && body.To != WOMenungguMaterial && body.To != WOSelesai {
		common.WriteError(w, http.StatusUnprocessableEntity, ErrTransition.Error())
		return
	}
	wo, ok := h.loadWO(w, r)
	if !ok {
		return
	}
	if after, ok := h.transition(w, r, wo, WOChange{To: body.To, Note: note}, []ActorKind{ActorOfficer, ActorAssignee}); ok {
		common.WriteJSON(w, http.StatusOK, after, nil)
	}
}

// CloseWorkOrder: facility_officer or the reporter closes a finished WO.
func (h *Handler) CloseWorkOrder(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Note string `json:"note"`
	}
	if !common.Decode(w, r, &body) {
		return
	}
	wo, ok := h.loadWO(w, r)
	if !ok {
		return
	}
	if after, ok := h.transition(w, r, wo, WOChange{To: WODitutup, Note: nilIfEmpty(body.Note)}, []ActorKind{ActorOfficer, ActorReporter}); ok {
		common.WriteJSON(w, http.StatusOK, after, nil)
	}
}

func (h *Handler) CancelWorkOrder(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Reason string `json:"reason"`
	}
	if !common.Decode(w, r, &body) {
		return
	}
	reason := nilIfEmpty(body.Reason)
	if reason == nil {
		common.WriteError(w, http.StatusUnprocessableEntity, "Alasan pembatalan wajib diisi")
		return
	}
	wo, ok := h.loadWO(w, r)
	if !ok {
		return
	}
	if after, ok := h.transition(w, r, wo, WOChange{To: WODibatalkan, Note: reason, CancelReason: reason}, []ActorKind{ActorOfficer}); ok {
		common.WriteJSON(w, http.StatusOK, after, nil)
	}
}

func (h *Handler) costAllowed(c *common.Claims) func(WorkOrder) bool {
	return func(wo WorkOrder) bool {
		rel := relations(c, wo)
		return slices.Contains(rel, ActorOfficer) || slices.Contains(rel, ActorAssignee)
	}
}

// AddCost records a maintenance cost line (FR-118-004).
func (h *Handler) AddCost(w http.ResponseWriter, r *http.Request) {
	c := common.FromCtx(r)
	var in Cost
	if !common.Decode(w, r, &in) {
		return
	}
	in.Description = strings.TrimSpace(in.Description)
	if !slices.Contains(CostKinds, in.Kind) || in.Description == "" || in.AmountIDR < 0 {
		common.WriteError(w, http.StatusUnprocessableEntity, "Jenis, keterangan, dan nominal biaya wajib valid")
		return
	}
	id := chi.URLParam(r, "id")
	if !isUUID(id) {
		common.WriteError(w, http.StatusNotFound, "Perintah kerja tidak ditemukan")
		return
	}
	cid, err := h.Store.AddCost(r.Context(), common.EffectiveSchoolID(r), id, in, h.costAllowed(c), actorOf(c))
	if err != nil {
		writeStoreErr(w, err, "Perintah kerja")
		return
	}
	common.WriteJSON(w, http.StatusCreated, map[string]string{"id": cid}, nil)
}

func (h *Handler) DeleteCost(w http.ResponseWriter, r *http.Request) {
	c := common.FromCtx(r)
	id, cid := chi.URLParam(r, "id"), chi.URLParam(r, "costId")
	if !isUUID(id) || !isUUID(cid) {
		common.WriteError(w, http.StatusNotFound, "Biaya tidak ditemukan")
		return
	}
	if err := h.Store.DeleteCost(r.Context(), common.EffectiveSchoolID(r), id, cid, h.costAllowed(c), actorOf(c)); err != nil {
		writeStoreErr(w, err, "Biaya")
		return
	}
	common.WriteJSON(w, http.StatusOK, map[string]string{"id": cid}, nil)
}

// PurchaseRequest creates a purchase request through the T15 hook and puts
// the WO on menunggu_material (FR-118-004).
func (h *Handler) PurchaseRequest(w http.ResponseWriter, r *http.Request) {
	c := common.FromCtx(r)
	var body struct {
		Lines []WOPurchaseLine `json:"lines"`
	}
	if !common.Decode(w, r, &body) {
		return
	}
	schoolID := common.EffectiveSchoolID(r)
	enabled := false
	if h.Purchase != nil {
		on, err := h.Client.FeatureEnabled(r.Context(), schoolID, "procurement")
		enabled = err == nil && on
	}
	if !enabled {
		common.WriteError(w, http.StatusConflict, "Modul pengadaan belum aktif")
		return
	}
	if len(body.Lines) == 0 {
		common.WriteError(w, http.StatusUnprocessableEntity, "Minimal satu baris barang")
		return
	}
	for _, l := range body.Lines {
		if strings.TrimSpace(l.Description) == "" || l.Qty <= 0 || l.EstAmountIDR < 0 {
			common.WriteError(w, http.StatusUnprocessableEntity, "Keterangan, jumlah, dan estimasi harga wajib valid")
			return
		}
	}
	wo, ok := h.loadWO(w, r)
	if !ok {
		return
	}
	if !CanTransition(wo.Status, WOMenungguMaterial, relations(c, wo)...) {
		common.WriteError(w, http.StatusUnprocessableEntity, ErrTransition.Error())
		return
	}
	ref, err := h.Purchase.CreateFromWorkOrder(r.Context(), WOPurchaseInput{SchoolID: schoolID, RequesterUserID: c.UserID,
		WorkOrderID: wo.ID, WorkOrderCode: wo.Code, Lines: body.Lines})
	if err != nil {
		writeStoreErr(w, err, "Permintaan pembelian")
		return
	}
	if after, ok := h.transition(w, r, wo, WOChange{To: WOMenungguMaterial, PurchaseRef: &ref,
		Note: nilIfEmpty("Permintaan pembelian " + ref)}, []ActorKind{ActorOfficer}); ok {
		common.WriteJSON(w, http.StatusOK, after, nil)
	}
}

// ---------------------------------------------------------------------------
// Maintenance schedules (FR-118-003)
// ---------------------------------------------------------------------------

func (h *Handler) ListSchedules(w http.ResponseWriter, r *http.Request) {
	list, err := h.Store.ListSchedules(r.Context(), common.EffectiveSchoolID(r))
	if err != nil {
		writeStoreErr(w, err, "Jadwal perawatan")
		return
	}
	common.WriteJSON(w, http.StatusOK, list, nil)
}

func (h *Handler) SaveSchedule(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	var m Schedule
	if !common.Decode(w, r, &m) {
		return
	}
	if id != "" && !isUUID(id) {
		common.WriteError(w, http.StatusNotFound, "Jadwal perawatan tidak ditemukan")
		return
	}
	schoolID := common.EffectiveSchoolID(r)
	m.Title = strings.TrimSpace(m.Title)
	if m.Priority == "" {
		m.Priority = "sedang"
	}
	if _, err := time.Parse("2006-01-02", m.NextDueDate); err != nil || m.Title == "" || m.IntervalCount <= 0 ||
		!slices.Contains(IntervalUnits, m.IntervalUnit) || !slices.Contains(Priorities, m.Priority) {
		common.WriteError(w, http.StatusUnprocessableEntity, "Judul, interval, prioritas, dan tanggal berikutnya wajib valid")
		return
	}
	roomID, err := h.resolveTarget(r.Context(), schoolID, m.RoomID, m.AssetID)
	if err != nil {
		writeStoreErr(w, err, "Ruang")
		return
	}
	m.RoomID = roomID
	if m.AssetID != nil && *m.AssetID == "" {
		m.AssetID = nil
	}
	m.DefaultAssigneeName = nil
	if m.DefaultAssigneeUserID != nil && *m.DefaultAssigneeUserID != "" {
		tech, err := h.findTechnician(r.Context(), schoolID, *m.DefaultAssigneeUserID)
		if err != nil {
			writeStoreErr(w, err, "Teknisi")
			return
		}
		m.DefaultAssigneeName = &tech.Name
	} else {
		m.DefaultAssigneeUserID = nil
	}
	newID, err := h.Store.SaveSchedule(r.Context(), schoolID, id, m, actorOf(common.FromCtx(r)))
	if err != nil {
		writeStoreErr(w, err, "Jadwal perawatan")
		return
	}
	status := http.StatusOK
	if id == "" {
		status = http.StatusCreated
	}
	common.WriteJSON(w, status, map[string]string{"id": newID}, nil)
}

func (h *Handler) DeleteSchedule(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if !isUUID(id) {
		common.WriteError(w, http.StatusNotFound, "Jadwal perawatan tidak ditemukan")
		return
	}
	if err := h.Store.DeleteSchedule(r.Context(), common.EffectiveSchoolID(r), id, actorOf(common.FromCtx(r))); err != nil {
		writeStoreErr(w, err, "Jadwal perawatan")
		return
	}
	common.WriteJSON(w, http.StatusOK, map[string]string{"id": id}, nil)
}

// RunPreventive is the daily preventive-maintenance job (AC-118-01).
func (h *Handler) RunPreventive(ctx context.Context) error {
	today := time.Now().In(h.Store.Loc).Format("2006-01-02")
	gen, err := h.Store.RunDueSchedules(ctx, today)
	for _, g := range gen {
		if g.AssigneeUserID != nil {
			h.notify(ctx, client.Notification{SchoolID: g.SchoolID, UserID: *g.AssigneeUserID, Type: "facility_work_order",
				Title: "Perawatan berkala: " + g.Title, Body: g.Title, Data: map[string]any{"work_order_id": g.WorkOrderID}},
				"wo:"+g.WorkOrderID+":assigned:"+*g.AssigneeUserID)
		}
	}
	return err
}
