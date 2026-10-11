package payroll

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"

	"skci/hr/internal/common"
)

type Handler struct {
	Store *Store
}

func NewHandler(s *Store) *Handler {
	return &Handler{Store: s}
}

// -----------------------------------------------------------------------------
// Pay Components
// -----------------------------------------------------------------------------

func (h *Handler) ListComponents(w http.ResponseWriter, r *http.Request) {
	schoolID := common.EffectiveSchoolID(r)
	list, err := h.Store.ListComponents(r.Context(), schoolID)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "gagal memuat komponen gaji: "+err.Error())
		return
	}
	common.WriteJSON(w, http.StatusOK, list, nil)
}

func (h *Handler) CreateComponent(w http.ResponseWriter, r *http.Request) {
	schoolID := common.EffectiveSchoolID(r)
	var c PayComponent
	if err := json.NewDecoder(r.Body).Decode(&c); err != nil {
		common.WriteError(w, http.StatusBadRequest, "format json tidak valid")
		return
	}
	c.SchoolID = schoolID

	created, err := h.Store.CreateComponent(r.Context(), c)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "gagal membuat komponen: "+err.Error())
		return
	}
	common.WriteJSON(w, http.StatusCreated, created, nil)
}

// -----------------------------------------------------------------------------
// Payroll Runs
// -----------------------------------------------------------------------------

func (h *Handler) ListRuns(w http.ResponseWriter, r *http.Request) {
	schoolID := common.EffectiveSchoolID(r)
	list, err := h.Store.ListRuns(r.Context(), schoolID)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "gagal memuat payroll run: "+err.Error())
		return
	}
	common.WriteJSON(w, http.StatusOK, list, nil)
}

func (h *Handler) GetRun(w http.ResponseWriter, r *http.Request) {
	schoolID := common.EffectiveSchoolID(r)
	id := chi.URLParam(r, "id")

	run, err := h.Store.GetRun(r.Context(), schoolID, id)
	if err != nil {
		common.WriteError(w, http.StatusNotFound, "payroll run tidak ditemukan")
		return
	}
	common.WriteJSON(w, http.StatusOK, run, nil)
}

func (h *Handler) CreateRun(w http.ResponseWriter, r *http.Request) {
	schoolID := common.EffectiveSchoolID(r)
	var run PayrollRun
	if err := json.NewDecoder(r.Body).Decode(&run); err != nil {
		common.WriteError(w, http.StatusBadRequest, "format json tidak valid")
		return
	}
	run.SchoolID = schoolID

	created, err := h.Store.CreateRun(r.Context(), run)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "gagal membuat payroll run: "+err.Error())
		return
	}
	common.WriteJSON(w, http.StatusCreated, created, nil)
}

func (h *Handler) CalculateRun(w http.ResponseWriter, r *http.Request) {
	schoolID := common.EffectiveSchoolID(r)
	id := chi.URLParam(r, "id")

	if err := h.Store.CalculateRun(r.Context(), schoolID, id); err != nil {
		common.WriteError(w, http.StatusInternalServerError, "gagal menghitung payroll: "+err.Error())
		return
	}
	common.WriteJSON(w, http.StatusOK, map[string]string{"message": "payroll berhasil dihitung"}, nil)
}

func (h *Handler) SubmitRun(w http.ResponseWriter, r *http.Request) {
	schoolID := common.EffectiveSchoolID(r)
	id := chi.URLParam(r, "id")

	if err := h.Store.SubmitRun(r.Context(), schoolID, id); err != nil {
		common.WriteError(w, http.StatusInternalServerError, "gagal mengajukan approval: "+err.Error())
		return
	}
	common.WriteJSON(w, http.StatusOK, map[string]string{"message": "payroll diajukan untuk persetujuan"}, nil)
}

func (h *Handler) FinalizeRun(w http.ResponseWriter, r *http.Request) {
	claims := common.FromCtx(r)
	schoolID := common.EffectiveSchoolID(r)
	id := chi.URLParam(r, "id")

	userID := ""
	if claims != nil {
		userID = claims.UserID
	}

	if err := h.Store.FinalizeRun(r.Context(), schoolID, id, userID); err != nil {
		common.WriteError(w, http.StatusInternalServerError, "gagal finalisasi payroll: "+err.Error())
		return
	}
	common.WriteJSON(w, http.StatusOK, map[string]string{"message": "payroll telah difinalisasi, slip dibuat dan jurnal dibukukan"}, nil)
}

// -----------------------------------------------------------------------------
// Lines & Payslips
// -----------------------------------------------------------------------------

func (h *Handler) ListLines(w http.ResponseWriter, r *http.Request) {
	schoolID := common.EffectiveSchoolID(r)
	id := chi.URLParam(r, "id")
	flagged := r.URL.Query().Get("flagged") == "true"

	lines, err := h.Store.ListLines(r.Context(), schoolID, id, flagged)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "gagal memuat rincian payroll: "+err.Error())
		return
	}
	common.WriteJSON(w, http.StatusOK, lines, nil)
}

func (h *Handler) MyPayslips(w http.ResponseWriter, r *http.Request) {
	claims := common.FromCtx(r)
	if claims == nil {
		common.WriteError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	schoolID := common.EffectiveSchoolID(r)

	list, err := h.Store.ListPayslips(r.Context(), schoolID, claims.UserID)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "gagal memuat slip gaji: "+err.Error())
		return
	}
	common.WriteJSON(w, http.StatusOK, list, nil)
}

func (h *Handler) ListPayslips(w http.ResponseWriter, r *http.Request) {
	schoolID := common.EffectiveSchoolID(r)
	empID := r.URL.Query().Get("employee_id")

	list, err := h.Store.ListPayslips(r.Context(), schoolID, empID)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "gagal memuat slip gaji: "+err.Error())
		return
	}
	common.WriteJSON(w, http.StatusOK, list, nil)
}

func (h *Handler) ListJournals(w http.ResponseWriter, r *http.Request) {
	schoolID := common.EffectiveSchoolID(r)
	list, err := h.Store.ListJournals(r.Context(), schoolID)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "gagal memuat jurnal payroll: "+err.Error())
		return
	}
	common.WriteJSON(w, http.StatusOK, list, nil)
}
