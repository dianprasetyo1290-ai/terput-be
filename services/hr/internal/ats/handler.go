package ats

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"

	"github.com/go-chi/chi/v5"

	"skci/hr/internal/client"
	"skci/hr/internal/common"
)

type Handler struct {
	Store     *Store
	Approvals *client.Client
}

func NewHandler(s *Store, approvals *client.Client) *Handler {
	return &Handler{Store: s, Approvals: approvals}
}

// -----------------------------------------------------------------------------
// Requisitions Handlers
// -----------------------------------------------------------------------------

func (h *Handler) ListRequisitions(w http.ResponseWriter, r *http.Request) {
	schoolID := common.EffectiveSchoolID(r)
	list, err := h.Store.ListRequisitions(r.Context(), schoolID)
	if err != nil {
		common.InternalError(w, "gagal memuat permintaan rekrutmen", err)
		return
	}
	common.WriteJSON(w, http.StatusOK, list, nil)
}

func (h *Handler) CreateRequisition(w http.ResponseWriter, r *http.Request) {
	claims := common.FromCtx(r)
	if claims == nil {
		common.WriteError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	schoolID := common.EffectiveSchoolID(r)

	var req Requisition
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		common.WriteError(w, http.StatusBadRequest, "format json tidak valid")
		return
	}
	req.SchoolID = schoolID
	req.RequestedBy = claims.UserID

	created, err := h.Store.CreateRequisition(r.Context(), req)
	if err != nil {
		common.InternalError(w, "gagal membuat permintaan rekrutmen", err)
		return
	}
	common.WriteJSON(w, http.StatusCreated, created, nil)
}

// SubmitRequisition opens core approval requisition (flag exceeds_headcount
// when the request is above the vacant headcount, AC-88-01) and only then
// marks the draft pending_approval.
func (h *Handler) SubmitRequisition(w http.ResponseWriter, r *http.Request) {
	claims := common.FromCtx(r)
	if claims == nil {
		common.WriteError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	schoolID := common.EffectiveSchoolID(r)
	id := chi.URLParam(r, "id")

	req, err := h.Store.GetRequisition(r.Context(), schoolID, id)
	if err != nil {
		common.WriteError(w, http.StatusNotFound, "permintaan rekrutmen tidak ditemukan")
		return
	}
	if req.Status != "draft" {
		common.WriteError(w, http.StatusConflict, "hanya permintaan berstatus draft yang dapat diajukan")
		return
	}
	var flags []string
	if req.ExceedsHeadcount {
		flags = append(flags, "exceeds_headcount")
	}
	appID, err := h.Approvals.CreateApproval(r.Context(), client.ApprovalRequest{
		SchoolID:    schoolID,
		RequestType: client.ReqTypeRequisition,
		SourceID:    req.ID,
		Title:       fmt.Sprintf("Permintaan %d tenaga kerja: %s", req.Quantity, req.PositionName),
		Summary:     map[string]any{"position": req.PositionName, "quantity": req.Quantity, "reason": req.Reason},
		AmountIDR:   req.BudgetIDR,
		Flags:       flags,
		MakerUserID: claims.UserID,
	})
	if err != nil {
		log.Printf("requisition %s: create approval: %v", req.ID, err)
		status, msg := client.ApprovalErrorMessage(err)
		common.WriteError(w, status, msg)
		return
	}

	if err := h.Store.SubmitRequisition(r.Context(), schoolID, id, appID); err != nil {
		common.InternalError(w, "gagal mengajukan persetujuan", err)
		return
	}
	common.WriteJSON(w, http.StatusOK, map[string]string{"message": "permintaan diajukan untuk persetujuan", "approval_id": appID}, nil)
}

// -----------------------------------------------------------------------------
// Job Postings Handlers
// -----------------------------------------------------------------------------

func (h *Handler) ListPostings(w http.ResponseWriter, r *http.Request) {
	schoolID := common.EffectiveSchoolID(r)
	status := r.URL.Query().Get("status")

	list, err := h.Store.ListPostings(r.Context(), schoolID, status)
	if err != nil {
		common.InternalError(w, "gagal memuat lowongan kerja", err)
		return
	}
	common.WriteJSON(w, http.StatusOK, list, nil)
}

func (h *Handler) GetPosting(w http.ResponseWriter, r *http.Request) {
	schoolID := common.EffectiveSchoolID(r)
	id := chi.URLParam(r, "id")

	p, err := h.Store.GetPosting(r.Context(), schoolID, id)
	if err != nil {
		common.WriteError(w, http.StatusNotFound, "lowongan tidak ditemukan")
		return
	}
	common.WriteJSON(w, http.StatusOK, p, nil)
}

func (h *Handler) CreatePosting(w http.ResponseWriter, r *http.Request) {
	schoolID := common.EffectiveSchoolID(r)
	var p JobPosting
	if err := json.NewDecoder(r.Body).Decode(&p); err != nil {
		common.WriteError(w, http.StatusBadRequest, "format json tidak valid")
		return
	}
	p.SchoolID = schoolID

	created, err := h.Store.CreatePosting(r.Context(), p)
	if err != nil {
		common.InternalError(w, "gagal membuat lowongan", err)
		return
	}
	common.WriteJSON(w, http.StatusCreated, created, nil)
}

func (h *Handler) PublishPosting(w http.ResponseWriter, r *http.Request) {
	schoolID := common.EffectiveSchoolID(r)
	id := chi.URLParam(r, "id")

	if err := h.Store.PublishPosting(r.Context(), schoolID, id); err != nil {
		common.InternalError(w, "gagal mempublikasikan lowongan", err)
		return
	}
	common.WriteJSON(w, http.StatusOK, map[string]string{"message": "lowongan berhasil dipublikasikan"}, nil)
}

func (h *Handler) ClosePosting(w http.ResponseWriter, r *http.Request) {
	schoolID := common.EffectiveSchoolID(r)
	id := chi.URLParam(r, "id")
	var body struct {
		Reason string `json:"reason"`
	}
	_ = json.NewDecoder(r.Body).Decode(&body)

	if err := h.Store.ClosePosting(r.Context(), schoolID, id, body.Reason); err != nil {
		common.InternalError(w, "gagal menutup lowongan", err)
		return
	}
	common.WriteJSON(w, http.StatusOK, map[string]string{"message": "lowongan berhasil ditutup"}, nil)
}

// -----------------------------------------------------------------------------
// Public Careers & Candidates
// -----------------------------------------------------------------------------

func (h *Handler) PublicListCareers(w http.ResponseWriter, r *http.Request) {
	schoolID := chi.URLParam(r, "schoolSlug") // or school ID passed
	list, err := h.Store.ListPostings(r.Context(), schoolID, "open")
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "gagal memuat info karier")
		return
	}
	common.WriteJSON(w, http.StatusOK, list, nil)
}

func (h *Handler) PublicGetPosting(w http.ResponseWriter, r *http.Request) {
	schoolID := chi.URLParam(r, "schoolSlug")
	slug := chi.URLParam(r, "postingSlug")

	p, err := h.Store.GetPostingBySlug(r.Context(), schoolID, slug)
	if err != nil {
		common.WriteError(w, http.StatusNotFound, "lowongan tidak ditemukan atau sudah ditutup")
		return
	}
	common.WriteJSON(w, http.StatusOK, p, nil)
}

func (h *Handler) PublicApply(w http.ResponseWriter, r *http.Request) {
	schoolID := chi.URLParam(r, "schoolSlug")
	postingSlug := chi.URLParam(r, "postingSlug")

	p, err := h.Store.GetPostingBySlug(r.Context(), schoolID, postingSlug)
	if err != nil {
		common.WriteError(w, http.StatusNotFound, "lowongan tidak ditemukan")
		return
	}

	var body struct {
		FullName          string `json:"full_name"`
		Email             string `json:"email"`
		CVObjectKey       string `json:"cv_object_key"`
		ConsentTalentPool bool   `json:"consent_talent_pool"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		common.WriteError(w, http.StatusBadRequest, "format json tidak valid")
		return
	}
	if body.FullName == "" || body.Email == "" {
		common.WriteError(w, http.StatusBadRequest, "nama lengkap dan email wajib diisi")
		return
	}

	app, token, err := h.Store.ApplyCandidate(
		r.Context(), schoolID, p.ID, body.FullName, body.Email, body.CVObjectKey, body.ConsentTalentPool,
	)
	if err != nil {
		if err == ErrConflict {
			common.WriteError(w, http.StatusConflict, "Anda sudah melamar pada lowongan ini sebelumnya")
			return
		}
		common.InternalError(w, "gagal mengirimkan lamaran", err)
		return
	}

	common.WriteJSON(w, http.StatusCreated, map[string]interface{}{
		"application":  app,
		"status_token": token,
	}, nil)
}

func (h *Handler) PublicApplicationStatus(w http.ResponseWriter, r *http.Request) {
	token := r.URL.Query().Get("token")
	if token == "" {
		common.WriteError(w, http.StatusBadRequest, "token tidak valid")
		return
	}

	app, err := h.Store.GetApplicationByToken(r.Context(), token)
	if err != nil {
		common.WriteError(w, http.StatusNotFound, "lamaran tidak ditemukan")
		return
	}
	common.WriteJSON(w, http.StatusOK, app, nil)
}

// -----------------------------------------------------------------------------
// Pipeline Board & Applications
// -----------------------------------------------------------------------------

func (h *Handler) ListApplications(w http.ResponseWriter, r *http.Request) {
	schoolID := common.EffectiveSchoolID(r)
	postingID := r.URL.Query().Get("posting_id")

	list, err := h.Store.ListApplications(r.Context(), schoolID, postingID)
	if err != nil {
		common.InternalError(w, "gagal memuat data pelamar", err)
		return
	}
	common.WriteJSON(w, http.StatusOK, list, nil)
}

func (h *Handler) MoveApplication(w http.ResponseWriter, r *http.Request) {
	claims := common.FromCtx(r)
	schoolID := common.EffectiveSchoolID(r)
	appID := chi.URLParam(r, "id")

	var body struct {
		ToStageID string `json:"to_stage_id"`
		Note      string `json:"note"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		common.WriteError(w, http.StatusBadRequest, "format json tidak valid")
		return
	}

	actorID := ""
	if claims != nil {
		actorID = claims.UserID
	}

	if err := h.Store.MoveApplication(r.Context(), schoolID, appID, body.ToStageID, body.Note, actorID); err != nil {
		common.InternalError(w, "gagal memindahkan tahap", err)
		return
	}
	common.WriteJSON(w, http.StatusOK, map[string]string{"message": "tahap berhasil diperbarui"}, nil)
}

func (h *Handler) ListStages(w http.ResponseWriter, r *http.Request) {
	schoolID := common.EffectiveSchoolID(r)
	stages, err := h.Store.EnsureDefaultStages(r.Context(), schoolID)
	if err != nil {
		common.InternalError(w, "gagal memuat tahapan seleksi", err)
		return
	}
	common.WriteJSON(w, http.StatusOK, stages, nil)
}

// -----------------------------------------------------------------------------
// Scorecard Templates & Interviews
// -----------------------------------------------------------------------------

func (h *Handler) ListScorecardTemplates(w http.ResponseWriter, r *http.Request) {
	schoolID := common.EffectiveSchoolID(r)
	list, err := h.Store.ListScorecardTemplates(r.Context(), schoolID)
	if err != nil {
		common.InternalError(w, "gagal memuat template asesmen", err)
		return
	}
	common.WriteJSON(w, http.StatusOK, list, nil)
}

func (h *Handler) CreateScorecardTemplate(w http.ResponseWriter, r *http.Request) {
	schoolID := common.EffectiveSchoolID(r)
	var t ScorecardTemplate
	if err := json.NewDecoder(r.Body).Decode(&t); err != nil {
		common.WriteError(w, http.StatusBadRequest, "format json tidak valid")
		return
	}
	t.SchoolID = schoolID

	created, err := h.Store.CreateScorecardTemplate(r.Context(), t)
	if err != nil {
		common.InternalError(w, "gagal membuat template asesmen", err)
		return
	}
	common.WriteJSON(w, http.StatusCreated, created, nil)
}

func (h *Handler) ScheduleInterview(w http.ResponseWriter, r *http.Request) {
	schoolID := common.EffectiveSchoolID(r)
	var body struct {
		Interview
		PanelUserIDs []string `json:"panel_user_ids"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		common.WriteError(w, http.StatusBadRequest, "format json tidak valid")
		return
	}
	body.Interview.SchoolID = schoolID

	inv, err := h.Store.ScheduleInterview(r.Context(), body.Interview, body.PanelUserIDs)
	if err != nil {
		common.InternalError(w, "gagal menjadwalkan wawancara", err)
		return
	}
	common.WriteJSON(w, http.StatusCreated, inv, nil)
}

func (h *Handler) MyInterviews(w http.ResponseWriter, r *http.Request) {
	claims := common.FromCtx(r)
	if claims == nil {
		common.WriteError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	schoolID := common.EffectiveSchoolID(r)

	list, err := h.Store.MyInterviews(r.Context(), schoolID, claims.UserID)
	if err != nil {
		common.InternalError(w, "gagal memuat jadwal wawancara", err)
		return
	}
	common.WriteJSON(w, http.StatusOK, list, nil)
}

// -----------------------------------------------------------------------------
// Offers Handlers
// -----------------------------------------------------------------------------

func (h *Handler) ListOffers(w http.ResponseWriter, r *http.Request) {
	schoolID := common.EffectiveSchoolID(r)
	appID := r.URL.Query().Get("application_id")
	list, err := h.Store.ListOffers(r.Context(), schoolID, appID)
	if err != nil {
		common.InternalError(w, "gagal memuat daftar penawaran", err)
		return
	}
	common.WriteJSON(w, http.StatusOK, list, nil)
}

func (h *Handler) CreateOffer(w http.ResponseWriter, r *http.Request) {
	schoolID := common.EffectiveSchoolID(r)
	var o Offer
	if err := json.NewDecoder(r.Body).Decode(&o); err != nil {
		common.WriteError(w, http.StatusBadRequest, "format json tidak valid")
		return
	}
	o.SchoolID = schoolID

	created, err := h.Store.CreateOffer(r.Context(), o)
	if err != nil {
		common.InternalError(w, "gagal membuat penawaran", err)
		return
	}
	common.WriteJSON(w, http.StatusCreated, created, nil)
}

func (h *Handler) SubmitOffer(w http.ResponseWriter, r *http.Request) {
	claims := common.FromCtx(r)
	if claims == nil {
		common.WriteError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	schoolID := common.EffectiveSchoolID(r)
	id := chi.URLParam(r, "id")

	off, err := h.Store.GetOffer(r.Context(), schoolID, id)
	if err != nil {
		common.WriteError(w, http.StatusNotFound, "penawaran kerja tidak ditemukan")
		return
	}
	if off.Status != "draft" {
		common.WriteError(w, http.StatusConflict, "hanya penawaran berstatus draft yang dapat diajukan")
		return
	}

	var flags []string
	if off.OutOfRange {
		flags = append(flags, "salary_out_of_range")
	}

	appID, err := h.Approvals.CreateApproval(r.Context(), client.ApprovalRequest{
		SchoolID:    schoolID,
		RequestType: client.ReqTypeOffer,
		SourceID:    off.ID,
		Title:       fmt.Sprintf("Penawaran Kerja: %s - %s", off.CandidateName, off.PositionName),
		Summary: map[string]any{
			"candidate_name":  off.CandidateName,
			"position_name":   off.PositionName,
			"start_date":      off.StartDate,
			"employment_type": off.EmploymentType,
			"base_salary_idr": off.BaseSalaryIDR,
			"out_of_range":    off.OutOfRange,
		},
		AmountIDR:   &off.BaseSalaryIDR,
		Flags:       flags,
		MakerUserID: claims.UserID,
	})
	if err != nil {
		log.Printf("offer %s: create approval: %v", off.ID, err)
		status, msg := client.ApprovalErrorMessage(err)
		common.WriteError(w, status, msg)
		return
	}

	if err := h.Store.SubmitOffer(r.Context(), schoolID, id, appID); err != nil {
		common.InternalError(w, "gagal mengajukan approval penawaran", err)
		return
	}
	common.WriteJSON(w, http.StatusOK, map[string]string{
		"message":     "penawaran diajukan untuk persetujuan",
		"approval_id": appID,
	}, nil)
}

func (h *Handler) SendOffer(w http.ResponseWriter, r *http.Request) {
	schoolID := common.EffectiveSchoolID(r)
	id := chi.URLParam(r, "id")

	off, err := h.Store.GetOffer(r.Context(), schoolID, id)
	if err != nil {
		common.WriteError(w, http.StatusNotFound, "penawaran kerja tidak ditemukan")
		return
	}
	if off.Status != "approved" {
		common.WriteError(w, http.StatusConflict, "penawaran harus disetujui (approved) sebelum dikirim ke kandidat")
		return
	}

	if err := h.Store.SendOffer(r.Context(), schoolID, id); err != nil {
		common.InternalError(w, "gagal mengirim penawaran", err)
		return
	}
	common.WriteJSON(w, http.StatusOK, map[string]string{
		"message": "penawaran kerja berhasil dikirim ke kandidat",
	}, nil)
}

func (h *Handler) PublicAcceptOffer(w http.ResponseWriter, r *http.Request) {
	offerID := chi.URLParam(r, "offerId")
	if err := h.Store.AcceptOffer(r.Context(), offerID); err != nil {
		common.InternalError(w, "gagal menerima penawaran", err)
		return
	}
	common.WriteJSON(w, http.StatusOK, map[string]string{"message": "penawaran kerja berhasil diterima"}, nil)
}

// -----------------------------------------------------------------------------
// Settings & Talent Pool
// -----------------------------------------------------------------------------

func (h *Handler) GetSettings(w http.ResponseWriter, r *http.Request) {
	schoolID := common.EffectiveSchoolID(r)
	st, err := h.Store.GetSettings(r.Context(), schoolID)
	if err != nil {
		common.InternalError(w, "gagal memuat pengaturan", err)
		return
	}
	common.WriteJSON(w, http.StatusOK, st, nil)
}

func (h *Handler) ListTalentPool(w http.ResponseWriter, r *http.Request) {
	schoolID := common.EffectiveSchoolID(r)
	list, err := h.Store.ListTalentPool(r.Context(), schoolID)
	if err != nil {
		common.InternalError(w, "gagal memuat talent pool", err)
		return
	}
	common.WriteJSON(w, http.StatusOK, list, nil)
}
