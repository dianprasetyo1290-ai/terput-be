package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"skci/attendance/internal/cache"
	"skci/attendance/internal/store"
)

type fakeHealthAbsence struct {
	calls []store.HealthAbsenceInput
}

func (f *fakeHealthAbsence) ApplyHealthAbsence(_ context.Context, in store.HealthAbsenceInput) (bool, string, error) {
	f.calls = append(f.calls, in)
	return true, "", nil
}

func TestInternalHealthAbsence(t *testing.T) {
	fake := &fakeHealthAbsence{}
	h := &Handler{healthAbsence: fake}
	router := NewRouter(h, RouterConfig{JWTSecret: "s", InternalKey: "ik", Cache: cache.New("", 0)})
	body := map[string]any{"school_id": "sch", "student_id": "stu", "date": "2026-10-12", "sick_since": "2026-10-12T03:15:00Z",
		"source_ref": "visit-1", "outcome": "sent_home", "actor_user_id": "uks", "reason": "Dipulangkan dari UKS pukul 10.15 WIB"}
	do := func(key string, b map[string]any) *httptest.ResponseRecorder {
		raw, _ := json.Marshal(b)
		req := httptest.NewRequest(http.MethodPost, "/internal/student-absences/health", bytes.NewReader(raw))
		if key != "" {
			req.Header.Set("X-Internal-Key", key)
		}
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		return rec
	}
	if rec := do("", body); rec.Code != http.StatusUnauthorized {
		t.Fatalf("no key: %d", rec.Code)
	}
	if rec := do("wrong", body); rec.Code != http.StatusUnauthorized {
		t.Fatalf("wrong key: %d", rec.Code)
	}
	bad := map[string]any{}
	for k, v := range body {
		bad[k] = v
	}
	bad["outcome"] = "back_to_class"
	if rec := do("ik", bad); rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("back_to_class: %d", rec.Code)
	}
	bad["outcome"], bad["date"] = "sent_home", "12-10-2026"
	if rec := do("ik", bad); rec.Code != http.StatusBadRequest {
		t.Fatalf("bad date: %d", rec.Code)
	}
	rec := do("ik", body)
	if rec.Code != http.StatusOK {
		t.Fatalf("ok: %d %s", rec.Code, rec.Body)
	}
	var env struct {
		Data struct {
			Applied bool `json:"applied"`
		} `json:"data"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &env)
	if !env.Data.Applied || len(fake.calls) != 1 || fake.calls[0].SourceRef != "visit-1" || fake.calls[0].Reason == "" {
		t.Fatalf("store call: %+v env=%+v", fake.calls, env)
	}
}
