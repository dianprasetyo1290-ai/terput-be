package upstream

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// A linked wali is a 200 from identity, on the exact path and with the
// internal key, and must read back as (true, nil).
func TestGuardianStudentAccessLinked(t *testing.T) {
	var gotPath, gotKey string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.String()
		gotKey = r.Header.Get("X-Internal-Key")
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"data":{"linked":true}}`))
	}))
	defer srv.Close()

	ic := NewIdentityClient(srv.URL, "secret-key", 5*time.Second)
	linked, err := ic.GuardianStudentAccess(context.Background(), "wali-1", "stu-1", "school-1")
	if err != nil {
		t.Fatalf("linked guardian must not error: %v", err)
	}
	if !linked {
		t.Error("200 must read as linked")
	}
	if want := "/internal/guardians/wali-1/students/stu-1/access?school_id=school-1"; gotPath != want {
		t.Errorf("path = %q, want %q", gotPath, want)
	}
	if gotKey != "secret-key" {
		t.Errorf("X-Internal-Key = %q, want %q", gotKey, "secret-key")
	}
}

// A 404 is identity's "not linked" answer -- a valid, non-degraded result
// that must be (false, nil), not an error: the gate needs to tell the two
// apart so it answers 403 (not 503) for a genuinely unlinked wali.
func TestGuardianStudentAccessNotLinkedIsNotAnError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	linked, err := NewIdentityClient(srv.URL, "k", 5*time.Second).
		GuardianStudentAccess(context.Background(), "wali-1", "stu-2", "school-1")
	if err != nil {
		t.Fatalf("404 (not linked) must be a clean false, got %v", err)
	}
	if linked {
		t.Error("404 must read as not linked")
	}
}

// Anything other than 200/404 means identity could not actually answer, so
// the caller must get an error it can fail closed on -- never a silent
// "linked" or "not linked".
func TestGuardianStudentAccessFailsClosedOnUnexpectedStatus(t *testing.T) {
	for _, code := range []int{http.StatusInternalServerError, http.StatusUnauthorized, http.StatusBadGateway} {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(code)
		}))
		linked, err := NewIdentityClient(srv.URL, "k", 5*time.Second).
			GuardianStudentAccess(context.Background(), "wali-1", "stu-1", "school-1")
		srv.Close()

		if linked {
			t.Errorf("status %d must never read as linked", code)
		}
		if err == nil {
			t.Errorf("status %d must return an error so the gate fails closed", code)
		}
		if !errors.Is(err, ErrUnavailable) {
			t.Errorf("status %d: error should wrap ErrUnavailable, got %v", code, err)
		}
	}
}

// An unconfigured client (empty base URL) cannot reach identity -- it must
// error rather than report anyone as linked.
func TestGuardianStudentAccessUnconfiguredFailsClosed(t *testing.T) {
	linked, err := NewIdentityClient("", "k", 5*time.Second).
		GuardianStudentAccess(context.Background(), "wali-1", "stu-1", "school-1")
	if linked {
		t.Error("an unreachable identity must not read as linked")
	}
	if err == nil {
		t.Error("an unreachable identity must return an error")
	}
}
