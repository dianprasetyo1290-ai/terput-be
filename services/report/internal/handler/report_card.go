package handler

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"skci/report/internal/common"
	"skci/report/internal/model"
	"skci/report/internal/upstream"
)

// classifyMapping buckets an ExamType.MappingRapor value into "sts"/"sas"/
// "" for the Capaian STS/Capaian SAS breakdown Deskripsi Rapor needs
// (client requirement: "belum terdapat capaian sts dan capaian sas" — the
// existing computed_score blends every mapped exam type together with no
// way to tell STS and SAS apart). mapping_rapor is a free-text field admins
// set per exam type (see exam-service; its admin form even hints "mis.
// sas" as an example) — but the real migrated prod data
// (skci_exam.exam_types) actually uses "mid_exam"/"final_exam" as the
// bucket values for "Sumatif Tengah Semester" (STS) / "Sumatif Akhir
// Jenjang|Tahun" exam types, never the literal strings "sts"/"sas".
// Matches both: substring "sts"/"sas" for a school that names its own
// exam type that way, and the mid_exam/final_exam convention the legacy
// system actually shipped. Anything else (UH, Tugas, ...) still counts
// toward the blended computed_score but not either STS/SAS bucket.
func classifyMapping(mappingRapor string) string {
	lower := strings.ToLower(strings.TrimSpace(mappingRapor))
	switch {
	case strings.Contains(lower, "sts") || lower == "mid_exam" || strings.Contains(lower, "tengah semester"):
		return "sts"
	case strings.Contains(lower, "sas") || lower == "final_exam" || strings.Contains(lower, "akhir jenjang") || strings.Contains(lower, "akhir semester"):
		return "sas"
	case strings.Contains(lower, "sumatif"):
		return "sumatif"
	case strings.Contains(lower, "formatif"):
		return "formatif"
	default:
		return "other"
	}
}

// classifyExamType is classifyMapping plus a fallback to the exam type's
// own Name. Real prod data shows mapping_rapor and Name can drift apart —
// e.g. exam_types rows named "Sumatif Tengah Semester"/"Sumatif Akhir
// Jenjang" (the spec's literal categorization anchor: "dikategorikan
// dengan tipe ujian ... merujuk pada master data di fitur Kelola Tipe
// Ujian") but with mapping_rapor mistakenly left as "Semester 1"/"AKHIR
// SEM" — both free-text fields an admin can type anything into. Name is
// checked only when mapping_rapor doesn't resolve to sts/sas, so an
// explicit mapping_rapor still wins.
func classifyExamType(mappingRapor, name string) string {
	if cat := classifyMapping(mappingRapor); cat == "sts" || cat == "sas" {
		return cat
	}
	return classifyMapping(name)
}

// resolveExamBucketScore (item 5): finds one student's exam-based score
// for a subject+bucket ("sts"/"sas") within a classroom+term, for the
// Penilaian Formatif dropdown's STS/SAS auto-populate. Deliberately reuses
// the same ListExams/ListExamTypes/GetScores calls (and classifyExamType
// bucketing) GenerateReportCards already relies on instead of adding a new
// exam-service endpoint -- exam-service's existing GET /exams/{id}/scores
// (all students for one exam) already carries everything needed; this
// just filters client-side for the one student being asked about. A
// small classroom makes the extra round-trips cheap; this is a
// single-teacher-loading-a-screen path, not a hot loop.
func (h *Handler) resolveExamBucketScore(ctx context.Context, classroomID, termID, academicYearID, subjectID, studentID, bucket string) *float64 {
	exams, err := h.d.Exam.ListExams(ctx, classroomID, termID)
	if err != nil {
		return nil
	}
	examTypes, err := h.d.Exam.ListExamTypes(ctx, academicYearID)
	if err != nil {
		return nil
	}
	mappingByType := map[string]string{}
	nameByType := map[string]string{}
	for _, et := range examTypes {
		mappingByType[et.ID] = et.MappingRapor
		nameByType[et.ID] = et.Name
	}
	sum, count := 0.0, 0
	for _, e := range exams {
		if e.SubjectID != subjectID {
			continue
		}
		m := mappingByType[e.ExamTypeID]
		if m == "" || classifyExamType(m, nameByType[e.ExamTypeID]) != bucket {
			continue
		}
		scores, err := h.d.Exam.GetScores(ctx, e.ID)
		if err != nil {
			continue
		}
		for _, sc := range scores {
			if sc.StudentID == studentID && sc.Score != nil {
				sum += *sc.Score
				count++
			}
		}
	}
	if count == 0 {
		return nil
	}
	avg := sum / float64(count)
	return &avg
}

func isAdmin(c *common.Claims) bool {
	return c != nil && (c.ActiveRole == "admin" || c.ActiveRole == "superadmin")
}

func isStaff(c *common.Claims) bool {
	return c != nil && (c.ActiveRole == "admin" || c.ActiveRole == "superadmin" || c.ActiveRole == "guru")
}

// isWaliKelasOwner is the wali-kelas classroom-ownership guard (Bug Report
// SEKOCI GPM 5 Agustus 2026, ROLE WALI KELAS: "generate dan deskripsi rapor
// untuk kelas yang menjadi tanggung jawab Wali Kelas tersebut"). Verified
// live against academic-service's classrooms.homeroom_teacher_user_id (the
// only place this assignment is stored) via the same GetClassroom call
// GenerateReportCards already makes to resolve Wali Kelas's own name for the
// header — never trust a client-supplied classroom_id. false for any
// non-walikelas caller (or on lookup failure) — callers combine this with
// isAdmin/isStaff via OR, so admin/guru/superadmin behavior is unchanged.
func (h *Handler) isWaliKelasOwner(ctx context.Context, c *common.Claims, classroomID string) bool {
	if c == nil || c.ActiveRole != "walikelas" || classroomID == "" {
		return false
	}
	classroom, err := h.d.Academic.GetClassroom(ctx, classroomID)
	if err != nil || classroom == nil || classroom.HomeroomTeacherUserID == nil {
		return false
	}
	return *classroom.HomeroomTeacherUserID == c.UserID && classroom.SchoolID == c.SchoolID
}

// resolveSchoolID applies the multi-tenancy rule from CONVENTIONS.md:
// scope to the caller's school_id, except superadmin may pass ?school_id=.
func resolveSchoolID(r *http.Request, c *common.Claims) string {
	if c != nil && c.ActiveRole == "superadmin" {
		if qs := r.URL.Query().Get("school_id"); qs != "" {
			return qs
		}
	}
	if c == nil {
		return ""
	}
	return c.SchoolID
}

func (h *Handler) GenerateReportCards(w http.ResponseWriter, r *http.Request) {
	claims := common.FromCtx(r)
	classroomID := r.URL.Query().Get("classroom_id")
	termID := r.URL.Query().Get("term_id")
	if classroomID == "" || termID == "" {
		common.WriteError(w, http.StatusBadRequest, "classroom_id and term_id query params are required")
		return
	}
	// Wali kelas's "Generate & Deskripsi Rapor" menu reuses this exact
	// endpoint, scoped to their own classroom (see isWaliKelasOwner) —
	// admin/superadmin behavior is unchanged.
	if !isAdmin(claims) && !h.isWaliKelasOwner(r.Context(), claims, classroomID) {
		common.WriteError(w, http.StatusForbidden, "only admin/superadmin, or the wali kelas of this classroom, can generate report cards")
		return
	}
	// Item 3: Rapot Bayangan is the same generate flow, just a different
	// report_type (which weight formula/subject columns get populated,
	// see calcSubjectScores) and a required period_label (e.g. "Triwulan
	// 1") since it's a quarterly report, not tied 1:1 to a semester term.
	reportType := r.URL.Query().Get("report_type")
	if reportType == "" {
		reportType = "semester"
	}
	if reportType != "semester" && reportType != "bayangan" {
		common.WriteError(w, http.StatusBadRequest, "report_type must be 'semester' or 'bayangan'")
		return
	}
	periodLabel := r.URL.Query().Get("period_label")
	if reportType == "bayangan" && periodLabel == "" {
		common.WriteError(w, http.StatusBadRequest, "period_label is required when report_type=bayangan (e.g. 'Triwulan 1')")
		return
	}
	schoolID := resolveSchoolID(r, claims)
	if schoolID == "" {
		common.WriteError(w, http.StatusBadRequest, "school_id could not be resolved (superadmin must pass ?school_id=)")
		return
	}
	weights, err := h.d.Store.GetWeightSettings(r.Context(), schoolID)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "failed to load weight settings")
		return
	}

	ctx := r.Context()
	warnings := map[string]string{}

	var classroomName, termName, homeroomTeacherName, principalName *string
	var academicYearID string

	if classroom, err := h.d.Academic.GetClassroom(ctx, classroomID); err != nil {
		warnings["academic_classroom"] = err.Error()
	} else if classroom != nil {
		academicYearID = classroom.AcademicYearID
		n := classroom.Name
		classroomName = &n

		// Wali Kelas: academic-service already stores the homeroom teacher
		// assignment on the classroom itself (Data Kelas admin form) --
		// just resolve that user's display name from identity-service, the
		// same as every other name lookup in this handler.
		if classroom.HomeroomTeacherUserID != nil {
			if teacher, terr := h.d.Identity.GetUser(ctx, *classroom.HomeroomTeacherUserID); terr != nil {
				warnings["identity_wali_kelas"] = terr.Error()
			} else if teacher != nil {
				n := teacher.Name
				homeroomTeacherName = &n
			}
		}
	}

	// Kepala Sekolah: not tied to this classroom at all -- it's whoever
	// currently holds the "Kepala Sekolah" jabatan for the school, per
	// identity-service's Organisasi Sekolah (Data Jabatan/Peran) feature.
	if orgPositions, oerr := h.d.Identity.GetOrganization(ctx, schoolID); oerr != nil {
		warnings["identity_kepala_sekolah"] = oerr.Error()
	} else if name := upstream.PrincipalName(orgPositions); name != "" {
		principalName = &name
	}
	if term, err := h.d.Academic.GetTerm(ctx, termID); err != nil {
		warnings["academic_term"] = err.Error()
	} else if term != nil {
		n := term.Name
		termName = &n
		if academicYearID == "" {
			academicYearID = term.AcademicYearID
		}
	}

	roster, err := h.d.Academic.ListClassroomStudents(ctx, classroomID, termID)
	if err != nil {
		// Hard dependency: without a roster we don't know which students to
		// generate cards for at all. This is the one call generate cannot
		// degrade around gracefully into partial rows — but we still answer
		// with a normal 200 + explicit warning, never a 500/crash.
		warnings["academic_roster"] = err.Error()
		common.WriteJSON(w, http.StatusOK, map[string]interface{}{
			"classroom_id": classroomID,
			"term_id":      termID,
			"created":      0,
			"refreshed":    0,
			"report_cards": []interface{}{},
			"warnings":     warnings,
		}, nil)
		return
	}
	if academicYearID == "" {
		warnings["academic_year"] = "could not resolve academic_year_id from classroom/term lookups"
		common.WriteJSON(w, http.StatusOK, map[string]interface{}{
			"classroom_id": classroomID,
			"term_id":      termID,
			"created":      0,
			"refreshed":    0,
			"report_cards": []interface{}{},
			"warnings":     warnings,
		}, nil)
		return
	}

	subjects, err := h.d.Academic.ListClassroomSubjects(ctx, classroomID, termID)
	if err != nil {
		warnings["academic_subjects"] = err.Error()
	}

	exams, err := h.d.Exam.ListExams(ctx, classroomID, termID)
	if err != nil {
		warnings["exam_exams"] = err.Error()
	}
	examTypes, err := h.d.Exam.ListExamTypes(ctx, academicYearID)
	if err != nil {
		warnings["exam_types"] = err.Error()
	}
	mappingByExamType := map[string]string{}
	nameByExamType := map[string]string{}
	for _, et := range examTypes {
		mappingByExamType[et.ID] = et.MappingRapor
		nameByExamType[et.ID] = et.Name
	}

	// examCategory only tracks "sts"/"sas" now -- item 2 restructures Nilai
	// Akhir into an explicit Formatif/STS/SAS weighted formula (see
	// calcSubjectScores below), so exams that classify as anything else
	// (old "sumatif"/"formatif"/"other" pooling) no longer feed a report
	// column; only exam-service's STS/SAS results still do, and Nilai
	// Formatif is now sourced purely from formative_grades (item 5's
	// dropdown-entered scores), not blended with misc exam scores.
	examsBySubject := map[string][]string{}
	examCategory := map[string]string{} // exam ID -> "sts" | "sas" | ""
	for _, e := range exams {
		m := mappingByExamType[e.ExamTypeID]
		if m == "" {
			continue // not mapped to any report bucket, e.g. old "TKA" type
		}
		cat := classifyExamType(m, nameByExamType[e.ExamTypeID])
		if cat != "sts" && cat != "sas" {
			continue
		}
		examsBySubject[e.SubjectID] = append(examsBySubject[e.SubjectID], e.ID)
		examCategory[e.ID] = cat
	}
	scoresByExam := map[string]map[string]*float64{}
	for _, e := range exams {
		if mappingByExamType[e.ExamTypeID] == "" {
			continue
		}
		scores, err := h.d.Exam.GetScores(ctx, e.ID)
		if err != nil {
			warnings["exam_scores_"+e.ID] = err.Error()
			continue
		}
		m := map[string]*float64{}
		for _, sc := range scores {
			m[sc.StudentID] = sc.Score
		}
		scoresByExam[e.ID] = m
	}

	created, refreshed := 0, 0
	cards := []model.ReportCard{}

	for _, stu := range roster {
		studentID := stu.StudentUserID
		rc, isNew, err := h.d.Store.GetOrCreateReportCard(ctx, schoolID, academicYearID, termID, classroomID, studentID, reportType, periodLabel)
		if err != nil || rc == nil {
			warnings["db_"+studentID] = "failed to create report card row"
			continue
		}
		if isNew {
			created++
		} else {
			refreshed++
		}

		// Header snapshot: identity-service is the only source for
		// name/NISN (academic-service's enrollment record doesn't embed
		// them). A down identity-service just leaves the header null
		// rather than crashing.
		var studentName, studentNISN *string
		if user, uerr := h.d.Identity.GetUser(ctx, studentID); uerr != nil {
			warnings["identity_"+studentID] = uerr.Error()
		} else if user != nil {
			n := user.Name
			studentName = &n
			studentNISN = user.NISN
		}
		_ = h.d.Store.UpdateHeaderSnapshot(ctx, rc.ID, studentName, studentNISN, classroomName, termName, homeroomTeacherName, principalName, time.Now())

		// A finalized card is locked: don't let a regenerate silently
		// change attendance/computed scores under a locked report. Admin
		// must PUT /unlock first.
		if rc.Status != "draft" {
			full, _ := h.loadReportCard(ctx, rc.ID)
			if full != nil {
				cards = append(cards, *full)
			}
			continue
		}

		var hadir, sakit, izin, tanpa *int
		var attErr *string
		if summary, aerr := h.d.Attendance.GetSummary(ctx, studentID, termID, academicYearID); aerr != nil {
			e := aerr.Error()
			attErr = &e
			warnings["attendance_"+studentID] = aerr.Error()
		} else if summary != nil {
			hd, sk, iz, tk := summary.Hadir, summary.Sakit, summary.Izin, summary.TanpaKeterangan
			hadir, sakit, izin, tanpa = &hd, &sk, &iz, &tk
		}
		_ = h.d.Store.UpdateAttendance(ctx, rc.ID, hadir, sakit, izin, tanpa, nowPtr(), attErr)
		// Note: attendance is still recorded/displayed on the report card
		// header (above) but item 2 dropped it from the Nilai Akhir formula
		// -- the client's explicit spec is "Nilai Akhir dari Nilai Formatif
		// + Nilai STS + Nilai SAS" only (see calcSubjectScores), superseding
		// the old ad-hoc 90/10 academic/attendance blend that used to live
		// here (that was this codebase's own "simplest reasonable call",
		// never a client requirement).

		// Agama religion-based filtering (item 13, V4, same rule already
		// applied in exam/attendance services): `subjects` above is every
		// subject assigned to the WHOLE classroom, so without this a
		// student got a report_card_subjects row (and a "Belum ada data"
		// placeholder) for every OTHER religion's Agama variant too, not
		// just their own. nil (lookup failed) fails open -- don't drop a
		// student's real subjects just because academic-service hiccuped.
		allowedSubjects := h.d.Academic.ActiveSubjectIDs(ctx, studentID, schoolID, academicYearID)
		if allowedSubjects != nil {
			allowedIDs := make([]string, 0, len(allowedSubjects))
			for sid := range allowedSubjects {
				allowedIDs = append(allowedIDs, sid)
			}
			if err := h.d.Store.DeleteSubjectsNotIn(ctx, rc.ID, allowedIDs); err != nil {
				warnings["subject_cleanup_"+studentID] = err.Error()
			}
		}
		for _, subj := range subjects {
			if allowedSubjects != nil && !allowedSubjects[subj.SubjectID] {
				continue
			}
			var sErr *string
			examIDs := examsBySubject[subj.SubjectID]
			if len(examIDs) == 0 && exams == nil {
				e := "exam-service unavailable"
				sErr = &e
			}

			// Nilai STS / Nilai SAS: averaged over this subject's exams
			// whose ExamType.mapping_rapor classifies as that bucket (same
			// classifyExamType used everywhere else in this handler).
			stsSum, sasSum := 0.0, 0.0
			stsCount, sasCount := 0, 0
			for _, eid := range examIDs {
				scoresForExam, ok := scoresByExam[eid]
				if !ok {
					continue
				}
				sc, ok := scoresForExam[studentID]
				if !ok || sc == nil {
					continue
				}
				switch examCategory[eid] {
				case "sts":
					stsSum += *sc
					stsCount++
				case "sas":
					sasSum += *sc
					sasCount++
				}
			}
			var stsScore, sasScore *float64
			if stsCount > 0 {
				avg := stsSum / float64(stsCount)
				stsScore = &avg
			}
			if sasCount > 0 {
				avg := sasSum / float64(sasCount)
				sasScore = &avg
			}

			// Nilai Formatif / Nilai Praktik: item 5's dropdown-entered
			// formative_grades, pooled per bucket (weighted by each
			// assessment type's admin-configured weight_percent).
			buckets, berr := h.d.Store.GetFormativeBucketScores(ctx, schoolID, termID, subj.ID, studentID)
			if berr != nil {
				w := "failed to load formative grades"
				sErr = &w
			}
			var formatifScore, praktikScore *float64
			count := stsCount + sasCount
			if b, ok := buckets["formatif"]; ok {
				formatifScore = b.Score
				count += b.Count
			}
			if b, ok := buckets["praktik"]; ok {
				praktikScore = b.Score
				count += b.Count
			}

			// Item 2/3: Nilai Akhir (semester) / Nilai Sementara (bayangan)
			// are each an admin-configurable weighted formula over the
			// buckets above -- see report_weight_settings (item 4). A
			// missing bucket (e.g. no STS exam yet) is excluded rather than
			// treated as 0 (model.WeightedAverage renormalizes over what's
			// present), so a subject isn't unfairly dragged down just
			// because one input hasn't arrived yet.
			var computed *float64
			if reportType == "bayangan" {
				// Client's text literally says "kalkulasi dari nilai
				// formatif dan sts" but the weight list right next to it
				// gives three components (formatif 30/praktik 40/sts 30) --
				// using all three since that's what the weights describe;
				// the two-component sentence appears to be the source
				// text's own inconsistency, not a deliberate exclusion of
				// Nilai Praktik (which the client lists as its own column).
				computed = model.WeightedAverage(
					model.WeightedPair{Value: formatifScore, Weight: weights.SementaraFormatifWeight},
					model.WeightedPair{Value: praktikScore, Weight: weights.SementaraPraktikWeight},
					model.WeightedPair{Value: stsScore, Weight: weights.SementaraStsWeight},
				)
			} else {
				computed = model.WeightedAverage(
					model.WeightedPair{Value: formatifScore, Weight: weights.AkhirFormatifWeight},
					model.WeightedPair{Value: stsScore, Weight: weights.AkhirStsWeight},
					model.WeightedPair{Value: sasScore, Weight: weights.AkhirSasWeight},
				)
			}

			name, code := subj.Name, subj.Code
			if reportType == "bayangan" {
				// sas_score doesn't apply to Rapot Bayangan's column set.
				_ = h.d.Store.UpsertSubjectComputed(ctx, rc.ID, schoolID, subj.SubjectID, &name, &code, computed, formatifScore, stsScore, nil, praktikScore, count, nowPtr(), sErr)
			} else {
				// praktik_score doesn't apply to the semester report's column set.
				_ = h.d.Store.UpsertSubjectComputed(ctx, rc.ID, schoolID, subj.SubjectID, &name, &code, computed, formatifScore, stsScore, sasScore, nil, count, nowPtr(), sErr)
			}
		}

		full, _ := h.loadReportCard(ctx, rc.ID)
		if full != nil {
			cards = append(cards, *full)
		}
	}

	common.WriteJSON(w, http.StatusOK, map[string]interface{}{
		"classroom_id": classroomID,
		"term_id":      termID,
		"created":      created,
		"refreshed":    refreshed,
		"report_cards": cards,
		"warnings":     warnings,
	}, nil)
}

// loadReportCard fetches a report card header + subject rows + grade
// labels, ready for a JSON response. Returns nil, nil if the report card
// doesn't exist.
func (h *Handler) loadReportCard(ctx context.Context, id string) (*model.ReportCard, error) {
	rc, err := h.d.Store.GetReportCard(ctx, id)
	if err != nil || rc == nil {
		return rc, err
	}
	subjects, err := h.d.Store.ListSubjects(ctx, id)
	if err != nil {
		return nil, err
	}
	bands, _ := h.d.Store.GetGradeBands(ctx, rc.SchoolID)
	applyGradeLabels(subjects, bands)
	rc.Subjects = subjects
	return rc, nil
}

// applyGradeLabels resolves predikat labels against a school's grade
// bands: GradeLabel for the blended final score (as before), plus
// StsLabel/SasLabel (item 2's "Capaian STS"/"Capaian SAS") applying the
// exact same bands mechanism to StsScore/SasScore individually. Computed
// at read time everywhere a report card is returned, never stored.
func applyGradeLabels(subjects []model.ReportCardSubject, bands []model.GradeBand) {
	for i := range subjects {
		if f := subjects[i].Final(); f != nil {
			if label := model.LabelFor(*f, bands); label != "" {
				subjects[i].GradeLabel = &label
			}
		}
		if subjects[i].StsScore != nil {
			if label := model.LabelFor(*subjects[i].StsScore, bands); label != "" {
				subjects[i].StsLabel = &label
			}
		}
		if subjects[i].SasScore != nil {
			if label := model.LabelFor(*subjects[i].SasScore, bands); label != "" {
				subjects[i].SasLabel = &label
			}
		}
	}
}

func (h *Handler) GetReportCard(w http.ResponseWriter, r *http.Request) {
	claims := common.FromCtx(r)
	id := chi.URLParam(r, "id")

	rc, err := h.d.Store.GetReportCard(r.Context(), id)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "failed to fetch report card")
		return
	}
	if rc == nil {
		common.WriteError(w, http.StatusNotFound, "report card not found")
		return
	}
	if !authorizeReportCardAccess(claims, rc.SchoolID, rc.StudentID) {
		common.WriteError(w, http.StatusNotFound, "report card not found")
		return
	}
	// authorizeReportCardAccess above treats any non-siswa/non-superadmin
	// same-school role (admin/guru/walikelas alike) as authorized — walikelas
	// needs the extra classroom-ownership restriction on top, so viewing/
	// printing a report card (Generate & Deskripsi Rapor's "Lihat
	// Detail"/print links) stays scoped to their own classroom.
	if claims.ActiveRole == "walikelas" && !h.isWaliKelasOwner(r.Context(), claims, rc.ClassroomID) {
		common.WriteError(w, http.StatusNotFound, "report card not found")
		return
	}

	subjects, err := h.d.Store.ListSubjects(r.Context(), id)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "failed to fetch subjects")
		return
	}
	bands, _ := h.d.Store.GetGradeBands(r.Context(), rc.SchoolID)
	applyGradeLabels(subjects, bands)
	h.enrichSubjectGroups(r.Context(), rc.ClassroomID, rc.TermID, subjects)
	rc.Subjects = subjects
	rc.School = h.fetchSchoolInfo(r.Context(), rc.SchoolID)
	common.WriteJSON(w, http.StatusOK, rc, nil)
}

// fetchSchoolInfo resolves the printable letterhead/signature-block school
// identity live from identity-service. Deliberately not cached anywhere
// (see model.ReportCard.School's doc comment) -- called only from the
// single-report-card GetReportCard read, never from a per-row loop, so one
// extra upstream call per read is cheap and keeps every school seeing its
// own name/address/logo, never another school's (multi-tenancy
// requirement). Returns nil (not an error) on any failure -- the print view
// falls back to blank letterhead fields rather than crashing.
func (h *Handler) fetchSchoolInfo(ctx context.Context, schoolID string) *model.SchoolInfo {
	school, err := h.d.Identity.GetSchool(ctx, schoolID)
	if err != nil || school == nil {
		return nil
	}
	return &model.SchoolInfo{
		Name:     school.Name,
		Address:  school.Address,
		City:     school.City,
		Province: school.Province,
		Logo:     school.Logo,
	}
}

// enrichSubjectGroups resolves each subject row's print-template grouping
// (see model.ReportCardSubject.SubjectGroup) live from academic-service's
// classroom subject list, keyed by subject_id. Same non-cached, single-read
// rationale as fetchSchoolInfo. A failed lookup just leaves every
// SubjectGroup nil -- the print view's flat-list fallback handles that.
func (h *Handler) enrichSubjectGroups(ctx context.Context, classroomID, termID string, subjects []model.ReportCardSubject) {
	classroomSubjects, err := h.d.Academic.ListClassroomSubjects(ctx, classroomID, termID)
	if err != nil {
		return
	}
	groupBySubjectID := make(map[string]*string, len(classroomSubjects))
	for _, cs := range classroomSubjects {
		groupBySubjectID[cs.SubjectID] = cs.Group
	}
	for i := range subjects {
		if g, ok := groupBySubjectID[subjects[i].SubjectID]; ok {
			subjects[i].SubjectGroup = g
		}
	}
}

// authorizeReportCardAccess enforces: siswa can only see their own report
// card; admin/guru/superadmin are scoped to their school (superadmin
// unrestricted).
func authorizeReportCardAccess(c *common.Claims, schoolID, studentID string) bool {
	if c == nil {
		return false
	}
	if c.ActiveRole == "siswa" {
		return c.UserID == studentID
	}
	if c.ActiveRole == "superadmin" {
		return true
	}
	return c.SchoolID == schoolID
}

func (h *Handler) ListStudentReportCards(w http.ResponseWriter, r *http.Request) {
	claims := common.FromCtx(r)
	studentID := chi.URLParam(r, "id")
	termID := r.URL.Query().Get("term_id")

	if claims == nil {
		common.WriteError(w, http.StatusUnauthorized, "missing token")
		return
	}
	if claims.ActiveRole == "siswa" && claims.UserID != studentID {
		common.WriteError(w, http.StatusForbidden, "students may only view their own report cards")
		return
	}

	cards, err := h.d.Store.ListReportCardsByStudent(r.Context(), studentID, termID)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "failed to fetch report cards")
		return
	}

	out := make([]model.ReportCard, 0, len(cards))
	for _, rc := range cards {
		if claims.ActiveRole != "superadmin" && claims.ActiveRole != "siswa" && rc.SchoolID != claims.SchoolID {
			continue
		}
		// Same classroom-ownership restriction as GetReportCard: walikelas
		// only ever sees report cards belonging to their own classroom.
		if claims.ActiveRole == "walikelas" && !h.isWaliKelasOwner(r.Context(), claims, rc.ClassroomID) {
			continue
		}
		subjects, err := h.d.Store.ListSubjects(r.Context(), rc.ID)
		if err == nil {
			bands, _ := h.d.Store.GetGradeBands(r.Context(), rc.SchoolID)
			applyGradeLabels(subjects, bands)
			rc.Subjects = subjects
		}
		out = append(out, rc)
	}
	common.WriteJSON(w, http.StatusOK, out, nil)
}

type overrideBody struct {
	OverrideScore *float64 `json:"override_score"`
	Notes         *string  `json:"notes"`
	Clear         bool     `json:"clear"` // if true, clears the override even though override_score is omitted/null
}

func (h *Handler) PatchSubjectOverride(w http.ResponseWriter, r *http.Request) {
	claims := common.FromCtx(r)
	if !isAdmin(claims) {
		common.WriteError(w, http.StatusForbidden, "only admin/superadmin can override scores")
		return
	}
	id := chi.URLParam(r, "id")
	subjectID := chi.URLParam(r, "subject_id")

	rc, err := h.d.Store.GetReportCard(r.Context(), id)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "failed to fetch report card")
		return
	}
	if rc == nil {
		common.WriteError(w, http.StatusNotFound, "report card not found")
		return
	}
	if claims.ActiveRole != "superadmin" && rc.SchoolID != claims.SchoolID {
		common.WriteError(w, http.StatusNotFound, "report card not found")
		return
	}
	if rc.Status == "finalized" {
		common.WriteError(w, http.StatusConflict, "report card is finalized; PUT /report-cards/{id}/unlock first")
		return
	}

	var body overrideBody
	if r.Body != nil {
		defer r.Body.Close()
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			common.WriteError(w, http.StatusBadRequest, "invalid JSON body")
			return
		}
	}
	if body.Clear {
		body.OverrideScore = nil
	}

	err = h.d.Store.PatchSubjectOverride(r.Context(), id, subjectID, body.OverrideScore, body.Notes, claims.UserID)
	if errors.Is(err, sql.ErrNoRows) {
		common.WriteError(w, http.StatusNotFound, "subject not found on this report card")
		return
	}
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "failed to save override")
		return
	}

	full, err := h.loadReportCard(r.Context(), id)
	if err != nil || full == nil {
		common.WriteError(w, http.StatusInternalServerError, "override saved but failed to reload report card")
		return
	}
	common.WriteJSON(w, http.StatusOK, full, nil)
}

func (h *Handler) FinalizeReportCard(w http.ResponseWriter, r *http.Request) {
	claims := common.FromCtx(r)
	if !isAdmin(claims) {
		common.WriteError(w, http.StatusForbidden, "only admin/superadmin can finalize report cards")
		return
	}
	id := chi.URLParam(r, "id")
	rc, err := h.d.Store.GetReportCard(r.Context(), id)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "failed to fetch report card")
		return
	}
	if rc == nil {
		common.WriteError(w, http.StatusNotFound, "report card not found")
		return
	}
	if claims.ActiveRole != "superadmin" && rc.SchoolID != claims.SchoolID {
		common.WriteError(w, http.StatusNotFound, "report card not found")
		return
	}
	if rc.Status == "finalized" {
		common.WriteError(w, http.StatusConflict, "already finalized")
		return
	}
	ok, err := h.d.Store.FinalizeReportCard(r.Context(), id, claims.UserID)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "failed to finalize")
		return
	}
	if !ok {
		common.WriteError(w, http.StatusConflict, "could not finalize (already finalized?)")
		return
	}
	full, _ := h.loadReportCard(r.Context(), id)
	common.WriteJSON(w, http.StatusOK, full, nil)
}

func (h *Handler) UnlockReportCard(w http.ResponseWriter, r *http.Request) {
	claims := common.FromCtx(r)
	if !isAdmin(claims) {
		common.WriteError(w, http.StatusForbidden, "only admin/superadmin can unlock report cards")
		return
	}
	id := chi.URLParam(r, "id")
	rc, err := h.d.Store.GetReportCard(r.Context(), id)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "failed to fetch report card")
		return
	}
	if rc == nil {
		common.WriteError(w, http.StatusNotFound, "report card not found")
		return
	}
	if claims.ActiveRole != "superadmin" && rc.SchoolID != claims.SchoolID {
		common.WriteError(w, http.StatusNotFound, "report card not found")
		return
	}
	if rc.Status != "finalized" {
		common.WriteError(w, http.StatusConflict, "report card is not finalized")
		return
	}
	ok, err := h.d.Store.UnlockReportCard(r.Context(), id, claims.UserID)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "failed to unlock")
		return
	}
	if !ok {
		common.WriteError(w, http.StatusConflict, "could not unlock (not finalized?)")
		return
	}
	full, _ := h.loadReportCard(r.Context(), id)
	common.WriteJSON(w, http.StatusOK, full, nil)
}
