// Package client holds internal service-to-service HTTP clients.
//
// Contract assumption (documented in README "Assumptions"): identity-service
// is being built in parallel by a sibling agent and its exact REST shape for
// academic_years was not observable at the time this client was written.
// Based on identity's own migration (services/identity/migrations/0001_init.up.sql)
// we know the underlying table is:
//
//	academic_years(id UUID, school_id UUID, label TEXT, is_current BOOLEAN, ...)
//
// This client assumes identity exposes `GET /academic-years?school_id=<id>`
// returning the envelope `{"data":[{"id":...,"school_id":...,"label":...,
// "is_current":...}, ...]}`. If that assumption is wrong, or identity is
// simply not reachable (e.g. during standalone academic-service dev), every
// caller of ResolveCurrentAcademicYear treats a failure as "unresolvable"
// and academic-service falls back to requiring an explicit
// `?academic_year_id=` query param (400 if missing) rather than guessing.
package client

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"time"
)

type IdentityClient struct {
	BaseURL     string
	InternalKey string
	HTTPClient  *http.Client
}

func NewIdentityClient() *IdentityClient {
	base := os.Getenv("IDENTITY_BASE_URL")
	if base == "" {
		base = "http://identity:8081"
	}
	key := os.Getenv("SEKOCI_INTERNAL_KEY")
	if key == "" {
		key = "change-me"
	}
	return &IdentityClient{
		BaseURL:     base,
		InternalKey: key,
		HTTPClient:  &http.Client{Timeout: 3 * time.Second},
	}
}

type academicYear struct {
	ID        string `json:"id"`
	SchoolID  string `json:"school_id"`
	Label     string `json:"label"`
	IsCurrent bool   `json:"is_current"`
}

type academicYearsEnvelope struct {
	Data []academicYear `json:"data"`
}

// ResolveCurrentAcademicYear returns the current academic_year_id for a
// school by asking identity-service, per the contract assumption documented
// above. Returns ("", err) if identity is unreachable, returns a
// non-2xx/unparseable response, or has no year flagged is_current for this
// school — callers should treat any error as "caller must pass
// ?academic_year_id= explicitly".
func (c *IdentityClient) ResolveCurrentAcademicYear(schoolID string) (string, error) {
	if schoolID == "" {
		return "", fmt.Errorf("empty school_id")
	}
	url := fmt.Sprintf("%s/academic-years?school_id=%s", c.BaseURL, schoolID)
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("X-Internal-Key", c.InternalKey)
	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("identity-service returned status %d", resp.StatusCode)
	}
	var env academicYearsEnvelope
	if err := json.NewDecoder(resp.Body).Decode(&env); err != nil {
		return "", err
	}
	for _, ay := range env.Data {
		if ay.IsCurrent {
			return ay.ID, nil
		}
	}
	return "", fmt.Errorf("no current academic year found for school %s", schoolID)
}

type User struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Email string `json:"email"`
}

type userEnvelope struct {
	Data User `json:"data"`
}

// GetUser fetches a user's basic profile (e.g. to display a guru/siswa's
// name) from identity-service. Not required for the core academic-service
// flows below (we store bare user_id UUIDs and let the frontend/other
// callers resolve names), but provided for handlers that want to enrich a
// response. Same failure contract as ResolveCurrentAcademicYear: caller
// decides how to degrade (e.g. omit the name field) on error.
func (c *IdentityClient) GetUser(userID string) (*User, error) {
	url := fmt.Sprintf("%s/users/%s", c.BaseURL, userID)
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("X-Internal-Key", c.InternalKey)
	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("identity-service returned status %d", resp.StatusCode)
	}
	var env userEnvelope
	if err := json.NewDecoder(resp.Body).Decode(&env); err != nil {
		return nil, err
	}
	return &env.Data, nil
}

// CreateOrLinkStudent links or creates a student account in the destination school.
func (c *IdentityClient) CreateOrLinkStudent(schoolID, studentUserID, name, email string) error {
	url := fmt.Sprintf("%s/internal/students/create-or-link", c.BaseURL)
	payload := map[string]any{
		"school_id":     schoolID,
		"source":        "academic_transfer",
		"external_ref":  studentUserID,
		"name":          name,
		"email":         email,
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("X-Internal-Key", c.InternalKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("identity create-or-link returned status %d", resp.StatusCode)
	}
	return nil
}

// DeactivateUser calls identity POST /internal/users/{id}/deactivate
func (c *IdentityClient) DeactivateUser(schoolID, userID, reason string) error {
	url := fmt.Sprintf("%s/internal/users/%s/deactivate", c.BaseURL, userID)
	payload := map[string]any{
		"school_id": schoolID,
		"reason":    reason,
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("X-Internal-Key", c.InternalKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("identity deactivate returned status %d", resp.StatusCode)
	}
	return nil
}


// ErrAcademicYearNotFound: identity answered 404 for the year.
var ErrAcademicYearNotFound = fmt.Errorf("academic year not found")

// AcademicYear is the identity view of one year (QA-IDN, identity
// migration 0020): date range (YYYY-MM-DD, may be null on legacy rows) and
// the period lock.
type AcademicYear struct {
	ID        string     `json:"id"`
	SchoolID  string     `json:"school_id"`
	Label     string     `json:"label"`
	IsCurrent bool       `json:"is_current"`
	StartDate *string    `json:"start_date"`
	EndDate   *string    `json:"end_date"`
	LockedAt  *time.Time `json:"locked_at"`
}

type academicYearEnvelope struct {
	Data AcademicYear `json:"data"`
}

// GetAcademicYear fetches GET /academic-years/{id} with the internal key.
func (c *IdentityClient) GetAcademicYear(id string) (*AcademicYear, error) {
	req, err := http.NewRequest(http.MethodGet, fmt.Sprintf("%s/academic-years/%s", c.BaseURL, id), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("X-Internal-Key", c.InternalKey)
	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return nil, ErrAcademicYearNotFound
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("identity-service returned status %d", resp.StatusCode)
	}
	var env academicYearEnvelope
	if err := json.NewDecoder(resp.Body).Decode(&env); err != nil {
		return nil, err
	}
	return &env.Data, nil
}
