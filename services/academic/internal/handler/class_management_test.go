package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"

	"skci/academic/internal/common"
	"skci/academic/internal/store"
)

// HTTP-level checks for PRD menus 39-40 against the real test DB.

func cmRequest(t *testing.T, method, path, body string, params map[string]string, schoolID, userID string) *http.Request {
	t.Helper()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	rctx := chi.NewRouteContext()
	for k, v := range params {
		rctx.URLParams.Add(k, v)
	}
	ctx := context.WithValue(req.Context(), chi.RouteCtxKey, rctx)
	ctx = common.WithClaims(ctx, &common.Claims{UserID: userID, SchoolID: schoolID, ActiveRole: "admin", Roles: []string{"admin"}})
	return req.WithContext(ctx)
}

func newID(t *testing.T, h *Handler) string {
	t.Helper()
	var id string
	if err := h.Store.DB.QueryRow(`SELECT gen_random_uuid()::text`).Scan(&id); err != nil {
		t.Fatal(err)
	}
	return id
}

// Over-capacity enrollment is refused with 422 and an Indonesian message,
// and the accepted enrollment is visible in the classroom's Riwayat.
func TestAddStudentCapacityAndHistoryHTTP(t *testing.T) {
	db := getAcademicTestDB(t)
	var reg *string
	if err := db.QueryRow(`SELECT to_regclass('public.class_enrollment_history')::text`).Scan(&reg); err != nil || reg == nil {
		t.Skip("migration 0006 not applied")
	}
	h := &Handler{Store: store.New(db)}
	schoolID, yearID, adminID := newID(t, h), newID(t, h), newID(t, h)
	t.Cleanup(func() {
		_, _ = db.Exec(`DELETE FROM class_enrollments WHERE school_id = $1`, schoolID)
		_, _ = db.Exec(`DELETE FROM class_enrollment_history WHERE school_id = $1`, schoolID)
		_, _ = db.Exec(`DELETE FROM classrooms WHERE school_id = $1`, schoolID)
	})
	var classID string
	if err := db.QueryRow(`INSERT INTO classrooms (school_id, academic_year_id, name, capacity) VALUES ($1, $2, '7A', 1) RETURNING id`, schoolID, yearID).Scan(&classID); err != nil {
		t.Fatal(err)
	}

	add := func(student string) *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		h.AddStudentToClassroom(w, cmRequest(t, http.MethodPost, "/classrooms/"+classID+"/students",
			`{"student_user_id":"`+student+`"}`, map[string]string{"id": classID}, schoolID, adminID))
		return w
	}
	if w := add(newID(t, h)); w.Code != http.StatusCreated {
		t.Fatalf("first add = %d %s", w.Code, w.Body)
	}
	w := add(newID(t, h))
	if w.Code != http.StatusUnprocessableEntity || !strings.Contains(w.Body.String(), "Kelas 7A sudah penuh (1/1 siswa)") {
		t.Fatalf("over-capacity add = %d %s, want 422 with message", w.Code, w.Body)
	}

	w = httptest.NewRecorder()
	h.ListClassroomEnrollmentHistory(w, cmRequest(t, http.MethodGet, "/classrooms/"+classID+"/enrollment-history", "", map[string]string{"id": classID}, schoolID, adminID))
	var env struct {
		Data []store.EnrollmentHistoryEntry `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &env); err != nil || w.Code != http.StatusOK {
		t.Fatalf("history = %d %s", w.Code, w.Body)
	}
	if len(env.Data) != 1 || env.Data[0].Action != store.ActionEnroll || env.Data[0].ActorUserID == nil || *env.Data[0].ActorUserID != adminID {
		t.Fatalf("history entries = %+v", env.Data)
	}
}

// PUT /classroom-subjects/{id} with an explicit null teacher un-assigns the
// guru; omitting the key keeps it. hours_per_week is validated.
func TestUpdateClassroomSubjectTeacherNullHTTP(t *testing.T) {
	db := getAcademicTestDB(t)
	var reg *string
	if err := db.QueryRow(`SELECT to_regclass('public.class_enrollment_history')::text`).Scan(&reg); err != nil || reg == nil {
		t.Skip("migration 0006 not applied")
	}
	h := &Handler{Store: store.New(db)}
	schoolID, yearID, guru := newID(t, h), newID(t, h), newID(t, h)
	t.Cleanup(func() {
		_, _ = db.Exec(`DELETE FROM classroom_subjects WHERE school_id = $1`, schoolID)
		_, _ = db.Exec(`DELETE FROM subjects WHERE school_id = $1`, schoolID)
		_, _ = db.Exec(`DELETE FROM classrooms WHERE school_id = $1`, schoolID)
	})
	var classID, subjectID, csID string
	if err := db.QueryRow(`INSERT INTO classrooms (school_id, academic_year_id, name) VALUES ($1, $2, '8B') RETURNING id`, schoolID, yearID).Scan(&classID); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(`INSERT INTO subjects (school_id, name) VALUES ($1, 'Matematika') RETURNING id`, schoolID).Scan(&subjectID); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(`INSERT INTO classroom_subjects (school_id, academic_year_id, classroom_id, subject_id, teacher_user_id) VALUES ($1, $2, $3, $4, $5) RETURNING id`,
		schoolID, yearID, classID, subjectID, guru).Scan(&csID); err != nil {
		t.Fatal(err)
	}
	put := func(body string) *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		h.UpdateClassroomSubject(w, cmRequest(t, http.MethodPut, "/classroom-subjects/"+csID, body, map[string]string{"id": csID}, schoolID, guru))
		return w
	}
	if w := put(`{"hours_per_week": 99}`); w.Code != http.StatusBadRequest {
		t.Fatalf("invalid hours = %d", w.Code)
	}
	if w := put(`{"hours_per_week": 5}`); w.Code != http.StatusOK || !bytes.Contains(w.Body.Bytes(), []byte(`"teacher_user_id":"`+guru+`"`)) {
		t.Fatalf("hours-only update must keep teacher: %d %s", w.Code, w.Body)
	}
	w := put(`{"teacher_user_id": null}`)
	if w.Code != http.StatusOK || !bytes.Contains(w.Body.Bytes(), []byte(`"teacher_user_id":null`)) || !bytes.Contains(w.Body.Bytes(), []byte(`"hours_per_week":5`)) {
		t.Fatalf("explicit null must clear teacher: %d %s", w.Code, w.Body)
	}
}
