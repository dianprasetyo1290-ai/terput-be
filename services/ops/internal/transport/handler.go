package transport

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"

	"skci/ops/internal/client"
	"skci/ops/internal/common"
)

var (
	Officers        = []string{common.RoleTransportOfficer, common.RoleAdmin, common.RoleSuperadmin}
	CrewRoles       = []string{common.RolePendamping, common.RolePengemudi, common.RoleTransportOfficer, common.RoleAdmin, common.RoleSuperadmin}
	FinanceOfficers = []string{common.RoleFinanceManager, common.RoleAdmin, common.RoleSuperadmin}
	Parents         = []string{common.RoleOrangTua}
)

type Handler struct {
	Store      *Store
	Client     *client.Client
	JobRunner  *JobRunner
	HRURL      string
	BillingURL string
}

func NewHandler(s *Store, c *client.Client, j *JobRunner, hrURL, billingURL string) *Handler {
	return &Handler{
		Store:      s,
		Client:     c,
		JobRunner:  j,
		HRURL:      hrURL,
		BillingURL: billingURL,
	}
}

func claimsUserID(r *http.Request) string {
	if c := common.FromCtx(r); c != nil {
		return c.UserID
	}
	return ""
}

// ---------------------------------------------------------------------
// Settings
// ---------------------------------------------------------------------

func (h *Handler) GetSettings(w http.ResponseWriter, r *http.Request) {
	schoolID := common.EffectiveSchoolID(r)
	st, err := h.Store.GetSettings(r.Context(), schoolID)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	common.WriteJSON(w, http.StatusOK, st, nil)
}

func (h *Handler) UpdateSettings(w http.ResponseWriter, r *http.Request) {
	schoolID := common.EffectiveSchoolID(r)
	var in Settings
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		common.WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	in.SchoolID = schoolID
	if err := h.Store.UpdateSettings(r.Context(), in); err != nil {
		common.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	st, _ := h.Store.GetSettings(r.Context(), schoolID)
	common.WriteJSON(w, http.StatusOK, st, nil)
}

// ---------------------------------------------------------------------
// Zones
// ---------------------------------------------------------------------

func (h *Handler) ListZones(w http.ResponseWriter, r *http.Request) {
	schoolID := common.EffectiveSchoolID(r)
	list, err := h.Store.ListZones(r.Context(), schoolID)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	common.WriteJSON(w, http.StatusOK, list, nil)
}

func (h *Handler) CreateZone(w http.ResponseWriter, r *http.Request) {
	schoolID := common.EffectiveSchoolID(r)
	var in struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil || in.Name == "" {
		common.WriteError(w, http.StatusBadRequest, "nama zona wajib diisi")
		return
	}
	z, err := h.Store.CreateZone(r.Context(), schoolID, in.Name)
	if err != nil {
		common.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	common.WriteJSON(w, http.StatusCreated, z, nil)
}

func (h *Handler) UpdateZone(w http.ResponseWriter, r *http.Request) {
	schoolID := common.EffectiveSchoolID(r)
	id := chi.URLParam(r, "id")
	var in struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil || in.Name == "" {
		common.WriteError(w, http.StatusBadRequest, "nama zona wajib diisi")
		return
	}
	z, err := h.Store.UpdateZone(r.Context(), schoolID, id, in.Name)
	if err != nil {
		common.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	common.WriteJSON(w, http.StatusOK, z, nil)
}

func (h *Handler) DeleteZone(w http.ResponseWriter, r *http.Request) {
	schoolID := common.EffectiveSchoolID(r)
	id := chi.URLParam(r, "id")
	if err := h.Store.DeleteZone(r.Context(), schoolID, id); err != nil {
		common.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	common.WriteJSON(w, http.StatusOK, map[string]string{"status": "deleted"}, nil)
}

// ---------------------------------------------------------------------
// Routes & Stops
// ---------------------------------------------------------------------

func (h *Handler) ListRoutes(w http.ResponseWriter, r *http.Request) {
	schoolID := common.EffectiveSchoolID(r)
	list, err := h.Store.ListRoutes(r.Context(), schoolID)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	common.WriteJSON(w, http.StatusOK, list, nil)
}

func (h *Handler) GetRoute(w http.ResponseWriter, r *http.Request) {
	schoolID := common.EffectiveSchoolID(r)
	id := chi.URLParam(r, "id")
	rt, err := h.Store.GetRoute(r.Context(), schoolID, id)
	if err != nil {
		common.WriteError(w, http.StatusNotFound, "rute tidak ditemukan")
		return
	}
	common.WriteJSON(w, http.StatusOK, rt, nil)
}

func (h *Handler) CreateRoute(w http.ResponseWriter, r *http.Request) {
	schoolID := common.EffectiveSchoolID(r)
	var in struct {
		Code string `json:"code"`
		Name string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil || in.Code == "" || in.Name == "" {
		common.WriteError(w, http.StatusBadRequest, "kode dan nama rute wajib diisi")
		return
	}
	rt, err := h.Store.CreateRoute(r.Context(), schoolID, in.Code, in.Name)
	if err != nil {
		common.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	common.WriteJSON(w, http.StatusCreated, rt, nil)
}

func (h *Handler) UpdateRoute(w http.ResponseWriter, r *http.Request) {
	schoolID := common.EffectiveSchoolID(r)
	id := chi.URLParam(r, "id")
	var in struct {
		Code   string `json:"code"`
		Name   string `json:"name"`
		Status string `json:"status"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil || in.Code == "" || in.Name == "" {
		common.WriteError(w, http.StatusBadRequest, "kode dan nama rute wajib diisi")
		return
	}
	if in.Status == "" {
		in.Status = "active"
	}
	rt, err := h.Store.UpdateRoute(r.Context(), schoolID, id, in.Code, in.Name, in.Status)
	if err != nil {
		common.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	common.WriteJSON(w, http.StatusOK, rt, nil)
}

func (h *Handler) DeleteRoute(w http.ResponseWriter, r *http.Request) {
	schoolID := common.EffectiveSchoolID(r)
	id := chi.URLParam(r, "id")
	if err := h.Store.DeleteRoute(r.Context(), schoolID, id); err != nil {
		common.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	common.WriteJSON(w, http.StatusOK, map[string]string{"status": "deleted"}, nil)
}

func (h *Handler) SetStops(w http.ResponseWriter, r *http.Request) {
	schoolID := common.EffectiveSchoolID(r)
	id := chi.URLParam(r, "id")
	var in struct {
		Stops []Stop `json:"stops"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		common.WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if err := h.Store.SetStops(r.Context(), schoolID, id, in.Stops); err != nil {
		common.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	stops, _ := h.Store.ListStops(r.Context(), schoolID, id)
	common.WriteJSON(w, http.StatusOK, stops, nil)
}

// GetRouteRoster returns stops and students allocated for route on date (AC-119-01, FR-119-002).
func (h *Handler) GetRouteRoster(w http.ResponseWriter, r *http.Request) {
	schoolID := common.EffectiveSchoolID(r)
	id := chi.URLParam(r, "id")

	dateStr := r.URL.Query().Get("date")
	targetDate := time.Now().In(h.Store.loc)
	if dateStr != "" {
		if parsed, err := time.Parse("2006-01-02", dateStr); err == nil {
			targetDate = parsed
		}
	}

	stops, err := h.Store.ListStops(r.Context(), schoolID, id)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}

	allocs, err := h.Store.ListAllocations(r.Context(), schoolID, id, "")
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}

	roster := StudentsByStop(stops, allocs, targetDate)
	common.WriteJSON(w, http.StatusOK, roster, nil)
}

// ---------------------------------------------------------------------
// Vehicles & Documents
// ---------------------------------------------------------------------

func (h *Handler) ListVehicles(w http.ResponseWriter, r *http.Request) {
	schoolID := common.EffectiveSchoolID(r)
	list, err := h.Store.ListVehicles(r.Context(), schoolID)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	common.WriteJSON(w, http.StatusOK, list, nil)
}

func (h *Handler) CreateVehicle(w http.ResponseWriter, r *http.Request) {
	schoolID := common.EffectiveSchoolID(r)
	var in Vehicle
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil || in.PlateNo == "" || in.Capacity <= 0 {
		common.WriteError(w, http.StatusBadRequest, "plat nomor dan kapasitas wajib valid")
		return
	}
	in.SchoolID = schoolID
	if in.Status == "" {
		in.Status = "active"
	}
	v, err := h.Store.CreateVehicle(r.Context(), in)
	if err != nil {
		common.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	common.WriteJSON(w, http.StatusCreated, v, nil)
}

func (h *Handler) UpdateVehicle(w http.ResponseWriter, r *http.Request) {
	schoolID := common.EffectiveSchoolID(r)
	id := chi.URLParam(r, "id")
	var in Vehicle
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil || in.PlateNo == "" || in.Capacity <= 0 {
		common.WriteError(w, http.StatusBadRequest, "plat nomor dan kapasitas wajib valid")
		return
	}
	in.SchoolID = schoolID
	in.ID = id
	v, err := h.Store.UpdateVehicle(r.Context(), in)
	if err != nil {
		common.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	common.WriteJSON(w, http.StatusOK, v, nil)
}

func (h *Handler) DeleteVehicle(w http.ResponseWriter, r *http.Request) {
	schoolID := common.EffectiveSchoolID(r)
	id := chi.URLParam(r, "id")
	if err := h.Store.DeleteVehicle(r.Context(), schoolID, id); err != nil {
		common.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	common.WriteJSON(w, http.StatusOK, map[string]string{"status": "deleted"}, nil)
}

func (h *Handler) ListDocuments(w http.ResponseWriter, r *http.Request) {
	schoolID := common.EffectiveSchoolID(r)
	vehicleID := chi.URLParam(r, "id")
	list, err := h.Store.ListDocuments(r.Context(), schoolID, vehicleID)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	common.WriteJSON(w, http.StatusOK, list, nil)
}

func (h *Handler) AddDocument(w http.ResponseWriter, r *http.Request) {
	schoolID := common.EffectiveSchoolID(r)
	vehicleID := chi.URLParam(r, "id")
	var in VehicleDocument
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil || in.Kind == "" || in.Number == "" {
		common.WriteError(w, http.StatusBadRequest, "jenis dan nomor dokumen wajib diisi")
		return
	}
	in.SchoolID = schoolID
	in.VehicleID = vehicleID
	d, err := h.Store.AddDocument(r.Context(), in)
	if err != nil {
		common.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	common.WriteJSON(w, http.StatusCreated, d, nil)
}

func (h *Handler) DeleteDocument(w http.ResponseWriter, r *http.Request) {
	schoolID := common.EffectiveSchoolID(r)
	docID := chi.URLParam(r, "docId")
	if err := h.Store.DeleteDocument(r.Context(), schoolID, docID); err != nil {
		common.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	common.WriteJSON(w, http.StatusOK, map[string]string{"status": "deleted"}, nil)
}

func (h *Handler) ListExpiring(w http.ResponseWriter, r *http.Request) {
	schoolID := common.EffectiveSchoolID(r)
	days := 30
	if dStr := r.URL.Query().Get("within_days"); dStr != "" {
		if parsed, err := strconv.Atoi(dStr); err == nil && parsed > 0 {
			days = parsed
		}
	}
	list, err := h.Store.ListExpiring(r.Context(), schoolID, days)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	common.WriteJSON(w, http.StatusOK, list, nil)
}

// ---------------------------------------------------------------------
// Crew & Route Assignments
// ---------------------------------------------------------------------

func (h *Handler) ListCrew(w http.ResponseWriter, r *http.Request) {
	schoolID := common.EffectiveSchoolID(r)
	list, err := h.Store.ListCrew(r.Context(), schoolID)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	common.WriteJSON(w, http.StatusOK, list, nil)
}

func (h *Handler) CreateCrew(w http.ResponseWriter, r *http.Request) {
	schoolID := common.EffectiveSchoolID(r)
	var in Crew
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil || in.Kind == "" {
		common.WriteError(w, http.StatusBadRequest, "jenis kru wajib diisi")
		return
	}
	in.SchoolID = schoolID
	c, err := h.Store.CreateCrew(r.Context(), in)
	if err != nil {
		common.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	common.WriteJSON(w, http.StatusCreated, c, nil)
}

func (h *Handler) UpdateCrew(w http.ResponseWriter, r *http.Request) {
	schoolID := common.EffectiveSchoolID(r)
	id := chi.URLParam(r, "id")
	var in Crew
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil || in.Kind == "" {
		common.WriteError(w, http.StatusBadRequest, "jenis kru wajib diisi")
		return
	}
	in.SchoolID = schoolID
	in.ID = id
	c, err := h.Store.UpdateCrew(r.Context(), in)
	if err != nil {
		common.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	common.WriteJSON(w, http.StatusOK, c, nil)
}

func (h *Handler) DeleteCrew(w http.ResponseWriter, r *http.Request) {
	schoolID := common.EffectiveSchoolID(r)
	id := chi.URLParam(r, "id")
	if err := h.Store.DeleteCrew(r.Context(), schoolID, id); err != nil {
		common.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	common.WriteJSON(w, http.StatusOK, map[string]string{"status": "deleted"}, nil)
}

func (h *Handler) AssignRoute(w http.ResponseWriter, r *http.Request) {
	schoolID := common.EffectiveSchoolID(r)
	routeID := chi.URLParam(r, "id")
	var in struct {
		VehicleID       string     `json:"vehicle_id"`
		DriverCrewID    string     `json:"driver_crew_id"`
		AttendantCrewID *string    `json:"attendant_crew_id"`
		ValidFrom       time.Time  `json:"valid_from"`
		ValidTo         *time.Time `json:"valid_to"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil || in.VehicleID == "" || in.DriverCrewID == "" {
		common.WriteError(w, http.StatusBadRequest, "kendaraan dan pengemudi wajib dipilih")
		return
	}
	if in.ValidFrom.IsZero() {
		in.ValidFrom = time.Now().In(h.Store.loc)
	}

	res, err := h.Store.AssignRoute(r.Context(), RouteAssignment{
		SchoolID:        schoolID,
		RouteID:         routeID,
		VehicleID:       in.VehicleID,
		DriverCrewID:    in.DriverCrewID,
		AttendantCrewID: in.AttendantCrewID,
		ValidFrom:       in.ValidFrom,
		ValidTo:         in.ValidTo,
	})
	if err != nil {
		common.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	common.WriteJSON(w, http.StatusCreated, res, nil)
}

func (h *Handler) ListEmployees(w http.ResponseWriter, r *http.Request) {
	schoolID := common.EffectiveSchoolID(r)
	q := r.URL.Query().Get("q")
	if h.Client != nil && h.HRURL != "" {
		emps, err := h.Client.Employees(r.Context(), h.HRURL, schoolID, q)
		if err == nil {
			common.WriteJSON(w, http.StatusOK, emps, nil)
			return
		}
	}
	common.WriteJSON(w, http.StatusOK, []any{}, nil)
}

// ---------------------------------------------------------------------
// Fares
// ---------------------------------------------------------------------

func (h *Handler) ListFares(w http.ResponseWriter, r *http.Request) {
	schoolID := common.EffectiveSchoolID(r)
	list, err := h.Store.ListFares(r.Context(), schoolID)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	common.WriteJSON(w, http.StatusOK, list, nil)
}

func (h *Handler) CreateFare(w http.ResponseWriter, r *http.Request) {
	schoolID := common.EffectiveSchoolID(r)
	var in Fare
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil || in.MonthlyAmountIDR < 0 {
		common.WriteError(w, http.StatusBadRequest, "nominal tarif per bulan wajib valid")
		return
	}
	if (in.ZoneID == nil && in.RouteID == nil) || (in.ZoneID != nil && in.RouteID != nil) {
		common.WriteError(w, http.StatusBadRequest, "pilih salah satu: tarif zona ATAU tarif rute")
		return
	}
	if in.EffectiveFrom.IsZero() {
		in.EffectiveFrom = time.Now().In(h.Store.loc)
	}
	in.SchoolID = schoolID
	f, err := h.Store.CreateFare(r.Context(), in)
	if err != nil {
		common.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	common.WriteJSON(w, http.StatusCreated, f, nil)
}

func (h *Handler) DeleteFare(w http.ResponseWriter, r *http.Request) {
	schoolID := common.EffectiveSchoolID(r)
	id := chi.URLParam(r, "id")
	if err := h.Store.DeleteFare(r.Context(), schoolID, id); err != nil {
		common.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	common.WriteJSON(w, http.StatusOK, map[string]string{"status": "deleted"}, nil)
}

// ---------------------------------------------------------------------
// Allocations (AC-121-01)
// ---------------------------------------------------------------------

func (h *Handler) ListAllocations(w http.ResponseWriter, r *http.Request) {
	schoolID := common.EffectiveSchoolID(r)
	routeID := r.URL.Query().Get("route_id")
	studentID := r.URL.Query().Get("student_id")
	list, err := h.Store.ListAllocations(r.Context(), schoolID, routeID, studentID)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	common.WriteJSON(w, http.StatusOK, list, nil)
}

func (h *Handler) CreateAllocations(w http.ResponseWriter, r *http.Request) {
	schoolID := common.EffectiveSchoolID(r)
	userID := claimsUserID(r)
	var in struct {
		StudentUserIDs []string  `json:"student_user_ids"`
		RouteID        string    `json:"route_id"`
		StopID         string    `json:"stop_id"`
		StartDate      time.Time `json:"start_date"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil || len(in.StudentUserIDs) == 0 || in.RouteID == "" || in.StopID == "" {
		common.WriteError(w, http.StatusBadRequest, "pilih siswa, rute, dan halte penjemputan")
		return
	}
	if in.StartDate.IsZero() {
		in.StartDate = time.Now().In(h.Store.loc)
	}

	err := h.Store.CreateAllocations(r.Context(), schoolID, in.StudentUserIDs, in.RouteID, in.StopID, in.StartDate, userID)
	if err != nil {
		common.WriteError(w, http.StatusUnprocessableEntity, err.Error())
		return
	}

	// Trigger adjust on billing if already issued
	now := time.Now().In(h.Store.loc)
	for _, sid := range in.StudentUserIDs {
		_ = h.JobRunner.AdjustChargeOnAllocationChange(r.Context(), schoolID, sid, now.Year(), int(now.Month()))
	}

	common.WriteJSON(w, http.StatusCreated, map[string]any{"status": "allocated", "count": len(in.StudentUserIDs)}, nil)
}

func (h *Handler) EndAllocation(w http.ResponseWriter, r *http.Request) {
	schoolID := common.EffectiveSchoolID(r)
	id := chi.URLParam(r, "id")
	var in struct {
		EndDate time.Time `json:"end_date"`
		Reason  string    `json:"reason"`
	}
	_ = json.NewDecoder(r.Body).Decode(&in)
	if in.EndDate.IsZero() {
		in.EndDate = time.Now().In(h.Store.loc)
	}
	if in.Reason == "" {
		in.Reason = "Berhenti antar-jemput"
	}

	if err := h.Store.EndAllocation(r.Context(), schoolID, id, in.EndDate, in.Reason); err != nil {
		common.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	common.WriteJSON(w, http.StatusOK, map[string]string{"status": "ended"}, nil)
}

func (h *Handler) MoveAllocation(w http.ResponseWriter, r *http.Request) {
	schoolID := common.EffectiveSchoolID(r)
	id := chi.URLParam(r, "id")
	userID := claimsUserID(r)
	var in struct {
		RouteID       string    `json:"route_id"`
		StopID        string    `json:"stop_id"`
		EffectiveDate time.Time `json:"effective_date"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil || in.RouteID == "" || in.StopID == "" {
		common.WriteError(w, http.StatusBadRequest, "rute dan halte baru wajib dipilih")
		return
	}
	if in.EffectiveDate.IsZero() {
		in.EffectiveDate = time.Now().In(h.Store.loc)
	}

	if err := h.Store.MoveAllocation(r.Context(), schoolID, id, in.RouteID, in.StopID, in.EffectiveDate, userID); err != nil {
		common.WriteError(w, http.StatusUnprocessableEntity, err.Error())
		return
	}
	common.WriteJSON(w, http.StatusOK, map[string]string{"status": "moved"}, nil)
}

// ---------------------------------------------------------------------
// Trips & Attendance (AC-121-02, FR-121-002)
// ---------------------------------------------------------------------

func (h *Handler) ListTrips(w http.ResponseWriter, r *http.Request) {
	schoolID := common.EffectiveSchoolID(r)
	dateStr := r.URL.Query().Get("date")
	targetDate := time.Now().In(h.Store.loc)
	if dateStr != "" {
		if parsed, err := time.Parse("2006-01-02", dateStr); err == nil {
			targetDate = parsed
		}
	}
	routeID := r.URL.Query().Get("route_id")
	trips, err := h.Store.GetOrCreateDailyTrips(r.Context(), schoolID, targetDate)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if routeID != "" {
		var filtered []Trip
		for _, t := range trips {
			if t.RouteID == routeID {
				filtered = append(filtered, t)
			}
		}
		trips = filtered
	}
	common.WriteJSON(w, http.StatusOK, trips, nil)
}

func (h *Handler) GetMyTrips(w http.ResponseWriter, r *http.Request) {
	schoolID := common.EffectiveSchoolID(r)
	userID := claimsUserID(r)
	dateStr := r.URL.Query().Get("date")
	targetDate := time.Now().In(h.Store.loc)
	if dateStr != "" {
		if parsed, err := time.Parse("2006-01-02", dateStr); err == nil {
			targetDate = parsed
		}
	}

	trips, err := h.Store.GetOrCreateDailyTrips(r.Context(), schoolID, targetDate)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}

	// Filter trips where user is driver or attendant
	var myTrips []Trip
	for _, t := range trips {
		var isMyTrip bool
		_ = h.Store.db.QueryRowContext(r.Context(), `
			SELECT true
			FROM transport_crew
			WHERE id IN ($1, $2) AND user_id = $3
		`, t.DriverCrewID, t.AttendantCrewID, userID).Scan(&isMyTrip)
		if isMyTrip {
			myTrips = append(myTrips, t)
		}
	}
	common.WriteJSON(w, http.StatusOK, myTrips, nil)
}

func (h *Handler) GetTripManifest(w http.ResponseWriter, r *http.Request) {
	schoolID := common.EffectiveSchoolID(r)
	tripID := chi.URLParam(r, "id")
	manifest, err := h.Store.GetTripManifest(r.Context(), schoolID, tripID)
	if err != nil {
		common.WriteError(w, http.StatusNotFound, err.Error())
		return
	}
	common.WriteJSON(w, http.StatusOK, manifest, nil)
}

func (h *Handler) RecordTripEvent(w http.ResponseWriter, r *http.Request) {
	schoolID := common.EffectiveSchoolID(r)
	tripID := chi.URLParam(r, "id")
	userID := claimsUserID(r)

	var in struct {
		StudentUserID string  `json:"student_user_id"`
		Code          string  `json:"code"`
		StopID        *string `json:"stop_id"`
		Event         string  `json:"event"` // 'board' | 'alight' | 'absent'
		ClientEventID string  `json:"client_event_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		common.WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	// Resolve student card QR code if code is supplied (T12 OQ-30)
	if in.StudentUserID == "" && in.Code != "" {
		if h.Client != nil && h.BillingURL != "" {
			sid, cardErr := h.Client.ResolveStudentCard(r.Context(), h.BillingURL, schoolID, in.Code)
			if cardErr != nil {
				common.WriteError(w, http.StatusUnprocessableEntity, cardErr.Error())
				return
			}
			in.StudentUserID = sid
		} else {
			common.WriteError(w, http.StatusBadRequest, "resolusi kartu tidak tersedia")
			return
		}
	}

	if in.StudentUserID == "" || in.Event == "" || in.ClientEventID == "" {
		common.WriteError(w, http.StatusBadRequest, "student_user_id, event, dan client_event_id wajib ada")
		return
	}

	ev, err := h.Store.RecordTripEvent(r.Context(), TripEvent{
		SchoolID:      schoolID,
		TripID:        tripID,
		StudentUserID: in.StudentUserID,
		StopID:        in.StopID,
		Event:         in.Event,
		Source:        "scan",
		RecordedBy:    userID,
		RecordedAt:    time.Now().In(h.Store.loc),
		ClientEventID: in.ClientEventID,
	})
	if err != nil {
		common.WriteError(w, http.StatusUnprocessableEntity, err.Error())
		return
	}

	// Async parent notification (AC-121-02, FR-121-003)
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
		defer cancel()

		st, stErr := h.Store.GetSettings(ctx, schoolID)
		if stErr == nil && st.NotifyParentOnTrip && h.Client != nil {
			guardians, gErr := h.Client.GuardiansForStudent(ctx, schoolID, in.StudentUserID)
			if gErr == nil {
				eventName := "naik ke armada antar-jemput"
				if in.Event == "alight" {
					eventName = "turun dari armada antar-jemput"
				}
				title := fmt.Sprintf("Informasi Transportasi: Siswa telah %s", eventName)
				body := fmt.Sprintf("Siswa tercatat %s pada %s.", eventName, ev.RecordedAt.Format("15:04 WIB"))
				idempKey := fmt.Sprintf("transport:%s:%s:%s", tripID, in.StudentUserID, in.Event)

				for _, g := range guardians {
					_ = h.Client.Notify(ctx, client.Notification{
						SchoolID: schoolID,
						UserID:   g.GuardianUserID,
						Type:     "transport",
						Title:    title,
						Body:     body,
						Data: map[string]any{
							"trip_id":         tripID,
							"student_user_id": in.StudentUserID,
							"event":           in.Event,
						},
					}, idempKey)
				}
			}
		}
	}()

	common.WriteJSON(w, http.StatusOK, ev, nil)
}

func (h *Handler) StartTrip(w http.ResponseWriter, r *http.Request) {
	schoolID := common.EffectiveSchoolID(r)
	tripID := chi.URLParam(r, "id")
	if err := h.Store.ChangeTripStatus(r.Context(), schoolID, tripID, "running"); err != nil {
		common.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	common.WriteJSON(w, http.StatusOK, map[string]string{"status": "running"}, nil)
}

func (h *Handler) FinishTrip(w http.ResponseWriter, r *http.Request) {
	schoolID := common.EffectiveSchoolID(r)
	tripID := chi.URLParam(r, "id")
	if err := h.Store.ChangeTripStatus(r.Context(), schoolID, tripID, "done"); err != nil {
		common.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	common.WriteJSON(w, http.StatusOK, map[string]string{"status": "done"}, nil)
}

// ---------------------------------------------------------------------
// Billing & Charges (AC-122-01)
// ---------------------------------------------------------------------

func (h *Handler) PreviewBilling(w http.ResponseWriter, r *http.Request) {
	schoolID := common.EffectiveSchoolID(r)
	year, month := currentOrParamPeriod(r)
	previews, err := h.Store.PreviewBilling(r.Context(), schoolID, year, month)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	common.WriteJSON(w, http.StatusOK, previews, nil)
}

func (h *Handler) IssueBilling(w http.ResponseWriter, r *http.Request) {
	schoolID := common.EffectiveSchoolID(r)
	var in struct {
		PeriodYear  int `json:"period_year"`
		PeriodMonth int `json:"period_month"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil || in.PeriodYear <= 0 || in.PeriodMonth < 1 || in.PeriodMonth > 12 {
		common.WriteError(w, http.StatusBadRequest, "periode tagihan wajib valid (tahun & bulan)")
		return
	}

	issued, failed, err := h.JobRunner.IssueMonthlyCharges(r.Context(), schoolID, in.PeriodYear, in.PeriodMonth)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}

	common.WriteJSON(w, http.StatusOK, map[string]any{
		"status": "completed",
		"issued": issued,
		"failed": failed,
	}, nil)
}

func (h *Handler) ListCharges(w http.ResponseWriter, r *http.Request) {
	schoolID := common.EffectiveSchoolID(r)
	year, month := currentOrParamPeriod(r)
	status := r.URL.Query().Get("status")
	list, err := h.Store.ListCharges(r.Context(), schoolID, year, month, status)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	common.WriteJSON(w, http.StatusOK, list, nil)
}

func (h *Handler) InternalRetryCharge(w http.ResponseWriter, r *http.Request) {
	schoolID := common.EffectiveSchoolID(r)
	chargeID := chi.URLParam(r, "id")
	ch, err := h.Store.GetCharge(r.Context(), schoolID, chargeID)
	if err != nil {
		common.WriteError(w, http.StatusNotFound, "tagihan tidak ditemukan")
		return
	}

	var items []ChargeItem
	_ = json.Unmarshal(ch.Items, &items)

	billID, billErr := h.JobRunner.callBillAdhoc(r.Context(), schoolID, ch.StudentUserID, ch.PeriodYear, ch.PeriodMonth, ch.Version+1, items)
	if billErr != nil {
		_ = h.Store.UpdateChargeStatus(r.Context(), schoolID, ch.ID, "failed", "", billErr.Error())
		common.WriteError(w, http.StatusBadGateway, billErr.Error())
		return
	}

	_ = h.Store.UpdateChargeStatus(r.Context(), schoolID, ch.ID, "issued", billID, "")
	common.WriteJSON(w, http.StatusOK, map[string]string{"status": "retried", "bill_id": billID}, nil)
}

// ---------------------------------------------------------------------
// Parent Portal (/me/transport)
// ---------------------------------------------------------------------

func (h *Handler) MyChildren(w http.ResponseWriter, r *http.Request) {
	schoolID := common.EffectiveSchoolID(r)
	parentUserID := claimsUserID(r)

	// In real identity, we look up guardian's children. For our endpoint:
	// We can list all active allocations in school that match children of parentUserID
	// Let's query student allocations for students accessible to parent.
	allocs, err := h.Store.ListAllocations(r.Context(), schoolID, "", "")
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}

	// Filter by parent access if identity client is available
	var childAllocs []Allocation
	for _, a := range allocs {
		if h.Client != nil {
			hasAccess, _ := h.Client.CheckGuardianStudentAccess(r.Context(), parentUserID, a.StudentUserID)
			if !hasAccess {
				continue
			}
		}
		childAllocs = append(childAllocs, a)
	}

	common.WriteJSON(w, http.StatusOK, childAllocs, nil)
}

func (h *Handler) MyChildEvents(w http.ResponseWriter, r *http.Request) {
	schoolID := common.EffectiveSchoolID(r)
	studentID := chi.URLParam(r, "studentId")
	parentUserID := claimsUserID(r)

	if h.Client != nil {
		hasAccess, _ := h.Client.CheckGuardianStudentAccess(r.Context(), parentUserID, studentID)
		if !hasAccess {
			common.WriteError(w, http.StatusForbidden, "akses ditolak: bukan wali siswa ini")
			return
		}
	}

	rows, err := h.Store.db.QueryContext(r.Context(), `
		SELECT e.id, e.trip_id, e.event, e.recorded_at, t.direction, t.trip_date
		FROM transport_trip_events e
		JOIN transport_trips t ON t.id = e.trip_id
		WHERE e.school_id = $1 AND e.student_user_id = $2
		ORDER BY e.recorded_at DESC
		LIMIT 30
	`, schoolID, studentID)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer rows.Close()

	type eventLog struct {
		ID         string    `json:"id"`
		TripID     string    `json:"trip_id"`
		Event      string    `json:"event"`
		RecordedAt time.Time `json:"recorded_at"`
		Direction  string    `json:"direction"`
		TripDate   time.Time `json:"trip_date"`
	}
	var logs []eventLog
	for rows.Next() {
		var l eventLog
		if err := rows.Scan(&l.ID, &l.TripID, &l.Event, &l.RecordedAt, &l.Direction, &l.TripDate); err == nil {
			logs = append(logs, l)
		}
	}
	common.WriteJSON(w, http.StatusOK, logs, nil)
}

func (h *Handler) CreateMyAbsence(w http.ResponseWriter, r *http.Request) {
	schoolID := common.EffectiveSchoolID(r)
	parentUserID := claimsUserID(r)

	var in struct {
		StudentUserID string    `json:"student_user_id"`
		NoticeDate    time.Time `json:"notice_date"`
		Direction     string    `json:"direction"` // 'jemput' | 'antar' | 'both'
		Reason        string    `json:"reason"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil || in.StudentUserID == "" || in.Direction == "" {
		common.WriteError(w, http.StatusBadRequest, "siswa dan arah perjalanan wajib diisi")
		return
	}

	if h.Client != nil {
		hasAccess, _ := h.Client.CheckGuardianStudentAccess(r.Context(), parentUserID, in.StudentUserID)
		if !hasAccess {
			common.WriteError(w, http.StatusForbidden, "akses ditolak: bukan wali siswa ini")
			return
		}
	}

	an, err := h.Store.CreateAbsenceNotice(r.Context(), AbsenceNotice{
		SchoolID:      schoolID,
		StudentUserID: in.StudentUserID,
		NoticeDate:    in.NoticeDate,
		Direction:     in.Direction,
		Reason:        &in.Reason,
		CreatedBy:     parentUserID,
	})
	if err != nil {
		common.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	common.WriteJSON(w, http.StatusCreated, an, nil)
}

func (h *Handler) CancelMyAbsence(w http.ResponseWriter, r *http.Request) {
	schoolID := common.EffectiveSchoolID(r)
	id := chi.URLParam(r, "id")
	if err := h.Store.DeleteAbsenceNotice(r.Context(), schoolID, id); err != nil {
		common.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	common.WriteJSON(w, http.StatusOK, map[string]string{"status": "cancelled"}, nil)
}

func currentOrParamPeriod(r *http.Request) (int, int) {
	now := time.Now()
	year := now.Year()
	month := int(now.Month())

	if yStr := r.URL.Query().Get("period_year"); yStr != "" {
		if y, err := strconv.Atoi(yStr); err == nil && y > 2000 {
			year = y
		}
	}
	if mStr := r.URL.Query().Get("period_month"); mStr != "" {
		if m, err := strconv.Atoi(mStr); err == nil && m >= 1 && m <= 12 {
			month = m
		}
	}
	return year, month
}
