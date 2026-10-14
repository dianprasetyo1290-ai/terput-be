package handler

import (
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"skci/billing/internal/common"
	"skci/billing/internal/model"
)

// BUG-122: the current year's payment history from the previous payment
// aggregator is imported as verified payments (source 'import') so bill
// statuses and receivable balances start correct. The file is all or nothing,
// re-importing the same lines is skipped (idempotency "import:"+row hash), and
// imported payments are not journaled: they were booked by the previous system
// and enter accounting as the opening balance.

type importBillState struct {
	id          string
	outstanding int64
}

// ImportPayments is POST /admin/payments/import (multipart: file, source_name,
// dry_run=true for a preview without saving).
func (a *App) ImportPayments(w http.ResponseWriter, r *http.Request) {
	c, schoolID := common.RequireFinanceStaff(w, r)
	if c == nil {
		return
	}
	if err := r.ParseMultipartForm(10 << 20); err != nil {
		common.WriteError(w, http.StatusBadRequest, "form unggah tidak valid")
		return
	}
	sourceName := strings.TrimSpace(r.FormValue("source_name"))
	if sourceName == "" {
		common.WriteError(w, http.StatusUnprocessableEntity, "nama aggregator sumber wajib diisi")
		return
	}
	dryRun := r.FormValue("dry_run") == "true"
	file, header, err := r.FormFile("file")
	if err != nil {
		common.WriteError(w, http.StatusBadRequest, "file wajib diunggah")
		return
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, 10<<20))
	if err != nil {
		common.WriteError(w, http.StatusBadRequest, "file tidak terbaca")
		return
	}
	rows, problems := model.ParsePaymentImportCSV(data, a.now().In(a.loc()))
	if len(problems) > 0 {
		common.WriteError(w, http.StatusUnprocessableEntity, "file ditolak: "+strings.Join(problems, "; ")+". Format: "+model.PaymentImportFormat)
		return
	}

	ctx := r.Context()
	tx, err := a.DB.BeginTx(ctx, nil)
	if err != nil {
		writeErr(w, err)
		return
	}
	defer func() { _ = tx.Rollback() }()

	bills := map[string]*importBillState{} // student|year|month|bill_id → bill
	var toInsert []model.PaymentImportRow
	var targets []*importBillState
	skipped := 0
	var total int64
	for _, row := range rows {
		var exists bool
		if err := tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM bill_payments WHERE school_id=$1 AND idempotency_key=$2)`,
			schoolID, "import:"+row.Hash).Scan(&exists); err != nil {
			writeErr(w, err)
			return
		}
		if exists {
			skipped++
			continue
		}
		key := fmt.Sprintf("%s|%d|%d|%s", row.StudentUserID, row.Year, row.Month, row.BillID)
		st, ok := bills[key]
		if !ok {
			var ids []string
			q, err := tx.QueryContext(ctx,
				`SELECT id::text FROM student_bills
				  WHERE school_id=$1 AND student_user_id::text=$2 AND period_year=$3 AND period_month=$4 AND status<>'void'
				    AND ($5='' OR id::text=$5)
				  ORDER BY id FOR UPDATE`, schoolID, row.StudentUserID, row.Year, row.Month, row.BillID)
			if err != nil {
				writeErr(w, err)
				return
			}
			for q.Next() {
				var id string
				if err := q.Scan(&id); err != nil {
					q.Close()
					writeErr(w, err)
					return
				}
				ids = append(ids, id)
			}
			q.Close()
			switch {
			case len(ids) == 0:
				problems = append(problems, fmt.Sprintf("baris %d: tagihan siswa periode %04d-%02d tidak ditemukan", row.Line, row.Year, row.Month))
				bills[key] = nil
				continue
			case len(ids) > 1:
				problems = append(problems, fmt.Sprintf("baris %d: siswa punya %d tagihan periode %04d-%02d, isi kolom bill_id", row.Line, len(ids), row.Year, row.Month))
				bills[key] = nil
				continue
			}
			_, _, _, outstanding, err := billTotalsQ(ctx, tx, schoolID, ids[0])
			if err != nil {
				writeErr(w, err)
				return
			}
			st = &importBillState{id: ids[0], outstanding: outstanding}
			bills[key] = st
		}
		if st == nil {
			continue
		}
		if row.AmountIDR > st.outstanding {
			problems = append(problems, fmt.Sprintf("baris %d: nominal %s melebihi sisa tagihan %s", row.Line,
				formatRupiah(row.AmountIDR), formatRupiah(st.outstanding)))
			continue
		}
		st.outstanding -= row.AmountIDR
		total += row.AmountIDR
		toInsert = append(toInsert, row)
		targets = append(targets, st)
	}
	if len(problems) > 0 {
		common.WriteError(w, http.StatusUnprocessableEntity, "file ditolak, tidak ada yang disimpan: "+strings.Join(problems, "; "))
		return
	}
	result := map[string]any{"row_count": len(rows), "imported": len(toInsert), "skipped": skipped, "total_idr": total, "dry_run": dryRun}
	if dryRun {
		common.WriteJSON(w, http.StatusOK, result, nil)
		return
	}
	var importID string
	if err := tx.QueryRowContext(ctx,
		`INSERT INTO payment_imports(school_id,file_name,source_name,row_count,imported,skipped,total_idr,imported_by)
		 VALUES($1,$2,$3,$4,$5,$6,$7,$8::uuid) RETURNING id::text`,
		schoolID, header.Filename, sourceName, len(rows), len(toInsert), skipped, total, c.UserID).Scan(&importID); err != nil {
		writeErr(w, err)
		return
	}
	touched := map[string]bool{}
	for i, row := range toInsert {
		var pID string
		paidAt := time.Date(row.PaidAt.Year(), row.PaidAt.Month(), row.PaidAt.Day(), 12, 0, 0, 0, a.loc())
		if err := tx.QueryRowContext(ctx,
			`INSERT INTO bill_payments(school_id,student_bill_id,amount_idr,transfer_at,bank_name,reference,status,source,source_id,
			                           idempotency_key,uploader_user_id,verified_at,review_note)
			 VALUES($1,$2::uuid,$3,$4,$5,NULLIF($6,''),'verified','import',$7::uuid,$8,$9::uuid,$4,'Impor riwayat pembayaran')
			 RETURNING id::text`,
			schoolID, targets[i].id, row.AmountIDR, paidAt, sourceName, row.Reference, importID, "import:"+row.Hash, c.UserID).Scan(&pID); err != nil {
			writeErr(w, err)
			return
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO bill_payment_allocations(school_id,payment_id,amount_idr) VALUES($1,$2,$3)`,
			schoolID, pID, row.AmountIDR); err != nil {
			writeErr(w, err)
			return
		}
		touched[targets[i].id] = true
	}
	for billID := range touched {
		if err := recomputeBillStatusQ(ctx, tx, schoolID, billID); err != nil {
			writeErr(w, err)
			return
		}
	}
	if err := tx.Commit(); err != nil {
		writeErr(w, err)
		return
	}
	result["id"] = importID
	a.auditBoth(ctx, schoolID, "billing", "payment_import", importID, "payment_history_imported", c.ActiveRole, c.UserID,
		"impor riwayat pembayaran dari "+sourceName, nil, map[string]any{"file_name": header.Filename, "source_name": sourceName,
			"row_count": len(rows), "imported": len(toInsert), "skipped": skipped, "total_idr": total, "bills": len(touched)})
	common.WriteJSON(w, http.StatusCreated, result, nil)
}

// ListPaymentImports is GET /admin/payments/imports?page&per_page.
func (a *App) ListPaymentImports(w http.ResponseWriter, r *http.Request) {
	c, schoolID := common.RequireFinanceStaff(w, r)
	if c == nil {
		return
	}
	page, perPage := common.Page(r)
	var total int
	if err := a.DB.QueryRowContext(r.Context(), `SELECT count(*) FROM payment_imports WHERE school_id=$1`, schoolID).Scan(&total); err != nil {
		writeErr(w, err)
		return
	}
	rows, err := a.DB.QueryContext(r.Context(),
		`SELECT id::text,file_name,source_name,row_count,imported,skipped,total_idr,imported_by::text,created_at
		   FROM payment_imports WHERE school_id=$1 ORDER BY created_at DESC LIMIT $2 OFFSET $3`,
		schoolID, perPage, common.Offset(page, perPage))
	if err != nil {
		writeErr(w, err)
		return
	}
	defer rows.Close()
	items := []map[string]any{}
	for rows.Next() {
		var (
			id, fileName, source, by       string
			rowCount, imported, skippedCnt int
			totalIDR                       int64
			createdAt                      time.Time
		)
		if err := rows.Scan(&id, &fileName, &source, &rowCount, &imported, &skippedCnt, &totalIDR, &by, &createdAt); err != nil {
			writeErr(w, err)
			return
		}
		items = append(items, map[string]any{"id": id, "file_name": fileName, "source_name": source, "row_count": rowCount,
			"imported": imported, "skipped": skippedCnt, "total_idr": totalIDR, "imported_by": by, "created_at": createdAt})
	}
	if err := rows.Err(); err != nil {
		writeErr(w, err)
		return
	}
	common.WriteJSON(w, http.StatusOK, items, &common.Meta{Page: page, PerPage: perPage, Total: total})
}
