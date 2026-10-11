package handler

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	"skci/hr/internal/common"
	"skci/hr/internal/model"
	"skci/hr/internal/store"
)

// --- DEPARTMENTS ---

func (a *App) ListDepartments(w http.ResponseWriter, r *http.Request) {
	schoolID := a.effectiveSchoolID(r)
	depts, err := store.ListDepartments(r.Context(), a.DB, schoolID)
	if err != nil {
		common.Error(w, http.StatusInternalServerError, "Gagal mengambil daftar departemen")
		return
	}
	common.JSON(w, http.StatusOK, depts)
}

func (a *App) CreateDepartment(w http.ResponseWriter, r *http.Request) {
	schoolID := a.effectiveSchoolID(r)
	var dept model.Department
	if err := decodeJSON(r, &dept); err != nil {
		common.Error(w, http.StatusBadRequest, "Format JSON tidak valid")
		return
	}
	if strings.TrimSpace(dept.Code) == "" || strings.TrimSpace(dept.Name) == "" {
		common.Error(w, http.StatusBadRequest, "Kode dan nama departemen wajib diisi")
		return
	}
	dept.SchoolID = schoolID

	if err := store.CreateDepartment(r.Context(), a.DB, &dept); err != nil {
		common.Error(w, http.StatusInternalServerError, "Gagal membuat departemen: "+err.Error())
		return
	}
	common.JSON(w, http.StatusCreated, dept)
}

func (a *App) UpdateDepartment(w http.ResponseWriter, r *http.Request) {
	schoolID := a.effectiveSchoolID(r)
	id := chi.URLParam(r, "id")

	var dept model.Department
	if err := decodeJSON(r, &dept); err != nil {
		common.Error(w, http.StatusBadRequest, "Format JSON tidak valid")
		return
	}
	dept.ID = id
	dept.SchoolID = schoolID

	if err := store.UpdateDepartment(r.Context(), a.DB, &dept); err != nil {
		common.Error(w, http.StatusInternalServerError, "Gagal memperbarui departemen")
		return
	}
	common.JSON(w, http.StatusOK, dept)
}

func (a *App) DeleteDepartment(w http.ResponseWriter, r *http.Request) {
	schoolID := a.effectiveSchoolID(r)
	id := chi.URLParam(r, "id")

	if err := store.DeleteDepartment(r.Context(), a.DB, schoolID, id); err != nil {
		common.Error(w, http.StatusInternalServerError, "Gagal menghapus departemen")
		return
	}
	common.JSON(w, http.StatusOK, map[string]string{"message": "Departemen berhasil dihapus"})
}

// --- POSITIONS ---

func (a *App) ListPositions(w http.ResponseWriter, r *http.Request) {
	schoolID := a.effectiveSchoolID(r)
	deptID := r.URL.Query().Get("department_id")

	positions, err := store.ListPositions(r.Context(), a.DB, schoolID, deptID)
	if err != nil {
		common.Error(w, http.StatusInternalServerError, "Gagal mengambil daftar jabatan")
		return
	}
	common.JSON(w, http.StatusOK, positions)
}

func (a *App) CreatePosition(w http.ResponseWriter, r *http.Request) {
	schoolID := a.effectiveSchoolID(r)
	var pos model.Position
	if err := decodeJSON(r, &pos); err != nil {
		common.Error(w, http.StatusBadRequest, "Format JSON tidak valid")
		return
	}
	if strings.TrimSpace(pos.Code) == "" || strings.TrimSpace(pos.Name) == "" {
		common.Error(w, http.StatusBadRequest, "Kode dan nama jabatan wajib diisi")
		return
	}
	pos.SchoolID = schoolID

	if err := store.CreatePosition(r.Context(), a.DB, &pos); err != nil {
		common.Error(w, http.StatusInternalServerError, "Gagal membuat jabatan: "+err.Error())
		return
	}
	common.JSON(w, http.StatusCreated, pos)
}

func (a *App) UpdatePosition(w http.ResponseWriter, r *http.Request) {
	schoolID := a.effectiveSchoolID(r)
	id := chi.URLParam(r, "id")

	var pos model.Position
	if err := decodeJSON(r, &pos); err != nil {
		common.Error(w, http.StatusBadRequest, "Format JSON tidak valid")
		return
	}
	pos.ID = id
	pos.SchoolID = schoolID

	if err := store.UpdatePosition(r.Context(), a.DB, &pos); err != nil {
		common.Error(w, http.StatusInternalServerError, "Gagal memperbarui jabatan")
		return
	}
	common.JSON(w, http.StatusOK, pos)
}

func (a *App) DeletePosition(w http.ResponseWriter, r *http.Request) {
	schoolID := a.effectiveSchoolID(r)
	id := chi.URLParam(r, "id")

	if err := store.DeletePosition(r.Context(), a.DB, schoolID, id); err != nil {
		common.Error(w, http.StatusInternalServerError, "Gagal menghapus jabatan")
		return
	}
	common.JSON(w, http.StatusOK, map[string]string{"message": "Jabatan berhasil dihapus"})
}

// --- ORG CHART ---

func (a *App) GetOrgChart(w http.ResponseWriter, r *http.Request) {
	schoolID := a.effectiveSchoolID(r)
	chart, err := store.GetOrgChart(r.Context(), a.DB, schoolID)
	if err != nil {
		common.Error(w, http.StatusInternalServerError, "Gagal memuat bagan organisasi")
		return
	}
	common.JSON(w, http.StatusOK, chart)
}

// --- HEADCOUNT ---

func (a *App) ListHeadcountPlans(w http.ResponseWriter, r *http.Request) {
	schoolID := a.effectiveSchoolID(r)
	year := 0
	if y := r.URL.Query().Get("year"); y != "" {
		if val, err := strconv.Atoi(y); err == nil {
			year = val
		}
	}

	plans, err := store.ListHeadcountPlans(r.Context(), a.DB, schoolID, year)
	if err != nil {
		common.Error(w, http.StatusInternalServerError, "Gagal memuat rencana kuota karyawan")
		return
	}
	common.JSON(w, http.StatusOK, plans)
}

func (a *App) CreateHeadcountPlan(w http.ResponseWriter, r *http.Request) {
	schoolID := a.effectiveSchoolID(r)
	var hp model.HeadcountPlan
	if err := decodeJSON(r, &hp); err != nil {
		common.Error(w, http.StatusBadRequest, "Format JSON tidak valid")
		return
	}
	if hp.PositionID == "" || hp.Year == 0 || hp.PlannedCount < 0 {
		common.Error(w, http.StatusBadRequest, "Data rencana formasi tidak lengkap")
		return
	}
	hp.SchoolID = schoolID

	if err := store.CreateHeadcountPlan(r.Context(), a.DB, &hp); err != nil {
		common.Error(w, http.StatusInternalServerError, "Gagal menyimpan rencana formasi: "+err.Error())
		return
	}
	common.JSON(w, http.StatusCreated, hp)
}

func (a *App) GetHeadcountOverview(w http.ResponseWriter, r *http.Request) {
	schoolID := a.effectiveSchoolID(r)
	year := 0
	if y := r.URL.Query().Get("year"); y != "" {
		if val, err := strconv.Atoi(y); err == nil {
			year = val
		}
	}

	overview, err := store.GetHeadcountOverview(r.Context(), a.DB, schoolID, year)
	if err != nil {
		common.Error(w, http.StatusInternalServerError, "Gagal memuat data komparasi formasi")
		return
	}
	common.JSON(w, http.StatusOK, overview)
}

// Internal: GET /internal/headcount/{positionId}
func (a *App) InternalGetHeadcountByPosition(w http.ResponseWriter, r *http.Request) {
	posID := chi.URLParam(r, "positionId")
	schoolID := r.URL.Query().Get("school_id")

	planned, actual, available, err := store.GetPositionHeadcount(r.Context(), a.DB, schoolID, posID)
	if err != nil {
		common.Error(w, http.StatusInternalServerError, "Gagal memuat data headcount internal")
		return
	}

	common.JSON(w, http.StatusOK, map[string]any{
		"position_id": posID,
		"planned":     planned,
		"actual":      actual,
		"available":   available,
	})
}
