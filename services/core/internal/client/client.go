package client

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

var (
	ErrNotConfigured = errors.New("service is not configured")
	ErrNotFound      = errors.New("resource not found downstream")
)

type HTTPError struct {
	Status int
	Body   string
	URL    string
}

func (e *HTTPError) Error() string {
	if e.Body == "" {
		return fmt.Sprintf("downstream %s returned %d", e.URL, e.Status)
	}
	return fmt.Sprintf("downstream %s returned %d: %s", e.URL, e.Status, e.Body)
}

type Client struct {
	NotificationURL string
	IdentityURL     string
	HRURL           string
	InternalKey     string
	HTTP            *http.Client
}

func New(notifURL, identityURL, hrURL, internalKey string) *Client {
	return &Client{
		NotificationURL: strings.TrimRight(notifURL, "/"),
		IdentityURL:     strings.TrimRight(identityURL, "/"),
		HRURL:           strings.TrimRight(hrURL, "/"),
		InternalKey:     internalKey,
		HTTP:            &http.Client{Timeout: 8 * time.Second},
	}
}

// SendCallback calls the source service's registered callback URL.
func (c *Client) SendCallback(ctx context.Context, callbackURL, idempotencyKey string, payload any) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal callback payload: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, callbackURL, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Internal-Key", c.InternalKey)
	req.Header.Set("Idempotency-Key", idempotencyKey)

	resp, err := c.HTTP.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
		return &HTTPError{Status: resp.StatusCode, Body: string(raw), URL: callbackURL}
	}
	return nil
}

// SendNotification dispatches a notification to the notification service.
func (c *Client) SendNotification(ctx context.Context, idempotencyKey string, payload any) error {
	if c.NotificationURL == "" {
		return ErrNotConfigured
	}
	fullURL := c.NotificationURL + "/internal/notifications"
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal notification: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, fullURL, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Internal-Key", c.InternalKey)
	req.Header.Set("Idempotency-Key", idempotencyKey)

	resp, err := c.HTTP.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
		return &HTTPError{Status: resp.StatusCode, Body: string(raw), URL: fullURL}
	}
	return nil
}

// GetSupervisor resolves the direct supervisor's user id via HR (T2 contract:
// GET /internal/employees/by-user/{userId}/supervisor?school_id= -> {user_id}).
// 404 (no employee / supervisor without an account) maps to ErrNotFound.
func (c *Client) GetSupervisor(ctx context.Context, schoolID, subjectUserID string) (string, error) {
	if c.HRURL == "" {
		return "", ErrNotConfigured
	}
	var out struct {
		UserID string `json:"user_id"`
	}
	full := fmt.Sprintf("%s/internal/employees/by-user/%s/supervisor?school_id=%s",
		c.HRURL, url.PathEscape(subjectUserID), url.QueryEscape(schoolID))
	if err := c.getJSON(ctx, full, &out); err != nil {
		return "", err
	}
	if out.UserID == "" {
		return "", ErrNotFound
	}
	return out.UserID, nil
}

// CallRetry triggers a retry on an exception's retry_url.
func (c *Client) CallRetry(ctx context.Context, retryURL string) error {
	if retryURL == "" {
		return errors.New("empty retry url")
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, retryURL, nil)
	if err != nil {
		return err
	}
	req.Header.Set("X-Internal-Key", c.InternalKey)
	req.Header.Set("Accept", "application/json")

	resp, err := c.HTTP.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
		return &HTTPError{Status: resp.StatusCode, Body: string(raw), URL: retryURL}
	}
	return nil
}

// getJSON performs an internal GET and decodes the envelope's data field.
func (c *Client) getJSON(ctx context.Context, fullURL string, dst any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fullURL, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("X-Internal-Key", c.InternalKey)
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return ErrNotFound
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
		return &HTTPError{Status: resp.StatusCode, Body: string(raw), URL: fullURL}
	}
	env := struct {
		Data any `json:"data"`
	}{Data: dst}
	if err := json.NewDecoder(resp.Body).Decode(&env); err != nil {
		return fmt.Errorf("decode %s: %w", fullURL, err)
	}
	return nil
}

// UserRoles returns the user's active roles in one school (identity GET /users/{id}).
func (c *Client) UserRoles(ctx context.Context, schoolID, userID string) ([]string, error) {
	if c.IdentityURL == "" {
		return nil, ErrNotConfigured
	}
	var u struct {
		Roles []struct {
			SchoolID *string `json:"school_id"`
			Role     string  `json:"role"`
			Status   string  `json:"status"`
		} `json:"roles"`
	}
	if err := c.getJSON(ctx, c.IdentityURL+"/users/"+url.PathEscape(userID), &u); err != nil {
		return nil, err
	}
	var roles []string
	for _, r := range u.Roles {
		if r.SchoolID != nil && *r.SchoolID == schoolID && r.Status != "inactive" {
			roles = append(roles, r.Role)
		}
	}
	return roles, nil
}

// UsersByRole lists the ids of users holding a role in a school (identity GET /users).
// ponytail: first 200 holders only; page through if a school ever has more approvers of one role.
func (c *Client) UsersByRole(ctx context.Context, schoolID, role string) ([]string, error) {
	if c.IdentityURL == "" {
		return nil, ErrNotConfigured
	}
	q := url.Values{"role": {role}, "school_id": {schoolID}, "status": {"active"}, "per_page": {"200"}}
	var users []struct {
		ID string `json:"id"`
	}
	if err := c.getJSON(ctx, c.IdentityURL+"/users?"+q.Encode(), &users); err != nil {
		return nil, err
	}
	ids := make([]string, 0, len(users))
	for _, u := range users {
		ids = append(ids, u.ID)
	}
	return ids, nil
}

// Notice is one in-app notification fanned out to several users.
type Notice struct {
	SchoolID string
	KeyBase  string // idempotency key prefix, e.g. approval:<id>:<seq>:<status>
	Type     string
	Title    string
	Body     string
	Data     map[string]any
}

// NotifyUsers sends the notice to each user with key "<KeyBase>:<user_id>"
// (the notification service dedupes per school+key, so the suffix keeps one
// row per recipient). Returns the first error; delivery continues past it.
func (c *Client) NotifyUsers(ctx context.Context, n Notice, userIDs []string) error {
	var first error
	seen := map[string]bool{}
	for _, uid := range userIDs {
		if uid == "" || seen[uid] {
			continue
		}
		seen[uid] = true
		err := c.SendNotification(ctx, n.KeyBase+":"+uid, map[string]any{
			"school_id": n.SchoolID,
			"user_id":   uid,
			"type":      n.Type,
			"title":     n.Title,
			"body":      n.Body,
			"data":      n.Data,
		})
		if err != nil && first == nil {
			first = err
		}
	}
	return first
}
