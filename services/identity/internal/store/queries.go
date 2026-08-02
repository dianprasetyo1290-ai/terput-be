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
	rows, err := db.Query(`SELECT id, user_id, school_id, role, status, academic_year_id FROM user_school_roles WHERE user_id = $1 ORDER BY created_at`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []model.UserSchoolRole
	for rows.Next() {
		var r model.UserSchoolRole
		if err := rows.Scan(&r.ID, &r.UserID, &r.SchoolID, &r.Role, &r.Status, &r.AcademicYearID); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func FindUserByEmail(db *sql.DB, email string) (model.User, string, error) {
	var u model.User
	var hash string
	err := db.QueryRow(`SELECT id, name, email, phone, password_hash, password_changed, photo, birth_date, birth_place, address, is_superadmin, created_at, updated_at
		FROM users WHERE email = $1 AND deleted_at IS NULL`, email).
		Scan(&u.ID, &u.Name, &u.Email, &u.Phone, &hash, &u.PasswordChanged, &u.Photo, &u.BirthDate, &u.BirthPlace, &u.Address, &u.IsSuperadmin, &u.CreatedAt, &u.UpdatedAt)
	if err == sql.ErrNoRows {
		return u, "", ErrNotFound
	}
	return u, hash, err
}

func FindUserByID(db *sql.DB, id string) (model.User, error) {
	var u model.User
	err := db.QueryRow(`SELECT id, name, email, phone, password_changed, photo, birth_date, birth_place, address, is_superadmin, created_at, updated_at
		FROM users WHERE id = $1 AND deleted_at IS NULL`, id).
		Scan(&u.ID, &u.Name, &u.Email, &u.Phone, &u.PasswordChanged, &u.Photo, &u.BirthDate, &u.BirthPlace, &u.Address, &u.IsSuperadmin, &u.CreatedAt, &u.UpdatedAt)
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
	priority := []string{"superadmin", "admin", "guru", "siswa"}
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
	return "", "", allRoleNames, false
}
