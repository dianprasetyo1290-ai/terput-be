package handler

import (
	"bytes"
	"crypto/rand"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/go-chi/chi/v5"
	_ "github.com/jackc/pgx/v5/stdlib"
)

func newIdentityTestUUID() string {
	var b [16]byte
	_, _ = rand.Read(b[:])
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

func TestInternalDeactivateUser(t *testing.T) {
	dsn := os.Getenv("TEST_PG_DSN")
	if dsn == "" {
		dsn = "postgres://localhost/skci_t7a_identity?sslmode=disable"
	}
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Skipf("cannot open identity db: %v", err)
	}
	if err := db.Ping(); err != nil {
		t.Skipf("identity db not reachable: %v", err)
	}
	defer db.Close()

	schoolID := newIdentityTestUUID()
	studentID := newIdentityTestUUID()
	guardianID := newIdentityTestUUID()

	// Seed school
	_, err = db.Exec(`INSERT INTO schools (id, name) VALUES ($1, 'School Test') ON CONFLICT DO NOTHING`, schoolID)
	if err != nil {
		t.Fatalf("seed school: %v", err)
	}

	// Seed users
	_, err = db.Exec(`INSERT INTO users (id, name, email, password_hash) VALUES ($1, 'Student Test', $2, 'hash')`,
		studentID, fmt.Sprintf("student-%s@test.com", studentID[:8]))
	if err != nil {
		t.Fatalf("seed student: %v", err)
	}
	_, err = db.Exec(`INSERT INTO users (id, name, email, password_hash) VALUES ($1, 'Guardian Test', $2, 'hash')`,
		guardianID, fmt.Sprintf("guardian-%s@test.com", guardianID[:8]))
	if err != nil {
		t.Fatalf("seed guardian: %v", err)
	}

	// Seed user_school_role
	_, err = db.Exec(`INSERT INTO user_school_roles (user_id, school_id, role, status) VALUES ($1, $2, 'siswa', 'active')`,
		studentID, schoolID)
	if err != nil {
		t.Fatalf("seed role: %v", err)
	}

	// Seed guardian_link
	_, err = db.Exec(`INSERT INTO guardian_links (school_id, guardian_user_id, student_user_id, relationship, status) VALUES ($1, $2, $3, 'wali', 'active')`,
		schoolID, guardianID, studentID)
	if err != nil {
		t.Fatalf("seed guardian link: %v", err)
	}

	app := NewApp(db, "secret", "internal-key")
	r := chi.NewRouter()
	r.Post("/internal/users/{id}/deactivate", app.InternalDeactivateUser)

	body, _ := json.Marshal(map[string]any{
		"school_id": schoolID,
		"reason":    "Mutasi keluar",
	})
	req := httptest.NewRequest(http.MethodPost, "/internal/users/"+studentID+"/deactivate", bytes.NewReader(body))
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d: %s", rec.Code, rec.Body.String())
	}

	// Verify user_school_roles is inactive and has deactivated_at
	var roleStatus string
	var hasDeactivatedAt bool
	err = db.QueryRow(`SELECT status, deactivated_at IS NOT NULL FROM user_school_roles WHERE user_id = $1 AND school_id = $2`,
		studentID, schoolID).Scan(&roleStatus, &hasDeactivatedAt)
	if err != nil {
		t.Fatalf("query role: %v", err)
	}
	if roleStatus != "inactive" {
		t.Errorf("expected role status 'inactive', got %q", roleStatus)
	}
	if !hasDeactivatedAt {
		t.Errorf("expected role deactivated_at to be set")
	}

	// Verify guardian_links is inactive and has deactivated_at
	var linkStatus string
	var linkHasDeactivatedAt bool
	err = db.QueryRow(`SELECT status, deactivated_at IS NOT NULL FROM guardian_links WHERE student_user_id = $1 AND school_id = $2`,
		studentID, schoolID).Scan(&linkStatus, &linkHasDeactivatedAt)
	if err != nil {
		t.Fatalf("query link: %v", err)
	}
	if linkStatus != "inactive" {
		t.Errorf("expected link status 'inactive', got %q", linkStatus)
	}
	if !linkHasDeactivatedAt {
		t.Errorf("expected link deactivated_at to be set")
	}
}
