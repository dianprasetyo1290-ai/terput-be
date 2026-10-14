package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"

	"skci/academic/internal/client"
	"skci/academic/internal/common"
	"skci/academic/internal/store"
)

// QA-IDN BUG-071/072/015: PUT /terms/{id} against a fake identity.
func TestUpdateTermValidation(t *testing.T) {
	db := getAcademicTestDB(t)
	ctx := context.Background()
	schoolID := "a1111111-1111-4111-8111-111111111111"
	yearID := "a2222222-2222-4222-8222-222222222222"
	_, _ = db.ExecContext(ctx, `DELETE FROM terms WHERE academic_year_id=$1`, yearID)
	var sem1, sem2 string
	if err := db.QueryRowContext(ctx, `INSERT INTO terms (school_id, academic_year_id, name, "order") VALUES ($1,$2,'Semester 1',1) RETURNING id`, schoolID, yearID).Scan(&sem1); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRowContext(ctx, `INSERT INTO terms (school_id, academic_year_id, name, "order", start_date, end_date) VALUES ($1,$2,'Semester 2',2,'2028-01-05','2028-06-20') RETURNING id`, schoolID, yearID).Scan(&sem2); err != nil {
		t.Fatal(err)
	}

	locked := false
	identity := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Internal-Key") != "k" || r.URL.Path != "/academic-years/"+yearID {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		y := map[string]any{"id": yearID, "school_id": schoolID, "label": "2027/2028", "start_date": "2027-07-01", "end_date": "2028-06-30", "locked_at": nil}
		if locked {
			y["locked_at"] = "2028-07-01T00:00:00Z"
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"data": y})
	}))
	defer identity.Close()

	h := New(store.New(db), &client.IdentityClient{BaseURL: identity.URL, InternalKey: "k", HTTPClient: identity.Client()})
	r := chi.NewRouter()
	r.Put("/terms/{id}", h.UpdateTerm)
	put := func(id string, body map[string]string) int {
		b, _ := json.Marshal(body)
		req := httptest.NewRequest(http.MethodPut, "/terms/"+id, bytes.NewReader(b))
		req = req.WithContext(common.WithClaims(req.Context(), &common.Claims{UserID: "u", SchoolID: schoolID, ActiveRole: "admin"}))
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)
		return rec.Code
	}

	cases := []struct {
		name string
		id   string
		body map[string]string
		want int
	}{
		{"end before start (BUG-071)", sem1, map[string]string{"start_date": "2026-10-14", "end_date": "2026-01-01"}, http.StatusBadRequest},
		{"outside year (BUG-072)", sem2, map[string]string{"end_date": "2028-10-31"}, http.StatusBadRequest},
		{"overlap semester 2", sem1, map[string]string{"start_date": "2027-07-12", "end_date": "2028-01-10"}, http.StatusBadRequest},
		{"bad format", sem1, map[string]string{"start_date": "14-10-2027"}, http.StatusBadRequest},
		{"valid", sem1, map[string]string{"start_date": "2027-07-12", "end_date": "2027-12-20"}, http.StatusOK},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := put(c.id, c.body); got != c.want {
				t.Fatalf("got %d want %d", got, c.want)
			}
		})
	}
	locked = true
	if got := put(sem1, map[string]string{"end_date": "2027-12-21"}); got != http.StatusConflict {
		t.Fatalf("locked year = %d, want 409", got)
	}
	identity.Close()
	if got := put(sem1, map[string]string{"end_date": "2027-12-21"}); got != http.StatusServiceUnavailable {
		t.Fatalf("identity down = %d, want 503", got)
	}
	// DB constraint (migration 0008) backs the rule.
	if _, err := db.ExecContext(ctx, `UPDATE terms SET start_date='2027-12-01', end_date='2027-01-01' WHERE id=$1`, sem1); err == nil {
		t.Fatal("terms_dates_check not enforced")
	}
}
