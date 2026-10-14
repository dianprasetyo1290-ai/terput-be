package handler

import (
	"context"
	"net/http"
	"testing"

	"skci/academic/internal/common"
)

// TestMenu78WaliKelasOwnClassOnly (QA-PAY round 2): a wali kelas lists,
// opens and looks up finances only for requests of their own class.
func TestMenu78WaliKelasOwnClassOnly(t *testing.T) {
	f := newMenu78Fixture(t)
	ctx := context.Background()
	const (
		wali  = "f7800000-0000-4000-8000-0000000000a1"
		other = "f7800000-0000-4000-8000-0000000000a2"
	)
	classID := "f7800000-0000-4000-8000-000000000005"
	if _, err := f.h.Store.DB.ExecContext(ctx, `UPDATE classrooms SET homeroom_teacher_user_id = $1 WHERE id = $2`, wali, classID); err != nil {
		t.Fatal(err)
	}

	code, env := f.create("walikelas", wali, "2026-08-01")
	if code != http.StatusCreated {
		t.Fatalf("create by own wali: %d %v", code, env)
	}
	id := dataField(env, "id")

	listCount := func(user string) int {
		code, env := f.do(http.MethodGet, "/student-status-requests", "walikelas", user, nil)
		if code != http.StatusOK {
			t.Fatalf("list %s: %d %v", user, code, env)
		}
		rows, _ := env["data"].([]any)
		return len(rows)
	}
	if n := listCount(wali); n != 1 {
		t.Fatalf("own wali sees %d rows, want 1", n)
	}
	if n := listCount(other); n != 0 {
		t.Fatalf("other wali sees %d rows, want 0", n)
	}
	if code, env := f.do(http.MethodGet, "/student-status-requests", common.RoleAdmin, f.admin, nil); code != http.StatusOK || len(env["data"].([]any)) != 1 {
		t.Fatalf("admin list: %d %v", code, env)
	}

	if code, _ := f.do(http.MethodGet, "/student-status-requests/"+id, "walikelas", wali, nil); code != http.StatusOK {
		t.Fatalf("detail own wali: %d", code)
	}
	if code, _ := f.do(http.MethodGet, "/student-status-requests/"+id, "walikelas", other, nil); code != http.StatusForbidden {
		t.Fatalf("detail other wali: %d want 403", code)
	}
	fin := "/student-status-requests/financial-summary?student_user_id=" + f.student
	if code, _ := f.do(http.MethodGet, fin, "walikelas", wali, nil); code != http.StatusOK {
		t.Fatalf("finance own wali: %d", code)
	}
	if code, _ := f.do(http.MethodGet, fin, "walikelas", other, nil); code != http.StatusForbidden {
		t.Fatalf("finance other wali: %d want 403", code)
	}
}
