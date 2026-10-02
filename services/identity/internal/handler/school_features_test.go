package handler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"skci/identity/internal/common"
	"skci/identity/internal/store"
)

func withClaims(r *http.Request, role, schoolID string) *http.Request {
	c := &common.Claims{UserID: "u1", SchoolID: schoolID, ActiveRole: role}
	return r.WithContext(context.WithValue(r.Context(), common.ClaimsCtxKey, c))
}

// Hanya superadmin yang boleh menyalakan/mematikan fitur. App.DB sengaja nil:
// kalau otorisasi bocor, test ini panik/500 bukan lewat diam-diam.
func TestSetSchoolFeatureSuperadminOnly(t *testing.T) {
	app := &App{}
	for _, role := range []string{common.RoleAdmin, common.RoleGuru, common.RoleKepalaSekolah, common.RoleKepalaYayasan} {
		r := withClaims(httptest.NewRequest(http.MethodPut, "/schools/s1/features/accounting", strings.NewReader(`{"enabled":true}`)), role, "s1")
		w := httptest.NewRecorder()
		app.SetSchoolFeature(w, r)
		if w.Code != http.StatusForbidden {
			t.Fatalf("role %s: got %d, want 403", role, w.Code)
		}
	}
	// Tanpa token sama sekali.
	w := httptest.NewRecorder()
	app.SetSchoolFeature(w, httptest.NewRequest(http.MethodPut, "/schools/s1/features/accounting", strings.NewReader(`{}`)))
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("anonymous: got %d, want 401", w.Code)
	}
}

// Admin sekolah lain tidak boleh membaca status fitur sekolah bukan miliknya.
func TestListSchoolFeaturesForeignSchoolForbidden(t *testing.T) {
	app := &App{}
	r := withClaims(httptest.NewRequest(http.MethodGet, "/schools/s2/features", nil), common.RoleAdmin, "s1")
	// chi URLParam kosong di sini ("" != "s1"), itu justru kasus terburuk:
	// pembacaan lintas sekolah harus tetap ditolak.
	w := httptest.NewRecorder()
	app.ListSchoolFeatures(w, r)
	if w.Code != http.StatusForbidden {
		t.Fatalf("got %d, want 403", w.Code)
	}
}

// Default MATI: sekolah tanpa baris school_features melaporkan semua fitur
// mati, dan tak ada fitur yang masuk daftar aktif.
func TestFeatureDefaultsOff(t *testing.T) {
	for _, existing := range []map[string]bool{nil, {}, {"accounting": false}} {
		for _, s := range store.FeatureStatuses(existing) {
			if s.Enabled {
				t.Fatalf("%v: fitur %s seharusnya mati secara default", existing, s.FeatureKey)
			}
		}
		if got := store.EnabledFeatureKeys(existing); len(got) != 0 {
			t.Fatalf("%v: daftar aktif seharusnya kosong, dapat %v", existing, got)
		}
	}
	if got := store.EnabledFeatureKeys(map[string]bool{"accounting": true}); len(got) != 1 || got[0] != "accounting" {
		t.Fatalf("fitur yang dinyalakan tidak muncul: %v", got)
	}
	// feature_key sembarang tidak boleh lolos ke tabel.
	if store.IsKnownFeature("rm -rf") || !store.IsKnownFeature("accounting") {
		t.Fatal("allowlist feature_key salah")
	}
}
