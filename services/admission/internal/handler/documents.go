package handler

import (
	"database/sql"
	"io"
	"net/http"
	"strconv"
	"strings"

	"skci/admission/internal/common"
)

// docReviewDBToAPI maps the stored review decision to the frontend vocabulary.
func docReviewDBToAPI(status string) string {
	switch status {
	case "accepted":
		return "verified"
	case "correction":
		return "correction_required"
	case "rejected":
		return "rejected"
	default:
		return "pending"
	}
}

// docReviewAPIToDB maps the frontend decision to the stored value.
func docReviewAPIToDB(status string) (string, bool) {
	switch status {
	case "verified", "accepted":
		return "accepted", true
	case "correction_required", "correction":
		return "correction", true
	case "rejected":
		return "rejected", true
	default:
		return "", false
	}
}

// ListDocuments is GET /guardian/applications/{id}/documents.
func (a *App) ListDocuments(w http.ResponseWriter, r *http.Request) {
	c := common.Guardian(r)
	applicationID := param(r, "id")
	if !a.guardianOwnsApplication(r, c.SchoolID, c.GuardianID, applicationID) {
		common.WriteError(w, http.StatusNotFound, "application not found")
		return
	}
	docs, err := a.applicationDocuments(r, c.SchoolID, applicationID)
	if err != nil {
		writeErr(w, err)
		return
	}
	reqs, err := a.documentRequirements(r, c.SchoolID, applicationID)
	if err != nil {
		writeErr(w, err)
		return
	}
	common.WriteJSON(w, http.StatusOK, map[string]any{"documents": docs, "requirements": reqs}, nil)
}

func (a *App) guardianOwnsApplication(r *http.Request, schoolID, guardianID, applicationID string) bool {
	var ok bool
	err := a.DB.QueryRowContext(r.Context(),
		`SELECT EXISTS(SELECT 1 FROM applications a JOIN applicants ap ON ap.id=a.applicant_id
		   WHERE a.id=$1 AND a.school_id=$2 AND ap.guardian_id=$3)`,
		applicationID, schoolID, guardianID).Scan(&ok)
	return err == nil && ok
}

func (a *App) applicationDocuments(r *http.Request, schoolID, applicationID string) ([]map[string]any, error) {
	rows, err := a.DB.QueryContext(r.Context(),
		`SELECT d.id,d.requirement_id,COALESCE(d.file_name,''),COALESCE(d.mime,''),d.size_bytes,d.created_at,
		        COALESCE(rev.status,''),COALESCE(rev.notes,'')
		 FROM application_documents d
		 LEFT JOIN LATERAL (
		     SELECT status,notes FROM document_reviews dr WHERE dr.document_id=d.id ORDER BY dr.created_at DESC LIMIT 1
		 ) rev ON true
		 WHERE d.application_id=$1 AND d.school_id=$2
		 ORDER BY d.created_at`, applicationID, schoolID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		var (
			id, requirementID, fileName, mime, reviewStatus, reviewNote string
			size                                                        int64
			uploadedAt                                                  any
		)
		if err := rows.Scan(&id, &requirementID, &fileName, &mime, &size, &uploadedAt, &reviewStatus, &reviewNote); err != nil {
			return nil, err
		}
		doc := map[string]any{
			"id":             id,
			"requirement_id": requirementID,
			"file_name":      fileName,
			"mime_type":      mime,
			"size":           size,
			"uploaded_at":    uploadedAt,
			"status":         docReviewDBToAPI(reviewStatus),
		}
		if reviewNote != "" {
			doc["review_note"] = reviewNote
		}
		out = append(out, doc)
	}
	return out, rows.Err()
}

func (a *App) documentRequirements(r *http.Request, schoolID, applicationID string) ([]map[string]any, error) {
	rows, err := a.DB.QueryContext(r.Context(),
		`SELECT dr.id,dr.name,COALESCE(dr.description,''),dr.allowed_mimes,dr.max_bytes,dr.required
		 FROM document_requirements dr
		 JOIN applications a ON a.period_id=dr.period_id
		 WHERE dr.school_id=$1 AND a.id=$2 AND dr.deleted_at IS NULL
		 ORDER BY dr.name`, schoolID, applicationID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		var (
			id, name, description string
			allowed               textArray
			maxBytes              int64
			required              bool
		)
		if err := rows.Scan(&id, &name, &description, &allowed, &maxBytes, &required); err != nil {
			return nil, err
		}
		out = append(out, map[string]any{
			"id":             id,
			"name":           name,
			"description":    description,
			"accepted_types": allowed,
			"max_size_mb":    int(maxBytes / (1 << 20)),
			"required":       required,
		})
	}
	return out, rows.Err()
}

// UploadDocument is POST /guardian/applications/{id}/documents (multipart:
// requirement_id + file). Documents may only change while the application is
// still a draft.
func (a *App) UploadDocument(w http.ResponseWriter, r *http.Request) {
	c := common.Guardian(r)
	if err := r.ParseMultipartForm(a.Config.MaxUploadBytes); err != nil {
		common.WriteError(w, http.StatusBadRequest, "invalid multipart upload")
		return
	}
	applicationID := param(r, "id")
	requirementID := r.FormValue("requirement_id")
	if requirementID == "" {
		common.WriteError(w, http.StatusBadRequest, "requirement_id is required")
		return
	}
	file, header, err := r.FormFile("file")
	if err != nil {
		common.WriteError(w, http.StatusBadRequest, "file is required")
		return
	}
	defer file.Close()
	mime := header.Header.Get("Content-Type")

	var (
		maxBytes int64
		allowed  textArray
	)
	err = a.DB.QueryRowContext(r.Context(),
		`SELECT dr.max_bytes,dr.allowed_mimes
		 FROM document_requirements dr
		 JOIN applications app ON app.period_id=dr.period_id
		 JOIN applicants ap ON ap.id=app.applicant_id
		 WHERE dr.id=$1 AND app.id=$2 AND app.school_id=$3 AND ap.guardian_id=$4
		   AND app.status='draft' AND dr.deleted_at IS NULL`,
		requirementID, applicationID, c.SchoolID, c.GuardianID).Scan(&maxBytes, &allowed)
	if err != nil {
		common.WriteError(w, http.StatusNotFound, "requirement or editable application not found")
		return
	}
	allowedMIME := false
	for _, v := range allowed {
		if v == mime {
			allowedMIME = true
			break
		}
	}
	if !allowedMIME || header.Size > maxBytes {
		common.WriteError(w, http.StatusUnsupportedMediaType, "file type or size is not allowed")
		return
	}

	key := c.SchoolID + "/applications/" + applicationID + "/" + randomKey()
	obj, err := a.Storage.Put(r.Context(), key, mime, file, header.Size)
	if err != nil {
		common.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}

	var id string
	err = a.DB.QueryRowContext(r.Context(),
		`INSERT INTO application_documents(school_id,application_id,requirement_id,object_key,file_name,mime,size_bytes,sha256,uploaded_by)
		 VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9)
		 ON CONFLICT(application_id,requirement_id) DO UPDATE SET
		     object_key=EXCLUDED.object_key,file_name=EXCLUDED.file_name,mime=EXCLUDED.mime,
		     size_bytes=EXCLUDED.size_bytes,sha256=EXCLUDED.sha256,uploaded_by=EXCLUDED.uploaded_by,created_at=now()
		 WHERE application_documents.locked_at IS NULL
		 RETURNING id`,
		c.SchoolID, applicationID, requirementID, obj.Key, header.Filename, obj.MIME, obj.Size, obj.Checksum, c.GuardianID).Scan(&id)
	if err != nil {
		_ = a.Storage.Delete(r.Context(), obj.Key)
		common.WriteError(w, http.StatusConflict, "document is locked")
		return
	}
	common.WriteJSON(w, http.StatusCreated, map[string]any{
		"id":          id,
		"file_name":   header.Filename,
		"mime_type":   obj.MIME,
		"size":        obj.Size,
		"sha256":      obj.Checksum,
		"status":      "pending",
		"scan_status": "pending",
	}, nil)
}

// DeleteDocument is DELETE /guardian/applications/{id}/documents/{docId}.
func (a *App) DeleteDocument(w http.ResponseWriter, r *http.Request) {
	c := common.Guardian(r)
	applicationID := param(r, "id")
	documentID := param(r, "docId")

	var objectKey string
	err := a.DB.QueryRowContext(r.Context(),
		`SELECT d.object_key
		 FROM application_documents d
		 JOIN applications app ON app.id=d.application_id
		 JOIN applicants ap ON ap.id=app.applicant_id
		 WHERE d.id=$1 AND d.application_id=$2 AND d.school_id=$3 AND ap.guardian_id=$4
		   AND app.status='draft' AND d.locked_at IS NULL`,
		documentID, applicationID, c.SchoolID, c.GuardianID).Scan(&objectKey)
	if err == sql.ErrNoRows {
		common.WriteError(w, http.StatusConflict, "document cannot be deleted")
		return
	}
	if err != nil {
		writeErr(w, err)
		return
	}
	if _, err = a.DB.ExecContext(r.Context(),
		`DELETE FROM application_documents WHERE id=$1 AND application_id=$2`, documentID, applicationID); err != nil {
		writeErr(w, err)
		return
	}
	_ = a.Storage.Delete(r.Context(), objectKey)
	common.WriteJSON(w, http.StatusOK, map[string]any{"id": documentID, "deleted": true}, nil)
}

// DownloadDocument is GET /guardian/applications/{id}/documents/{docId}/download.
func (a *App) DownloadDocument(w http.ResponseWriter, r *http.Request) {
	c := common.Guardian(r)
	var key, mime, checksum string
	var size int64
	err := a.DB.QueryRowContext(r.Context(),
		`SELECT d.object_key,d.mime,d.size_bytes,d.sha256
		 FROM application_documents d
		 JOIN applications app ON app.id=d.application_id
		 JOIN applicants ap ON ap.id=app.applicant_id
		 WHERE d.id=$1 AND d.application_id=$2 AND d.school_id=$3 AND ap.guardian_id=$4`,
		param(r, "docId"), param(r, "id"), c.SchoolID, c.GuardianID).Scan(&key, &mime, &size, &checksum)
	if err != nil {
		common.WriteError(w, http.StatusNotFound, "document not found")
		return
	}
	a.streamObject(w, r, key, mime, checksum, size)
}

// AdminDownloadDocument is GET /admin/documents/{id}/download.
func (a *App) AdminDownloadDocument(w http.ResponseWriter, r *http.Request) {
	if !common.RequireAdmin(w, r) {
		return
	}
	schoolID := common.EffectiveSchoolID(r)
	var key, mime, checksum string
	var size int64
	err := a.DB.QueryRowContext(r.Context(),
		`SELECT object_key,mime,size_bytes,sha256 FROM application_documents WHERE id=$1 AND school_id=$2`,
		param(r, "id"), schoolID).Scan(&key, &mime, &size, &checksum)
	if err != nil {
		common.WriteError(w, http.StatusNotFound, "document not found")
		return
	}
	a.streamObject(w, r, key, mime, checksum, size)
}

func (a *App) streamObject(w http.ResponseWriter, r *http.Request, key, mime, checksum string, size int64) {
	f, err := a.Storage.Open(r.Context(), key)
	if err != nil {
		common.WriteError(w, http.StatusNotFound, "object not found")
		return
	}
	defer f.Close()
	w.Header().Set("Content-Type", mime)
	w.Header().Set("Content-Length", strconv.FormatInt(size, 10))
	if checksum != "" {
		w.Header().Set("ETag", `"`+checksum+`"`)
	}
	w.Header().Set("Content-Disposition", `attachment; filename="document`+strings.ReplaceAll(mime, "/", ".")+`"`)
	_, _ = io.Copy(w, f)
}

// DocumentQueue is GET /admin/documents.
func (a *App) DocumentQueue(w http.ResponseWriter, r *http.Request) {
	if !common.RequireAdmin(w, r) {
		return
	}
	schoolID := common.EffectiveSchoolID(r)
	status := r.URL.Query().Get("status")
	search := strings.TrimSpace(r.URL.Query().Get("search"))
	page, perPage := common.Page(r)

	rows, err := a.DB.QueryContext(r.Context(),
		`SELECT d.id,d.application_id,d.requirement_id,COALESCE(d.file_name,''),COALESCE(d.mime,''),d.size_bytes,d.created_at,
		        ap.full_name,a.registration_number,dr.name,
		        COALESCE(rev.status,''),COALESCE(rev.notes,'')
		 FROM application_documents d
		 JOIN applications a ON a.id=d.application_id
		 JOIN applicants ap ON ap.id=a.applicant_id
		 JOIN document_requirements dr ON dr.id=d.requirement_id
		 LEFT JOIN LATERAL (
		     SELECT status,notes FROM document_reviews rv WHERE rv.document_id=d.id ORDER BY rv.created_at DESC LIMIT 1
		 ) rev ON true
		 WHERE d.school_id=$1
		   AND ($2='' OR $2 = CASE COALESCE(rev.status,'')
		        WHEN 'accepted' THEN 'verified'
		        WHEN 'correction' THEN 'correction_required'
		        WHEN 'rejected' THEN 'rejected'
		        ELSE 'pending' END)
		   AND ($3='' OR ap.full_name ILIKE '%'||$3||'%' OR a.registration_number ILIKE '%'||$3||'%')
		 ORDER BY d.created_at DESC
		 LIMIT $4 OFFSET $5`,
		schoolID, status, search, perPage, common.Offset(page, perPage))
	if err != nil {
		writeErr(w, err)
		return
	}
	defer rows.Close()
	items := []map[string]any{}
	for rows.Next() {
		var (
			id, applicationID, requirementID, fileName, mime   string
			size                                               int64
			uploadedAt                                         any
			applicantName, registrationNumber, requirementName string
			reviewStatus, reviewNote                           string
		)
		if err := rows.Scan(&id, &applicationID, &requirementID, &fileName, &mime, &size, &uploadedAt,
			&applicantName, &registrationNumber, &requirementName, &reviewStatus, &reviewNote); err != nil {
			writeErr(w, err)
			return
		}
		item := map[string]any{
			"id":                  id,
			"application_id":      applicationID,
			"requirement_id":      requirementID,
			"requirement_name":    requirementName,
			"applicant_name":      applicantName,
			"registration_number": registrationNumber,
			"file_name":           fileName,
			"mime_type":           mime,
			"size":                size,
			"uploaded_at":         uploadedAt,
			"status":              docReviewDBToAPI(reviewStatus),
		}
		if reviewNote != "" {
			item["review_note"] = reviewNote
		}
		items = append(items, item)
	}
	common.WriteJSON(w, http.StatusOK, map[string]any{"items": items}, nil)
}

// ReviewDocument is POST /admin/documents/{id}/review.
func (a *App) ReviewDocument(w http.ResponseWriter, r *http.Request) {
	if !common.RequireAdmin(w, r) {
		return
	}
	schoolID := common.EffectiveSchoolID(r)
	claims := common.Admin(r)
	var in struct {
		Status string `json:"status"`
		Note   string `json:"note"`
	}
	if !common.Decode(w, r, &in) {
		return
	}
	dbStatus, ok := docReviewAPIToDB(in.Status)
	if !ok {
		common.WriteError(w, http.StatusUnprocessableEntity, "invalid review status")
		return
	}
	if dbStatus != "accepted" && strings.TrimSpace(in.Note) == "" {
		common.WriteError(w, http.StatusUnprocessableEntity, "a note is required for rejection or correction")
		return
	}
	documentID := param(r, "id")

	var applicationID string
	err := a.DB.QueryRowContext(r.Context(),
		`SELECT application_id FROM application_documents WHERE id=$1 AND school_id=$2`, documentID, schoolID).Scan(&applicationID)
	if err == sql.ErrNoRows {
		common.WriteError(w, http.StatusNotFound, "document not found")
		return
	}
	if err != nil {
		writeErr(w, err)
		return
	}
	if _, err = a.DB.ExecContext(r.Context(),
		`INSERT INTO document_reviews(school_id,document_id,status,notes,reviewer_id) VALUES($1,$2,$3,NULLIF($4,''),$5)`,
		schoolID, documentID, dbStatus, in.Note, claims.UserID); err != nil {
		writeErr(w, err)
		return
	}
	a.audit(r.Context(), schoolID, "document", documentID, "document_reviewed", "admin", claims.UserID, "",
		in.Note, nil, map[string]any{"status": docReviewDBToAPI(dbStatus)})
	common.WriteJSON(w, http.StatusOK, map[string]any{"id": documentID, "status": docReviewDBToAPI(dbStatus)}, nil)
}
