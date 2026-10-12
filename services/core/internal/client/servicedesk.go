package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

// Service Desk (T11) calls to identity and ops.

// SchoolFeatures returns feature_key → enabled for a school
// (identity GET /schools/{id}/features, internal key accepted).
func (c *Client) SchoolFeatures(ctx context.Context, schoolID string) (map[string]bool, error) {
	if c.IdentityURL == "" {
		return nil, ErrNotConfigured
	}
	var statuses []struct {
		FeatureKey string `json:"feature_key"`
		Enabled    bool   `json:"enabled"`
	}
	if err := c.getJSON(ctx, c.IdentityURL+"/schools/"+url.PathEscape(schoolID)+"/features", &statuses); err != nil {
		return nil, err
	}
	out := make(map[string]bool, len(statuses))
	for _, s := range statuses {
		out[s.FeatureKey] = s.Enabled
	}
	return out, nil
}

// UserRef is a user id with its display name.
type UserRef struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// UsersInSchool lists users by role or by id (identity GET /users).
func (c *Client) UsersInSchool(ctx context.Context, schoolID, role string, ids []string) ([]UserRef, error) {
	if c.IdentityURL == "" {
		return nil, ErrNotConfigured
	}
	q := url.Values{"school_id": {schoolID}, "per_page": {"200"}}
	if role != "" {
		q.Set("role", role)
		q.Set("status", "active")
	}
	if len(ids) > 0 {
		q.Set("ids", strings.Join(ids, ","))
	}
	var users []UserRef
	if err := c.getJSON(ctx, c.IdentityURL+"/users?"+q.Encode(), &users); err != nil {
		return nil, err
	}
	return users, nil
}

// WorkOrderInput is the T9 §4.2 body of POST /internal/facility/work-orders.
type WorkOrderInput struct {
	SchoolID       string  `json:"school_id"`
	RoomID         *string `json:"room_id,omitempty"`
	AssetID        *string `json:"asset_id,omitempty"`
	Title          string  `json:"title"`
	Description    string  `json:"description"`
	Priority       string  `json:"priority"`
	ReporterUserID string  `json:"reporter_user_id"`
	TicketRef      string  `json:"ticket_ref"`
	IdempotencyKey string  `json:"idempotency_key"`
}

// WorkOrder is the ops reply / list row (T9 §4.2).
type WorkOrder struct {
	ID           string `json:"id"`
	Code         string `json:"code"`
	Status       string `json:"status"`
	AssigneeName string `json:"assignee_name,omitempty"`
	UpdatedAt    string `json:"updated_at,omitempty"`
}

// CreateWorkOrder converts a ticket into a work order; the Idempotency-Key
// header and body key are identical so a replay returns the same order.
func (c *Client) CreateWorkOrder(ctx context.Context, in WorkOrderInput) (WorkOrder, error) {
	var out WorkOrder
	if c.OpsURL == "" {
		return out, ErrNotConfigured
	}
	body, err := json.Marshal(in)
	if err != nil {
		return out, err
	}
	full := c.OpsURL + "/internal/facility/work-orders"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, full, bytes.NewReader(body))
	if err != nil {
		return out, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("X-Internal-Key", c.InternalKey)
	req.Header.Set("Idempotency-Key", in.IdempotencyKey)
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return out, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
		return out, &HTTPError{Status: resp.StatusCode, Body: string(raw), URL: full}
	}
	env := struct {
		Data *WorkOrder `json:"data"`
	}{Data: &out}
	if err := json.NewDecoder(resp.Body).Decode(&env); err != nil {
		return out, fmt.Errorf("decode %s: %w", full, err)
	}
	if out.ID == "" {
		return out, fmt.Errorf("ops %s: response without id", full)
	}
	return out, nil
}

// WorkOrdersByTicket pulls the work orders created from a ticket.
func (c *Client) WorkOrdersByTicket(ctx context.Context, schoolID, ticketRef string) ([]WorkOrder, error) {
	if c.OpsURL == "" {
		return nil, ErrNotConfigured
	}
	q := url.Values{"school_id": {schoolID}, "ticket_ref": {ticketRef}}
	var out []WorkOrder
	err := c.getJSON(ctx, c.OpsURL+"/internal/facility/work-orders?"+q.Encode(), &out)
	return out, err
}

// Room is the subset of T9's room row the ticket form needs.
type Room struct {
	ID       string `json:"id"`
	Code     string `json:"code"`
	Name     string `json:"name"`
	Building *struct {
		Name string `json:"name"`
	} `json:"building,omitempty"`
}

// FacilityRooms lists the school's rooms (T9 GET /internal/facility/rooms).
func (c *Client) FacilityRooms(ctx context.Context, schoolID string) ([]Room, error) {
	if c.OpsURL == "" {
		return nil, ErrNotConfigured
	}
	var out []Room
	err := c.getJSON(ctx, c.OpsURL+"/internal/facility/rooms?school_id="+url.QueryEscape(schoolID), &out)
	return out, err
}
