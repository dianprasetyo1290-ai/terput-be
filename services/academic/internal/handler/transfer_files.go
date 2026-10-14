package handler

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/go-chi/chi/v5"

	"skci/academic/internal/common"
	"skci/academic/internal/model"
)

// Menu 78 attachments (QA-PAY round 2). Before this the form only stored a
// typed file name with a made-up key; nothing was uploaded and the detail
// page could not open it. Upload first, then reference the returned
// object_key in the request; download goes through the request's own
// access rule (role + wali kelas class).

const statusRequestMaxUpload = 10 << 20 // 10 MB

var statusRequestAllowedExt = map[string]bool{".pdf": true, ".jpg": true, ".jpeg": true, ".png": true}

func statusRequestAttachmentPrefix(schoolID, userID string) string {
	return schoolID + "/transfers/" + userID + "/"
}

// normalizeStatusRequestAttachments keeps only keys the caller uploaded to
// this school and fills an empty name from the key.
func normalizeStatusRequestAttachments(in []model.AttachmentItem, schoolID, userID string) ([]model.AttachmentItem, error) {
	prefix := statusRequestAttachmentPrefix(schoolID, userID)
	out := make([]model.AttachmentItem, 0, len(in))
	for _, a := range in {
		if !strings.HasPrefix(a.ObjectKey, prefix) || strings.Contains(a.ObjectKey, "..") {
			return nil, errors.New("lampiran tidak valid; unggah ulang berkasnya")
		}
		if strings.TrimSpace(a.Name) == "" {
			a.Name = filepath.Base(a.ObjectKey)
		}
		out = append(out, a)
	}
	return out, nil
}

// UploadStatusRequestAttachment is POST /student-status-request-attachments
// (multipart "file"; PDF/JPEG/PNG up to 10 MB), for roles that may submit.
func (h *Handler) UploadStatusRequestAttachment(w http.ResponseWriter, r *http.Request) {
	claims, ok := requireMenu78Role(w, r, model.CanCreateStatusRequest)
	if !ok {
		return
	}
	schoolID, ok := scopeSchoolID(w, r)
	if !ok {
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, statusRequestMaxUpload+(1<<20))
	if err := r.ParseMultipartForm(8 << 20); err != nil {
		common.WriteError(w, http.StatusBadRequest, "form tidak valid atau berkas melebihi 10 MB")
		return
	}
	f, hdr, err := r.FormFile("file")
	if err != nil {
		common.WriteError(w, http.StatusBadRequest, "berkas wajib dipilih")
		return
	}
	defer f.Close()
	ext := strings.ToLower(filepath.Ext(hdr.Filename))
	if !statusRequestAllowedExt[ext] {
		common.WriteError(w, http.StatusUnprocessableEntity, "tipe berkas tidak didukung (PDF, JPG, PNG)")
		return
	}
	if hdr.Size <= 0 || hdr.Size > statusRequestMaxUpload {
		common.WriteError(w, http.StatusUnprocessableEntity, "ukuran berkas maksimal 10 MB")
		return
	}
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		common.WriteError(w, http.StatusInternalServerError, "gagal menyimpan berkas")
		return
	}
	key := statusRequestAttachmentPrefix(schoolID, claims.UserID) + hex.EncodeToString(b) + ext
	p := filepath.Join(learningStorageRoot(), key)
	if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
		common.WriteError(w, http.StatusServiceUnavailable, "penyimpanan berkas belum dikonfigurasi di server (STORAGE_ROOT)")
		return
	}
	out, err := os.OpenFile(p, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		common.WriteError(w, http.StatusServiceUnavailable, "penyimpanan berkas belum dikonfigurasi di server (STORAGE_ROOT)")
		return
	}
	n, copyErr := io.Copy(out, io.LimitReader(f, statusRequestMaxUpload+1))
	closeErr := out.Close()
	if copyErr != nil || closeErr != nil || n > statusRequestMaxUpload {
		_ = os.Remove(p)
		common.WriteError(w, http.StatusInternalServerError, "gagal menyimpan berkas")
		return
	}
	mt := mime.TypeByExtension(ext)
	if mt == "" {
		mt = "application/octet-stream"
	}
	common.WriteJSON(w, http.StatusCreated, model.AttachmentItem{
		Name: filepath.Base(hdr.Filename), ObjectKey: key, Mime: mt, SizeBytes: n,
	}, nil)
}

// DownloadStatusRequestAttachment is GET /student-status-requests/{id}/files?key=.
func (h *Handler) DownloadStatusRequestAttachment(w http.ResponseWriter, r *http.Request) {
	if _, ok := requireMenu78Role(w, r, model.CanViewStatusRequests); !ok {
		return
	}
	schoolID, ok := scopeSchoolID(w, r)
	if !ok {
		return
	}
	req, err := h.Store.GetStudentStatusRequest(r.Context(), schoolID, chi.URLParam(r, "id"))
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "failed to fetch status request: "+err.Error())
		return
	}
	if req == nil {
		common.WriteError(w, http.StatusNotFound, "status request not found")
		return
	}
	if !h.requireOwnStatusRequestIfWaliKelas(w, r, schoolID, req) {
		return
	}
	key := r.URL.Query().Get("key")
	var list []model.AttachmentItem
	_ = json.Unmarshal(req.Attachments, &list)
	for _, a := range list {
		if key != "" && a.ObjectKey == key {
			k, n, m := a.ObjectKey, a.Name, a.Mime
			serveLearningFile(w, &k, &n, &m)
			return
		}
	}
	common.WriteError(w, http.StatusNotFound, "berkas tidak ditemukan pada pengajuan ini")
}
