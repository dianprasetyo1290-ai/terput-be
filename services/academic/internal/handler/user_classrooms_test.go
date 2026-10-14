package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sort"
	"testing"

	"skci/academic/internal/common"
	"skci/academic/internal/store"
)

func TestParseUserIDs(t *testing.T) {
	cases := []struct {
		raw  string
		ok   bool
		want int
	}{
		{"", false, 0},
		{"abc", false, 0},
		{"11111111-1111-1111-1111-111111111111", true, 1},
		{"11111111-1111-1111-1111-111111111111, 22222222-2222-2222-2222-222222222222,", true, 2},
		{"11111111-1111-1111-1111-111111111111,'; drop", false, 0},
	}
	for _, c := range cases {
		got, ok := parseUserIDs(c.raw)
		if ok != c.ok || len(got) != c.want {
			t.Errorf("parseUserIDs(%q) = %v,%v want %d,%v", c.raw, got, ok, c.want, c.ok)
		}
	}
}

func TestInternalUserClassrooms(t *testing.T) {
	db := getAcademicTestDB(t)
	ctx := context.Background()
	h := &Handler{Store: store.New(db)}
	srv := common.InternalOrJWTMiddleware("jwt-secret", "internal-key")(http.HandlerFunc(h.InternalUserClassrooms))

	school := "a1a1a1a1-0000-0000-0000-000000000001"
	year := "a1a1a1a1-0000-0000-0000-000000000002"
	classA := "a1a1a1a1-0000-0000-0000-00000000000a"
	classB := "a1a1a1a1-0000-0000-0000-00000000000b"
	classC := "a1a1a1a1-0000-0000-0000-00000000000c"
	student := "a1a1a1a1-0000-0000-0000-0000000000a1"
	wali := "a1a1a1a1-0000-0000-0000-0000000000a2"
	guru := "a1a1a1a1-0000-0000-0000-0000000000a3"
	subject := "a1a1a1a1-0000-0000-0000-0000000000f1"
	cleanup := func() {
		_, _ = db.ExecContext(ctx, `DELETE FROM classroom_subjects WHERE school_id=$1`, school)
		_, _ = db.ExecContext(ctx, `DELETE FROM class_enrollments WHERE school_id=$1`, school)
		_, _ = db.ExecContext(ctx, `DELETE FROM classrooms WHERE school_id=$1`, school)
		_, _ = db.ExecContext(ctx, `DELETE FROM subjects WHERE id=$1`, subject)
	}
	cleanup()
	t.Cleanup(cleanup)
	mustExec := func(q string, args ...any) {
		t.Helper()
		if _, err := db.ExecContext(ctx, q, args...); err != nil {
			t.Fatalf("%s: %v", q, err)
		}
	}
	mustExec(`INSERT INTO classrooms (id, school_id, academic_year_id, name, homeroom_teacher_user_id) VALUES
		($1,$4,$5,'X-A',NULL), ($2,$4,$5,'X-B',$6), ($3,$4,$5,'X-C',NULL)`, classA, classB, classC, school, year, wali)
	mustExec(`INSERT INTO class_enrollments (school_id, academic_year_id, classroom_id, student_user_id) VALUES ($1,$2,$3,$4)`, school, year, classA, student)
	mustExec(`INSERT INTO subjects (id, school_id, name) VALUES ($1,$2,'Matematika UC')`, subject, school)
	mustExec(`INSERT INTO classroom_subjects (school_id, academic_year_id, classroom_id, subject_id, teacher_user_id) VALUES ($1,$2,$3,$4,$5)`, school, year, classC, subject, guru)

	call := func(key, users string) (int, []string) {
		req := httptest.NewRequest(http.MethodGet, "/internal/user-classrooms?school_id="+school+"&user_ids="+users, nil)
		if key != "" {
			req.Header.Set("X-Internal-Key", key)
		}
		rec := httptest.NewRecorder()
		srv.ServeHTTP(rec, req)
		var env struct {
			Data []string `json:"data"`
		}
		_ = json.Unmarshal(rec.Body.Bytes(), &env)
		sort.Strings(env.Data)
		return rec.Code, env.Data
	}
	cases := []struct {
		name, key, users string
		code             int
		want             []string
	}{
		{"no key", "", student, http.StatusUnauthorized, nil},
		{"bad ids", "internal-key", "x", http.StatusBadRequest, nil},
		{"student", "internal-key", student, http.StatusOK, []string{classA}},
		{"wali kelas", "internal-key", wali, http.StatusOK, []string{classB}},
		{"guru pengampu", "internal-key", guru, http.StatusOK, []string{classC}},
		{"parent passes children", "internal-key", student + "," + wali, http.StatusOK, []string{classA, classB}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			code, got := call(c.key, c.users)
			if code != c.code {
				t.Fatalf("code=%d want %d", code, c.code)
			}
			if c.want != nil && (len(got) != len(c.want) || (len(got) > 0 && got[0] != c.want[0])) {
				t.Fatalf("got %v want %v", got, c.want)
			}
		})
	}
}
