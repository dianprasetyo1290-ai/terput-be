package handler

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"sort"
	"strings"
	"time"

	"skci/billing/internal/common"
	"skci/billing/internal/model"
)

// Accounting contract (QA-ACC): the journal billing posts carries
// source_refs {"<type>": [ids]}, and accounting asks back for the
// documents behind a reference to render them on /journal/{no}.
//
// The outbox stores source_refs keyed by table name; these are the type
// names accounting sees.
var journalSourceTypes = map[string]string{
	"student_bills":             "bill",
	"bill_payments":             "payment",
	"cashier_payments":          "cashier_payment",
	"bill_refunds":              "refund",
	"wallet_movements":          "wallet_movement",
	"canteen_settlements":       "canteen_settlement",
	"gateway_transactions":      "gateway_transaction",
	"reconciliation_exceptions": "recon_exception",
}

// journalSourceRefs maps the outbox source_refs (table keys) to the
// accounting type names. Unknown keys pass through unchanged; empty lists
// are dropped. It returns nil when nothing is left.
func journalSourceRefs(raw map[string][]string) map[string][]string {
	out := map[string][]string{}
	for k, ids := range raw {
		if len(ids) == 0 {
			continue
		}
		t := journalSourceTypes[k]
		if t == "" {
			t = k
		}
		out[t] = append(out[t], ids...)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

type journalSourceItem struct {
	Type        string `json:"type"`
	ID          string `json:"id"`
	Number      string `json:"number"`
	StudentName string `json:"student_name"`
	NIS         string `json:"nis"`
	Unit        string `json:"unit"`
	AmountIDR   int64  `json:"amount_idr"`
	Date        string `json:"date"`
	Status      string `json:"status"`

	studentID string
}

// journalSourceQueries: one query per source table. Each takes
// ($1 school_id, $2 ids text[]) and returns
// id, number, student_user_id, amount_idr, date, status.
var journalSourceQueries = map[string]string{
	"student_bills": `SELECT b.id::text, COALESCE(NULLIF(b.source_ref,''), to_char(b.period_year,'FM0000')||'-'||to_char(b.period_month,'FM00')),
	        b.student_user_id::text, b.net_idr, b.created_at, b.status
	   FROM student_bills b WHERE b.school_id = $1 AND b.id::text = ANY($2::text[])`,
	"bill_payments": `SELECT p.id::text, COALESCE((SELECT r.receipt_no FROM receipts r WHERE r.school_id = p.school_id AND r.source_id::text = p.source_id::text LIMIT 1), NULLIF(p.reference,''), ''),
	        b.student_user_id::text, p.amount_idr, COALESCE(p.verified_at, p.created_at), p.status
	   FROM bill_payments p JOIN student_bills b ON b.id = p.student_bill_id
	  WHERE p.school_id = $1 AND p.id::text = ANY($2::text[])`,
	"cashier_payments": `SELECT c.id::text, COALESCE((SELECT r.receipt_no FROM receipts r WHERE r.school_id = c.school_id AND r.source = 'cashier' AND r.source_id::text = c.id::text LIMIT 1), ''),
	        c.student_user_id::text, c.amount_idr, c.created_at, c.status
	   FROM cashier_payments c WHERE c.school_id = $1 AND c.id::text = ANY($2::text[])`,
	"bill_refunds": `SELECT f.id::text, '', f.student_user_id::text, f.amount_idr, COALESCE(f.processed_at, f.created_at), f.status
	   FROM bill_refunds f WHERE f.school_id = $1 AND f.id::text = ANY($2::text[])`,
	"wallet_movements": `SELECT m.id::text, COALESCE((SELECT r.receipt_no FROM receipts r WHERE r.school_id = m.school_id AND r.source = 'wallet' AND r.source_id::text = m.ref_id::text LIMIT 1), m.kind),
	        w.student_user_id::text, m.amount_idr, m.created_at, m.kind
	   FROM wallet_movements m JOIN wallets w ON w.id = m.wallet_id
	  WHERE m.school_id = $1 AND m.id::text = ANY($2::text[])`,
	"gateway_transactions": `SELECT g.id::text, g.tx_no, COALESCE(g.student_user_id::text,''), g.total_idr, COALESCE(g.paid_at, g.created_at), g.status
	   FROM gateway_transactions g WHERE g.school_id = $1 AND g.id::text = ANY($2::text[])`,
	"canteen_settlements": `SELECT s.id::text, COALESCE(t.name,''), '', s.tenant_payout_idr, COALESCE(s.paid_at, s.created_at), s.status
	   FROM canteen_settlements s LEFT JOIN canteen_tenants t ON t.id = s.tenant_id
	  WHERE s.school_id = $1 AND s.id::text = ANY($2::text[])`,
	"reconciliation_exceptions": `SELECT e.id::text, e.ref, '', 0::bigint, e.created_at, e.status
	   FROM reconciliation_exceptions e WHERE e.school_id = $1 AND e.id::text = ANY($2::text[])`,
}

// InternalJournalSources: GET /internal/journal-sources?school_id=&reference=
// lists the billing documents behind one journal reference. 404 when the
// reference is not billing's (accounting reads that as "no sources").
func (a *App) InternalJournalSources(w http.ResponseWriter, r *http.Request) {
	schoolID := strings.TrimSpace(r.URL.Query().Get("school_id"))
	reference := strings.TrimSpace(r.URL.Query().Get("reference"))
	if schoolID == "" || reference == "" {
		common.WriteError(w, http.StatusUnprocessableEntity, "school_id and reference are required")
		return
	}
	ctx := r.Context()
	var rawRefs []byte
	err := a.DB.QueryRowContext(ctx,
		`SELECT COALESCE(source_refs, '{}'::jsonb) FROM gateway_journal_postings WHERE school_id::text = $1 AND reference = $2`,
		schoolID, reference).Scan(&rawRefs)
	if err == sql.ErrNoRows {
		common.WriteError(w, http.StatusNotFound, "journal reference not found")
		return
	}
	if err != nil {
		writeErr(w, err)
		return
	}
	refs := map[string][]string{}
	if err := json.Unmarshal(rawRefs, &refs); err != nil {
		writeErr(w, err)
		return
	}

	unit := ""
	if set, err := a.loadJournalSettings(ctx, schoolID); err == nil {
		unit = set.UnitDivCode
	}
	if unit == "" && a.IdentitySvc.Configured() {
		if n, err := a.IdentitySvc.SchoolName(ctx, schoolID); err == nil {
			unit = n
		}
	}

	keys := make([]string, 0, len(refs))
	for k := range refs {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	items := []journalSourceItem{}
	for _, table := range keys {
		q, ok := journalSourceQueries[table]
		if !ok || len(refs[table]) == 0 {
			continue
		}
		got, err := a.loadJournalSourceItems(ctx, q, journalSourceTypes[table], schoolID, refs[table])
		if err != nil {
			writeErr(w, err)
			return
		}
		items = append(items, got...)
	}

	names := map[string][2]string{}
	for i := range items {
		items[i].Unit = unit
		sid := items[i].studentID
		if sid == "" {
			continue
		}
		n, ok := names[sid]
		if !ok {
			n = [2]string{sid, ""}
			if a.IdentitySvc.Configured() {
				if u, err := a.IdentitySvc.User(ctx, sid); err == nil {
					if u.Name != "" {
						n[0] = u.Name
					}
					if u.NISN != nil {
						n[1] = *u.NISN
					}
				}
			}
			names[sid] = n
		}
		items[i].StudentName, items[i].NIS = n[0], n[1]
	}
	common.WriteJSON(w, http.StatusOK, map[string]any{"reference": reference, "items": items}, nil)
}

func (a *App) loadJournalSourceItems(ctx context.Context, q, typ, schoolID string, ids []string) ([]journalSourceItem, error) {
	rows, err := a.DB.QueryContext(ctx, q, schoolID, ids)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []journalSourceItem
	for rows.Next() {
		var it journalSourceItem
		var at time.Time
		if err := rows.Scan(&it.ID, &it.Number, &it.studentID, &it.AmountIDR, &at, &it.Status); err != nil {
			return nil, err
		}
		it.Type = typ
		if it.Number == "" {
			it.Number = it.ID
		}
		it.Date = at.In(a.location()).Format("2006-01-02")
		out = append(out, it)
	}
	return out, rows.Err()
}

// InternalReceivablesSummary: GET /internal/receivables/summary?school_id=&as_of=
// is the receivable control total for accounting (menu 059): what students
// still owe on non-void bills issued up to as_of, counting only payments
// verified and reversals made up to as_of.
func (a *App) InternalReceivablesSummary(w http.ResponseWriter, r *http.Request) {
	schoolID := strings.TrimSpace(r.URL.Query().Get("school_id"))
	if schoolID == "" {
		common.WriteError(w, http.StatusUnprocessableEntity, "school_id is required")
		return
	}
	loc := a.location()
	asOf := time.Now().In(loc)
	if s := strings.TrimSpace(r.URL.Query().Get("as_of")); s != "" {
		d, err := time.ParseInLocation("2006-01-02", s, loc)
		if err != nil {
			common.WriteError(w, http.StatusUnprocessableEntity, "as_of must be YYYY-MM-DD")
			return
		}
		asOf = d
	}
	day := asOf.Format("2006-01-02")
	end := time.Date(asOf.Year(), asOf.Month(), asOf.Day(), 0, 0, 0, 0, loc).AddDate(0, 0, 1)

	rows, err := a.DB.QueryContext(r.Context(),
		`SELECT b.student_user_id::text, b.net_idr,
		        COALESCE((SELECT sum(al.amount_idr) FROM bill_payment_allocations al JOIN bill_payments p ON p.id = al.payment_id
		                  WHERE p.student_bill_id = b.id AND p.school_id = b.school_id AND p.status = 'verified' AND p.verified_at < $2), 0),
		        COALESCE((SELECT sum(rv.amount_idr) FROM bill_payment_reversals rv JOIN bill_payments p ON p.id = rv.payment_id
		                  WHERE p.student_bill_id = b.id AND p.school_id = b.school_id AND rv.created_at < $2), 0)
		   FROM student_bills b
		  WHERE b.school_id::text = $1 AND b.status <> 'void' AND b.created_at < $2`,
		schoolID, end)
	if err != nil {
		writeErr(w, err)
		return
	}
	defer rows.Close()
	var total int64
	students := map[string]bool{}
	for rows.Next() {
		var sid string
		var net, verified, reversed int64
		if err := rows.Scan(&sid, &net, &verified, &reversed); err != nil {
			writeErr(w, err)
			return
		}
		out, err := model.Outstanding(net, []int64{verified}, []int64{reversed})
		if err != nil {
			// Over-allocation on a historic date: nothing is owed on it.
			continue
		}
		if out > 0 {
			total += out
			students[sid] = true
		}
	}
	if err := rows.Err(); err != nil {
		writeErr(w, err)
		return
	}
	common.WriteJSON(w, http.StatusOK, map[string]any{
		"outstanding_idr": total, "students": len(students), "as_of": day,
	}, nil)
}
