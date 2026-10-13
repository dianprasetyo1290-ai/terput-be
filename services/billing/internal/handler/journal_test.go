package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	"skci/billing/internal/client"
	"skci/billing/internal/model"
)

// fakeAccounting mimics accounting POST /api/internal/journals: it answers
// fail (status) while failing is set, else 201 {status, trans_no}, and is
// idempotent on (source, reference) like AutoJournalService.
type fakeAccounting struct {
	mu     sync.Mutex
	fail   int
	bodies []client.JournalRequest
	posted map[string]string
	srv    *httptest.Server
}

func newFakeAccounting(t *testing.T) *fakeAccounting {
	fa := &fakeAccounting{posted: map[string]string{}}
	fa.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		var req client.JournalRequest
		_ = json.Unmarshal(raw, &req)
		fa.mu.Lock()
		defer fa.mu.Unlock()
		fa.bodies = append(fa.bodies, req)
		w.Header().Set("Content-Type", "application/json")
		if fa.fail != 0 {
			w.WriteHeader(fa.fail)
			_, _ = w.Write([]byte(`{"status":` + strconv.Itoa(fa.fail) + `,"trans_no":"","message":"gagal"}`))
			return
		}
		key := req.Source + "|" + req.Reference
		no, ok := fa.posted[key]
		status := http.StatusOK
		if !ok {
			no = fmt.Sprintf("BIL-2026-%05d", len(fa.posted)+1)
			fa.posted[key] = no
			status = http.StatusCreated
		}
		w.WriteHeader(status)
		_ = json.NewEncoder(w).Encode(map[string]any{"status": status, "trans_no": no})
	}))
	t.Cleanup(fa.srv.Close)
	return fa
}

func (fa *fakeAccounting) setFail(status int) {
	fa.mu.Lock()
	fa.fail = status
	fa.mu.Unlock()
}

// calls returns the requests for one school (the sender is global and the
// test database is shared with other tests).
func (fa *fakeAccounting) calls(schoolID string) []client.JournalRequest {
	fa.mu.Lock()
	defer fa.mu.Unlock()
	var out []client.JournalRequest
	for _, b := range fa.bodies {
		if b.SchoolID == schoolID {
			out = append(out, b)
		}
	}
	return out
}

func (f *extFixture) internalRouter() *httptest.Server {
	r := chi.NewRouter()
	r.Post("/internal/journal-postings/{id}/retry", f.app.InternalRetryJournalPosting)
	r.Post("/internal/journal-queue/retry", f.app.InternalRetryJournalQueue)
	srv := httptest.NewServer(r)
	f.t.Cleanup(srv.Close)
	return srv
}

func postStatus(t *testing.T, u string) (int, string) {
	t.Helper()
	resp, err := http.Post(u, "application/json", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, string(b)
}

func (f *extFixture) outboxRow(reference string) string {
	id := genUUID()
	lines := []model.JournalLine{
		{AccCode: "1-1300", Description: "Piutang SPP", DebitIDR: 250000},
		{AccCode: "4-1100", Description: "Pendapatan SPP", CreditIDR: 250000},
	}
	f.exec(`INSERT INTO gateway_journal_postings (id, school_id, reference, trans_date, description, lines, kind, source)
	        VALUES ($1, $2, $3, '2026-10-10', 'Tagihan terbit 2026-10-10', $4, 'bill_issued', 'billing')`,
		id, f.schoolID, reference, jsonRaw(lines))
	return id
}

func TestJournalOutboxRetriesWithBackoffThenRaisesException(t *testing.T) {
	f := newExtFixture(t)
	fa := newFakeAccounting(t)
	f.app.Accounting = client.New(fa.srv.URL, "k")
	internal := f.internalRouter()
	f.app.Config.Gateway.BillingInternalURL = internal.URL
	f.exec(`INSERT INTO billing_journal_settings (school_id, unit_div_code) VALUES ($1, 'SD')`, f.schoolID)
	ref := "bil:issue:" + f.schoolID + ":2026-10-10"
	id := f.outboxRow(ref)
	ctx := context.Background()

	fa.setFail(http.StatusServiceUnavailable)
	now := time.Now()
	for i := 1; i <= 5; i++ {
		if err := f.app.sendJournalOutbox(ctx, now); err != nil {
			t.Fatal(err)
		}
		// Not due yet: a second run inside the backoff must not post again.
		if err := f.app.sendJournalOutbox(ctx, now.Add(time.Second)); err != nil {
			t.Fatal(err)
		}
		if n := len(fa.calls(f.schoolID)); n != i {
			t.Fatalf("attempt %d: accounting called %d times", i, n)
		}
		st := f.scalar(`SELECT status || ':' || attempts FROM gateway_journal_postings WHERE id = $1`, id)
		want := "pending:"
		if i == 5 {
			want = "failed:"
		}
		if st != want+string(rune('0'+i)) {
			t.Fatalf("after attempt %d status=%s", i, st)
		}
		if i < 5 {
			var next time.Time
			if err := f.db.QueryRow(`SELECT next_attempt_at FROM gateway_journal_postings WHERE id = $1`, id).Scan(&next); err != nil {
				t.Fatal(err)
			}
			if got := next.Sub(now); got != model.JournalBackoff(i) {
				t.Fatalf("attempt %d backoff %s want %s", i, got, model.JournalBackoff(i))
			}
			now = next
		}
	}
	if n := f.core.count("/internal/exceptions", `"idempotency_key":"journal-post:`+ref+`"`); n != 1 {
		t.Fatalf("exception raised %d times, want 1", n)
	}
	exc := f.core.get("/internal/exceptions")[len(f.core.get("/internal/exceptions"))-1]
	var ev client.ExceptionEvent
	_ = json.Unmarshal([]byte(exc), &ev)
	if ev.Kind != "journal_posting" || ev.MaxAttempts != 5 || ev.RetryURL != internal.URL+"/internal/journal-postings/"+id+"/retry" {
		t.Fatalf("exception %+v", ev)
	}
	if f.scalar(`SELECT exception_raised_at IS NOT NULL FROM gateway_journal_postings WHERE id = $1`, id) != "true" {
		t.Fatal("exception_raised_at not stored")
	}
	// Failed rows are left to the exception queue.
	if err := f.app.sendJournalOutbox(ctx, now.Add(24*time.Hour)); err != nil {
		t.Fatal(err)
	}
	if len(fa.calls(f.schoolID)) != 5 {
		t.Fatal("failed row must not be retried automatically after max attempts")
	}

	// Retry from the exception queue while accounting is still down: 502, still failed.
	if code, _ := postStatus(t, ev.RetryURL); code != http.StatusBadGateway {
		t.Fatalf("retry while down=%d want 502", code)
	}
	fa.setFail(0)
	code, body := postStatus(t, ev.RetryURL)
	if code != http.StatusOK || !strings.Contains(body, `"status":"posted"`) {
		t.Fatalf("retry=%d %s", code, body)
	}
	transNo := f.scalar(`SELECT trans_no FROM gateway_journal_postings WHERE id = $1 AND status = 'posted' AND posted_at IS NOT NULL`, id)
	if transNo == "" {
		t.Fatal("trans_no not stored (IF-ACC-02)")
	}
	last := fa.calls(f.schoolID)[len(fa.calls(f.schoolID))-1]
	if last.Source != "billing" || last.Reference != ref || last.TransDate != "2026-10-10" || len(last.Lines) != 2 ||
		last.Lines[0].DivCode == nil || *last.Lines[0].DivCode != "SD" {
		t.Fatalf("payload %+v", last)
	}
	// A second retry is a no-op returning the stored number.
	before := len(fa.calls(f.schoolID))
	if code, body := postStatus(t, ev.RetryURL); code != http.StatusOK || !strings.Contains(body, transNo) || len(fa.calls(f.schoolID)) != before {
		t.Fatalf("idempotent retry=%d %s", code, body)
	}
}

func TestJournalOutboxValidationErrorFailsAtOnce(t *testing.T) {
	f := newExtFixture(t)
	fa := newFakeAccounting(t)
	f.app.Accounting = client.New(fa.srv.URL, "k")
	ref := "bil:issue:" + f.schoolID + ":2026-10-09"
	id := f.outboxRow(ref)
	fa.setFail(http.StatusUnprocessableEntity)
	if err := f.app.sendJournalOutbox(context.Background(), time.Now()); err != nil {
		t.Fatal(err)
	}
	if st := f.scalar(`SELECT status || ':' || attempts FROM gateway_journal_postings WHERE id = $1`, id); st != "failed:1" {
		t.Fatalf("status=%s want failed:1", st)
	}
	if f.core.count("/internal/exceptions", "journal-post:"+ref) != 1 {
		t.Fatal("validation failure must raise an exception")
	}
}

func TestJournalOutboxWaitsWithoutAccounting(t *testing.T) {
	f := newExtFixture(t)
	id := f.outboxRow("bil:issue:" + f.schoolID + ":2026-10-08")
	if err := f.app.sendJournalOutbox(context.Background(), time.Now()); err != nil {
		t.Fatal(err)
	}
	if st := f.scalar(`SELECT status || ':' || attempts FROM gateway_journal_postings WHERE id = $1`, id); st != "pending:0" {
		t.Fatalf("status=%s: attempts must not burn while accounting is unconfigured", st)
	}
}

// billingDay seeds one day (2026-10-10 WIB) of billing activity: a bill with
// two components and a scholarship, a cash payment allocated to both, a
// processed refund and a wallet cash top-up.
func (f *extFixture) billingDay(t *testing.T) {
	at := func(hm string) time.Time { return wib(t, "2026-10-10 "+hm) }
	bill := genUUID()
	f.exec(`INSERT INTO student_bills (id, school_id, student_user_id, scheme_id, period_year, period_month, base_idr, discount_idr, net_idr, status, created_at)
	        VALUES ($1, $2, $3, $4, 2026, 10, 1300000, 100000, 1200000, 'partial', $5)`, bill, f.schoolID, f.studentID, f.schemeID, at("08:00"))
	spp, gedung := genUUID(), genUUID()
	f.exec(`INSERT INTO bill_items (id, school_id, bill_id, name, component_code, amount_idr, discount_idr, net_idr, created_at)
	        VALUES ($1, $3, $4, 'SPP Oktober', 'SPP', 300000, 0, 300000, $5), ($2, $3, $4, 'Uang Gedung', 'GEDUNG', 1000000, 0, 1000000, $5)`,
		spp, gedung, f.schoolID, bill, at("08:00"))
	disc := genUUID()
	f.exec(`INSERT INTO bill_discounts (id, school_id, name, kind, value, scholarship) VALUES ($1, $2, 'Beasiswa Prestasi', 'nominal', 100000, true)`, disc, f.schoolID)
	f.exec(`INSERT INTO bill_discount_steps (school_id, bill_id, discount_id, sequence, source, label, priority, base_before_idr, requested_idr, applied_idr, base_after_idr, created_at)
	        VALUES ($1, $2, $3, 1, 'scholarship', 'Beasiswa Prestasi', 1, 1300000, 100000, 100000, 1200000, $4)`, f.schoolID, bill, disc, at("08:00"))
	cp := genUUID()
	f.exec(`INSERT INTO cashier_payments (id, school_id, student_user_id, method, amount_idr, received_by, idempotency_key, created_at)
	        VALUES ($1, $2, $3, 'cash', 500000, $4, $5, $6)`, cp, f.schoolID, f.studentID, genUUID(), "cp-"+cp, at("10:00"))
	pay := genUUID()
	f.exec(`INSERT INTO bill_payments (id, school_id, student_bill_id, amount_idr, status, idempotency_key, uploader_user_id, verified_at, source, source_id, created_at)
	        VALUES ($1, $2, $3, 500000, 'verified', $4, $5, $6, 'cashier', $7, $6)`, pay, f.schoolID, bill, "bp-"+pay, genUUID(), at("10:00"), cp)
	f.exec(`INSERT INTO bill_payment_allocations (school_id, payment_id, bill_item_id, amount_idr) VALUES ($1, $2, $3, 300000), ($1, $2, $4, 200000)`,
		f.schoolID, pay, spp, gedung)
	f.exec(`INSERT INTO bill_refunds (school_id, student_bill_id, student_user_id, amount_idr, reason, status, requested_by, decided_by, decided_at, processed_by, processed_at)
	        VALUES ($1, $2, $3, 50000, 'Kelebihan bayar', 'processed', $4, $5, $6, $5, $6)`, f.schoolID, bill, f.studentID, genUUID(), genUUID(), at("11:00"))
	wal := genUUID()
	f.exec(`INSERT INTO wallets (id, school_id, student_user_id, kind, balance_idr) VALUES ($1, $2, $3, 'allowance', 100000)`, wal, f.schoolID, f.studentID)
	f.exec(`INSERT INTO wallet_movements (school_id, wallet_id, kind, amount_idr, balance_after_idr, idempotency_key, created_at)
	        VALUES ($1, $2, 'topup_cashier', 100000, 100000, $3, $4)`, f.schoolID, wal, "wm-"+wal, at("12:00"))
}

func (f *extFixture) assertBalanced(reference string) []model.JournalLine {
	f.t.Helper()
	var lines []model.JournalLine
	if err := json.Unmarshal([]byte(f.scalar(`SELECT lines::text FROM gateway_journal_postings WHERE reference = $1`, reference)), &lines); err != nil {
		f.t.Fatalf("%s: %v", reference, err)
	}
	if err := model.ValidateJournal(lines); err != nil {
		f.t.Fatalf("%s: %v (%+v)", reference, err, lines)
	}
	return lines
}

func TestBillingJournalsQueuedIdempotentAndBalanced(t *testing.T) {
	f := newExtFixture(t)
	internal := f.internalRouter()
	f.app.Config.Gateway.BillingInternalURL = internal.URL
	f.billingDay(t)
	ctx := context.Background()
	now := wib(t, "2026-10-11 01:00")
	day := ":" + f.schoolID + ":2026-10-10"
	refs := map[string]string{"bill_issued": "bil:issue" + day, "discount": "bil:disc" + day, "receipt": "bil:rcpt" + day,
		"refund": "bil:rfnd" + day, "wallet": "wal" + day}

	// No mapping: nothing guessed, one exception per journal per day.
	for i := 0; i < 2; i++ {
		if err := f.app.runBillingJournals(ctx, now); err != nil {
			t.Fatal(err)
		}
	}
	if n := f.scalar(`SELECT count(*)::text FROM gateway_journal_postings WHERE school_id = $1`, f.schoolID); n != "0" {
		t.Fatalf("journals queued without mapping: %s", n)
	}
	for kind, ref := range refs {
		if n := f.core.count("/internal/exceptions", `"idempotency_key":"journal-queue:`+ref+`"`); n != 1 {
			t.Fatalf("%s: exception raised %d times, want 1", kind, n)
		}
	}
	retry := internal.URL + "/internal/journal-queue/retry?reference=" + url.QueryEscape(refs["bill_issued"])
	if code, body := postStatus(t, retry); code != http.StatusUnprocessableEntity || !strings.Contains(body, "belum dipetakan") {
		t.Fatalf("rebuild without mapping=%d %s", code, body)
	}

	f.exec(`INSERT INTO billing_journal_settings (school_id, unit_div_code, receivable_acc, cash_acc, bank_acc, discount_acc, scholarship_acc, refund_acc, deposit_acc)
	        VALUES ($1, 'SD', '1-1300', '1-1100', '1-1200', '5-1100', '5-1200', '4-1900', '2-1500')`, f.schoolID)
	f.exec(`INSERT INTO fee_component_accounts (school_id, component_code, name, revenue_acc) VALUES ($1, 'SPP', 'SPP', '4-1100'), ($1, 'GEDUNG', 'Uang Gedung', '4-1200')`, f.schoolID)
	f.exec(`INSERT INTO wallet_accounts (school_id, allowance_liability_acc, savings_liability_acc, cash_acc, tenant_payable_acc, canteen_revenue_acc)
	        VALUES ($1, '2-2100', '2-2200', '1-1100', '2-2300', '4-3100')`, f.schoolID)

	if code, body := postStatus(t, retry); code != http.StatusOK {
		t.Fatalf("rebuild with mapping=%d %s", code, body)
	}
	for i := 0; i < 2; i++ {
		if err := f.app.runBillingJournals(ctx, now); err != nil {
			t.Fatal(err)
		}
	}
	for kind, ref := range refs {
		if n := f.scalar(`SELECT count(*)::text FROM gateway_journal_postings WHERE reference = $1 AND kind = $2`, ref, kind); n != "1" {
			t.Fatalf("%s: %s rows, want exactly 1", ref, n)
		}
		f.assertBalanced(ref)
	}
	sum := func(lines []model.JournalLine, acc string) (d, c int64) {
		for _, l := range lines {
			if l.AccCode == acc {
				d += l.DebitIDR
				c += l.CreditIDR
			}
		}
		return
	}
	if _, c := sum(f.assertBalanced(refs["bill_issued"]), "4-1200"); c != 1000000 {
		t.Fatalf("revenue Uang Gedung credit %d", c)
	}
	if d, _ := sum(f.assertBalanced(refs["discount"]), "5-1200"); d != 100000 {
		t.Fatalf("scholarship debit %d", d)
	}
	if d, _ := sum(f.assertBalanced(refs["receipt"]), "1-1100"); d != 500000 {
		t.Fatalf("cash debit %d", d)
	}
	if d, _ := sum(f.assertBalanced(refs["refund"]), "4-1900"); d != 50000 {
		t.Fatalf("refund debit %d", d)
	}
	if _, c := sum(f.assertBalanced(refs["wallet"]), "2-2100"); c != 100000 {
		t.Fatalf("wallet liability credit %d", c)
	}
	if !strings.Contains(f.scalar(`SELECT source_refs::text FROM gateway_journal_postings WHERE reference = $1`, refs["receipt"]), "cashier_payments") {
		t.Fatal("receipt journal must reference its detail transactions (BR-ACC-06)")
	}

	// Send everything: one accounting post per journal, all with trans_no.
	fa := newFakeAccounting(t)
	f.app.Accounting = client.New(fa.srv.URL, "k")
	for i := 0; i < 2; i++ {
		if err := f.app.sendJournalOutbox(ctx, time.Now()); err != nil {
			t.Fatal(err)
		}
	}
	if n := len(fa.calls(f.schoolID)); n != len(refs) {
		t.Fatalf("accounting called %d times, want %d", n, len(refs))
	}
	if n := f.scalar(`SELECT count(*)::text FROM gateway_journal_postings WHERE school_id = $1 AND status = 'posted' AND trans_no <> ''`, f.schoolID); n != "5" {
		t.Fatalf("posted with trans_no: %s", n)
	}
}
