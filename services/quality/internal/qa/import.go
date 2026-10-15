package qa

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/xuri/excelize/v2"

	"skci/quality/internal/common"
)

type importRow struct {
	line                    int
	stdCode, stdName        string
	stdWeight               int
	itemCode, itemStatement string
	ind                     QAIndicator
}

// parseImportRows validates every row of the template (header in row 1).
func parseImportRows(rows [][]string) ([]importRow, []string) {
	var out []importRow
	var errs []string
	col := func(r []string, i int) string {
		if i < len(r) {
			return strings.TrimSpace(r[i])
		}
		return ""
	}
	for i, r := range rows[1:] {
		line := i + 2
		if strings.TrimSpace(strings.Join(r, "")) == "" {
			continue
		}
		stdW, err1 := strconv.Atoi(col(r, 2))
		indW, err2 := strconv.Atoi(col(r, 7))
		row := importRow{line: line, stdCode: col(r, 0), stdName: col(r, 1), stdWeight: stdW, itemCode: col(r, 3), itemStatement: col(r, 4)}
		row.ind = QAIndicator{Code: col(r, 5), Name: col(r, 6), WeightBP: indW, Unit: col(r, 8), Direction: col(r, 9), SourceKind: col(r, 10)}
		if mk := col(r, 11); mk != "" {
			row.ind.MetricKey = &mk
		}
		switch {
		case err1 != nil || err2 != nil:
			errs = append(errs, fmt.Sprintf("baris %d: bobot harus angka (bp)", line))
			continue
		case row.stdCode == "" || row.stdName == "":
			errs = append(errs, fmt.Sprintf("baris %d: kode/nama standar kosong", line))
			continue
		case row.itemCode == "" || row.itemStatement == "":
			errs = append(errs, fmt.Sprintf("baris %d: kode/pernyataan butir kosong", line))
			continue
		}
		if msg := validIndicator(&row.ind); msg != "" {
			errs = append(errs, fmt.Sprintf("baris %d: %s", line, msg))
			continue
		}
		out = append(out, row)
	}
	if len(out) == 0 && len(errs) == 0 {
		errs = append(errs, "tidak ada baris data")
	}
	return out, errs
}

// ImportInstrument: POST /qa/instruments/import?dry_run= (XLSX from the
// template). Dry run only validates; a real run creates a draft instrument
// with the full standard → item → indicator tree in one transaction.
func (h *Handler) ImportInstrument(w http.ResponseWriter, r *http.Request) {
	claims := common.FromContext(r.Context())
	dryRun := r.URL.Query().Get("dry_run") == "true"
	schoolID := h.effectiveSchoolID(r)

	if err := r.ParseMultipartForm(10 << 20); err != nil {
		common.WriteError(w, http.StatusBadRequest, "berkas XLSX wajib diunggah")
		return
	}
	file, header, err := r.FormFile("file")
	if err != nil {
		common.WriteError(w, http.StatusBadRequest, "berkas XLSX wajib diunggah")
		return
	}
	defer file.Close()
	f, err := excelize.OpenReader(file)
	if err != nil {
		common.WriteError(w, http.StatusBadRequest, "berkas bukan XLSX yang valid")
		return
	}
	defer f.Close()
	rows, err := f.GetRows(f.GetSheetName(0))
	if err != nil || len(rows) < 2 {
		common.WriteError(w, http.StatusBadRequest, "berkas tidak memiliki baris data")
		return
	}
	parsed, errs := parseImportRows(rows)
	if len(errs) > 0 {
		common.WriteError(w, http.StatusUnprocessableEntity, fmt.Sprintf("%d baris tidak valid: %s", len(errs), strings.Join(errs, "; ")), map[string]any{"errors": errs})
		return
	}
	if dryRun {
		common.WriteJSON(w, http.StatusOK, map[string]any{
			"dry_run": true, "valid": true, "imported_count": 0, "row_count": len(parsed),
			"message": fmt.Sprintf("Validasi berhasil: %d indikator siap diimpor.", len(parsed)),
		})
		return
	}

	ctx := r.Context()
	tx, err := h.store.pool.Begin(ctx)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "gagal mengimpor instrumen")
		return
	}
	defer tx.Rollback(ctx)
	name := strings.TrimSuffix(header.Filename, ".xlsx")
	var instID string
	if err := tx.QueryRow(ctx, `
		INSERT INTO qa_instruments (school_id, code, name, issuer, version_label, score_min, score_max, status)
		VALUES ($1, $2, $3, 'Impor XLSX', 'v1', 1, 4, 'draft') RETURNING id`,
		schoolID, fmt.Sprintf("IMP-%d", time.Now().Unix()), "Impor "+safeFileName(name)).Scan(&instID); err != nil {
		common.WriteError(w, http.StatusInternalServerError, "gagal mengimpor instrumen")
		return
	}
	stdIDs, itemIDs := map[string]string{}, map[string]string{}
	for i, row := range parsed {
		sid, ok := stdIDs[row.stdCode]
		if !ok {
			if err := tx.QueryRow(ctx, `INSERT INTO qa_standards (school_id, instrument_id, sort, code, name, weight_bp) VALUES ($1,$2,$3,$4,$5,$6) RETURNING id`,
				schoolID, instID, len(stdIDs), row.stdCode, row.stdName, row.stdWeight).Scan(&sid); err != nil {
				common.WriteError(w, http.StatusInternalServerError, fmt.Sprintf("gagal mengimpor baris %d", row.line))
				return
			}
			stdIDs[row.stdCode] = sid
		}
		key := row.stdCode + "\x00" + row.itemCode
		iid, ok := itemIDs[key]
		if !ok {
			if err := tx.QueryRow(ctx, `INSERT INTO qa_items (school_id, standard_id, sort, code, statement) VALUES ($1,$2,$3,$4,$5) RETURNING id`,
				schoolID, sid, len(itemIDs), row.itemCode, row.itemStatement).Scan(&iid); err != nil {
				common.WriteError(w, http.StatusInternalServerError, fmt.Sprintf("gagal mengimpor baris %d", row.line))
				return
			}
			itemIDs[key] = iid
		}
		if _, err := tx.Exec(ctx, `INSERT INTO qa_indicators (school_id, item_id, sort, code, name, weight_bp, unit, direction, source_kind, metric_key, rubric) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,'[]')`,
			schoolID, iid, i, row.ind.Code, row.ind.Name, row.ind.WeightBP, row.ind.Unit, row.ind.Direction, row.ind.SourceKind, row.ind.MetricKey); err != nil {
			common.WriteError(w, http.StatusInternalServerError, fmt.Sprintf("gagal mengimpor baris %d", row.line))
			return
		}
	}
	if err := tx.Commit(ctx); err != nil {
		common.WriteError(w, http.StatusInternalServerError, "gagal mengimpor instrumen")
		return
	}
	_ = h.store.RecordAudit(ctx, schoolID, "qa_instrument", instID, "import", claims.UserID, claims.ActiveRole, nil, map[string]int{"indicators": len(parsed)}, "Impor XLSX")
	common.WriteJSON(w, http.StatusCreated, map[string]any{
		"message":        fmt.Sprintf("Instrumen diimpor sebagai draf: %d standar, %d butir, %d indikator.", len(stdIDs), len(itemIDs), len(parsed)),
		"instrument_id":  instID,
		"imported_count": len(parsed),
	})
}
