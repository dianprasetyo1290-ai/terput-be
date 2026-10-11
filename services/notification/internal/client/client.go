// Package client is a thin, best-effort internal HTTP client for the sibling
// identity-service. notification-service never owns users or their contact
// details — it references identity user ids and, when an email/whatsapp
// delivery is requested, looks the address up over internal HTTP with the
// shared X-Internal-Key header. A lookup failure is not fatal: the delivery is
// recorded as failed with the reason.
package client

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// Client talks to one downstream base URL.
type Client struct {
	BaseURL     string
	InternalKey string
	HTTP        *http.Client
}

// New builds a client. url may be empty, in which case Configured() is false
// and every call fails cleanly.
func New(url, key string) *Client {
	return &Client{
		BaseURL:     strings.TrimRight(url, "/"),
		InternalKey: key,
		HTTP:        &http.Client{Timeout: 5 * time.Second},
	}
}

// Configured reports whether the client has a usable base URL.
func (c *Client) Configured() bool { return c != nil && c.BaseURL != "" }

// UserProfile mirrors the contact fields notification-service needs from
// identity-service.
type UserProfile struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Email string `json:"email"`
	Phone string `json:"phone"`
}

// GetUser fetches a user's contact details for an external delivery.
func (c *Client) GetUser(ctx context.Context, id string) (*UserProfile, error) {
	if !c.Configured() {
		return nil, fmt.Errorf("identity-service is not configured")
	}
	if id == "" {
		return nil, fmt.Errorf("user id is required")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.BaseURL+"/users/"+id, nil)
	if err != nil {
		return nil, err
	}
	if c.InternalKey != "" {
		req.Header.Set("X-Internal-Key", c.InternalKey)
	}
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("identity-service returned %d for user %s", resp.StatusCode, id)
	}
	var env struct {
		Data json.RawMessage `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&env); err != nil {
		return nil, err
	}
	if len(env.Data) == 0 {
		return nil, fmt.Errorf("identity-service returned an empty envelope")
	}
	var u UserProfile
	if err := json.Unmarshal(env.Data, &u); err != nil {
		return nil, err
	}
	return &u, nil
}
