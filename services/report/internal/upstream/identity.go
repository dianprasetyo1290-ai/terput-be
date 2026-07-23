package upstream

import (
	"context"
	"fmt"
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
