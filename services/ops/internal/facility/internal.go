package facility

import (
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"skci/ops/internal/common"
)

// Internal contract (T9 §4.2), X-Internal-Key only. school_id is mandatory
// on every call (query for GET, body for POST); a row of another school is
// answered as 404 / left out.

// FeatureCheck reports whether the facility feature is on for a school; the
// router wires it to identity so internal mutations obey the module switch.
type FeatureCheck func(r *http.Request, schoolID string) bool

func internalSchool(w http.ResponseWriter, r *http.Request) (string, bool) {
	s := strings.TrimSpace(r.URL.Query().Get("school_id"))
	if !isUUID(s) {
		common.WriteError(w, http.StatusBadRequest, "school_id wajib diisi")
		return "", false
	}
	return s, true
}

type internalFloor struct {
	ID    string `json:"id"`
	Level int    `json:"level"`
	Name  string `json:"name"`
}

type internalBuilding struct {
	ID     string          `json:"id"`
	Code   string          `json:"code"`
	Name   string          `json:"name"`
	Floors []internalFloor `json:"floors"`
}

type internalFacility struct {
	Name string `json:"name"`
	Qty  int    `json:"qty"`
}

type internalRoom struct {
	ID               string             `json:"id"`
	Code             string             `json:"code"`
	Name             string             `json:"name"`
	Kind             string             `json:"kind"`
	Capacity         int                `json:"capacity"`
	Usage            string             `json:"usage"`
	Status           string             `json:"status"`
	RequiresApproval bool               `json:"requires_approval"`
	Building         RefBuilding        `json:"building"`
	Floor            RefFloor           `json:"floor"`
	Facilities       []internalFacility `json:"facilities"`
}

func toInternalRoom(r Room) internalRoom {
	out := internalRoom{ID: r.ID, Code: r.Code, Name: r.Name, Kind: r.Kind, Capacity: r.Capacity, Usage: r.Usage,
		Status: r.Status, RequiresApproval: r.RequiresApproval, Building: r.Building, Floor: r.Floor,
		Facilities: []internalFacility{}}
	for _, f := range r.Facilities {
		out.Facilities = append(out.Facilities, internalFacility{Name: f.Name, Qty: f.Qty})
	}
	return out
}

func (h *Handler) InternalBuildings(w http.ResponseWriter, r *http.Request) {
	schoolID, ok := internalSchool(w, r)
	if !ok {
		return
	}
	list, err := h.Store.ListBuildings(r.Context(), schoolID)
	if err != nil {
		writeStoreErr(w, err, "Gedung")
		return
	}
	out := make([]internalBuilding, 0, len(list))
	for _, b := range list {
		ib := internalBuilding{ID: b.ID, Code: b.Code, Name: b.Name, Floors: []internalFloor{}}
		for _, f := range b.Floors {
			ib.Floors = append(ib.Floors, internalFloor{ID: f.ID, Level: f.Level, Name: f.Name})
		}
		out = append(out, ib)
	}
	common.WriteJSON(w, http.StatusOK, out, nil)
}

func (h *Handler) InternalRooms(w http.ResponseWriter, r *http.Request) {
	schoolID, ok := internalSchool(w, r)
	if !ok {
		return
	}
	q := r.URL.Query()
	ids, ok := splitIDs(q.Get("ids"))
	if !ok {
		common.WriteError(w, http.StatusBadRequest, "ids tidak valid")
		return
	}
	rooms, _, err := h.Store.ListRooms(r.Context(), schoolID, RoomFilter{Kind: q.Get("kind"), Status: q.Get("status"),
		Bookable: q.Get("bookable") == "true", IDs: ids, Q: strings.TrimSpace(q.Get("q"))})
	if err != nil {
		writeStoreErr(w, err, "Ruang")
		return
	}
	out := make([]internalRoom, 0, len(rooms))
	for _, room := range rooms {
		out = append(out, toInternalRoom(room))
	}
	common.WriteJSON(w, http.StatusOK, out, nil)
}

func (h *Handler) InternalRoom(w http.ResponseWriter, r *http.Request) {
	schoolID, ok := internalSchool(w, r)
	if !ok {
		return
	}
	id := chi.URLParam(r, "id")
	if !isUUID(id) {
		common.WriteError(w, http.StatusNotFound, "Ruang tidak ditemukan")
		return
	}
	room, err := h.Store.GetRoom(r.Context(), schoolID, id)
	if err != nil {
		writeStoreErr(w, err, "Ruang")
		return
	}
	common.WriteJSON(w, http.StatusOK, toInternalRoom(room), nil)
}

type availability struct {
	RoomID string     `json:"room_id"`
	Busy   []BusySlot `json:"busy"`
}

func (h *Handler) InternalAvailability(w http.ResponseWriter, r *http.Request) {
	schoolID, ok := internalSchool(w, r)
	if !ok {
		return
	}
	q := r.URL.Query()
	ids, ok := splitIDs(q.Get("room_ids"))
	from, err1 := parseTime(q.Get("from"))
	to, err2 := parseTime(q.Get("to"))
	if !ok || len(ids) == 0 || err1 != nil || err2 != nil || !to.After(from) || to.Sub(from) > 62*24*time.Hour {
		common.WriteError(w, http.StatusBadRequest, "room_ids, from, dan to wajib valid (maksimal 62 hari)")
		return
	}
	// Only rooms of this school are reported.
	rooms, _, err := h.Store.ListRooms(r.Context(), schoolID, RoomFilter{IDs: ids})
	if err != nil {
		writeStoreErr(w, err, "Ruang")
		return
	}
	own := make([]string, 0, len(rooms))
	for _, room := range rooms {
		own = append(own, room.ID)
	}
	busy, err := h.Store.BusyIntervals(r.Context(), h.Store.DB, schoolID, own, from, to)
	if err != nil {
		writeStoreErr(w, err, "Ketersediaan")
		return
	}
	out := make([]availability, 0, len(own))
	for _, id := range own {
		out = append(out, availability{RoomID: id, Busy: busy[id]})
	}
	common.WriteJSON(w, http.StatusOK, out, nil)
}

// InternalCreateBooking (T21): same rules as the portal booking; a repeated
// idempotency_key returns 200 with the same data.
func (h *Handler) InternalCreateBooking(check FeatureCheck) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			bookingBody
			SchoolID        string `json:"school_id"`
			RequesterUserID string `json:"requester_user_id"`
			SourceService   string `json:"source_service"`
			SourceRef       string `json:"source_ref"`
			IdempotencyKey  string `json:"idempotency_key"`
		}
		if !common.Decode(w, r, &body) {
			return
		}
		if !isUUID(body.SchoolID) || !isUUID(body.RequesterUserID) || strings.TrimSpace(body.IdempotencyKey) == "" ||
			strings.TrimSpace(body.SourceService) == "" {
			common.WriteError(w, http.StatusBadRequest, "school_id, requester_user_id, source_service, dan idempotency_key wajib diisi")
			return
		}
		if !check(r, body.SchoolID) {
			common.WriteError(w, http.StatusForbidden, "Modul Fasilitas dinonaktifkan untuk sekolah ini")
			return
		}
		period, err := body.validate()
		if err != nil {
			common.WriteError(w, http.StatusUnprocessableEntity, err.Error())
			return
		}
		b, created, err := h.Store.CreateBooking(r.Context(), BookingInput{
			SchoolID: body.SchoolID, RoomID: body.RoomID, Period: period, Purpose: strings.TrimSpace(body.Purpose),
			Participants: body.Participants, RequesterUserID: body.RequesterUserID, Source: "internal",
			SourceService: nilIfEmpty(body.SourceService), SourceRef: nilIfEmpty(body.SourceRef),
			IdempotencyKey: "internal:" + body.SourceService + ":" + body.IdempotencyKey,
		}, h.approvalOpener(body.SchoolID), Actor{UserID: body.RequesterUserID, Role: "service:" + body.SourceService})
		if err != nil {
			writeStoreErr(w, err, "Ruang")
			return
		}
		status := http.StatusOK
		if created {
			status = http.StatusCreated
		}
		common.WriteJSON(w, status, map[string]string{"id": b.ID, "status": b.Status}, nil)
	}
}

func (h *Handler) InternalCancelBooking(w http.ResponseWriter, r *http.Request) {
	var body struct {
		SchoolID    string `json:"school_id"`
		ActorUserID string `json:"actor_user_id"`
		Reason      string `json:"reason"`
	}
	if !common.Decode(w, r, &body) {
		return
	}
	id := chi.URLParam(r, "id")
	if !isUUID(body.SchoolID) || strings.TrimSpace(body.Reason) == "" {
		common.WriteError(w, http.StatusBadRequest, "school_id dan reason wajib diisi")
		return
	}
	if !isUUID(id) {
		common.WriteError(w, http.StatusNotFound, "Pemesanan tidak ditemukan")
		return
	}
	actor := Actor{Role: "service"}
	if isUUID(body.ActorUserID) {
		actor.UserID = body.ActorUserID
	}
	h.cancelBooking(w, r, body.SchoolID, id, strings.TrimSpace(body.Reason), actor)
}

// InternalCreateWorkOrder (T11 ticket → WO, source='ticket').
func (h *Handler) InternalCreateWorkOrder(check FeatureCheck) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			SchoolID       string  `json:"school_id"`
			RoomID         *string `json:"room_id"`
			AssetID        *string `json:"asset_id"`
			Title          string  `json:"title"`
			Description    string  `json:"description"`
			Priority       string  `json:"priority"`
			ReporterUserID string  `json:"reporter_user_id"`
			TicketRef      string  `json:"ticket_ref"`
			CallbackURL    string  `json:"callback_url"`
			IdempotencyKey string  `json:"idempotency_key"`
		}
		if !common.Decode(w, r, &body) {
			return
		}
		if !isUUID(body.SchoolID) || !isUUID(body.ReporterUserID) || strings.TrimSpace(body.IdempotencyKey) == "" {
			common.WriteError(w, http.StatusBadRequest, "school_id, reporter_user_id, dan idempotency_key wajib diisi")
			return
		}
		if !check(r, body.SchoolID) {
			common.WriteError(w, http.StatusForbidden, "Modul Fasilitas dinonaktifkan untuk sekolah ini")
			return
		}
		body.Title = strings.TrimSpace(body.Title)
		if body.Title == "" || !slices.Contains(Priorities, body.Priority) {
			common.WriteError(w, http.StatusUnprocessableEntity, "Judul dan prioritas wajib valid")
			return
		}
		roomID, err := h.resolveTarget(r.Context(), body.SchoolID, body.RoomID, body.AssetID)
		if err != nil {
			writeStoreErr(w, err, "Ruang")
			return
		}
		key := "ticket:" + body.IdempotencyKey
		assetID := body.AssetID
		if assetID != nil && *assetID == "" {
			assetID = nil
		}
		wo, created, err := h.Store.CreateWorkOrder(r.Context(), WOInput{
			SchoolID: body.SchoolID, Source: SourceTicket, RoomID: roomID, AssetID: assetID, Title: body.Title,
			Description: strings.TrimSpace(body.Description), Priority: body.Priority, ReporterUserID: body.ReporterUserID,
			TicketRef: nilIfEmpty(body.TicketRef), CallbackURL: nilIfEmpty(body.CallbackURL), IdempotencyKey: &key,
		}, nil, Actor{UserID: body.ReporterUserID, Role: "service:core"})
		if err != nil {
			writeStoreErr(w, err, "Ruang")
			return
		}
		status := http.StatusOK
		if created {
			status = http.StatusCreated
		}
		common.WriteJSON(w, status, map[string]string{"id": wo.ID, "code": wo.Code, "status": wo.Status}, nil)
	}
}

type internalWO struct {
	ID           string    `json:"id"`
	Code         string    `json:"code"`
	RoomID       *string   `json:"room_id"`
	AssetID      *string   `json:"asset_id"`
	Title        string    `json:"title"`
	Priority     string    `json:"priority"`
	Status       string    `json:"status"`
	AssigneeName *string   `json:"assignee_name"`
	UpdatedAt    time.Time `json:"updated_at"`
}

func (h *Handler) InternalWorkOrders(w http.ResponseWriter, r *http.Request) {
	schoolID, ok := internalSchool(w, r)
	if !ok {
		return
	}
	q := r.URL.Query()
	ids, ok := splitIDs(q.Get("room_ids"))
	if !ok {
		common.WriteError(w, http.StatusBadRequest, "room_ids tidak valid")
		return
	}
	f := WOFilter{RoomIDs: ids, TicketRef: q.Get("ticket_ref")}
	if s := q.Get("status"); s == "active" {
		f.ActiveOnly = true
	} else {
		f.Status = s
	}
	list, _, err := h.Store.ListWorkOrders(r.Context(), schoolID, f)
	if err != nil {
		writeStoreErr(w, err, "Perintah kerja")
		return
	}
	out := make([]internalWO, 0, len(list))
	for _, wo := range list {
		out = append(out, internalWO{ID: wo.ID, Code: wo.Code, RoomID: wo.RoomID, AssetID: wo.AssetID, Title: wo.Title,
			Priority: wo.Priority, Status: wo.Status, AssigneeName: wo.AssigneeName, UpdatedAt: wo.UpdatedAt})
	}
	common.WriteJSON(w, http.StatusOK, out, nil)
}
