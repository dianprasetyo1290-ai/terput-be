package handler

import (
	"fmt"
	"net/http"
	"slices"
	"strings"

	"github.com/go-chi/chi/v5"

	"skci/core/internal/common"
	"skci/core/internal/model"
)

type createFlowRequest struct {
	RequestType string           `json:"request_type"`
	Name        string           `json:"name"`
	Active      bool             `json:"active"`
	Steps       []model.FlowStep `json:"steps"`
}

func (h *Handler) ListFlows(w http.ResponseWriter, r *http.Request) {
	claims := common.FromCtx(r)
	if claims == nil || claims.ActiveRole != common.RoleSuperadmin {
		common.WriteError(w, http.StatusForbidden, "hanya Super Admin yang dapat melihat alur approval")
		return
	}

	schoolID := common.EffectiveSchoolID(r)
	if schoolID == "" {
		common.WriteError(w, http.StatusBadRequest, "school_id diperlukan")
		return
	}

	flows, err := h.Store.ListFlows(r.Context(), schoolID)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "gagal memuat alur approval: "+err.Error())
		return
	}
	common.WriteJSON(w, http.StatusOK, flows, nil)
}

func (h *Handler) GetFlow(w http.ResponseWriter, r *http.Request) {
	claims := common.FromCtx(r)
	if claims == nil || claims.ActiveRole != common.RoleSuperadmin {
		common.WriteError(w, http.StatusForbidden, "hanya Super Admin yang dapat melihat alur approval")
		return
	}

	schoolID := common.EffectiveSchoolID(r)
	id := chi.URLParam(r, "id")
	flow, err := h.Store.GetFlow(r.Context(), schoolID, id)
	if err != nil {
		common.WriteError(w, http.StatusNotFound, "alur approval tidak ditemukan")
		return
	}
	common.WriteJSON(w, http.StatusOK, flow, nil)
}

func (h *Handler) GetRequestTypes(w http.ResponseWriter, r *http.Request) {
	type item struct {
		Type  string `json:"type"`
		Label string `json:"label"`
	}
	var res []item
	for _, t := range model.KnownRequestTypes {
		label := model.RequestTypeLabels[t]
		if label == "" {
			label = t
		}
		res = append(res, item{Type: t, Label: label})
	}
	common.WriteJSON(w, http.StatusOK, res, nil)
}

func (h *Handler) CreateFlow(w http.ResponseWriter, r *http.Request) {
	claims := common.FromCtx(r)
	if claims == nil || claims.ActiveRole != common.RoleSuperadmin {
		common.WriteError(w, http.StatusForbidden, "hanya Super Admin yang dapat membuat alur approval")
		return
	}

	schoolID := common.EffectiveSchoolID(r)
	if schoolID == "" {
		common.WriteError(w, http.StatusBadRequest, "school_id diperlukan")
		return
	}

	var req createFlowRequest
	if !common.Decode(w, r, &req) {
		return
	}

	if err := validateFlow(req.RequestType, req.Name, req.Steps); err != nil {
		common.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}

	flow := model.ApprovalFlow{
		SchoolID:    schoolID,
		RequestType: req.RequestType,
		Name:        req.Name,
		Active:      req.Active,
		CreatedBy:   claims.GetUserID(),
		Steps:       req.Steps,
	}

	created, err := h.Store.CreateFlow(r.Context(), schoolID, claims.GetUserID(), flow)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "gagal membuat alur approval: "+err.Error())
		return
	}

	_ = h.Store.InsertAudit(r.Context(), model.AuditEvent{
		SchoolID:    &schoolID,
		Service:     "core",
		Module:      "approval_flow",
		EntityType:  "approval_flow",
		EntityID:    created.ID,
		Action:      "flow_created",
		ActorUserID: ptr(claims.GetUserID()),
		ActorRole:   ptr(claims.ActiveRole),
		AfterData:   created,
		IPHash:      HashIP(ClientIP(r)),
	})

	common.WriteJSON(w, http.StatusCreated, created, nil)
}

func (h *Handler) UpdateFlow(w http.ResponseWriter, r *http.Request) {
	claims := common.FromCtx(r)
	if claims == nil || claims.ActiveRole != common.RoleSuperadmin {
		common.WriteError(w, http.StatusForbidden, "hanya Super Admin yang dapat mengubah alur approval")
		return
	}

	schoolID := common.EffectiveSchoolID(r)
	id := chi.URLParam(r, "id")
	if id == "" {
		common.WriteError(w, http.StatusBadRequest, "id alur diperlukan")
		return
	}

	var req createFlowRequest
	if !common.Decode(w, r, &req) {
		return
	}

	if err := validateFlow(req.RequestType, req.Name, req.Steps); err != nil {
		common.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}

	flow := model.ApprovalFlow{
		SchoolID:    schoolID,
		RequestType: req.RequestType,
		Name:        req.Name,
		Active:      req.Active,
		Steps:       req.Steps,
	}

	updated, err := h.Store.UpdateFlow(r.Context(), schoolID, id, flow)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "gagal mengubah alur approval: "+err.Error())
		return
	}

	_ = h.Store.InsertAudit(r.Context(), model.AuditEvent{
		SchoolID:    &schoolID,
		Service:     "core",
		Module:      "approval_flow",
		EntityType:  "approval_flow",
		EntityID:    updated.ID,
		Action:      "flow_updated",
		ActorUserID: ptr(claims.GetUserID()),
		ActorRole:   ptr(claims.ActiveRole),
		AfterData:   updated,
		IPHash:      HashIP(ClientIP(r)),
	})

	common.WriteJSON(w, http.StatusOK, updated, nil)
}

func (h *Handler) DeleteFlow(w http.ResponseWriter, r *http.Request) {
	claims := common.FromCtx(r)
	if claims == nil || claims.ActiveRole != common.RoleSuperadmin {
		common.WriteError(w, http.StatusForbidden, "hanya Super Admin yang dapat menghapus alur approval")
		return
	}

	schoolID := common.EffectiveSchoolID(r)
	id := chi.URLParam(r, "id")
	if id == "" {
		common.WriteError(w, http.StatusBadRequest, "id alur diperlukan")
		return
	}

	if err := h.Store.DeleteFlow(r.Context(), schoolID, id); err != nil {
		common.WriteError(w, http.StatusInternalServerError, "gagal menghapus alur approval: "+err.Error())
		return
	}

	_ = h.Store.InsertAudit(r.Context(), model.AuditEvent{
		SchoolID:    &schoolID,
		Service:     "core",
		Module:      "approval_flow",
		EntityType:  "approval_flow",
		EntityID:    id,
		Action:      "flow_deleted",
		ActorUserID: ptr(claims.GetUserID()),
		ActorRole:   ptr(claims.ActiveRole),
		IPHash:      HashIP(ClientIP(r)),
	})

	common.WriteJSON(w, http.StatusOK, map[string]string{"message": "alur approval berhasil dihapus"}, nil)
}

func validateFlow(requestType, name string, steps []model.FlowStep) error {
	if strings.TrimSpace(name) == "" {
		return fmt.Errorf("nama alur wajib diisi")
	}
	if !slices.Contains(model.KnownRequestTypes, requestType) {
		return fmt.Errorf("jenis permintaan %q tidak dikenal", requestType)
	}
	if len(steps) == 0 {
		return fmt.Errorf("alur approval harus memiliki minimal satu tahap")
	}

	for i, st := range steps {
		expectedSeq := i + 1
		if st.Seq != expectedSeq {
			return fmt.Errorf("urutan tahap (seq) harus berurutan mulai dari 1 (ditemukan %d pada posisi %d)", st.Seq, expectedSeq)
		}
		switch st.ApproverKind {
		case model.ApproverKindRole:
			if st.ApproverRole == nil || strings.TrimSpace(*st.ApproverRole) == "" {
				return fmt.Errorf("tahap %d: peran penyetuju (approver_role) wajib diisi untuk jenis 'role'", st.Seq)
			}
		case model.ApproverKindUser:
			if st.ApproverUserID == nil || strings.TrimSpace(*st.ApproverUserID) == "" {
				return fmt.Errorf("tahap %d: user_id penyetuju (approver_user_id) wajib diisi untuk jenis 'user'", st.Seq)
			}
		case model.ApproverKindDirectSupervisor:
			// No extra parameter needed
		default:
			return fmt.Errorf("tahap %d: jenis penyetuju %q tidak valid", st.Seq, st.ApproverKind)
		}

		if st.SLAHours != nil && *st.SLAHours <= 0 {
			return fmt.Errorf("tahap %d: SLA hours harus bernilai positif", st.Seq)
		}
	}
	return model.ValidateAmountThresholds(steps)
}
