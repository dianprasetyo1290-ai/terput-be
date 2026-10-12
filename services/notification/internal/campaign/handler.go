package campaign

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"skci/notification/internal/client"
	"skci/notification/internal/common"
	"skci/notification/internal/provider"
)

// Roles allowed per endpoint group (names final per T9 §2).
var (
	ManagerRoles  = []string{"humas", "admin", "superadmin"}
	SettingsRoles = []string{"admin", "superadmin"}
)

// Handler is the HTTP surface for menu 147 and marketing consent.
type Handler struct {
	Svc *Service
}

// NewHandler wires the handler.
func NewHandler(svc *Service) *Handler { return &Handler{Svc: svc} }

// Mount registers the JWT routes on r (already behind JWTMiddleware).
func (h *Handler) Mount(r chi.Router) {
	r.Group(func(m chi.Router) {
		m.Use(requireRoles(ManagerRoles...))
		m.Get("/campaign-segments", h.ListSegments)
		m.Post("/campaign-segments", h.CreateSegment)
		m.Get("/campaign-segments/{id}", h.GetSegment)
		m.Put("/campaign-segments/{id}", h.UpdateSegment)
		m.Delete("/campaign-segments/{id}", h.DeleteSegment)
		m.Get("/campaign-segments/{id}/preview", h.PreviewSegment)

		m.Get("/wa-templates", h.ListTemplates)
		m.Post("/wa-templates", h.CreateTemplate)
		m.Get("/wa-templates/{id}", h.GetTemplate)
		m.Put("/wa-templates/{id}", h.UpdateTemplate)
		m.Delete("/wa-templates/{id}", h.DeleteTemplate)

		m.Get("/campaigns", h.ListCampaigns)
		m.Post("/campaigns", h.CreateCampaign)
		m.Get("/campaigns/{id}", h.GetCampaign)
		m.Put("/campaigns/{id}", h.UpdateCampaign)
		m.Delete("/campaigns/{id}", h.DeleteCampaign)
		m.Post("/campaigns/{id}/schedule", h.ScheduleCampaign)
		m.Post("/campaigns/{id}/send-now", h.SendNow)
		m.Post("/campaigns/{id}/cancel", h.CancelCampaign)
		m.Get("/campaigns/{id}/stats", h.Stats)
		m.Get("/campaigns/{id}/recipients", h.Recipients)
		m.Get("/campaign-channels", h.Channels)
	})
	r.Group(func(a chi.Router) {
		a.Use(requireRoles(SettingsRoles...))
		a.Get("/campaign-settings", h.GetSettings)
		a.Put("/campaign-settings", h.PutSettings)
	})
	// Every signed-in user manages their own promotional consent.
	r.Get("/marketing-consents", h.GetMyConsents)
	r.Put("/marketing-consents", h.PutMyConsents)
}

func requireRoles(roles ...string) func(http.Handler) http.Handler {
	allowed := map[string]bool{}
	for _, r := range roles {
		allowed[r] = true
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			c := common.FromCtx(r)
			if c == nil {
				common.WriteError(w, http.StatusUnauthorized, "missing token")
				return
			}
			if !allowed[c.ActiveRole] {
				common.WriteError(w, http.StatusForbidden, "Anda tidak memiliki akses ke fitur ini")
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// schoolOf applies the multi-tenancy rule (superadmin may pass ?school_id=).
func schoolOf(w http.ResponseWriter, r *http.Request) (string, *common.Claims, bool) {
	c := common.FromCtx(r)
	school := c.SchoolID
	if c.ActiveRole == "superadmin" {
		if s := strings.TrimSpace(r.URL.Query().Get("school_id")); s != "" {
			school = s
		}
	}
	if school == "" {
		common.WriteError(w, http.StatusBadRequest, "school scope is required (superadmin must pass ?school_id=)")
		return "", nil, false
	}
	return school, c, true
}

func writeStoreErr(w http.ResponseWriter, err error, what string) {
	if errors.Is(err, ErrNotFound) {
		common.WriteError(w, http.StatusNotFound, what+" tidak ditemukan")
		return
	}
	common.CaptureError(err)
	common.WriteError(w, http.StatusInternalServerError, "gagal memproses "+what)
}

func (h *Handler) auditEvent(r *http.Request, c *common.Claims, school, entity, id, action string, before, after any) {
	h.Svc.audit(r.Context(), client.AuditEvent{
		SchoolID: school, EntityType: entity, EntityID: id, Action: action,
		ActorUserID: c.UserID, ActorRole: c.ActiveRole, Before: before, After: after,
	})
}

// ---- segments ----------------------------------------------------------------

type segmentPayload struct {
	Name   string        `json:"name"`
	Kind   string        `json:"kind"`
	Filter SegmentFilter `json:"filter"`
}

func (p segmentPayload) validate() string {
	if strings.TrimSpace(p.Name) == "" {
		return "nama segmen wajib diisi"
	}
	if !ValidSegmentKinds[p.Kind] {
		return "jenis segmen tidak dikenal (parents|ppdb_applicants|alumni_parents)"
	}
	return ""
}

// ListSegments handles GET /campaign-segments.
func (h *Handler) ListSegments(w http.ResponseWriter, r *http.Request) {
	school, _, ok := schoolOf(w, r)
	if !ok {
		return
	}
	list, err := h.Svc.Store.ListSegments(r.Context(), school)
	if err != nil {
		writeStoreErr(w, err, "segmen")
		return
	}
	common.WriteJSON(w, http.StatusOK, list, nil)
}

// GetSegment handles GET /campaign-segments/{id}.
func (h *Handler) GetSegment(w http.ResponseWriter, r *http.Request) {
	school, _, ok := schoolOf(w, r)
	if !ok {
		return
	}
	seg, err := h.Svc.Store.GetSegment(r.Context(), school, chi.URLParam(r, "id"))
	if err != nil {
		writeStoreErr(w, err, "segmen")
		return
	}
	common.WriteJSON(w, http.StatusOK, seg, nil)
}

// CreateSegment handles POST /campaign-segments.
func (h *Handler) CreateSegment(w http.ResponseWriter, r *http.Request) {
	school, c, ok := schoolOf(w, r)
	if !ok {
		return
	}
	var p segmentPayload
	if !common.Decode(w, r, &p) {
		return
	}
	if msg := p.validate(); msg != "" {
		common.WriteError(w, http.StatusUnprocessableEntity, msg)
		return
	}
	seg, err := h.Svc.Store.CreateSegment(r.Context(), Segment{SchoolID: school, Name: strings.TrimSpace(p.Name), Kind: p.Kind, Filter: p.Filter, CreatedBy: c.UserID})
	if err != nil {
		writeStoreErr(w, err, "segmen")
		return
	}
	h.auditEvent(r, c, school, "campaign_segment", seg.ID, "create", nil, seg)
	common.WriteJSONMsg(w, http.StatusCreated, seg, nil, "Segmen disimpan")
}

// UpdateSegment handles PUT /campaign-segments/{id}.
func (h *Handler) UpdateSegment(w http.ResponseWriter, r *http.Request) {
	school, c, ok := schoolOf(w, r)
	if !ok {
		return
	}
	var p segmentPayload
	if !common.Decode(w, r, &p) {
		return
	}
	if msg := p.validate(); msg != "" {
		common.WriteError(w, http.StatusUnprocessableEntity, msg)
		return
	}
	before, err := h.Svc.Store.GetSegment(r.Context(), school, chi.URLParam(r, "id"))
	if err != nil {
		writeStoreErr(w, err, "segmen")
		return
	}
	seg, err := h.Svc.Store.UpdateSegment(r.Context(), Segment{ID: before.ID, SchoolID: school, Name: strings.TrimSpace(p.Name), Kind: p.Kind, Filter: p.Filter})
	if err != nil {
		writeStoreErr(w, err, "segmen")
		return
	}
	h.auditEvent(r, c, school, "campaign_segment", seg.ID, "update", before, seg)
	common.WriteJSONMsg(w, http.StatusOK, seg, nil, "Segmen diperbarui")
}

// DeleteSegment handles DELETE /campaign-segments/{id} (soft delete).
func (h *Handler) DeleteSegment(w http.ResponseWriter, r *http.Request) {
	school, c, ok := schoolOf(w, r)
	if !ok {
		return
	}
	id := chi.URLParam(r, "id")
	if err := h.Svc.Store.DeleteSegment(r.Context(), school, id); err != nil {
		writeStoreErr(w, err, "segmen")
		return
	}
	h.auditEvent(r, c, school, "campaign_segment", id, "delete", nil, nil)
	common.WriteJSONMsg(w, http.StatusOK, map[string]string{"id": id}, nil, "Segmen dihapus")
}

// PreviewSegment handles GET /campaign-segments/{id}/preview?channel=.
func (h *Handler) PreviewSegment(w http.ResponseWriter, r *http.Request) {
	school, _, ok := schoolOf(w, r)
	if !ok {
		return
	}
	channel := r.URL.Query().Get("channel")
	if channel == "" {
		channel = ChannelInApp
	}
	if !ValidChannels[channel] {
		common.WriteError(w, http.StatusBadRequest, "kanal tidak dikenal")
		return
	}
	seg, err := h.Svc.Store.GetSegment(r.Context(), school, chi.URLParam(r, "id"))
	if err != nil {
		writeStoreErr(w, err, "segmen")
		return
	}
	p, err := h.Svc.PreviewSegment(r.Context(), school, *seg, channel)
	if err != nil {
		common.CaptureError(err)
		common.WriteError(w, http.StatusBadGateway, "gagal membaca data segmen: "+err.Error())
		return
	}
	common.WriteJSON(w, http.StatusOK, p, nil)
}

// ---- templates ------------------------------------------------------------------

type templatePayload struct {
	Name               string   `json:"name"`
	ProviderTemplateID string   `json:"provider_template_id"`
	Language           string   `json:"language"`
	BodyPreview        string   `json:"body_preview"`
	Variables          []string `json:"variables"`
	ApprovalStatus     string   `json:"approval_status"`
}

func (p *templatePayload) validate() string {
	if strings.TrimSpace(p.Name) == "" || strings.TrimSpace(p.ProviderTemplateID) == "" || strings.TrimSpace(p.Language) == "" {
		return "nama, ID template penyedia, dan bahasa wajib diisi"
	}
	if p.ApprovalStatus == "" {
		p.ApprovalStatus = TemplatePending
	}
	if !ValidTemplateStatuses[p.ApprovalStatus] {
		return "status persetujuan tidak dikenal (pending|approved|rejected)"
	}
	if p.Variables == nil {
		p.Variables = []string{}
	}
	return ""
}

func (p templatePayload) toTemplate(school, id string) WATemplate {
	return WATemplate{ID: id, SchoolID: school, Name: strings.TrimSpace(p.Name), ProviderTemplateID: strings.TrimSpace(p.ProviderTemplateID),
		Language: strings.TrimSpace(p.Language), BodyPreview: p.BodyPreview, Variables: p.Variables, ApprovalStatus: p.ApprovalStatus}
}

// ListTemplates handles GET /wa-templates.
func (h *Handler) ListTemplates(w http.ResponseWriter, r *http.Request) {
	school, _, ok := schoolOf(w, r)
	if !ok {
		return
	}
	list, err := h.Svc.Store.ListTemplates(r.Context(), school)
	if err != nil {
		writeStoreErr(w, err, "template")
		return
	}
	common.WriteJSON(w, http.StatusOK, list, nil)
}

// GetTemplate handles GET /wa-templates/{id}.
func (h *Handler) GetTemplate(w http.ResponseWriter, r *http.Request) {
	school, _, ok := schoolOf(w, r)
	if !ok {
		return
	}
	t, err := h.Svc.Store.GetTemplate(r.Context(), school, chi.URLParam(r, "id"))
	if err != nil {
		writeStoreErr(w, err, "template")
		return
	}
	common.WriteJSON(w, http.StatusOK, t, nil)
}

// CreateTemplate handles POST /wa-templates.
func (h *Handler) CreateTemplate(w http.ResponseWriter, r *http.Request) {
	school, c, ok := schoolOf(w, r)
	if !ok {
		return
	}
	var p templatePayload
	if !common.Decode(w, r, &p) {
		return
	}
	if msg := p.validate(); msg != "" {
		common.WriteError(w, http.StatusUnprocessableEntity, msg)
		return
	}
	t, err := h.Svc.Store.CreateTemplate(r.Context(), p.toTemplate(school, ""))
	if err != nil {
		writeStoreErr(w, err, "template")
		return
	}
	h.auditEvent(r, c, school, "wa_template", t.ID, "create", nil, t)
	common.WriteJSONMsg(w, http.StatusCreated, t, nil, "Template disimpan")
}

// UpdateTemplate handles PUT /wa-templates/{id}.
func (h *Handler) UpdateTemplate(w http.ResponseWriter, r *http.Request) {
	school, c, ok := schoolOf(w, r)
	if !ok {
		return
	}
	var p templatePayload
	if !common.Decode(w, r, &p) {
		return
	}
	if msg := p.validate(); msg != "" {
		common.WriteError(w, http.StatusUnprocessableEntity, msg)
		return
	}
	before, err := h.Svc.Store.GetTemplate(r.Context(), school, chi.URLParam(r, "id"))
	if err != nil {
		writeStoreErr(w, err, "template")
		return
	}
	t, err := h.Svc.Store.UpdateTemplate(r.Context(), p.toTemplate(school, before.ID))
	if err != nil {
		writeStoreErr(w, err, "template")
		return
	}
	h.auditEvent(r, c, school, "wa_template", t.ID, "update", before, t)
	common.WriteJSONMsg(w, http.StatusOK, t, nil, "Template diperbarui")
}

// DeleteTemplate handles DELETE /wa-templates/{id}.
func (h *Handler) DeleteTemplate(w http.ResponseWriter, r *http.Request) {
	school, c, ok := schoolOf(w, r)
	if !ok {
		return
	}
	id := chi.URLParam(r, "id")
	if err := h.Svc.Store.DeleteTemplate(r.Context(), school, id); err != nil {
		writeStoreErr(w, err, "template")
		return
	}
	h.auditEvent(r, c, school, "wa_template", id, "delete", nil, nil)
	common.WriteJSONMsg(w, http.StatusOK, map[string]string{"id": id}, nil, "Template dihapus")
}

// ---- campaigns --------------------------------------------------------------------

type campaignPayload struct {
	Name         string            `json:"name"`
	SegmentID    string            `json:"segment_id"`
	Channel      string            `json:"channel"`
	TemplateID   string            `json:"template_id"`
	TemplateVars map[string]string `json:"template_vars"`
	Subject      string            `json:"subject"`
	Body         string            `json:"body"`
}

// buildCampaign validates a payload against the school's segment/template and
// the WhatsApp adapter state. It returns (status, message) on failure.
func (h *Handler) buildCampaign(r *http.Request, school string, p campaignPayload) (Campaign, int, string) {
	c := Campaign{SchoolID: school, Name: strings.TrimSpace(p.Name), SegmentID: p.SegmentID, Channel: p.Channel, TemplateVars: p.TemplateVars}
	if c.TemplateVars == nil {
		c.TemplateVars = map[string]string{}
	}
	if c.Name == "" || c.SegmentID == "" {
		return c, http.StatusUnprocessableEntity, "nama kampanye dan segmen wajib diisi"
	}
	if !ValidChannels[c.Channel] {
		return c, http.StatusUnprocessableEntity, "kanal tidak dikenal (whatsapp|email|in_app)"
	}
	if _, err := h.Svc.Store.GetSegment(r.Context(), school, c.SegmentID); err != nil {
		return c, http.StatusUnprocessableEntity, "segmen tidak ditemukan"
	}
	switch c.Channel {
	case ChannelWhatsApp:
		if h.Svc.WhatsApp == nil {
			return c, http.StatusConflict, provider.ErrWhatsAppDisabled.Error()
		}
		if p.TemplateID == "" {
			return c, http.StatusUnprocessableEntity, "template WhatsApp wajib dipilih"
		}
		tpl, err := h.Svc.Store.GetTemplate(r.Context(), school, p.TemplateID)
		if err != nil {
			return c, http.StatusUnprocessableEntity, "template WhatsApp tidak ditemukan"
		}
		if _, err := RenderTemplate(Template{ApprovalStatus: tpl.ApprovalStatus, Variables: tpl.Variables, Body: tpl.BodyPreview}, c.TemplateVars); err != nil {
			return c, http.StatusUnprocessableEntity, err.Error()
		}
		id := tpl.ID
		c.TemplateID = &id
	case ChannelEmail:
		if strings.TrimSpace(p.Subject) == "" || strings.TrimSpace(p.Body) == "" {
			return c, http.StatusUnprocessableEntity, "subjek dan isi email wajib diisi"
		}
	default:
		if strings.TrimSpace(p.Subject) == "" {
			return c, http.StatusUnprocessableEntity, "judul notifikasi wajib diisi"
		}
	}
	if s := strings.TrimSpace(p.Subject); s != "" {
		c.Subject = &s
	}
	if b := p.Body; strings.TrimSpace(b) != "" {
		c.Body = &b
	}
	return c, 0, ""
}

// ListCampaigns handles GET /campaigns?status=&page=&per_page=.
func (h *Handler) ListCampaigns(w http.ResponseWriter, r *http.Request) {
	school, _, ok := schoolOf(w, r)
	if !ok {
		return
	}
	page, perPage := common.Page(r)
	list, total, err := h.Svc.Store.ListCampaigns(r.Context(), school, r.URL.Query().Get("status"), page, perPage)
	if err != nil {
		writeStoreErr(w, err, "kampanye")
		return
	}
	common.WriteJSON(w, http.StatusOK, list, common.NewPageMeta(page, perPage, total))
}

// GetCampaign handles GET /campaigns/{id}.
func (h *Handler) GetCampaign(w http.ResponseWriter, r *http.Request) {
	school, _, ok := schoolOf(w, r)
	if !ok {
		return
	}
	c, err := h.Svc.Store.GetCampaign(r.Context(), school, chi.URLParam(r, "id"))
	if err != nil {
		writeStoreErr(w, err, "kampanye")
		return
	}
	common.WriteJSON(w, http.StatusOK, c, nil)
}

// CreateCampaign handles POST /campaigns (draft).
func (h *Handler) CreateCampaign(w http.ResponseWriter, r *http.Request) {
	school, cl, ok := schoolOf(w, r)
	if !ok {
		return
	}
	var p campaignPayload
	if !common.Decode(w, r, &p) {
		return
	}
	c, status, msg := h.buildCampaign(r, school, p)
	if status != 0 {
		common.WriteError(w, status, msg)
		return
	}
	c.CreatedBy = cl.UserID
	out, err := h.Svc.Store.CreateCampaign(r.Context(), c)
	if err != nil {
		writeStoreErr(w, err, "kampanye")
		return
	}
	h.auditEvent(r, cl, school, "campaign", out.ID, "create", nil, out)
	common.WriteJSONMsg(w, http.StatusCreated, out, nil, "Kampanye disimpan sebagai draf")
}

// UpdateCampaign handles PUT /campaigns/{id} (draft only).
func (h *Handler) UpdateCampaign(w http.ResponseWriter, r *http.Request) {
	school, cl, ok := schoolOf(w, r)
	if !ok {
		return
	}
	var p campaignPayload
	if !common.Decode(w, r, &p) {
		return
	}
	before, err := h.Svc.Store.GetCampaign(r.Context(), school, chi.URLParam(r, "id"))
	if err != nil {
		writeStoreErr(w, err, "kampanye")
		return
	}
	if before.Status != StatusDraft {
		common.WriteError(w, http.StatusConflict, "hanya kampanye draf yang dapat diubah")
		return
	}
	c, status, msg := h.buildCampaign(r, school, p)
	if status != 0 {
		common.WriteError(w, status, msg)
		return
	}
	c.ID = before.ID
	out, err := h.Svc.Store.UpdateCampaign(r.Context(), c)
	if err != nil {
		writeStoreErr(w, err, "kampanye")
		return
	}
	h.auditEvent(r, cl, school, "campaign", out.ID, "update", before, out)
	common.WriteJSONMsg(w, http.StatusOK, out, nil, "Kampanye diperbarui")
}

// DeleteCampaign handles DELETE /campaigns/{id} (draft without recipients).
func (h *Handler) DeleteCampaign(w http.ResponseWriter, r *http.Request) {
	school, cl, ok := schoolOf(w, r)
	if !ok {
		return
	}
	id := chi.URLParam(r, "id")
	if err := h.Svc.Store.DeleteDraftCampaign(r.Context(), school, id); err != nil {
		if errors.Is(err, ErrNotFound) {
			common.WriteError(w, http.StatusConflict, "hanya draf yang belum pernah dikirim yang dapat dihapus")
			return
		}
		writeStoreErr(w, err, "kampanye")
		return
	}
	h.auditEvent(r, cl, school, "campaign", id, "delete", nil, nil)
	common.WriteJSONMsg(w, http.StatusOK, map[string]string{"id": id}, nil, "Draf kampanye dihapus")
}

// ScheduleCampaign handles POST /campaigns/{id}/schedule {scheduled_at}.
func (h *Handler) ScheduleCampaign(w http.ResponseWriter, r *http.Request) {
	school, cl, ok := schoolOf(w, r)
	if !ok {
		return
	}
	var p struct {
		ScheduledAt time.Time `json:"scheduled_at"`
	}
	if !common.Decode(w, r, &p) {
		return
	}
	if p.ScheduledAt.IsZero() || p.ScheduledAt.Before(h.Svc.now().Add(-time.Minute)) {
		common.WriteError(w, http.StatusUnprocessableEntity, "waktu jadwal wajib diisi dan tidak boleh di masa lalu")
		return
	}
	if !h.whatsAppReady(w, r, school) {
		return
	}
	c, err := h.Svc.Store.ScheduleCampaign(r.Context(), school, chi.URLParam(r, "id"), p.ScheduledAt)
	if errors.Is(err, ErrNotFound) {
		common.WriteError(w, http.StatusConflict, "kampanye tidak dalam status draf/terjadwal")
		return
	}
	if err != nil {
		writeStoreErr(w, err, "kampanye")
		return
	}
	h.auditEvent(r, cl, school, "campaign", c.ID, "schedule", nil, map[string]any{"scheduled_at": c.ScheduledAt})
	common.WriteJSONMsg(w, http.StatusOK, c, nil, "Kampanye dijadwalkan")
}

// whatsAppReady rejects acting on a WhatsApp campaign while the adapter is off.
func (h *Handler) whatsAppReady(w http.ResponseWriter, r *http.Request, school string) bool {
	c, err := h.Svc.Store.GetCampaign(r.Context(), school, chi.URLParam(r, "id"))
	if err != nil {
		writeStoreErr(w, err, "kampanye")
		return false
	}
	if c.Channel == ChannelWhatsApp && h.Svc.WhatsApp == nil {
		common.WriteError(w, http.StatusConflict, provider.ErrWhatsAppDisabled.Error())
		return false
	}
	return true
}

// SendNow handles POST /campaigns/{id}/send-now: schedule at now and dispatch
// synchronously.
func (h *Handler) SendNow(w http.ResponseWriter, r *http.Request) {
	school, cl, ok := schoolOf(w, r)
	if !ok {
		return
	}
	if !h.whatsAppReady(w, r, school) {
		return
	}
	id := chi.URLParam(r, "id")
	if _, err := h.Svc.Store.ScheduleCampaign(r.Context(), school, id, h.Svc.now()); err != nil {
		if errors.Is(err, ErrNotFound) {
			common.WriteError(w, http.StatusConflict, "kampanye tidak dalam status draf/terjadwal")
			return
		}
		writeStoreErr(w, err, "kampanye")
		return
	}
	h.auditEvent(r, cl, school, "campaign", id, "send_now", nil, nil)
	if err := h.Svc.Dispatch(r.Context(), id); err != nil && !errors.Is(err, ErrNotDispatchable) {
		common.CaptureError(err)
		common.WriteError(w, http.StatusBadGateway, "pengiriman kampanye gagal: "+err.Error())
		return
	}
	c, err := h.Svc.Store.GetCampaign(r.Context(), school, id)
	if err != nil {
		writeStoreErr(w, err, "kampanye")
		return
	}
	common.WriteJSONMsg(w, http.StatusOK, c, nil, "Kampanye dikirim")
}

// CancelCampaign handles POST /campaigns/{id}/cancel.
func (h *Handler) CancelCampaign(w http.ResponseWriter, r *http.Request) {
	school, cl, ok := schoolOf(w, r)
	if !ok {
		return
	}
	c, err := h.Svc.Store.CancelCampaign(r.Context(), school, chi.URLParam(r, "id"))
	if errors.Is(err, ErrNotFound) {
		common.WriteError(w, http.StatusConflict, "hanya kampanye draf/terjadwal yang dapat dibatalkan")
		return
	}
	if err != nil {
		writeStoreErr(w, err, "kampanye")
		return
	}
	h.auditEvent(r, cl, school, "campaign", c.ID, "cancel", nil, nil)
	common.WriteJSONMsg(w, http.StatusOK, c, nil, "Kampanye dibatalkan")
}

// Stats handles GET /campaigns/{id}/stats (FR-147-004).
func (h *Handler) Stats(w http.ResponseWriter, r *http.Request) {
	school, _, ok := schoolOf(w, r)
	if !ok {
		return
	}
	c, err := h.Svc.Store.GetCampaign(r.Context(), school, chi.URLParam(r, "id"))
	if err != nil {
		writeStoreErr(w, err, "kampanye")
		return
	}
	if err := h.Svc.Store.SyncInAppReads(r.Context(), c.ID); err != nil {
		writeStoreErr(w, err, "statistik")
		return
	}
	states, err := h.Svc.Store.RecipientStates(r.Context(), c.ID)
	if err != nil {
		writeStoreErr(w, err, "statistik")
		return
	}
	common.WriteJSON(w, http.StatusOK, ComputeStats(states), map[string]any{"generated_at": h.Svc.now()})
}

// Recipients handles GET /campaigns/{id}/recipients?status=.
func (h *Handler) Recipients(w http.ResponseWriter, r *http.Request) {
	school, _, ok := schoolOf(w, r)
	if !ok {
		return
	}
	c, err := h.Svc.Store.GetCampaign(r.Context(), school, chi.URLParam(r, "id"))
	if err != nil {
		writeStoreErr(w, err, "kampanye")
		return
	}
	if err := h.Svc.Store.SyncInAppReads(r.Context(), c.ID); err != nil {
		writeStoreErr(w, err, "penerima")
		return
	}
	page, perPage := common.Page(r)
	list, total, err := h.Svc.Store.ListRecipients(r.Context(), c.ID, r.URL.Query().Get("status"), page, perPage)
	if err != nil {
		writeStoreErr(w, err, "penerima")
		return
	}
	common.WriteJSON(w, http.StatusOK, list, common.NewPageMeta(page, perPage, total))
}

// Channels handles GET /campaign-channels: which channels are usable now.
func (h *Handler) Channels(w http.ResponseWriter, _ *http.Request) {
	common.WriteJSON(w, http.StatusOK, map[string]bool{
		ChannelWhatsApp: h.Svc.WhatsApp != nil,
		ChannelEmail:    h.Svc.Email != nil,
		ChannelInApp:    true,
	}, nil)
}

// ---- settings --------------------------------------------------------------------

// GetSettings handles GET /campaign-settings.
func (h *Handler) GetSettings(w http.ResponseWriter, r *http.Request) {
	school, _, ok := schoolOf(w, r)
	if !ok {
		return
	}
	q, err := h.Svc.Store.MonthlyWAQuota(r.Context(), school)
	if err != nil {
		writeStoreErr(w, err, "pengaturan")
		return
	}
	used, err := h.Svc.Store.WAUsedThisMonth(r.Context(), school, h.Svc.now(), h.Svc.Loc)
	if err != nil {
		writeStoreErr(w, err, "pengaturan")
		return
	}
	common.WriteJSON(w, http.StatusOK, map[string]any{"monthly_wa_quota": q, "wa_used_this_month": used}, nil)
}

// PutSettings handles PUT /campaign-settings {monthly_wa_quota|null}.
func (h *Handler) PutSettings(w http.ResponseWriter, r *http.Request) {
	school, cl, ok := schoolOf(w, r)
	if !ok {
		return
	}
	var p struct {
		MonthlyWAQuota *int `json:"monthly_wa_quota"`
	}
	if !common.Decode(w, r, &p) {
		return
	}
	if p.MonthlyWAQuota != nil && *p.MonthlyWAQuota < 0 {
		common.WriteError(w, http.StatusUnprocessableEntity, "kuota tidak boleh negatif")
		return
	}
	before, _ := h.Svc.Store.MonthlyWAQuota(r.Context(), school)
	if err := h.Svc.Store.SetMonthlyWAQuota(r.Context(), school, p.MonthlyWAQuota); err != nil {
		writeStoreErr(w, err, "pengaturan")
		return
	}
	h.auditEvent(r, cl, school, "campaign_settings", school, "update", map[string]any{"monthly_wa_quota": before}, map[string]any{"monthly_wa_quota": p.MonthlyWAQuota})
	common.WriteJSONMsg(w, http.StatusOK, map[string]any{"monthly_wa_quota": p.MonthlyWAQuota}, nil, "Pengaturan kampanye disimpan")
}
