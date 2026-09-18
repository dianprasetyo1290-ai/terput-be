package upstream

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// A classroom with more exams than one page must come back whole. This is
// the SMK GPM bug: 53 exams in a term, the first (newest) 20 fetched, the
// previous week's STS silently missing from Generate Rapor.
func TestListExamsWalksEveryPage(t *testing.T) {
	const total = 53
	var gotPages []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		gotPages = append(gotPages, q.Get("page")+"/"+q.Get("per_page"))
		if q.Get("per_page") == "" || q.Get("page") == "" {
			t.Errorf("request without paging params: %s", r.URL.String())
		}
		page, perPage := 1, 200
		fmt.Sscanf(q.Get("page"), "%d", &page)
		fmt.Sscanf(q.Get("per_page"), "%d", &perPage)
		start := (page - 1) * perPage
		items := []Exam{}
		for i := start; i < start+perPage && i < total; i++ {
			items = append(items, Exam{ID: fmt.Sprintf("e%d", i)})
		}
		body, _ := json.Marshal(items)
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"data":%s}`, body)
	}))
	defer srv.Close()

	ec := NewExamClient(srv.URL, "k", 5*time.Second)
	out, err := ec.ListExams(context.Background(), "c1", "t1")
	if err != nil {
		t.Fatal(err)
	}
	if len(out) != total {
		t.Fatalf("got %d exams, want %d -- pages are being dropped", len(out), total)
	}
	if out[0].ID != "e0" || out[total-1].ID != fmt.Sprintf("e%d", total-1) {
		t.Fatalf("pages stitched out of order: first=%s last=%s", out[0].ID, out[total-1].ID)
	}
	// 53 < 200, so one short page is enough -- no needless extra round trip.
	if len(gotPages) != 1 {
		t.Fatalf("made %d requests (%v), want 1 for a short page", len(gotPages), gotPages)
	}
}

// An exactly-full page must be followed by one more request, or the tail is
// lost whenever the count is a multiple of per_page.
func TestListExamsFetchesPageAfterFullPage(t *testing.T) {
	const perPage = 200
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		page := 1
		fmt.Sscanf(r.URL.Query().Get("page"), "%d", &page)
		n := 0
		if page == 1 {
			n = perPage
		} else if page == 2 {
			n = 7
		}
		items := make([]Exam, n)
		for i := range items {
			items[i] = Exam{ID: fmt.Sprintf("p%d-%d", page, i)}
		}
		body, _ := json.Marshal(items)
		fmt.Fprintf(w, `{"data":%s}`, body)
	}))
	defer srv.Close()

	out, err := NewExamClient(srv.URL, "k", 5*time.Second).ListExams(context.Background(), "c1", "t1")
	if err != nil {
		t.Fatal(err)
	}
	if len(out) != perPage+7 {
		t.Fatalf("got %d, want %d -- the page after a full page was not fetched", len(out), perPage+7)
	}
	if calls != 2 {
		t.Fatalf("made %d requests, want 2", calls)
	}
}
