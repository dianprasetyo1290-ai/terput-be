package client

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

// Only students of classes the teacher is wali of or teaches are returned.
func TestTeacherStudentIDs(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Internal-Key") != "k" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		switch r.URL.Path {
		case "/classrooms":
			_, _ = w.Write([]byte(`{"data":[{"id":"c-wali","homeroom_teacher_user_id":"g1"},{"id":"c-teach","homeroom_teacher_user_id":"g2"},{"id":"c-other","homeroom_teacher_user_id":"g2"}]}`))
		case "/teaching-assignments":
			_, _ = w.Write([]byte(`{"data":[{"classroom_id":"c-teach"},{"classroom_id":"c-last-year"}]}`))
		case "/classrooms/c-wali/students":
			_, _ = w.Write([]byte(`{"data":[{"student_user_id":"s1"}]}`))
		case "/classrooms/c-teach/students":
			_, _ = w.Write([]byte(`{"data":[{"student_user_id":"s2"}]}`))
		default:
			t.Errorf("unexpected call %s", r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	got, err := New(srv.URL, "k").TeacherStudentIDs(context.Background(), "school", "g1")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || !got["s1"] || !got["s2"] {
		t.Fatalf("got %v, want s1+s2 only", got)
	}
	if _, err := New(srv.URL, "wrong").TeacherStudentIDs(context.Background(), "school", "g1"); err == nil {
		t.Fatal("downstream error must fail closed")
	}
}
