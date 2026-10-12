package client

import (
	"context"
	"net/url"
)

// RoleAccount is identity's reply to POST /internal/role-accounts/create-or-link.
// DefaultPassword is only present when the account was just created.
type RoleAccount struct {
	UserID          string `json:"user_id"`
	Created         bool   `json:"created"`
	DefaultPassword string `json:"default_password,omitempty"`
}

// CreateOrLinkRoleAccount creates (or links) an external role account such as
// canteen_operator in a school (T12 §4.2).
func (c *Client) CreateOrLinkRoleAccount(ctx context.Context, schoolID, email, fullName, role string) (RoleAccount, error) {
	var out RoleAccount
	_, err := c.PostJSON(ctx, "/internal/role-accounts/create-or-link", "", map[string]string{
		"school_id": schoolID, "email": email, "full_name": fullName, "role": role,
	}, &out)
	return out, err
}

// StudentGuardianIDs lists the active guardians of a student (T9 contract
// GET /internal/students/{id}/guardians).
func (c *Client) StudentGuardianIDs(ctx context.Context, schoolID, studentUserID string) ([]string, error) {
	var rows []struct {
		GuardianUserID string `json:"guardian_user_id"`
	}
	if err := c.Get(ctx, "/internal/students/"+url.PathEscape(studentUserID)+"/guardians?school_id="+url.QueryEscape(schoolID), &rows); err != nil {
		return nil, err
	}
	out := make([]string, 0, len(rows))
	for _, r := range rows {
		out = append(out, r.GuardianUserID)
	}
	return out, nil
}

// GuardianChildIDs lists the students a guardian is actively linked to.
func (c *Client) GuardianChildIDs(ctx context.Context, schoolID, guardianUserID string) ([]string, error) {
	var rows []struct {
		StudentUserID string `json:"student_user_id"`
	}
	if err := c.Get(ctx, "/internal/guardians/"+url.PathEscape(guardianUserID)+"/children?school_id="+url.QueryEscape(schoolID), &rows); err != nil {
		return nil, err
	}
	out := make([]string, 0, len(rows))
	for _, r := range rows {
		out = append(out, r.StudentUserID)
	}
	return out, nil
}
