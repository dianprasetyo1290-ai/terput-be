package store

import (
	"database/sql"
	"errors"
	"time"

	"skci/identity/internal/model"
)

var (
	ErrClaimInvalid   = errors.New("claim code is invalid or expired")
	ErrClaimUsed      = errors.New("claim code has already been used")
	ErrClaimRateLimit = errors.New("too many claim attempts")
)

func LinkedChildren(db *sql.DB, schoolID, guardianUserID string) ([]model.LinkedChild, error) {
	rows, err := db.Query(`SELECT gl.id, gl.school_id, gl.guardian_user_id, gl.student_user_id,
		gl.relationship, gl.is_primary, gl.status, gl.verified_at, gl.created_at, gl.updated_at,
		u.name, u.email, u.nisn, u.photo
		FROM guardian_links gl
		JOIN users u ON u.id=gl.student_user_id AND u.deleted_at IS NULL
		JOIN user_school_roles sr ON sr.user_id=u.id AND sr.school_id=gl.school_id AND sr.role='siswa' AND sr.status='active'
		WHERE gl.school_id=$1 AND gl.guardian_user_id=$2 AND gl.status='active'
		ORDER BY gl.is_primary DESC, u.name, u.id`, schoolID, guardianUserID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []model.LinkedChild{}
	for rows.Next() {
		var child model.LinkedChild
		if err := rows.Scan(&child.ID, &child.SchoolID, &child.GuardianUserID, &child.StudentUserID,
			&child.Relationship, &child.IsPrimary, &child.Status, &child.VerifiedAt, &child.CreatedAt, &child.UpdatedAt,
			&child.Name, &child.Email, &child.NISN, &child.Photo); err != nil {
			return nil, err
		}
		out = append(out, child)
	}
	return out, rows.Err()
}

// guardianActiveRoleSQL is the role-consistency predicate shared by the two
// internal guardian-read queries below. A guardian_link only counts while the
// guardian still holds an active orang_tua assignment at the very school the
// link belongs to — a link whose guardian lost/never had that role must not be
// readable by a sibling service that only vouches for the school scope.
// Written against guardian_links aliased `gl`.
const guardianActiveRoleSQL = `EXISTS (SELECT 1 FROM user_school_roles gr
		WHERE gr.user_id=gl.guardian_user_id AND gr.school_id=gl.school_id AND gr.role='orang_tua' AND gr.status='active')`

// LinkedGuardianChildren is the tenant-scoped read behind identity's internal
// GET /internal/guardians/{guardianUserId}/children endpoint. Unlike
// LinkedChildren (the browser /me/children read, which trusts the JWT-derived
// guardian id and never re-checks the guardian's own role) it is called with
// an untrusted-but-authenticated school scope, so it also requires the
// guardian to still hold an active orang_tua role at that school and the
// student an active siswa role there. Returns the minimal
// {student_user_id,name,nisn} projection; school_id is mandatory at the
// handler so a missing tenant scope never becomes a cross-school read.
func LinkedGuardianChildren(db *sql.DB, schoolID, guardianUserID string) ([]model.GuardianChild, error) {
	rows, err := db.Query(`SELECT gl.student_user_id, u.name, u.nisn
		FROM guardian_links gl
		JOIN users u ON u.id=gl.student_user_id AND u.deleted_at IS NULL
		JOIN user_school_roles sr ON sr.user_id=u.id AND sr.school_id=gl.school_id AND sr.role='siswa' AND sr.status='active'
		WHERE gl.school_id=$1 AND gl.guardian_user_id=$2 AND gl.status='active' AND `+guardianActiveRoleSQL+`
		ORDER BY gl.is_primary DESC, u.name, u.id`, schoolID, guardianUserID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []model.GuardianChild{}
	for rows.Next() {
		var child model.GuardianChild
		if err := rows.Scan(&child.StudentUserID, &child.Name, &child.NISN); err != nil {
			return nil, err
		}
		out = append(out, child)
	}
	return out, rows.Err()
}

// GuardianStudentAccess answers the internal
// GET /internal/guardians/{guardianUserId}/students/{studentUserId}/access
// endpoint: whether an active link connects this guardian to this student
// within one school, under the same role-consistency rules as
// LinkedGuardianChildren. Returning (false, nil) means "no such link" and is
// surfaced by the handler as 404; an error is a real lookup failure.
func GuardianStudentAccess(db *sql.DB, schoolID, guardianUserID, studentUserID string) (bool, error) {
	var allowed bool
	err := db.QueryRow(`SELECT EXISTS (
		SELECT 1 FROM guardian_links gl
		JOIN users u ON u.id=gl.student_user_id AND u.deleted_at IS NULL
		JOIN user_school_roles sr ON sr.user_id=u.id AND sr.school_id=gl.school_id AND sr.role='siswa' AND sr.status='active'
		WHERE gl.school_id=$1 AND gl.guardian_user_id=$2 AND gl.student_user_id=$3 AND gl.status='active' AND `+guardianActiveRoleSQL+`
	)`, schoolID, guardianUserID, studentUserID).Scan(&allowed)
	return allowed, err
}

// StudentGuardians backs GET /internal/students/{studentUserId}/guardians:
// the active links of one student at one school, under the same
// role-consistency rules as LinkedGuardianChildren (guardian holds an active
// orang_tua role there, student an active siswa role). Another school's
// links never appear.
func StudentGuardians(db *sql.DB, schoolID, studentUserID string) ([]model.StudentGuardian, error) {
	rows, err := db.Query(`SELECT gl.guardian_user_id, gl.relationship, gl.is_primary
		FROM guardian_links gl
		JOIN users u ON u.id=gl.student_user_id AND u.deleted_at IS NULL
		JOIN user_school_roles sr ON sr.user_id=u.id AND sr.school_id=gl.school_id AND sr.role='siswa' AND sr.status='active'
		WHERE gl.school_id=$1 AND gl.student_user_id=$2 AND gl.status='active' AND `+guardianActiveRoleSQL+`
		ORDER BY gl.is_primary DESC, gl.created_at, gl.guardian_user_id`, schoolID, studentUserID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []model.StudentGuardian{}
	for rows.Next() {
		var g model.StudentGuardian
		if err := rows.Scan(&g.GuardianUserID, &g.Relationship, &g.IsPrimary); err != nil {
			return nil, err
		}
		out = append(out, g)
	}
	return out, rows.Err()
}

func ScanGuardianLink(row interface{ Scan(...interface{}) error }) (model.GuardianLink, error) {
	var link model.GuardianLink
	err := row.Scan(&link.ID, &link.SchoolID, &link.GuardianUserID, &link.StudentUserID, &link.Relationship,
		&link.IsPrimary, &link.Status, &link.VerifiedAt, &link.CreatedAt, &link.UpdatedAt)
	return link, err
}

const GuardianLinkColumns = `id, school_id, guardian_user_id, student_user_id, relationship, is_primary, status, verified_at, created_at, updated_at`

// ConsumeGuardianClaim persists rate accounting, then performs code locking,
// single-use consumption, and link creation atomically. checkHash deliberately
// lives outside this package's crypto policy to keep the transactional logic
// testable and to allow bcrypt upgrades without changing SQL behavior.
func ConsumeGuardianClaim(db *sql.DB, schoolID, guardianID, nisn string, now time.Time, checkHash func(string) bool) (model.GuardianLink, error) {
	// Persist rate accounting outside the claim transaction: invalid-code
	// rollbacks must not erase the attempt.
	var attempts int
	err := db.QueryRow(`INSERT INTO guardian_claim_rate_limits (school_id, guardian_user_id, window_started_at, attempts)
		VALUES ($1,$2,$3,1)
		ON CONFLICT (school_id, guardian_user_id) DO UPDATE SET
			window_started_at = CASE WHEN guardian_claim_rate_limits.window_started_at < $3 - interval '15 minutes' THEN $3 ELSE guardian_claim_rate_limits.window_started_at END,
			attempts = CASE WHEN guardian_claim_rate_limits.window_started_at < $3 - interval '15 minutes' THEN 1 ELSE guardian_claim_rate_limits.attempts + 1 END
		RETURNING attempts`, schoolID, guardianID, now).Scan(&attempts)
	if err != nil {
		return model.GuardianLink{}, err
	}
	if attempts > 8 {
		return model.GuardianLink{}, ErrClaimRateLimit
	}

	tx, err := db.Begin()
	if err != nil {
		return model.GuardianLink{}, err
	}
	defer tx.Rollback()

	rows, err := tx.Query(`SELECT gcc.id, gcc.code_hash, gcc.used_at, gcc.expires_at, gcc.student_user_id
		FROM guardian_claim_codes gcc
		JOIN users u ON u.id=gcc.student_user_id AND u.deleted_at IS NULL
		JOIN user_school_roles sr ON sr.user_id=u.id AND sr.school_id=gcc.school_id AND sr.role='siswa' AND sr.status='active'
		WHERE gcc.school_id=$1 AND u.nisn=$2 AND gcc.revoked_at IS NULL AND gcc.expires_at>$3
		ORDER BY gcc.created_at DESC FOR UPDATE OF gcc`, schoolID, nisn, now)
	if err != nil {
		return model.GuardianLink{}, err
	}
	type candidate struct {
		id, hash, studentID string
		usedAt              sql.NullTime
	}
	var matched *candidate
	for rows.Next() {
		var c candidate
		var expires time.Time
		if err := rows.Scan(&c.id, &c.hash, &c.usedAt, &expires, &c.studentID); err != nil {
			rows.Close()
			return model.GuardianLink{}, err
		}
		if checkHash(c.hash) {
			matched = &c
			break
		}
	}
	rows.Close()
	if matched == nil {
		return model.GuardianLink{}, ErrClaimInvalid
	}
	if matched.usedAt.Valid {
		return model.GuardianLink{}, ErrClaimUsed
	}

	res, err := tx.Exec(`UPDATE guardian_claim_codes SET used_at=$1, used_by=$2 WHERE id=$3 AND used_at IS NULL`, now, guardianID, matched.id)
	if err != nil {
		return model.GuardianLink{}, err
	}
	if n, _ := res.RowsAffected(); n != 1 {
		return model.GuardianLink{}, ErrClaimUsed
	}
	row := tx.QueryRow(`INSERT INTO guardian_links (school_id, guardian_user_id, student_user_id, relationship, status, verified_at)
		VALUES ($1,$2,$3,'orang_tua','active',$4)
		ON CONFLICT (school_id, guardian_user_id, student_user_id) DO UPDATE SET
			status='active', verified_at=COALESCE(guardian_links.verified_at, EXCLUDED.verified_at), updated_at=now()
		RETURNING `+GuardianLinkColumns, schoolID, guardianID, matched.studentID, now)
	link, err := ScanGuardianLink(row)
	if err != nil {
		return model.GuardianLink{}, err
	}
	if err := tx.Commit(); err != nil {
		return model.GuardianLink{}, err
	}
	return link, nil
}
