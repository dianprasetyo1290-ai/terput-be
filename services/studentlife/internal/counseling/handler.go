package counseling

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"skci/studentlife/internal/client"
	"skci/studentlife/internal/common"
	"skci/studentlife/internal/config"
	"skci/studentlife/internal/consent"
	"skci/studentlife/internal/sensitive"
	"skci/studentlife/internal/store"
)

type Handler struct {
	DB     *sql.DB
	Store  *Store
	Crypto *sensitive.Service
	Client *client.Client
	Cfg    config.Config
}

func NewHandler(db *sql.DB, cr *sensitive.Service, cl *client.Client, cfg config.Config) *Handler {
	return &Handler{
		DB:     db,
		Store:  NewStore(db),
		Crypto: cr,
		Client: cl,
		Cfg:    cfg,
	}
}

// ConsentSubject registers T20 hook for generic consent handler (AC-132-01).
func (h *Handler) ConsentSubject() consent.Subject {
	return consent.Subject{
		Feature: common.FeatureCounseling,
		Describe: func(ctx context.Context, schoolID, subjectID string) (map[string]any, error) {
			ref, err := h.Store.GetReferral(ctx, schoolID, subjectID)
			if err != nil {
				return map[string]any{"title": "Rujukan Konseling", "details": "Rujukan layanan bimbingan konseling"}, nil
			}
			aad := sensitive.AAD("referrals", ref.SchoolID, ref.ID)
			targetName, _ := h.Crypto.Decrypt(ref.TargetNameEnc, aad)
			parentSum, _ := h.Crypto.Decrypt(ref.ParentSummaryEnc, aad)

			// Log access by guardian (NFR-OPS-03)
			_ = sensitive.LogAccess(ctx, h.DB, sensitive.AccessEntry{
				SchoolID:   ref.SchoolID,
				Module:     sensitive.ModuleCounseling,
				EntityType: "referral",
				EntityID:   ref.ID,
				StudentID:  ref.StudentID,
				ActorRole:  common.RoleParent,
				Action:     sensitive.ActionView,
				Reason:     "parent_consent_review",
			})

			return map[string]any{
				"title":          fmt.Sprintf("Rujukan Konseling (%s)", targetName),
				"details":        parentSum,
				"parent_summary": parentSum,
				"target_kind":    ref.TargetKind,
			}, nil
		},
		InUse: func(ctx context.Context, schoolID, consentID string) (bool, error) {
			ref, err := h.Store.GetReferralByConsentID(ctx, consentID)
			if err != nil {
				return false, nil
			}
			return ref.Status == ReferralStatusActive || ref.Status == ReferralStatusCompleted, nil
		},
		OnDecided: func(ctx context.Context, tx *sql.Tx, schoolID, subjectID, decision string) error {
			newStatus := ReferralInitialStatus(decision)
			_, err := tx.ExecContext(ctx, `
				UPDATE referrals
				SET status = $1, updated_at = now()
				WHERE id = $2 AND school_id = $3`, newStatus, subjectID, schoolID)
			return err
		},
	}
}

func clientIP(r *http.Request) string {
	ip, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return ip
}

// Categories
func (h *Handler) ListCategories(w http.ResponseWriter, r *http.Request) {
	claims := common.GetClaims(r.Context())
	list, err := h.Store.ListCategories(r.Context(), claims.SchoolID)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "Gagal memuat kategori konseling")
		return
	}
	common.WriteSuccess(w, list, nil)
}

func (h *Handler) CreateCategory(w http.ResponseWriter, r *http.Request) {
	claims := common.GetClaims(r.Context())
	if claims.ActiveRole != common.RoleKonselor && claims.ActiveRole != common.RoleAdmin && claims.ActiveRole != common.RoleSuperadmin {
		common.WriteError(w, http.StatusForbidden, "Akses ditolak")
		return
	}
	var req struct {
		Name   string `json:"name"`
		Active *bool  `json:"active"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || strings.TrimSpace(req.Name) == "" {
		common.WriteError(w, http.StatusBadRequest, "Nama kategori wajib diisi")
		return
	}
	active := true
	if req.Active != nil {
		active = *req.Active
	}
	cat, err := h.Store.CreateCategory(r.Context(), Category{
		SchoolID: claims.SchoolID,
		Name:     strings.TrimSpace(req.Name),
		Active:   active,
	})
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "Gagal membuat kategori konseling")
		return
	}
	common.WriteSuccess(w, cat, nil)
}

func (h *Handler) UpdateCategory(w http.ResponseWriter, r *http.Request) {
	claims := common.GetClaims(r.Context())
	id := chi.URLParam(r, "id")
	var req struct {
		Name   string `json:"name"`
		Active bool   `json:"active"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || strings.TrimSpace(req.Name) == "" {
		common.WriteError(w, http.StatusBadRequest, "Nama kategori tidak valid")
		return
	}
	cat, err := h.Store.UpdateCategory(r.Context(), Category{
		ID:       id,
		SchoolID: claims.SchoolID,
		Name:     strings.TrimSpace(req.Name),
		Active:   req.Active,
	})
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "Gagal memperbarui kategori")
		return
	}
	common.WriteSuccess(w, cat, nil)
}

func (h *Handler) DeleteCategory(w http.ResponseWriter, r *http.Request) {
	claims := common.GetClaims(r.Context())
	id := chi.URLParam(r, "id")
	if err := h.Store.DeleteCategory(r.Context(), claims.SchoolID, id); err != nil {
		common.WriteError(w, http.StatusInternalServerError, "Gagal menghapus kategori")
		return
	}
	common.WriteSuccess(w, map[string]string{"status": "deleted"}, nil)
}

// Cases
func (h *Handler) ListCases(w http.ResponseWriter, r *http.Request) {
	claims := common.GetClaims(r.Context())
	status := r.URL.Query().Get("status")
	studentID := r.URL.Query().Get("student_id")

	// Check if user has policy or is counselor/kesiswaan
	policies, _ := sensitive.ActivePolicyRoles(r.Context(), h.DB, claims.SchoolID, sensitive.ModuleCounseling)
	hasPolicy := false
	for _, p := range policies {
		if p == claims.ActiveRole {
			hasPolicy = true
			break
		}
	}

	memberUser := claims.UserID
	if hasPolicy {
		memberUser = "" // policy holders can see all cases
	}

	list, err := h.Store.ListCases(r.Context(), claims.SchoolID, status, studentID, memberUser)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "Gagal memuat daftar kasus konseling")
		return
	}
	common.WriteSuccess(w, list, nil)
}

func (h *Handler) GetCase(w http.ResponseWriter, r *http.Request) {
	claims := common.GetClaims(r.Context())
	id := chi.URLParam(r, "id")

	c, err := h.Store.GetCase(r.Context(), claims.SchoolID, id)
	if err != nil {
		common.WriteError(w, http.StatusNotFound, "Kasus konseling tidak ditemukan")
		return
	}

	// Access control (AC-130-01)
	isMember, _, _ := h.Store.IsCaseMember(r.Context(), id, claims.UserID)
	policies, _ := sensitive.ActivePolicyRoles(r.Context(), h.DB, claims.SchoolID, sensitive.ModuleCounseling)
	lvl := sensitive.CanAccess(sensitive.ModuleCounseling, claims.ActiveRole, sensitive.Facts{IsCaseMember: isMember}, policies)

	if lvl != sensitive.Full {
		_ = sensitive.LogAccess(r.Context(), h.DB, sensitive.AccessEntry{
			SchoolID: claims.SchoolID, Module: sensitive.ModuleCounseling, EntityType: "case",
			EntityID: id, StudentID: c.StudentID, ActorUserID: claims.UserID, ActorRole: claims.ActiveRole,
			Action: sensitive.ActionDeny, Reason: "unauthorized_case_access",
		})
		common.WriteError(w, http.StatusNotFound, "Kasus tidak ditemukan")
		return
	}

	// Decrypt close reason if present
	if len(c.CloseReasonEnc) > 0 {
		aad := sensitive.AAD("counseling_cases", c.SchoolID, c.ID)
		c.CloseReason, _ = h.Crypto.Decrypt(c.CloseReasonEnc, aad)
	}

	_ = sensitive.LogAccess(r.Context(), h.DB, sensitive.AccessEntry{
		SchoolID: claims.SchoolID, Module: sensitive.ModuleCounseling, EntityType: "case",
		EntityID: id, StudentID: c.StudentID, ActorUserID: claims.UserID, ActorRole: claims.ActiveRole,
		Action: sensitive.ActionView,
	})

	common.WriteSuccess(w, c, nil)
}

func (h *Handler) CreateCase(w http.ResponseWriter, r *http.Request) {
	claims := common.GetClaims(r.Context())
	if claims.ActiveRole != common.RoleKonselor && claims.ActiveRole != common.RoleKesiswaan && claims.ActiveRole != common.RoleAdmin && claims.ActiveRole != common.RoleSuperadmin {
		common.WriteError(w, http.StatusForbidden, "Akses ditolak")
		return
	}

	var req struct {
		StudentID          string `json:"student_id"`
		CategoryID         string `json:"category_id"`
		DiscloseToHomeroom bool   `json:"disclose_to_homeroom"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.StudentID == "" || req.CategoryID == "" {
		common.WriteError(w, http.StatusBadRequest, "Siswa dan kategori wajib diisi")
		return
	}

	memberRole := "counselor"
	if claims.ActiveRole == common.RoleKesiswaan {
		memberRole = "kesiswaan"
	}

	newCase, err := h.Store.CreateCase(r.Context(), Case{
		SchoolID:           claims.SchoolID,
		StudentID:          req.StudentID,
		CategoryID:         req.CategoryID,
		OpenedBy:           claims.UserID,
		DiscloseToHomeroom: req.DiscloseToHomeroom,
	}, memberRole)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "Gagal membuat kasus konseling")
		return
	}

	_ = sensitive.LogAccess(r.Context(), h.DB, sensitive.AccessEntry{
		SchoolID: claims.SchoolID, Module: sensitive.ModuleCounseling, EntityType: "case",
		EntityID: newCase.ID, StudentID: req.StudentID, ActorUserID: claims.UserID, ActorRole: claims.ActiveRole,
		Action: sensitive.ActionCreate,
	})

	common.WriteSuccess(w, newCase, nil)
}

func (h *Handler) UpdateCase(w http.ResponseWriter, r *http.Request) {
	claims := common.GetClaims(r.Context())
	id := chi.URLParam(r, "id")

	isMember, memberRole, err := h.Store.IsCaseMember(r.Context(), id, claims.UserID)
	if err != nil || !isMember {
		common.WriteError(w, http.StatusForbidden, "Hanya anggota penanganan kasus yang dapat memperbarui")
		return
	}

	var req struct {
		CategoryID         string `json:"category_id"`
		Status             string `json:"status"`
		DiscloseToHomeroom *bool  `json:"disclose_to_homeroom"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		common.WriteError(w, http.StatusBadRequest, "Data tidak valid")
		return
	}

	c, err := h.Store.GetCase(r.Context(), claims.SchoolID, id)
	if err != nil {
		common.WriteError(w, http.StatusNotFound, "Kasus tidak ditemukan")
		return
	}

	if req.CategoryID != "" {
		c.CategoryID = req.CategoryID
	}
	if req.Status != "" {
		c.Status = req.Status
	}
	// AC-130-01: disclose_to_homeroom hanya dapat diubah oleh konselor
	if req.DiscloseToHomeroom != nil {
		if memberRole != "counselor" && claims.ActiveRole != common.RoleKonselor {
			common.WriteError(w, http.StatusForbidden, "Hanya konselor yang dapat mengatur izin akses wali kelas")
			return
		}
		c.DiscloseToHomeroom = *req.DiscloseToHomeroom
	}

	if err := h.Store.UpdateCase(r.Context(), *c); err != nil {
		common.WriteError(w, http.StatusInternalServerError, "Gagal memperbarui kasus")
		return
	}

	common.WriteSuccess(w, c, nil)
}

func (h *Handler) CloseCase(w http.ResponseWriter, r *http.Request) {
	claims := common.GetClaims(r.Context())
	id := chi.URLParam(r, "id")

	isMember, memberRole, err := h.Store.IsCaseMember(r.Context(), id, claims.UserID)
	if err != nil || !isMember || memberRole != "counselor" {
		common.WriteError(w, http.StatusForbidden, "Hanya konselor penanganan kasus yang dapat menutup kasus")
		return
	}

	var req struct {
		CloseReason string `json:"close_reason"`
	}
	_ = json.NewDecoder(r.Body).Decode(&req)

	tasks, _ := h.Store.ListCaseTasks(r.Context(), claims.SchoolID, id)
	if !CanClose(tasks, req.CloseReason) {
		common.WriteError(w, http.StatusUnprocessableEntity, "Masih terdapat tugas tindak lanjut yang belum selesai. Harap selesaikan tugas atau berikan alasan penutupan kasus.")
		return
	}

	var closeReasonEnc []byte
	if strings.TrimSpace(req.CloseReason) != "" {
		aad := sensitive.AAD("counseling_cases", claims.SchoolID, id)
		closeReasonEnc, _ = h.Crypto.Encrypt(strings.TrimSpace(req.CloseReason), aad)
	}

	if err := h.Store.CloseCase(r.Context(), claims.SchoolID, id, closeReasonEnc, time.Now()); err != nil {
		common.WriteError(w, http.StatusInternalServerError, "Gagal menutup kasus")
		return
	}

	_ = sensitive.LogAccess(r.Context(), h.DB, sensitive.AccessEntry{
		SchoolID: claims.SchoolID, Module: sensitive.ModuleCounseling, EntityType: "case",
		EntityID: id, ActorUserID: claims.UserID, ActorRole: claims.ActiveRole,
		Action: sensitive.ActionUpdate, Reason: "close_case",
	})

	common.WriteSuccess(w, map[string]string{"status": "closed"}, nil)
}

func (h *Handler) AddCaseMember(w http.ResponseWriter, r *http.Request) {
	claims := common.GetClaims(r.Context())
	id := chi.URLParam(r, "id")

	isMember, memberRole, _ := h.Store.IsCaseMember(r.Context(), id, claims.UserID)
	if !isMember || memberRole != "counselor" {
		common.WriteError(w, http.StatusForbidden, "Hanya konselor anggota kasus yang dapat menambah anggota")
		return
	}

	var req struct {
		UserID string `json:"user_id"`
		Role   string `json:"role"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.UserID == "" {
		common.WriteError(w, http.StatusBadRequest, "User ID wajib diisi")
		return
	}
	if req.Role == "" {
		req.Role = "counselor"
	}

	if err := h.Store.AddCaseMember(r.Context(), id, req.UserID, req.Role, claims.UserID); err != nil {
		common.WriteError(w, http.StatusInternalServerError, "Gagal menambah anggota kasus")
		return
	}
	common.WriteSuccess(w, map[string]string{"status": "added"}, nil)
}

func (h *Handler) RemoveCaseMember(w http.ResponseWriter, r *http.Request) {
	claims := common.GetClaims(r.Context())
	id := chi.URLParam(r, "id")
	targetUserID := chi.URLParam(r, "userId")

	isMember, memberRole, _ := h.Store.IsCaseMember(r.Context(), id, claims.UserID)
	if !isMember || memberRole != "counselor" {
		common.WriteError(w, http.StatusForbidden, "Hanya konselor anggota kasus yang dapat menghapus anggota")
		return
	}

	if err := h.Store.RemoveCaseMember(r.Context(), id, targetUserID); err != nil {
		common.WriteError(w, http.StatusInternalServerError, "Gagal menghapus anggota kasus")
		return
	}
	common.WriteSuccess(w, map[string]string{"status": "removed"}, nil)
}

// Homeroom Student Status (AC-130-01)
func (h *Handler) GetStudentHomeroomStatus(w http.ResponseWriter, r *http.Request) {
	claims := common.GetClaims(r.Context())
	studentID := chi.URLParam(r, "id")

	// Homeroom check
	if claims.ActiveRole != common.RoleWalikelas && claims.ActiveRole != common.RoleAdmin && claims.ActiveRole != common.RoleSuperadmin {
		common.WriteError(w, http.StatusForbidden, "Akses ditolak")
		return
	}

	cases, err := h.Store.GetStudentCasesForHomeroom(r.Context(), claims.SchoolID, studentID)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "Gagal memeriksa status pendampingan")
		return
	}

	status := HomeroomStatus(cases)
	common.WriteSuccess(w, map[string]string{
		"student_id": studentID,
		"status":     status,
	}, nil)
}

// Sessions
func (h *Handler) ListSessions(w http.ResponseWriter, r *http.Request) {
	claims := common.GetClaims(r.Context())
	caseID := r.URL.Query().Get("case_id")
	counselorUser := claims.UserID
	if claims.ActiveRole == common.RoleAdmin || claims.ActiveRole == common.RoleSuperadmin {
		counselorUser = ""
	}

	list, err := h.Store.ListSessions(r.Context(), claims.SchoolID, counselorUser, caseID, nil, nil)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "Gagal memuat jadwal sesi")
		return
	}
	common.WriteSuccess(w, list, nil)
}

func (h *Handler) GetSession(w http.ResponseWriter, r *http.Request) {
	claims := common.GetClaims(r.Context())
	id := chi.URLParam(r, "id")

	sess, err := h.Store.GetSession(r.Context(), claims.SchoolID, id)
	if err != nil {
		common.WriteError(w, http.StatusNotFound, "Sesi tidak ditemukan")
		return
	}

	// Decrypt notes and follow-up plan
	aad := sensitive.AAD("counseling_sessions", sess.SchoolID, sess.ID)
	if len(sess.NotesEnc) > 0 {
		sess.Notes, _ = h.Crypto.Decrypt(sess.NotesEnc, aad)
	}
	if len(sess.FollowUpPlanEnc) > 0 {
		sess.FollowUpPlan, _ = h.Crypto.Decrypt(sess.FollowUpPlanEnc, aad)
	}

	_ = sensitive.LogAccess(r.Context(), h.DB, sensitive.AccessEntry{
		SchoolID: claims.SchoolID, Module: sensitive.ModuleCounseling, EntityType: "session",
		EntityID: id, ActorUserID: claims.UserID, ActorRole: claims.ActiveRole,
		Action: sensitive.ActionView,
	})

	common.WriteSuccess(w, sess, nil)
}

func (h *Handler) CreateSession(w http.ResponseWriter, r *http.Request) {
	claims := common.GetClaims(r.Context())
	var req struct {
		CaseID       *string  `json:"case_id"`
		Kind         string   `json:"kind"`
		CategoryID   string   `json:"category_id"`
		ScheduledAt  string   `json:"scheduled_at"`
		StudentIDs   []string `json:"student_ids"`
		Notes        string   `json:"notes"`
		FollowUpPlan string   `json:"follow_up_plan"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		common.WriteError(w, http.StatusBadRequest, "Permintaan tidak valid")
		return
	}

	if err := ValidateSession(req.Kind, req.StudentIDs, SessionStatusScheduled, nil); err != nil {
		common.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}

	parsedTime, err := time.Parse(time.RFC3339, req.ScheduledAt)
	if err != nil {
		common.WriteError(w, http.StatusBadRequest, "Waktu jadwal tidak valid (RFC3339)")
		return
	}

	sess := Session{
		SchoolID:        claims.SchoolID,
		CaseID:          req.CaseID,
		Kind:            req.Kind,
		CategoryID:      req.CategoryID,
		CounselorUserID: claims.UserID,
		ScheduledAt:     parsedTime,
		Status:          SessionStatusScheduled,
	}

	created, err := h.Store.CreateSession(r.Context(), sess, req.StudentIDs)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "Gagal membuat sesi konseling")
		return
	}

	// Encrypt notes if provided
	if req.Notes != "" || req.FollowUpPlan != "" {
		aad := sensitive.AAD("counseling_sessions", claims.SchoolID, created.ID)
		if req.Notes != "" {
			created.NotesEnc, _ = h.Crypto.Encrypt(req.Notes, aad)
		}
		if req.FollowUpPlan != "" {
			created.FollowUpPlanEnc, _ = h.Crypto.Encrypt(req.FollowUpPlan, aad)
		}
		_ = h.Store.UpdateSession(r.Context(), *created, nil)
	}

	_ = sensitive.LogAccess(r.Context(), h.DB, sensitive.AccessEntry{
		SchoolID: claims.SchoolID, Module: sensitive.ModuleCounseling, EntityType: "session",
		EntityID: created.ID, ActorUserID: claims.UserID, ActorRole: claims.ActiveRole,
		Action: sensitive.ActionCreate,
	})

	common.WriteSuccess(w, created, nil)
}

func (h *Handler) UpdateSession(w http.ResponseWriter, r *http.Request) {
	claims := common.GetClaims(r.Context())
	id := chi.URLParam(r, "id")

	sess, err := h.Store.GetSession(r.Context(), claims.SchoolID, id)
	if err != nil {
		common.WriteError(w, http.StatusNotFound, "Sesi tidak ditemukan")
		return
	}

	var req struct {
		CategoryID   string   `json:"category_id"`
		ScheduledAt  string   `json:"scheduled_at"`
		HeldAt       *string  `json:"held_at"`
		Status       string   `json:"status"`
		StudentIDs   []string `json:"student_ids"`
		Notes        *string  `json:"notes"`
		FollowUpPlan *string  `json:"follow_up_plan"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		common.WriteError(w, http.StatusBadRequest, "Format tidak valid")
		return
	}

	if req.CategoryID != "" {
		sess.CategoryID = req.CategoryID
	}
	if req.ScheduledAt != "" {
		if pt, err := time.Parse(time.RFC3339, req.ScheduledAt); err == nil {
			sess.ScheduledAt = pt
		}
	}
	if req.HeldAt != nil && *req.HeldAt != "" {
		if ht, err := time.Parse(time.RFC3339, *req.HeldAt); err == nil {
			sess.HeldAt = &ht
		}
	}
	if req.Status != "" {
		sess.Status = req.Status
	}

	if err := ValidateSession(sess.Kind, req.StudentIDs, sess.Status, sess.HeldAt); err != nil {
		common.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}

	aad := sensitive.AAD("counseling_sessions", claims.SchoolID, id)
	if req.Notes != nil {
		sess.NotesEnc, _ = h.Crypto.Encrypt(*req.Notes, aad)
	}
	if req.FollowUpPlan != nil {
		sess.FollowUpPlanEnc, _ = h.Crypto.Encrypt(*req.FollowUpPlan, aad)
	}

	if err := h.Store.UpdateSession(r.Context(), *sess, req.StudentIDs); err != nil {
		common.WriteError(w, http.StatusInternalServerError, "Gagal memperbarui sesi")
		return
	}

	common.WriteSuccess(w, sess, nil)
}

// Referrals
func (h *Handler) ListReferrals(w http.ResponseWriter, r *http.Request) {
	claims := common.GetClaims(r.Context())
	caseID := chi.URLParam(r, "id")

	list, err := h.Store.ListReferrals(r.Context(), claims.SchoolID, caseID)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "Gagal memuat rujukan")
		return
	}

	// Decrypt fields
	for i := range list {
		aad := sensitive.AAD("referrals", claims.SchoolID, list[i].ID)
		list[i].TargetName, _ = h.Crypto.Decrypt(list[i].TargetNameEnc, aad)
		list[i].TargetContact, _ = h.Crypto.Decrypt(list[i].TargetContactEnc, aad)
		list[i].Reason, _ = h.Crypto.Decrypt(list[i].ReasonEnc, aad)
		list[i].ParentSummary, _ = h.Crypto.Decrypt(list[i].ParentSummaryEnc, aad)
		list[i].Outcome, _ = h.Crypto.Decrypt(list[i].OutcomeEnc, aad)
	}

	_ = sensitive.LogAccess(r.Context(), h.DB, sensitive.AccessEntry{
		SchoolID: claims.SchoolID, Module: sensitive.ModuleCounseling, EntityType: "referral_list",
		EntityID: caseID, ActorUserID: claims.UserID, ActorRole: claims.ActiveRole,
		Action: sensitive.ActionView,
	})

	common.WriteSuccess(w, list, nil)
}

func (h *Handler) CreateReferral(w http.ResponseWriter, r *http.Request) {
	claims := common.GetClaims(r.Context())
	caseID := chi.URLParam(r, "id")

	c, err := h.Store.GetCase(r.Context(), claims.SchoolID, caseID)
	if err != nil {
		common.WriteError(w, http.StatusNotFound, "Kasus tidak ditemukan")
		return
	}

	var req struct {
		TargetKind    string `json:"target_kind"`
		TargetName    string `json:"target_name"`
		TargetContact string `json:"target_contact"`
		Reason        string `json:"reason"`
		ParentSummary string `json:"parent_summary"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.TargetName == "" || req.Reason == "" || req.ParentSummary == "" {
		common.WriteError(w, http.StatusBadRequest, "Target, alasan, dan ringkasan orang tua wajib diisi")
		return
	}

	refID := store.NewID()
	aad := sensitive.AAD("referrals", claims.SchoolID, refID)

	targetNameEnc, _ := h.Crypto.Encrypt(req.TargetName, aad)
	targetContactEnc, _ := h.Crypto.Encrypt(req.TargetContact, aad)
	reasonEnc, _ := h.Crypto.Encrypt(req.Reason, aad)
	parentSummaryEnc, _ := h.Crypto.Encrypt(req.ParentSummary, aad)

	// Create parent consent in pending status
	consentID := store.NewID()
	_, err = h.DB.ExecContext(r.Context(), `
		INSERT INTO parent_consents (id, school_id, subject_type, subject_id, student_id, status, requested_by)
		VALUES ($1, $2, $3, $4, $5, 'pending', $6)`,
		consentID, claims.SchoolID, ConsentSubjectReferral, refID, c.StudentID, claims.UserID)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "Gagal membuat persetujuan orang tua")
		return
	}

	created, err := h.Store.CreateReferral(r.Context(), Referral{
		ID:               refID,
		SchoolID:         claims.SchoolID,
		CaseID:           caseID,
		StudentID:        c.StudentID,
		TargetKind:       req.TargetKind,
		TargetNameEnc:    targetNameEnc,
		TargetContactEnc: targetContactEnc,
		ReasonEnc:        reasonEnc,
		ParentSummaryEnc: parentSummaryEnc,
		Status:           ReferralStatusAwaitingConsent,
		ConsentID:        &consentID,
		CreatedBy:        claims.UserID,
	})
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "Gagal membuat rujukan")
		return
	}

	// Notify student's guardians
	guardians, _ := h.Client.StudentGuardians(r.Context(), c.StudentID, claims.SchoolID)
	for _, g := range guardians {
		_ = h.Client.Notify(r.Context(), client.Notification{
			SchoolID: claims.SchoolID,
			UserID:   g.GuardianUserID,
			Type:     "counseling_referral_consent",
			Title:    "Persetujuan Rujukan Konseling",
			Body:     "Sekolah mengajukan rujukan pendampingan untuk Ananda. Mohon tinjau dan berikan persetujuan Anda.",
			Data:     map[string]any{"consent_id": consentID, "student_id": c.StudentID},
		}, "studentlife:consent:"+consentID+":"+g.GuardianUserID)
	}

	_ = sensitive.LogAccess(r.Context(), h.DB, sensitive.AccessEntry{
		SchoolID: claims.SchoolID, Module: sensitive.ModuleCounseling, EntityType: "referral",
		EntityID: created.ID, StudentID: c.StudentID, ActorUserID: claims.UserID, ActorRole: claims.ActiveRole,
		Action: sensitive.ActionCreate,
	})

	common.WriteSuccess(w, created, nil)
}

func (h *Handler) UpdateReferral(w http.ResponseWriter, r *http.Request) {
	claims := common.GetClaims(r.Context())
	refID := chi.URLParam(r, "id")

	var req struct {
		Status  string  `json:"status"`
		Outcome *string `json:"outcome"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		common.WriteError(w, http.StatusBadRequest, "Permintaan tidak valid")
		return
	}

	var outcomeEnc []byte
	if req.Outcome != nil {
		aad := sensitive.AAD("referrals", claims.SchoolID, refID)
		outcomeEnc, _ = h.Crypto.Encrypt(*req.Outcome, aad)
	}

	if err := h.Store.UpdateReferralStatus(r.Context(), claims.SchoolID, refID, req.Status, nil, outcomeEnc); err != nil {
		common.WriteError(w, http.StatusInternalServerError, "Gagal memperbarui status rujukan")
		return
	}

	common.WriteSuccess(w, map[string]string{"status": "updated"}, nil)
}

// Case Tasks
func (h *Handler) ListCaseTasks(w http.ResponseWriter, r *http.Request) {
	claims := common.GetClaims(r.Context())
	caseID := chi.URLParam(r, "id")

	list, err := h.Store.ListCaseTasks(r.Context(), claims.SchoolID, caseID)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "Gagal memuat tugas")
		return
	}

	today := time.Now().Format("2006-01-02")
	for i := range list {
		list[i].DisplayStatus = TaskDisplayStatus(list[i], today)
	}

	common.WriteSuccess(w, list, nil)
}

func (h *Handler) ListMyTasks(w http.ResponseWriter, r *http.Request) {
	claims := common.GetClaims(r.Context())
	list, err := h.Store.ListMyTasks(r.Context(), claims.SchoolID, claims.UserID)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "Gagal memuat tugas saya")
		return
	}

	today := time.Now().Format("2006-01-02")
	for i := range list {
		list[i].DisplayStatus = TaskDisplayStatus(list[i], today)
	}

	common.WriteSuccess(w, list, nil)
}

func (h *Handler) CreateCaseTask(w http.ResponseWriter, r *http.Request) {
	claims := common.GetClaims(r.Context())
	caseID := chi.URLParam(r, "id")

	var req struct {
		Title          string `json:"title"`
		AssigneeUserID string `json:"assignee_user_id"`
		DueDate        string `json:"due_date"`
		Note           string `json:"note"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Title == "" || req.AssigneeUserID == "" || req.DueDate == "" {
		common.WriteError(w, http.StatusBadRequest, "Judul, penerima tugas, dan batas waktu wajib diisi")
		return
	}

	taskID := store.NewID()
	var noteEnc []byte
	if req.Note != "" {
		aad := sensitive.AAD("case_tasks", claims.SchoolID, taskID)
		noteEnc, _ = h.Crypto.Encrypt(req.Note, aad)
	}

	task, err := h.Store.CreateTask(r.Context(), Task{
		ID:             taskID,
		SchoolID:       claims.SchoolID,
		CaseID:         caseID,
		Title:          strings.TrimSpace(req.Title),
		AssigneeUserID: req.AssigneeUserID,
		DueDate:        req.DueDate,
		NoteEnc:        noteEnc,
		CreatedBy:      claims.UserID,
	})
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "Gagal membuat tugas")
		return
	}

	common.WriteSuccess(w, task, nil)
}

func (h *Handler) MarkTaskDone(w http.ResponseWriter, r *http.Request) {
	claims := common.GetClaims(r.Context())
	taskID := chi.URLParam(r, "id")

	if err := h.Store.MarkTaskDone(r.Context(), common.EffectiveSchoolID(r), taskID, claims.UserID, time.Now()); err != nil {
		if errors.Is(err, ErrNotFound) {
			common.WriteError(w, http.StatusNotFound, "Tugas tidak ditemukan")
			return
		}
		common.WriteError(w, http.StatusInternalServerError, "Gagal menyelesaikan tugas: "+err.Error())
		return
	}
	common.WriteSuccess(w, map[string]string{"status": "done"}, nil)
}

// Settings
func (h *Handler) GetSettings(w http.ResponseWriter, r *http.Request) {
	claims := common.GetClaims(r.Context())
	st, err := h.Store.GetSettings(r.Context(), claims.SchoolID)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "Gagal memuat pengaturan")
		return
	}
	common.WriteSuccess(w, st, nil)
}

func (h *Handler) PutSettings(w http.ResponseWriter, r *http.Request) {
	claims := common.GetClaims(r.Context())
	if claims.ActiveRole != common.RoleKonselor && claims.ActiveRole != common.RoleAdmin && claims.ActiveRole != common.RoleSuperadmin {
		common.WriteError(w, http.StatusForbidden, "Akses ditolak")
		return
	}
	var req struct {
		ReminderDaysBefore int `json:"reminder_days_before"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.ReminderDaysBefore < 0 {
		common.WriteError(w, http.StatusBadRequest, "Hari pengingat harus >= 0")
		return
	}
	st, err := h.Store.PutSettings(r.Context(), Settings{
		SchoolID:           claims.SchoolID,
		ReminderDaysBefore: req.ReminderDaysBefore,
	})
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "Gagal menyimpan pengaturan")
		return
	}
	common.WriteSuccess(w, st, nil)
}
