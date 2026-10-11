package handler

import (
	"context"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"

	"skci/core/internal/common"
	"skci/core/internal/jobs"
	"skci/core/internal/model"
	"skci/core/internal/store"
)

// markCanDecide fills can_decide for each request from the caller's point of view.
func markCanDecide(reqs []model.ApprovalRequest, user model.UserInfo, dels []model.ActiveDelegation) {
	for i := range reqs {
		reqs[i].CanDecide = false
		if st := model.CurrentStep(&reqs[i]); st != nil {
			reqs[i].CanDecide, _ = model.CanDecide(reqs[i].MakerUserID, model.StepApprover(*st), user, dels)
		}
	}
}

func pageMeta(page, perPage, total int) map[string]any {
	return map[string]any{"page": page, "per_page": perPage, "total": total}
}

// ListInbox: requests whose current step names the caller or someone
// delegating to them. Items the caller made still show (can_decide=false,
// "Menunggu penyetuju lain" in the UI) when they match by role.
func (h *Handler) ListInbox(w http.ResponseWriter, r *http.Request) {
	claims, schoolID, ok := tenant(w, r)
	if !ok {
		return
	}
	status := r.URL.Query().Get("status")
	if status != "decided" {
		status = "pending"
	}
	page, perPage := common.Page(r)
	user := userInfo(claims)
	dels := h.delegationsFor(r.Context(), schoolID, user.UserID)

	f := store.InboxFilter{SchoolID: schoolID, UserID: user.UserID, Status: status,
		ActorIDs: []string{user.UserID}, ActorRoles: append([]string{claims.ActiveRole}, claims.Roles...)}
	for _, d := range dels {
		f.ActorIDs = append(f.ActorIDs, d.FromUserID)
		f.ActorRoles = append(f.ActorRoles, d.FromUserRoles...)
	}

	reqs, total, err := h.Store.ListInbox(r.Context(), f, page, perPage)
	if err != nil {
		writeStoreErr(w, err, "")
		return
	}
	markCanDecide(reqs, user, dels)
	common.WriteJSON(w, http.StatusOK, nonNil(reqs), pageMeta(page, perPage, total))
}

func (h *Handler) ListMine(w http.ResponseWriter, r *http.Request) {
	claims, schoolID, ok := tenant(w, r)
	if !ok {
		return
	}
	page, perPage := common.Page(r)
	reqs, total, err := h.Store.ListMine(r.Context(), schoolID, claims.GetUserID(), page, perPage)
	if err != nil {
		writeStoreErr(w, err, "")
		return
	}
	common.WriteJSON(w, http.StatusOK, nonNil(reqs), pageMeta(page, perPage, total))
}

// loadVisible fetches a request and enforces read access: admin/superadmin,
// or anyone model.CanView admits (maker, subject, approvers, delegates).
// Not visible is reported as 404 so ids of other people's requests don't leak.
func (h *Handler) loadVisible(w http.ResponseWriter, r *http.Request, claims *common.Claims, schoolID string,
	load func(ctx context.Context) (*model.ApprovalRequest, error)) (*model.ApprovalRequest, []model.ActiveDelegation, bool) {
	req, err := load(r.Context())
	if err != nil {
		writeStoreErr(w, err, "permintaan approval tidak ditemukan")
		return nil, nil, false
	}
	user := userInfo(claims)
	dels := h.delegationsFor(r.Context(), schoolID, user.UserID)
	if !isAdmin(claims) && !model.CanView(req, user, dels) {
		common.WriteError(w, http.StatusNotFound, "permintaan approval tidak ditemukan")
		return nil, nil, false
	}
	one := []model.ApprovalRequest{*req}
	markCanDecide(one, user, dels)
	return &one[0], dels, true
}

func (h *Handler) GetApproval(w http.ResponseWriter, r *http.Request) {
	claims, schoolID, ok := tenant(w, r)
	if !ok {
		return
	}
	id := chi.URLParam(r, "id")
	if !isUUID(id) {
		common.WriteError(w, http.StatusNotFound, "permintaan approval tidak ditemukan")
		return
	}
	req, _, ok := h.loadVisible(w, r, claims, schoolID, func(ctx context.Context) (*model.ApprovalRequest, error) {
		return h.Store.GetRequestByID(ctx, schoolID, id)
	})
	if ok {
		common.WriteJSON(w, http.StatusOK, req, nil)
	}
}

// GetApprovalBySource backs components/core/ApprovalTimeline.vue on a source
// transaction page (FR-79-007): the newest request for that transaction.
func (h *Handler) GetApprovalBySource(w http.ResponseWriter, r *http.Request) {
	claims, schoolID, ok := tenant(w, r)
	if !ok {
		return
	}
	svc, srcID := r.URL.Query().Get("source_service"), r.URL.Query().Get("source_id")
	if svc == "" || srcID == "" {
		common.WriteError(w, http.StatusBadRequest, "source_service dan source_id diperlukan")
		return
	}
	req, _, ok := h.loadVisible(w, r, claims, schoolID, func(ctx context.Context) (*model.ApprovalRequest, error) {
		return h.Store.GetRequestBySource(ctx, schoolID, svc, srcID)
	})
	if ok {
		common.WriteJSON(w, http.StatusOK, req, nil)
	}
}

type decideRequest struct {
	Comment string `json:"comment"`
}

func (h *Handler) Approve(w http.ResponseWriter, r *http.Request) {
	h.decide(w, r, model.StepStatusApproved)
}

func (h *Handler) Reject(w http.ResponseWriter, r *http.Request) {
	h.decide(w, r, model.StepStatusRejected)
}

// decide enforces BR-GEN-09 maker-checker (403, also through delegation),
// approver authority (403) and the mandatory reject comment (422), then
// applies the transition under a row lock (stale step -> 409).
func (h *Handler) decide(w http.ResponseWriter, r *http.Request, decision string) {
	claims, schoolID, ok := tenant(w, r)
	if !ok {
		return
	}
	var body decideRequest
	if r.ContentLength != 0 && !common.Decode(w, r, &body) {
		return
	}
	body.Comment = strings.TrimSpace(body.Comment)

	id := chi.URLParam(r, "id")
	if !isUUID(id) {
		common.WriteError(w, http.StatusNotFound, "permintaan approval tidak ditemukan")
		return
	}
	req, err := h.Store.GetRequestByID(r.Context(), schoolID, id)
	if err != nil {
		writeStoreErr(w, err, "permintaan approval tidak ditemukan")
		return
	}
	step := model.CurrentStep(req)
	if step == nil {
		common.WriteError(w, http.StatusConflict, "permintaan ini tidak sedang menunggu persetujuan")
		return
	}
	user := userInfo(claims)
	if user.UserID == req.MakerUserID {
		common.WriteError(w, http.StatusForbidden, "pembuat permintaan tidak dapat memutuskan permintaannya sendiri (maker-checker)")
		return
	}
	can, delegateOf := model.CanDecide(req.MakerUserID, model.StepApprover(*step), user, h.delegationsFor(r.Context(), schoolID, user.UserID))
	if !can {
		common.WriteError(w, http.StatusForbidden, "Anda tidak berwenang memutuskan tahap ini")
		return
	}
	if decision == model.StepStatusRejected && body.Comment == "" {
		common.WriteError(w, http.StatusUnprocessableEntity, "komentar penolakan wajib diisi")
		return
	}

	updated, err := h.Store.DecideStep(r.Context(), schoolID, id, step.Seq, decision, user.UserID, delegateOf, body.Comment)
	if err != nil {
		writeStoreErr(w, err, "permintaan approval tidak ditemukan")
		return
	}

	after := map[string]any{"seq": step.Seq, "decision": decision, "request_status": updated.Status}
	if delegateOf != nil {
		after["on_behalf_of"] = *delegateOf
	}
	h.audit(r, model.AuditEvent{
		SchoolID: &schoolID, Module: "approval", EntityType: "approval_request", EntityID: id,
		Action: decision, AfterData: after, Reason: strPtr(body.Comment), CorrelationID: &updated.CorrelationID,
	})
	h.bg(func(ctx context.Context) {
		jobs.AfterTransition(ctx, h.Store, h.Client, updated, h.Config.CallbackMaxAttempts)
	})
	common.WriteJSON(w, http.StatusOK, updated, nil)
}

// nonNil keeps empty lists as [] rather than null in the envelope.
func nonNil[T any](s []T) []T {
	if s == nil {
		return []T{}
	}
	return s
}
