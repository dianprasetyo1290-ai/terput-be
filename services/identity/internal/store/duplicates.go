package store

import (
	"context"
	"database/sql"

	"github.com/lib/pq"
)

// DuplicateMatch is another live account sharing an identifying number.
type DuplicateMatch struct {
	Field     string   `json:"field"` // nisn | nik | phone
	UserID    string   `json:"user_id"`
	Name      string   `json:"name"`
	SchoolIDs []string `json:"-"`
}

// phoneNormSQL mirrors rules.NormalizePhone in SQL.
const phoneNormSQL = `(CASE WHEN d LIKE '0%' THEN '62' || substr(d, 2) WHEN d LIKE '8%' THEN '62' || d ELSE d END)`

// FindDuplicateUsers lists accounts other than excludeID with the same NISN,
// NIK or (normalized) phone. Empty inputs are skipped. BUG-016.
func FindDuplicateUsers(ctx context.Context, db *sql.DB, excludeID, nisn, nik, phoneNorm string) ([]DuplicateMatch, error) {
	if nisn == "" && nik == "" && phoneNorm == "" {
		return nil, nil
	}
	rows, err := db.QueryContext(ctx, `
		WITH c AS (
			SELECT u.id, u.name, u.nisn, u.nik, regexp_replace(COALESCE(u.phone,''), '\D', '', 'g') AS d
			FROM users u
			WHERE u.deleted_at IS NULL AND u.id::text <> $1
			  AND (($2 <> '' AND u.nisn = $2) OR ($3 <> '' AND u.nik = $3) OR ($4 <> '' AND u.phone IS NOT NULL))
		)
		SELECT f.field, c.id, c.name,
		       COALESCE((SELECT array_agg(DISTINCT school_id::text) FROM user_school_roles r WHERE r.user_id = c.id AND r.school_id IS NOT NULL), '{}')
		FROM c
		CROSS JOIN LATERAL (VALUES
			(CASE WHEN $2 <> '' AND c.nisn = $2 THEN 'nisn' END),
			(CASE WHEN $3 <> '' AND c.nik = $3 THEN 'nik' END),
			(CASE WHEN $4 <> '' AND c.d <> '' AND `+phoneNormSQL+` = $4 THEN 'phone' END)
		) AS f(field)
		WHERE f.field IS NOT NULL
		ORDER BY f.field, c.name
		LIMIT 20`, excludeID, nisn, nik, phoneNorm)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []DuplicateMatch
	for rows.Next() {
		var m DuplicateMatch
		if err := rows.Scan(&m.Field, &m.UserID, &m.Name, pq.Array(&m.SchoolIDs)); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}
