package handler

// Excel export for the Attendance Report (FR-43-002). Uses excelize, already
// a dependency of this service (import template). PDF follows the repo-wide
// convention — the frontend's print view + window.print() ("Cetak / Simpan
// PDF"); there is deliberately no Go PDF library (services/report/README.md).

import (
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/xuri/excelize/v2"

	"skci/attendance/internal/common"
	"skci/attendance/internal/model"
	"skci/attendance/internal/store"
)

// reportTable is a rendered sheet: title lines + header + body rows.
type reportTable struct {
	Title    string
	Subtitle []string
	Headers  []string
	Rows     [][]interface{}
	PctCols  map[int]bool // 0-based column indexes formatted as percentages
}

var countHeaders = []string{"Hadir", "Terlambat", "Sakit", "Izin", "Alpa", "Total", "% Hadir", "% Terlambat", "% Tidak Hadir", "% Kehadiran"}

func countCells(c model.AttendanceCounts, p model.AttendancePercentages) []interface{} {
	return []interface{}{c.Hadir, c.Terlambat, c.Sakit, c.Izin, c.TanpaKeterangan, c.Total, p.Hadir, p.Terlambat, p.TidakHadir, p.Kehadiran}
}

func pctColsFrom(offset int) map[int]bool {
	return map[int]bool{offset + 6: true, offset + 7: true, offset + 8: true, offset + 9: true}
}

func recapTable(scope string, period reportPeriod, rows []model.AttendanceRecapRow, summary model.AttendanceRecapRow) reportTable {
	scopeLabel := map[string]string{"student": "per Siswa", "class": "per Kelas", "unit": "per Unit"}[scope]
	t := reportTable{Title: "Rekap Kehadiran Siswa " + scopeLabel, Subtitle: []string{period.Label}}
	var lead []string
	switch scope {
	case "student":
		lead = []string{"No", "NISN", "Nama Siswa", "Kelas"}
	case "class":
		lead = []string{"No", "Unit", "Kelas", "Jumlah Siswa"}
	default:
		lead = []string{"No", "Unit", "Jumlah Siswa"}
	}
	t.Headers = append(lead, countHeaders...)
	t.PctCols = pctColsFrom(len(lead))
	for i, r := range rows {
		var cells []interface{}
		switch scope {
		case "student":
			cells = []interface{}{i + 1, r.NISN, r.StudentName, r.ClassroomName}
		case "class":
			cells = []interface{}{i + 1, r.SchoolName, r.ClassroomName, r.Students}
		default:
			cells = []interface{}{i + 1, r.SchoolName, r.Students}
		}
		t.Rows = append(t.Rows, append(cells, countCells(r.Counts, r.Percent)...))
	}
	total := make([]interface{}, len(lead))
	total[0] = "TOTAL"
	total[len(lead)-1] = ""
	if scope != "student" {
		total[len(lead)-1] = summary.Students
	}
	t.Rows = append(t.Rows, append(total, countCells(summary.Counts, summary.Percent)...))
	return t
}

func comparisonTable(period reportPeriod, rows []model.AttendanceRecapRow, summary model.AttendanceRecapRow) reportTable {
	lead := []string{"Peringkat", "Kelas", "Jumlah Siswa"}
	t := reportTable{Title: "Perbandingan Kehadiran Antar Kelas", Subtitle: []string{period.Label, fmt.Sprintf("Rata-rata kehadiran: %.1f%%", summary.Percent.Kehadiran)}}
	t.Headers = append(append(lead, countHeaders...), "Selisih vs Rata-rata")
	t.PctCols = pctColsFrom(len(lead))
	for _, r := range rows {
		delta := 0.0
		if r.DeltaVsAvg != nil {
			delta = *r.DeltaVsAvg
		}
		cells := append([]interface{}{r.Rank, r.ClassroomName, r.Students}, countCells(r.Counts, r.Percent)...)
		t.Rows = append(t.Rows, append(cells, delta))
	}
	return t
}

func subjectTable(group string, period reportPeriod, rows []model.AttendanceRecapRow, summary model.AttendanceRecapRow) reportTable {
	t := reportTable{Title: "Laporan Kehadiran per Mata Pelajaran", Subtitle: []string{period.Label}}
	var lead []string
	switch group {
	case "student":
		lead = []string{"No", "Mata Pelajaran", "Kelas", "NISN", "Nama Siswa", "Pertemuan"}
	case "subject":
		lead = []string{"No", "Mata Pelajaran", "Jumlah Siswa", "Pertemuan"}
	default:
		lead = []string{"No", "Mata Pelajaran", "Kelas", "Jumlah Siswa", "Pertemuan"}
	}
	t.Headers = append(lead, countHeaders...)
	t.PctCols = pctColsFrom(len(lead))
	for i, r := range rows {
		var cells []interface{}
		switch group {
		case "student":
			cells = []interface{}{i + 1, r.SubjectName, r.ClassroomName, r.NISN, r.StudentName, r.Sessions}
		case "subject":
			cells = []interface{}{i + 1, r.SubjectName, r.Students, r.Sessions}
		default:
			cells = []interface{}{i + 1, r.SubjectName, r.ClassroomName, r.Students, r.Sessions}
		}
		t.Rows = append(t.Rows, append(cells, countCells(r.Counts, r.Percent)...))
	}
	total := make([]interface{}, len(lead))
	total[0] = "TOTAL"
	t.Rows = append(t.Rows, append(total, countCells(summary.Counts, summary.Percent)...))
	return t
}

func trendTable(period reportPeriod, granularity string, points []model.AttendanceTrendPoint) reportTable {
	g := map[string]string{"day": "Harian", "week": "Mingguan", "month": "Bulanan"}[granularity]
	t := reportTable{Title: "Tren Kehadiran Siswa (" + g + ")", Subtitle: []string{period.Label}}
	t.Headers = append([]string{"Periode"}, countHeaders...)
	t.PctCols = pctColsFrom(1)
	for _, p := range points {
		t.Rows = append(t.Rows, append([]interface{}{p.Period}, countCells(p.Counts, p.Percent)...))
	}
	return t
}

// writeReportXLSX renders t as a single-sheet workbook.
func writeReportXLSX(out io.Writer, t reportTable) error {
	f := excelize.NewFile()
	defer f.Close()
	sheet := "Laporan"
	f.SetSheetName(f.GetSheetName(0), sheet)

	titleStyle, _ := f.NewStyle(&excelize.Style{Font: &excelize.Font{Bold: true, Size: 14}})
	headStyle, _ := f.NewStyle(&excelize.Style{
		Font:      &excelize.Font{Bold: true, Color: "FFFFFF"},
		Fill:      excelize.Fill{Type: "pattern", Pattern: 1, Color: []string{"001F60"}},
		Alignment: &excelize.Alignment{Horizontal: "center", Vertical: "center", WrapText: true},
	})
	pctStyle, _ := f.NewStyle(&excelize.Style{CustomNumFmt: strPtr(`0.0"%"`)})

	row := 1
	_ = f.SetCellValue(sheet, "A1", t.Title)
	_ = f.SetCellStyle(sheet, "A1", "A1", titleStyle)
	row++
	for _, s := range t.Subtitle {
		_ = f.SetCellValue(sheet, fmt.Sprintf("A%d", row), s)
		row++
	}
	row++
	headerRow := row
	for i, h := range t.Headers {
		cell, _ := excelize.CoordinatesToCellName(i+1, headerRow)
		_ = f.SetCellValue(sheet, cell, h)
	}
	first, _ := excelize.CoordinatesToCellName(1, headerRow)
	last, _ := excelize.CoordinatesToCellName(len(t.Headers), headerRow)
	_ = f.SetCellStyle(sheet, first, last, headStyle)
	for ri, cells := range t.Rows {
		r := headerRow + 1 + ri
		cell, _ := excelize.CoordinatesToCellName(1, r)
		_ = f.SetSheetRow(sheet, cell, &cells)
	}
	if len(t.Rows) > 0 {
		for c := range t.PctCols {
			col, _ := excelize.ColumnNumberToName(c + 1)
			_ = f.SetCellStyle(sheet, fmt.Sprintf("%s%d", col, headerRow+1), fmt.Sprintf("%s%d", col, headerRow+len(t.Rows)), pctStyle)
		}
	}
	for i, h := range t.Headers {
		col, _ := excelize.ColumnNumberToName(i + 1)
		width := float64(len(h) + 4)
		if strings.Contains(h, "Nama") || h == "Kelas" || h == "Unit" || h == "Mata Pelajaran" {
			width = 28
		}
		_ = f.SetColWidth(sheet, col, col, width)
	}
	_ = f.SetPanes(sheet, &excelize.Panes{Freeze: true, YSplit: headerRow, TopLeftCell: fmt.Sprintf("A%d", headerRow+1), ActivePane: "bottomLeft"})
	return f.Write(out)
}

func strPtr(s string) *string { return &s }

// AttendanceReportExport: GET /admin/attendance-report/export?report=recap|comparison|subjects|trend&format=xlsx
// plus the same params as the matching JSON endpoint (scope / group /
// granularity). Always the full row set.
func (h *Handler) AttendanceReportExport(w http.ResponseWriter, r *http.Request) {
	rq, ok := h.parseReportRequest(w, r)
	if !ok {
		return
	}
	q := r.URL.Query()
	if f := q.Get("format"); f != "" && f != "xlsx" {
		common.WriteError(w, http.StatusBadRequest, "format must be xlsx (PDF: use the print view)")
		return
	}
	report := q.Get("report")
	if report == "" {
		report = "recap"
	}
	ctx := r.Context()
	var t reportTable
	switch report {
	case "recap":
		scope := q.Get("scope")
		if scope == "" {
			scope = "class"
		}
		if !validRecapScope(scope) {
			common.WriteError(w, http.StatusBadRequest, "scope must be student, class or unit")
			return
		}
		rows, summary, err := h.buildRecap(ctx, rq, scope)
		if err != nil {
			common.WriteError(w, http.StatusInternalServerError, err.Error())
			return
		}
		t = recapTable(scope, rq.Period, rows, summary)
	case "comparison":
		rows, summary, err := h.buildRecap(ctx, rq, "class")
		if err != nil {
			common.WriteError(w, http.StatusInternalServerError, err.Error())
			return
		}
		t = comparisonTable(rq.Period, store.RankClasses(rows), summary)
	case "subjects":
		group := q.Get("group")
		if group == "" {
			group = "class"
		}
		if !validSubjectGroup(group) {
			common.WriteError(w, http.StatusBadRequest, "group must be subject, class or student")
			return
		}
		rq.Filter.SubjectID = q.Get("subject_id")
		rows, summary, err := h.buildSubjects(ctx, rq, group)
		if err != nil {
			common.WriteError(w, http.StatusInternalServerError, err.Error())
			return
		}
		t = subjectTable(group, rq.Period, rows, summary)
	case "trend":
		g := q.Get("granularity")
		if g == "" {
			g = "day"
		}
		points, err := h.Store.AttendanceTrend(ctx, rq.Filter, g)
		if err != nil {
			common.WriteError(w, http.StatusBadRequest, err.Error())
			return
		}
		t = trendTable(rq.Period, g, points)
	default:
		common.WriteError(w, http.StatusBadRequest, "report must be recap, comparison, subjects or trend")
		return
	}
	name := fmt.Sprintf("laporan-kehadiran-%s-%s.xlsx", report, strings.ReplaceAll(rq.Period.From+"_"+rq.Period.To, "-", ""))
	w.Header().Set("Content-Type", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, name))
	_ = writeReportXLSX(w, t)
}
