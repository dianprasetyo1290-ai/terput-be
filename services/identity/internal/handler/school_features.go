package handler

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"

	"skci/identity/internal/common"
	"skci/identity/internal/store"
)

// Saklar fitur per sekolah (ADDENDUM 2 desain integrasi accounting).
// Default MATI: tidak ada baris di school_features = fitur mati.
//
//	GET /schools/{id}/features        superadmin, admin sekolah itu (read-only),
//	                                  atau sibling service via X-Internal-Key
//	PUT /schools/{id}/features/{key}  HANYA superadmin
//	GET /me/features                  semua role, fitur aktif sekolah si pemanggil
//	GET /features                      superadmin, semua baris (halaman Data Sekolah)

// ListSchoolFeatures mengembalikan status SEMUA fitur yang dikenal untuk
// satu sekolah (bukan cuma yang punya baris), supaya UI bisa merender
// saklarnya apa adanya.
func (a *App) ListSchoolFeatures(w http.ResponseWriter, r *http.Request) {
	schoolID := chi.URLParam(r, "id")
	claims := common.FromCtx(r)
	switch {
	case claims == nil && common.IsInternal(r):
		// Panggilan antar-service (mis. Laravel /sso memeriksa apakah
		// accounting menyala untuk sekolah itu) — lihat internal_auth.go.
	case claims == nil:
		common.WriteError(w, http.StatusUnauthorized, "Sesi tidak ditemukan. Silakan masuk kembali")
		return
	case claims.ActiveRole != common.RoleSuperadmin && claims.SchoolID != schoolID:
		common.WriteError(w, http.StatusForbidden, "Akses ditolak")
		return
	}
	existing, err := store.SchoolFeatureMap(a.DB, schoolID)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "Gagal memuat fitur unit")
		return
	}
	common.WriteJSON(w, http.StatusOK, store.FeatureStatuses(existing), nil)
}

// SetSchoolFeature menyalakan/mematikan satu fitur. Hanya superadmin —
// jangan izinkan admin sekolah membuka modul keuangannya sendiri.
func (a *App) SetSchoolFeature(w http.ResponseWriter, r *http.Request) {
	if !requireRole(w, r, common.RoleSuperadmin) {
		return
	}
	schoolID := chi.URLParam(r, "id")
	key := chi.URLParam(r, "key")
	if !store.IsKnownFeature(key) {
		common.WriteError(w, http.StatusBadRequest, "Kunci fitur tidak dikenal: "+key)
		return
	}
	var req struct {
		Enabled bool `json:"enabled"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		common.WriteError(w, http.StatusBadRequest, "Status aktif (enabled) wajib diisi")
		return
	}
	if req.Enabled {
		existing, err := store.SchoolFeatureMap(a.DB, schoolID)
		if err != nil {
			common.WriteError(w, http.StatusInternalServerError, "Gagal memuat fitur unit")
			return
		}
		if len(store.MissingRequirements(key, existing)) > 0 {
			common.WriteError(w, http.StatusUnprocessableEntity, "Aktifkan modul Fasilitas terlebih dahulu")
			return
		}
	}
	updatedBy := ""
	if c := common.FromCtx(r); c != nil {
		updatedBy = c.UserID
	}
	if err := store.SetSchoolFeature(a.DB, schoolID, key, req.Enabled, updatedBy); err != nil {
		common.WriteDBError(w, err, "Gagal menyimpan fitur unit")
		return
	}
	common.WriteJSON(w, http.StatusOK, store.FeatureStatus{FeatureKey: key, Enabled: req.Enabled}, nil)
}

// MyFeatures: fitur yang menyala untuk sekolah si pemanggil, dipakai SPA
// sekali setelah login. Superadmin tidak punya school_id di JWT-nya; ia
// memakai sekolah yang sedang dipilih (?school_id=, disuntik otomatis oleh
// web/src/api/client.ts). Tanpa sekolah terpilih hasilnya kosong — menu
// Akuntansi baru muncul setelah superadmin memilih sekolah.
func (a *App) MyFeatures(w http.ResponseWriter, r *http.Request) {
	schoolID, ok := resolveSchoolScope(r)
	if !ok || schoolID == "" {
		common.WriteJSON(w, http.StatusOK, []string{}, nil)
		return
	}
	existing, err := store.SchoolFeatureMap(a.DB, schoolID)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "Gagal memuat fitur")
		return
	}
	common.WriteJSON(w, http.StatusOK, store.EnabledFeatureKeys(existing), nil)
}

// ListAllFeatures: semua baris sekaligus supaya halaman Data Sekolah
// (puluhan/ratusan sekolah) tidak menembak satu request per baris.
func (a *App) ListAllFeatures(w http.ResponseWriter, r *http.Request) {
	if !requireRole(w, r, common.RoleSuperadmin) {
		return
	}
	out, err := store.AllSchoolFeatures(a.DB)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "Gagal memuat daftar fitur")
		return
	}
	common.WriteJSON(w, http.StatusOK, out, nil)
}
