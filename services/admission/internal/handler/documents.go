package handler

import (
	"database/sql"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"skci/admission/internal/common"
	"skci/admission/internal/model"
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
	// A review only counts for the file version it was made on: a re-upload
	// after a correction request goes back to "pending" (BUG-160).
	rows, err := a.DB.QueryContext(r.Context(),
		`SELECT d.id,d.requirement_id,COALESCE(d.file_name,''),COALESCE(d.mime,''),d.size_bytes,d.created_at,
		        d.version,d.lock_version,COALESCE(rev.status,''),COALESCE(rev.notes,''),COALESCE(rev.reviewer_name,''),rev.created_at,
		        (SELECT count(*) FROM application_document_versions v WHERE v.document_id=d.id)
		 FROM application_documents d
		 LEFT JOIN LATERAL (
		     SELECT status,notes,reviewer_name,created_at FROM document_reviews dr
		     WHERE dr.document_id=d.id AND dr.document_version=d.version ORDER BY dr.created_at DESC LIMIT 1
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
			id, requirementID, fileName, mime, reviewStatus, reviewNote, reviewer string
			size                                                                  int64
			uploadedAt                                                            time.Time
			version, lockVersion, previous                                        int
			reviewedAt                                                            sql.NullTime
		)
		if err := rows.Scan(&id, &requirementID, &fileName, &mime, &size, &uploadedAt, &version, &lockVersion,
			&reviewStatus, &reviewNote, &reviewer, &reviewedAt, &previous); err != nil {
			return nil, err
		}
		doc := map[string]any{
			"id":                id,
			"requirement_id":    requirementID,
			"file_name":         fileName,
			"mime_type":         mime,
			"size":              size,
			"uploaded_at":       uploadedAt,
			"status":            docReviewDBToAPI(reviewStatus),
			"version":           version,
			"lock_version":      lockVersion,
			"previous_versions": previous,
		}
		if reviewNote != "" {
			doc["review_note"] = reviewNote
		}
		if reviewedAt.Valid {
			doc["reviewed_at"] = reviewedAt.Time
			doc["reviewer_name"] = reviewer
		}
		out = append(out, doc)
	}
	return out, rows.Err()
}

func (a *App) documentRequirements(r *http.Request, schoolID, applicationID string) ([]map[string]any, error) {
	rows, err := a.DB.QueryContext(r.Context(),
		`SELECT dr.id,dr.name,COALESCE(dr.description,''),dr.allowed_mimes,dr.max_bytes,dr.required
		 FROM document_requirements dr
		 JOIN applications a ON a.period_id=dr.period_id AND (dr.track_id IS NULL OR dr.track_id=a.track_id)
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
// requirement_id + file). Files are accepted while the application is a draft,
// or later when the committee asked for a correction / rejected that file. A
// re-upload archives the previous version instead of overwriting it
// (BUG-160). The type is decided from the file content and extension, never
// from the client Content-Type (BUG-042/100).
func (a *App) UploadDocument(w http.ResponseWriter, r *http.Request) {
	c := common.Guardian(r)
	r.Body = http.MaxBytesReader(w, r.Body, a.Config.MaxUploadBytes+(1<<20))
	if err := r.ParseMultipartForm(a.Config.MaxUploadBytes); err != nil {
		common.WriteError(w, http.StatusBadRequest, "unggahan tidak valid atau melebihi batas ukuran")
		return
	}
	applicationID := param(r, "id")
	requirementID := r.FormValue("requirement_id")
	if requirementID == "" {
		common.WriteError(w, http.StatusUnprocessableEntity, "requirement_id wajib diisi")
		return
	}
	file, header, err := r.FormFile("file")
	if err != nil {
		common.WriteError(w, http.StatusUnprocessableEntity, "berkas wajib diunggah")
		return
	}
	defer file.Close()

	var (
		maxBytes int64
		allowed  textArray
		status   string
	)
	err = a.DB.QueryRowContext(r.Context(),
		`SELECT dr.max_bytes,dr.allowed_mimes,app.status
		 FROM document_requirements dr
		 JOIN applications app ON app.period_id=dr.period_id AND (dr.track_id IS NULL OR dr.track_id=app.track_id)
		 JOIN applicants ap ON ap.id=app.applicant_id
		 WHERE dr.id=$1 AND app.id=$2 AND app.school_id=$3 AND ap.guardian_id=$4 AND dr.deleted_at IS NULL`,
		requirementID, applicationID, c.SchoolID, c.GuardianID).Scan(&maxBytes, &allowed, &status)
	if err == sql.ErrNoRows {
		common.WriteError(w, http.StatusNotFound, "persyaratan dokumen tidak ditemukan")
		return
	}
	if err != nil {
		writeErr(w, err)
		return
	}
	mime, body, err := sniffUpload(file, header)
	if err != nil {
		writeErr(w, err)
		return
	}
	if !contains(allowed, mime) {
		common.WriteError(w, http.StatusUnprocessableEntity, "tipe berkas tidak diizinkan untuk dokumen ini")
		return
	}
	if header.Size > maxBytes {
		common.WriteError(w, http.StatusUnprocessableEntity, "ukuran berkas melebihi batas")
		return
	}

	tx, err := a.DB.BeginTx(r.Context(), nil)
	if err != nil {
		writeErr(w, err)
		return
	}
	defer func() { _ = tx.Rollback() }()

	var (
		existingID, existingReview string
		existingVersion            int
		locked                     sql.NullTime
	)
	err = tx.QueryRowContext(r.Context(),
		`SELECT d.id,d.version,d.locked_at,COALESCE((SELECT rv.status FROM document_reviews rv
		        WHERE rv.document_id=d.id AND rv.document_version=d.version ORDER BY rv.created_at DESC LIMIT 1),'')
		 FROM application_documents d WHERE d.application_id=$1 AND d.requirement_id=$2 FOR UPDATE`,
		applicationID, requirementID).Scan(&existingID, &existingVersion, &locked, &existingReview)
	if err != nil && err != sql.ErrNoRows {
		writeErr(w, err)
		return
	}
	exists := err == nil
	if !model.DocumentUploadAllowed(status, exists, existingReview, locked.Valid) {
		common.WriteError(w, http.StatusConflict, "dokumen tidak dapat diubah pada tahap ini")
		return
	}

	key := c.SchoolID + "/applications/" + applicationID + "/" + randomKey()
	obj, err := a.Storage.Put(r.Context(), key, mime, body, header.Size)
	if err != nil {
		common.WriteError(w, http.StatusUnprocessableEntity, "berkas gagal disimpan")
		return
	}
	cleanup := func() { _ = a.Storage.Delete(r.Context(), obj.Key) }

	var (
		id      string
		version = 1
	)
	if exists {
		if _, err = tx.ExecContext(r.Context(),
			`INSERT INTO application_document_versions(school_id,document_id,application_id,version,object_key,file_name,mime,size_bytes,sha256,uploaded_by,uploaded_at)
			 SELECT school_id,id,application_id,version,object_key,file_name,mime,size_bytes,sha256,uploaded_by,created_at
			 FROM application_documents WHERE id=$1`, existingID); err != nil {
			cleanup()
			writeErr(w, err)
			return
		}
		version = existingVersion + 1
		err = tx.QueryRowContext(r.Context(),
			`UPDATE application_documents SET object_key=$2,file_name=$3,mime=$4,size_bytes=$5,sha256=$6,uploaded_by=$7,
			        created_at=now(),version=$8,lock_version=lock_version+1,scan_status='pending'
			 WHERE id=$1 RETURNING id`,
			existingID, obj.Key, header.Filename, obj.MIME, obj.Size, obj.Checksum, c.GuardianID, version).Scan(&id)
	} else {
		err = tx.QueryRowContext(r.Context(),
			`INSERT INTO application_documents(school_id,application_id,requirement_id,object_key,file_name,mime,size_bytes,sha256,uploaded_by)
			 VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9) RETURNING id`,
			c.SchoolID, applicationID, requirementID, obj.Key, header.Filename, obj.MIME, obj.Size, obj.Checksum, c.GuardianID).Scan(&id)
	}
	if err != nil {
		cleanup()
		writeErr(w, err)
		return
	}
	if _, err = tx.ExecContext(r.Context(),
		`INSERT INTO audit_events(school_id,aggregate_type,aggregate_id,action,actor_type,actor_id,actor_name,after_data)
		 VALUES($1,'application',$2,'document_uploaded','guardian',$3,'',$4)`,
		c.SchoolID, applicationID, c.GuardianID,
		jsonRaw(map[string]any{"document_id": id, "requirement_id": requirementID, "version": version, "mime": obj.MIME, "size": obj.Size})); err != nil {
		cleanup()
		writeErr(w, err)
		return
	}
	if err = tx.Commit(); err != nil {
		cleanup()
		writeErr(w, err)
		return
	}
	common.WriteJSON(w, http.StatusCreated, map[string]any{
		"id":          id,
		"file_name":   header.Filename,
		"mime_type":   obj.MIME,
		"size":        obj.Size,
		"sha256":      obj.Checksum,
		"version":     version,
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
	var key, mime, checksum, fileName string
	var size int64
	err := a.DB.QueryRowContext(r.Context(),
		`SELECT d.object_key,d.mime,d.size_bytes,d.sha256,COALESCE(d.file_name,'')
		 FROM application_documents d
		 JOIN applications app ON app.id=d.application_id
		 JOIN applicants ap ON ap.id=app.applicant_id
		 WHERE d.id=$1 AND d.application_id=$2 AND d.school_id=$3 AND ap.guardian_id=$4`,
		param(r, "docId"), param(r, "id"), c.SchoolID, c.GuardianID).Scan(&key, &mime, &size, &checksum, &fileName)
	if err != nil {
		common.WriteError(w, http.StatusNotFound, "document not found")
		return
	}
	a.streamObject(w, r, key, mime, checksum, fileName, size)
}

// AdminDownloadDocument is GET /admin/documents/{id}/download. ?inline=1
// renders PDFs/images in the browser for the verification preview (BUG-158);
// ?version=N streams an archived version.
func (a *App) AdminDownloadDocument(w http.ResponseWriter, r *http.Request) {
	if !common.RequireAdmin(w, r) {
		return
	}
	schoolID := common.EffectiveSchoolID(r)
	var key, mime, checksum, fileName string
	var size int64
	var err error
	if v := r.URL.Query().Get("version"); v != "" {
		err = a.DB.QueryRowContext(r.Context(),
			`SELECT object_key,mime,size_bytes,sha256,COALESCE(file_name,'') FROM application_document_versions
			 WHERE document_id=$1 AND school_id=$2 AND version::text=$3`,
			param(r, "id"), schoolID, v).Scan(&key, &mime, &size, &checksum, &fileName)
	} else {
		err = a.DB.QueryRowContext(r.Context(),
			`SELECT object_key,mime,size_bytes,sha256,COALESCE(file_name,'') FROM application_documents WHERE id=$1 AND school_id=$2`,
			param(r, "id"), schoolID).Scan(&key, &mime, &size, &checksum, &fileName)
	}
	if err != nil {
		common.WriteError(w, http.StatusNotFound, "dokumen tidak ditemukan")
		return
	}
	a.streamObject(w, r, key, mime, checksum, fileName, size)
}

// AdminDocumentVersions is GET /admin/documents/{id}/versions.
func (a *App) AdminDocumentVersions(w http.ResponseWriter, r *http.Request) {
	if !common.RequireAdmin(w, r) {
		return
	}
	schoolID := common.EffectiveSchoolID(r)
	rows, err := a.DB.QueryContext(r.Context(),
		`SELECT v.version,COALESCE(v.file_name,''),v.mime,v.size_bytes,v.uploaded_at,v.archived_at,
		        COALESCE((SELECT rv.status FROM document_reviews rv WHERE rv.document_id=v.document_id
		                  AND rv.document_version=v.version ORDER BY rv.created_at DESC LIMIT 1),''),
		        COALESCE((SELECT rv.notes FROM document_reviews rv WHERE rv.document_id=v.document_id
		                  AND rv.document_version=v.version ORDER BY rv.created_at DESC LIMIT 1),'')
		 FROM application_document_versions v WHERE v.document_id=$1 AND v.school_id=$2 ORDER BY v.version DESC`,
		param(r, "id"), schoolID)
	if err != nil {
		writeErr(w, err)
		return
	}
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		var (
			version                      int
			fileName, mime, review, note string
			size                         int64
			uploadedAt, archivedAt       time.Time
		)
		if err := rows.Scan(&version, &fileName, &mime, &size, &uploadedAt, &archivedAt, &review, &note); err != nil {
			writeErr(w, err)
			return
		}
		out = append(out, map[string]any{"version": version, "file_name": fileName, "mime_type": mime, "size": size,
			"uploaded_at": uploadedAt, "archived_at": archivedAt, "status": docReviewDBToAPI(review), "review_note": note})
	}
	common.WriteJSON(w, http.StatusOK, out, nil)
}

func (a *App) streamObject(w http.ResponseWriter, r *http.Request, key, mime, checksum, fileName string, size int64) {
	f, err := a.Storage.Open(r.Context(), key)
	if err != nil {
		common.WriteError(w, http.StatusNotFound, "berkas tidak ditemukan")
		return
	}
	defer f.Close()
	w.Header().Set("Content-Type", mime)
	w.Header().Set("Content-Length", strconv.FormatInt(size, 10))
	w.Header().Set("X-Content-Type-Options", "nosniff")
	if checksum != "" {
		w.Header().Set("ETag", `"`+checksum+`"`)
	}
	disposition := "attachment"
	if r.URL.Query().Get("inline") == "1" && contains(model.AllowedUploadMIMEs, mime) {
		disposition = "inline"
		w.Header().Set("Content-Security-Policy", "default-src 'none'; img-src 'self'; style-src 'unsafe-inline'; sandbox")
	}
	w.Header().Set("Content-Disposition", disposition+`; filename="`+model.SafeFileName(fileName, mime)+`"`)
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
		        COALESCE(rev.status,''),COALESCE(rev.notes,''),d.version,d.lock_version,a.status
		 FROM application_documents d
		 JOIN applications a ON a.id=d.application_id
		 JOIN applicants ap ON ap.id=a.applicant_id
		 JOIN document_requirements dr ON dr.id=d.requirement_id
		 LEFT JOIN LATERAL (
		     SELECT status,notes FROM document_reviews rv WHERE rv.document_id=d.id AND rv.document_version=d.version
		     ORDER BY rv.created_at DESC LIMIT 1
		 ) rev ON true
		 WHERE d.school_id=$1 AND a.status<>'draft'
		   AND ($2='' OR $2 = CASE COALESCE(rev.status,'')
		        WHEN 'accepted' THEN 'verified'
		        WHEN 'correction' THEN 'correction_required'
		        WHEN 'rejected' THEN 'rejected'
		        ELSE 'pending' END)
		   AND ($3='' OR ap.full_name ILIKE '%'||$3||'%' OR a.registration_number ILIKE '%'||$3||'%')
		   AND ($6='' OR d.application_id::text=$6)
		 ORDER BY d.created_at DESC
		 LIMIT $4 OFFSET $5`,
		schoolID, status, search, perPage, common.Offset(page, perPage), r.URL.Query().Get("application_id"))
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
			reviewStatus, reviewNote, applicationStatus        string
			version, lockVersion                               int
		)
		if err := rows.Scan(&id, &applicationID, &requirementID, &fileName, &mime, &size, &uploadedAt,
			&applicantName, &registrationNumber, &requirementName, &reviewStatus, &reviewNote,
			&version, &lockVersion, &applicationStatus); err != nil {
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
			"version":             version,
			"lock_version":        lockVersion,
			"application_status":  applicationStatus,
		}
		if reviewNote != "" {
			item["review_note"] = reviewNote
		}
		items = append(items, item)
	}
	common.WriteJSON(w, http.StatusOK, map[string]any{"items": items}, nil)
}

// ReviewDocument is POST /admin/documents/{id}/review. The client sends the
// lock_version it displayed; a concurrent review or re-upload returns 409
// (BUG-159). Draft applications and unpaid forms cannot be verified
// (BUG-107), and the guardian is notified of the result (BUG-104).
func (a *App) ReviewDocument(w http.ResponseWriter, r *http.Request) {
	if !common.RequireAdmin(w, r) {
		return
	}
	schoolID := common.EffectiveSchoolID(r)
	claims := common.Admin(r)
	var in struct {
		Status      string `json:"status"`
		Note        string `json:"note"`
		LockVersion int    `json:"lock_version"`
	}
	if !common.Decode(w, r, &in) {
		return
	}
	dbStatus, ok := docReviewAPIToDB(in.Status)
	if !ok {
		common.WriteError(w, http.StatusUnprocessableEntity, "status verifikasi tidak valid")
		return
	}
	if dbStatus != "accepted" && strings.TrimSpace(in.Note) == "" {
		common.WriteError(w, http.StatusUnprocessableEntity, "catatan wajib diisi untuk penolakan atau permintaan perbaikan")
		return
	}
	if in.LockVersion < 1 {
		common.WriteError(w, http.StatusUnprocessableEntity, "lock_version wajib dikirim; muat ulang halaman")
		return
	}
	documentID := param(r, "id")

	tx, err := a.DB.BeginTx(r.Context(), nil)
	if err != nil {
		writeErr(w, err)
		return
	}
	defer func() { _ = tx.Rollback() }()

	var (
		applicationID, appStatus, requirementName string
		version, lockVersion                      int
	)
	err = tx.QueryRowContext(r.Context(),
		`SELECT d.application_id,d.version,d.lock_version,a.status,dr.name
		 FROM application_documents d
		 JOIN applications a ON a.id=d.application_id
		 JOIN document_requirements dr ON dr.id=d.requirement_id
		 WHERE d.id=$1 AND d.school_id=$2 FOR UPDATE OF d`, documentID, schoolID).
		Scan(&applicationID, &version, &lockVersion, &appStatus, &requirementName)
	if err == sql.ErrNoRows {
		common.WriteError(w, http.StatusNotFound, "dokumen tidak ditemukan")
		return
	}
	if err != nil {
		writeErr(w, err)
		return
	}
	if lockVersion != in.LockVersion {
		common.WriteError(w, http.StatusConflict, "dokumen sudah diverifikasi atau diunggah ulang oleh pengguna lain; muat ulang halaman")
		return
	}
	if model.ApplicationStatus(appStatus) == model.StatusDraft {
		common.WriteError(w, http.StatusUnprocessableEntity, "pendaftaran masih draf; dokumen belum dapat diverifikasi")
		return
	}
	settled, _, _, err := stageSettled(r.Context(), tx, schoolID, applicationID, stageRegistration)
	if err != nil {
		writeErr(w, err)
		return
	}
	if !settled {
		common.WriteError(w, http.StatusUnprocessableEntity, "biaya formulir belum lunas; verifikasi dokumen menunggu pembayaran")
		return
	}
	if _, err = tx.ExecContext(r.Context(),
		`INSERT INTO document_reviews(school_id,document_id,status,notes,reviewer_id,document_version,reviewer_name)
		 VALUES($1,$2,$3,NULLIF($4,''),$5,$6,$7)`,
		schoolID, documentID, dbStatus, in.Note, claims.UserID, version, claims.Name); err != nil {
		writeErr(w, err)
		return
	}
	if _, err = tx.ExecContext(r.Context(),
		`UPDATE application_documents SET lock_version=lock_version+1 WHERE id=$1`, documentID); err != nil {
		writeErr(w, err)
		return
	}
	if _, err = tx.ExecContext(r.Context(),
		`INSERT INTO audit_events(school_id,aggregate_type,aggregate_id,action,actor_type,actor_id,actor_name,after_data,reason)
		 VALUES($1,'application',$2,'document_reviewed','admin',$3,$4,$5,NULLIF($6,''))`,
		schoolID, applicationID, claims.UserID, claims.Name,
		jsonRaw(map[string]any{"document_id": documentID, "requirement": requirementName, "version": version, "status": docReviewDBToAPI(dbStatus)}),
		in.Note); err != nil {
		writeErr(w, err)
		return
	}
	title, body := model.DocumentReviewMessage(requirementName, dbStatus, in.Note)
	if err = a.notify(r.Context(), tx, schoolID, applicationID, "document_reviewed", title, body,
		"doc_review:"+documentID+":"+strconv.Itoa(version)+":"+dbStatus, map[string]any{"document_id": documentID}); err != nil {
		writeErr(w, err)
		return
	}
	if err = tx.Commit(); err != nil {
		writeErr(w, err)
		return
	}
	common.WriteJSON(w, http.StatusOK, map[string]any{"id": documentID, "status": docReviewDBToAPI(dbStatus), "lock_version": lockVersion + 1}, nil)
}
