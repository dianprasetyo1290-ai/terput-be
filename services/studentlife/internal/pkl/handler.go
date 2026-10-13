package pkl

import (
	"context"
	"database/sql"
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"skci/studentlife/internal/client"
	"skci/studentlife/internal/common"
	"skci/studentlife/internal/config"
)

type Handler struct {
	DB          *sql.DB
	Store       *Store
	Client      *client.Client
	Cfg         config.Config
	StoragePath string
}

func NewHandler(db *sql.DB, s *Store, cl *client.Client, cfg config.Config) *Handler {
	storagePath := cfg.StoragePath
	if storagePath == "" {
		storagePath = "./data/storage"
	}
	_ = os.MkdirAll(filepath.Join(storagePath, "pkl", "mou"), 0755)
	_ = os.MkdirAll(filepath.Join(storagePath, "pkl", "journals"), 0755)
	_ = os.MkdirAll(filepath.Join(storagePath, "pkl", "reports"), 0755)
	_ = os.MkdirAll(filepath.Join(storagePath, "pkl", "leaves"), 0755)
	_ = os.MkdirAll(filepath.Join(storagePath, "pkl", "certificates"), 0755)

	return &Handler{
		DB:          db,
		Store:       s,
		Client:      cl,
		Cfg:         cfg,
		StoragePath: storagePath,
	}
}

func (h *Handler) audit(ctx context.Context, schoolID, entityType, entityID, action string, actorClaims *common.Claims, before, after map[string]any, reason string) {
	if h.Client == nil {
		return
	}
	var actorID, actorRole *string
	if actorClaims != nil {
		actorID = &actorClaims.UserID
		actorRole = &actorClaims.ActiveRole
	}
	var reasonPtr *string
	if reason != "" {
		reasonPtr = &reason
	}
	var beforeRaw, afterRaw json.RawMessage
	if before != nil {
		beforeRaw, _ = json.Marshal(before)
	}
	if after != nil {
		afterRaw, _ = json.Marshal(after)
	}
	_ = h.Client.PostAudit(ctx, client.AuditEvent{
		SchoolID:    schoolID,
		Service:     "studentlife",
		Module:      "pkl",
		EntityType:  entityType,
		EntityID:    entityID,
		Action:      action,
		ActorUserID: actorID,
		ActorRole:   actorRole,
		Before:      beforeRaw,
		After:       afterRaw,
		Reason:      reasonPtr,
	})
}

// ---------------------------------------------------------------------------
// 1. Majors & Classroom-Majors
// ---------------------------------------------------------------------------

func (h *Handler) ListMajors(w http.ResponseWriter, r *http.Request) {
	schoolID := common.EffectiveSchoolID(r)
	list, err := h.Store.ListMajors(r.Context(), schoolID)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "Gagal mengambil daftar jurusan")
		return
	}
	common.WriteSuccess(w, list, nil)
}

func (h *Handler) GetMajor(w http.ResponseWriter, r *http.Request) {
	schoolID := common.EffectiveSchoolID(r)
	id := chi.URLParam(r, "id")
	m, err := h.Store.GetMajor(r.Context(), schoolID, id)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			common.WriteError(w, http.StatusNotFound, "Jurusan tidak ditemukan")
			return
		}
		common.WriteError(w, http.StatusInternalServerError, "Gagal mengambil data jurusan")
		return
	}
	common.WriteSuccess(w, m, nil)
}

func (h *Handler) CreateMajor(w http.ResponseWriter, r *http.Request) {
	claims, schoolID := common.RequireRole(w, r, common.RoleKoordinatorPKL, common.RoleAdmin, common.RoleSuperadmin)
	if claims == nil {
		return
	}
	var body struct {
		Code   string `json:"code"`
		Name   string `json:"name"`
		Active *bool  `json:"active"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		common.WriteError(w, http.StatusBadRequest, "Body tidak valid")
		return
	}
	body.Code = strings.TrimSpace(body.Code)
	body.Name = strings.TrimSpace(body.Name)
	if body.Code == "" || body.Name == "" {
		common.WriteError(w, http.StatusBadRequest, "Kode dan nama jurusan wajib diisi")
		return
	}
	active := true
	if body.Active != nil {
		active = *body.Active
	}

	m, err := h.Store.CreateMajor(r.Context(), Major{
		SchoolID: schoolID,
		Code:     body.Code,
		Name:     body.Name,
		Active:   active,
	})
	if err != nil {
		if errors.Is(err, ErrAlreadyExists) {
			common.WriteError(w, http.StatusConflict, "Kode jurusan sudah terdaftar")
			return
		}
		common.WriteError(w, http.StatusInternalServerError, "Gagal menyimpan jurusan")
		return
	}
	h.audit(r.Context(), schoolID, "major", m.ID, "create", claims, nil, map[string]any{"code": m.Code, "name": m.Name}, "")
	common.WriteSuccess(w, m, nil)
}

func (h *Handler) UpdateMajor(w http.ResponseWriter, r *http.Request) {
	claims, schoolID := common.RequireRole(w, r, common.RoleKoordinatorPKL, common.RoleAdmin, common.RoleSuperadmin)
	if claims == nil {
		return
	}
	id := chi.URLParam(r, "id")
	var body struct {
		Code   string `json:"code"`
		Name   string `json:"name"`
		Active bool   `json:"active"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		common.WriteError(w, http.StatusBadRequest, "Body tidak valid")
		return
	}
	m, err := h.Store.UpdateMajor(r.Context(), Major{
		ID:       id,
		SchoolID: schoolID,
		Code:     body.Code,
		Name:     body.Name,
		Active:   body.Active,
	})
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			common.WriteError(w, http.StatusNotFound, "Jurusan tidak ditemukan")
			return
		}
		if errors.Is(err, ErrAlreadyExists) {
			common.WriteError(w, http.StatusConflict, "Kode jurusan sudah terdaftar")
			return
		}
		common.WriteError(w, http.StatusInternalServerError, "Gagal memperbarui jurusan")
		return
	}
	h.audit(r.Context(), schoolID, "major", m.ID, "update", claims, nil, map[string]any{"code": m.Code, "name": m.Name}, "")
	common.WriteSuccess(w, m, nil)
}

func (h *Handler) DeleteMajor(w http.ResponseWriter, r *http.Request) {
	claims, schoolID := common.RequireRole(w, r, common.RoleKoordinatorPKL, common.RoleAdmin, common.RoleSuperadmin)
	if claims == nil {
		return
	}
	id := chi.URLParam(r, "id")
	if err := h.Store.DeleteMajor(r.Context(), schoolID, id); err != nil {
		if errors.Is(err, ErrNotFound) {
			common.WriteError(w, http.StatusNotFound, "Jurusan tidak ditemukan")
			return
		}
		common.WriteError(w, http.StatusInternalServerError, "Gagal menghapus jurusan")
		return
	}
	h.audit(r.Context(), schoolID, "major", id, "delete", claims, nil, nil, "")
	common.WriteSuccess(w, map[string]string{"message": "Jurusan berhasil dihapus"}, nil)
}

func (h *Handler) ListClassroomMajors(w http.ResponseWriter, r *http.Request) {
	schoolID := common.EffectiveSchoolID(r)
	list, err := h.Store.ListClassroomMajors(r.Context(), schoolID)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "Gagal mengambil pemetaan kelas-jurusan")
		return
	}
	common.WriteSuccess(w, list, nil)
}

func (h *Handler) PutClassroomMajors(w http.ResponseWriter, r *http.Request) {
	claims, schoolID := common.RequireRole(w, r, common.RoleKoordinatorPKL, common.RoleAdmin, common.RoleSuperadmin)
	if claims == nil {
		return
	}
	var items []ClassroomMajor
	if err := json.NewDecoder(r.Body).Decode(&items); err != nil {
		common.WriteError(w, http.StatusBadRequest, "Body tidak valid")
		return
	}
	for i := range items {
		items[i].SchoolID = schoolID
	}
	if err := h.Store.PutClassroomMajors(r.Context(), schoolID, items); err != nil {
		common.WriteError(w, http.StatusInternalServerError, "Gagal menyimpan pemetaan kelas-jurusan")
		return
	}
	h.audit(r.Context(), schoolID, "classroom_majors", "bulk", "update", claims, nil, map[string]any{"count": len(items)}, "")
	common.WriteSuccess(w, map[string]string{"message": "Pemetaan kelas-jurusan berhasil diperbarui"}, nil)
}

// ---------------------------------------------------------------------------
// 2. Partners & MoUs
// ---------------------------------------------------------------------------

func (h *Handler) ListPartners(w http.ResponseWriter, r *http.Request) {
	schoolID := common.EffectiveSchoolID(r)
	list, err := h.Store.ListPartners(r.Context(), schoolID)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "Gagal mengambil daftar mitra")
		return
	}
	common.WriteSuccess(w, list, nil)
}

func (h *Handler) GetPartner(w http.ResponseWriter, r *http.Request) {
	schoolID := common.EffectiveSchoolID(r)
	id := chi.URLParam(r, "id")
	p, err := h.Store.GetPartner(r.Context(), schoolID, id)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			common.WriteError(w, http.StatusNotFound, "Mitra tidak ditemukan")
			return
		}
		common.WriteError(w, http.StatusInternalServerError, "Gagal mengambil data mitra")
		return
	}
	common.WriteSuccess(w, p, nil)
}

func (h *Handler) CreatePartner(w http.ResponseWriter, r *http.Request) {
	claims, schoolID := common.RequireRole(w, r, common.RoleKoordinatorPKL, common.RoleAdmin, common.RoleSuperadmin)
	if claims == nil {
		return
	}
	var p Partner
	if err := json.NewDecoder(r.Body).Decode(&p); err != nil {
		common.WriteError(w, http.StatusBadRequest, "Body tidak valid")
		return
	}
	p.Name = strings.TrimSpace(p.Name)
	if p.Name == "" {
		common.WriteError(w, http.StatusBadRequest, "Nama mitra wajib diisi")
		return
	}
	p.SchoolID = schoolID
	p.Active = true

	created, err := h.Store.CreatePartner(r.Context(), p)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "Gagal membuat mitra")
		return
	}
	h.audit(r.Context(), schoolID, "partner", created.ID, "create", claims, nil, map[string]any{"name": created.Name}, "")
	common.WriteSuccess(w, created, nil)
}

func (h *Handler) UpdatePartner(w http.ResponseWriter, r *http.Request) {
	claims, schoolID := common.RequireRole(w, r, common.RoleKoordinatorPKL, common.RoleAdmin, common.RoleSuperadmin)
	if claims == nil {
		return
	}
	id := chi.URLParam(r, "id")
	var p Partner
	if err := json.NewDecoder(r.Body).Decode(&p); err != nil {
		common.WriteError(w, http.StatusBadRequest, "Body tidak valid")
		return
	}
	p.ID = id
	p.SchoolID = schoolID
	updated, err := h.Store.UpdatePartner(r.Context(), p)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			common.WriteError(w, http.StatusNotFound, "Mitra tidak ditemukan")
			return
		}
		common.WriteError(w, http.StatusInternalServerError, "Gagal memperbarui mitra")
		return
	}
	h.audit(r.Context(), schoolID, "partner", updated.ID, "update", claims, nil, map[string]any{"name": updated.Name}, "")
	common.WriteSuccess(w, updated, nil)
}

func (h *Handler) DeletePartner(w http.ResponseWriter, r *http.Request) {
	claims, schoolID := common.RequireRole(w, r, common.RoleKoordinatorPKL, common.RoleAdmin, common.RoleSuperadmin)
	if claims == nil {
		return
	}
	id := chi.URLParam(r, "id")
	if err := h.Store.DeletePartner(r.Context(), schoolID, id); err != nil {
		if errors.Is(err, ErrNotFound) {
			common.WriteError(w, http.StatusNotFound, "Mitra tidak ditemukan")
			return
		}
		common.WriteError(w, http.StatusInternalServerError, "Gagal menghapus mitra")
		return
	}
	h.audit(r.Context(), schoolID, "partner", id, "delete", claims, nil, nil, "")
	common.WriteSuccess(w, map[string]string{"message": "Mitra berhasil dihapus"}, nil)
}

func (h *Handler) ListPartnerMous(w http.ResponseWriter, r *http.Request) {
	schoolID := common.EffectiveSchoolID(r)
	partnerID := chi.URLParam(r, "id")
	list, err := h.Store.ListPartnerMous(r.Context(), schoolID, partnerID)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "Gagal mengambil daftar MoU")
		return
	}
	common.WriteSuccess(w, list, nil)
}

func (h *Handler) CreatePartnerMou(w http.ResponseWriter, r *http.Request) {
	claims, schoolID := common.RequireRole(w, r, common.RoleKoordinatorPKL, common.RoleAdmin, common.RoleSuperadmin)
	if claims == nil {
		return
	}
	partnerID := chi.URLParam(r, "id")

	// Support multipart upload or JSON
	var mouNumber string
	var validFrom, validTo time.Time
	var objectKey, fileName, mime *string

	if strings.Contains(r.Header.Get("Content-Type"), "multipart/form-data") {
		if err := r.ParseMultipartForm(10 << 20); err != nil {
			common.WriteError(w, http.StatusBadRequest, "File terlalu besar (maks 10MB)")
			return
		}
		mouNumber = strings.TrimSpace(r.FormValue("mou_number"))
		vf, err1 := time.Parse("2006-01-02", strings.TrimSpace(r.FormValue("valid_from")))
		vt, err2 := time.Parse("2006-01-02", strings.TrimSpace(r.FormValue("valid_to")))
		if err1 != nil || err2 != nil {
			common.WriteError(w, http.StatusBadRequest, "Format tanggal tidak valid (YYYY-MM-DD)")
			return
		}
		validFrom = vf
		validTo = vt

		file, header, err := r.FormFile("file")
		if err == nil && file != nil {
			defer file.Close()
			fn := header.Filename
			fileName = &fn
			ct := header.Header.Get("Content-Type")
			if ct == "" {
				ct = "application/pdf"
			}
			mime = &ct
			key := fmt.Sprintf("pkl/mou/%s_%s_%s", schoolID, partnerID, filepath.Base(fn))
			dstPath := filepath.Join(h.StoragePath, key)
			_ = os.MkdirAll(filepath.Dir(dstPath), 0755)
			dst, err := os.Create(dstPath)
			if err == nil {
				defer dst.Close()
				_, _ = io.Copy(dst, file)
				objectKey = &key
			}
		}
	} else {
		var body struct {
			MouNumber string `json:"mou_number"`
			ValidFrom string `json:"valid_from"`
			ValidTo   string `json:"valid_to"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			common.WriteError(w, http.StatusBadRequest, "Body tidak valid")
			return
		}
		mouNumber = strings.TrimSpace(body.MouNumber)
		vf, err1 := time.Parse("2006-01-02", body.ValidFrom)
		vt, err2 := time.Parse("2006-01-02", body.ValidTo)
		if err1 != nil || err2 != nil {
			common.WriteError(w, http.StatusBadRequest, "Format tanggal tidak valid (YYYY-MM-DD)")
			return
		}
		validFrom = vf
		validTo = vt
	}

	if mouNumber == "" {
		common.WriteError(w, http.StatusBadRequest, "Nomor MoU wajib diisi")
		return
	}
	if validTo.Before(validFrom) {
		common.WriteError(w, http.StatusBadRequest, "Tanggal berakhir MoU harus lebih besar atau sama dengan tanggal mulai")
		return
	}

	uploader := claims.UserID
	created, err := h.Store.CreatePartnerMou(r.Context(), PartnerMou{
		SchoolID:   schoolID,
		PartnerID:  partnerID,
		MouNumber:  mouNumber,
		ValidFrom:  validFrom,
		ValidTo:    validTo,
		ObjectKey:  objectKey,
		FileName:   fileName,
		Mime:       mime,
		UploadedBy: &uploader,
	})
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "Gagal menyimpan MoU")
		return
	}
	h.audit(r.Context(), schoolID, "partner_mou", created.ID, "create", claims, nil, map[string]any{"mou_number": created.MouNumber}, "")
	common.WriteSuccess(w, created, nil)
}

func (h *Handler) GetMouFile(w http.ResponseWriter, r *http.Request) {
	schoolID := common.EffectiveSchoolID(r)
	id := chi.URLParam(r, "id")
	mou, err := h.Store.GetPartnerMou(r.Context(), schoolID, id)
	if err != nil {
		common.WriteError(w, http.StatusNotFound, "MoU tidak ditemukan")
		return
	}
	if mou.ObjectKey == nil || *mou.ObjectKey == "" {
		common.WriteError(w, http.StatusNotFound, "File fisik dokumen MoU tidak ada")
		return
	}
	filePath := filepath.Join(h.StoragePath, *mou.ObjectKey)
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		common.WriteError(w, http.StatusNotFound, "File dokumen tidak ditemukan di server")
		return
	}
	w.Header().Set("Content-Type", "application/pdf")
	if mou.FileName != nil {
		w.Header().Set("Content-Disposition", fmt.Sprintf(`inline; filename="%s"`, *mou.FileName))
	}
	http.ServeFile(w, r, filePath)
}

// ---------------------------------------------------------------------------
// 3. Periods & Quotas
// ---------------------------------------------------------------------------

func (h *Handler) ListPeriods(w http.ResponseWriter, r *http.Request) {
	schoolID := common.EffectiveSchoolID(r)
	list, err := h.Store.ListPeriods(r.Context(), schoolID)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "Gagal mengambil daftar periode")
		return
	}
	common.WriteSuccess(w, list, nil)
}

func (h *Handler) GetPeriod(w http.ResponseWriter, r *http.Request) {
	schoolID := common.EffectiveSchoolID(r)
	id := chi.URLParam(r, "id")
	p, err := h.Store.GetPeriod(r.Context(), schoolID, id)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			common.WriteError(w, http.StatusNotFound, "Periode tidak ditemukan")
			return
		}
		common.WriteError(w, http.StatusInternalServerError, "Gagal mengambil data periode")
		return
	}
	common.WriteSuccess(w, p, nil)
}

func (h *Handler) CreatePeriod(w http.ResponseWriter, r *http.Request) {
	claims, schoolID := common.RequireRole(w, r, common.RoleKoordinatorPKL, common.RoleAdmin, common.RoleSuperadmin)
	if claims == nil {
		return
	}
	var body struct {
		Name           string  `json:"name"`
		AcademicYearID *string `json:"academic_year_id"`
		StartsOn       string  `json:"starts_on"`
		EndsOn         string  `json:"ends_on"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		common.WriteError(w, http.StatusBadRequest, "Body tidak valid")
		return
	}
	body.Name = strings.TrimSpace(body.Name)
	if body.Name == "" {
		common.WriteError(w, http.StatusBadRequest, "Nama periode wajib diisi")
		return
	}
	startsOn, err1 := time.Parse("2006-01-02", body.StartsOn)
	endsOn, err2 := time.Parse("2006-01-02", body.EndsOn)
	if err1 != nil || err2 != nil {
		common.WriteError(w, http.StatusBadRequest, "Format tanggal mulai/selesai tidak valid (YYYY-MM-DD)")
		return
	}
	if endsOn.Before(startsOn) {
		common.WriteError(w, http.StatusBadRequest, "Tanggal selesai harus lebih besar atau sama dengan tanggal mulai")
		return
	}

	p, err := h.Store.CreatePeriod(r.Context(), Period{
		SchoolID:       schoolID,
		AcademicYearID: body.AcademicYearID,
		Name:           body.Name,
		StartsOn:       startsOn,
		EndsOn:         endsOn,
		Status:         PeriodStatusPlanning,
	})
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "Gagal membuat periode")
		return
	}
	h.audit(r.Context(), schoolID, "period", p.ID, "create", claims, nil, map[string]any{"name": p.Name}, "")
	common.WriteSuccess(w, p, nil)
}

func (h *Handler) UpdatePeriod(w http.ResponseWriter, r *http.Request) {
	claims, schoolID := common.RequireRole(w, r, common.RoleKoordinatorPKL, common.RoleAdmin, common.RoleSuperadmin)
	if claims == nil {
		return
	}
	id := chi.URLParam(r, "id")
	var body struct {
		Name           string  `json:"name"`
		AcademicYearID *string `json:"academic_year_id"`
		StartsOn       string  `json:"starts_on"`
		EndsOn         string  `json:"ends_on"`
		Status         string  `json:"status"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		common.WriteError(w, http.StatusBadRequest, "Body tidak valid")
		return
	}
	startsOn, err1 := time.Parse("2006-01-02", body.StartsOn)
	endsOn, err2 := time.Parse("2006-01-02", body.EndsOn)
	if err1 != nil || err2 != nil {
		common.WriteError(w, http.StatusBadRequest, "Format tanggal tidak valid")
		return
	}

	p, err := h.Store.UpdatePeriod(r.Context(), Period{
		ID:             id,
		SchoolID:       schoolID,
		AcademicYearID: body.AcademicYearID,
		Name:           body.Name,
		StartsOn:       startsOn,
		EndsOn:         endsOn,
		Status:         body.Status,
	})
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			common.WriteError(w, http.StatusNotFound, "Periode tidak ditemukan")
			return
		}
		common.WriteError(w, http.StatusInternalServerError, "Gagal memperbarui periode")
		return
	}
	h.audit(r.Context(), schoolID, "period", p.ID, "update", claims, nil, map[string]any{"name": p.Name}, "")
	common.WriteSuccess(w, p, nil)
}

func (h *Handler) ClosePeriod(w http.ResponseWriter, r *http.Request) {
	claims, schoolID := common.RequireRole(w, r, common.RoleKoordinatorPKL, common.RoleAdmin, common.RoleSuperadmin)
	if claims == nil {
		return
	}
	id := chi.URLParam(r, "id")

	if err := h.Store.ClosePeriod(r.Context(), schoolID, id); err != nil {
		if errors.Is(err, ErrNotFound) {
			common.WriteError(w, http.StatusNotFound, "Periode tidak ditemukan")
			return
		}
		common.WriteError(w, http.StatusInternalServerError, "Gagal menutup periode")
		return
	}

	// §4.1 (b): Deactivate industry mentors with no other active/planned placements or incomplete assessments
	eligibleMentors, err := h.Store.FindMentorsEligibleForClosure(r.Context(), schoolID, id)
	if err == nil && len(eligibleMentors) > 0 {
		for _, m := range eligibleMentors {
			_ = h.Store.DeactivateIndustryMentor(r.Context(), schoolID, m.ID)
			if h.Client != nil {
				_ = h.Client.DeactivateUser(r.Context(), schoolID, m.UserID, "pkl_period_closed")
			}
		}
	}

	h.audit(r.Context(), schoolID, "period", id, "close", claims, nil, map[string]any{"status": "closed"}, "Periode PKL ditutup")
	common.WriteSuccess(w, map[string]string{"message": "Periode PKL berhasil ditutup"}, nil)
}

func (h *Handler) ListQuotas(w http.ResponseWriter, r *http.Request) {
	schoolID := common.EffectiveSchoolID(r)
	periodID := chi.URLParam(r, "id")
	list, err := h.Store.ListQuotas(r.Context(), schoolID, periodID)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "Gagal mengambil matriks kuota")
		return
	}
	common.WriteSuccess(w, list, nil)
}

func (h *Handler) PutQuotas(w http.ResponseWriter, r *http.Request) {
	claims, schoolID := common.RequireRole(w, r, common.RoleKoordinatorPKL, common.RoleAdmin, common.RoleSuperadmin)
	if claims == nil {
		return
	}
	periodID := chi.URLParam(r, "id")
	var quotas []Quota
	if err := json.NewDecoder(r.Body).Decode(&quotas); err != nil {
		common.WriteError(w, http.StatusBadRequest, "Body tidak valid")
		return
	}
	for i := range quotas {
		quotas[i].SchoolID = schoolID
		quotas[i].PeriodID = periodID
	}
	if err := h.Store.PutQuotas(r.Context(), schoolID, periodID, quotas); err != nil {
		common.WriteError(w, http.StatusInternalServerError, "Gagal menyimpan kuota mitra")
		return
	}
	h.audit(r.Context(), schoolID, "quotas", periodID, "update", claims, nil, map[string]any{"count": len(quotas)}, "")
	common.WriteSuccess(w, map[string]string{"message": "Kuota berhasil diperbarui"}, nil)
}

// ---------------------------------------------------------------------------
// 4. Industry Mentors
// ---------------------------------------------------------------------------

func (h *Handler) ListIndustryMentors(w http.ResponseWriter, r *http.Request) {
	schoolID := common.EffectiveSchoolID(r)
	partnerID := chi.URLParam(r, "id")
	list, err := h.Store.ListIndustryMentors(r.Context(), schoolID, partnerID)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "Gagal mengambil daftar pembimbing industri")
		return
	}
	common.WriteSuccess(w, list, nil)
}

func (h *Handler) CreateIndustryMentor(w http.ResponseWriter, r *http.Request) {
	claims, schoolID := common.RequireRole(w, r, common.RoleKoordinatorPKL, common.RoleAdmin, common.RoleSuperadmin)
	if claims == nil {
		return
	}
	partnerID := chi.URLParam(r, "id")

	var body struct {
		FullName string  `json:"full_name"`
		Email    string  `json:"email"`
		Phone    *string `json:"phone"`
		JobTitle *string `json:"job_title"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		common.WriteError(w, http.StatusBadRequest, "Body tidak valid")
		return
	}
	body.FullName = strings.TrimSpace(body.FullName)
	body.Email = strings.ToLower(strings.TrimSpace(body.Email))
	if body.FullName == "" || body.Email == "" {
		common.WriteError(w, http.StatusBadRequest, "Nama lengkap dan email wajib diisi")
		return
	}

	// Call identity to create or link role account (§4.1)
	if h.Client == nil {
		common.WriteError(w, http.StatusInternalServerError, "Client identity tidak terkonfigurasi")
		return
	}

	acc, err := h.Client.CreateOrLinkRoleAccount(r.Context(), schoolID, body.Email, body.FullName, common.RolePembimbingIndustri)
	if err != nil {
		common.WriteError(w, http.StatusBadRequest, "Gagal membuat/menautkan akun pembimbing: "+err.Error())
		return
	}

	mentor, err := h.Store.CreateIndustryMentor(r.Context(), IndustryMentor{
		SchoolID:  schoolID,
		PartnerID: partnerID,
		UserID:    acc.UserID,
		FullName:  body.FullName,
		Email:     body.Email,
		Phone:     body.Phone,
		JobTitle:  body.JobTitle,
	})
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "Gagal menyimpan data pembimbing industri")
		return
	}

	h.audit(r.Context(), schoolID, "industry_mentor", mentor.ID, "create", claims, nil, map[string]any{"email": mentor.Email}, "")

	// default_password only sent once if newly created (§4.1)
	res := map[string]any{
		"mentor":  mentor,
		"created": acc.Created,
	}
	if acc.Created && acc.DefaultPassword != "" {
		res["default_password"] = acc.DefaultPassword
	}
	common.WriteSuccess(w, res, nil)
}

func (h *Handler) DeactivateIndustryMentor(w http.ResponseWriter, r *http.Request) {
	claims, schoolID := common.RequireRole(w, r, common.RoleKoordinatorPKL, common.RoleAdmin, common.RoleSuperadmin)
	if claims == nil {
		return
	}
	id := chi.URLParam(r, "id")
	m, err := h.Store.GetIndustryMentor(r.Context(), schoolID, id)
	if err != nil {
		common.WriteError(w, http.StatusNotFound, "Pembimbing industri tidak ditemukan")
		return
	}

	if err := h.Store.DeactivateIndustryMentor(r.Context(), schoolID, id); err != nil {
		common.WriteError(w, http.StatusInternalServerError, "Gagal menonaktifkan pembimbing industri")
		return
	}

	if h.Client != nil {
		_ = h.Client.DeactivateUser(r.Context(), schoolID, m.UserID, "pkl_mentor_inactive")
	}

	h.audit(r.Context(), schoolID, "industry_mentor", id, "deactivate", claims, nil, nil, "Dinonaktifkan oleh koordinator")
	common.WriteSuccess(w, map[string]string{"message": "Pembimbing industri berhasil dinonaktifkan"}, nil)
}

// ---------------------------------------------------------------------------
// 5. Placements Check & CRUD
// ---------------------------------------------------------------------------

func (h *Handler) CheckPlacement(w http.ResponseWriter, r *http.Request) {
	schoolID := common.EffectiveSchoolID(r)
	periodID := r.URL.Query().Get("period_id")
	partnerID := r.URL.Query().Get("partner_id")
	studentID := r.URL.Query().Get("student_id")
	majorID := r.URL.Query().Get("major_id")

	if periodID == "" || partnerID == "" {
		common.WriteError(w, http.StatusBadRequest, "period_id dan partner_id wajib diisi")
		return
	}

	// Resolve student's major if not provided directly
	if majorID == "" && studentID != "" && h.Client != nil {
		classID, err := h.Client.StudentClassroomID(r.Context(), studentID, schoolID)
		if err == nil && classID != "" {
			m, err := h.Store.GetMajorByClassroom(r.Context(), schoolID, classID)
			if err == nil && m != nil {
				majorID = m.ID
			}
		}
	}

	if majorID == "" {
		common.WriteError(w, http.StatusBadRequest, "Jurusan siswa tidak dapat ditentukan, pastikan kelas siswa sudah dipetakan ke jurusan")
		return
	}

	check, err := h.Store.CheckPlacement(r.Context(), schoolID, periodID, partnerID, majorID)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	common.WriteSuccess(w, check, nil)
}

func (h *Handler) ListPlacements(w http.ResponseWriter, r *http.Request) {
	claims := common.FromCtx(r)
	schoolID := common.EffectiveSchoolID(r)

	filter := PlacementFilter{
		PeriodID:  r.URL.Query().Get("period_id"),
		PartnerID: r.URL.Query().Get("partner_id"),
		MajorID:   r.URL.Query().Get("major_id"),
		StudentID: r.URL.Query().Get("student_id"),
		Status:    r.URL.Query().Get("status"),
	}

	// Scope restrictions
	if claims.ActiveRole == common.RolePembimbingIndustri {
		filter.IndustryMentorID = claims.UserID
	} else if claims.ActiveRole == common.RoleGuru || claims.ActiveRole == common.RoleWalikelas {
		// Only placements where they are school mentor
		filter.SchoolMentorID = claims.UserID
	} else if claims.ActiveRole == common.RoleSiswa {
		filter.StudentID = claims.UserID
	}

	list, err := h.Store.ListPlacements(r.Context(), schoolID, filter)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "Gagal mengambil daftar penempatan")
		return
	}

	// Resolve student names from identity if possible
	if h.Client != nil && len(list) > 0 {
		var sids []string
		for _, pl := range list {
			sids = append(sids, pl.StudentID)
		}
		names, _ := h.Client.UserNames(r.Context(), schoolID, sids)
		for i := range list {
			if n, ok := names[list[i].StudentID]; ok {
				list[i].StudentName = n
			}
		}
	}

	common.WriteSuccess(w, list, nil)
}

func (h *Handler) GetPlacement(w http.ResponseWriter, r *http.Request) {
	claims := common.FromCtx(r)
	schoolID := common.EffectiveSchoolID(r)
	id := chi.URLParam(r, "id")

	pl, err := h.Store.GetPlacement(r.Context(), schoolID, id)
	if err != nil {
		common.WriteError(w, http.StatusNotFound, "Penempatan tidak ditemukan")
		return
	}

	// Check scope
	var mentorUserID *string
	if pl.IndustryMentorID != nil {
		im, _ := h.Store.GetIndustryMentor(r.Context(), schoolID, *pl.IndustryMentorID)
		if im != nil {
			mentorUserID = &im.UserID
		}
	}
	if !Scope(claims.ActiveRole, claims.UserID, *pl, mentorUserID) {
		common.WriteError(w, http.StatusNotFound, "Penempatan tidak ditemukan")
		return
	}

	if h.Client != nil {
		names, _ := h.Client.UserNames(r.Context(), schoolID, []string{pl.StudentID, pl.SchoolMentorUserID})
		if n, ok := names[pl.StudentID]; ok {
			pl.StudentName = n
		}
		if n, ok := names[pl.SchoolMentorUserID]; ok {
			pl.SchoolMentorName = n
		}
	}

	common.WriteSuccess(w, pl, nil)
}

func (h *Handler) CreatePlacement(w http.ResponseWriter, r *http.Request) {
	claims, schoolID := common.RequireRole(w, r, common.RoleKoordinatorPKL, common.RoleAdmin, common.RoleSuperadmin)
	if claims == nil {
		return
	}

	idempotencyKey := r.Header.Get("Idempotency-Key")

	var body struct {
		PeriodID           string  `json:"period_id"`
		PartnerID          string  `json:"partner_id"`
		MajorID            string  `json:"major_id"`
		StudentID          string  `json:"student_id"`
		SchoolMentorUserID string  `json:"school_mentor_user_id"`
		IndustryMentorID   *string `json:"industry_mentor_id"`
		StartsOn           string  `json:"starts_on"`
		EndsOn             string  `json:"ends_on"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		common.WriteError(w, http.StatusBadRequest, "Body tidak valid")
		return
	}

	if body.PeriodID == "" || body.PartnerID == "" || body.StudentID == "" || body.SchoolMentorUserID == "" {
		common.WriteError(w, http.StatusBadRequest, "Periode, mitra, siswa, dan pembimbing sekolah wajib diisi")
		return
	}

	// If majorID is empty, try to resolve from student's classroom
	if body.MajorID == "" && h.Client != nil {
		classID, err := h.Client.StudentClassroomID(r.Context(), body.StudentID, schoolID)
		if err == nil && classID != "" {
			m, err := h.Store.GetMajorByClassroom(r.Context(), schoolID, classID)
			if err == nil && m != nil {
				body.MajorID = m.ID
			}
		}
	}
	if body.MajorID == "" {
		common.WriteError(w, http.StatusBadRequest, "Jurusan siswa belum dipetakan")
		return
	}

	// Default starts_on and ends_on from period if not explicitly set
	period, err := h.Store.GetPeriod(r.Context(), schoolID, body.PeriodID)
	if err != nil {
		common.WriteError(w, http.StatusBadRequest, "Periode tidak ditemukan")
		return
	}

	startsOn := period.StartsOn
	endsOn := period.EndsOn
	if body.StartsOn != "" {
		if t, err := time.Parse("2006-01-02", body.StartsOn); err == nil {
			startsOn = t
		}
	}
	if body.EndsOn != "" {
		if t, err := time.Parse("2006-01-02", body.EndsOn); err == nil {
			endsOn = t
		}
	}

	var idemKeyPtr *string
	if idempotencyKey != "" {
		idemKeyPtr = &idempotencyKey
	}
	creatorID := claims.UserID

	status := PlacementStatusPlanned
	todayStr := time.Now().Format("2006-01-02")
	if startsOn.Format("2006-01-02") <= todayStr {
		status = PlacementStatusActive
	}

	pl, err := h.Store.CreatePlacement(r.Context(), Placement{
		SchoolID:           schoolID,
		PeriodID:           body.PeriodID,
		PartnerID:          body.PartnerID,
		MajorID:            body.MajorID,
		StudentID:          body.StudentID,
		SchoolMentorUserID: body.SchoolMentorUserID,
		IndustryMentorID:   body.IndustryMentorID,
		StartsOn:           startsOn,
		EndsOn:             endsOn,
		Status:             status,
		CreatedBy:          &creatorID,
		IdempotencyKey:     idemKeyPtr,
	})
	if err != nil {
		// Business validation failures (AC-140-01, AC-139-01) must return 422
		common.WriteError(w, http.StatusUnprocessableEntity, err.Error())
		return
	}

	h.audit(r.Context(), schoolID, "placement", pl.ID, "create", claims, nil, map[string]any{"student_id": pl.StudentID, "partner_id": pl.PartnerID}, "")

	// Send notifications (§4.2)
	if h.Client != nil {
		// To student
		_ = h.Client.Notify(r.Context(), client.Notification{
			SchoolID: schoolID,
			UserID:   pl.StudentID,
			Type:     "pkl_placement",
			Title:    "Penempatan PKL Baru",
			Body:     fmt.Sprintf("Anda telah ditempatkan di %s untuk periode PKL.", pl.PartnerName),
			Data:     map[string]any{"placement_id": pl.ID},
		}, fmt.Sprintf("studentlife:pkl_placement:%s:%s", pl.ID, pl.StudentID))

		// To school mentor
		_ = h.Client.Notify(r.Context(), client.Notification{
			SchoolID: schoolID,
			UserID:   pl.SchoolMentorUserID,
			Type:     "pkl_placement",
			Title:    "Penugasan Pembimbing PKL",
			Body:     fmt.Sprintf("Anda ditugaskan sebagai pembimbing sekolah untuk siswa di %s.", pl.PartnerName),
			Data:     map[string]any{"placement_id": pl.ID},
		}, fmt.Sprintf("studentlife:pkl_placement:%s:%s", pl.ID, pl.SchoolMentorUserID))

		// To industry mentor if assigned
		if pl.IndustryMentorID != nil {
			im, _ := h.Store.GetIndustryMentor(r.Context(), schoolID, *pl.IndustryMentorID)
			if im != nil {
				_ = h.Client.Notify(r.Context(), client.Notification{
					SchoolID: schoolID,
					UserID:   im.UserID,
					Type:     "pkl_placement",
					Title:    "Penugasan Pembimbing Industri PKL",
					Body:     fmt.Sprintf("Siswa baru telah ditempatkan di perusahaan Anda untuk dibimbing."),
					Data:     map[string]any{"placement_id": pl.ID},
				}, fmt.Sprintf("studentlife:pkl_placement:%s:%s", pl.ID, im.UserID))
			}
		}
	}

	common.WriteJSON(w, http.StatusCreated, pl, nil)
}

func (h *Handler) UpdatePlacement(w http.ResponseWriter, r *http.Request) {
	claims, schoolID := common.RequireRole(w, r, common.RoleKoordinatorPKL, common.RoleAdmin, common.RoleSuperadmin)
	if claims == nil {
		return
	}
	id := chi.URLParam(r, "id")

	var body struct {
		SchoolMentorUserID string  `json:"school_mentor_user_id"`
		IndustryMentorID   *string `json:"industry_mentor_id"`
		StartsOn           string  `json:"starts_on"`
		EndsOn             string  `json:"ends_on"`
		Status             string  `json:"status"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		common.WriteError(w, http.StatusBadRequest, "Body tidak valid")
		return
	}
	startsOn, err1 := time.Parse("2006-01-02", body.StartsOn)
	endsOn, err2 := time.Parse("2006-01-02", body.EndsOn)
	if err1 != nil || err2 != nil {
		common.WriteError(w, http.StatusBadRequest, "Format tanggal tidak valid")
		return
	}

	pl, err := h.Store.UpdatePlacement(r.Context(), Placement{
		ID:                 id,
		SchoolID:           schoolID,
		SchoolMentorUserID: body.SchoolMentorUserID,
		IndustryMentorID:   body.IndustryMentorID,
		StartsOn:           startsOn,
		EndsOn:             endsOn,
		Status:             body.Status,
	})
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "Gagal memperbarui penempatan")
		return
	}
	h.audit(r.Context(), schoolID, "placement", pl.ID, "update", claims, nil, map[string]any{"status": pl.Status}, "")
	common.WriteSuccess(w, pl, nil)
}

func (h *Handler) CancelPlacement(w http.ResponseWriter, r *http.Request) {
	claims, schoolID := common.RequireRole(w, r, common.RoleKoordinatorPKL, common.RoleAdmin, common.RoleSuperadmin)
	if claims == nil {
		return
	}
	id := chi.URLParam(r, "id")
	var body struct {
		Reason string `json:"reason"`
	}
	_ = json.NewDecoder(r.Body).Decode(&body)

	if err := h.Store.CancelPlacement(r.Context(), schoolID, id, body.Reason); err != nil {
		if errors.Is(err, ErrNotFound) {
			common.WriteError(w, http.StatusNotFound, "Penempatan tidak ditemukan atau sudah dibatalkan")
			return
		}
		common.WriteError(w, http.StatusInternalServerError, "Gagal membatalkan penempatan")
		return
	}
	h.audit(r.Context(), schoolID, "placement", id, "cancel", claims, nil, nil, body.Reason)
	common.WriteSuccess(w, map[string]string{"message": "Penempatan berhasil dibatalkan"}, nil)
}

// ---------------------------------------------------------------------------
// 6. Attendance & Recap
// ---------------------------------------------------------------------------

func (h *Handler) ListPlacementAttendance(w http.ResponseWriter, r *http.Request) {
	claims := common.FromCtx(r)
	schoolID := common.EffectiveSchoolID(r)
	placementID := chi.URLParam(r, "id")

	pl, err := h.Store.GetPlacement(r.Context(), schoolID, placementID)
	if err != nil {
		common.WriteError(w, http.StatusNotFound, "Penempatan tidak ditemukan")
		return
	}
	var mentorUserID *string
	if pl.IndustryMentorID != nil {
		im, _ := h.Store.GetIndustryMentor(r.Context(), schoolID, *pl.IndustryMentorID)
		if im != nil {
			mentorUserID = &im.UserID
		}
	}
	if !Scope(claims.ActiveRole, claims.UserID, *pl, mentorUserID) {
		common.WriteError(w, http.StatusNotFound, "Penempatan tidak ditemukan")
		return
	}

	list, err := h.Store.ListAttendance(r.Context(), schoolID, placementID)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "Gagal mengambil daftar presensi")
		return
	}
	common.WriteSuccess(w, list, nil)
}

func (h *Handler) ConfirmAttendance(w http.ResponseWriter, r *http.Request) {
	claims := common.FromCtx(r)
	schoolID := common.EffectiveSchoolID(r)
	attendanceID := chi.URLParam(r, "id")

	att, err := h.Store.GetAttendance(r.Context(), schoolID, attendanceID)
	if err != nil {
		common.WriteError(w, http.StatusNotFound, "Presensi tidak ditemukan")
		return
	}
	pl, err := h.Store.GetPlacement(r.Context(), schoolID, att.PlacementID)
	if err != nil {
		common.WriteError(w, http.StatusNotFound, "Penempatan tidak ditemukan")
		return
	}

	// Only industry mentor for that placement or koordinator_pkl
	if claims.ActiveRole == common.RolePembimbingIndustri {
		if pl.IndustryMentorID == nil {
			common.WriteError(w, http.StatusForbidden, "Tidak berwenang mengonfirmasi presensi ini")
			return
		}
		im, _ := h.Store.GetIndustryMentor(r.Context(), schoolID, *pl.IndustryMentorID)
		if im == nil || im.UserID != claims.UserID {
			common.WriteError(w, http.StatusForbidden, "Hanya pembimbing industri penempatan ini yang dapat mengonfirmasi")
			return
		}
	} else if claims.ActiveRole != common.RoleKoordinatorPKL && claims.ActiveRole != common.RoleAdmin && claims.ActiveRole != common.RoleSuperadmin {
		common.WriteError(w, http.StatusForbidden, "Tidak memiliki izin mengonfirmasi presensi")
		return
	}

	if err := h.Store.ConfirmAttendance(r.Context(), schoolID, attendanceID, claims.UserID); err != nil {
		common.WriteError(w, http.StatusInternalServerError, "Gagal mengonfirmasi presensi")
		return
	}
	common.WriteSuccess(w, map[string]string{"message": "Presensi berhasil dikonfirmasi"}, nil)
}

func (h *Handler) RejectAttendance(w http.ResponseWriter, r *http.Request) {
	claims := common.FromCtx(r)
	schoolID := common.EffectiveSchoolID(r)
	attendanceID := chi.URLParam(r, "id")

	var body struct {
		Reason string `json:"reason"`
	}
	_ = json.NewDecoder(r.Body).Decode(&body)
	body.Reason = strings.TrimSpace(body.Reason)
	if body.Reason == "" {
		common.WriteError(w, http.StatusBadRequest, "Alasan penolakan presensi wajib diisi")
		return
	}

	att, err := h.Store.GetAttendance(r.Context(), schoolID, attendanceID)
	if err != nil {
		common.WriteError(w, http.StatusNotFound, "Presensi tidak ditemukan")
		return
	}
	pl, err := h.Store.GetPlacement(r.Context(), schoolID, att.PlacementID)
	if err != nil {
		common.WriteError(w, http.StatusNotFound, "Penempatan tidak ditemukan")
		return
	}

	if claims.ActiveRole == common.RolePembimbingIndustri {
		if pl.IndustryMentorID == nil {
			common.WriteError(w, http.StatusForbidden, "Tidak berwenang menolak presensi ini")
			return
		}
		im, _ := h.Store.GetIndustryMentor(r.Context(), schoolID, *pl.IndustryMentorID)
		if im == nil || im.UserID != claims.UserID {
			common.WriteError(w, http.StatusForbidden, "Hanya pembimbing industri penempatan ini yang dapat menolak")
			return
		}
	} else if claims.ActiveRole != common.RoleKoordinatorPKL && claims.ActiveRole != common.RoleAdmin && claims.ActiveRole != common.RoleSuperadmin {
		common.WriteError(w, http.StatusForbidden, "Tidak memiliki izin menolak presensi")
		return
	}

	if err := h.Store.RejectAttendance(r.Context(), schoolID, attendanceID, body.Reason, claims.UserID); err != nil {
		common.WriteError(w, http.StatusInternalServerError, "Gagal menolak presensi")
		return
	}

	// Notify student (§4.2)
	if h.Client != nil {
		_ = h.Client.Notify(r.Context(), client.Notification{
			SchoolID: schoolID,
			UserID:   pl.StudentID,
			Type:     "pkl_attendance_rejected",
			Title:    "Presensi PKL Ditolak",
			Body:     fmt.Sprintf("Presensi tanggal %s ditolak: %s", att.WorkDate.Format("02-01-2006"), body.Reason),
			Data:     map[string]any{"placement_id": pl.ID, "work_date": att.WorkDate.Format("2006-01-02"), "reason": body.Reason},
		}, fmt.Sprintf("studentlife:pkl_att_rej:%s:%s", att.ID, pl.StudentID))
	}

	common.WriteSuccess(w, map[string]string{"message": "Presensi berhasil ditolak"}, nil)
}

func (h *Handler) GetRecap(w http.ResponseWriter, r *http.Request) {
	schoolID := common.EffectiveSchoolID(r)
	periodID := r.URL.Query().Get("period_id")
	partnerID := r.URL.Query().Get("partner_id")
	format := r.URL.Query().Get("format")

	rows, err := h.Store.RecapAttendance(r.Context(), schoolID, periodID, partnerID)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "Gagal mengambil rekap kehadiran")
		return
	}

	// Resolve student names
	if h.Client != nil && len(rows) > 0 {
		var sids []string
		for _, row := range rows {
			sids = append(sids, row.StudentID)
		}
		names, _ := h.Client.UserNames(r.Context(), schoolID, sids)
		for i := range rows {
			if n, ok := names[rows[i].StudentID]; ok {
				rows[i].StudentName = n
			}
		}
	}

	if format == "csv" {
		w.Header().Set("Content-Type", "text/csv; charset=utf-8")
		w.Header().Set("Content-Disposition", "attachment; filename=\"rekap-pkl.csv\"")
		writer := csv.NewWriter(w)
		_ = writer.Write([]string{"No", "ID Siswa", "Nama Siswa", "Mitra", "Jurusan", "Hadir", "Izin", "Sakit", "Total Hari", "Menunggu", "Terkonfirmasi", "Ditolak", "Persentase Kehadiran (%)"})
		for i, rk := range rows {
			pct := "0"
			if rk.Total > 0 {
				pct = fmt.Sprintf("%.1f", float64(rk.Hadir)/float64(rk.Total)*100.0)
			}
			_ = writer.Write([]string{
				strconv.Itoa(i + 1),
				rk.StudentID,
				rk.StudentName,
				rk.PartnerName,
				rk.MajorName,
				strconv.Itoa(rk.Hadir),
				strconv.Itoa(rk.Izin),
				strconv.Itoa(rk.Sakit),
				strconv.Itoa(rk.Total),
				strconv.Itoa(rk.Pending),
				strconv.Itoa(rk.Confirmed),
				strconv.Itoa(rk.Rejected),
				pct,
			})
		}
		writer.Flush()
		return
	}

	common.WriteSuccess(w, rows, nil)
}

// ---------------------------------------------------------------------------
// 7. Journals & Final Reports
// ---------------------------------------------------------------------------

func (h *Handler) ListPlacementJournals(w http.ResponseWriter, r *http.Request) {
	claims := common.FromCtx(r)
	schoolID := common.EffectiveSchoolID(r)
	placementID := chi.URLParam(r, "id")

	pl, err := h.Store.GetPlacement(r.Context(), schoolID, placementID)
	if err != nil {
		common.WriteError(w, http.StatusNotFound, "Penempatan tidak ditemukan")
		return
	}
	var mentorUserID *string
	if pl.IndustryMentorID != nil {
		im, _ := h.Store.GetIndustryMentor(r.Context(), schoolID, *pl.IndustryMentorID)
		if im != nil {
			mentorUserID = &im.UserID
		}
	}
	if !Scope(claims.ActiveRole, claims.UserID, *pl, mentorUserID) {
		common.WriteError(w, http.StatusNotFound, "Penempatan tidak ditemukan")
		return
	}

	list, err := h.Store.ListJournals(r.Context(), schoolID, placementID)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "Gagal mengambil daftar jurnal")
		return
	}
	common.WriteSuccess(w, list, nil)
}

func (h *Handler) AddJournalNote(w http.ResponseWriter, r *http.Request) {
	claims := common.FromCtx(r)
	schoolID := common.EffectiveSchoolID(r)
	placementID := chi.URLParam(r, "id")
	journalID := chi.URLParam(r, "jid")

	pl, err := h.Store.GetPlacement(r.Context(), schoolID, placementID)
	if err != nil {
		common.WriteError(w, http.StatusNotFound, "Penempatan tidak ditemukan")
		return
	}
	var mentorUserID *string
	if pl.IndustryMentorID != nil {
		im, _ := h.Store.GetIndustryMentor(r.Context(), schoolID, *pl.IndustryMentorID)
		if im != nil {
			mentorUserID = &im.UserID
		}
	}
	if !Scope(claims.ActiveRole, claims.UserID, *pl, mentorUserID) {
		common.WriteError(w, http.StatusNotFound, "Penempatan tidak ditemukan")
		return
	}

	var body struct {
		Note string `json:"note"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		common.WriteError(w, http.StatusBadRequest, "Body tidak valid")
		return
	}
	body.Note = strings.TrimSpace(body.Note)
	if body.Note == "" {
		common.WriteError(w, http.StatusBadRequest, "Catatan pembimbing wajib diisi")
		return
	}

	if err := h.Store.AddJournalMentorNote(r.Context(), schoolID, journalID, body.Note, claims.UserID); err != nil {
		common.WriteError(w, http.StatusInternalServerError, "Gagal menambahkan catatan pembimbing")
		return
	}
	common.WriteSuccess(w, map[string]string{"message": "Catatan berhasil ditambahkan"}, nil)
}

func (h *Handler) ListPlacementFinalReports(w http.ResponseWriter, r *http.Request) {
	claims := common.FromCtx(r)
	schoolID := common.EffectiveSchoolID(r)
	placementID := chi.URLParam(r, "id")

	pl, err := h.Store.GetPlacement(r.Context(), schoolID, placementID)
	if err != nil {
		common.WriteError(w, http.StatusNotFound, "Penempatan tidak ditemukan")
		return
	}
	var mentorUserID *string
	if pl.IndustryMentorID != nil {
		im, _ := h.Store.GetIndustryMentor(r.Context(), schoolID, *pl.IndustryMentorID)
		if im != nil {
			mentorUserID = &im.UserID
		}
	}
	if !Scope(claims.ActiveRole, claims.UserID, *pl, mentorUserID) {
		common.WriteError(w, http.StatusNotFound, "Penempatan tidak ditemukan")
		return
	}

	list, err := h.Store.ListFinalReports(r.Context(), schoolID, placementID)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "Gagal mengambil daftar laporan akhir")
		return
	}
	common.WriteSuccess(w, list, nil)
}

// ---------------------------------------------------------------------------
// 8. Schemes & Scores
// ---------------------------------------------------------------------------

func (h *Handler) GetScheme(w http.ResponseWriter, r *http.Request) {
	schoolID := common.EffectiveSchoolID(r)
	periodID := chi.URLParam(r, "id")

	sc, err := h.Store.GetScheme(r.Context(), schoolID, periodID)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			common.WriteError(w, http.StatusNotFound, "Skema penilaian belum diatur untuk periode ini")
			return
		}
		common.WriteError(w, http.StatusInternalServerError, "Gagal mengambil skema penilaian")
		return
	}
	common.WriteSuccess(w, sc, nil)
}

func (h *Handler) PutScheme(w http.ResponseWriter, r *http.Request) {
	claims, schoolID := common.RequireRole(w, r, common.RoleKoordinatorPKL, common.RoleAdmin, common.RoleSuperadmin)
	if claims == nil {
		return
	}
	periodID := chi.URLParam(r, "id")

	var sc Scheme
	if err := json.NewDecoder(r.Body).Decode(&sc); err != nil {
		common.WriteError(w, http.StatusBadRequest, "Body tidak valid")
		return
	}
	sc.SchoolID = schoolID
	sc.PeriodID = periodID

	saved, err := h.Store.PutScheme(r.Context(), schoolID, periodID, sc)
	if err != nil {
		if errors.Is(err, ErrSchemeLocked) {
			common.WriteError(w, http.StatusConflict, err.Error())
			return
		}
		common.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	h.audit(r.Context(), schoolID, "scheme", periodID, "update", claims, nil, map[string]any{"industry_weight_bp": saved.IndustryWeightBP, "school_weight_bp": saved.SchoolWeightBP}, "")
	common.WriteSuccess(w, saved, nil)
}

func (h *Handler) ListPlacementScores(w http.ResponseWriter, r *http.Request) {
	claims := common.FromCtx(r)
	schoolID := common.EffectiveSchoolID(r)
	placementID := chi.URLParam(r, "id")

	pl, err := h.Store.GetPlacement(r.Context(), schoolID, placementID)
	if err != nil {
		common.WriteError(w, http.StatusNotFound, "Penempatan tidak ditemukan")
		return
	}
	var mentorUserID *string
	if pl.IndustryMentorID != nil {
		im, _ := h.Store.GetIndustryMentor(r.Context(), schoolID, *pl.IndustryMentorID)
		if im != nil {
			mentorUserID = &im.UserID
		}
	}
	if !Scope(claims.ActiveRole, claims.UserID, *pl, mentorUserID) {
		common.WriteError(w, http.StatusNotFound, "Penempatan tidak ditemukan")
		return
	}

	scores, err := h.Store.ListScores(r.Context(), schoolID, placementID)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "Gagal mengambil nilai")
		return
	}
	res, _ := h.Store.GetResult(r.Context(), schoolID, placementID)

	common.WriteSuccess(w, map[string]any{
		"scores": scores,
		"result": res,
	}, nil)
}

func (h *Handler) PutPlacementScores(w http.ResponseWriter, r *http.Request) {
	claims := common.FromCtx(r)
	schoolID := common.EffectiveSchoolID(r)
	placementID := chi.URLParam(r, "id")

	var body struct {
		Assessor string `json:"assessor"`
		Items    []struct {
			ComponentID string `json:"component_id"`
			ScoreX100   int    `json:"score_x100"`
		} `json:"items"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		common.WriteError(w, http.StatusBadRequest, "Body tidak valid")
		return
	}

	if body.Assessor != AssessorIndustry && body.Assessor != AssessorSchool {
		common.WriteError(w, http.StatusBadRequest, "Assessor harus 'industry' atau 'school'")
		return
	}

	pl, err := h.Store.GetPlacement(r.Context(), schoolID, placementID)
	if err != nil {
		common.WriteError(w, http.StatusNotFound, "Penempatan tidak ditemukan")
		return
	}

	// Verify assessor matches caller's role (FR-141-002)
	if body.Assessor == AssessorIndustry {
		if claims.ActiveRole != common.RolePembimbingIndustri && claims.ActiveRole != common.RoleAdmin && claims.ActiveRole != common.RoleSuperadmin {
			common.WriteError(w, http.StatusForbidden, "Hanya pembimbing industri yang dapat mengisi nilai industri")
			return
		}
		if claims.ActiveRole == common.RolePembimbingIndustri {
			if pl.IndustryMentorID == nil {
				common.WriteError(w, http.StatusForbidden, "Pembimbing industri belum ditetapkan pada penempatan ini")
				return
			}
			im, _ := h.Store.GetIndustryMentor(r.Context(), schoolID, *pl.IndustryMentorID)
			if im == nil || im.UserID != claims.UserID {
				common.WriteError(w, http.StatusForbidden, "Hanya pembimbing industri penempatan ini yang dapat mengisi nilai")
				return
			}
		}
	} else if body.Assessor == AssessorSchool {
		if claims.ActiveRole == common.RolePembimbingIndustri {
			common.WriteError(w, http.StatusForbidden, "Pembimbing industri tidak memiliki akses mengisi nilai sekolah")
			return
		}
		if claims.ActiveRole != common.RoleKoordinatorPKL && claims.ActiveRole != common.RoleAdmin && claims.ActiveRole != common.RoleSuperadmin {
			if pl.SchoolMentorUserID != claims.UserID {
				common.WriteError(w, http.StatusForbidden, "Hanya pembimbing sekolah penempatan ini yang dapat mengisi nilai sekolah")
				return
			}
		}
	}

	var scores []Score
	for _, it := range body.Items {
		if it.ScoreX100 < 0 || it.ScoreX100 > 10000 {
			common.WriteError(w, http.StatusBadRequest, "Nilai harus berada di rentang 0 sampai 100 (0..10000)")
			return
		}
		scores = append(scores, Score{
			SchoolID:    schoolID,
			PlacementID: placementID,
			ComponentID: it.ComponentID,
			ScoreX100:   it.ScoreX100,
		})
	}

	result, err := h.Store.PutScores(r.Context(), schoolID, placementID, body.Assessor, scores, claims.UserID)
	if err != nil {
		common.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}

	h.audit(r.Context(), schoolID, "scores", placementID, "update", claims, nil, map[string]any{"assessor": body.Assessor, "items_count": len(scores)}, "")

	// Notify coordinator if assessment is fully complete (§4.2)
	if result != nil && result.FinalX100 != nil && h.Client != nil {
		// Notifies koordinator_pkl (via broadcast or audit)
	}

	var formattedFinal string
	if result != nil && result.FinalX100 != nil {
		formattedFinal = FormatScore(*result.FinalX100)
	}

	common.WriteSuccess(w, map[string]any{
		"result":          result,
		"formatted_final": formattedFinal,
	}, nil)
}

func (h *Handler) FinalizeResult(w http.ResponseWriter, r *http.Request) {
	claims, schoolID := common.RequireRole(w, r, common.RoleKoordinatorPKL, common.RoleAdmin, common.RoleSuperadmin)
	if claims == nil {
		return
	}
	placementID := chi.URLParam(r, "id")

	res, err := h.Store.FinalizeResult(r.Context(), schoolID, placementID, claims.UserID)
	if err != nil {
		common.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}

	h.audit(r.Context(), schoolID, "result", placementID, "finalize", claims, nil, map[string]any{"final_x100": res.FinalX100}, "")
	common.WriteSuccess(w, res, nil)
}

// ---------------------------------------------------------------------------
// 9. Certificates
// ---------------------------------------------------------------------------

func (h *Handler) IssueCertificate(w http.ResponseWriter, r *http.Request) {
	claims, schoolID := common.RequireRole(w, r, common.RoleKoordinatorPKL, common.RoleAdmin, common.RoleSuperadmin)
	if claims == nil {
		return
	}
	placementID := chi.URLParam(r, "id")

	pl, err := h.Store.GetPlacement(r.Context(), schoolID, placementID)
	if err != nil {
		common.WriteError(w, http.StatusNotFound, "Penempatan tidak ditemukan")
		return
	}
	res, err := h.Store.GetResult(r.Context(), schoolID, placementID)
	if err != nil {
		common.WriteError(w, http.StatusBadRequest, "Hasil nilai PKL belum ada")
		return
	}
	if res == nil || !CanIssueCertificate(*pl, *res) {
		common.WriteError(w, http.StatusBadRequest, "Sertifikat hanya dapat diterbitkan untuk penempatan yang sudah selesai (completed) dan nilai telah difinalisasi")
		return
	}

	// Resolve student name and details
	studentName := pl.StudentName
	if studentName == "" && h.Client != nil {
		names, _ := h.Client.UserNames(r.Context(), schoolID, []string{pl.StudentID})
		if n, ok := names[pl.StudentID]; ok {
			studentName = n
		}
	}
	if studentName == "" {
		studentName = "Siswa SMK"
	}

	finalScoreStr := "0"
	if res.FinalX100 != nil {
		finalScoreStr = FormatScore(*res.FinalX100)
	}

	// Generate certificate PDF
	pdfBytes := BuildCertificatePDF(CertificateDetails{
		CertificateNo: "PREVIEW", // will be updated with actual cert no
		SchoolName:    "SMK Teratai Putih Global",
		StudentName:   studentName,
		StudentNIS:    pl.StudentNIS,
		MajorName:     pl.MajorName,
		PartnerName:   pl.PartnerName,
		FinalScoreStr: finalScoreStr,
		StartsOn:      pl.StartsOn,
		EndsOn:        pl.EndsOn,
		IssuedDate:    time.Now(),
		IssuedBy:      claims.UserID,
	})

	certFilename := fmt.Sprintf("%s_%s_%d.pdf", schoolID, placementID, time.Now().Unix())
	objectKey := fmt.Sprintf("pkl/certificates/%s", certFilename)
	dstPath := filepath.Join(h.StoragePath, objectKey)
	_ = os.MkdirAll(filepath.Dir(dstPath), 0755)
	if err := os.WriteFile(dstPath, pdfBytes, 0644); err != nil {
		common.WriteError(w, http.StatusInternalServerError, "Gagal menyimpan file PDF sertifikat")
		return
	}

	cert, err := h.Store.IssueCertificate(r.Context(), schoolID, placementID, claims.UserID, objectKey)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "Gagal menerbitkan sertifikat: "+err.Error())
		return
	}

	// Re-generate PDF with final certificate_no
	finalPdfBytes := BuildCertificatePDF(CertificateDetails{
		CertificateNo: cert.CertificateNo,
		SchoolName:    "SMK Teratai Putih Global",
		StudentName:   studentName,
		StudentNIS:    pl.StudentNIS,
		MajorName:     pl.MajorName,
		PartnerName:   pl.PartnerName,
		FinalScoreStr: finalScoreStr,
		StartsOn:      pl.StartsOn,
		EndsOn:        pl.EndsOn,
		IssuedDate:    cert.IssuedAt,
		IssuedBy:      claims.UserID,
	})
	_ = os.WriteFile(dstPath, finalPdfBytes, 0644)

	h.audit(r.Context(), schoolID, "certificate", cert.ID, "issue", claims, nil, map[string]any{"certificate_no": cert.CertificateNo}, "")

	// Notify student (§4.2)
	if h.Client != nil {
		_ = h.Client.Notify(r.Context(), client.Notification{
			SchoolID: schoolID,
			UserID:   pl.StudentID,
			Type:     "pkl_certificate",
			Title:    "Sertifikat PKL Telah Terbit",
			Body:     fmt.Sprintf("Sertifikat PKL Anda dengan nomor %s telah diterbitkan.", cert.CertificateNo),
			Data:     map[string]any{"placement_id": pl.ID, "certificate_id": cert.ID, "certificate_no": cert.CertificateNo},
		}, fmt.Sprintf("studentlife:pkl_cert:%s:%s", cert.ID, pl.StudentID))
	}

	common.WriteSuccess(w, cert, nil)
}

func (h *Handler) GetCertificateFile(w http.ResponseWriter, r *http.Request) {
	claims := common.FromCtx(r)
	schoolID := common.EffectiveSchoolID(r)
	certID := chi.URLParam(r, "id")

	cert, err := h.Store.GetCertificate(r.Context(), schoolID, certID)
	if err != nil {
		common.WriteError(w, http.StatusNotFound, "Sertifikat tidak ditemukan")
		return
	}

	pl, err := h.Store.GetPlacement(r.Context(), schoolID, cert.PlacementID)
	if err != nil {
		common.WriteError(w, http.StatusNotFound, "Penempatan tidak ditemukan")
		return
	}

	// Access: koordinator_pkl, admin, superadmin, or student owner
	if claims.ActiveRole == common.RoleSiswa && pl.StudentID != claims.UserID {
		common.WriteError(w, http.StatusForbidden, "Tidak memiliki izin mengunduh sertifikat ini")
		return
	}

	filePath := filepath.Join(h.StoragePath, cert.ObjectKey)
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		common.WriteError(w, http.StatusNotFound, "File fisik sertifikat tidak ditemukan")
		return
	}

	w.Header().Set("Content-Type", "application/pdf")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`inline; filename="sertifikat_%s.pdf"`, cert.CertificateNo))
	http.ServeFile(w, r, filePath)
}

// ---------------------------------------------------------------------------
// 10. Supervision
// ---------------------------------------------------------------------------

func (h *Handler) ListMySupervision(w http.ResponseWriter, r *http.Request) {
	claims := common.FromCtx(r)
	schoolID := common.EffectiveSchoolID(r)

	list, err := h.Store.ListSupervision(r.Context(), schoolID, claims.UserID, claims.ActiveRole)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "Gagal mengambil data bimbingan")
		return
	}

	// Resolve student names
	if h.Client != nil && len(list) > 0 {
		var sids []string
		for _, item := range list {
			sids = append(sids, item.StudentID)
		}
		names, _ := h.Client.UserNames(r.Context(), schoolID, sids)
		for i := range list {
			if n, ok := names[list[i].StudentID]; ok {
				list[i].StudentName = n
			}
		}
	}

	common.WriteSuccess(w, list, nil)
}

// ---------------------------------------------------------------------------
// 11. Student Portal Endpoints
// ---------------------------------------------------------------------------

func (h *Handler) GetMyPlacement(w http.ResponseWriter, r *http.Request) {
	claims := common.FromCtx(r)
	schoolID := common.EffectiveSchoolID(r)

	pl, err := h.Store.GetPlacementByStudent(r.Context(), schoolID, claims.UserID)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			common.WriteError(w, http.StatusNotFound, "Anda belum memiliki penempatan PKL aktif")
			return
		}
		common.WriteError(w, http.StatusInternalServerError, "Gagal mengambil data penempatan")
		return
	}

	res, _ := h.Store.GetResult(r.Context(), schoolID, pl.ID)
	cert, _ := h.Store.GetCertificateByPlacement(r.Context(), schoolID, pl.ID)

	common.WriteSuccess(w, map[string]any{
		"placement":   pl,
		"result":      res,
		"certificate": cert,
	}, nil)
}

func (h *Handler) StudentCheckIn(w http.ResponseWriter, r *http.Request) {
	claims := common.FromCtx(r)
	schoolID := common.EffectiveSchoolID(r)

	pl, err := h.Store.GetPlacementByStudent(r.Context(), schoolID, claims.UserID)
	if err != nil {
		common.WriteError(w, http.StatusNotFound, "Anda belum memiliki penempatan PKL aktif")
		return
	}

	now := time.Now()
	att, err := h.Store.RecordCheckIn(r.Context(), schoolID, pl.ID, now)
	if err != nil {
		if errors.Is(err, ErrAlreadyScored) {
			common.WriteError(w, http.StatusConflict, "Anda sudah mencatat kehadiran untuk hari ini")
			return
		}
		common.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	common.WriteSuccess(w, att, nil)
}

func (h *Handler) StudentCheckOut(w http.ResponseWriter, r *http.Request) {
	claims := common.FromCtx(r)
	schoolID := common.EffectiveSchoolID(r)

	pl, err := h.Store.GetPlacementByStudent(r.Context(), schoolID, claims.UserID)
	if err != nil {
		common.WriteError(w, http.StatusNotFound, "Anda belum memiliki penempatan PKL aktif")
		return
	}

	now := time.Now()
	att, err := h.Store.RecordCheckOut(r.Context(), schoolID, pl.ID, now)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			common.WriteError(w, http.StatusBadRequest, "Belum ada presensi masuk hari ini untuk di-checkout")
			return
		}
		common.WriteError(w, http.StatusInternalServerError, "Gagal mencatat checkout")
		return
	}
	common.WriteSuccess(w, att, nil)
}

func (h *Handler) StudentSubmitLeave(w http.ResponseWriter, r *http.Request) {
	claims := common.FromCtx(r)
	schoolID := common.EffectiveSchoolID(r)

	pl, err := h.Store.GetPlacementByStudent(r.Context(), schoolID, claims.UserID)
	if err != nil {
		common.WriteError(w, http.StatusNotFound, "Anda belum memiliki penempatan PKL aktif")
		return
	}

	var status, note string
	var workDate time.Time
	var attachmentKey *string

	if strings.Contains(r.Header.Get("Content-Type"), "multipart/form-data") {
		if err := r.ParseMultipartForm(10 << 20); err != nil {
			common.WriteError(w, http.StatusBadRequest, "File terlalu besar")
			return
		}
		status = strings.TrimSpace(r.FormValue("status"))
		note = strings.TrimSpace(r.FormValue("note"))
		wdStr := strings.TrimSpace(r.FormValue("work_date"))
		wd, err := time.Parse("2006-01-02", wdStr)
		if err != nil {
			wd = time.Now()
		}
		workDate = wd

		file, header, err := r.FormFile("attachment")
		if err == nil && file != nil {
			defer file.Close()
			fn := fmt.Sprintf("%s_%s_%s", schoolID, pl.ID, filepath.Base(header.Filename))
			key := fmt.Sprintf("pkl/leaves/%s", fn)
			dstPath := filepath.Join(h.StoragePath, key)
			_ = os.MkdirAll(filepath.Dir(dstPath), 0755)
			dst, err := os.Create(dstPath)
			if err == nil {
				defer dst.Close()
				_, _ = io.Copy(dst, file)
				attachmentKey = &key
			}
		}
	} else {
		var body struct {
			Status        string  `json:"status"`
			WorkDate      string  `json:"work_date"`
			Note          string  `json:"note"`
			AttachmentKey *string `json:"attachment_key"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			common.WriteError(w, http.StatusBadRequest, "Body tidak valid")
			return
		}
		status = strings.TrimSpace(body.Status)
		note = strings.TrimSpace(body.Note)
		wd, err := time.Parse("2006-01-02", body.WorkDate)
		if err != nil {
			wd = time.Now()
		}
		workDate = wd
		attachmentKey = body.AttachmentKey
	}

	if status != AttendanceStatusIzin && status != AttendanceStatusSakit {
		common.WriteError(w, http.StatusBadRequest, "Status harus 'izin' atau 'sakit'")
		return
	}

	att, err := h.Store.SubmitLeave(r.Context(), schoolID, pl.ID, workDate, status, note, attachmentKey)
	if err != nil {
		if errors.Is(err, ErrAlreadyScored) {
			common.WriteError(w, http.StatusConflict, "Presensi untuk tanggal ini sudah diajukan")
			return
		}
		common.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	common.WriteSuccess(w, att, nil)
}

func (h *Handler) StudentListJournals(w http.ResponseWriter, r *http.Request) {
	claims := common.FromCtx(r)
	schoolID := common.EffectiveSchoolID(r)

	pl, err := h.Store.GetPlacementByStudent(r.Context(), schoolID, claims.UserID)
	if err != nil {
		common.WriteError(w, http.StatusNotFound, "Anda belum memiliki penempatan PKL")
		return
	}

	list, err := h.Store.ListJournals(r.Context(), schoolID, pl.ID)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "Gagal mengambil daftar jurnal")
		return
	}
	common.WriteSuccess(w, list, nil)
}

func (h *Handler) StudentCreateJournal(w http.ResponseWriter, r *http.Request) {
	claims := common.FromCtx(r)
	schoolID := common.EffectiveSchoolID(r)

	pl, err := h.Store.GetPlacementByStudent(r.Context(), schoolID, claims.UserID)
	if err != nil {
		common.WriteError(w, http.StatusNotFound, "Anda belum memiliki penempatan PKL aktif")
		return
	}

	var entryDate time.Time
	var activity string
	var photos []string

	if strings.Contains(r.Header.Get("Content-Type"), "multipart/form-data") {
		if err := r.ParseMultipartForm(15 << 20); err != nil {
			common.WriteError(w, http.StatusBadRequest, "Form terlalu besar")
			return
		}
		activity = strings.TrimSpace(r.FormValue("activity"))
		edStr := strings.TrimSpace(r.FormValue("entry_date"))
		ed, err := time.Parse("2006-01-02", edStr)
		if err != nil {
			ed = time.Now()
		}
		entryDate = ed

		if form := r.MultipartForm; form != nil {
			for _, headers := range form.File {
				for _, header := range headers {
					file, err := header.Open()
					if err == nil {
						defer file.Close()
						fn := fmt.Sprintf("%s_%s_%d_%s", schoolID, pl.ID, time.Now().UnixNano(), filepath.Base(header.Filename))
						key := fmt.Sprintf("pkl/journals/%s", fn)
						dstPath := filepath.Join(h.StoragePath, key)
						_ = os.MkdirAll(filepath.Dir(dstPath), 0755)
						dst, err := os.Create(dstPath)
						if err == nil {
							defer dst.Close()
							_, _ = io.Copy(dst, file)
							photos = append(photos, "/api/v1/pkl/files/journals/"+fn)
						}
					}
				}
			}
		}
	} else {
		var body struct {
			EntryDate string   `json:"entry_date"`
			Activity  string   `json:"activity"`
			Photos    []string `json:"photos"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			common.WriteError(w, http.StatusBadRequest, "Body tidak valid")
			return
		}
		activity = strings.TrimSpace(body.Activity)
		ed, err := time.Parse("2006-01-02", body.EntryDate)
		if err != nil {
			ed = time.Now()
		}
		entryDate = ed
		photos = body.Photos
	}

	if activity == "" {
		common.WriteError(w, http.StatusBadRequest, "Uraian kegiatan jurnal wajib diisi")
		return
	}

	j, err := h.Store.CreateJournal(r.Context(), Journal{
		SchoolID:    schoolID,
		PlacementID: pl.ID,
		EntryDate:   entryDate,
		Activity:    activity,
		Photos:      photos,
	})
	if err != nil {
		if errors.Is(err, ErrAlreadyExists) {
			common.WriteError(w, http.StatusConflict, "Jurnal untuk tanggal ini sudah ada")
			return
		}
		common.WriteError(w, http.StatusInternalServerError, "Gagal membuat jurnal")
		return
	}
	common.WriteSuccess(w, j, nil)
}

func (h *Handler) StudentUpdateJournal(w http.ResponseWriter, r *http.Request) {
	claims := common.FromCtx(r)
	schoolID := common.EffectiveSchoolID(r)
	id := chi.URLParam(r, "id")

	var body struct {
		Activity string   `json:"activity"`
		Photos   []string `json:"photos"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		common.WriteError(w, http.StatusBadRequest, "Body tidak valid")
		return
	}

	j, err := h.Store.GetJournal(r.Context(), schoolID, id)
	if err != nil {
		common.WriteError(w, http.StatusNotFound, "Jurnal tidak ditemukan")
		return
	}

	pl, err := h.Store.GetPlacement(r.Context(), schoolID, j.PlacementID)
	if err != nil || pl.StudentID != claims.UserID {
		common.WriteError(w, http.StatusForbidden, "Tidak memiliki izin mengubah jurnal ini")
		return
	}

	j.Activity = body.Activity
	if body.Photos != nil {
		j.Photos = body.Photos
	}

	updated, err := h.Store.UpdateJournal(r.Context(), *j)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "Gagal memperbarui jurnal")
		return
	}
	common.WriteSuccess(w, updated, nil)
}

func (h *Handler) StudentUploadFinalReport(w http.ResponseWriter, r *http.Request) {
	claims := common.FromCtx(r)
	schoolID := common.EffectiveSchoolID(r)

	pl, err := h.Store.GetPlacementByStudent(r.Context(), schoolID, claims.UserID)
	if err != nil {
		common.WriteError(w, http.StatusNotFound, "Anda belum memiliki penempatan PKL")
		return
	}

	if err := r.ParseMultipartForm(25 << 20); err != nil {
		common.WriteError(w, http.StatusBadRequest, "File laporan terlalu besar (maks 25MB)")
		return
	}

	file, header, err := r.FormFile("file")
	if err != nil || file == nil {
		common.WriteError(w, http.StatusBadRequest, "File laporan wajib diunggah")
		return
	}
	defer file.Close()

	fn := header.Filename
	mime := header.Header.Get("Content-Type")
	key := fmt.Sprintf("pkl/reports/%s_%s_%d_%s", schoolID, pl.ID, time.Now().Unix(), filepath.Base(fn))
	dstPath := filepath.Join(h.StoragePath, key)
	_ = os.MkdirAll(filepath.Dir(dstPath), 0755)

	dst, err := os.Create(dstPath)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "Gagal menyimpan file laporan")
		return
	}
	defer dst.Close()
	if _, err := io.Copy(dst, file); err != nil {
		common.WriteError(w, http.StatusInternalServerError, "Gagal menulis file laporan")
		return
	}

	rep, err := h.Store.CreateFinalReport(r.Context(), FinalReport{
		SchoolID:    schoolID,
		PlacementID: pl.ID,
		ObjectKey:   key,
		FileName:    &fn,
		Mime:        &mime,
	})
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "Gagal mencatat laporan akhir")
		return
	}

	common.WriteSuccess(w, rep, nil)
}

func (h *Handler) StudentGetResult(w http.ResponseWriter, r *http.Request) {
	claims := common.FromCtx(r)
	schoolID := common.EffectiveSchoolID(r)

	pl, err := h.Store.GetPlacementByStudent(r.Context(), schoolID, claims.UserID)
	if err != nil {
		common.WriteError(w, http.StatusNotFound, "Anda belum memiliki penempatan PKL")
		return
	}

	res, _ := h.Store.GetResult(r.Context(), schoolID, pl.ID)
	cert, _ := h.Store.GetCertificateByPlacement(r.Context(), schoolID, pl.ID)
	scores, _ := h.Store.ListScores(r.Context(), schoolID, pl.ID)

	common.WriteSuccess(w, map[string]any{
		"scores":      scores,
		"result":      res,
		"certificate": cert,
	}, nil)
}

// ---------------------------------------------------------------------------
// 12. File Serving
// ---------------------------------------------------------------------------

func (h *Handler) ServeFile(w http.ResponseWriter, r *http.Request) {
	category := chi.URLParam(r, "category")
	filename := chi.URLParam(r, "filename")

	cleanCategory := filepath.Clean(category)
	cleanFilename := filepath.Clean(filename)

	if strings.Contains(cleanCategory, "..") || strings.Contains(cleanFilename, "..") {
		common.WriteError(w, http.StatusBadRequest, "Invalid path")
		return
	}

	fullPath := filepath.Join(h.StoragePath, "pkl", cleanCategory, cleanFilename)
	if _, err := os.Stat(fullPath); os.IsNotExist(err) {
		common.WriteError(w, http.StatusNotFound, "File tidak ditemukan")
		return
	}

	http.ServeFile(w, r, fullPath)
}
