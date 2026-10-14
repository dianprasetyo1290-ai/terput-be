package handler

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/go-chi/chi/v5"

	"skci/core/internal/common"
	"skci/core/internal/model"
	"skci/core/internal/store"
)

// Menu 81 attachments (QA-PAY round 2): files are stored in the same
// private storage as service desk tickets, never as a bare file name.
// Upload first (POST /service-request-attachments), then reference the
// returned key in the request or the result; download goes through
// GET /service-requests/{id}/files?key= with the detail's access rule.

var srUnsafeName = regexp.MustCompile(`[^A-Za-z0-9._-]+`)

// errAttachmentKey is returned when a request references a file the caller
// did not upload to this school.
var errAttachmentKey = errors.New("lampiran tidak valid; unggah ulang berkasnya")

func srAttachmentPrefix(schoolID, userID string) string {
	return "service-requests/" + schoolID + "/" + userID + "/"
}

// canHandleServiceRequest: school admin, superadmin, or holder of the
// category's handler role (FR-81 role matrix).
func canHandleServiceRequest(c *common.Claims, handlerRole string) bool {
	return c.HasRole(common.RoleAdmin) || c.HasRole(common.RoleSuperadmin) || (handlerRole != "" && c.HasRole(handlerRole))
}

// normalizeAttachments parses an attachments payload and requires every key
// to be one the caller uploaded for this school. Empty input is "[]".
func normalizeAttachments(raw json.RawMessage, schoolID, userID string) (json.RawMessage, error) {
	if len(raw) == 0 || string(raw) == "null" {
		return json.RawMessage("[]"), nil
	}
	var list []model.Attachment
	if err := json.Unmarshal(raw, &list); err != nil {
		return nil, errAttachmentKey
	}
	prefix := srAttachmentPrefix(schoolID, userID)
	for i := range list {
		if !strings.HasPrefix(list[i].Key, prefix) || strings.Contains(list[i].Key, "..") {
			return nil, errAttachmentKey
		}
		list[i].URL = ""
	}
	out, err := json.Marshal(list)
	return out, err
}

func srRandomHex(n int) string {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// UploadServiceRequestAttachment: POST /service-request-attachments
// (multipart "file"). Parents attach supporting documents, handlers attach
// result documents. PDF/JPEG/PNG only, size capped by MAX_UPLOAD_BYTES.
func (h *Handler) UploadServiceRequestAttachment(w http.ResponseWriter, r *http.Request) {
	claims := common.FromCtx(r)
	if claims == nil {
		common.WriteError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	schoolID := common.EffectiveSchoolID(r)
	if schoolID == "" {
		common.WriteError(w, http.StatusBadRequest, "school_id wajib")
		return
	}
	if h.ServiceDesk == nil || h.ServiceDesk.Files == nil {
		common.WriteError(w, http.StatusServiceUnavailable, "penyimpanan lampiran belum dikonfigurasi di server")
		return
	}
	max := h.Config.MaxUploadBytes
	if max <= 0 {
		max = 10 << 20
	}
	r.Body = http.MaxBytesReader(w, r.Body, max+1<<20)
	if err := r.ParseMultipartForm(max); err != nil {
		common.WriteError(w, http.StatusBadRequest, "berkas terlalu besar atau tidak terbaca")
		return
	}
	f, fh, err := r.FormFile("file")
	if err != nil {
		common.WriteError(w, http.StatusBadRequest, "berkas wajib dipilih")
		return
	}
	defer f.Close()
	name := srUnsafeName.ReplaceAllString(filepath.Base(fh.Filename), "_")
	key := srAttachmentPrefix(schoolID, claims.GetUserID()) + srRandomHex(12) + "-" + name
	obj, err := h.ServiceDesk.Files.Put(r.Context(), key, fh.Header.Get("Content-Type"), f, fh.Size)
	if err != nil {
		common.WriteError(w, http.StatusUnprocessableEntity, "lampiran ditolak (hanya PDF/JPEG/PNG, ukuran maksimum dibatasi)")
		return
	}
	common.WriteJSON(w, http.StatusCreated, model.Attachment{Name: fh.Filename, Key: obj.Key, MimeType: obj.MIME, Size: obj.Size}, nil)
}

// DownloadServiceRequestAttachment: GET /service-requests/{id}/files?key=.
// Same access rule as the detail (requester or handler); the key must be
// one of the request's own or result attachments.
func (h *Handler) DownloadServiceRequestAttachment(w http.ResponseWriter, r *http.Request) {
	claims := common.FromCtx(r)
	if claims == nil {
		common.WriteError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	schoolID := common.EffectiveSchoolID(r)
	sr, err := h.Store.GetServiceRequest(r.Context(), schoolID, chi.URLParam(r, "id"))
	if errors.Is(err, store.ErrNotFound) {
		common.WriteError(w, http.StatusNotFound, "permintaan layanan tidak ditemukan")
		return
	} else if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "gagal memuat permintaan layanan")
		return
	}
	if sr.RequesterUserID != claims.GetUserID() && !canHandleServiceRequest(claims, sr.HandlerRole) {
		common.WriteError(w, http.StatusForbidden, "tidak memiliki hak akses ke permintaan layanan ini")
		return
	}
	key := r.URL.Query().Get("key")
	var found *model.Attachment
	for _, raw := range []json.RawMessage{sr.Attachments, sr.ResultAttachments} {
		var list []model.Attachment
		_ = json.Unmarshal(raw, &list)
		for i := range list {
			if key != "" && list[i].Key == key {
				found = &list[i]
			}
		}
	}
	if found == nil {
		common.WriteError(w, http.StatusNotFound, "berkas tidak ditemukan")
		return
	}
	if h.ServiceDesk == nil || h.ServiceDesk.Files == nil {
		common.WriteError(w, http.StatusServiceUnavailable, "penyimpanan lampiran belum dikonfigurasi di server")
		return
	}
	f, err := h.ServiceDesk.Files.Open(r.Context(), found.Key)
	if err != nil {
		// Requests filed before attachments were stored only kept a name.
		common.WriteError(w, http.StatusNotFound, "berkas tidak tersedia di penyimpanan")
		return
	}
	defer f.Close()
	mime := found.MimeType
	if mime == "" {
		mime = "application/octet-stream"
	}
	w.Header().Set("Content-Type", mime)
	w.Header().Set("Content-Disposition", `attachment; filename="`+srUnsafeName.ReplaceAllString(found.Name, "_")+`"`)
	w.Header().Set("Cache-Control", "private, no-store")
	_, _ = io.Copy(w, f)
}

// requireServiceRequestHandler: only admin/superadmin or the category's
// handler role may move a request or attach its result; before this a
// parent (or any staff) could close any request of the school.
func (h *Handler) requireServiceRequestHandler(w http.ResponseWriter, r *http.Request, claims *common.Claims, schoolID, id string) bool {
	sr, err := h.Store.GetServiceRequest(r.Context(), schoolID, id)
	if errors.Is(err, store.ErrNotFound) {
		common.WriteError(w, http.StatusNotFound, "permintaan layanan tidak ditemukan")
		return false
	} else if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "gagal memuat permintaan layanan")
		return false
	}
	if !canHandleServiceRequest(claims, sr.HandlerRole) {
		common.WriteError(w, http.StatusForbidden, "hanya petugas kategori ini atau admin sekolah yang dapat memproses permintaan")
		return false
	}
	return true
}
