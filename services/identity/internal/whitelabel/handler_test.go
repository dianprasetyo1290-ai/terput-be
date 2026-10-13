package whitelabel

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"skci/identity/internal/common"
)

func TestHandler_NormalizeAndPublicManifest(t *testing.T) {
	// 1. NormalizeHost test
	if NormalizeHost("Sub.Teratai.ID:8080") != "sub.teratai.id" {
		t.Errorf("NormalizeHost failed")
	}

	// 2. BuildManifest test AC-159-01
	draft := BrandingDraft{
		SchoolID:        "sch-01",
		AppName:         "Teratai App",
		ShortName:       "Teratai",
		ThemeColor:      "#16A34A",
		BackgroundColor: "#FFFFFF",
	}

	manifest := BuildManifest(draft, 1, "/")
	mBytes, err := json.Marshal(manifest)
	if err != nil {
		t.Fatalf("marshal error: %v", err)
	}

	mStr := strings.ToUpper(string(mBytes))
	if strings.Contains(mStr, "SEKOCI") {
		t.Fatalf("manifest contains SEKOCI: %s", string(mBytes))
	}
	if manifest.Name != "Teratai App" || manifest.ThemeColor != "#16A34A" {
		t.Errorf("manifest mismatch: %+v", manifest)
	}
}

func TestHandler_RoleCheck_AC159_01(t *testing.T) {
	h := &Handler{
		Store: nil,
	}

	// Guru is denied (403)
	req := httptest.NewRequest("GET", "/white-label/brandings?school_id=sch-1", nil)
	req = req.WithContext(context.WithValue(req.Context(), "claims", &common.Claims{
		SchoolID:   "sch-1",
		ActiveRole: common.RoleGuru,
	}))
	w := httptest.NewRecorder()
	h.GetDraft(w, req)

	if w.Code != http.StatusForbidden {
		t.Errorf("expected 403 for guru on branding management, got %d", w.Code)
	}
}

func TestHandler_PublicAssetLinks(t *testing.T) {
	h := &Handler{
		Store: nil,
	}

	r := chi.NewRouter()
	r.Get("/public/assetlinks.json", h.PublicAssetLinks)

	req := httptest.NewRequest("GET", "/public/assetlinks.json?host=unknown.sch.id", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	// Host without listing returns empty array [] with 200
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	if strings.TrimSpace(w.Body.String()) != "[]" {
		t.Errorf("expected empty array '[]', got %s", w.Body.String())
	}
}
