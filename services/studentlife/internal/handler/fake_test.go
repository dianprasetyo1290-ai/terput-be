package handler

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"skci/studentlife/internal/client"
	"skci/studentlife/internal/common"
	"skci/studentlife/internal/config"
	"skci/studentlife/internal/sensitive"
	"skci/studentlife/internal/store"
)

const (
	testSecret   = "studentlife-test-secret"
	testInternal = "studentlife-test-internal"
)

// fakeWorld stands in for identity, academic, attendance, notification,
// core and ops over one httptest server.
type fakeWorld struct {
	mu             sync.Mutex
	features       map[string]bool              // school → health enabled
	guardians      map[string][]string          // student → guardian ids
	homeroom       map[string]string            // classroom → teacher
	roster         map[string][]string          // classroom → students
	grade          map[string]string            // classroom → grade
	names          map[string]string            // user → name
	flowConfigured bool                         // core approval flow exists
	opsStatus      int                          // 0 = 201
	attendance     []map[string]any             // calls
	notifications  map[string]int               // idempotency key → calls
	approvals      []map[string]any             // calls
	audits         int
}

func (f *fakeWorld) classOf(student string) string {
	for cl, ss := range f.roster {
		for _, s := range ss {
			if s == student {
				return cl
			}
		}
	}
	return ""
}

func (f *fakeWorld) write(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{"data": data, "message": ""})
}

func (f *fakeWorld) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if r.Header.Get("X-Internal-Key") != testInternal {
		f.write(w, 401, nil)
		return
	}
	raw, _ := io.ReadAll(r.Body)
	var body map[string]any
	_ = json.Unmarshal(raw, &body)
	parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
	switch {
	case len(parts) == 3 && parts[0] == "schools" && parts[2] == "features":
		f.write(w, 200, []map[string]any{{"feature_key": common.FeatureHealth, "enabled": f.features[parts[1]]}})
	case len(parts) == 6 && parts[0] == "internal" && parts[1] == "guardians" && parts[5] == "access":
		allowed := false
		for _, g := range f.guardians[parts[4]] {
			allowed = allowed || g == parts[2]
		}
		f.write(w, 200, map[string]bool{"allowed": allowed})
	case len(parts) == 4 && parts[0] == "internal" && parts[1] == "guardians" && parts[3] == "children":
		out := []map[string]string{}
		for s, gs := range f.guardians {
			for _, g := range gs {
				if g == parts[2] {
					out = append(out, map[string]string{"student_user_id": s, "name": f.names[s]})
				}
			}
		}
		f.write(w, 200, out)
	case len(parts) == 4 && parts[0] == "internal" && parts[1] == "students" && parts[3] == "guardians":
		out := []map[string]any{}
		for i, g := range f.guardians[parts[2]] {
			out = append(out, map[string]any{"guardian_user_id": g, "relationship": "wali", "is_primary": i == 0})
		}
		f.write(w, 200, out)
	case r.URL.Path == "/users/search":
		out := []map[string]string{}
		for _, id := range strings.Split(body["ids"].(string), ",") {
			out = append(out, map[string]string{"id": id, "name": f.names[id]})
		}
		f.write(w, 200, out)
	case r.URL.Path == "/students":
		f.write(w, 200, []map[string]string{})
	case len(parts) == 3 && parts[0] == "students" && parts[2] == "classroom":
		f.write(w, 200, map[string]string{"classroom_id": f.classOf(parts[1])})
	case r.URL.Path == "/classrooms":
		out := []map[string]any{}
		for cl := range f.roster {
			g := f.grade[cl]
			out = append(out, map[string]any{"id": cl, "name": cl, "grade_level": g})
		}
		f.write(w, 200, out)
	case len(parts) == 2 && parts[0] == "classrooms":
		if _, ok := f.roster[parts[1]]; !ok {
			f.write(w, 404, nil)
			return
		}
		t := f.homeroom[parts[1]]
		f.write(w, 200, map[string]any{"id": parts[1], "name": parts[1], "homeroom_teacher_user_id": t})
	case len(parts) == 3 && parts[0] == "classrooms" && parts[2] == "students":
		out := []map[string]string{}
		for _, s := range f.roster[parts[1]] {
			out = append(out, map[string]string{"student_user_id": s})
		}
		f.write(w, 200, out)
	case r.URL.Path == "/internal/student-absences/health":
		f.attendance = append(f.attendance, body)
		f.write(w, 200, map[string]any{"applied": true})
	case r.URL.Path == "/internal/notifications":
		f.notifications[r.Header.Get("Idempotency-Key")]++
		f.write(w, 201, map[string]any{})
	case r.URL.Path == "/internal/approvals":
		if !f.flowConfigured {
			f.write(w, 422, nil)
			return
		}
		f.approvals = append(f.approvals, body)
		f.write(w, 201, map[string]any{"id": "00000000-0000-4000-8000-00000000a001", "status": "pending"})
	case r.URL.Path == "/internal/audit":
		f.audits++
		f.write(w, 202, nil)
	case r.URL.Path == "/internal/exceptions":
		f.write(w, 201, nil)
	case r.URL.Path == "/internal/inventory/issues":
		st := f.opsStatus
		if st == 0 {
			st = 201
		}
		f.write(w, st, map[string]any{})
	default:
		f.write(w, 404, nil)
	}
}

type env struct {
	t      *testing.T
	db     *sql.DB
	app    *App
	world  *fakeWorld
	server *httptest.Server
	router http.Handler
	school string
}

func newID(t *testing.T, db *sql.DB) string {
	var id string
	if err := db.QueryRow(`SELECT gen_random_uuid()::text`).Scan(&id); err != nil {
		t.Fatal(err)
	}
	return id
}

// setup needs a migrated studentlife DB in TEST_PG_DSN.
func setup(t *testing.T) *env {
	dsn := os.Getenv("TEST_PG_DSN")
	if dsn == "" {
		t.Skip("TEST_PG_DSN not set")
	}
	db, err := store.Open(dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	world := &fakeWorld{features: map[string]bool{}, guardians: map[string][]string{}, homeroom: map[string]string{},
		roster: map[string][]string{}, grade: map[string]string{}, names: map[string]string{}, notifications: map[string]int{}}
	srv := httptest.NewServer(world)
	t.Cleanup(srv.Close)
	cfg := config.Config{JWTSecret: testSecret, InternalKey: testInternal, PublicInternalURL: "http://studentlife.test",
		PortalURL: "http://portal.test", DefaultTimezone: "Asia/Jakarta", JobMaxAttempts: 3}
	cr, err := sensitive.New(bytes.Repeat([]byte{0x21}, 32), "studentlife-test-blind-key")
	if err != nil {
		t.Fatal(err)
	}
	cl := client.New(client.URLs{Identity: srv.URL, Academic: srv.URL, Attendance: srv.URL, Notification: srv.URL, Core: srv.URL}, testInternal)
	app := New(db, cfg, cr, cl)
	e := &env{t: t, db: db, app: app, world: world, server: srv, router: app.Router(), school: newID(t, db)}
	world.features[e.school] = true
	return e
}

func (e *env) token(userID, role string) string {
	tok := jwt.NewWithClaims(jwt.SigningMethodHS256, common.Claims{UserID: userID, SchoolID: e.school, ActiveRole: role,
		RegisteredClaims: jwt.RegisteredClaims{ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour))}})
	s, err := tok.SignedString([]byte(testSecret))
	if err != nil {
		e.t.Fatal(err)
	}
	return s
}

type resp struct {
	Code int
	Body []byte
}

func (r resp) data(t *testing.T, dst any) {
	var envl struct {
		Data json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(r.Body, &envl); err != nil {
		t.Fatalf("decode %s: %v", r.Body, err)
	}
	if err := json.Unmarshal(envl.Data, dst); err != nil {
		t.Fatalf("decode data %s: %v", envl.Data, err)
	}
}

func (e *env) do(method, path, token string, body any, headers ...string) resp {
	var rd io.Reader
	if body != nil {
		raw, _ := json.Marshal(body)
		rd = bytes.NewReader(raw)
	}
	req := httptest.NewRequest(method, path, rd)
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	for i := 0; i+1 < len(headers); i += 2 {
		req.Header.Set(headers[i], headers[i+1])
	}
	rec := httptest.NewRecorder()
	e.router.ServeHTTP(rec, req)
	return resp{Code: rec.Code, Body: rec.Body.Bytes()}
}

func (e *env) count(query string, args ...any) int {
	var n int
	if err := e.db.QueryRow(query, args...).Scan(&n); err != nil {
		e.t.Fatalf("%s: %v", query, err)
	}
	return n
}
