package handler

// Attendance Report (PRD menu 43):
//   FR-43-001 recap harian / bulanan / semester per siswa, kelas, unit
//   FR-43-002 tren + perbandingan antar kelas + ekspor Excel (PDF = browser
//             print, the repo-wide convention — see services/report/README.md)
//   FR-43-003 laporan kehadiran per mata pelajaran
//
// All endpoints share one query-string contract (parseReportRequest):
//   period=daily|monthly|semester|range   (default monthly)
//   date=YYYY-MM-DD (daily) · month=YYYY-MM (monthly)
//   term_id (semester; optional from/to narrow it) · from,to (range)
//   academic_year_id, classroom_id, student_id, school_id (superadmin)
// Kept in its own file (plus report_attendance_export.go) so it never
// conflicts with concurrent edits to sessions.go / store.go.

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"skci/attendance/internal/common"
	"skci/attendance/internal/model"
	"skci/attendance/internal/store"
)

var reportRoles = []string{"admin", "superadmin", "kepala_sekolah", "walikelas"}

var reportBulanIndo = [...]string{"", "Januari", "Februari", "Maret", "April", "Mei", "Juni", "Juli", "Agustus", "September", "Oktober", "November", "Desember"}

// reportPeriod is the resolved time window plus a human label for headers.
type reportPeriod struct {
	Kind   string `json:"kind"` // daily | monthly | semester | range
	From   string `json:"from,omitempty"`
	To     string `json:"to,omitempty"`
	TermID string `json:"term_id,omitempty"`
	Label  string `json:"label"`
}

type reportRequest struct {
	Filter store.AttendanceReportFilter
	Period reportPeriod
}

func fmtTanggalIndo(t time.Time) string {
	return fmt.Sprintf("%d %s %d", t.Day(), reportBulanIndo[t.Month()], t.Year())
}

// parseReportPeriod resolves the period params into an inclusive date
// window. now is injected for tests.
func parseReportPeriod(q url.Values, now time.Time) (reportPeriod, error) {
	kind := q.Get("period")
	if kind == "" {
		kind = "monthly"
	}
	parseDate := func(name, v string) (time.Time, error) {
		t, err := time.Parse("2006-01-02", v)
		if err != nil {
			return t, fmt.Errorf("%s must be YYYY-MM-DD", name)
		}
		return t, nil
	}
	p := reportPeriod{Kind: kind}
	switch kind {
	case "daily":
		d := q.Get("date")
		if d == "" {
			d = now.Format("2006-01-02")
		}
		t, err := parseDate("date", d)
		if err != nil {
			return p, err
		}
		p.From, p.To = d, d
		p.Label = "Harian " + fmtTanggalIndo(t)
	case "monthly":
		m := q.Get("month")
		if m == "" {
			m = now.Format("2006-01")
		}
		start, err := time.Parse("2006-01", m)
		if err != nil {
			return p, fmt.Errorf("month must be YYYY-MM")
		}
		end := start.AddDate(0, 1, -1)
		p.From, p.To = start.Format("2006-01-02"), end.Format("2006-01-02")
		p.Label = fmt.Sprintf("Bulanan %s %d", reportBulanIndo[start.Month()], start.Year())
	case "semester":
		p.TermID = q.Get("term_id")
		if p.TermID == "" {
			return p, fmt.Errorf("term_id is required for period=semester")
		}
		p.Label = "Semester"
		if v := q.Get("from"); v != "" {
			if _, err := parseDate("from", v); err != nil {
				return p, err
			}
			p.From = v
		}
		if v := q.Get("to"); v != "" {
			if _, err := parseDate("to", v); err != nil {
				return p, err
			}
			p.To = v
		}
	case "range":
		f, err := parseDate("from", q.Get("from"))
		if err != nil {
			return p, err
		}
		t, err := parseDate("to", q.Get("to"))
		if err != nil {
			return p, err
		}
		if t.Before(f) {
			return p, fmt.Errorf("to must not be before from")
		}
		if t.Sub(f) > 400*24*time.Hour {
			return p, fmt.Errorf("range must not exceed 400 days")
		}
		p.From, p.To = q.Get("from"), q.Get("to")
		p.Label = fmt.Sprintf("Periode %s – %s", fmtTanggalIndo(f), fmtTanggalIndo(t))
	default:
		return p, fmt.Errorf("period must be daily, monthly, semester or range")
	}
	return p, nil
}

// parseReportRequest applies role checks (wali kelas is pinned to their own
// classroom, every non-superadmin to their own school) and the period params.
func (h *Handler) parseReportRequest(w http.ResponseWriter, r *http.Request) (*reportRequest, bool) {
	claims := common.FromCtx(r)
	if !requireRole(w, claims, reportRoles...) {
		return nil, false
	}
	q := r.URL.Query()
	if claims.ActiveRole == "walikelas" && !h.requireOwnClassroom(w, r, claims, q.Get("classroom_id")) {
		return nil, false
	}
	p, err := parseReportPeriod(q, time.Now().In(wibLocation))
	if err != nil {
		common.WriteError(w, http.StatusBadRequest, err.Error())
		return nil, false
	}
	if p.Kind == "semester" {
		if name := h.Client.TermName(r.Context(), p.TermID); name != "" {
			p.Label = "Semester " + name
		}
	}
	return &reportRequest{
		Period: p,
		Filter: store.AttendanceReportFilter{
			SchoolID:       scopeFromRequest(claims, r),
			AcademicYearID: academicYearFromRequest(r),
			TermID:         p.TermID,
			ClassroomID:    q.Get("classroom_id"),
			StudentID:      q.Get("student_id"),
			From:           p.From,
			To:             p.To,
		},
	}, true
}

// resolveReportNames fills school/classroom/student/subject names in place.
// Each distinct id is looked up once; students go through one batched call.
func (h *Handler) resolveReportNames(ctx context.Context, rows []model.AttendanceRecapRow, students bool) {
	schools, classes, subjects := map[string]string{}, map[string]string{}, map[string]string{}
	var studentIDs []string
	seenStudent := map[string]bool{}
	for _, r := range rows {
		if r.SchoolID != "" {
			schools[r.SchoolID] = ""
		}
		if r.ClassroomID != "" {
			classes[r.ClassroomID] = ""
		}
		if r.SubjectID != "" {
			subjects[r.SubjectID] = ""
		}
		if students && r.StudentID != "" && !seenStudent[r.StudentID] {
			seenStudent[r.StudentID] = true
			studentIDs = append(studentIDs, r.StudentID)
		}
	}
	fill := func(m map[string]string, lookup func(context.Context, string) string) {
		for id := range m {
			m[id] = lookup(ctx, id)
		}
	}
	fill(schools, h.Client.SchoolName)
	fill(classes, h.Client.ClassroomName)
	fill(subjects, h.Client.SubjectName)
	users := h.Client.LookupUsers(ctx, studentIDs)

	orUnknown := func(s string) string {
		if s == "" {
			return "Tidak diketahui"
		}
		return s
	}
	for i := range rows {
		r := &rows[i]
		if r.SchoolID != "" {
			r.SchoolName = orUnknown(schools[r.SchoolID])
		}
		if r.ClassroomID != "" {
			r.ClassroomName = orUnknown(classes[r.ClassroomID])
		}
		if r.SubjectID != "" {
			r.SubjectName = orUnknown(subjects[r.SubjectID])
		}
		if students && r.StudentID != "" {
			u := users[r.StudentID]
			r.StudentName = orUnknown(u.Name)
			if u.NISN != nil {
				r.NISN = *u.NISN
			}
		}
	}
}

func validRecapScope(s string) bool { return s == "student" || s == "class" || s == "unit" }

// buildRecap returns the scope rows (named, sorted) and their overall total.
func (h *Handler) buildRecap(ctx context.Context, rq *reportRequest, scope string) ([]model.AttendanceRecapRow, model.AttendanceRecapRow, error) {
	base, err := h.Store.AttendanceRecapStudentRows(ctx, rq.Filter)
	if err != nil {
		return nil, model.AttendanceRecapRow{}, err
	}
	h.resolveReportNames(ctx, base, scope == "student")
	summary := store.SumCounts(base, "summary")
	rows := store.RollupRecap(base, scope)
	store.SortRecapRows(rows)
	return rows, summary, nil
}

func (h *Handler) buildSubjects(ctx context.Context, rq *reportRequest, group string) ([]model.AttendanceRecapRow, model.AttendanceRecapRow, error) {
	base, err := h.Store.AttendanceSubjectRows(ctx, rq.Filter)
	if err != nil {
		return nil, model.AttendanceRecapRow{}, err
	}
	h.resolveReportNames(ctx, base, group == "student")
	summary := store.SumCounts(base, "summary")
	rows := store.RollupSubject(base, group)
	store.SortRecapRows(rows)
	return rows, summary, nil
}

// paginate slices rows when per_page > 0; per_page omitted/0 returns every
// row (exports and print need the whole set — no 500-row cap).
func paginateReportRows(rows []model.AttendanceRecapRow, r *http.Request) ([]model.AttendanceRecapRow, map[string]int) {
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	perPage, _ := strconv.Atoi(r.URL.Query().Get("per_page"))
	total := len(rows)
	if perPage <= 0 {
		return rows, map[string]int{"page": 1, "per_page": total, "total": total}
	}
	if page < 1 {
		page = 1
	}
	start := (page - 1) * perPage
	if start > total {
		start = total
	}
	end := start + perPage
	if end > total {
		end = total
	}
	return rows[start:end], map[string]int{"page": page, "per_page": perPage, "total": total}
}

// AttendanceReportRecap: GET /admin/attendance-report/recap?scope=student|class|unit
func (h *Handler) AttendanceReportRecap(w http.ResponseWriter, r *http.Request) {
	rq, ok := h.parseReportRequest(w, r)
	if !ok {
		return
	}
	scope := r.URL.Query().Get("scope")
	if scope == "" {
		scope = "class"
	}
	if !validRecapScope(scope) {
		common.WriteError(w, http.StatusBadRequest, "scope must be student, class or unit")
		return
	}
	rows, summary, err := h.buildRecap(r.Context(), rq, scope)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	pageRows, meta := paginateReportRows(rows, r)
	common.WriteJSON(w, http.StatusOK, map[string]interface{}{
		"scope": scope, "period": rq.Period, "summary": summary, "rows": pageRows,
	}, meta)
}

// AttendanceReportClassComparison: GET /admin/attendance-report/class-comparison
func (h *Handler) AttendanceReportClassComparison(w http.ResponseWriter, r *http.Request) {
	rq, ok := h.parseReportRequest(w, r)
	if !ok {
		return
	}
	rows, summary, err := h.buildRecap(r.Context(), rq, "class")
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	common.WriteJSON(w, http.StatusOK, map[string]interface{}{
		"period": rq.Period, "summary": summary, "rows": store.RankClasses(rows),
	}, nil)
}

// AttendanceReportTrend: GET /admin/attendance-report/trend?granularity=day|week|month
func (h *Handler) AttendanceReportTrend(w http.ResponseWriter, r *http.Request) {
	rq, ok := h.parseReportRequest(w, r)
	if !ok {
		return
	}
	g := r.URL.Query().Get("granularity")
	if g == "" {
		g = "day"
	}
	if !store.TrendGranularities[g] {
		common.WriteError(w, http.StatusBadRequest, "granularity must be day, week or month")
		return
	}
	points, err := h.Store.AttendanceTrend(r.Context(), rq.Filter, g)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	common.WriteJSON(w, http.StatusOK, map[string]interface{}{
		"period": rq.Period, "granularity": g, "points": points,
	}, nil)
}

func validSubjectGroup(s string) bool { return s == "subject" || s == "class" || s == "student" }

// AttendanceReportSubjects: GET /admin/attendance-report/subjects?group=subject|class|student&subject_id=
func (h *Handler) AttendanceReportSubjects(w http.ResponseWriter, r *http.Request) {
	rq, ok := h.parseReportRequest(w, r)
	if !ok {
		return
	}
	group := r.URL.Query().Get("group")
	if group == "" {
		group = "class"
	}
	if !validSubjectGroup(group) {
		common.WriteError(w, http.StatusBadRequest, "group must be subject, class or student")
		return
	}
	rq.Filter.SubjectID = r.URL.Query().Get("subject_id")
	rows, summary, err := h.buildSubjects(r.Context(), rq, group)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	pageRows, meta := paginateReportRows(rows, r)
	common.WriteJSON(w, http.StatusOK, map[string]interface{}{
		"group": group, "period": rq.Period, "summary": summary, "rows": pageRows,
	}, meta)
}
