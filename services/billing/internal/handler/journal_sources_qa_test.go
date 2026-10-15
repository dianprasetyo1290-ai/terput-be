package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"

	"skci/billing/internal/client"
	"skci/billing/internal/common"
	"skci/billing/internal/config"
)

// journalSourcesFixture: one school with two students, a paid bill, an
// unpaid bill, a void bill, a verified transfer and a cashier receipt, and
// the identity stub accounting's journal page needs (name + NISN).
type journalSourcesFixture struct {
	app                        *App
	r                          http.Handler
	schoolID, s1, s2           string
	bill1, bill2, bill3, billV string
	payID, cashID              string
}

func newJournalSourcesFixture(t *testing.T) *journalSourcesFixture {
	db := getTestDB(t)
	t.Cleanup(func() { db.Close() })
	f := &journalSourcesFixture{schoolID: genUUID(), s1: genUUID(), s2: genUUID(),
		bill1: genUUID(), bill2: genUUID(), bill3: genUUID(), billV: genUUID(), payID: genUUID(), cashID: genUUID()}
	schemeID := genUUID()
	mustExec(t, db, `INSERT INTO billing_schemes (id, school_id, academic_year_id, name, status, installment, due_day)
		VALUES ($1, $2, $3, 'SPP QA', 'active', true, 10)`, schemeID, f.schoolID, genUUID())
	bill := func(id, student string, month, net int, status, created string) {
		mustExec(t, db, `INSERT INTO student_bills (id, school_id, student_user_id, scheme_id, period_year, period_month,
			base_idr, discount_idr, net_idr, status, due_at, created_at)
			VALUES ($1, $2, $3, $4, 2026, $5, $6, 0, $6, $7, $8::timestamptz + interval '9 days', $8::timestamptz)`,
			id, f.schoolID, student, schemeID, month, net, status, created)
	}
	bill(f.bill1, f.s1, 9, 750000, "paid", "2026-09-01T05:00:00Z")
	bill(f.bill2, f.s1, 10, 300000, "unpaid", "2026-10-01T05:00:00Z")
	bill(f.bill3, f.s2, 9, 200000, "unpaid", "2026-09-02T05:00:00Z")
	bill(f.billV, f.s2, 8, 999000, "void", "2026-08-01T05:00:00Z")
	mustExec(t, db, `INSERT INTO bill_payments (id, school_id, student_bill_id, amount_idr, transfer_at, reference, object_key, mime, size_bytes, sha256,
		status, idempotency_key, uploader_user_id, verified_at, source)
		VALUES ($1, $2, $3, 750000, '2026-10-04T05:00:00Z', 'TRX-QA-1', 'k', 'image/png', 1, 'x', 'verified', $4, $5, '2026-10-05T05:00:00Z', 'upload')`,
		f.payID, f.schoolID, f.bill1, "bp-"+f.payID, genUUID())
	mustExec(t, db, `INSERT INTO bill_payment_allocations (school_id, payment_id, amount_idr) VALUES ($1, $2, 750000)`, f.schoolID, f.payID)
	mustExec(t, db, `INSERT INTO cashier_payments (id, school_id, student_user_id, method, amount_idr, received_by, note, status, idempotency_key)
		VALUES ($1, $2, $3, 'cash', 100000, $4, 'x', 'recorded', $5)`, f.cashID, f.schoolID, f.s2, genUUID(), "cp-"+f.cashID)
	mustExec(t, db, `INSERT INTO receipts (school_id, receipt_no, source, source_id, sent_at)
		VALUES ($1, $2, 'cashier', $3, now())`, f.schoolID, "KW-JS-"+f.cashID[:8], f.cashID)
	mustExec(t, db, `INSERT INTO billing_journal_settings (school_id, unit_div_code) VALUES ($1, 'SD')`, f.schoolID)

	identity := chi.NewRouter()
	identity.Get("/users/{id}", func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		nisn := "00123" + id[:3]
		name := map[string]string{f.s1: "Budi Santoso", f.s2: "Siti Aminah"}[id]
		common.WriteJSON(w, http.StatusOK, map[string]any{"id": id, "name": name, "email": "", "nisn": nisn}, nil)
	})
	idSrv := httptest.NewServer(identity)
	t.Cleanup(idSrv.Close)

	f.app = New(db, config.Config{}, stubIdentity{}, nil)
	f.app.IdentitySvc = client.New(idSrv.URL, "k")
	r := chi.NewRouter()
	r.Group(func(in chi.Router) {
		in.Use(common.InternalAuth("k"))
		in.Get("/internal/journal-sources", f.app.InternalJournalSources)
		in.Get("/internal/receivables/summary", f.app.InternalReceivablesSummary)
	})
	f.r = r
	return f
}

func (f *journalSourcesFixture) get(t *testing.T, path, key string) (int, map[string]any) {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, path, nil)
	if key != "" {
		req.Header.Set("X-Internal-Key", key)
	}
	rec := httptest.NewRecorder()
	f.r.ServeHTTP(rec, req)
	var env map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &env)
	return rec.Code, env
}

func TestInternalJournalSources(t *testing.T) {
	f := newJournalSourcesFixture(t)
	ref := "bil:rcpt:" + f.schoolID + ":2026-10-05"
	refs := map[string][]string{"student_bills": {f.bill1}, "bill_payments": {f.payID}, "cashier_payments": {f.cashID}}
	mustExec(t, f.app.DB, `INSERT INTO gateway_journal_postings (school_id, reference, trans_date, description, lines, kind, source, source_refs)
		VALUES ($1, $2, '2026-10-05', 'Penerimaan', '[]', 'payment_received', 'billing', $3)`, f.schoolID, ref, jsonRaw(refs))

	q := "/internal/journal-sources?school_id=" + f.schoolID + "&reference=" + ref
	if code, _ := f.get(t, q, ""); code != http.StatusUnauthorized {
		t.Fatalf("without X-Internal-Key: %d, want 401", code)
	}
	if code, _ := f.get(t, "/internal/journal-sources?school_id="+f.schoolID+"&reference=bil:none", "k"); code != http.StatusNotFound {
		t.Fatalf("unknown reference: %d, want 404 (accounting reads it as no sources)", code)
	}
	if code, _ := f.get(t, "/internal/journal-sources?school_id="+genUUID()+"&reference="+ref, "k"); code != http.StatusNotFound {
		t.Fatalf("another school's reference: %d, want 404", code)
	}
	if code, _ := f.get(t, "/internal/journal-sources?school_id="+f.schoolID, "k"); code != http.StatusUnprocessableEntity {
		t.Fatalf("missing reference: %d, want 422", code)
	}

	code, env := f.get(t, q, "k")
	if code != http.StatusOK {
		t.Fatalf("journal-sources: %d %v", code, env)
	}
	d := dataOf(env)
	if d["reference"] != ref {
		t.Fatalf("reference = %v", d["reference"])
	}
	items, _ := d["items"].([]any)
	if len(items) != 3 {
		t.Fatalf("items = %d, want 3: %v", len(items), items)
	}
	byType := map[string]map[string]any{}
	for _, it := range items {
		m := it.(map[string]any)
		for _, k := range []string{"type", "id", "number", "student_name", "nis", "unit", "amount_idr", "date", "status"} {
			if _, ok := m[k]; !ok {
				t.Fatalf("item missing %q: %v", k, m)
			}
		}
		byType[m["type"].(string)] = m
	}
	b := byType["bill"]
	if b == nil || b["id"] != f.bill1 || b["amount_idr"].(float64) != 750000 || b["status"] != "paid" ||
		b["student_name"] != "Budi Santoso" || b["nis"] != "00123"+f.s1[:3] || b["unit"] != "SD" || b["number"] != "2026-09" {
		t.Fatalf("bill item = %v", b)
	}
	p := byType["payment"]
	if p == nil || p["id"] != f.payID || p["number"] != "TRX-QA-1" || p["date"] != "2026-10-05" || p["status"] != "verified" ||
		p["student_name"] != "Budi Santoso" {
		t.Fatalf("payment item = %v", p)
	}
	c := byType["cashier_payment"]
	if c == nil || c["number"] != "KW-JS-"+f.cashID[:8] || c["amount_idr"].(float64) != 100000 || c["student_name"] != "Siti Aminah" {
		t.Fatalf("cashier item = %v", c)
	}

	// A reference with no documents behind it still answers 200 with [].
	ref2 := "bil:issue:" + f.schoolID + ":2026-10-06"
	mustExec(t, f.app.DB, `INSERT INTO gateway_journal_postings (school_id, reference, trans_date, description, lines, kind, source)
		VALUES ($1, $2, '2026-10-06', 'Kosong', '[]', 'bill_issued', 'billing')`, f.schoolID, ref2)
	code, env = f.get(t, "/internal/journal-sources?school_id="+f.schoolID+"&reference="+ref2, "k")
	if items, _ := dataOf(env)["items"].([]any); code != http.StatusOK || items == nil || len(items) != 0 {
		t.Fatalf("empty refs: %d %v", code, env)
	}
}

func TestInternalReceivablesSummary(t *testing.T) {
	f := newJournalSourcesFixture(t)
	base := "/internal/receivables/summary?school_id=" + f.schoolID
	cases := []struct {
		asOf     string
		out      float64
		students float64
	}{
		// Before any bill.
		{"2026-08-31", 0, 0},
		// bill1 (750k, paid only on 10-05) + bill3 (200k); the void bill never counts.
		{"2026-09-30", 950000, 2},
		// bill1 paid; bill2 (300k) + bill3 (200k).
		{"2026-10-10", 500000, 2},
	}
	for _, c := range cases {
		code, env := f.get(t, base+"&as_of="+c.asOf, "k")
		d := dataOf(env)
		if code != http.StatusOK || d["outstanding_idr"] != c.out || d["students"] != c.students || d["as_of"] != c.asOf {
			t.Fatalf("as_of %s: %d %v, want outstanding %v students %v", c.asOf, code, env, c.out, c.students)
		}
	}
	if code, _ := f.get(t, base+"&as_of=30-09-2026", "k"); code != http.StatusUnprocessableEntity {
		t.Fatalf("bad as_of: %d, want 422", code)
	}
	if code, _ := f.get(t, "/internal/receivables/summary", "k"); code != http.StatusUnprocessableEntity {
		t.Fatalf("missing school_id: %d, want 422", code)
	}
	if code, _ := f.get(t, base, ""); code != http.StatusUnauthorized {
		t.Fatalf("without X-Internal-Key: %d, want 401", code)
	}
	if code, env := f.get(t, base, "k"); code != http.StatusOK || dataOf(env)["as_of"] == "" {
		t.Fatalf("default as_of: %d %v", code, env)
	}
}

// TestJournalRequestCarriesSourceRefs: the journal POST to accounting
// carries the outbox source_refs under accounting's type names.
func TestJournalRequestCarriesSourceRefs(t *testing.T) {
	f := newJournalSourcesFixture(t)
	ctx := context.Background()
	id := genUUID()
	refs := map[string][]string{"student_bills": {f.bill1}, "bill_payments": {f.payID}, "cashier_payments": {f.cashID},
		"bill_refunds": {}, "wallet_movements": {"w1"}}
	mustExec(t, f.app.DB, `INSERT INTO gateway_journal_postings (id, school_id, reference, trans_date, description, lines, kind, source, source_refs)
		VALUES ($1, $2, $3, '2026-10-05', 'Penerimaan', '[]', 'payment_received', 'billing', $4)`,
		id, f.schoolID, "bil:rcpt:"+f.schoolID+":x", jsonRaw(refs))
	row, err := loadOutboxRow(ctx, f.app.DB, id, "")
	if err != nil {
		t.Fatal(err)
	}
	req, err := f.app.journalRequest(ctx, row)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string][]string{"bill": {f.bill1}, "payment": {f.payID}, "cashier_payment": {f.cashID}, "wallet_movement": {"w1"}}
	if !reflect.DeepEqual(req.SourceRefs, want) {
		t.Fatalf("source_refs = %v, want %v", req.SourceRefs, want)
	}
	raw, _ := json.Marshal(req)
	if !strings.Contains(string(raw), `"source_refs":{`) || !strings.Contains(string(raw), `"bill":["`+f.bill1+`"]`) {
		t.Fatalf("POST body lacks source_refs: %s", raw)
	}

	// No refs: the optional field is left out of the body.
	id2 := genUUID()
	mustExec(t, f.app.DB, `INSERT INTO gateway_journal_postings (id, school_id, reference, trans_date, description, lines, kind, source)
		VALUES ($1, $2, $3, '2026-10-05', 'x', '[]', 'bill_issued', 'billing')`, id2, f.schoolID, "bil:issue:"+f.schoolID+":y")
	row, err = loadOutboxRow(ctx, f.app.DB, id2, "")
	if err != nil {
		t.Fatal(err)
	}
	req, _ = f.app.journalRequest(ctx, row)
	if raw, _ := json.Marshal(req); strings.Contains(string(raw), "source_refs") {
		t.Fatalf("empty source_refs must be omitted: %s", raw)
	}
}
