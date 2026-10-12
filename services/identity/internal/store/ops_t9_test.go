package store

import (
	"database/sql"
	"os"
	"slices"
	"testing"

	"skci/identity/internal/common"
)

// T9 §2b: all Bagian 3–4 feature keys are known and default off.
func TestFeatureStatusesIncludesOpsKeys(t *testing.T) {
	ops := []string{"facility", "health", "service_desk", "canteen", "wallet", "campaign", "teacher_parent_chat",
		"ppdb_marketing", "payment_multi_provider", "payment_autodebit", "inventory", "procurement", "asset", "budget",
		"tax_nonpayroll", "payable", "library", "transport", "counseling", "discipline", "event", "extracurricular",
		"achievement", "pkl", "quality", "gov_reporting", "bi", "ai_assistant", "predictive", "campus3d", "white_label"}
	if len(ops) != 31 {
		t.Fatalf("vector has %d keys", len(ops))
	}
	statuses := FeatureStatuses(map[string]bool{"facility": true})
	byKey := map[string]bool{}
	for _, s := range statuses {
		byKey[s.FeatureKey] = s.Enabled
	}
	for _, k := range ops {
		on, ok := byKey[k]
		if !ok {
			t.Errorf("FeatureStatuses misses %s", k)
		}
		if on != (k == "facility") {
			t.Errorf("%s enabled=%v", k, on)
		}
		if !IsKnownFeature(k) {
			t.Errorf("%s not allowed by PUT", k)
		}
	}
}

func TestMissingRequirements(t *testing.T) {
	cases := []struct {
		key     string
		enabled map[string]bool
		want    []string
	}{
		{"campus3d", map[string]bool{}, []string{"facility"}},
		{"campus3d", map[string]bool{"facility": false}, []string{"facility"}},
		{"campus3d", map[string]bool{"facility": true}, []string{}},
		{"facility", map[string]bool{}, []string{}},
	}
	for _, c := range cases {
		if got := MissingRequirements(c.key, c.enabled); !slices.Equal(got, c.want) {
			t.Errorf("%s %v: got %v want %v", c.key, c.enabled, got, c.want)
		}
	}
}

// openTestDB connects to a migrated identity database (TEST_PG_DSN).
func openTestDB(t *testing.T) *sql.DB {
	t.Helper()
	dsn := os.Getenv("TEST_PG_DSN")
	if dsn == "" {
		t.Skip("TEST_PG_DSN not set")
	}
	db, err := Open(dsn)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func mustID(t *testing.T, db *sql.DB, q string, args ...any) string {
	t.Helper()
	var id string
	if err := db.QueryRow(q, args...).Scan(&id); err != nil {
		t.Fatalf("%s: %v", q, err)
	}
	return id
}

func seedUser(t *testing.T, db *sql.DB, schoolID, role string) string {
	t.Helper()
	id := mustID(t, db, `INSERT INTO users (name, email, password_hash) VALUES ('U', gen_random_uuid()::text || '@t9.test', 'x') RETURNING id`)
	if _, err := db.Exec(`INSERT INTO user_school_roles (user_id, school_id, role) VALUES ($1,$2,$3)`, id, schoolID, role); err != nil {
		t.Fatalf("role %s: %v", role, err)
	}
	return id
}

// Migration 0017 accepts every Bagian 3–4 role (and IsValidRole agrees).
func TestOpsRolesAcceptedByCheck(t *testing.T) {
	if len(common.OpsRoles) != 25 {
		t.Fatalf("OpsRoles has %d roles", len(common.OpsRoles))
	}
	db := openTestDB(t)
	school := mustID(t, db, `INSERT INTO schools (name) VALUES ('Sekolah T9') RETURNING id`)
	for _, role := range common.OpsRoles {
		if !common.IsValidRole(role) {
			t.Errorf("IsValidRole(%s) = false", role)
		}
		seedUser(t, db, school, role)
	}
	if _, err := db.Exec(`INSERT INTO user_school_roles (user_id, school_id, role)
		VALUES ((SELECT id FROM users LIMIT 1), $1, 'peran_karangan')`, school); err == nil {
		t.Error("CHECK accepted an unknown role")
	}
}

// GET /internal/students/{id}/guardians: active links only, scoped per school.
func TestStudentGuardians(t *testing.T) {
	db := openTestDB(t)
	schoolA := mustID(t, db, `INSERT INTO schools (name) VALUES ('A') RETURNING id`)
	schoolB := mustID(t, db, `INSERT INTO schools (name) VALUES ('B') RETURNING id`)
	student := seedUser(t, db, schoolA, "siswa")
	mom := seedUser(t, db, schoolA, "orang_tua")
	dad := seedUser(t, db, schoolA, "orang_tua")
	ex := seedUser(t, db, schoolA, "orang_tua")
	link := func(g, rel string, primary bool, status string) {
		if _, err := db.Exec(`INSERT INTO guardian_links (school_id, guardian_user_id, student_user_id, relationship, is_primary, status)
			VALUES ($1,$2,$3,$4,$5,$6)`, schoolA, g, student, rel, primary, status); err != nil {
			t.Fatal(err)
		}
	}
	link(dad, "ayah", false, "active")
	link(mom, "ibu", true, "active")
	link(ex, "wali", false, "inactive")

	got, err := StudentGuardians(db, schoolA, student)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].GuardianUserID != mom || !got[0].IsPrimary || got[0].Relationship != "ibu" || got[1].GuardianUserID != dad {
		t.Fatalf("guardians = %+v", got)
	}
	other, err := StudentGuardians(db, schoolB, student)
	if err != nil {
		t.Fatal(err)
	}
	if len(other) != 0 {
		t.Errorf("other school sees %+v", other)
	}
}
