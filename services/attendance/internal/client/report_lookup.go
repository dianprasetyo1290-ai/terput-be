package client

// Batch name lookups for the Attendance Report (PRD menu 43). Best-effort
// like the rest of this package: a failed lookup leaves names empty and the
// caller falls back to "-" rather than failing the report.

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
)

// StudentIdentity is the slice of identity-service's user record the report
// prints (name + NISN).
type StudentIdentity struct {
	ID   string  `json:"id"`
	Name string  `json:"name"`
	NISN *string `json:"nisn"`
}

func (c *Client) postJSON(ctx context.Context, u string, body, out interface{}) error {
	payload, err := json.Marshal(body)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, u, bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Internal-Key", c.internalKey)
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("upstream %s returned %d", u, resp.StatusCode)
	}
	var env envelope
	if err := json.NewDecoder(resp.Body).Decode(&env); err != nil {
		return err
	}
	return json.Unmarshal(env.Data, out)
}

// LookupUsers resolves many user ids in batches of 200 through
// identity-service's POST /users/search (ids in the body, so a whole school's
// roster never hits the URL-length limit). Missing ids are simply absent.
func (c *Client) LookupUsers(ctx context.Context, ids []string) map[string]StudentIdentity {
	out := map[string]StudentIdentity{}
	if c == nil || c.identityBaseURL == "" || len(ids) == 0 {
		return out
	}
	const batch = 200
	for i := 0; i < len(ids); i += batch {
		end := i + batch
		if end > len(ids) {
			end = len(ids)
		}
		var users []StudentIdentity
		u := fmt.Sprintf("%s/users/search?per_page=%d&page=1", c.identityBaseURL, batch)
		if err := c.postJSON(ctx, u, map[string]string{"ids": strings.Join(ids[i:end], ",")}, &users); err != nil {
			continue
		}
		for _, us := range users {
			out[us.ID] = us
		}
	}
	return out
}

// SchoolName resolves a school (unit) display name; "" on failure.
func (c *Client) SchoolName(ctx context.Context, id string) string {
	if c == nil || id == "" {
		return ""
	}
	var s struct {
		Name string `json:"name"`
	}
	if err := c.get(ctx, fmt.Sprintf("%s/schools/%s", c.identityBaseURL, url.PathEscape(id)), &s); err != nil {
		return ""
	}
	return s.Name
}

// ClassroomName / SubjectName / TermName: thin nil-safe wrappers so report
// code never has to repeat the error-swallowing.
func (c *Client) ClassroomName(ctx context.Context, id string) string {
	if c == nil || id == "" {
		return ""
	}
	if cr, err := c.GetClassroom(ctx, id); err == nil && cr != nil {
		return cr.Name
	}
	return ""
}

func (c *Client) SubjectName(ctx context.Context, id string) string {
	if c == nil || id == "" {
		return ""
	}
	if s, err := c.GetSubject(ctx, id); err == nil && s != nil {
		return s.Name
	}
	return ""
}

func (c *Client) TermName(ctx context.Context, id string) string {
	if c == nil || id == "" {
		return ""
	}
	if t, err := c.GetTerm(ctx, id); err == nil && t != nil {
		return t.Name
	}
	return ""
}
