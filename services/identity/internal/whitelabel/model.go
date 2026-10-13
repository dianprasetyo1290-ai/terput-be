package whitelabel

import (
	"encoding/json"
	"fmt"
	"math"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"
)

var hexColorRegex = regexp.MustCompile(`^#[0-9a-fA-F]{6}$`)

type FieldError struct {
	Field   string `json:"field"`
	Message string `json:"message"`
}

type BrandingDraft struct {
	ID               string    `json:"id"`
	SchoolID         string    `json:"school_id"`
	AppName          string    `json:"app_name"`
	ShortName        string    `json:"short_name"`
	ThemeColor       string    `json:"theme_color"`
	BackgroundColor  string    `json:"background_color"`
	Icon192          *string   `json:"icon_192,omitempty"`
	Icon512          *string   `json:"icon_512,omitempty"`
	IconMaskable     *string   `json:"icon_maskable,omitempty"`
	Splash           *string   `json:"splash,omitempty"`
	Domain           *string   `json:"domain,omitempty"`
	IncludedPortals  []string  `json:"included_portals"`
	PrivacyPolicyURL *string   `json:"privacy_policy_url,omitempty"`
	SupportEmail     *string   `json:"support_email,omitempty"`
	UpdatedBy        *string   `json:"updated_by,omitempty"`
	CreatedAt        time.Time `json:"created_at"`
	UpdatedAt        time.Time `json:"updated_at"`
}

type BrandingVersion struct {
	ID           string          `json:"id"`
	SchoolID     string          `json:"school_id"`
	Version      int             `json:"version"`
	Snapshot     json.RawMessage `json:"snapshot"`
	Icon192      *string         `json:"icon_192,omitempty"`
	Icon512      *string         `json:"icon_512,omitempty"`
	IconMaskable *string         `json:"icon_maskable,omitempty"`
	Splash       *string         `json:"splash,omitempty"`
	PublishedBy  *string         `json:"published_by,omitempty"`
	PublishedAt  time.Time       `json:"published_at"`
}

type StoreListing struct {
	ID                 string    `json:"id"`
	SchoolID           string    `json:"school_id"`
	Store              string    `json:"store"` // "google_play" | "app_store"
	AccountOwner       string    `json:"account_owner"` // "client" | "fullcircle"
	PackageID          *string   `json:"package_id,omitempty"`
	SHA256Fingerprints []string  `json:"sha256_fingerprints"`
	Status             string    `json:"status"` // "not_started" | "in_review" | "published" | "rejected"
	ListingURL         *string   `json:"listing_url,omitempty"`
	Note               *string   `json:"note,omitempty"`
	UpdatedBy          *string   `json:"updated_by,omitempty"`
	CreatedAt          time.Time `json:"created_at"`
	UpdatedAt          time.Time `json:"updated_at"`
}

type ManifestIcon struct {
	Src     string `json:"src"`
	Sizes   string `json:"sizes"`
	Type    string `json:"type"`
	Purpose string `json:"purpose,omitempty"`
}

type Manifest struct {
	Name            string         `json:"name"`
	ShortName       string         `json:"short_name"`
	ThemeColor      string         `json:"theme_color"`
	BackgroundColor string         `json:"background_color"`
	Display         string         `json:"display"`
	StartURL        string         `json:"start_url"`
	Scope           string         `json:"scope"`
	Icons           []ManifestIcon `json:"icons"`
}

func ValidateBranding(b BrandingDraft, isPublish bool) []FieldError {
	var errs []FieldError

	if strings.TrimSpace(b.AppName) == "" {
		errs = append(errs, FieldError{Field: "app_name", Message: "Nama aplikasi tidak boleh kosong"})
	}

	short := strings.TrimSpace(b.ShortName)
	if short == "" {
		errs = append(errs, FieldError{Field: "short_name", Message: "Nama pendek tidak boleh kosong"})
	} else if len([]rune(short)) > 12 {
		errs = append(errs, FieldError{Field: "short_name", Message: "Nama pendek maksimal 12 karakter"})
	}

	if !hexColorRegex.MatchString(b.ThemeColor) {
		errs = append(errs, FieldError{Field: "theme_color", Message: "Format theme_color harus hex 6 karakter (misal: #16A34A)"})
	}

	if !hexColorRegex.MatchString(b.BackgroundColor) {
		errs = append(errs, FieldError{Field: "background_color", Message: "Format background_color harus hex 6 karakter (misal: #FFFFFF)"})
	}

	if b.Domain != nil && *b.Domain != "" {
		if *b.Domain != strings.ToLower(*b.Domain) {
			errs = append(errs, FieldError{Field: "domain", Message: "Domain harus menggunakan huruf kecil"})
		}
	}

	validPortals := map[string]bool{"parent": true, "student": true, "teacher": true}
	for _, p := range b.IncludedPortals {
		if !validPortals[p] {
			errs = append(errs, FieldError{Field: "included_portals", Message: fmt.Sprintf("Portal '%s' tidak valid", p)})
		}
	}

	if isPublish {
		if b.PrivacyPolicyURL == nil || strings.TrimSpace(*b.PrivacyPolicyURL) == "" {
			errs = append(errs, FieldError{Field: "privacy_policy_url", Message: "Kebijakan privasi wajib sebelum diterbitkan"})
		} else {
			if _, err := url.ParseRequestURI(*b.PrivacyPolicyURL); err != nil {
				errs = append(errs, FieldError{Field: "privacy_policy_url", Message: "URL kebijakan privasi tidak valid"})
			}
		}

		if b.Icon192 == nil || !strings.HasPrefix(*b.Icon192, "data:image/png") {
			errs = append(errs, FieldError{Field: "icon_192", Message: "Ikon 192x192 PNG wajib ada sebelum diterbitkan"})
		}
		if b.Icon512 == nil || !strings.HasPrefix(*b.Icon512, "data:image/png") {
			errs = append(errs, FieldError{Field: "icon_512", Message: "Ikon 512x512 PNG wajib ada sebelum diterbitkan"})
		}
	}

	return errs
}

// ContrastRatio calculates WCAG 2.x contrast ratio between two hex colors.
func ContrastRatio(hex1, hex2 string) float64 {
	l1 := relativeLuminance(hex1)
	l2 := relativeLuminance(hex2)
	lighter := math.Max(l1, l2)
	darker := math.Min(l1, l2)
	ratio := (lighter + 0.05) / (darker + 0.05)
	return math.Round(ratio*100) / 100
}

func parseHexRGB(hex string) (float64, float64, float64) {
	clean := strings.TrimPrefix(hex, "#")
	if len(clean) != 6 {
		return 0, 0, 0
	}
	r, _ := strconv.ParseInt(clean[0:2], 16, 64)
	g, _ := strconv.ParseInt(clean[2:4], 16, 64)
	b, _ := strconv.ParseInt(clean[4:6], 16, 64)
	return float64(r), float64(g), float64(b)
}

func sRGBtoLinear(c float64) float64 {
	cNorm := c / 255.0
	if cNorm <= 0.04045 {
		return cNorm / 12.92
	}
	return math.Pow((cNorm+0.055)/1.055, 2.4)
}

func relativeLuminance(hex string) float64 {
	r, g, b := parseHexRGB(hex)
	rLin := sRGBtoLinear(r)
	gLin := sRGBtoLinear(g)
	bLin := sRGBtoLinear(b)
	return 0.2126*rLin + 0.7152*gLin + 0.0722*bLin
}

func NormalizeHost(hostHeader string) string {
	h := strings.TrimSpace(hostHeader)
	// Remove port if present
	if colon := strings.Index(h, ":"); colon != -1 {
		h = h[:colon]
	}
	return strings.ToLower(h)
}

func BuildManifest(b BrandingDraft, version int, baseURL string) Manifest {
	cleanBase := strings.TrimRight(baseURL, "/")
	if cleanBase == "" {
		cleanBase = "/"
	}

	manifest := Manifest{
		Name:            b.AppName,
		ShortName:       b.ShortName,
		ThemeColor:      b.ThemeColor,
		BackgroundColor: b.BackgroundColor,
		Display:         "standalone",
		StartURL:        cleanBase,
		Scope:           cleanBase,
		Icons: []ManifestIcon{
			{
				Src:   fmt.Sprintf("%s/public/branding/%s/icon_192.png?v=%d", cleanBase, b.SchoolID, version),
				Sizes: "192x192",
				Type:  "image/png",
			},
			{
				Src:   fmt.Sprintf("%s/public/branding/%s/icon_512.png?v=%d", cleanBase, b.SchoolID, version),
				Sizes: "512x512",
				Type:  "image/png",
			},
		},
	}

	if b.IconMaskable != nil && *b.IconMaskable != "" {
		manifest.Icons = append(manifest.Icons, ManifestIcon{
			Src:     fmt.Sprintf("%s/public/branding/%s/icon_maskable.png?v=%d", cleanBase, b.SchoolID, version),
			Sizes:   "512x512",
			Type:    "image/png",
			Purpose: "maskable",
		})
	}

	return manifest
}
