package servicedesk

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"skci/core/internal/common"
	"skci/core/internal/store"
)

// Routes mounts the JWT endpoints (caller adds JWTMiddleware).
func (s *Service) Routes(r chi.Router) {
	// Banner lookup for menu 81 pages: independent of the feature switch,
	// like the 409 guard (a ticket outlives a disabled feature).
	r.Get("/service-desk/service-requests/{srId}/ticket", s.TicketForServiceRequest)

	r.Group(func(fr chi.Router) {
		fr.Use(s.requireFeature)

		fr.Get("/service-desk/teams", s.ListTeams)
		fr.Post("/service-desk/teams", s.SaveTeam)
		fr.Put("/service-desk/teams/{id}", s.SaveTeam)
		fr.Delete("/service-desk/teams/{id}", s.DeleteTeam)
		fr.Get("/service-desk/agents", s.ListAgents)

		fr.Get("/service-desk/categories", s.ListCategories)
		fr.Post("/service-desk/categories", s.SaveCategory)
		fr.Get("/service-desk/categories/{id}", s.GetCategory)
		fr.Put("/service-desk/categories/{id}", s.SaveCategory)
		fr.Delete("/service-desk/categories/{id}", s.DeleteCategory)

		fr.Get("/service-desk/business-hours", s.GetBusinessHours)
		fr.Put("/service-desk/business-hours", s.PutBusinessHours)
		fr.Get("/service-desk/holidays", s.ListHolidays)
		fr.Post("/service-desk/holidays", s.CreateHoliday)
		fr.Delete("/service-desk/holidays/{id}", s.DeleteHoliday)
		fr.Get("/service-desk/sla-policies", s.ListPolicies)
		fr.Post("/service-desk/sla-policies", s.SavePolicy)
		fr.Put("/service-desk/sla-policies/{id}", s.SavePolicy)
		fr.Delete("/service-desk/sla-policies/{id}", s.DeletePolicy)

		fr.Get("/service-desk/tickets", s.ListTickets)
		fr.Post("/service-desk/tickets", s.CreateAgentTicket)
		fr.Get("/service-desk/tickets/{id}", s.GetTicket)
		fr.Post("/service-desk/tickets/{id}/assign", s.AssignTicket)
		fr.Post("/service-desk/tickets/{id}/status", s.ChangeStatus)
		fr.Post("/service-desk/tickets/{id}/comments", s.AgentComment)
		fr.Post("/service-desk/tickets/{id}/work-order", s.CreateWorkOrder)
		fr.Get("/service-desk/tickets/{id}/files/*", s.AgentFile)
		fr.Get("/service-desk/rooms", s.ListRooms)
		fr.Get("/service-desk/reports", s.Report)

		fr.Get("/me/tickets/categories", s.MyCategories)
		fr.Post("/me/tickets", s.CreateMyTicket)
		fr.Get("/me/tickets", s.ListMyTickets)
		fr.Get("/me/tickets/{id}", s.GetMyTicket)
		fr.Post("/me/tickets/{id}/comments", s.MyComment)
		fr.Post("/me/tickets/{id}/reopen", s.Reopen)
		fr.Post("/me/tickets/{id}/satisfaction", s.Rate)
		fr.Get("/me/tickets/{id}/files/*", s.MyFile)
	})
}

// InternalRoutes mounts the X-Internal-Key endpoints.
func (s *Service) InternalRoutes(r chi.Router) {
	r.Post("/internal/service-desk/from-service-request/{id}", s.InternalFromServiceRequest)
}

// ---------------------------------------------------------------------------
// helpers
// ---------------------------------------------------------------------------

type ctxKey struct{}

var uuidRe = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

func isUUID(v string) bool { return uuidRe.MatchString(v) }

type caller struct {
	claims   *common.Claims
	schoolID string
	userID   string
}

func (s *Service) requireFeature(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c := common.FromCtx(r)
		schoolID := common.TenantSchoolID(r)
		if c == nil {
			common.WriteError(w, http.StatusUnauthorized, "tidak terautentikasi")
			return
		}
		if !isUUID(schoolID) {
			common.WriteError(w, http.StatusBadRequest, "school_id diperlukan")
			return
		}
		if !s.FeatureEnabled(r.Context(), schoolID) {
			common.WriteError(w, http.StatusForbidden, "Modul Service Desk belum aktif untuk sekolah ini")
			return
		}
		ctx := context.WithValue(r.Context(), ctxKey{}, caller{claims: c, schoolID: schoolID, userID: c.GetUserID()})
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func who(r *http.Request) caller {
	c, _ := r.Context().Value(ctxKey{}).(caller)
	return c
}

func isAdmin(role string) bool { return role == common.RoleAdmin || role == common.RoleSuperadmin }

func isManager(role string) bool { return role == RoleManager || isAdmin(role) }

func isDeskStaff(role string) bool { return role == RoleAgent || isManager(role) }

// allow writes 403 unless the caller's active role passes check.
func allow(w http.ResponseWriter, r *http.Request, check func(string) bool) (caller, bool) {
	c := who(r)
	if !check(c.claims.ActiveRole) {
		common.WriteError(w, http.StatusForbidden, "peran Anda tidak memiliki akses ke fitur ini")
		return c, false
	}
	return c, true
}

func writeErr(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, ErrNotFound):
		common.WriteError(w, http.StatusNotFound, "data tidak ditemukan")
	case errors.Is(err, ErrConflict):
		common.WriteError(w, http.StatusConflict, strings.TrimPrefix(err.Error(), ErrConflict.Error()+": "))
	case errors.Is(err, ErrInvalidTransition), errors.Is(err, ErrNoteRequired), errors.Is(err, ErrCannotRate),
		errors.Is(err, ErrNotFacility), errors.Is(err, ErrTicketClosed):
		common.WriteError(w, http.StatusUnprocessableEntity, err.Error())
	case errors.Is(err, ErrOpsCallFailed):
		common.WriteError(w, http.StatusBadGateway, ErrOpsCallFailed.Error())
	default:
		log.Printf("service desk: %v", err)
		common.CaptureError(err)
		common.WriteError(w, http.StatusInternalServerError, "terjadi kesalahan pada server")
	}
}

func validationErr(w http.ResponseWriter, msg string, fields map[string]string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusUnprocessableEntity)
	errs := map[string]string{"message": msg}
	for k, v := range fields {
		errs[k] = v
	}
	_ = json.NewEncoder(w).Encode(common.Envelope{Message: msg, Errors: errs})
}

func trimPtr(p *string) *string {
	if p == nil || strings.TrimSpace(*p) == "" {
		return nil
	}
	v := strings.TrimSpace(*p)
	return &v
}

// names resolves display names best-effort (identity); missing → empty.
func (s *Service) names(ctx context.Context, schoolID string, ids []string) map[string]string {
	out := map[string]string{}
	var uniq []string
	for _, id := range ids {
		if id != "" && !slices.Contains(uniq, id) {
			uniq = append(uniq, id)
		}
	}
	if len(uniq) == 0 {
		return out
	}
	users, err := s.Client.UsersInSchool(ctx, schoolID, "", uniq)
	if err != nil {
		return out
	}
	for _, u := range users {
		out[u.ID] = u.Name
	}
	return out
}

func (s *Service) decorate(ctx context.Context, schoolID string, tickets []Ticket, events []Event) {
	var ids []string
	for _, t := range tickets {
		ids = append(ids, t.ReporterUserID)
		if t.AssigneeUserID != nil {
			ids = append(ids, *t.AssigneeUserID)
		}
	}
	for _, e := range events {
		if e.ActorUserID != nil {
			ids = append(ids, *e.ActorUserID)
		}
	}
	n := s.names(ctx, schoolID, ids)
	now := s.Now()
	for i := range tickets {
		t := &tickets[i]
		t.ReporterName = n[t.ReporterUserID]
		if t.AssigneeUserID != nil {
			t.AssigneeName = n[*t.AssigneeUserID]
		}
		t.SLAState = s.liveState(t, now)
	}
	for i := range events {
		if events[i].ActorUserID != nil {
			events[i].ActorName = n[*events[i].ActorUserID]
		}
	}
}

// liveState is the badge state: stamped breaches win; otherwise an open
// ticket past its due time is breached, and an escalated one is warning.
func (s *Service) liveState(t *Ticket, now time.Time) string {
	if t.SLA == nil {
		return ""
	}
	if t.ResolutionBreachedAt != nil || t.ResponseBreachedAt != nil {
		return SLABreached
	}
	if !IsOpen(t.Status) {
		return SLAOk
	}
	if t.ResolutionDueAt != nil && now.After(*t.ResolutionDueAt) {
		return SLABreached
	}
	if t.FirstRespondedAt == nil && t.ResponseDueAt != nil && now.After(*t.ResponseDueAt) {
		return SLABreached
	}
	if t.EscalationLevel > 0 {
		return SLAWarning
	}
	return SLAOk
}

// ---------------------------------------------------------------------------
// Teams
// ---------------------------------------------------------------------------

func (s *Service) ListTeams(w http.ResponseWriter, r *http.Request) {
	c, ok := allow(w, r, isDeskStaff)
	if !ok {
		return
	}
	teams, err := s.Store.ListTeams(r.Context(), c.schoolID)
	if err != nil {
		writeErr(w, err)
		return
	}
	common.WriteJSON(w, http.StatusOK, teams, nil)
}

func (s *Service) SaveTeam(w http.ResponseWriter, r *http.Request) {
	c, ok := allow(w, r, isManager)
	if !ok {
		return
	}
	var in Team
	if !common.Decode(w, r, &in) {
		return
	}
	in.Name = strings.TrimSpace(in.Name)
	in.ManagerUserID = trimPtr(in.ManagerUserID)
	if in.Name == "" {
		common.WriteError(w, http.StatusUnprocessableEntity, "nama tim wajib diisi")
		return
	}
	if in.ManagerUserID != nil && !isUUID(*in.ManagerUserID) {
		common.WriteError(w, http.StatusUnprocessableEntity, "supervisor tim tidak valid")
		return
	}
	for _, m := range in.MemberIDs {
		if !isUUID(m) {
			common.WriteError(w, http.StatusUnprocessableEntity, "anggota tim tidak valid")
			return
		}
	}
	id, err := s.Store.SaveTeam(r.Context(), c.schoolID, chi.URLParam(r, "id"), in)
	if err != nil {
		writeErr(w, err)
		return
	}
	in.ID, in.SchoolID = id, c.schoolID
	s.audit(r.Context(), c.schoolID, id, "team_saved", &c.userID, nil, in, "")
	common.WriteJSON(w, http.StatusOK, in, nil)
}

func (s *Service) DeleteTeam(w http.ResponseWriter, r *http.Request) {
	c, ok := allow(w, r, isManager)
	if !ok {
		return
	}
	id := chi.URLParam(r, "id")
	if err := s.Store.DeleteTeam(r.Context(), c.schoolID, id); err != nil {
		writeErr(w, err)
		return
	}
	s.audit(r.Context(), c.schoolID, id, "team_deleted", &c.userID, nil, nil, "")
	common.WriteJSON(w, http.StatusOK, map[string]string{"id": id}, nil)
}

// ListAgents lists service desk agents and managers (pickers for teams,
// assignment and escalation targets).
func (s *Service) ListAgents(w http.ResponseWriter, r *http.Request) {
	c, ok := allow(w, r, isDeskStaff)
	if !ok {
		return
	}
	out := []map[string]string{}
	for _, role := range []string{RoleAgent, RoleManager} {
		users, err := s.Client.UsersInSchool(r.Context(), c.schoolID, role, nil)
		if err != nil {
			common.WriteError(w, http.StatusBadGateway, "gagal memuat daftar petugas dari identitas")
			return
		}
		for _, u := range users {
			out = append(out, map[string]string{"id": u.ID, "name": u.Name, "role": role})
		}
	}
	common.WriteJSON(w, http.StatusOK, out, nil)
}

// ---------------------------------------------------------------------------
// Categories
// ---------------------------------------------------------------------------

func (s *Service) ListCategories(w http.ResponseWriter, r *http.Request) {
	c, ok := allow(w, r, isDeskStaff)
	if !ok {
		return
	}
	cats, err := s.Store.ListCategories(r.Context(), c.schoolID, r.URL.Query().Get("active") == "true")
	if err != nil {
		writeErr(w, err)
		return
	}
	common.WriteJSON(w, http.StatusOK, cats, nil)
}

func (s *Service) GetCategory(w http.ResponseWriter, r *http.Request) {
	c, ok := allow(w, r, isDeskStaff)
	if !ok {
		return
	}
	cat, err := s.Store.GetCategory(r.Context(), c.schoolID, chi.URLParam(r, "id"))
	if err != nil {
		writeErr(w, err)
		return
	}
	common.WriteJSON(w, http.StatusOK, cat, nil)
}

func (s *Service) SaveCategory(w http.ResponseWriter, r *http.Request) {
	c, ok := allow(w, r, isManager)
	if !ok {
		return
	}
	var in Category
	if !common.Decode(w, r, &in) {
		return
	}
	in.Name = strings.TrimSpace(in.Name)
	in.Description = strings.TrimSpace(in.Description)
	in.ServiceRequestCategoryID = trimPtr(in.ServiceRequestCategoryID)
	if in.FormSchema == nil {
		in.FormSchema = []FormField{}
	}
	switch {
	case in.Name == "":
		common.WriteError(w, http.StatusUnprocessableEntity, "nama kategori wajib diisi")
		return
	case !isUUID(in.TeamID):
		common.WriteError(w, http.StatusUnprocessableEntity, "tim penanggung jawab wajib dipilih")
		return
	case len(in.ReporterKinds) == 0:
		common.WriteError(w, http.StatusUnprocessableEntity, "pilih minimal satu jenis pelapor")
		return
	case in.ServiceRequestCategoryID != nil && !isUUID(*in.ServiceRequestCategoryID):
		common.WriteError(w, http.StatusUnprocessableEntity, "kategori layanan orang tua tidak valid")
		return
	}
	for _, k := range in.ReporterKinds {
		if !slices.Contains(ReporterKinds, k) {
			common.WriteError(w, http.StatusUnprocessableEntity, "jenis pelapor tidak dikenal: "+k)
			return
		}
	}
	if err := ValidateSchema(in.FormSchema); err != nil {
		common.WriteError(w, http.StatusUnprocessableEntity, err.Error())
		return
	}
	id := chi.URLParam(r, "id")
	var before *Category
	if id != "" {
		before, _ = s.Store.GetCategory(r.Context(), c.schoolID, id)
	}
	id, err := s.Store.SaveCategory(r.Context(), c.schoolID, id, in)
	if err != nil {
		writeErr(w, err)
		return
	}
	saved, err := s.Store.GetCategory(r.Context(), c.schoolID, id)
	if err != nil {
		writeErr(w, err)
		return
	}
	s.audit(r.Context(), c.schoolID, id, "category_saved", &c.userID, before, saved, "")
	common.WriteJSON(w, http.StatusOK, saved, nil)
}

func (s *Service) DeleteCategory(w http.ResponseWriter, r *http.Request) {
	c, ok := allow(w, r, isManager)
	if !ok {
		return
	}
	id := chi.URLParam(r, "id")
	if err := s.Store.DeleteCategory(r.Context(), c.schoolID, id); err != nil {
		writeErr(w, err)
		return
	}
	s.audit(r.Context(), c.schoolID, id, "category_deleted", &c.userID, nil, nil, "")
	common.WriteJSON(w, http.StatusOK, map[string]string{"id": id}, nil)
}

// ---------------------------------------------------------------------------
// Business hours, holidays, SLA policies
// ---------------------------------------------------------------------------

func (s *Service) GetBusinessHours(w http.ResponseWriter, r *http.Request) {
	c, ok := allow(w, r, isManager)
	if !ok {
		return
	}
	hours, err := s.Store.BusinessHours(r.Context(), c.schoolID)
	if err != nil {
		writeErr(w, err)
		return
	}
	common.WriteJSON(w, http.StatusOK, hours, nil)
}

func (s *Service) PutBusinessHours(w http.ResponseWriter, r *http.Request) {
	c, ok := allow(w, r, isManager)
	if !ok {
		return
	}
	var in []BusinessHour
	if !common.Decode(w, r, &in) {
		return
	}
	for _, h := range in {
		o, err1 := ParseClock(h.OpensAt)
		cl, err2 := ParseClock(h.ClosesAt)
		if h.Weekday < 1 || h.Weekday > 7 || err1 != nil || err2 != nil || cl <= o {
			common.WriteError(w, http.StatusUnprocessableEntity, "jam layanan tidak valid: jam tutup harus setelah jam buka")
			return
		}
	}
	before, _ := s.Store.BusinessHours(r.Context(), c.schoolID)
	if err := s.Store.ReplaceBusinessHours(r.Context(), c.schoolID, in); err != nil {
		writeErr(w, err)
		return
	}
	s.audit(r.Context(), c.schoolID, c.schoolID, "business_hours_saved", &c.userID, before, in, "")
	common.WriteJSON(w, http.StatusOK, in, nil)
}

func (s *Service) ListHolidays(w http.ResponseWriter, r *http.Request) {
	c, ok := allow(w, r, isManager)
	if !ok {
		return
	}
	hols, err := s.Store.ListHolidays(r.Context(), c.schoolID)
	if err != nil {
		writeErr(w, err)
		return
	}
	common.WriteJSON(w, http.StatusOK, hols, nil)
}

func (s *Service) CreateHoliday(w http.ResponseWriter, r *http.Request) {
	c, ok := allow(w, r, isManager)
	if !ok {
		return
	}
	var in Holiday
	if !common.Decode(w, r, &in) {
		return
	}
	in.Name = strings.TrimSpace(in.Name)
	if _, err := time.Parse("2006-01-02", in.Date); err != nil || in.Name == "" {
		common.WriteError(w, http.StatusUnprocessableEntity, "tanggal (YYYY-MM-DD) dan nama hari libur wajib diisi")
		return
	}
	id, err := s.Store.CreateHoliday(r.Context(), c.schoolID, in)
	if err != nil {
		writeErr(w, err)
		return
	}
	in.ID = id
	s.audit(r.Context(), c.schoolID, id, "holiday_created", &c.userID, nil, in, "")
	common.WriteJSON(w, http.StatusCreated, in, nil)
}

func (s *Service) DeleteHoliday(w http.ResponseWriter, r *http.Request) {
	c, ok := allow(w, r, isManager)
	if !ok {
		return
	}
	id := chi.URLParam(r, "id")
	if err := s.Store.DeleteHoliday(r.Context(), c.schoolID, id); err != nil {
		writeErr(w, err)
		return
	}
	s.audit(r.Context(), c.schoolID, id, "holiday_deleted", &c.userID, nil, nil, "")
	common.WriteJSON(w, http.StatusOK, map[string]string{"id": id}, nil)
}

func (s *Service) ListPolicies(w http.ResponseWriter, r *http.Request) {
	c, ok := allow(w, r, isManager)
	if !ok {
		return
	}
	ps, err := s.Store.ListPolicies(r.Context(), c.schoolID)
	if err != nil {
		writeErr(w, err)
		return
	}
	common.WriteJSON(w, http.StatusOK, ps, nil)
}

func validatePolicy(p Policy) string {
	switch {
	case !slices.Contains(Priorities, p.Priority):
		return "prioritas tidak dikenal"
	case p.ResponseValue <= 0 || p.ResolutionValue <= 0:
		return "target respons dan penyelesaian harus lebih dari 0"
	case !slices.Contains(Units, p.ResponseUnit) || !slices.Contains(Units, p.ResolutionUnit):
		return "satuan target tidak dikenal"
	case p.WarnBp < 1 || p.WarnBp > 9999:
		return "ambang peringatan harus antara 0,01% dan 99,99%"
	case p.CategoryID != nil && !isUUID(*p.CategoryID):
		return "kategori tidak valid"
	}
	if _, err := time.Parse("2006-01-02", p.EffectiveFrom); err != nil {
		return "tanggal berlaku wajib diisi (YYYY-MM-DD)"
	}
	if p.EffectiveTo != nil {
		if _, err := time.Parse("2006-01-02", *p.EffectiveTo); err != nil || *p.EffectiveTo < p.EffectiveFrom {
			return "tanggal berakhir tidak valid"
		}
	}
	for _, r := range p.Rules {
		switch {
		case !slices.Contains(Triggers, r.Trigger) || !slices.Contains(TargetKinds, r.TargetKind) || r.Level < 1:
			return "aturan eskalasi tidak valid"
		case r.TargetKind == TargetRole && (r.TargetRole == nil || *r.TargetRole == ""):
			return "aturan eskalasi ke peran wajib menyebut peran"
		case r.TargetKind == TargetUser && (r.TargetUserID == nil || !isUUID(*r.TargetUserID)):
			return "aturan eskalasi ke pengguna wajib menyebut pengguna"
		}
	}
	return ""
}

func (s *Service) SavePolicy(w http.ResponseWriter, r *http.Request) {
	c, ok := allow(w, r, isManager)
	if !ok {
		return
	}
	var in Policy
	if !common.Decode(w, r, &in) {
		return
	}
	in.CategoryID = trimPtr(in.CategoryID)
	in.EffectiveTo = trimPtr(in.EffectiveTo)
	if in.Rules == nil {
		in.Rules = []EscalationRule{}
	}
	if msg := validatePolicy(in); msg != "" {
		common.WriteError(w, http.StatusUnprocessableEntity, msg)
		return
	}
	id, err := s.Store.SavePolicy(r.Context(), c.schoolID, chi.URLParam(r, "id"), in)
	if err != nil {
		writeErr(w, err)
		return
	}
	in.ID, in.SchoolID = id, c.schoolID
	s.audit(r.Context(), c.schoolID, id, "sla_policy_saved", &c.userID, nil, in, "")
	common.WriteJSON(w, http.StatusOK, in, nil)
}

func (s *Service) DeletePolicy(w http.ResponseWriter, r *http.Request) {
	c, ok := allow(w, r, isManager)
	if !ok {
		return
	}
	id := chi.URLParam(r, "id")
	if err := s.Store.DeletePolicy(r.Context(), c.schoolID, id); err != nil {
		writeErr(w, err)
		return
	}
	s.audit(r.Context(), c.schoolID, id, "sla_policy_deleted", &c.userID, nil, nil, "")
	common.WriteJSON(w, http.StatusOK, map[string]string{"id": id}, nil)
}

// ---------------------------------------------------------------------------
// Agent ticket work
// ---------------------------------------------------------------------------

// agentTicket loads a ticket the caller may handle: managers see all,
// agents only tickets of their teams (others → 404).
func (s *Service) agentTicket(w http.ResponseWriter, r *http.Request) (caller, *Ticket, bool) {
	c, ok := allow(w, r, isDeskStaff)
	if !ok {
		return c, nil, false
	}
	t, err := s.Store.GetTicket(r.Context(), c.schoolID, chi.URLParam(r, "id"))
	if err != nil {
		writeErr(w, err)
		return c, nil, false
	}
	if !isManager(c.claims.ActiveRole) {
		teams, err := s.Store.UserTeamIDs(r.Context(), c.schoolID, c.userID)
		if err != nil {
			writeErr(w, err)
			return c, nil, false
		}
		if !slices.Contains(teams, t.TeamID) {
			common.WriteError(w, http.StatusNotFound, "tiket tidak ditemukan")
			return c, nil, false
		}
	}
	return c, t, true
}

func (s *Service) ListTickets(w http.ResponseWriter, r *http.Request) {
	c, ok := allow(w, r, isDeskStaff)
	if !ok {
		return
	}
	q := r.URL.Query()
	f := TicketFilter{SchoolID: c.schoolID, Status: q.Get("status"), CategoryID: q.Get("category_id"),
		TeamID: q.Get("team_id"), SLA: q.Get("sla"), Now: s.Now()}
	for _, v := range []string{f.CategoryID, f.TeamID} {
		if v != "" && !isUUID(v) {
			common.WriteError(w, http.StatusBadRequest, "filter tidak valid")
			return
		}
	}
	if q.Get("assignee") == "me" {
		f.AssigneeID = c.userID
	}
	if !isManager(c.claims.ActiveRole) {
		teams, err := s.Store.UserTeamIDs(r.Context(), c.schoolID, c.userID)
		if err != nil {
			writeErr(w, err)
			return
		}
		f.TeamIDs = teams
	}
	page, perPage := common.Page(r)
	list, total, err := s.Store.ListTickets(r.Context(), f, page, perPage)
	if err != nil {
		writeErr(w, err)
		return
	}
	s.decorate(r.Context(), c.schoolID, list, nil)
	common.WriteJSON(w, http.StatusOK, list, map[string]int{"page": page, "per_page": perPage, "total": total})
}

type ticketInput struct {
	CategoryID     string         `json:"category_id"`
	Priority       string         `json:"priority"`
	Subject        string         `json:"subject"`
	Description    string         `json:"description"`
	FormAnswers    map[string]any `json:"form_answers"`
	StudentUserID  *string        `json:"student_user_id"`
	ReporterUserID string         `json:"reporter_user_id"` // agent channel only
	ReporterKind   string         `json:"reporter_kind"`    // agent channel only
}

// prepare validates a ticket body against its category; writes the error.
func (s *Service) prepare(w http.ResponseWriter, r *http.Request, schoolID string, in *ticketInput, kind string) (*Category, bool) {
	in.Subject, in.Description = strings.TrimSpace(in.Subject), strings.TrimSpace(in.Description)
	in.StudentUserID = trimPtr(in.StudentUserID)
	if in.Priority == "" {
		in.Priority = "normal"
	}
	switch {
	case !isUUID(in.CategoryID) || in.Subject == "" || in.Description == "":
		common.WriteError(w, http.StatusUnprocessableEntity, "kategori, judul, dan uraian wajib diisi")
		return nil, false
	case !slices.Contains(Priorities, in.Priority):
		common.WriteError(w, http.StatusUnprocessableEntity, "prioritas tidak dikenal")
		return nil, false
	case in.StudentUserID != nil && !isUUID(*in.StudentUserID):
		common.WriteError(w, http.StatusUnprocessableEntity, "siswa tidak valid")
		return nil, false
	}
	cat, err := s.Store.GetCategory(r.Context(), schoolID, in.CategoryID)
	if errors.Is(err, ErrNotFound) {
		common.WriteError(w, http.StatusUnprocessableEntity, "kategori tiket tidak ditemukan")
		return nil, false
	} else if err != nil {
		writeErr(w, err)
		return nil, false
	}
	if !CanReport(*cat, kind) {
		common.WriteError(w, http.StatusUnprocessableEntity, "kategori ini tidak menerima tiket dari jenis pelapor Anda")
		return nil, false
	}
	if errs := ValidateForm(cat.FormSchema, in.FormAnswers); len(errs) > 0 {
		validationErr(w, "isian formulir belum lengkap", errs)
		return nil, false
	}
	return cat, true
}

func (s *Service) CreateAgentTicket(w http.ResponseWriter, r *http.Request) {
	c, ok := allow(w, r, isDeskStaff)
	if !ok {
		return
	}
	var in ticketInput
	if !common.Decode(w, r, &in) {
		return
	}
	if !isUUID(in.ReporterUserID) || !slices.Contains(ReporterKinds, in.ReporterKind) {
		common.WriteError(w, http.StatusUnprocessableEntity, "pelapor dan jenis pelapor wajib diisi")
		return
	}
	cat, ok := s.prepare(w, r, c.schoolID, &in, in.ReporterKind)
	if !ok {
		return
	}
	t, err := s.CreateTicket(r.Context(), cat, NewTicket{
		SchoolID: c.schoolID, Priority: in.Priority, Subject: in.Subject, Description: in.Description,
		FormAnswers: in.FormAnswers, Channel: ChannelAgent, ReporterUserID: in.ReporterUserID,
		StudentUserID: in.StudentUserID, ActorUserID: c.userID, IdempotencyKey: idemKey(r),
	})
	if err != nil {
		writeErr(w, err)
		return
	}
	common.WriteJSON(w, http.StatusCreated, t, nil)
}

func (s *Service) GetTicket(w http.ResponseWriter, r *http.Request) {
	c, t, ok := s.agentTicket(w, r)
	if !ok {
		return
	}
	events, err := s.Store.ListEvents(r.Context(), c.schoolID, t.ID, true)
	if err != nil {
		writeErr(w, err)
		return
	}
	list := []Ticket{*t}
	s.decorate(r.Context(), c.schoolID, list, events)
	resp := map[string]any{"ticket": list[0], "events": events}
	if t.WorkOrderID != nil && t.WorkOrderNumber != nil {
		// Status is pulled from ops when the detail opens (no callback).
		wos, err := s.Client.WorkOrdersByTicket(r.Context(), c.schoolID, t.Number)
		if err != nil {
			resp["work_order_error"] = "status perintah kerja tidak dapat dimuat"
		}
		for _, wo := range wos {
			if wo.ID == *t.WorkOrderID {
				resp["work_order"] = wo
			}
		}
	}
	common.WriteJSON(w, http.StatusOK, resp, nil)
}

func (s *Service) mutate(w http.ResponseWriter, r *http.Request, c caller, t *Ticket, ch Change, action, what string) {
	ch.ActorUserID, ch.Now = c.userID, s.Now()
	res, err := s.Store.Mutate(r.Context(), c.schoolID, t.ID, ch)
	if err != nil {
		writeErr(w, err)
		return
	}
	reason := ch.Note
	s.audit(r.Context(), c.schoolID, t.ID, action, &c.userID,
		map[string]any{"status": t.Status, "assignee_user_id": t.AssigneeUserID},
		map[string]any{"status": res.Ticket.Status, "assignee_user_id": res.Ticket.AssigneeUserID, "internal": ch.Internal}, reason)
	if res.EventID != "" {
		s.NotifyReporter(r.Context(), res.Ticket, res.EventID, what)
	}
	common.WriteJSON(w, http.StatusOK, res.Ticket, nil)
}

func (s *Service) AssignTicket(w http.ResponseWriter, r *http.Request) {
	c, t, ok := s.agentTicket(w, r)
	if !ok {
		return
	}
	var in struct {
		UserID string `json:"user_id"`
	}
	if !common.Decode(w, r, &in) {
		return
	}
	if !isUUID(in.UserID) {
		common.WriteError(w, http.StatusUnprocessableEntity, "petugas wajib dipilih")
		return
	}
	// Agents may only pick themselves or a teammate; managers anyone on the team.
	members, manager, err := s.Store.TeamRecipients(r.Context(), c.schoolID, t.TeamID)
	if err != nil {
		writeErr(w, err)
		return
	}
	if !slices.Contains(members, in.UserID) && (manager == nil || *manager != in.UserID) {
		common.WriteError(w, http.StatusUnprocessableEntity, "petugas bukan anggota tim kategori ini")
		return
	}
	s.mutate(w, r, c, t, Change{ByAgent: true, AssigneeID: in.UserID}, "ticket_assigned", "Tiket Anda sudah ditugaskan ke petugas")
}

func (s *Service) ChangeStatus(w http.ResponseWriter, r *http.Request) {
	c, t, ok := s.agentTicket(w, r)
	if !ok {
		return
	}
	var in struct {
		To   string `json:"to"`
		Note string `json:"note"`
	}
	if !common.Decode(w, r, &in) {
		return
	}
	in.Note = strings.TrimSpace(in.Note)
	msg := "Status tiket berubah menjadi " + statusLabels[in.To]
	if in.Note != "" {
		msg += ": " + in.Note
	}
	s.mutate(w, r, c, t, Change{ByAgent: true, To: in.To, Note: in.Note}, "ticket_status_changed", msg)
}

func (s *Service) AgentComment(w http.ResponseWriter, r *http.Request) {
	c, t, ok := s.agentTicket(w, r)
	if !ok {
		return
	}
	body, internal, atts, ok := s.readComment(w, r, c.schoolID)
	if !ok {
		return
	}
	s.mutate(w, r, c, t, Change{ByAgent: true, Comment: &body, Internal: internal, Attachments: atts},
		"ticket_commented", "Petugas menambahkan tanggapan")
}

func (s *Service) CreateWorkOrder(w http.ResponseWriter, r *http.Request) {
	c, t, ok := s.agentTicket(w, r)
	if !ok {
		return
	}
	var in struct {
		RoomID *string `json:"room_id"`
	}
	if r.ContentLength != 0 && !common.Decode(w, r, &in) {
		return
	}
	in.RoomID = trimPtr(in.RoomID)
	if in.RoomID != nil && !isUUID(*in.RoomID) {
		common.WriteError(w, http.StatusUnprocessableEntity, "ruang tidak valid")
		return
	}
	updated, err := s.ConvertToWorkOrder(r.Context(), t, in.RoomID, c.userID)
	if err != nil {
		writeErr(w, err)
		return
	}
	common.WriteJSON(w, http.StatusOK, updated, nil)
}

func (s *Service) ListRooms(w http.ResponseWriter, r *http.Request) {
	c, ok := allow(w, r, isDeskStaff)
	if !ok {
		return
	}
	rooms, err := s.Client.FacilityRooms(r.Context(), c.schoolID)
	if err != nil {
		common.WriteError(w, http.StatusBadGateway, "daftar ruang dari modul fasilitas tidak dapat dimuat")
		return
	}
	common.WriteJSON(w, http.StatusOK, rooms, nil)
}

func (s *Service) AgentFile(w http.ResponseWriter, r *http.Request) {
	c, t, ok := s.agentTicket(w, r)
	if !ok {
		return
	}
	s.serveFile(w, r, c.schoolID, t.ID, true)
}

// ---------------------------------------------------------------------------
// Report (FR-138-003)
// ---------------------------------------------------------------------------

func (s *Service) Report(w http.ResponseWriter, r *http.Request) {
	c, ok := allow(w, r, func(role string) bool { return role == RoleManager || role == common.RoleKepalaSekolah || isAdmin(role) })
	if !ok {
		return
	}
	q := r.URL.Query()
	from, err1 := time.ParseInLocation("2006-01-02", q.Get("from"), s.Loc)
	to, err2 := time.ParseInLocation("2006-01-02", q.Get("to"), s.Loc)
	if err1 != nil || err2 != nil || to.Before(from) {
		common.WriteError(w, http.StatusBadRequest, "rentang tanggal (from, to) wajib diisi dengan format YYYY-MM-DD")
		return
	}
	groupBy := q.Get("group_by")
	if groupBy != "team" {
		groupBy = "category"
	}
	rows, err := s.Store.Report(r.Context(), c.schoolID, from, to.AddDate(0, 0, 1), groupBy)
	if err != nil {
		writeErr(w, err)
		return
	}
	common.WriteJSON(w, http.StatusOK, rows, map[string]any{"generated_at": s.Now(), "group_by": groupBy})
}

// ---------------------------------------------------------------------------
// Reporter portal (/me/tickets)
// ---------------------------------------------------------------------------

func (s *Service) MyCategories(w http.ResponseWriter, r *http.Request) {
	c := who(r)
	cats, err := s.Store.ListCategories(r.Context(), c.schoolID, true)
	if err != nil {
		writeErr(w, err)
		return
	}
	kind := ReporterKindForRole(c.claims.ActiveRole)
	out := []Category{}
	for _, cat := range cats {
		if CanReport(cat, kind) {
			out = append(out, cat)
		}
	}
	common.WriteJSON(w, http.StatusOK, out, nil)
}

func (s *Service) CreateMyTicket(w http.ResponseWriter, r *http.Request) {
	c := who(r)
	if err := r.ParseMultipartForm(s.Config.MaxUploadBytes); err != nil {
		common.WriteError(w, http.StatusBadRequest, "format formulir tidak valid")
		return
	}
	var in ticketInput
	if err := json.Unmarshal([]byte(r.FormValue("data")), &in); err != nil {
		common.WriteError(w, http.StatusBadRequest, "data tiket tidak valid")
		return
	}
	in.Priority = "normal" // reporters do not set priority; agents triage
	role := c.claims.ActiveRole
	kind := ReporterKindForRole(role)
	switch kind {
	case KindStudent:
		self := c.userID
		in.StudentUserID = &self
	case KindParent:
		if in.StudentUserID != nil && strings.TrimSpace(*in.StudentUserID) != "" {
			okLink, err := s.Client.GuardianHasStudent(r.Context(), c.schoolID, c.userID, strings.TrimSpace(*in.StudentUserID))
			if err != nil {
				common.WriteError(w, http.StatusBadGateway, "gagal memverifikasi data anak")
				return
			}
			if !okLink {
				common.WriteError(w, http.StatusForbidden, "siswa ini tidak terhubung dengan akun Anda")
				return
			}
		}
	default:
		in.StudentUserID = nil
	}
	cat, ok := s.prepare(w, r, c.schoolID, &in, kind)
	if !ok {
		return
	}
	atts, ok := s.saveFiles(w, r, c.schoolID)
	if !ok {
		return
	}
	t, err := s.CreateTicket(r.Context(), cat, NewTicket{
		SchoolID: c.schoolID, Priority: in.Priority, Subject: in.Subject, Description: in.Description,
		FormAnswers: in.FormAnswers, Channel: ChannelForKind(kind), ReporterUserID: c.userID,
		StudentUserID: in.StudentUserID, Attachments: atts, ActorUserID: c.userID, IdempotencyKey: idemKey(r),
	})
	if err != nil {
		writeErr(w, err)
		return
	}
	common.WriteJSON(w, http.StatusCreated, t, nil)
}

func (s *Service) ListMyTickets(w http.ResponseWriter, r *http.Request) {
	c := who(r)
	page, perPage := common.Page(r)
	list, total, err := s.Store.ListTickets(r.Context(), TicketFilter{SchoolID: c.schoolID, ReporterID: c.userID,
		Status: r.URL.Query().Get("status")}, page, perPage)
	if err != nil {
		writeErr(w, err)
		return
	}
	for i := range list {
		redactForReporter(&list[i])
	}
	common.WriteJSON(w, http.StatusOK, list, map[string]int{"page": page, "per_page": perPage, "total": total})
}

// redactForReporter hides desk-internal fields from the reporter's view.
func redactForReporter(t *Ticket) {
	t.SLA = nil
	t.EscalationLevel = 0
	t.ResponseBreachedAt, t.ResolutionBreachedAt = nil, nil
}

// myTicket loads the caller's own ticket (others → 404).
func (s *Service) myTicket(w http.ResponseWriter, r *http.Request) (caller, *Ticket, bool) {
	c := who(r)
	t, err := s.Store.GetTicket(r.Context(), c.schoolID, chi.URLParam(r, "id"))
	if err == nil && t.ReporterUserID != c.userID {
		err = ErrNotFound
	}
	if err != nil {
		writeErr(w, err)
		return c, nil, false
	}
	return c, t, true
}

func (s *Service) GetMyTicket(w http.ResponseWriter, r *http.Request) {
	c, t, ok := s.myTicket(w, r)
	if !ok {
		return
	}
	events, err := s.Store.ListEvents(r.Context(), c.schoolID, t.ID, false)
	if err != nil {
		writeErr(w, err)
		return
	}
	list := []Ticket{*t}
	s.decorate(r.Context(), c.schoolID, list, events)
	redactForReporter(&list[0])
	list[0].SLAState = ""
	common.WriteJSON(w, http.StatusOK, map[string]any{"ticket": list[0], "events": events}, nil)
}

func (s *Service) MyComment(w http.ResponseWriter, r *http.Request) {
	c, t, ok := s.myTicket(w, r)
	if !ok {
		return
	}
	body, _, atts, ok := s.readComment(w, r, c.schoolID)
	if !ok {
		return
	}
	res, err := s.Store.Mutate(r.Context(), c.schoolID, t.ID, Change{ActorUserID: c.userID, Comment: &body, Attachments: atts, Now: s.Now()})
	if err != nil {
		writeErr(w, err)
		return
	}
	s.audit(r.Context(), c.schoolID, t.ID, "ticket_commented", &c.userID, nil, map[string]any{"by": "reporter"}, "")
	s.NotifyTeam(r.Context(), res.Ticket, "comment:"+res.CommentID, "Tanggapan pelapor pada "+t.Number, body)
	redactForReporter(res.Ticket)
	common.WriteJSON(w, http.StatusOK, res.Ticket, nil)
}

func (s *Service) Reopen(w http.ResponseWriter, r *http.Request) {
	c, t, ok := s.myTicket(w, r)
	if !ok {
		return
	}
	var in struct {
		Note string `json:"note"`
	}
	if r.ContentLength != 0 && !common.Decode(w, r, &in) {
		return
	}
	if t.Status != StatusResolved {
		common.WriteError(w, http.StatusUnprocessableEntity, "hanya tiket berstatus selesai yang dapat dibuka ulang")
		return
	}
	res, err := s.Store.Mutate(r.Context(), c.schoolID, t.ID, Change{ActorUserID: c.userID, To: StatusInProgress,
		Note: strings.TrimSpace(in.Note), Now: s.Now()})
	if err != nil {
		writeErr(w, err)
		return
	}
	s.audit(r.Context(), c.schoolID, t.ID, "ticket_reopened", &c.userID, map[string]any{"status": t.Status},
		map[string]any{"status": res.Ticket.Status}, in.Note)
	s.NotifyTeam(r.Context(), res.Ticket, "reopen:"+res.EventID, "Tiket "+t.Number+" dibuka ulang", in.Note)
	redactForReporter(res.Ticket)
	common.WriteJSON(w, http.StatusOK, res.Ticket, nil)
}

func (s *Service) Rate(w http.ResponseWriter, r *http.Request) {
	c, t, ok := s.myTicket(w, r)
	if !ok {
		return
	}
	var in struct {
		Score   int     `json:"score"`
		Comment *string `json:"comment"`
	}
	if !common.Decode(w, r, &in) {
		return
	}
	if in.Score < 1 || in.Score > 5 {
		common.WriteError(w, http.StatusUnprocessableEntity, "nilai kepuasan harus 1 sampai 5")
		return
	}
	if err := s.Store.Rate(r.Context(), c.schoolID, t.ID, c.userID, in.Score, trimPtr(in.Comment), s.Now()); err != nil {
		writeErr(w, err)
		return
	}
	s.audit(r.Context(), c.schoolID, t.ID, "ticket_rated", &c.userID, nil, map[string]any{"score": in.Score}, "")
	updated, err := s.Store.GetTicket(r.Context(), c.schoolID, t.ID)
	if err != nil {
		writeErr(w, err)
		return
	}
	redactForReporter(updated)
	common.WriteJSON(w, http.StatusOK, updated, nil)
}

func (s *Service) MyFile(w http.ResponseWriter, r *http.Request) {
	c, t, ok := s.myTicket(w, r)
	if !ok {
		return
	}
	s.serveFile(w, r, c.schoolID, t.ID, false)
}

// ---------------------------------------------------------------------------
// Menu 81 link + internal retry
// ---------------------------------------------------------------------------

// TicketForServiceRequest returns {ticket_id, number, status} for a menu 81
// request the caller can see (requester, its handler role, admin, desk staff).
func (s *Service) TicketForServiceRequest(w http.ResponseWriter, r *http.Request) {
	c := common.FromCtx(r)
	schoolID := common.TenantSchoolID(r)
	if c == nil || !isUUID(schoolID) {
		common.WriteError(w, http.StatusBadRequest, "school_id diperlukan")
		return
	}
	srID := chi.URLParam(r, "srId")
	sr, err := s.Core.GetServiceRequest(r.Context(), schoolID, srID)
	if errors.Is(err, store.ErrNotFound) {
		common.WriteError(w, http.StatusNotFound, "permintaan layanan tidak ditemukan")
		return
	} else if err != nil {
		writeErr(w, err)
		return
	}
	if sr.RequesterUserID != c.GetUserID() && !c.HasRole(sr.HandlerRole) && !isAdmin(c.ActiveRole) && !isDeskStaff(c.ActiveRole) {
		common.WriteError(w, http.StatusNotFound, "permintaan layanan tidak ditemukan")
		return
	}
	t, err := s.Store.TicketByServiceRequest(r.Context(), schoolID, srID)
	if err != nil {
		writeErr(w, err)
		return
	}
	common.WriteJSON(w, http.StatusOK, map[string]string{"ticket_id": t.ID, "number": t.Number, "status": t.Status}, nil)
}

// InternalFromServiceRequest is the exception-queue retry for BR-SD-01.
func (s *Service) InternalFromServiceRequest(w http.ResponseWriter, r *http.Request) {
	schoolID := r.URL.Query().Get("school_id")
	if !isUUID(schoolID) {
		common.WriteError(w, http.StatusBadRequest, "school_id diperlukan")
		return
	}
	sr, err := s.Core.GetServiceRequest(r.Context(), schoolID, chi.URLParam(r, "id"))
	if errors.Is(err, store.ErrNotFound) {
		common.WriteError(w, http.StatusNotFound, "permintaan layanan tidak ditemukan")
		return
	} else if err != nil {
		writeErr(w, err)
		return
	}
	if !s.FeatureEnabled(r.Context(), schoolID) {
		common.WriteError(w, http.StatusForbidden, "Modul Service Desk belum aktif untuk sekolah ini")
		return
	}
	t, err := s.TicketFromServiceRequest(r.Context(), *sr)
	if errors.Is(err, ErrNotFound) {
		common.WriteError(w, http.StatusUnprocessableEntity, "belum ada kategori tiket untuk kategori layanan ini")
		return
	} else if err != nil {
		writeErr(w, err)
		return
	}
	common.WriteJSON(w, http.StatusOK, t, nil)
}

// ---------------------------------------------------------------------------
// files
// ---------------------------------------------------------------------------

func idemKey(r *http.Request) string {
	if k := strings.TrimSpace(r.Header.Get("Idempotency-Key")); k != "" {
		return k
	}
	return "auto:" + randomHex(16)
}

func randomHex(n int) string {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

var unsafeName = regexp.MustCompile(`[^A-Za-z0-9._-]+`)

// saveFiles stores the multipart "files" parts in private storage.
func (s *Service) saveFiles(w http.ResponseWriter, r *http.Request, schoolID string) ([]Attachment, bool) {
	out := []Attachment{}
	if r.MultipartForm == nil {
		return out, true
	}
	for _, fh := range r.MultipartForm.File["files"] {
		f, err := fh.Open()
		if err != nil {
			common.WriteError(w, http.StatusBadRequest, "lampiran tidak dapat dibaca")
			return nil, false
		}
		name := unsafeName.ReplaceAllString(filepath.Base(fh.Filename), "_")
		key := "service-desk/" + schoolID + "/" + randomHex(12) + "-" + name
		mime := fh.Header.Get("Content-Type")
		obj, err := s.Files.Put(r.Context(), key, mime, f, fh.Size)
		f.Close()
		if err != nil {
			common.WriteError(w, http.StatusUnprocessableEntity, "lampiran ditolak (hanya PDF/JPEG/PNG, ukuran maksimum dibatasi): "+fh.Filename)
			return nil, false
		}
		out = append(out, Attachment{Name: fh.Filename, Key: obj.Key, MimeType: obj.MIME, Size: obj.Size})
	}
	return out, true
}

// readComment parses a multipart comment: body, internal, files.
func (s *Service) readComment(w http.ResponseWriter, r *http.Request, schoolID string) (string, bool, []Attachment, bool) {
	if err := r.ParseMultipartForm(s.Config.MaxUploadBytes); err != nil {
		common.WriteError(w, http.StatusBadRequest, "format formulir tidak valid")
		return "", false, nil, false
	}
	body := strings.TrimSpace(r.FormValue("body"))
	if body == "" {
		common.WriteError(w, http.StatusUnprocessableEntity, "komentar wajib diisi")
		return "", false, nil, false
	}
	atts, ok := s.saveFiles(w, r, schoolID)
	return body, r.FormValue("internal") == "true", atts, ok
}

// serveFile streams an attachment that belongs to the ticket; reporters
// only reach attachments of public events.
func (s *Service) serveFile(w http.ResponseWriter, r *http.Request, schoolID, ticketID string, includeInternal bool) {
	key := chi.URLParam(r, "*")
	events, err := s.Store.ListEvents(r.Context(), schoolID, ticketID, includeInternal)
	if err != nil {
		writeErr(w, err)
		return
	}
	for _, e := range events {
		for _, a := range e.Attachments {
			if a.Key != key {
				continue
			}
			f, err := s.Files.Open(r.Context(), key)
			if err != nil {
				common.WriteError(w, http.StatusNotFound, "berkas tidak ditemukan")
				return
			}
			defer f.Close()
			w.Header().Set("Content-Type", a.MimeType)
			w.Header().Set("Content-Disposition", `inline; filename="`+unsafeName.ReplaceAllString(a.Name, "_")+`"`)
			w.Header().Set("Cache-Control", "private, no-store")
			_, _ = io.Copy(w, f)
			return
		}
	}
	common.WriteError(w, http.StatusNotFound, "berkas tidak ditemukan")
}
