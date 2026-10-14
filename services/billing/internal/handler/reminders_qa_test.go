package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	"skci/billing/internal/client"
	"skci/billing/internal/common"
)

// fakeDownstream stands in for identity (guardians), notification and core
// (exceptions) over HTTP, recording what billing sent.
type fakeDownstream struct {
	mu         sync.Mutex
	guardians  map[string][]string // student -> guardians
	failNotify map[string]bool     // guardian -> notification fails
	sent       []string            // Idempotency-Key of delivered notifications
	exceptions []string
}

func (f *fakeDownstream) handler() http.Handler {
	r := chi.NewRouter()
	r.Get("/internal/students/{id}/guardians", func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		var rows []map[string]string
		for _, g := range f.guardians[chi.URLParam(r, "id")] {
			rows = append(rows, map[string]string{"guardian_user_id": g})
		}
		common.WriteJSON(w, http.StatusOK, rows, nil)
	})
	r.Post("/internal/notifications", func(w http.ResponseWriter, r *http.Request) {
		var n client.Notification
		_ = json.NewDecoder(r.Body).Decode(&n)
		f.mu.Lock()
		defer f.mu.Unlock()
		if f.failNotify[n.UserID] {
			common.WriteError(w, http.StatusBadGateway, "smtp down")
			return
		}
		f.sent = append(f.sent, r.Header.Get("Idempotency-Key"))
		common.WriteJSON(w, http.StatusCreated, map[string]string{"id": "n1"}, nil)
	})
	r.Post("/internal/exceptions", func(w http.ResponseWriter, r *http.Request) {
		var e client.ExceptionEvent
		_ = json.NewDecoder(r.Body).Decode(&e)
		f.mu.Lock()
		defer f.mu.Unlock()
		f.exceptions = append(f.exceptions, e.Kind+"|"+e.CorrelationID)
		common.WriteJSON(w, http.StatusCreated, map[string]string{"id": "x1"}, nil)
	})
	return r
}

// TestBillReminders covers BUG-007/173: scheduled H-x reminders reach the
// guardians once, failures retry and then land in core's exception queue,
// and the manual endpoint really sends.
func TestBillReminders(t *testing.T) {
	db := getTestDB(t)
	defer db.Close()
	app, _ := setupTestApp(t, db, genUUID(), genUUID())
	fake := &fakeDownstream{guardians: map[string][]string{}, failNotify: map[string]bool{}}
	ds := httptest.NewServer(fake.handler())
	defer ds.Close()
	app.IdentitySvc = client.New(ds.URL, "k")
	app.Notification = client.New(ds.URL, "k")
	app.Core = client.New(ds.URL, "k")
	wib := time.FixedZone("WIB", 7*3600)
	app.Config.Gateway.Location = wib
	now := time.Date(2026, 9, 7, 9, 0, 0, 0, wib) // H-3 for a bill due 2026-09-10
	app.Clock = func() time.Time { return now }

	r := chi.NewRouter()
	r.Use(common.JWTMiddleware(testSecret))
	r.Put("/admin/billing/reminder-settings", app.PutReminderSettings)
	r.Post("/admin/bills/reminders", app.AdminBillReminders)
	srv := httptest.NewServer(r)
	defer srv.Close()

	schoolID, schemeID := genUUID(), genUUID()
	okStudent, failStudent := genUUID(), genUUID()
	gOK, gFail := genUUID(), genUUID()
	fake.guardians[okStudent] = []string{gOK}
	fake.guardians[failStudent] = []string{gFail}
	fake.failNotify[gFail] = true
	mustExec(t, db, `INSERT INTO billing_schemes (id, school_id, academic_year_id, name, status, installment, due_day)
		VALUES ($1, $2, $3, 'SPP', 'active', true, 10)`, schemeID, schoolID, genUUID())
	for _, st := range []string{okStudent, failStudent} {
		mustExec(t, db, `INSERT INTO student_bills (school_id, student_user_id, scheme_id, period_year, period_month,
			base_idr, discount_idr, net_idr, status, due_at) VALUES ($1, $2, $3, 2026, 9, 500000, 0, 500000, 'unpaid', '2026-09-10T00:00:00+07:00')`,
			schoolID, st, schemeID)
	}
	token := signToken(t, genUUID(), schoolID, common.RoleFinanceManager)
	call := func(method, path string, body any) (int, map[string]any) {
		b, _ := json.Marshal(body)
		req, _ := http.NewRequest(method, srv.URL+path, bytes.NewReader(b))
		req.Header.Set("Authorization", "Bearer "+token)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		raw, _ := io.ReadAll(resp.Body)
		var env map[string]any
		_ = json.Unmarshal(raw, &env)
		return resp.StatusCode, env
	}

	if code, env := call(http.MethodPut, "/admin/billing/reminder-settings", map[string]any{"enabled": true, "days_before": []int{0}, "channels": []string{"in_app"}}); code != http.StatusUnprocessableEntity {
		t.Fatalf("invalid settings status=%d env=%v", code, env)
	}
	if code, env := call(http.MethodPut, "/admin/billing/reminder-settings", map[string]any{
		"enabled": true, "days_before": []int{3}, "on_due": true, "days_after": []int{3}, "send_hour": 8, "channels": []string{"in_app", "email"}}); code != http.StatusOK {
		t.Fatalf("settings status=%d env=%v", code, env)
	}

	ctx := context.Background()
	for i := 0; i < 4; i++ { // run 1 sends, the rest retry only the failing one
		if _, err := app.runScheduledReminders(ctx, schoolID, now); err != nil {
			t.Fatal(err)
		}
	}
	fake.mu.Lock()
	sent, excs := len(fake.sent), append([]string{}, fake.exceptions...)
	fake.mu.Unlock()
	if sent != 1 {
		t.Fatalf("sent=%d want exactly 1 (idempotent)", sent)
	}
	if len(excs) != 1 || !strings.HasPrefix(excs[0], "notification|reminder:auto:") {
		t.Fatalf("exceptions=%v", excs)
	}
	var attempts int
	var status string
	_ = db.QueryRow(`SELECT attempts, status FROM bill_reminder_log WHERE school_id=$1 AND guardian_user_id=$2`, schoolID, gFail).Scan(&attempts, &status)
	if attempts != 3 || status != "failed" {
		t.Fatalf("failing reminder attempts=%d status=%s", attempts, status)
	}

	// Before the send hour nothing goes out.
	early := time.Date(2026, 9, 7, 7, 0, 0, 0, wib)
	if res, _ := app.runScheduledReminders(ctx, schoolID, early); len(res) != 0 {
		t.Fatalf("sent before send_hour: %v", res)
	}

	code, env := call(http.MethodPost, "/admin/bills/reminders", map[string]any{"student_user_ids": []string{okStudent}, "message": "Mohon segera dilunasi."})
	if code != http.StatusOK {
		t.Fatalf("manual status=%d env=%v", code, env)
	}
	if d := env["data"].(map[string]any); d["sent"].(float64) != 1 {
		t.Fatalf("manual result=%v", d)
	}
	if code, _ := call(http.MethodPost, "/admin/bills/reminders", map[string]any{}); code != http.StatusBadRequest {
		t.Fatalf("empty manual status=%d", code)
	}
}
