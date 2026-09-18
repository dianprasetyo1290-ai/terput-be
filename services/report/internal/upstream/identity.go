package upstream

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// IdentityClient talks to identity-service (port 8081), which owns
// users/schools. See README > Expected upstream contracts for the exact
// endpoint shape assumed here (identity-service is being built in
// parallel and had no routes yet at the time this was written).
type IdentityClient struct{ c *client }

func NewIdentityClient(baseURL, internalKey string, timeout time.Duration) *IdentityClient {
	return &IdentityClient{c: newClient(baseURL, internalKey, "identity", timeout)}
}

// User is the subset of identity-service's user record report-service
// needs for a report-card header.
type User struct {
	ID       string  `json:"id"`
	Name     string  `json:"name"`
	NISN     *string `json:"nisn"`
	SchoolID string  `json:"school_id"`
}

// GetUser fetches GET /users/{id}. Returns (nil, nil) if the upstream
// responds 404 (user genuinely not found there); returns a non-nil error
// wrapping ErrUnavailable if identity-service could not be reached at all.
func (ic *IdentityClient) GetUser(ctx context.Context, userID string) (*User, error) {
	var u User
	if err := ic.c.getJSON(ctx, "GetUser", fmt.Sprintf("/users/%s", userID), &u); err != nil {
		return nil, err
	}
	if u.ID == "" {
		return nil, nil
	}
	return &u, nil
}

// OrgPosition mirrors one row of identity-service's GET /organization
// response (its model.PositionAssignment) -- the "Organisasi Sekolah" list
// (Jabatan | Nama) admin already maintains via Data Jabatan/Peran. Kepala
// Sekolah is just the row whose PositionName is "Kepala Sekolah", already
// joined to the holder's name server-side (no extra GetUser round trip
// needed).
type OrgPosition struct {
	PositionName string `json:"position_name"`
	UserID       string `json:"user_id"`
	UserName     string `json:"user_name"`
}

// GetOrganization fetches GET /organization?school_id=, the school's full
// jabatan-holder list. Report-service scans it for "Kepala Sekolah" to
// auto-fill the report card's principal signature.
func (ic *IdentityClient) GetOrganization(ctx context.Context, schoolID string) ([]OrgPosition, error) {
	var out []OrgPosition
	if err := ic.c.getJSON(ctx, "GetOrganization", fmt.Sprintf("/organization?school_id=%s", schoolID), &out); err != nil {
		return nil, err
	}
	return out, nil
}

// PrincipalName scans a GetOrganization result for the "Kepala Sekolah"
// jabatan holder (case-insensitive, trimmed -- matches how admin free-types
// position names in Data Jabatan/Peran). Returns "" if no such position is
// currently assigned.
func PrincipalName(positions []OrgPosition) string {
	for _, p := range positions {
		if strings.EqualFold(strings.TrimSpace(p.PositionName), "Kepala Sekolah") {
			return p.UserName
		}
	}
	return ""
}

// School mirrors the subset of identity-service's School record (see
// services/identity/internal/model/model.go's School struct) the printed
// rapor's letterhead/signature-date line needs. Every field here is what
// makes the print output per-school instead of hardcoded to whichever
// school's reference template this feature was built against -- see the
// task's multi-tenancy requirement.
type School struct {
	ID       string  `json:"id"`
	Name     string  `json:"name"`
	Address  *string `json:"address"`
	City     *string `json:"city"`
	Province *string `json:"province"`
	Logo     *string `json:"logo"`
	// GovernmentLogo: SEKOCI V10 item C -- "Logo Pemerintah", printed at the
	// rapor letterhead's top-left with the school's own logo at top-right.
	GovernmentLogo *string `json:"government_logo"`
}

// GetSchool fetches GET /schools/{id}, already reachable via X-Internal-Key
// per identity-service's ir.Get("/schools/{id}", ...) route (see
// cmd/server/main.go there). Returns (nil, nil) on a genuine 404-shaped
// empty response; a non-nil error wraps ErrUnavailable on network failure.
func (ic *IdentityClient) GetSchool(ctx context.Context, schoolID string) (*School, error) {
	var s School
	if err := ic.c.getJSON(ctx, "GetSchool", fmt.Sprintf("/schools/%s", schoolID), &s); err != nil {
		return nil, err
	}
	if s.ID == "" {
		return nil, nil
	}
	return &s, nil
}
