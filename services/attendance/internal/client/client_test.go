package client

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestCheckGuardianAccessLinked(t *testing.T) {
	var gotPath, gotKey, gotSchool string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotKey = r.Header.Get("X-Internal-Key")
		gotSchool = r.URL.Query().Get("school_id")
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	c := NewWithBaseURLs(srv.URL, "", "secret-key")
	if err := c.CheckGuardianAccess(context.Background(), "g-1", "s-1", "school-1"); err != nil {
		t.Fatalf("linked guardian: expected nil, got %v", err)
	}
	if gotPath != "/internal/guardians/g-1/students/s-1/access" {
		t.Errorf("path = %q, want /internal/guardians/g-1/students/s-1/access", gotPath)
	}
	if gotKey != "secret-key" {
		t.Errorf("X-Internal-Key = %q, want secret-key", gotKey)
	}
	if gotSchool != "school-1" {
		t.Errorf("school_id = %q, want school-1", gotSchool)
	}
}

func TestCheckGuardianAccessNotLinked(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	c := NewWithBaseURLs(srv.URL, "", "k")
	err := c.CheckGuardianAccess(context.Background(), "g", "s", "sch")
	if !errors.Is(err, ErrGuardianNoAccess) {
		t.Fatalf("404 should map to ErrGuardianNoAccess, got %v", err)
	}
}

func TestCheckGuardianAccessUpstreamErrorDenies(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	c := NewWithBaseURLs(srv.URL, "", "k")
	err := c.CheckGuardianAccess(context.Background(), "g", "s", "sch")
	if err == nil {
		t.Fatal("upstream 500 must return an error (deny), got nil")
	}
	if errors.Is(err, ErrGuardianNoAccess) || errors.Is(err, ErrGuardianCheckUnavailable) {
		t.Fatalf("upstream 500 should be a generic error, got %v", err)
	}
}

func TestCheckGuardianAccessUnconfiguredFailsClosed(t *testing.T) {
	c := NewWithBaseURLs("", "", "k")
	err := c.CheckGuardianAccess(context.Background(), "g", "s", "sch")
	if !errors.Is(err, ErrGuardianCheckUnavailable) {
		t.Fatalf("unconfigured client must fail closed with ErrGuardianCheckUnavailable, got %v", err)
	}
}
