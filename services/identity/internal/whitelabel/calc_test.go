package whitelabel

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestValidateBranding_AC159_01(t *testing.T) {
	icon192 := "data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAAMAAAADACAYAAABS3GwH"
	icon512 := "data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAAgAAAAIACAYAAAD0eNT6"
	privacy := "https://teratai.sch.id/privacy"
	domain := "teratai.sch.id"

	validDraft := BrandingDraft{
		SchoolID:         "sch-1",
		AppName:          "Teratai App",
		ShortName:        "Teratai",
		ThemeColor:       "#16A34A",
		BackgroundColor:  "#FFFFFF",
		Icon192:          &icon192,
		Icon512:          &icon512,
		Domain:           &domain,
		IncludedPortals:  []string{"parent", "student", "teacher"},
		PrivacyPolicyURL: &privacy,
	}

	// 1. Valid branding for publishing -> 0 errors
	errs := ValidateBranding(validDraft, true)
	if len(errs) != 0 {
		t.Fatalf("expected 0 errors, got: %+v", errs)
	}

	// 2. short_name > 12 chars -> error
	invalidShort := validDraft
	invalidShort.ShortName = "TerataiSatuDuaTiga" // 18 chars
	errs = ValidateBranding(invalidShort, false)
	if len(errs) == 0 || errs[0].Field != "short_name" {
		t.Errorf("expected short_name error, got: %+v", errs)
	}

	// 3. invalid hex color -> error
	invalidHex := validDraft
	invalidHex.ThemeColor = "green"
	errs = ValidateBranding(invalidHex, false)
	if len(errs) == 0 || errs[0].Field != "theme_color" {
		t.Errorf("expected theme_color error, got: %+v", errs)
	}

	// 4. publish without privacy_policy_url -> error
	noPrivacy := validDraft
	noPrivacy.PrivacyPolicyURL = nil
	errs = ValidateBranding(noPrivacy, true)
	var foundPrivacy bool
	for _, e := range errs {
		if e.Field == "privacy_policy_url" {
			foundPrivacy = true
			if e.Message != "Kebijakan privasi wajib sebelum diterbitkan" {
				t.Errorf("unexpected message: %s", e.Message)
			}
		}
	}
	if !foundPrivacy {
		t.Errorf("expected privacy_policy_url error on publish")
	}

	// 5. non-PNG icon -> error on publish
	badIcon := validDraft
	badIconStr := "data:image/jpeg;base64,abc"
	badIcon.Icon192 = &badIconStr
	errs = ValidateBranding(badIcon, true)
	var foundIcon bool
	for _, e := range errs {
		if e.Field == "icon_192" {
			foundIcon = true
		}
	}
	if !foundIcon {
		t.Errorf("expected icon_192 error for non-png data URL")
	}
}

func TestContrastRatio(t *testing.T) {
	// Vector: #FFFFFF vs #000000 = 21.0
	ratio := ContrastRatio("#FFFFFF", "#000000")
	if ratio != 21.0 {
		t.Errorf("expected contrast 21.0, got %f", ratio)
	}

	// Same color = 1.0
	sameRatio := ContrastRatio("#16A34A", "#16A34A")
	if sameRatio != 1.0 {
		t.Errorf("expected contrast 1.0 for same colors, got %f", sameRatio)
	}
}

func TestNormalizeHost(t *testing.T) {
	// Vector: "Teratai.Example.ID:443" -> "teratai.example.id"
	norm := NormalizeHost("Teratai.Example.ID:443")
	if norm != "teratai.example.id" {
		t.Errorf("expected 'teratai.example.id', got '%s'", norm)
	}

	norm2 := NormalizeHost("  Sekolah.sch.id  ")
	if norm2 != "sekolah.sch.id" {
		t.Errorf("expected 'sekolah.sch.id', got '%s'", norm2)
	}
}

func TestBuildManifest_AC159_01(t *testing.T) {
	b := BrandingDraft{
		SchoolID:        "sch-teratai",
		AppName:         "Teratai App",
		ShortName:       "Teratai",
		ThemeColor:      "#16A34A",
		BackgroundColor: "#FFFFFF",
	}

	manifest := BuildManifest(b, 2, "/")
	if manifest.Name != "Teratai App" || manifest.ShortName != "Teratai" {
		t.Errorf("mismatched name or short_name: %+v", manifest)
	}
	if manifest.ThemeColor != "#16A34A" || manifest.BackgroundColor != "#FFFFFF" {
		t.Errorf("mismatched colors: %+v", manifest)
	}
	if manifest.Display != "standalone" {
		t.Errorf("expected standalone display")
	}
	if len(manifest.Icons) != 2 {
		t.Fatalf("expected 2 icons, got %d", len(manifest.Icons))
	}
	if !strings.Contains(manifest.Icons[0].Src, "?v=2") {
		t.Errorf("expected version 2 in icon src, got %s", manifest.Icons[0].Src)
	}

	// AC-159-01: NO string "SEKOCI" in manifest content (case-insensitive)
	jsonBytes, err := json.Marshal(manifest)
	if err != nil {
		t.Fatalf("failed to marshal manifest: %v", err)
	}
	jsonStr := strings.ToUpper(string(jsonBytes))
	if strings.Contains(jsonStr, "SEKOCI") {
		t.Errorf("AC-159-01 VIOLATION: manifest contains SEKOCI string: %s", string(jsonBytes))
	}
}
