package handler

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"skci/hr/internal/common"
	"skci/hr/internal/model"
	"skci/hr/internal/store"
)

// --- DOCUMENT TYPES ---

func (a *App) ListDocumentTypes(w http.ResponseWriter, r *http.Request) {
	schoolID := a.effectiveSchoolID(r)
	types, err := store.ListDocumentTypes(r.Context(), a.DB, schoolID)
	if err != nil {
		common.Error(w, http.StatusInternalServerError, "Gagal mengambil jenis dokumen")
		return
	}
	common.JSON(w, http.StatusOK, types)
}

func (a *App) CreateDocumentType(w http.ResponseWriter, r *http.Request) {
	schoolID := a.effectiveSchoolID(r)
	var dt model.DocumentType
	if err := decodeJSON(r, &dt); err != nil {
		common.Error(w, http.StatusBadRequest, "Format JSON tidak valid")
		return
	}
	if strings.TrimSpace(dt.Code) == "" || strings.TrimSpace(dt.Name) == "" {
		common.Error(w, http.StatusBadRequest, "Kode dan nama jenis dokumen wajib diisi")
		return
	}
	dt.SchoolID = schoolID

	if err := store.CreateDocumentType(r.Context(), a.DB, &dt); err != nil {
		common.Error(w, http.StatusInternalServerError, "Gagal membuat jenis dokumen: "+err.Error())
		return
	}
	common.JSON(w, http.StatusCreated, dt)
}

func (a *App) UpdateDocumentType(w http.ResponseWriter, r *http.Request) {
	schoolID := a.effectiveSchoolID(r)
	id := chi.URLParam(r, "id")

	var dt model.DocumentType
	if err := decodeJSON(r, &dt); err != nil {
		common.Error(w, http.StatusBadRequest, "Format JSON tidak valid")
		return
	}
	dt.ID = id
	dt.SchoolID = schoolID

	if err := store.UpdateDocumentType(r.Context(), a.DB, &dt); err != nil {
		common.Error(w, http.StatusInternalServerError, "Gagal memperbarui jenis dokumen")
		return
	}
	common.JSON(w, http.StatusOK, dt)
}

func (a *App) DeleteDocumentType(w http.ResponseWriter, r *http.Request) {
	schoolID := a.effectiveSchoolID(r)
	id := chi.URLParam(r, "id")

	if err := store.DeleteDocumentType(r.Context(), a.DB, schoolID, id); err != nil {
		common.Error(w, http.StatusInternalServerError, "Gagal menghapus jenis dokumen")
		return
	}
	common.JSON(w, http.StatusOK, map[string]string{"message": "Jenis dokumen berhasil dihapus"})
}

// --- REQUIRED DOCUMENTS ---

func (a *App) ListRequiredDocuments(w http.ResponseWriter, r *http.Request) {
	schoolID := a.effectiveSchoolID(r)
	empType := r.URL.Query().Get("employment_type")
	reqs, err := store.ListRequiredDocuments(r.Context(), a.DB, schoolID, empType)
	if err != nil {
		common.Error(w, http.StatusInternalServerError, "Gagal memuat aturan dokumen wajib")
		return
	}
	common.JSON(w, http.StatusOK, reqs)
}

func (a *App) CreateRequiredDocument(w http.ResponseWriter, r *http.Request) {
	schoolID := a.effectiveSchoolID(r)
	var rd model.RequiredDocument
	if err := decodeJSON(r, &rd); err != nil {
		common.Error(w, http.StatusBadRequest, "Format JSON tidak valid")
		return
	}
	if rd.DocumentTypeID == "" || rd.EmploymentType == "" {
		common.Error(w, http.StatusBadRequest, "Jenis dokumen dan tipe pegawai wajib diisi")
		return
	}
	rd.SchoolID = schoolID

	if err := store.CreateRequiredDocument(r.Context(), a.DB, &rd); err != nil {
		common.Error(w, http.StatusInternalServerError, "Gagal menyimpan dokumen wajib: "+err.Error())
		return
	}
	common.JSON(w, http.StatusCreated, rd)
}

func (a *App) DeleteRequiredDocument(w http.ResponseWriter, r *http.Request) {
	schoolID := a.effectiveSchoolID(r)
	id := chi.URLParam(r, "id")

	if err := store.DeleteRequiredDocument(r.Context(), a.DB, schoolID, id); err != nil {
		common.Error(w, http.StatusInternalServerError, "Gagal menghapus aturan dokumen wajib")
		return
	}
	common.JSON(w, http.StatusOK, map[string]string{"message": "Dokumen wajib berhasil dihapus"})
}

// --- EMPLOYEE DOCUMENTS ---

func (a *App) ListEmployeeDocuments(w http.ResponseWriter, r *http.Request) {
	schoolID := a.effectiveSchoolID(r)
	empID := chi.URLParam(r, "id")

	docs, err := store.ListEmployeeDocuments(r.Context(), a.DB, schoolID, empID)
	if err != nil {
		common.Error(w, http.StatusInternalServerError, "Gagal memuat dokumen pegawai")
		return
	}
	common.JSON(w, http.StatusOK, docs)
}

func (a *App) UploadEmployeeDocument(w http.ResponseWriter, r *http.Request) {
	schoolID := a.effectiveSchoolID(r)
	empID := chi.URLParam(r, "id")
	claims := common.GetClaims(r.Context())

	// Limit multipart body to 15MB
	if err := r.ParseMultipartForm(15 << 20); err != nil {
		common.Error(w, http.StatusBadRequest, "Gagal membaca form multipart")
		return
	}

	docTypeID := r.FormValue("document_type_id")
	if docTypeID == "" {
		common.Error(w, http.StatusBadRequest, "Jenis dokumen wajib dipilih")
		return
	}
	expiryDate := r.FormValue("expiry_date")

	file, header, err := r.FormFile("file")
	if err != nil {
		common.Error(w, http.StatusBadRequest, "File dokumen wajib diunggah")
		return
	}
	defer file.Close()

	if header.Size > 10<<20 { // 10MB limit
		common.Error(w, http.StatusBadRequest, "Ukuran file maksimal 10MB")
		return
	}

	data, err := io.ReadAll(file)
	if err != nil {
		common.Error(w, http.StatusInternalServerError, "Gagal membaca konten file")
		return
	}

	storageKey := fmt.Sprintf("docs/%s/%s/%d-%s", schoolID, empID, time.Now().Unix(), header.Filename)
	contentType := header.Header.Get("Content-Type")
	if contentType == "" || contentType == "application/octet-stream" {
		ext := strings.ToLower(filepath.Ext(header.Filename))
		switch ext {
		case ".pdf":
			contentType = "application/pdf"
		case ".jpg", ".jpeg":
			contentType = "image/jpeg"
		case ".png":
			contentType = "image/png"
		case ".csv":
			contentType = "text/csv"
		case ".xlsx":
			contentType = "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"
		default:
			contentType = "application/octet-stream"
		}
	}

	if _, err := a.Storage.Put(r.Context(), storageKey, contentType, bytes.NewReader(data), header.Size); err != nil {
		common.Error(w, http.StatusInternalServerError, "Gagal menyimpan file ke penyimpanan")
		return
	}

	doc := model.EmployeeDocument{
		SchoolID:       schoolID,
		EmployeeID:     empID,
		DocumentTypeID: docTypeID,
		ObjectKey:      storageKey,
		FileName:       header.Filename,
		SizeBytes:      header.Size,
		MIME:           contentType,
	}
	if expiryDate != "" {
		doc.ExpiresOn = &expiryDate
	}
	if claims != nil && claims.Subject != "" {
		doc.UploadedBy = &claims.Subject
	}

	if err := store.CreateEmployeeDocument(r.Context(), a.DB, &doc); err != nil {
		_ = a.Storage.Delete(r.Context(), storageKey)
		common.Error(w, http.StatusInternalServerError, "Gagal mencatat dokumen di database")
		return
	}

	common.JSON(w, http.StatusCreated, doc)
}

func (a *App) DownloadDocument(w http.ResponseWriter, r *http.Request) {
	schoolID := a.effectiveSchoolID(r)
	docID := chi.URLParam(r, "id")

	doc, err := store.GetEmployeeDocument(r.Context(), a.DB, schoolID, docID)
	if err != nil {
		common.Error(w, http.StatusNotFound, "Dokumen tidak ditemukan")
		return
	}

	// Verify authorization: Superadmin, HR Admin, HR Manager, or owner employee
	claims := common.GetClaims(r.Context())
	if claims != nil && claims.Role == common.RolePegawai {
		emp, err := store.GetEmployeeByUserID(r.Context(), a.DB, schoolID, claims.Subject)
		if err != nil || emp.ID != doc.EmployeeID {
			common.Error(w, http.StatusForbidden, "Tidak memiliki izin mengakses dokumen ini")
			return
		}
	}

	reader, err := a.Storage.Open(r.Context(), doc.ObjectKey)
	if err != nil {
		common.Error(w, http.StatusNotFound, "File fisik tidak ditemukan di penyimpanan")
		return
	}
	defer reader.Close()

	w.Header().Set("Content-Type", doc.MIME)
	w.Header().Set("Content-Disposition", fmt.Sprintf(`inline; filename="%s"`, doc.FileName))
	w.Header().Set("Content-Length", fmt.Sprintf("%d", doc.SizeBytes))
	w.WriteHeader(http.StatusOK)
	_, _ = io.Copy(w, reader)
}

func (a *App) DeleteDocument(w http.ResponseWriter, r *http.Request) {
	schoolID := a.effectiveSchoolID(r)
	docID := chi.URLParam(r, "id")

	doc, err := store.GetEmployeeDocument(r.Context(), a.DB, schoolID, docID)
	if err != nil {
		common.Error(w, http.StatusNotFound, "Dokumen tidak ditemukan")
		return
	}

	if err := store.DeleteEmployeeDocument(r.Context(), a.DB, schoolID, docID); err != nil {
		common.Error(w, http.StatusInternalServerError, "Gagal menghapus dokumen")
		return
	}

	_ = a.Storage.Delete(r.Context(), doc.ObjectKey)
	common.JSON(w, http.StatusOK, map[string]string{"message": "Dokumen berhasil dihapus"})
}

// --- DOCUMENT COMPLETENESS ---

func (a *App) GetDocumentCompleteness(w http.ResponseWriter, r *http.Request) {
	schoolID := a.effectiveSchoolID(r)
	empID := r.URL.Query().Get("employee_id")

	if empID != "" {
		emp, err := store.GetEmployeeByID(r.Context(), a.DB, schoolID, empID)
		if err != nil {
			common.Error(w, http.StatusNotFound, "Pegawai tidak ditemukan")
			return
		}
		empType := "kontrak"
		if emp.CurrentEmploymentType != nil {
			empType = *emp.CurrentEmploymentType
		}

		required, err := store.ListRequiredDocuments(r.Context(), a.DB, schoolID, empType)
		if err != nil {
			common.Error(w, http.StatusInternalServerError, "Gagal memuat dokumen wajib")
			return
		}

		uploaded, err := store.ListEmployeeDocuments(r.Context(), a.DB, schoolID, empID)
		if err != nil {
			common.Error(w, http.StatusInternalServerError, "Gagal memuat dokumen yang telah diunggah")
			return
		}

		isComplete, missing := model.DocumentCompleteness(required, uploaded, time.Now())
		pct := 100
		if len(required) > 0 {
			validCount := len(required) - len(missing)
			if validCount < 0 {
				validCount = 0
			}
			pct = int(float64(validCount) / float64(len(required)) * 100)
		}
		common.JSON(w, http.StatusOK, map[string]any{
			"employee_id":       empID,
			"is_complete":       isComplete,
			"completeness_pct":  pct,
			"missing_count":     len(missing),
			"missing_documents": missing,
		})
		return
	}

	// Completeness summary
	summary, err := store.GetDocumentsCompletenessSummary(r.Context(), a.DB, schoolID)
	if err != nil {
		common.Error(w, http.StatusInternalServerError, "Gagal memuat ringkasan kelengkapan dokumen")
		return
	}
	common.JSON(w, http.StatusOK, summary)
}

// --- DOCUMENT REQUESTS ---

func (a *App) ListDocumentRequests(w http.ResponseWriter, r *http.Request) {
	schoolID := a.effectiveSchoolID(r)
	page, perPage := parsePagination(r)
	empID := r.URL.Query().Get("employee_id")
	status := r.URL.Query().Get("status")

	reqs, total, err := store.ListDocumentRequests(r.Context(), a.DB, schoolID, empID, status, page, perPage)
	if err != nil {
		common.Error(w, http.StatusInternalServerError, "Gagal memuat permintaan dokumen")
		return
	}
	common.JSONWithMeta(w, http.StatusOK, reqs, common.Meta{
		Page:       page,
		PerPage:    perPage,
		Total:      total,
		TotalPages: (total + perPage - 1) / perPage,
	})
}

func (a *App) CreateDocumentRequest(w http.ResponseWriter, r *http.Request) {
	schoolID := a.effectiveSchoolID(r)
	var dr model.DocumentRequest
	if err := decodeJSON(r, &dr); err != nil {
		common.Error(w, http.StatusBadRequest, "Format JSON tidak valid")
		return
	}
	if dr.EmployeeID == "" || dr.DocumentTypeID == "" {
		common.Error(w, http.StatusBadRequest, "Pegawai dan jenis dokumen wajib dipilih")
		return
	}
	dr.SchoolID = schoolID

	if err := store.CreateDocumentRequest(r.Context(), a.DB, &dr); err != nil {
		common.Error(w, http.StatusInternalServerError, "Gagal membuat permintaan dokumen: "+err.Error())
		return
	}
	common.JSON(w, http.StatusCreated, dr)
}

func (a *App) UpdateDocumentRequest(w http.ResponseWriter, r *http.Request) {
	schoolID := a.effectiveSchoolID(r)
	id := chi.URLParam(r, "id")

	var body struct {
		Status string `json:"status"`
	}
	if err := decodeJSON(r, &body); err != nil {
		common.Error(w, http.StatusBadRequest, "Format JSON tidak valid")
		return
	}

	if err := store.UpdateDocumentRequestStatus(r.Context(), a.DB, schoolID, id, body.Status); err != nil {
		common.Error(w, http.StatusInternalServerError, "Gagal memperbarui status permintaan dokumen")
		return
	}
	common.JSON(w, http.StatusOK, map[string]string{"message": "Status berhasil diperbarui"})
}
