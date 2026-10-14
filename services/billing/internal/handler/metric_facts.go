package handler

import (
	"database/sql"
	"fmt"
	"net/http"
	"strings"
	"time"

	"skci/billing/internal/common"
)

// InternalMetricFacts handles GET /internal/metric-facts for billing metrics:
// collection_rate, outstanding_idr, billed_idr, paid_idr.
func (a *App) InternalMetricFacts(w http.ResponseWriter, r *http.Request) {
	schoolID := strings.TrimSpace(r.URL.Query().Get("school_id"))
	if schoolID == "" {
		common.WriteError(w, http.StatusUnprocessableEntity, "school_id is required")
		return
	}
	metric := strings.TrimSpace(r.URL.Query().Get("metric"))
	if metric == "" {
		common.WriteError(w, http.StatusUnprocessableEntity, "metric is required")
		return
	}
	fromStr := strings.TrimSpace(r.URL.Query().Get("from"))
	toStr := strings.TrimSpace(r.URL.Query().Get("to"))
	groupBy := strings.TrimSpace(r.URL.Query().Get("group_by"))

	ctx := r.Context()
	now := time.Now()

	// Query bills for the school
	// If from/to are specified, we parse year/month or filter by date
	query := `SELECT b.student_user_id::text, b.period_year, b.period_month, b.net_idr, b.due_at,
	                 COALESCE((SELECT sum(al.amount_idr) FROM bill_payment_allocations al JOIN bill_payments p ON p.id=al.payment_id
	                           WHERE p.student_bill_id=b.id AND p.status='verified'), 0)
	               - COALESCE((SELECT sum(rv.amount_idr) FROM bill_payment_reversals rv JOIN bill_payments p ON p.id=rv.payment_id
	                           WHERE p.student_bill_id=b.id), 0)
	          FROM student_bills b
	          WHERE b.school_id = $1 AND b.status <> 'void'`

	args := []interface{}{schoolID}
	if fromStr != "" {
		query += " AND b.due_at >= $2"
		args = append(args, fromStr)
	}
	if toStr != "" {
		query += fmt.Sprintf(" AND b.due_at <= $%d", len(args)+1)
		args = append(args, toStr)
	}

	rows, err := a.DB.QueryContext(ctx, query, args...)
	if err != nil {
		writeErr(w, err)
		return
	}
	defer rows.Close()

	var totalBilled, totalPaid, totalOutstanding int64
	aging := map[string]*struct {
		Count int64
		IDR   int64
	}{
		"current":  {},
		"d1_30":    {},
		"d31_60":   {},
		"d61_90":   {},
		"d90_plus": {},
	}
	monthlyBilled := make(map[int]int64)
	monthlyPaid := make(map[int]int64)

	for rows.Next() {
		var (
			student string
			y, m    int
			net, pd int64
			dueAt   *time.Time
		)
		if err := rows.Scan(&student, &y, &m, &net, &dueAt, &pd); err != nil {
			writeErr(w, err)
			return
		}

		out := max(net-pd, 0)
		totalBilled += net
		totalPaid += pd
		totalOutstanding += out

		monthlyBilled[m] += net
		monthlyPaid[m] += pd

		if out > 0 {
			key := "current"
			if dueAt != nil && now.After(*dueAt) {
				switch days := int(now.Sub(*dueAt).Hours() / 24); {
				case days <= 30:
					key = "d1_30"
				case days <= 60:
					key = "d31_60"
				case days <= 90:
					key = "d61_90"
				default:
					key = "d90_plus"
				}
			}
			aging[key].Count++
			aging[key].IDR += out
		}
	}

	type factRow struct {
		DimValue    string `json:"dim_value"`
		Numerator   *int64 `json:"numerator,omitempty"`
		Denominator *int64 `json:"denominator,omitempty"`
		Value       *int64 `json:"value,omitempty"`
	}

	resp := map[string]interface{}{
		"metric": metric,
		"as_of":  now,
	}

	switch metric {
	case "collection_rate":
		resp["unit"] = "percent_bp"
		resp["numerator"] = totalPaid
		resp["denominator"] = totalBilled
		var rateBP int64 = 0
		if totalBilled > 0 {
			rateBP = (totalPaid * 10000) / totalBilled
		}
		resp["value"] = rateBP

		if groupBy == "month" {
			var fRows []factRow
			for m := 1; m <= 12; m++ {
				b := monthlyBilled[m]
				p := monthlyPaid[m]
				var rVal int64 = 0
				if b > 0 {
					rVal = (p * 10000) / b
				}
				pCopy, bCopy, rValCopy := p, b, rVal
				fRows = append(fRows, factRow{
					DimValue:    fmt.Sprintf("%02d", m),
					Numerator:   &pCopy,
					Denominator: &bCopy,
					Value:       &rValCopy,
				})
			}
			resp["rows"] = fRows
		}

	case "outstanding_idr":
		resp["unit"] = "idr"
		resp["value"] = totalOutstanding
		if groupBy == "aging_bucket" {
			buckets := []string{"current", "d1_30", "d31_60", "d61_90", "d90_plus"}
			var fRows []factRow
			for _, b := range buckets {
				amt := aging[b].IDR
				amtCopy := amt
				fRows = append(fRows, factRow{
					DimValue: b,
					Value:    &amtCopy,
				})
			}
			resp["rows"] = fRows
		}

	case "billed_idr":
		resp["unit"] = "idr"
		resp["value"] = totalBilled
		if groupBy == "month" {
			var fRows []factRow
			for m := 1; m <= 12; m++ {
				b := monthlyBilled[m]
				bCopy := b
				fRows = append(fRows, factRow{
					DimValue: fmt.Sprintf("%02d", m),
					Value:    &bCopy,
				})
			}
			resp["rows"] = fRows
		}

	case "paid_idr":
		resp["unit"] = "idr"
		resp["value"] = totalPaid
		if groupBy == "month" {
			var fRows []factRow
			for m := 1; m <= 12; m++ {
				p := monthlyPaid[m]
				pCopy := p
				fRows = append(fRows, factRow{
					DimValue: fmt.Sprintf("%02d", m),
					Value:    &pCopy,
				})
			}
			resp["rows"] = fRows
		}

	default:
		common.WriteError(w, http.StatusBadRequest, "Unknown billing metric: "+metric)
		return
	}

	common.WriteJSON(w, http.StatusOK, resp, nil)
}

// InternalRiskFactsArrears handles GET /internal/risk-facts/arrears
func (a *App) InternalRiskFactsArrears(w http.ResponseWriter, r *http.Request) {
	schoolID := strings.TrimSpace(r.URL.Query().Get("school_id"))
	if schoolID == "" {
		common.WriteError(w, http.StatusUnprocessableEntity, "school_id is required")
		return
	}
	ctx := r.Context()
	now := time.Now()

	query := `SELECT b.student_user_id::text, b.net_idr, b.due_at,
	                 COALESCE((SELECT sum(al.amount_idr) FROM bill_payment_allocations al JOIN bill_payments p ON p.id=al.payment_id
	                           WHERE p.student_bill_id=b.id AND p.status='verified'), 0)
	               - COALESCE((SELECT sum(rv.amount_idr) FROM bill_payment_reversals rv JOIN bill_payments p ON p.id=rv.payment_id
	                           WHERE p.student_bill_id=b.id), 0) as paid_idr,
	                 COALESCE((SELECT count(*) FROM bill_payments p WHERE p.student_bill_id=b.id AND p.status='verified' AND p.paid_at > b.due_at), 0) as late_payments
	          FROM student_bills b
	          WHERE b.school_id = $1 AND b.status <> 'void'`

	rows, err := a.DB.QueryContext(ctx, query, schoolID)
	if err != nil {
		writeErr(w, err)
		return
	}
	defer rows.Close()

	type studentFact struct {
		StudentUserID      string `json:"student_user_id"`
		OverdueBillCount   int    `json:"overdue_bill_count"`
		MaxDaysOverdue     int    `json:"max_days_overdue"`
		BilledIDR          int64  `json:"billed_idr"`
		OutstandingIDR     int64  `json:"outstanding_idr"`
		OutstandingRatioBP int    `json:"outstanding_ratio_bp"`
		LatePaymentCount   int    `json:"late_payment_count"`
	}

	factsMap := make(map[string]*studentFact)

	for rows.Next() {
		var (
			studentID    string
			net, pd      int64
			dueAt        sql.NullTime
			latePayments int
		)
		if err := rows.Scan(&studentID, &net, &dueAt, &pd, &latePayments); err != nil {
			writeErr(w, err)
			return
		}

		sf := factsMap[studentID]
		if sf == nil {
			sf = &studentFact{StudentUserID: studentID}
			factsMap[studentID] = sf
		}

		sf.BilledIDR += net
		out := max(net-pd, 0)
		sf.OutstandingIDR += out
		sf.LatePaymentCount += latePayments

		if out > 0 && dueAt.Valid && now.After(dueAt.Time) {
			sf.OverdueBillCount++
			daysOverdue := int(now.Sub(dueAt.Time).Hours() / 24)
			if daysOverdue > sf.MaxDaysOverdue {
				sf.MaxDaysOverdue = daysOverdue
			}
		}
	}

	result := make([]studentFact, 0, len(factsMap))
	for _, sf := range factsMap {
		if sf.BilledIDR > 0 {
			sf.OutstandingRatioBP = int((sf.OutstandingIDR * 10000) / sf.BilledIDR)
		}
		result = append(result, *sf)
	}

	common.WriteJSON(w, http.StatusOK, result, nil)
}
