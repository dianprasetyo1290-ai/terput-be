package store

import (
	"database/sql"
	"errors"

	"skci/identity/internal/model"
)

var ErrNotFound = errors.New("not found")

// UserRoles returns every role row the user holds (dashboard roles +
// jabatan-only tags), across all schools.
func UserRoles(db *sql.DB, userID string) ([]model.UserSchoolRole, error) {
	rows, err := db.Query(`SELECT id, user_id, school_id, role, status, academic_year_id, student_status FROM user_school_roles WHERE user_id = $1 ORDER BY created_at`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []model.UserSchoolRole
	for rows.Next() {
		var r model.UserSchoolRole
		if err := rows.Scan(&r.ID, &r.UserID, &r.SchoolID, &r.Role, &r.Status, &r.AcademicYearID, &r.StudentStatus); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func FindUserByEmail(db *sql.DB, email string) (model.User, string, error) {
	var u model.User
	var hash string
	err := db.QueryRow(`SELECT id, name, email, phone, password_hash, password_changed, photo, birth_date, birth_place, address, is_superadmin, created_at, updated_at, managed_by_hr, deactivate_at
		FROM users WHERE email = $1 AND deleted_at IS NULL`, email).
		Scan(&u.ID, &u.Name, &u.Email, &u.Phone, &hash, &u.PasswordChanged, &u.Photo, &u.BirthDate, &u.BirthPlace, &u.Address, &u.IsSuperadmin, &u.CreatedAt, &u.UpdatedAt, &u.ManagedByHR, &u.DeactivateAt)
	if err == sql.ErrNoRows {
		return u, "", ErrNotFound
	}
	return u, hash, err
}

func FindUserByID(db *sql.DB, id string) (model.User, error) {
	var u model.User
	err := db.QueryRow(`SELECT id, name, email, phone, password_changed, photo, birth_date, birth_place, address, religion, nisn, is_superadmin, created_at, updated_at, managed_by_hr, deactivate_at
		FROM users WHERE id = $1 AND deleted_at IS NULL`, id).
		Scan(&u.ID, &u.Name, &u.Email, &u.Phone, &u.PasswordChanged, &u.Photo, &u.BirthDate, &u.BirthPlace, &u.Address, &u.Religion, &u.NISN, &u.IsSuperadmin, &u.CreatedAt, &u.UpdatedAt, &u.ManagedByHR, &u.DeactivateAt)
	if err == sql.ErrNoRows {
		return u, ErrNotFound
	}
	return u, err
}

// HasRole reports whether the user holds the given role (optionally scoped
// to a school; pass "" to ignore school scoping, e.g. for superadmin).
func HasRole(db *sql.DB, userID, role, schoolID string) (bool, error) {
	var exists bool
	var err error
	if schoolID == "" {
		err = db.QueryRow(`SELECT EXISTS(SELECT 1 FROM user_school_roles WHERE user_id=$1 AND role=$2)`, userID, role).Scan(&exists)
	} else {
		err = db.QueryRow(`SELECT EXISTS(SELECT 1 FROM user_school_roles WHERE user_id=$1 AND role=$2 AND school_id=$3)`, userID, role, schoolID).Scan(&exists)
	}
	return exists, err
}

// PickActiveRole picks the default active_role to issue at login: prefers
// dashboard roles (superadmin > admin > guru > siswa) since jabatan-only
// roles have no dashboard of their own.
func PickActiveRole(roles []model.UserSchoolRole) (activeRole, schoolID string, allRoleNames []string, ok bool) {
	// Priority only orders the roles that can collide on one account; it is
	// NOT an allowlist. It used to be treated as one -- any account holding
	// *only* a role missing from this list (walikelas, kepala_sekolah,
	// kepala_yayasan, and V11's pengawas) fell through to ok=false, so
	// Login answered "account has no assigned role" and the login screen
	// showed "Email atau password salah." The bug stayed hidden because
	// every walikelas/kepala_sekolah in production also holds guru; the
	// first pengawas-only account exposed it. The fallback below means a
	// role added later never has to be remembered here again.
	// Jabatan roles rank ABOVE guru: an account that holds guru plus
	// walikelas/kepala_sekolah/kepala_yayasan used to log straight into the
	// classroom (active_role=guru), so Kepala Sekolah and Wali Kelas landed on
	// exactly the guru UI and looked identical to a plain teacher until the
	// user manually switched. Leadership/duty role wins so each account lands
	// on its own dashboard.
	priority := []string{"superadmin", "admin", "kepala_yayasan", "kepala_sekolah", "walikelas", "guru", "siswa", "orang_tua"}
	byRole := map[string]model.UserSchoolRole{}
	// roles is ordered by created_at ascending (see UserRoles) — a user can hold the same
	// role at more than one school (unique constraint is user_id+school_id+role, not
	// user_id+role), so keep the FIRST (oldest/primary) assignment per role name rather than
	// overwriting on every iteration. This must match SwitchRole's own first-match selection
	// below, or the same account gets a different school_id from login vs. switch-role —
	// which is exactly the kind of inconsistency that made one admin's session bleed into
	// another school's data.
	for _, r := range roles {
		if _, exists := byRole[r.Role]; !exists {
			byRole[r.Role] = r
		}
		allRoleNames = append(allRoleNames, r.Role)
	}
	for _, p := range priority {
		if r, found := byRole[p]; found {
			sid := ""
			if r.SchoolID != nil {
				sid = *r.SchoolID
			}
			return r.Role, sid, allRoleNames, true
		}
	}
	// Holds a role, just not a prioritised one: use the oldest assignment.
	// roles is ordered by created_at ascending (see UserRoles).
	if len(roles) > 0 {
		r := roles[0]
		sid := ""
		if r.SchoolID != nil {
			sid = *r.SchoolID
		}
		return r.Role, sid, allRoleNames, true
	}
	return "", "", allRoleNames, false
}
