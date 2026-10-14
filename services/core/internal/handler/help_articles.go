package handler

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	"skci/core/internal/common"
	"skci/core/internal/model"
)

// helpArticleRequest is the create/update body (BUG-165).
type helpArticleRequest struct {
	Audience   string  `json:"audience"`
	Kind       string  `json:"kind"`
	Title      string  `json:"title"`
	Body       string  `json:"body"`
	SortOrder  int     `json:"sort_order"`
	Published  bool    `json:"published"`
	ReplacesID *string `json:"replaces_id,omitempty"`
}

func (in helpArticleRequest) article() model.HelpArticle {
	return model.HelpArticle{Audience: in.Audience, Kind: in.Kind, Title: in.Title, Body: in.Body,
		SortOrder: in.SortOrder, Published: in.Published, ReplacesID: in.ReplacesID}
}

// readerAudience: parents read the parent guide, students the student guide;
// staff may preview either with ?audience=.
func readerAudience(r *http.Request, c *common.Claims) string {
	switch c.ActiveRole {
	case common.RoleParent:
		return model.HelpAudienceParent
	case common.RoleStudent:
		return model.HelpAudienceStudent
	}
	if r.URL.Query().Get("audience") == model.HelpAudienceStudent {
		return model.HelpAudienceStudent
	}
	return model.HelpAudienceParent
}

func helpAdmin(w http.ResponseWriter, c *common.Claims) bool {
	if c.ActiveRole != common.RoleAdmin && c.ActiveRole != common.RoleSuperadmin {
		common.WriteError(w, http.StatusForbidden, "hanya admin sekolah yang dapat mengelola panduan & FAQ")
		return false
	}
	return true
}

// ListHelpArticles handles GET /help-articles: the published guide & FAQ for
// the caller's audience (school articles plus the defaults not replaced).
func (h *Handler) ListHelpArticles(w http.ResponseWriter, r *http.Request) {
	claims, schoolID, ok := tenant(w, r)
	if !ok {
		return
	}
	rows, err := h.Store.ListHelpArticles(r.Context(), schoolID, readerAudience(r, claims))
	if err != nil {
		writeStoreErr(w, err, "")
		return
	}
	common.WriteJSON(w, http.StatusOK, model.MergeHelpArticles(rows, true), nil)
}

// ManageHelpArticles handles GET /help-articles/manage?audience=: every row
// including drafts and replaced defaults (admin).
func (h *Handler) ManageHelpArticles(w http.ResponseWriter, r *http.Request) {
	claims, schoolID, ok := tenant(w, r)
	if !ok || !helpAdmin(w, claims) {
		return
	}
	rows, err := h.Store.ListHelpArticles(r.Context(), schoolID, readerAudience(r, claims))
	if err != nil {
		writeStoreErr(w, err, "")
		return
	}
	common.WriteJSON(w, http.StatusOK, model.MergeHelpArticles(rows, false), nil)
}

// CreateHelpArticle handles POST /help-articles. replaces_id must name a
// platform default; the new article then replaces it (or hides it while
// unpublished).
func (h *Handler) CreateHelpArticle(w http.ResponseWriter, r *http.Request) {
	claims, schoolID, ok := tenant(w, r)
	if !ok || !helpAdmin(w, claims) {
		return
	}
	var in helpArticleRequest
	if !common.Decode(w, r, &in) {
		return
	}
	a := in.article()
	if err := model.ValidateHelpArticle(a); err != nil {
		common.WriteError(w, http.StatusUnprocessableEntity, err.Error())
		return
	}
	if a.ReplacesID != nil {
		def, err := h.Store.GetHelpArticle(r.Context(), schoolID, *a.ReplacesID)
		if err != nil || def.SchoolID != nil {
			common.WriteError(w, http.StatusUnprocessableEntity, "replaces_id harus merujuk artikel bawaan")
			return
		}
		a.Audience = def.Audience
	}
	a.SchoolID = &schoolID
	created, err := h.Store.CreateHelpArticle(r.Context(), a, claims.GetUserID())
	if err != nil {
		writeStoreErr(w, err, "")
		return
	}
	h.audit(r, model.AuditEvent{SchoolID: &schoolID, Module: "help_center", EntityType: "help_article",
		EntityID: created.ID, Action: "create", AfterData: created})
	common.WriteJSON(w, http.StatusCreated, created, nil)
}

// UpdateHelpArticle handles PUT /help-articles/{id} (school articles only).
func (h *Handler) UpdateHelpArticle(w http.ResponseWriter, r *http.Request) {
	claims, schoolID, ok := tenant(w, r)
	if !ok || !helpAdmin(w, claims) {
		return
	}
	id := chi.URLParam(r, "id")
	var in helpArticleRequest
	if !common.Decode(w, r, &in) {
		return
	}
	a := in.article()
	if err := model.ValidateHelpArticle(a); err != nil {
		common.WriteError(w, http.StatusUnprocessableEntity, err.Error())
		return
	}
	before, err := h.Store.GetHelpArticle(r.Context(), schoolID, id)
	if err == nil && before.SchoolID == nil {
		common.WriteError(w, http.StatusUnprocessableEntity, "artikel bawaan tidak dapat diubah langsung; buat pengganti untuk sekolah Anda")
		return
	}
	if err != nil {
		writeStoreErr(w, err, "artikel tidak ditemukan")
		return
	}
	if before.ReplacesID != nil {
		a.Audience = before.Audience
	}
	updated, err := h.Store.UpdateHelpArticle(r.Context(), schoolID, id, a, claims.GetUserID())
	if err != nil {
		writeStoreErr(w, err, "artikel tidak ditemukan")
		return
	}
	h.audit(r, model.AuditEvent{SchoolID: &schoolID, Module: "help_center", EntityType: "help_article",
		EntityID: id, Action: "update", BeforeData: before, AfterData: updated})
	common.WriteJSON(w, http.StatusOK, updated, nil)
}

// DeleteHelpArticle handles DELETE /help-articles/{id} (school articles only;
// deleting a replacement shows the default again).
func (h *Handler) DeleteHelpArticle(w http.ResponseWriter, r *http.Request) {
	claims, schoolID, ok := tenant(w, r)
	if !ok || !helpAdmin(w, claims) {
		return
	}
	id := chi.URLParam(r, "id")
	before, err := h.Store.GetHelpArticle(r.Context(), schoolID, id)
	if err == nil && before.SchoolID == nil {
		common.WriteError(w, http.StatusUnprocessableEntity, "artikel bawaan tidak dapat dihapus; sembunyikan dengan membuat pengganti yang tidak diterbitkan")
		return
	}
	if err == nil {
		err = h.Store.DeleteHelpArticle(r.Context(), schoolID, id)
	}
	if err != nil {
		writeStoreErr(w, err, "artikel tidak ditemukan")
		return
	}
	h.audit(r, model.AuditEvent{SchoolID: &schoolID, Module: "help_center", EntityType: "help_article",
		EntityID: id, Action: "delete", BeforeData: before})
	common.WriteJSON(w, http.StatusOK, map[string]string{"id": id}, nil)
}
