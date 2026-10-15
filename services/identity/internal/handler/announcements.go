package handler

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/lib/pq"

	"skci/identity/internal/client"
	"skci/identity/internal/common"
	"skci/identity/internal/model"
	"skci/identity/internal/rules"
)

// Informasi (announcements): admin publishes to an explicitly chosen
// audience; every user lists the announcements visible to them.
//
// Two content types share this table (see migration 0003_kelola_informasi):
//   - "event" (Acara/Event): Location/URL/ThumbnailURL/DocumentationPhotoURLs
//   - "pengumuman": Jenis ("umum" | "ujian")
//
// Audience (migration 0023, QA-CORE BUG-089/153/154): roles ∩ unit ∩ class.
//   - audience_all=true or at least one target role is required (422).
//   - target_school_ids limits a global (superadmin) announcement to units.
//   - target_class_ids limits a unit announcement to classrooms; membership
//     comes from academic GET /internal/user-classrooms (student, wali kelas,
//     guru pengampu; a parent through the children).
//   - publish_at schedules it: visible once published AND publish_at <= now.
//     Publishing with no/past publish_at sends immediately and needs
//     confirm_send_now=true (409 requires_confirmation otherwise).
//
// Message doubles as Konten (event) / Pesan (pengumuman).

const (
	announcementTypeEvent      = "event"
	announcementTypePengumuman = "pengumuman"
	maxAnnouncementClasses     = 50
)

func isValidAnnouncementType(t string) bool {
	return t == announcementTypeEvent || t == announcementTypePengumuman
}

func isValidJenis(j string) bool {
	return j == "umum" || j == "ujian"
}

type announcementRequest struct {
	SchoolID               string   `json:"school_id"`
	Type                   string   `json:"type"` // "event" | "pengumuman", default "pengumuman"
	Title                  string   `json:"title"`
	Message                string   `json:"message"` // Konten (event) / Pesan (pengumuman)
	AudienceAll            bool     `json:"audience_all"`
	TargetRoles            []string `json:"target_roles"`
	TargetSchoolIDs        []string `json:"target_school_ids"`
	TargetClassIDs         []string `json:"target_class_ids"`
	PublishAt              *string  `json:"publish_at"` // RFC3339; empty = now
	ConfirmSendNow         bool     `json:"confirm_send_now"`
	Location               *string  `json:"location"`                 // event only
	URL                    *string  `json:"url"`                      // event only
	ThumbnailURL           *string  `json:"thumbnail_url"`            // event only
	DocumentationPhotoURLs []string `json:"documentation_photo_urls"` // event only
	Jenis                  *string  `json:"jenis"`                    // pengumuman only: "umum" | "ujian"
	Published              *bool    `json:"published"`                // omitted on create = published

	publishAt *time.Time
}

func announcementStatus(an *model.Announcement, now time.Time) string {
	switch {
	case !an.Published:
		return "draf"
	case an.PublishAt != nil && an.PublishAt.After(now):
		return "terjadwal"
	default:
		return "terbit"
	}
}

func scanAnnouncement(row interface{ Scan(...interface{}) error }) (model.Announcement, error) {
	var an model.Announcement
	err := row.Scan(
		&an.ID, &an.SchoolID, &an.Type, &an.Title, &an.Message, pq.Array(&an.TargetRoles),
		&an.Location, &an.URL, &an.ThumbnailURL, pq.Array(&an.DocumentationPhotoURLs), &an.Jenis,
		&an.Published, &an.AudienceAll, pq.Array(&an.TargetSchoolIDs), pq.Array(&an.TargetClassIDs),
		pq.Array(&an.TargetClassNames), &an.PublishAt,
		&an.CreatedBy, &an.CreatedAt, &an.UpdatedAt,
	)
	if err == nil {
		an.Status = announcementStatus(&an, time.Now())
	}
	return an, err
}

const announcementCols = `id, school_id, type, title, message, target_roles,
	location, url, thumbnail_url, documentation_photo_urls, jenis,
	published, audience_all, target_school_ids::text[], target_class_ids::text[],
	target_class_names, publish_at, created_by, created_at, updated_at`

// validateAnnouncementRequest checks the shared fields plus type-specific
// ones, and nils out fields that don't belong to the chosen type so a
// pengumuman never carries stray event fields (or vice versa) regardless of
// what the client sent. Returns field errors (empty = ok).
func validateAnnouncementRequest(req *announcementRequest, now time.Time) (string, map[string]string) {
	errs := map[string]string{}
	req.Title = strings.TrimSpace(req.Title)
	req.Message = strings.TrimSpace(req.Message)
	if req.Title == "" {
		errs["title"] = "Judul wajib diisi"
	}
	if req.Message == "" {
		errs["message"] = "Isi pengumuman wajib diisi"
	}
	if req.Type == "" {
		req.Type = announcementTypePengumuman
	}
	if !isValidAnnouncementType(req.Type) {
		errs["type"] = "Pilih jenis konten: event atau pengumuman"
	}
	if req.TargetRoles == nil {
		req.TargetRoles = []string{}
	}
	for _, role := range req.TargetRoles {
		if !common.IsValidRole(role) {
			errs["target_roles"] = "Peran tidak dikenal: " + role
		}
	}
	if req.AudienceAll {
		req.TargetRoles = []string{}
	} else if len(req.TargetRoles) == 0 {
		errs["target_roles"] = "Audiens wajib dipilih: pilih peran penerima atau \"Semua peran\""
	}
	req.TargetSchoolIDs = dedupe(req.TargetSchoolIDs)
	for _, id := range req.TargetSchoolIDs {
		if !rules.IsUUID(id) {
			errs["target_school_ids"] = "Unit tujuan tidak valid"
		}
	}
	req.TargetClassIDs = dedupe(req.TargetClassIDs)
	for _, id := range req.TargetClassIDs {
		if !rules.IsUUID(id) {
			errs["target_class_ids"] = "Kelas tujuan tidak valid"
		}
	}
	if len(req.TargetClassIDs) > maxAnnouncementClasses {
		errs["target_class_ids"] = "Maksimal 50 kelas per pengumuman"
	}
	req.publishAt = nil
	if req.PublishAt != nil && strings.TrimSpace(*req.PublishAt) != "" {
		t, err := time.Parse(time.RFC3339, strings.TrimSpace(*req.PublishAt))
		switch {
		case err != nil:
			errs["publish_at"] = "Format waktu terbit tidak valid"
		case t.After(now.AddDate(1, 0, 0)):
			errs["publish_at"] = "Jadwal terbit maksimal 1 tahun ke depan"
		default:
			req.publishAt = &t
		}
	}
	if req.Type == announcementTypePengumuman {
		if req.Jenis == nil || *req.Jenis == "" {
			umum := "umum"
			req.Jenis = &umum
		} else if !isValidJenis(*req.Jenis) {
			errs["jenis"] = "Jenis pengumuman harus umum atau ujian"
		}
		req.Location, req.URL, req.ThumbnailURL = nil, nil, nil
		req.DocumentationPhotoURLs = []string{}
	} else {
		req.Jenis = nil
		if req.DocumentationPhotoURLs == nil {
			req.DocumentationPhotoURLs = []string{}
		}
	}
	if len(errs) > 0 {
		return "Periksa kembali isian pengumuman", errs
	}
	return "", nil
}

func dedupe(in []string) []string {
	seen := map[string]bool{}
	out := []string{}
	for _, s := range in {
		s = strings.ToLower(strings.TrimSpace(s))
		if s != "" && !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	return out
}

// needsSendNowConfirmation: publishing now (no or past publish_at) sends to
// the audience immediately, so the caller must confirm it explicitly.
func needsSendNowConfirmation(publish bool, publishAt *time.Time, wasLive bool, confirmed bool, now time.Time) bool {
	if !publish || wasLive || confirmed {
		return false
	}
	return publishAt == nil || !publishAt.After(now)
}

func writeSendNowConfirmation(w http.ResponseWriter) {
	common.WriteJSONMsg(w, http.StatusConflict, map[string]any{"requires_confirmation": true}, nil,
		"Pengumuman akan langsung terkirim ke audiens. Konfirmasi \"Kirim sekarang\" atau isi jadwal terbit.")
}

// resolveAnnouncementTargets validates class ids against academic (same
// unit) and returns their names; unit targets only apply to global ones.
func (a *App) resolveAnnouncementTargets(req *announcementRequest, schoolID *string) (names []string, field map[string]string, upstream string) {
	names = []string{}
	if schoolID != nil {
		// A unit announcement is already limited to its unit.
		req.TargetSchoolIDs = []string{}
	}
	if len(req.TargetClassIDs) == 0 {
		return names, nil, ""
	}
	if schoolID == nil {
		return nil, map[string]string{"target_class_ids": "Target kelas hanya untuk pengumuman satu unit; pilih unit terlebih dahulu"}, ""
	}
	if a.Academic == nil {
		return nil, nil, "Layanan akademik belum dikonfigurasi"
	}
	for _, id := range req.TargetClassIDs {
		name, err := a.Academic.ClassroomName(*schoolID, id)
		if errors.Is(err, client.ErrClassroomNotFound) {
			return nil, map[string]string{"target_class_ids": "Kelas tidak ditemukan di unit ini"}, ""
		} else if err != nil {
			common.CaptureError(err)
			return nil, nil, "Layanan akademik tidak dapat dihubungi untuk memeriksa kelas"
		}
		names = append(names, name)
	}
	return names, nil, ""
}

func (a *App) CreateAnnouncement(w http.ResponseWriter, r *http.Request) {
	if !a.permit(w, r, "announcements", "create") {
		return
	}
	claims := common.FromCtx(r)
	if claims == nil {
		common.WriteError(w, http.StatusForbidden, "Hanya untuk pengguna")
		return
	}
	var req announcementRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		common.WriteError(w, http.StatusBadRequest, "Isian tidak valid")
		return
	}
	now := time.Now()
	if msg, errs := validateAnnouncementRequest(&req, now); errs != nil {
		common.WriteValidationError(w, http.StatusUnprocessableEntity, msg, errs)
		return
	}
	var schoolID *string
	if claims.ActiveRole == common.RoleSuperadmin {
		// Superadmin: a unit announcement, or a global one (optionally
		// limited to target_school_ids).
		if req.SchoolID != "" {
			if !rules.IsUUID(req.SchoolID) {
				common.WriteValidationError(w, http.StatusUnprocessableEntity, "Unit tidak valid", map[string]string{"school_id": "Tidak valid"})
				return
			}
			schoolID = &req.SchoolID
		}
	} else {
		if claims.SchoolID == "" {
			common.WriteError(w, http.StatusForbidden, "Akun Anda tidak terikat pada unit")
			return
		}
		schoolID = &claims.SchoolID
	}
	names, fieldErr, upstream := a.resolveAnnouncementTargets(&req, schoolID)
	if fieldErr != nil {
		common.WriteValidationError(w, http.StatusUnprocessableEntity, fieldErr["target_class_ids"], fieldErr)
		return
	}
	if upstream != "" {
		common.WriteError(w, http.StatusBadGateway, upstream)
		return
	}
	published := true
	if req.Published != nil {
		published = *req.Published
	}
	if needsSendNowConfirmation(published, req.publishAt, false, req.ConfirmSendNow, now) {
		writeSendNowConfirmation(w)
		return
	}
	row := a.DB.QueryRowContext(r.Context(), `INSERT INTO announcements
		(school_id, type, title, message, target_roles, location, url, thumbnail_url,
		 documentation_photo_urls, jenis, published, audience_all, target_school_ids,
		 target_class_ids, target_class_names, publish_at, created_by)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13::uuid[],$14::uuid[],$15,$16,$17) RETURNING `+announcementCols,
		schoolID, req.Type, req.Title, req.Message, pq.Array(req.TargetRoles),
		req.Location, req.URL, req.ThumbnailURL, pq.Array(req.DocumentationPhotoURLs), req.Jenis,
		published, req.AudienceAll, pq.Array(req.TargetSchoolIDs), pq.Array(req.TargetClassIDs),
		pq.Array(names), req.publishAt, claims.UserID)
	an, err := scanAnnouncement(row)
	if err != nil {
		common.WriteDBError(w, err, "Gagal menyimpan pengumuman")
		return
	}
	common.WriteJSONMsg(w, http.StatusCreated, an, nil, announcementSavedMessage(an))
}

func announcementSavedMessage(an model.Announcement) string {
	switch an.Status {
	case "draf":
		return "Pengumuman disimpan sebagai draf"
	case "terjadwal":
		return "Pengumuman dijadwalkan terbit " + an.PublishAt.In(jakarta()).Format("02-01-2006 15:04") + " WIB"
	default:
		return "Pengumuman diterbitkan"
	}
}

// viewer is who is looking at the announcement list.
type viewer struct {
	SchoolID string
	Role     string
	Classes  map[string]bool // nil = not loaded / unavailable
}

// announcementVisibleTo is the audience rule (pure; table-tested).
func announcementVisibleTo(an *model.Announcement, v viewer, now time.Time) bool {
	if !an.Published || (an.PublishAt != nil && an.PublishAt.After(now)) {
		return false
	}
	if an.SchoolID != nil && *an.SchoolID != v.SchoolID {
		return false
	}
	if an.SchoolID == nil && len(an.TargetSchoolIDs) > 0 && !contains(an.TargetSchoolIDs, strings.ToLower(v.SchoolID)) {
		return false
	}
	if !an.AudienceAll && !contains(an.TargetRoles, v.Role) {
		return false
	}
	if len(an.TargetClassIDs) > 0 {
		if v.Classes == nil {
			return false
		}
		for _, c := range an.TargetClassIDs {
			if v.Classes[strings.ToLower(c)] {
				return true
			}
		}
		return false
	}
	return true
}

// viewerClasses loads the classrooms the caller (or, for a parent, the
// children) belongs to. nil on failure: class-targeted items stay hidden.
func (a *App) viewerClasses(ctx context.Context, userID, role, schoolID string) map[string]bool {
	if a.Academic == nil || schoolID == "" {
		return nil
	}
	ids := []string{userID}
	if role == common.RoleOrangTua {
		rows, err := a.DB.QueryContext(ctx, `SELECT student_user_id::text FROM guardian_links
			WHERE guardian_user_id::text=$1 AND school_id::text=$2 AND status='active'`, userID, schoolID)
		if err != nil {
			common.CaptureError(err)
			return nil
		}
		ids = ids[:0]
		for rows.Next() {
			var id string
			if rows.Scan(&id) == nil {
				ids = append(ids, id)
			}
		}
		rows.Close()
		if len(ids) == 0 {
			return map[string]bool{}
		}
	}
	classes, err := a.Academic.UserClassrooms(schoolID, ids)
	if err != nil {
		log.Printf("announcements: class membership unavailable: %v", err)
		common.CaptureError(err)
		return nil
	}
	out := map[string]bool{}
	for _, c := range classes {
		out[strings.ToLower(c)] = true
	}
	return out
}

// ListVisibleAnnouncements returns the announcements visible to the caller
// (the read-only "Informasi" view). Drafts and scheduled items never appear.
func (a *App) ListVisibleAnnouncements(w http.ResponseWriter, r *http.Request) {
	claims := common.FromCtx(r)
	if claims == nil {
		common.WriteError(w, http.StatusUnauthorized, "Sesi tidak ditemukan. Silakan masuk kembali")
		return
	}
	if !a.permit(w, r, "announcements", "view") {
		return
	}
	rows, err := a.DB.QueryContext(r.Context(), `SELECT `+announcementCols+` FROM announcements
		WHERE published = true AND (publish_at IS NULL OR publish_at <= now())
		AND (school_id IS NULL OR school_id = $1)
		ORDER BY COALESCE(publish_at, created_at) DESC`, nullIfEmpty(claims.SchoolID))
	if err != nil {
		common.WriteDBError(w, err, "Gagal memuat pengumuman")
		return
	}
	defer rows.Close()
	all := []model.Announcement{}
	needClasses := false
	for rows.Next() {
		an, err := scanAnnouncement(rows)
		if err != nil {
			common.WriteDBError(w, err, "Gagal memuat pengumuman")
			return
		}
		needClasses = needClasses || len(an.TargetClassIDs) > 0
		all = append(all, an)
	}
	v := viewer{SchoolID: claims.SchoolID, Role: claims.ActiveRole}
	if needClasses {
		v.Classes = a.viewerClasses(r.Context(), claims.UserID, claims.ActiveRole, claims.SchoolID)
	}
	now := time.Now()
	out := []model.Announcement{}
	for i := range all {
		if announcementVisibleTo(&all[i], v, now) {
			out = append(out, all[i])
		}
	}
	common.WriteJSON(w, http.StatusOK, out, nil)
}

// ListAllAnnouncements (management view "Kelola Informasi"): every
// announcement of the unit including drafts and scheduled ones (superadmin:
// all, or ?school_id=).
func (a *App) ListAllAnnouncements(w http.ResponseWriter, r *http.Request) {
	if !a.permit(w, r, "announcements", "create") {
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
	rows, err := a.DB.QueryContext(r.Context(), query, args...)
	if err != nil {
		common.WriteDBError(w, err, "Gagal memuat pengumuman")
		return
	}
	defer rows.Close()
	out := []model.Announcement{}
	for rows.Next() {
		an, err := scanAnnouncement(rows)
		if err != nil {
			common.WriteDBError(w, err, "Gagal memuat pengumuman")
			return
		}
		out = append(out, an)
	}
	common.WriteJSON(w, http.StatusOK, out, nil)
}

// GetAnnouncement: managers see any announcement in their scope; everyone
// else only what is visible to them.
func (a *App) GetAnnouncement(w http.ResponseWriter, r *http.Request) {
	claims := common.FromCtx(r)
	if claims == nil {
		common.WriteError(w, http.StatusUnauthorized, "Sesi tidak ditemukan. Silakan masuk kembali")
		return
	}
	an, err := scanAnnouncement(a.DB.QueryRowContext(r.Context(), `SELECT `+announcementCols+` FROM announcements WHERE id::text=$1`, chi.URLParam(r, "id")))
	if errors.Is(err, sql.ErrNoRows) {
		common.WriteError(w, http.StatusNotFound, "Pengumuman tidak ditemukan")
		return
	} else if err != nil {
		common.WriteDBError(w, err, "Gagal memuat pengumuman")
		return
	}
	if claims.ActiveRole == common.RoleSuperadmin {
		common.WriteJSON(w, http.StatusOK, an, nil)
		return
	}
	manager, err := a.allowedTo(r.Context(), claims.SchoolID, claims.ActiveRole, "announcements", "create")
	if err != nil {
		common.WriteDBError(w, err, "Gagal memeriksa hak akses")
		return
	}
	if manager && an.SchoolID != nil && *an.SchoolID == claims.SchoolID {
		common.WriteJSON(w, http.StatusOK, an, nil)
		return
	}
	v := viewer{SchoolID: claims.SchoolID, Role: claims.ActiveRole}
	if len(an.TargetClassIDs) > 0 {
		v.Classes = a.viewerClasses(r.Context(), claims.UserID, claims.ActiveRole, claims.SchoolID)
	}
	if !announcementVisibleTo(&an, v, time.Now()) {
		common.WriteError(w, http.StatusNotFound, "Pengumuman tidak ditemukan")
		return
	}
	common.WriteJSON(w, http.StatusOK, an, nil)
}

// loadManagedAnnouncement loads id within the caller's scope.
func (a *App) loadManagedAnnouncement(w http.ResponseWriter, r *http.Request) (model.Announcement, bool) {
	claims := common.FromCtx(r)
	q := `SELECT ` + announcementCols + ` FROM announcements WHERE id::text=$1`
	args := []any{chi.URLParam(r, "id")}
	if claims != nil && claims.ActiveRole != common.RoleSuperadmin {
		q += ` AND school_id::text=$2`
		args = append(args, claims.SchoolID)
	}
	an, err := scanAnnouncement(a.DB.QueryRowContext(r.Context(), q, args...))
	if errors.Is(err, sql.ErrNoRows) {
		common.WriteError(w, http.StatusNotFound, "Pengumuman tidak ditemukan")
		return an, false
	} else if err != nil {
		common.WriteDBError(w, err, "Gagal memuat pengumuman")
		return an, false
	}
	return an, true
}

func (a *App) UpdateAnnouncement(w http.ResponseWriter, r *http.Request) {
	if !a.permit(w, r, "announcements", "edit") {
		return
	}
	cur, ok := a.loadManagedAnnouncement(w, r)
	if !ok {
		return
	}
	var req announcementRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		common.WriteError(w, http.StatusBadRequest, "Isian tidak valid")
		return
	}
	now := time.Now()
	if msg, errs := validateAnnouncementRequest(&req, now); errs != nil {
		common.WriteValidationError(w, http.StatusUnprocessableEntity, msg, errs)
		return
	}
	names, fieldErr, upstream := a.resolveAnnouncementTargets(&req, cur.SchoolID)
	if fieldErr != nil {
		common.WriteValidationError(w, http.StatusUnprocessableEntity, fieldErr["target_class_ids"], fieldErr)
		return
	}
	if upstream != "" {
		common.WriteError(w, http.StatusBadGateway, upstream)
		return
	}
	published := cur.Published
	if req.Published != nil {
		published = *req.Published
	}
	wasLive := cur.Status == "terbit"
	if needsSendNowConfirmation(published, req.publishAt, wasLive, req.ConfirmSendNow, now) {
		writeSendNowConfirmation(w)
		return
	}
	query := `UPDATE announcements SET type=$1, title=$2, message=$3, target_roles=$4,
		location=$5, url=$6, thumbnail_url=$7, documentation_photo_urls=$8, jenis=$9,
		audience_all=$10, target_school_ids=$11::uuid[], target_class_ids=$12::uuid[],
		target_class_names=$13, publish_at=$14, published=$15, updated_at=now()
		WHERE id=$16 RETURNING ` + announcementCols
	an, err := scanAnnouncement(a.DB.QueryRowContext(r.Context(), query,
		req.Type, req.Title, req.Message, pq.Array(req.TargetRoles),
		req.Location, req.URL, req.ThumbnailURL, pq.Array(req.DocumentationPhotoURLs), req.Jenis,
		req.AudienceAll, pq.Array(req.TargetSchoolIDs), pq.Array(req.TargetClassIDs),
		pq.Array(names), req.publishAt, published, cur.ID))
	if err != nil {
		common.WriteDBError(w, err, "Gagal menyimpan pengumuman")
		return
	}
	common.WriteJSONMsg(w, http.StatusOK, an, nil, announcementSavedMessage(an))
}

// setAnnouncementPublished backs the Publish / Non Publish buttons. Publish
// with no future schedule needs {"confirm_send_now": true}.
func (a *App) setAnnouncementPublished(w http.ResponseWriter, r *http.Request, published bool) {
	if !a.permit(w, r, "announcements", "approve") {
		return
	}
	cur, ok := a.loadManagedAnnouncement(w, r)
	if !ok {
		return
	}
	var body struct {
		ConfirmSendNow bool `json:"confirm_send_now"`
	}
	if r.ContentLength != 0 {
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			common.WriteError(w, http.StatusBadRequest, "Isian tidak valid")
			return
		}
	}
	if needsSendNowConfirmation(published, cur.PublishAt, cur.Published, body.ConfirmSendNow, time.Now()) {
		writeSendNowConfirmation(w)
		return
	}
	an, err := scanAnnouncement(a.DB.QueryRowContext(r.Context(),
		`UPDATE announcements SET published=$1, updated_at=now() WHERE id=$2 RETURNING `+announcementCols, published, cur.ID))
	if err != nil {
		common.WriteDBError(w, err, "Gagal mengubah status pengumuman")
		return
	}
	msg := "Pengumuman ditarik (tidak lagi tampil)"
	if published {
		msg = announcementSavedMessage(an)
	}
	common.WriteJSONMsg(w, http.StatusOK, an, nil, msg)
}

func (a *App) PublishAnnouncement(w http.ResponseWriter, r *http.Request) {
	a.setAnnouncementPublished(w, r, true)
}

func (a *App) UnpublishAnnouncement(w http.ResponseWriter, r *http.Request) {
	a.setAnnouncementPublished(w, r, false)
}

func (a *App) DeleteAnnouncement(w http.ResponseWriter, r *http.Request) {
	if !a.permit(w, r, "announcements", "delete") {
		return
	}
	cur, ok := a.loadManagedAnnouncement(w, r)
	if !ok {
		return
	}
	if _, err := a.DB.ExecContext(r.Context(), `DELETE FROM announcements WHERE id=$1`, cur.ID); err != nil {
		common.WriteDBError(w, err, "Gagal menghapus pengumuman")
		return
	}
	common.WriteJSONMsg(w, http.StatusOK, map[string]string{"id": cur.ID}, nil, "Pengumuman dihapus")
}

func nullIfEmpty(s string) interface{} {
	if s == "" {
		return nil
	}
	return s
}

func (a *App) DeleteAllAnnouncements(w http.ResponseWriter, r *http.Request) {
	if !a.permit(w, r, "announcements", "delete") {
		return
	}
	claims := common.FromCtx(r)
	query := `DELETE FROM announcements`
	args := []interface{}{}
	if claims == nil || claims.ActiveRole != common.RoleSuperadmin {
		if claims == nil || claims.SchoolID == "" {
			common.WriteError(w, http.StatusForbidden, "Akun Anda tidak terikat pada unit")
			return
		}
		query += ` WHERE school_id=$1`
		args = append(args, claims.SchoolID)
	}
	res, err := a.DB.ExecContext(r.Context(), query, args...)
	if err != nil {
		common.WriteDBError(w, err, "Gagal menghapus pengumuman")
		return
	}
	n, _ := res.RowsAffected()
	common.WriteJSONMsg(w, http.StatusOK, map[string]int64{"deleted": n}, nil, strconv.FormatInt(n, 10)+" pengumuman dihapus")
}
