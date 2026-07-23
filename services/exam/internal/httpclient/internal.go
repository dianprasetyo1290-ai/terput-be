// Package httpclient makes best-effort internal HTTP calls to sibling
// services (identity, academic). Every call here is defensive: on any error
// (service down, route not yet implemented by the sibling agent building it
// in parallel, timeout) callers get a zero-value fallback rather than a hard
// failure, since exam-service's own state machine never depends on these
// calls succeeding. See README Assumptions.
package httpclient

import (
	"encoding/json"
	"net/http"
	"time"
)

type Client struct {
	IdentityURL string
	AcademicURL string
	InternalKey string
	http        *http.Client
}

func New(identityURL, academicURL, internalKey string) *Client {
	return &Client{
		IdentityURL: identityURL,
		AcademicURL: academicURL,
		InternalKey: internalKey,
		http:        &http.Client{Timeout: 2 * time.Second},
	}
}

type envelope struct {
	Data json.RawMessage `json:"data"`
}

type userDTO struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// UserName does a best-effort lookup of a student/guru's display name from
// identity-service. Returns "" (never an error) if unreachable.
func (c *Client) UserName(userID string) string {
	req, err := http.NewRequest(http.MethodGet, c.IdentityURL+"/internal/users/"+userID, nil)
	if err != nil {
		return ""
	}
	req.Header.Set("X-Internal-Key", c.InternalKey)
	resp, err := c.http.Do(req)
	if err != nil {
		return ""
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return ""
	}
	var env envelope
	if err := json.NewDecoder(resp.Body).Decode(&env); err != nil {
		return ""
	}
	var u userDTO
	if err := json.Unmarshal(env.Data, &u); err != nil {
		return ""
	}
	return u.Name
}
