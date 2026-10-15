package handler

import (
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
)

func TestParseDemoDomains(t *testing.T) {
	got := ParseDemoDomains(" Teratai.sch.id, @sekoci.local ,, ")
	want := []string{"teratai.sch.id", "sekoci.local"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("ParseDemoDomains = %v, want %v", got, want)
	}
	if got := ParseDemoDomains(""); len(got) != 0 {
		t.Fatalf("empty DEMO_ACCESS_DOMAINS must disable demo access, got %v", got)
	}
}

// A school's live system leaves DEMO_ACCESS_DOMAINS unset: the page must
// answer 404 without touching the database.
func TestDemoAccessDisabledIs404(t *testing.T) {
	a := &App{}
	rec := httptest.NewRecorder()
	a.DemoAccess(rec, httptest.NewRequest(http.MethodGet, "/public/demo-access", nil))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
}
