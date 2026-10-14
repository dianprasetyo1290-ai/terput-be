package ats

import (
	"bytes"
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"strings"

	"github.com/go-chi/chi/v5"

	"skci/hr/internal/common"
)

// maxCVBytes caps a candidate CV upload.
const maxCVBytes = 5 << 20

type applyBody struct {
	FullName          string `json:"full_name"`
	Email             string `json:"email"`
	ConsentTalentPool bool   `json:"consent_talent_pool"`
	cv                []byte
	cvName            string
}

var errBadCV = errors.New("CV harus berupa berkas PDF maksimal 5 MB")

// readApplyBody accepts multipart/form-data (full_name, email,
// consent_talent_pool, cv file) or JSON without a CV. A client-sent
// cv_object_key is ignored: object keys are only ever assigned here, so a
// public caller cannot point an application at someone else's file.
func readApplyBody(r *http.Request) (applyBody, error) {
	var b applyBody
	if !strings.HasPrefix(r.Header.Get("Content-Type"), "multipart/form-data") {
		if err := json.NewDecoder(r.Body).Decode(&b); err != nil {
			return b, errors.New("format json tidak valid")
		}
		return b, nil
	}
	r.Body = http.MaxBytesReader(nil, r.Body, maxCVBytes+(1<<20))
	if err := r.ParseMultipartForm(maxCVBytes + (1 << 20)); err != nil {
		return b, errBadCV
	}
	b.FullName = r.FormValue("full_name")
	b.Email = r.FormValue("email")
	b.ConsentTalentPool = r.FormValue("consent_talent_pool") == "true"
	f, hdr, err := r.FormFile("cv")
	if errors.Is(err, http.ErrMissingFile) {
		return b, nil
	}
	if err != nil {
		return b, errBadCV
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, maxCVBytes+1))
	if err != nil || len(data) > maxCVBytes || !bytes.HasPrefix(data, []byte("%PDF-")) ||
		!strings.EqualFold(filepath.Ext(hdr.Filename), ".pdf") {
		return b, errBadCV
	}
	b.cv, b.cvName = data, hdr.Filename
	return b, nil
}

// storeCV saves the uploaded CV and returns its object key ("" when none).
func (h *Handler) storeCV(ctx context.Context, schoolID string, b applyBody) (string, error) {
	if len(b.cv) == 0 {
		return "", nil
	}
	if h.Files == nil {
		return "", errors.New("penyimpanan berkas belum dikonfigurasi")
	}
	rnd := make([]byte, 16)
	if _, err := rand.Read(rnd); err != nil {
		return "", err
	}
	key := fmt.Sprintf("ats/cv/%s/%s.pdf", schoolID, hex.EncodeToString(rnd))
	if _, err := h.Files.Put(ctx, key, "application/pdf", bytes.NewReader(b.cv), int64(len(b.cv))); err != nil {
		return "", err
	}
	return key, nil
}

// DownloadCV: GET /ats/applications/{id}/cv (candidate readers).
func (h *Handler) DownloadCV(w http.ResponseWriter, r *http.Request) {
	schoolID := common.EffectiveSchoolID(r)
	var key, name string
	err := h.Store.DB.QueryRowContext(r.Context(), `
		SELECT COALESCE(a.cv_object_key, ''), c.full_name
		FROM applications a JOIN candidates c ON c.id = a.candidate_id
		WHERE a.id = $1 AND a.school_id = $2`, chi.URLParam(r, "id"), schoolID).Scan(&key, &name)
	if errors.Is(err, sql.ErrNoRows) {
		common.WriteError(w, http.StatusNotFound, "lamaran tidak ditemukan")
		return
	}
	if err != nil {
		common.InternalError(w, "gagal memuat lamaran", err)
		return
	}
	if !strings.HasPrefix(key, "ats/cv/") {
		common.WriteError(w, http.StatusNotFound, "kandidat tidak mengunggah CV")
		return
	}
	if h.Files == nil {
		common.WriteError(w, http.StatusServiceUnavailable, "penyimpanan berkas belum dikonfigurasi")
		return
	}
	rc, err := h.Files.Open(r.Context(), key)
	if err != nil {
		common.WriteError(w, http.StatusNotFound, "berkas CV tidak ditemukan di penyimpanan")
		return
	}
	defer rc.Close()
	safe := strings.Map(func(c rune) rune {
		if c == '"' || c == '\\' || c < 32 {
			return '_'
		}
		return c
	}, name)
	w.Header().Set("Content-Type", "application/pdf")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="CV_%s.pdf"`, safe))
	w.WriteHeader(http.StatusOK)
	_, _ = io.Copy(w, rc)
}
