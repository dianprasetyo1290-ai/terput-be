package handler

import (
	"context"
	"database/sql"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	"skci/billing/internal/common"
	"skci/billing/internal/model"
)

// ---------------------------------------------------------------------------
// Student receivable card (BUG-130) and unallocated funds (BUG-132)
// ---------------------------------------------------------------------------

var paymentSourceLabel = map[string]string{
	"upload": "Transfer (bukti)", "gateway": "Payment gateway", "cashier": "Kasir", "deposit": "Dari deposit",
}

// studentLedgerEntries collects every receivable and deposit event of one
// student: bills (net after discount, with discount and term detail),
// adjustments, legacy voids, verified payments, reversals/refunds and
// deposit movements.
func (a *App) studentLedgerEntries(ctx context.Context, schoolID, studentID string) ([]model.LedgerEntry, error) {
	var out []model.LedgerEntry
	rows, err := a.DB.QueryContext(ctx,
		`SELECT b.id::text, COALESCE(s.name, b.source_module, 'Tagihan'), b.period_year, b.period_month, b.status,
		        b.base_idr, b.discount_idr, b.net_idr, b.created_at, b.updated_at,
		        COALESCE((SELECT sum(x.delta_idr) FROM bill_adjustments x WHERE x.bill_id = b.id AND x.kind <> 'cancel'), 0),
		        EXISTS (SELECT 1 FROM bill_adjustments x WHERE x.bill_id = b.id AND x.kind = 'cancel'),
		        (SELECT count(*) FROM bill_installments i WHERE i.bill_id = b.id),
		        COALESCE((SELECT string_agg(st.label, ', ' ORDER BY st.sequence) FROM bill_discount_steps st WHERE st.bill_id = b.id), '')
		   FROM student_bills b LEFT JOIN billing_schemes s ON s.id = b.scheme_id
		  WHERE b.school_id = $1 AND b.student_user_id::text = $2`, schoolID, studentID)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var (
			id, name, status, discounts string
			y, m, terms                 int
			base, disc, net, adjSum     int64
			created, updated            time.Time
			cancelled                   bool
		)
		if err := rows.Scan(&id, &name, &y, &m, &status, &base, &disc, &net, &created, &updated, &adjSum, &cancelled, &terms, &discounts); err != nil {
			rows.Close()
			return nil, err
		}
		original := net - adjSum
		desc := fmt.Sprintf("%s %02d/%d", name, m, y)
		if disc > 0 {
			desc += fmt.Sprintf(" (bruto %s, potongan %s", formatRupiah(base), formatRupiah(disc))
			if discounts != "" {
				desc += ": " + discounts
			}
			desc += ")"
		}
		if terms > 0 {
			desc += fmt.Sprintf(" · %d termin", terms)
		}
		out = append(out, model.LedgerEntry{At: created, Kind: model.LedgerBill, Ref: id, BillID: id, Description: desc, DebitIDR: original})
		if status == string(model.BillVoid) && !cancelled {
			out = append(out, model.LedgerEntry{At: updated, Kind: model.LedgerVoid, Ref: id, BillID: id,
				Description: "Tagihan dibatalkan: " + desc, CreditIDR: net})
		}
	}
	rows.Close()

	rows, err = a.DB.QueryContext(ctx,
		`SELECT x.id::text, x.bill_id::text, x.kind, x.delta_idr, x.reason, x.created_at
		   FROM bill_adjustments x JOIN student_bills b ON b.id = x.bill_id
		  WHERE x.school_id = $1 AND b.student_user_id::text = $2`, schoolID, studentID)
	if err != nil {
		return nil, err
	}
	kindLabel := map[string]string{"prorate": "Prorata", "adjust": "Penyesuaian", "cancel": "Pembatalan"}
	for rows.Next() {
		var id, bill, kind, reason string
		var delta int64
		var at time.Time
		if err := rows.Scan(&id, &bill, &kind, &delta, &reason, &at); err != nil {
			rows.Close()
			return nil, err
		}
		d, c := model.SignedLedger(delta)
		label := kindLabel[kind]
		if label == "" {
			label = kind
		}
		out = append(out, model.LedgerEntry{At: at, Kind: model.LedgerAdjustment, Ref: id, BillID: bill,
			Description: strings.TrimSpace(label + ": " + reason), DebitIDR: d, CreditIDR: c})
	}
	rows.Close()

	rows, err = a.DB.QueryContext(ctx,
		`SELECT p.id::text, p.student_bill_id::text, p.source, COALESCE(p.reference, ''), COALESCE(p.bank_name, ''), p.amount_idr, p.verified_at
		   FROM bill_payments p JOIN student_bills b ON b.id = p.student_bill_id
		  WHERE p.school_id = $1 AND b.student_user_id::text = $2 AND p.status = 'verified'`, schoolID, studentID)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var id, bill, source, ref, bank string
		var amount int64
		var at time.Time
		if err := rows.Scan(&id, &bill, &source, &ref, &bank, &amount, &at); err != nil {
			rows.Close()
			return nil, err
		}
		desc := "Pembayaran " + paymentSourceLabel[source]
		if bank != "" {
			desc += " " + bank
		}
		if ref != "" {
			desc += " (" + ref + ")"
		}
		out = append(out, model.LedgerEntry{At: at, Kind: model.LedgerPayment, Ref: id, BillID: bill, Description: desc, CreditIDR: amount})
	}
	rows.Close()

	rows, err = a.DB.QueryContext(ctx,
		`SELECT rv.id::text, p.student_bill_id::text, rv.refund_id IS NOT NULL, rv.reason, rv.amount_idr, rv.created_at
		   FROM bill_payment_reversals rv JOIN bill_payments p ON p.id = rv.payment_id JOIN student_bills b ON b.id = p.student_bill_id
		  WHERE rv.school_id = $1 AND b.student_user_id::text = $2`, schoolID, studentID)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var id, bill, reason string
		var refund bool
		var amount int64
		var at time.Time
		if err := rows.Scan(&id, &bill, &refund, &reason, &amount, &at); err != nil {
			rows.Close()
			return nil, err
		}
		desc := "Koreksi pembayaran: " + reason
		if refund {
			desc = "Refund: " + reason
		}
		out = append(out, model.LedgerEntry{At: at, Kind: model.LedgerReversal, Ref: id, BillID: bill, Description: desc, DebitIDR: amount})
	}
	rows.Close()

	rows, err = a.DB.QueryContext(ctx,
		`SELECT id::text, reason, amount_idr, created_at FROM deposit_movements
		  WHERE school_id = $1 AND student_user_id::text = $2`, schoolID, studentID)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var id, reason string
		var amount int64
		var at time.Time
		if err := rows.Scan(&id, &reason, &amount, &at); err != nil {
			rows.Close()
			return nil, err
		}
		out = append(out, model.LedgerEntry{At: at, Kind: model.LedgerDeposit, Ref: id, Description: "Deposit: " + reason, DepositIDR: amount})
	}
	rows.Close()
	return out, nil
}

// StudentLedger is GET /admin/students/{id}/ledger?from=&to=YYYY-MM-DD&page&per_page:
// the receivable card with running receivable and deposit balances, the
// opening balance before from and the closing balance (BUG-130).
func (a *App) StudentLedger(w http.ResponseWriter, r *http.Request) {
	c, schoolID := common.RequireFinanceViewer(w, r)
	if c == nil {
		return
	}
	studentID := param(r, "id")
	var from, to *time.Time
	for _, p := range []struct {
		key string
		dst **time.Time
		end bool
	}{{"from", &from, false}, {"to", &to, true}} {
		v := strings.TrimSpace(r.URL.Query().Get(p.key))
		if v == "" {
			continue
		}
		d, err := time.ParseInLocation("2006-01-02", v, a.loc())
		if err != nil {
			common.WriteError(w, http.StatusUnprocessableEntity, p.key+" harus berformat YYYY-MM-DD")
			return
		}
		if p.end {
			d = d.AddDate(0, 0, 1).Add(-time.Nanosecond)
		}
		*p.dst = &d
	}
	if from != nil && to != nil && to.Before(*from) {
		common.WriteError(w, http.StatusUnprocessableEntity, "tanggal akhir tidak boleh sebelum tanggal awal")
		return
	}
	entries, err := a.studentLedgerEntries(r.Context(), schoolID, studentID)
	if err != nil {
		writeErr(w, err)
		return
	}
	lines, totals := model.BuildLedger(entries, from, to)
	page, perPage := common.Page(r)
	total := len(lines)
	start := min(common.Offset(page, perPage), total)
	end := min(start+perPage, total)
	common.WriteJSON(w, http.StatusOK, map[string]any{
		"student_user_id": studentID,
		"student_name":    a.userNames(r.Context(), []string{studentID})[studentID],
		"entries":         lines[start:end],
		"totals":          totals,
	}, &common.Meta{Page: page, PerPage: perPage, Total: total})
}

// unallocatedRow is one sum of money received but not applied to a bill.
type unallocatedRow struct {
	Kind          string    `json:"kind"` // deposit | bank_unmatched
	Ref           string    `json:"ref"`
	StudentUserID string    `json:"student_user_id,omitempty"`
	StudentName   string    `json:"student_name,omitempty"`
	Description   string    `json:"description"`
	AmountIDR     int64     `json:"amount_idr"`
	ReceivedAt    time.Time `json:"received_at"`
	AgeDays       int       `json:"age_days"`
	Bucket        string    `json:"bucket"`
}

// UnallocatedFunds is GET /admin/finance/unallocated?kind=&page&per_page:
// student deposits not yet used (aged per inflow, FIFO) and incoming bank
// statement lines not matched to any payment, oldest first (BUG-132).
func (a *App) UnallocatedFunds(w http.ResponseWriter, r *http.Request) {
	c, schoolID := common.RequireFinanceViewer(w, r)
	if c == nil {
		return
	}
	kind := r.URL.Query().Get("kind")
	if kind != "" && kind != "deposit" && kind != "bank_unmatched" {
		common.WriteError(w, http.StatusUnprocessableEntity, "kind harus deposit atau bank_unmatched")
		return
	}
	ctx := r.Context()
	asOf := a.now()
	rows := []unallocatedRow{}
	if kind == "" || kind == "deposit" {
		deps, err := a.unallocatedDeposits(ctx, schoolID)
		if err != nil {
			writeErr(w, err)
			return
		}
		rows = append(rows, deps...)
	}
	if kind == "" || kind == "bank_unmatched" {
		q, err := a.DB.QueryContext(ctx,
			`SELECT id::text, account, posted_on, amount_idr, description FROM bank_statement_lines
			  WHERE school_id = $1 AND amount_idr > 0 AND matched_ref IS NULL`, schoolID)
		if err != nil {
			writeErr(w, err)
			return
		}
		for q.Next() {
			var u unallocatedRow
			var account, desc string
			var posted time.Time
			if err := q.Scan(&u.Ref, &account, &posted, &u.AmountIDR, &desc); err != nil {
				q.Close()
				writeErr(w, err)
				return
			}
			u.Kind = "bank_unmatched"
			u.ReceivedAt = time.Date(posted.Year(), posted.Month(), posted.Day(), 0, 0, 0, 0, a.loc())
			u.Description = strings.TrimSpace(account + " · " + desc)
			rows = append(rows, u)
		}
		q.Close()
	}
	summary := map[string]int64{"deposit": 0, "bank_unmatched": 0}
	for i := range rows {
		rows[i].AgeDays = model.AgeDays(rows[i].ReceivedAt, asOf)
		at := rows[i].ReceivedAt
		rows[i].Bucket = model.AgingBucket(&at, asOf)
		summary[rows[i].Kind] += rows[i].AmountIDR
	}
	sort.SliceStable(rows, func(i, j int) bool { return rows[i].ReceivedAt.Before(rows[j].ReceivedAt) })
	page, perPage := common.Page(r)
	total := len(rows)
	start := min(common.Offset(page, perPage), total)
	end := min(start+perPage, total)
	pageRows := rows[start:end]
	var ids []string
	for _, u := range pageRows {
		if u.StudentUserID != "" {
			ids = append(ids, u.StudentUserID)
		}
	}
	names := a.userNames(ctx, ids)
	for i := range pageRows {
		pageRows[i].StudentName = names[pageRows[i].StudentUserID]
	}
	common.WriteJSON(w, http.StatusOK, map[string]any{
		"rows":    pageRows,
		"summary": map[string]any{"deposit_idr": summary["deposit"], "bank_unmatched_idr": summary["bank_unmatched"], "total_idr": summary["deposit"] + summary["bank_unmatched"]},
	}, &common.Meta{Page: page, PerPage: perPage, Total: total})
}

// unallocatedDeposits splits every positive deposit balance into the
// inflows it still consists of. A balance older than its movement history
// (seeded data) is shown as one lot dated at the balance's last update.
func (a *App) unallocatedDeposits(ctx context.Context, schoolID string) ([]unallocatedRow, error) {
	q, err := a.DB.QueryContext(ctx,
		`SELECT d.student_user_id::text, d.balance_idr, d.updated_at, m.amount_idr, m.created_at
		   FROM student_deposits d
		   LEFT JOIN deposit_movements m ON m.school_id = d.school_id AND m.student_user_id = d.student_user_id
		  WHERE d.school_id = $1 AND d.balance_idr > 0
		  ORDER BY d.student_user_id, m.created_at`, schoolID)
	if err != nil {
		return nil, err
	}
	type acc struct {
		balance int64
		updated time.Time
		moves   []model.DepositMovement
	}
	byStudent := map[string]*acc{}
	var order []string
	for q.Next() {
		var sid string
		var bal int64
		var upd time.Time
		var amt sql.NullInt64
		var at sql.NullTime
		if err := q.Scan(&sid, &bal, &upd, &amt, &at); err != nil {
			q.Close()
			return nil, err
		}
		s := byStudent[sid]
		if s == nil {
			s = &acc{balance: bal, updated: upd}
			byStudent[sid] = s
			order = append(order, sid)
		}
		if amt.Valid && at.Valid {
			s.moves = append(s.moves, model.DepositMovement{At: at.Time, AmountIDR: amt.Int64})
		}
	}
	q.Close()
	var out []unallocatedRow
	for _, sid := range order {
		s := byStudent[sid]
		lots := model.OpenDepositLots(s.moves)
		var sum int64
		for _, l := range lots {
			sum += l.RemainingIDR
		}
		if sum < s.balance {
			lots = append(lots, model.DepositLot{At: s.updated, RemainingIDR: s.balance - sum})
		}
		left := s.balance // never show more than the actual balance
		for _, l := range lots {
			amt := min(l.RemainingIDR, left)
			if amt <= 0 {
				break
			}
			left -= amt
			out = append(out, unallocatedRow{Kind: "deposit", Ref: sid + ":" + l.At.Format(time.RFC3339), StudentUserID: sid,
				Description: "Deposit kelebihan bayar", AmountIDR: amt, ReceivedAt: l.At})
		}
	}
	return out, nil
}
