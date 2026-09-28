package handler

import (
	"archive/zip"
	"bytes"
	"fmt"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"skci/report/internal/common"
	"skci/report/internal/docx"
	"skci/report/internal/model"
)

// Mail merge: render a report card through the school's own uploaded .docx
// instead of the built-in print view. The two run side by side -- a school
// that has uploaded no .docx keeps ReportCardPrintView.vue exactly as
// before, so nothing here can regress an existing school's rapor.
//
// Which template a card uses follows its report_type: a Rapot Bayangan
// takes the "tengah_semester" slot, a regular E-Rapor the "akhir_semester"
// one -- the same two slots the Import Template popup already offers.

// raporDateLine renders "Kota Bekasi, 24 Juni 2026" from the school's own
// city, or just the date when the school hasn't set one (never hardcode one
// school's city -- see fetchSchoolInfo).
var monthsID = [...]string{"Januari", "Februari", "Maret", "April", "Mei", "Juni",
	"Juli", "Agustus", "September", "Oktober", "November", "Desember"}

func tanggalID(t time.Time) string {
	return fmt.Sprintf("%d %s %d", t.Day(), monthsID[int(t.Month())-1], t.Year())
}

// fmtScore prints a score the way a rapor does: a whole number, rounded
// half up (BUG SEKOCI 2026-09-24 item B -- 88.90 -> 89, 83.35 -> 83).
// Empty string for a missing score, so a blank cell prints blank rather
// than "0" -- a zero is a real grade and must not be invented.
func fmtScore(v *float64) string {
	if v == nil {
		return ""
	}
	return strconv.FormatInt(int64(math.Round(*v+1e-9)), 10)
}

func fmtCount(v *int) string {
	if v == nil {
		return "0"
	}
	return strconv.Itoa(*v)
}

func str(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

// groupLetter pulls the "A"/"B"/"C" out of a subject_group label like
// "A. Kelompok Mata Pelajaran Umum", so the template can address each
// kelompok's table separately ({{#mapel_a}} ... {{/mapel_a}}). Subjects
// with no group land in none of the lettered loops but always appear in
// {{#mapel}}.
func groupLetter(g *string) string {
	s := strings.TrimSpace(str(g))
	if len(s) >= 2 && s[1] == '.' && s[0] >= 'A' && s[0] <= 'Z' {
		return strings.ToLower(s[:1])
	}
	return ""
}

// buildMergeData maps one report card onto the placeholder names an admin
// writes in their Word template. Everything is a plain string here: the
// template decides layout, this decides only what a name means.
func buildMergeData(rc *model.ReportCard) docx.Data {
	school := rc.School
	if school == nil {
		school = &model.SchoolInfo{}
	}

	var jumlah float64
	var n int
	for i := range rc.Subjects {
		if f := rc.Subjects[i].Final(); f != nil {
			jumlah += *f
			n++
		}
	}
	rata := ""
	if n > 0 {
		v := jumlah / float64(n)
		rata = fmtScore(&v)
	}

	now := time.Now()
	kota := str(school.City)
	tanggalKota := tanggalID(now)
	if kota != "" {
		tanggalKota = kota + ", " + tanggalID(now)
	}

	jenis := "ASESMEN SUMATIF AKHIR SEMESTER"
	if rc.ReportType == "bayangan" {
		jenis = "ASESMEN SUMATIF TENGAH SEMESTER"
	}

	scalars := map[string]string{
		"nama_siswa":     str(rc.StudentName),
		"nisn":           str(rc.StudentNISN),
		"kelas":          str(rc.ClassroomName),
		"fase":           rc.Fase,
		"semester":       str(rc.TermName),
		"nama_sekolah":   school.Name,
		"alamat_sekolah": str(school.Address),
		"kota":           kota,
		"provinsi":       str(school.Province),
		"wali_kelas":     str(rc.HomeroomTeacherName),
		"kepala_sekolah": str(rc.PrincipalName),

		"jenis_rapor":  jenis,
		"periode":      rc.PeriodLabel,
		"tanggal":      tanggalID(now),
		"tanggal_kota": tanggalKota,

		"jumlah_nilai": fmtScore(&jumlah),
		"rata_rata":    rata,

		"kokurikuler":   str(rc.KokurikulerNote),
		"catatan_walas": str(rc.ClassTeacherNote),

		"hadir":            fmtCount(rc.Attendance.Hadir),
		"sakit":            fmtCount(rc.Attendance.Sakit),
		"izin":             fmtCount(rc.Attendance.Izin),
		"tanpa_keterangan": fmtCount(rc.Attendance.TanpaKeterangan),

		"kenaikan_status": str(rc.KenaikanStatus),
		"kenaikan_kelas":  str(rc.KenaikanKelas),
	}

	loops := map[string][]map[string]string{
		"mapel":    {},
		"mapel_a":  {},
		"mapel_b":  {},
		"mapel_c":  {},
		"ekskul":   {},
		"prestasi": {},
	}
	for i := range rc.Subjects {
		s := &rc.Subjects[i]
		item := map[string]string{
			// no_urut numbers the mapel continuously across kelompok A/B/C,
			// the way a printed rapor does (1..9 down the whole table).
			// {{no}} restarts at 1 inside each {{#mapel_a}}/{{#mapel_b}}
			// loop, which is right for ekskul but wrong here.
			"no_urut":     strconv.Itoa(i + 1),
			"nama":        str(s.SubjectName),
			"kode":        str(s.SubjectCode),
			"kelompok":    str(s.SubjectGroup),
			"nilai_akhir": fmtScore(s.Final()),
			"formatif":    fmtScore(s.FormatifScore),
			"praktik":     fmtScore(s.PraktikScore),
			"pts":         fmtScore(s.StsScore),
			"pas":         fmtScore(s.SasScore),
			"predikat":    str(s.GradeLabel),
			"capaian":     str(s.FinalNote),
			"kemajuan":    str(s.MidNote),
			// Capaian kolom 2 (Report Sekoci V15 item 1): a second box per
			// bucket, its own placeholder so a template can print both.
			"capaian2":  str(s.FinalNote2),
			"kemajuan2": str(s.MidNote2),
		}
		loops["mapel"] = append(loops["mapel"], item)
		if l := groupLetter(s.SubjectGroup); l != "" {
			key := "mapel_" + l
			// A school may letter its kelompok past C; create the bucket
			// on demand rather than silently dropping those subjects.
			copyItem := make(map[string]string, len(item))
			for k, v := range item {
				copyItem[k] = v
			}
			loops[key] = append(loops[key], copyItem)
		}
	}
	for _, e := range rc.Ekstrakurikuler {
		loops["ekskul"] = append(loops["ekskul"], map[string]string{
			"kegiatan": e.Kegiatan, "predikat": e.Predikat, "keterangan": e.Keterangan,
		})
	}
	for _, p := range rc.Prestasi {
		loops["prestasi"] = append(loops["prestasi"], map[string]string{
			"jenis": p.Jenis, "keterangan": p.Keterangan,
		})
	}
	return docx.Data{Scalars: scalars, Loops: loops}
}

// templateTypeFor maps a report card to the Import Template slot its .docx
// lives in.
func templateTypeFor(rc *model.ReportCard) string {
	if rc.ReportType == "bayangan" {
		return "tengah_semester"
	}
	return "akhir_semester"
}

// templateSlotLabel names an Import Template slot the way Generate Rapor >
// Import Template labels it, so a "template belum diunggah" error can point
// at the row the admin has to fill.
//
// Report Sekoci V15 item 2: a guru downloading from Deskripsi Rapor always
// got "belum ada template untuk tipe rapor ini" while the admin could see a
// template sitting right there on Generate Rapor -- so it read as the system
// failing to see its own upload. It was a different slot. Deskripsi Rapor
// lists report_type "semester" cards only (GetSubjectDescriptionGrid), which
// need the akhir_semester slot, and every school that had uploaded anything
// had filled tengah_semester instead. Naming the slot is the whole fix; the
// slots are deliberately separate documents and must not fall back to each
// other.
func templateSlotLabel(templateType string) string {
	switch templateType {
	case "tengah_semester":
		return "Rapor tengah semester (3 bulan)"
	case "akhir_semester":
		return "Rapor akhir semester (6 bulan)"
	case "akhir_semester_hal2":
		return "Rapor akhir semester (6 bulan) Halaman 2"
	}
	return templateType
}

// usableDocx: an Import Template slot only helps if it actually holds a Word
// file -- the Halaman 2 slot commonly holds an .xlsx.
func usableDocx(t *model.ReportTemplate) bool {
	return t != nil && strings.HasSuffix(strings.ToLower(t.FileName), ".docx")
}

// missingTemplateMsg explains which slot is empty and, crucially, that a
// template in another slot is not a substitute.
func missingTemplateMsg(rc *model.ReportCard) string {
	return fmt.Sprintf(
		"Sekolah belum mengunggah template Word (.docx) untuk slot %q, dan tidak ada template Word di slot lain yang bisa dipakai. Unggah lewat Generate Rapor > Import Template.",
		templateSlotLabel(templateTypeFor(rc)))
}

// renderCardDocx loads a card in full (subjects, groups, school) and merges
// it through the school's uploaded .docx. Returns a nil body with no error
// when the school has no .docx for that slot -- the caller decides whether
// that's a 404 (single download) or a skip (batch).
func (h *Handler) renderCardDocx(r *http.Request, rc *model.ReportCard) ([]byte, string, error) {
	t, raw, err := h.d.Store.GetReportTemplateFile(r.Context(), rc.SchoolID, templateTypeFor(rc))
	if err != nil {
		return nil, "", err
	}
	if !usableDocx(t) {
		// Report Sekoci V15 item 2: the school HAS a Word rapor template, just
		// in the other slot, and a guru clicking Unduh Word got a dead end that
		// read like the system ignoring the admin's upload. Every slot holds
		// the same merge fields, so falling back to whichever .docx the school
		// did upload produces a usable rapor instead of nothing. The error
		// below still fires when there is genuinely no .docx anywhere.
		for _, alt := range model.ReportTemplateTypes {
			if alt == templateTypeFor(rc) {
				continue
			}
			at, araw, aerr := h.d.Store.GetReportTemplateFile(r.Context(), rc.SchoolID, alt)
			if aerr == nil && usableDocx(at) {
				t, raw = at, araw
				break
			}
		}
	}
	if !usableDocx(t) {
		return nil, "", nil
	}
	subjects, err := h.d.Store.ListSubjects(r.Context(), rc.ID)
	if err != nil {
		return nil, "", err
	}
	bands, _ := h.d.Store.GetGradeBands(r.Context(), rc.SchoolID)
	applyGradeLabels(subjects, bands)
	h.enrichSubjectGroups(r.Context(), rc.ClassroomID, rc.TermID, subjects)
	rc.Subjects = subjects
	rc.School = h.fetchSchoolInfo(r.Context(), rc.SchoolID)
	rc.Fase = h.resolveFase(r.Context(), rc.ClassroomID)

	out, err := docx.Render(raw, buildMergeData(rc))
	if err != nil {
		return nil, "", err
	}
	return out, t.FileName, nil
}

// safeFileName keeps a student's name usable as a download filename.
func safeFileName(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return "rapor"
	}
	repl := func(r rune) rune {
		switch r {
		case '/', '\\', ':', '*', '?', '"', '<', '>', '|', '\n', '\r':
			return '_'
		}
		return r
	}
	return strings.Map(repl, s)
}

// DownloadReportCardDocx: GET /report-cards/{id}/docx -- one student's
// rapor, rendered through the school's uploaded Word template.
func (h *Handler) DownloadReportCardDocx(w http.ResponseWriter, r *http.Request) {
	claims := common.FromCtx(r)
	if !isStaff(claims) {
		common.WriteError(w, http.StatusForbidden, "admin/superadmin/guru only")
		return
	}
	id := chi.URLParam(r, "id")
	rc, err := h.d.Store.GetReportCard(r.Context(), id)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "failed to fetch report card")
		return
	}
	if rc == nil || (claims.ActiveRole != "superadmin" && rc.SchoolID != claims.SchoolID) {
		common.WriteError(w, http.StatusNotFound, "report card not found")
		return
	}
	out, _, err := h.renderCardDocx(r, rc)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "gagal merender template: "+err.Error())
		return
	}
	if out == nil {
		common.WriteError(w, http.StatusNotFound, missingTemplateMsg(rc))
		return
	}
	name := safeFileName(str(rc.StudentName)) + " - Rapor.docx"
	w.Header().Set("Content-Type", "application/vnd.openxmlformats-officedocument.wordprocessingml.document")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", name))
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(out)
}

// DownloadClassroomDocxZip: GET /classrooms/{id}/report-cards/docx?term_id=
// &report_type=&period_label= -- every report card in one classroom/term,
// merged and returned as a single .zip. Wali kelas print a whole class at
// once; doing that one HTTP request per student is what makes the current
// print flow unusable for them.
func (h *Handler) DownloadClassroomDocxZip(w http.ResponseWriter, r *http.Request) {
	claims := common.FromCtx(r)
	if !isStaff(claims) {
		common.WriteError(w, http.StatusForbidden, "admin/superadmin/guru only")
		return
	}
	classroomID := chi.URLParam(r, "id")
	termID := strings.TrimSpace(r.URL.Query().Get("term_id"))
	if termID == "" {
		common.WriteError(w, http.StatusBadRequest, "term_id is required")
		return
	}
	reportType := strings.TrimSpace(r.URL.Query().Get("report_type"))
	if reportType == "" {
		reportType = "semester"
	}
	periodLabel := strings.TrimSpace(r.URL.Query().Get("period_label"))

	cards, err := h.d.Store.ListReportCardsByClassroomTerm(r.Context(), classroomID, termID, reportType, periodLabel)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "failed to fetch report cards")
		return
	}
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	used := make(map[string]int)
	rendered := 0
	for i := range cards {
		rc := &cards[i]
		if claims.ActiveRole != "superadmin" && rc.SchoolID != claims.SchoolID {
			continue
		}
		out, _, err := h.renderCardDocx(r, rc)
		if err != nil {
			common.WriteError(w, http.StatusInternalServerError, "gagal merender template: "+err.Error())
			return
		}
		if out == nil {
			continue
		}
		name := safeFileName(str(rc.StudentName))
		// Two students with the same name must not overwrite each other
		// inside the zip.
		used[name]++
		if used[name] > 1 {
			name = fmt.Sprintf("%s (%d)", name, used[name])
		}
		f, err := zw.Create(name + ".docx")
		if err != nil {
			common.WriteError(w, http.StatusInternalServerError, "failed to build zip")
			return
		}
		if _, err := f.Write(out); err != nil {
			common.WriteError(w, http.StatusInternalServerError, "failed to build zip")
			return
		}
		rendered++
	}
	if rendered == 0 {
		common.WriteError(w, http.StatusNotFound,
			"tidak ada rapor yang bisa dirender -- pastikan template .docx sudah diunggah dan rapor sudah digenerate")
		return
	}
	if err := zw.Close(); err != nil {
		common.WriteError(w, http.StatusInternalServerError, "failed to build zip")
		return
	}
	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", "Rapor Sekelas.zip"))
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(buf.Bytes())
}

// ListMergeParameters: GET /settings/report-template-parameters -- the
// placeholder reference the admin screen shows (and the school hands to
// whoever builds their Word file). Served from the code that actually does
// the merging so the documentation can't drift from the implementation.
func (h *Handler) ListMergeParameters(w http.ResponseWriter, r *http.Request) {
	claims := common.FromCtx(r)
	if !isStaff(claims) {
		common.WriteError(w, http.StatusForbidden, "admin/superadmin/guru only")
		return
	}
	common.WriteJSON(w, http.StatusOK, model.MergeParameterGroups, nil)
}
