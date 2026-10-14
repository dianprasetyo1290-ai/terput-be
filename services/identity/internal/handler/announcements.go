package handler

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/lib/pq"

	"skci/identity/internal/common"
	"skci/identity/internal/model"
)

// Informasi (announcements): admin publishes to all roles or a subset;
// guru/siswa/admin list announcements visible to their active_role.
//
// Two content types share this table (see migration 0003_kelola_informasi):
//   - "event" (Acara/Event): Location/URL/ThumbnailURL/DocumentationPhotoURLs
//   - "pengumuman": Jenis ("umum" | "ujian")
//
// Message doubles as Konten (event) / Pesan (pengumuman).

const (
	announcementTypeEvent      = "event"
	announcementTypePengumuman = "pengumuman"
)

func isValidAnnouncementType(t string) bool {
	return t == announcementTypeEvent || t == announcementTypePengumuman
}

func isValidJenis(j string) bool {
	return j == "umum" || j == "ujian"
}

type announcementRequest struct {
	SchoolID                string   `json:"school_id"`
	Type                     string   `json:"type"` // "event" | "pengumuman", default "pengumuman"
	Title                    string   `json:"title"`
	Message                  string   `json:"message"` // Konten (event) / Pesan (pengumuman)
	TargetRoles              []string `json:"target_roles"` // empty/omitted = visible to all roles
	Location                 *string  `json:"location"`      // event only
	URL                      *string  `json:"url"`            // event only
	ThumbnailURL             *string  `json:"thumbnail_url"`  // event only
	DocumentationPhotoURLs   []string `json:"documentation_photo_urls"` // event only
	Jenis                    *string  `json:"jenis"`          // pengumuman only: "umum" | "ujian"
	Published                *bool    `json:"published"`      // omitted on create = published
}

func scanAnnouncement(row interface{ Scan(...interface{}) error }) (model.Announcement, error) {
	var an model.Announcement
	err := row.Scan(
		&an.ID, &an.SchoolID, &an.Type, &an.Title, &an.Message, pq.Array(&an.TargetRoles),
		&an.Location, &an.URL, &an.ThumbnailURL, pq.Array(&an.DocumentationPhotoURLs), &an.Jenis,
		&an.Published, &an.CreatedBy, &an.CreatedAt, &an.UpdatedAt,
	)
	return an, err
}

const announcementCols = `id, school_id, type, title, message, target_roles,
	location, url, thumbnail_url, documentation_photo_urls, jenis,
	published, created_by, created_at, updated_at`

// validateAnnouncementRequest checks the shared fields plus type-specific
// ones, and nils out fields that don't belong to the chosen type so a
// pengumuman never carries stray event fields (or vice versa) regardless of
// what the client sent.
func validateAnnouncementRequest(req *announcementRequest) (msg string, ok bool) {
	if req.Title == "" || req.Message == "" {
		return "title and message are required", false
	}
	if req.Type == "" {
		req.Type = announcementTypePengumuman
	}
	if !isValidAnnouncementType(req.Type) {
		return "type must be 'event' or 'pengumuman'", false
	}
	// target_roles is also NOT NULL DEFAULT '{}' — same nil-slice-becomes-
	// SQL-NULL trap as documentation_photo_urls below.
	if req.TargetRoles == nil {
		req.TargetRoles = []string{}
	}
	for _, role := range req.TargetRoles {
		if !common.IsValidRole(role) {
			return "invalid target role: " + role, false
		}
	}
	if req.Type == announcementTypePengumuman {
		if req.Jenis == nil || *req.Jenis == "" {
			umum := "umum"
			req.Jenis = &umum
		} else if !isValidJenis(*req.Jenis) {
			return "jenis must be 'umum' or 'ujian'", false
		}
		req.Location, req.URL, req.ThumbnailURL = nil, nil, nil
		req.DocumentationPhotoURLs = []string{}
	} else {
		req.Jenis = nil
		// documentation_photo_urls is NOT NULL DEFAULT '{}' — a nil Go
		// slice (field omitted by the client) would otherwise reach
		// pq.Array() as SQL NULL and violate that constraint.
		if req.DocumentationPhotoURLs == nil {
			req.DocumentationPhotoURLs = []string{}
		}
	}
	return "", true
}

func (a *App) CreateAnnouncement(w http.ResponseWriter, r *http.Request) {
	if !requireRole(w, r, common.RoleAdmin) {
		return
	}
	claims := common.FromCtx(r)
	var req announcementRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		common.WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if msg, ok := validateAnnouncementRequest(&req); !ok {
		common.WriteError(w, http.StatusBadRequest, msg)
		return
	}
	var schoolID interface{}
	if claims.ActiveRole == common.RoleSuperadmin {
		if req.SchoolID == "" {
			common.WriteError(w, http.StatusBadRequest, "school_id is required")
			return
		}
		schoolID = req.SchoolID
	} else {
		schoolID = claims.SchoolID
	}
	published := true // default: visible immediately, same as before this feature existed
	if req.Published != nil {
		published = *req.Published
	}
	row := a.DB.QueryRow(`INSERT INTO announcements
		(school_id, type, title, message, target_roles, location, url, thumbnail_url,
		 documentation_photo_urls, jenis, published, created_by)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12) RETURNING `+announcementCols,
		schoolID, req.Type, req.Title, req.Message, pq.Array(req.TargetRoles),
		req.Location, req.URL, req.ThumbnailURL, pq.Array(req.DocumentationPhotoURLs), req.Jenis,
		published, claims.UserID)
	an, err := scanAnnouncement(row)
	if err != nil {
		common.WriteDBError(w, err, "failed to create announcement")
		return
	}
	common.WriteJSON(w, http.StatusCreated, an, nil)
}

// ListVisibleAnnouncements returns published announcements visible to the
// caller: scoped to their school (or global/school-less ones) AND either
// untargeted (visible to all roles) or targeting the caller's active_role.
// This is the read-only "Informasi" view — drafts (published=false) never
// appear here regardless of role/school scope.
func (a *App) ListVisibleAnnouncements(w http.ResponseWriter, r *http.Request) {
	claims := common.FromCtx(r)
	rows, err := a.DB.Query(`SELECT `+announcementCols+` FROM announcements
		WHERE published = true
		AND (school_id IS NULL OR school_id = $1)
		AND (cardinality(target_roles) = 0 OR $2 = ANY(target_roles))
		ORDER BY created_at DESC`, nullIfEmpty(claims.SchoolID), claims.ActiveRole)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "failed to list announcements")
		return
	}
	defer rows.Close()
	out := []model.Announcement{}
	for rows.Next() {
		an, err := scanAnnouncement(rows)
		if err != nil {
			common.WriteError(w, http.StatusInternalServerError, "failed to list announcements")
			return
		}
		out = append(out, an)
	}
	common.WriteJSON(w, http.StatusOK, out, nil)
}

// ListAllAnnouncements (admin management view "Kelola Informasi"): every
// announcement the admin published for their school, including unpublished
// drafts — (for superadmin, optionally filtered by ?school_id=).
func (a *App) ListAllAnnouncements(w http.ResponseWriter, r *http.Request) {
	if !requireRole(w, r, common.RoleAdmin) {
		return
	}
	schoolID, filtered := resolveSchoolScope(r)
	query := `SELECT ` + announcementCols + ` FROM announcements`
	args := []interface{}{}
	if filtered {
		query += ` WHERE school_id = $1`
		args = append(args, schoolID)
	}
	query += ` ORDER BY created_at DESC`
	rows, err := a.DB.Query(query, args...)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "failed to list announcements")
		return
	}
	defer rows.Close()
	out := []model.Announcement{}
	for rows.Next() {
		an, err := scanAnnouncement(rows)
		if err != nil {
			common.WriteError(w, http.StatusInternalServerError, "failed to list announcements")
			return
		}
		out = append(out, an)
	}
	common.WriteJSON(w, http.StatusOK, out, nil)
}

func (a *App) GetAnnouncement(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	row := a.DB.QueryRow(`SELECT `+announcementCols+` FROM announcements WHERE id=$1`, id)
	an, err := scanAnnouncement(row)
	if err == sql.ErrNoRows {
		common.WriteError(w, http.StatusNotFound, "announcement not found")
		return
	} else if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "failed to get announcement")
		return
	}
	common.WriteJSON(w, http.StatusOK, an, nil)
}

func (a *App) UpdateAnnouncement(w http.ResponseWriter, r *http.Request) {
	if !requireRole(w, r, common.RoleAdmin) {
		return
	}
	claims := common.FromCtx(r)
	id := chi.URLParam(r, "id")
	var req announcementRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		common.WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if msg, ok := validateAnnouncementRequest(&req); !ok {
		common.WriteError(w, http.StatusBadRequest, msg)
		return
	}
	// Published isn't touched by a plain Update unless explicitly sent —
	// dedicated Publish/Unpublish endpoints below are the normal path for
	// the Publish / Non Publish button, this just avoids silently
	// resetting the flag if a client omits it.
	query := `UPDATE announcements SET type=$1, title=$2, message=$3, target_roles=$4,
		location=$5, url=$6, thumbnail_url=$7, documentation_photo_urls=$8, jenis=$9,
		updated_at=now()`
	args := []interface{}{req.Type, req.Title, req.Message, pq.Array(req.TargetRoles),
		req.Location, req.URL, req.ThumbnailURL, pq.Array(req.DocumentationPhotoURLs), req.Jenis}
	if req.Published != nil {
		args = append(args, *req.Published)
		query += `, published=$` + strconv.Itoa(len(args))
	}
	args = append(args, id)
	query += ` WHERE id=$` + strconv.Itoa(len(args))
	if claims.ActiveRole != common.RoleSuperadmin {
		args = append(args, claims.SchoolID)
		query += ` AND school_id=$` + strconv.Itoa(len(args))
	}
	query += ` RETURNING ` + announcementCols
	row := a.DB.QueryRow(query, args...)
	an, err := scanAnnouncement(row)
	if err == sql.ErrNoRows {
		common.WriteError(w, http.StatusNotFound, "announcement not found")
		return
	} else if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "failed to update announcement")
		return
	}
	common.WriteJSON(w, http.StatusOK, an, nil)
}

// setAnnouncementPublished backs both PublishAnnouncement and
// UnpublishAnnouncement — the Publish / Non Publish button pair.
func (a *App) setAnnouncementPublished(w http.ResponseWriter, r *http.Request, published bool) {
	if !requireRole(w, r, common.RoleAdmin) {
		return
	}
	claims := common.FromCtx(r)
	id := chi.URLParam(r, "id")
	query := `UPDATE announcements SET published=$1, updated_at=now() WHERE id=$2`
	args := []interface{}{published, id}
	if claims.ActiveRole != common.RoleSuperadmin {
		query += ` AND school_id=$3`
		args = append(args, claims.SchoolID)
	}
	query += ` RETURNING ` + announcementCols
	row := a.DB.QueryRow(query, args...)
	an, err := scanAnnouncement(row)
	if err == sql.ErrNoRows {
		common.WriteError(w, http.StatusNotFound, "announcement not found")
		return
	} else if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "failed to update announcement")
		return
	}
	common.WriteJSON(w, http.StatusOK, an, nil)
}

func (a *App) PublishAnnouncement(w http.ResponseWriter, r *http.Request) {
	a.setAnnouncementPublished(w, r, true)
}

func (a *App) UnpublishAnnouncement(w http.ResponseWriter, r *http.Request) {
	a.setAnnouncementPublished(w, r, false)
}

func (a *App) DeleteAnnouncement(w http.ResponseWriter, r *http.Request) {
	if !requireRole(w, r, common.RoleAdmin) {
		return
	}
	claims := common.FromCtx(r)
	id := chi.URLParam(r, "id")
	query := `DELETE FROM announcements WHERE id=$1`
	args := []interface{}{id}
	if claims.ActiveRole != common.RoleSuperadmin {
		query += ` AND school_id=$2`
		args = append(args, claims.SchoolID)
	}
	res, err := a.DB.Exec(query, args...)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "failed to delete announcement")
		return
	}
	if n, _ := res.RowsAffected(); n == 0 {
		common.WriteError(w, http.StatusNotFound, "announcement not found")
		return
	}
	common.WriteJSON(w, http.StatusOK, map[string]string{"id": id}, nil)
}

func nullIfEmpty(s string) interface{} {
	if s == "" {
		return nil
	}
	return s
}

func (a *App) DeleteAllAnnouncements(w http.ResponseWriter, r *http.Request) {
	if !requireRole(w, r, common.RoleAdmin) {
		return
	}
	claims := common.FromCtx(r)
	query := `DELETE FROM announcements`
	args := []interface{}{}
	if claims.ActiveRole != common.RoleSuperadmin {
		query += ` WHERE school_id=$1`
		args = append(args, claims.SchoolID)
	}
	res, err := a.DB.Exec(query, args...)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "failed to delete all announcements")
		return
	}
	n, _ := res.RowsAffected()
	common.WriteJSON(w, http.StatusOK, map[string]int64{"deleted": n}, nil)
}
