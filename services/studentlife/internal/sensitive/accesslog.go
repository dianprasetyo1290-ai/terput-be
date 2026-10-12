package sensitive

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"skci/studentlife/internal/common"
	"skci/studentlife/internal/store"
)

// Actions recorded in sensitive_access_logs (CHECK di migrasi 0001).
const (
	ActionView   = "view"
	ActionCreate = "create"
	ActionUpdate = "update"
	ActionPrint  = "print"
	ActionDeny   = "deny"
)

// GrantableRoles are the roles an admin may grant a confidential access
// policy to (OQ-31). Parents/students are never grantable.
var GrantableRoles = []string{common.RoleKepalaSekolah, common.RoleAdmin, common.RoleGuru, common.RoleWalikelas}

// ErrPolicyExists is returned when the role already has an active policy.
var ErrPolicyExists = errors.New("policy already active")

// AccessEntry is one sensitive_access_logs row (never carries record content).
type AccessEntry struct {
	SchoolID    string
	Module      string
	EntityType  string
	EntityID    string // "" = list/aggregate access
	StudentID   string // "" = not student-specific
	ActorUserID string
	ActorRole   string
	Action      string
	Reason      string
	IPHash      string
}

func nullable(s string) any {
	if s == "" {
		return nil
	}
	return s
}

// LogAccess writes one access log row. Callers must fail closed: when this
// returns an error the sensitive data must not be sent.
func LogAccess(ctx context.Context, q store.Q, e AccessEntry) error {
	_, err := q.ExecContext(ctx, `INSERT INTO sensitive_access_logs
		(school_id, module, entity_type, entity_id, student_id, actor_user_id, actor_role, action, reason, ip_hash)
		VALUES ($1,$2,$3,$4::uuid,$5::uuid,$6,$7,$8,$9,$10)`,
		e.SchoolID, e.Module, e.EntityType, nullable(e.EntityID), nullable(e.StudentID),
		e.ActorUserID, e.ActorRole, e.Action, nullable(e.Reason), nullable(e.IPHash))
	return err
}

// ActivePolicyRoles returns the roles holding an active policy for module.
func ActivePolicyRoles(ctx context.Context, q store.Q, schoolID, module string) ([]string, error) {
	rows, err := q.QueryContext(ctx, `SELECT role FROM confidential_access_policies
		WHERE school_id = $1 AND module = $2 AND revoked_at IS NULL`, schoolID, module)
	return store.Collect(rows, err, func(r *sql.Rows) (string, error) {
		var s string
		return s, r.Scan(&s)
	})
}

// Policy is one confidential_access_policies row.
type Policy struct {
	ID        string     `json:"id"`
	Module    string     `json:"module"`
	Role      string     `json:"role"`
	GrantedBy string     `json:"granted_by"`
	GrantedAt time.Time  `json:"granted_at"`
	RevokedBy *string    `json:"revoked_by"`
	RevokedAt *time.Time `json:"revoked_at"`
}

const policyCols = `id, module, role, granted_by, granted_at, revoked_by, revoked_at`

func scanPolicy(r interface{ Scan(...any) error }) (Policy, error) {
	var p Policy
	err := r.Scan(&p.ID, &p.Module, &p.Role, &p.GrantedBy, &p.GrantedAt, &p.RevokedBy, &p.RevokedAt)
	return p, err
}

// ListPolicies returns the module's policies, active first.
func ListPolicies(ctx context.Context, q store.Q, schoolID, module string) ([]Policy, error) {
	rows, err := q.QueryContext(ctx, `SELECT `+policyCols+` FROM confidential_access_policies
		WHERE school_id = $1 AND module = $2 ORDER BY (revoked_at IS NULL) DESC, granted_at DESC`, schoolID, module)
	return store.Collect(rows, err, func(r *sql.Rows) (Policy, error) { return scanPolicy(r) })
}

// GrantPolicy inserts an active policy; ErrPolicyExists when one is active.
func GrantPolicy(ctx context.Context, q store.Q, schoolID, module, role, actor string) (Policy, error) {
	p, err := scanPolicy(q.QueryRowContext(ctx, `INSERT INTO confidential_access_policies (school_id, module, role, granted_by)
		VALUES ($1,$2,$3,$4) ON CONFLICT (school_id, module, role) WHERE revoked_at IS NULL DO NOTHING
		RETURNING `+policyCols, schoolID, module, role, actor))
	if errors.Is(err, sql.ErrNoRows) {
		return Policy{}, ErrPolicyExists
	}
	return p, err
}

// RevokePolicy stamps revoked_* on an active policy (riwayat tetap tersimpan).
func RevokePolicy(ctx context.Context, q store.Q, schoolID, id, actor string) (Policy, error) {
	p, err := scanPolicy(q.QueryRowContext(ctx, `UPDATE confidential_access_policies
		SET revoked_by = $3, revoked_at = now(), updated_at = now()
		WHERE school_id = $1 AND id = $2 AND revoked_at IS NULL RETURNING `+policyCols, schoolID, id, actor))
	return p, store.One(err)
}

// LogFilter narrows ListLogs.
type LogFilter struct {
	SchoolID, Module, StudentID, ActorUserID string
	From, To                                 string // YYYY-MM-DD, inclusive
}

// AccessLog is one row as shown to admin/auditor.
type AccessLog struct {
	ID          int64     `json:"id"`
	Module      string    `json:"module"`
	EntityType  string    `json:"entity_type"`
	EntityID    *string   `json:"entity_id"`
	StudentID   *string   `json:"student_id"`
	ActorUserID string    `json:"actor_user_id"`
	ActorRole   string    `json:"actor_role"`
	Action      string    `json:"action"`
	Reason      *string   `json:"reason"`
	CreatedAt   time.Time `json:"created_at"`
	StudentName string    `json:"student_name,omitempty"`
	ActorName   string    `json:"actor_name,omitempty"`
}

// ListLogs pages the access log, newest first.
func ListLogs(ctx context.Context, q store.Q, f LogFilter, page, perPage int) ([]AccessLog, int, error) {
	where := []string{"school_id = $1"}
	args := []any{f.SchoolID}
	add := func(cond, v string) {
		if v == "" {
			return
		}
		args = append(args, v)
		where = append(where, fmt.Sprintf(cond, len(args)))
	}
	add("module = $%d", f.Module)
	add("student_id = $%d::uuid", f.StudentID)
	add("actor_user_id = $%d::uuid", f.ActorUserID)
	add("created_at >= $%d::date", f.From)
	add("created_at < ($%d::date + 1)", f.To)
	w := strings.Join(where, " AND ")
	var total int
	if err := q.QueryRowContext(ctx, `SELECT count(*) FROM sensitive_access_logs WHERE `+w, args...).Scan(&total); err != nil {
		return nil, 0, err
	}
	args = append(args, perPage, common.Offset(page, perPage))
	rows, err := q.QueryContext(ctx, fmt.Sprintf(`SELECT id, module, entity_type, entity_id::text, student_id::text,
		actor_user_id::text, actor_role, action, reason, created_at FROM sensitive_access_logs WHERE %s
		ORDER BY created_at DESC, id DESC LIMIT $%d OFFSET $%d`, w, len(args)-1, len(args)), args...)
	list, err := store.Collect(rows, err, func(r *sql.Rows) (AccessLog, error) {
		var l AccessLog
		err := r.Scan(&l.ID, &l.Module, &l.EntityType, &l.EntityID, &l.StudentID, &l.ActorUserID, &l.ActorRole, &l.Action, &l.Reason, &l.CreatedAt)
		return l, err
	})
	return list, total, err
}
