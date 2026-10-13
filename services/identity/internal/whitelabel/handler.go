package whitelabel

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"skci/identity/internal/common"
)

type Handler struct {
	Store       *Store
	CoreURL     string
	InternalKey string
}

func NewHandler(s *Store, coreURL, internalKey string) *Handler {
	return &Handler{
		Store:       s,
		CoreURL:     coreURL,
		InternalKey: internalKey,
	}
}

func (h *Handler) canAccessSchool(claims *common.Claims, targetSchoolID string) (bool, error) {
	if claims == nil {
		return false, nil
	}
	if claims.ActiveRole == common.RoleSuperadmin {
		return true, nil
	}
	if claims.ActiveRole == common.RoleKepalaYayasan {
		// Must belong to user's foundation
		targetFound, err := h.Store.GetSchoolFoundationID(context.Background(), targetSchoolID)
		if err != nil || targetFound == "" {
			return false, err
		}
		userFound, err := h.Store.GetSchoolFoundationID(context.Background(), claims.SchoolID)
		if err != nil || userFound == "" {
			return false, err
		}
		return targetFound == userFound, nil
	}
	return false, nil
}

func (h *Handler) recordAudit(schoolID, action, entityID string, actorUserID, actorRole string, changes any) {
	if h.CoreURL == "" {
		return
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()

		payload := map[string]any{
			"school_id":     schoolID,
			"service":       "identity",
			"module":        "white_label",
			"entity_type":   "branding",
			"entity_id":     entityID,
			"action":        action,
			"actor_user_id": actorUserID,
			"actor_role":    actorRole,
			"after":         changes,
		}
		body, _ := json.Marshal(payload)
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(h.CoreURL, "/")+"/internal/audit", bytes.NewReader(body))
		if err != nil {
			return
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Internal-Key", h.InternalKey)

		cl := &http.Client{Timeout: 2 * time.Second}
		resp, err := cl.Do(req)
		if err == nil {
			_ = resp.Body.Close()
		}
	}()
}

// GetDraft handles GET /white-label/brandings?school_id=
func (h *Handler) GetDraft(w http.ResponseWriter, r *http.Request) {
	claims := common.FromCtx(r)
	schoolID := r.URL.Query().Get("school_id")
	if schoolID == "" && claims != nil {
		schoolID = claims.SchoolID
	}
	if schoolID == "" {
		common.WriteError(w, http.StatusBadRequest, "school_id wajib disertakan")
		return
	}

	ok, err := h.canAccessSchool(claims, schoolID)
	if err != nil || !ok {
		common.WriteError(w, http.StatusForbidden, "Akses ditolak")
		return
	}

	active, _ := h.Store.IsFeatureActive(r.Context(), schoolID, "white_label")
	if !active {
		common.WriteError(w, http.StatusForbidden, "Fitur white_label belum diaktifkan untuk sekolah ini")
		return
	}

	draft, err := h.Store.GetDraft(r.Context(), schoolID)
	if errors.Is(err, ErrNotFound) {
		// Return template default
		defaultDraft := BrandingDraft{
			SchoolID:        schoolID,
			AppName:         "",
			ShortName:       "",
			ThemeColor:      "#16A34A",
			BackgroundColor: "#FFFFFF",
			IncludedPortals: []string{"parent", "student", "teacher"},
		}
		common.WriteJSON(w, http.StatusOK, defaultDraft, nil)
		return
	}
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "Gagal memuat draf merek")
		return
	}

	common.WriteJSON(w, http.StatusOK, draft, nil)
}

// SaveDraft handles PUT /white-label/brandings/{schoolId}
func (h *Handler) SaveDraft(w http.ResponseWriter, r *http.Request) {
	claims := common.FromCtx(r)
	schoolID := chi.URLParam(r, "schoolId")

	ok, err := h.canAccessSchool(claims, schoolID)
	if err != nil || !ok {
		common.WriteError(w, http.StatusForbidden, "Akses ditolak")
		return
	}

	active, _ := h.Store.IsFeatureActive(r.Context(), schoolID, "white_label")
	if !active {
		common.WriteError(w, http.StatusForbidden, "Fitur white_label belum diaktifkan untuk sekolah ini")
		return
	}

	var req BrandingDraft
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		common.WriteError(w, http.StatusBadRequest, "Payload JSON tidak valid")
		return
	}
	req.SchoolID = schoolID

	valErrs := ValidateBranding(req, false)
	if len(valErrs) > 0 {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnprocessableEntity)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"message": "Validasi merek gagal",
			"errors":  valErrs,
		})
		return
	}

	updatedBy := ""
	if claims != nil {
		updatedBy = claims.UserID
	}

	saved, err := h.Store.UpsertDraft(r.Context(), req, updatedBy)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "Gagal menyimpan draf merek")
		return
	}

	// Audit without large data URLs
	var changed []string
	if req.Icon192 != nil {
		changed = append(changed, "icon_192")
	}
	if req.Icon512 != nil {
		changed = append(changed, "icon_512")
	}
	if req.IconMaskable != nil {
		changed = append(changed, "icon_maskable")
	}
	if req.Splash != nil {
		changed = append(changed, "splash")
	}
	h.recordAudit(schoolID, "update_branding_draft", saved.ID, claims.UserID, claims.ActiveRole, map[string]any{
		"app_name": saved.AppName, "theme_color": saved.ThemeColor, "changed_icons": changed,
	})

	common.WriteJSON(w, http.StatusOK, saved, nil)
}

// PublishVersion handles POST /white-label/brandings/{schoolId}/publish
func (h *Handler) PublishVersion(w http.ResponseWriter, r *http.Request) {
	claims := common.FromCtx(r)
	schoolID := chi.URLParam(r, "schoolId")

	ok, err := h.canAccessSchool(claims, schoolID)
	if err != nil || !ok {
		common.WriteError(w, http.StatusForbidden, "Akses ditolak")
		return
	}

	active, _ := h.Store.IsFeatureActive(r.Context(), schoolID, "white_label")
	if !active {
		common.WriteError(w, http.StatusForbidden, "Fitur white_label belum diaktifkan untuk sekolah ini")
		return
	}

	publishedBy := ""
	if claims != nil {
		publishedBy = claims.UserID
	}

	bv, valErrors, err := h.Store.Publish(r.Context(), schoolID, publishedBy)
	if len(valErrors) > 0 {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnprocessableEntity)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"message": "Validasi penerbitan gagal",
			"errors":  valErrors,
		})
		return
	}
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "Gagal menerbitkan versi merek")
		return
	}

	h.recordAudit(schoolID, "publish_branding_version", bv.ID, claims.UserID, claims.ActiveRole, map[string]any{
		"version": bv.Version,
	})

	common.WriteJSON(w, http.StatusOK, bv, nil)
}

// ListVersions handles GET /white-label/brandings/{schoolId}/versions
func (h *Handler) ListVersions(w http.ResponseWriter, r *http.Request) {
	claims := common.FromCtx(r)
	schoolID := chi.URLParam(r, "schoolId")

	ok, err := h.canAccessSchool(claims, schoolID)
	if err != nil || !ok {
		common.WriteError(w, http.StatusForbidden, "Akses ditolak")
		return
	}

	versions, err := h.Store.ListVersions(r.Context(), schoolID)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "Gagal memuat riwayat versi")
		return
	}

	common.WriteJSON(w, http.StatusOK, versions, nil)
}

// UpdateDomain handles PUT /white-label/brandings/{schoolId}/domain (superadmin only)
func (h *Handler) UpdateDomain(w http.ResponseWriter, r *http.Request) {
	claims := common.FromCtx(r)
	if claims == nil || claims.ActiveRole != common.RoleSuperadmin {
		common.WriteError(w, http.StatusForbidden, "Hanya superadmin yang dapat mengubah domain")
		return
	}

	schoolID := chi.URLParam(r, "schoolId")
	var req struct {
		Domain string `json:"domain"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		common.WriteError(w, http.StatusBadRequest, "Payload tidak valid")
		return
	}

	err := h.Store.UpdateDomain(r.Context(), schoolID, req.Domain)
	if errors.Is(err, ErrConflict) {
		common.WriteError(w, http.StatusConflict, "Domain sudah digunakan oleh sekolah lain")
		return
	}
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "Gagal memperbarui domain")
		return
	}

	h.recordAudit(schoolID, "update_domain", schoolID, claims.UserID, claims.ActiveRole, map[string]any{"domain": req.Domain})
	common.WriteJSON(w, http.StatusOK, map[string]string{"message": "Domain berhasil diperbarui"}, nil)
}

// GetStoreListing handles GET /white-label/store-listings/{schoolId}/{store}
func (h *Handler) GetStoreListing(w http.ResponseWriter, r *http.Request) {
	claims := common.FromCtx(r)
	schoolID := chi.URLParam(r, "schoolId")
	storeName := chi.URLParam(r, "store")

	ok, err := h.canAccessSchool(claims, schoolID)
	if err != nil || !ok {
		common.WriteError(w, http.StatusForbidden, "Akses ditolak")
		return
	}

	sl, err := h.Store.GetStoreListing(r.Context(), schoolID, storeName)
	if errors.Is(err, ErrNotFound) {
		// Return empty template
		sl = &StoreListing{
			SchoolID:           schoolID,
			Store:              storeName,
			AccountOwner:       "fullcircle",
			Status:             "not_started",
			SHA256Fingerprints: []string{},
		}
	} else if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "Gagal memuat listing toko")
		return
	}

	common.WriteJSON(w, http.StatusOK, sl, nil)
}

// SaveStoreListing handles PUT /white-label/store-listings/{schoolId}/{store} (superadmin only)
func (h *Handler) SaveStoreListing(w http.ResponseWriter, r *http.Request) {
	claims := common.FromCtx(r)
	if claims == nil || claims.ActiveRole != common.RoleSuperadmin {
		common.WriteError(w, http.StatusForbidden, "Hanya superadmin yang dapat mengedit data listing toko")
		return
	}

	schoolID := chi.URLParam(r, "schoolId")
	storeName := chi.URLParam(r, "store")

	var req StoreListing
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		common.WriteError(w, http.StatusBadRequest, "Payload tidak valid")
		return
	}
	req.SchoolID = schoolID
	req.Store = storeName

	out, err := h.Store.UpsertStoreListing(r.Context(), req, claims.UserID)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "Gagal menyimpan listing toko")
		return
	}

	h.recordAudit(schoolID, "update_store_listing", out.ID, claims.UserID, claims.ActiveRole, out)
	common.WriteJSON(w, http.StatusOK, out, nil)
}

// -----------------------------------------------------------------------------
// PUBLIC ENDPOINTS (No JWT)
// -----------------------------------------------------------------------------

// PublicBranding handles GET /public/branding?host=
func (h *Handler) PublicBranding(w http.ResponseWriter, r *http.Request) {
	if h.Store == nil {
		common.WriteError(w, http.StatusNotFound, "Merek white-label tidak ditemukan untuk domain ini")
		return
	}
	host := r.URL.Query().Get("host")
	if host == "" {
		host = r.Header.Get("X-Forwarded-Host")
	}
	if host == "" {
		host = r.Host
	}

	draft, version, err := h.Store.GetPublishedByHost(r.Context(), host)
	if err != nil {
		common.WriteError(w, http.StatusNotFound, "Merek white-label tidak ditemukan untuk domain ini")
		return
	}

	w.Header().Set("Cache-Control", "public, max-age=60")
	common.WriteJSON(w, http.StatusOK, map[string]any{
		"branding": draft,
		"version":  version,
	}, nil)
}

// PublicManifest handles GET /public/manifest.webmanifest?host=
func (h *Handler) PublicManifest(w http.ResponseWriter, r *http.Request) {
	if h.Store == nil {
		w.Header().Set("Content-Type", "application/manifest+json")
		w.WriteHeader(http.StatusNotFound)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "Manifest tidak ditemukan"})
		return
	}
	host := r.URL.Query().Get("host")
	if host == "" {
		host = r.Header.Get("X-Forwarded-Host")
	}
	if host == "" {
		host = r.Host
	}

	draft, version, err := h.Store.GetPublishedByHost(r.Context(), host)
	if err != nil {
		w.Header().Set("Content-Type", "application/manifest+json")
		w.WriteHeader(http.StatusNotFound)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "Manifest tidak ditemukan"})
		return
	}

	baseURL := "/"
	manifest := BuildManifest(*draft, version, baseURL)

	w.Header().Set("Content-Type", "application/manifest+json")
	w.Header().Set("Cache-Control", "public, max-age=300")
	_ = json.NewEncoder(w).Encode(manifest)
}

// PublicIcon handles GET /public/branding/{schoolId}/{kind}.png?v=
func (h *Handler) PublicIcon(w http.ResponseWriter, r *http.Request) {
	if h.Store == nil {
		http.NotFound(w, r)
		return
	}
	schoolID := chi.URLParam(r, "schoolId")
	kind := chi.URLParam(r, "kind")

	draft, version, err := h.Store.GetPublishedBySchool(r.Context(), schoolID)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	var dataURL *string
	switch kind {
	case "icon_192":
		dataURL = draft.Icon192
	case "icon_512":
		dataURL = draft.Icon512
	case "icon_maskable":
		dataURL = draft.IconMaskable
	case "splash":
		dataURL = draft.Splash
	}

	if dataURL == nil || *dataURL == "" {
		http.NotFound(w, r)
		return
	}

	// Strip data:image/png;base64,
	idx := strings.Index(*dataURL, ",")
	if idx == -1 {
		http.NotFound(w, r)
		return
	}
	rawB64 := (*dataURL)[idx+1:]
	pngBytes, err := base64.StdEncoding.DecodeString(rawB64)
	if err != nil {
		http.Error(w, "Gagal mendekode gambar", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "image/png")
	w.Header().Set("Cache-Control", "public, max-age=86400, immutable")
	w.Header().Set("ETag", fmt.Sprintf(`W/"%d-%s"`, version, kind))
	_, _ = w.Write(pngBytes)
}

// PublicAssetLinks handles GET /public/assetlinks.json?host=
func (h *Handler) PublicAssetLinks(w http.ResponseWriter, r *http.Request) {
	if h.Store == nil {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte("[]"))
		return
	}
	host := r.URL.Query().Get("host")
	if host == "" {
		host = r.Header.Get("X-Forwarded-Host")
	}
	if host == "" {
		host = r.Host
	}

	draft, _, err := h.Store.GetPublishedByHost(r.Context(), host)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte("[]"))
		return
	}

	listing, err := h.Store.GetStoreListing(r.Context(), draft.SchoolID, "google_play")
	if err != nil || listing == nil || listing.PackageID == nil || len(listing.SHA256Fingerprints) == 0 {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte("[]"))
		return
	}

	assetLinks := []map[string]any{
		{
			"relation": []string{"delegate_permission/common.handle_all_urls"},
			"target": map[string]any{
				"namespace":                "android_app",
				"package_name":             *listing.PackageID,
				"sha256_cert_fingerprints": listing.SHA256Fingerprints,
			},
		},
	}

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "public, max-age=3600")
	_ = json.NewEncoder(w).Encode(assetLinks)
}
