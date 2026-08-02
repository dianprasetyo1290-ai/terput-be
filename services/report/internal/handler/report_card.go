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
	case strings.Contains(lower, "sts"), lower == "mid_exam":
		return "sts"
	case strings.Contains(lower, "sas"), lower == "final_exam":
		return "sas"
	default:
		return ""
	}
}

func isAdmin(c *common.Claims) bool {
	return c != nil && (c.ActiveRole == "admin" || c.ActiveRole == "superadmin")
}

func isStaff(c *common.Claims) bool {
	return c != nil && (c.ActiveRole == "admin" || c.ActiveRole == "superadmin" || c.ActiveRole == "guru")
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
	if !isAdmin(claims) {
		common.WriteError(w, http.StatusForbidden, "only admin/superadmin can generate report cards")
		return
	}
	classroomID := r.URL.Query().Get("classroom_id")
	termID := r.URL.Query().Get("term_id")
	if classroomID == "" || termID == "" {
		common.WriteError(w, http.StatusBadRequest, "classroom_id and term_id query params are required")
		return
	}
	schoolID := resolveSchoolID(r, claims)
	if schoolID == "" {
		common.WriteError(w, http.StatusBadRequest, "school_id could not be resolved (superadmin must pass ?school_id=)")
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
	for _, et := range examTypes {
		mappingByExamType[et.ID] = et.MappingRapor
	}

	examsBySubject := map[string][]string{}
	examCategory := map[string]string{} // exam ID -> "sts" | "sas" | ""
	for _, e := range exams {
		m := mappingByExamType[e.ExamTypeID]
		if m == "" {
			continue // not mapped to any report bucket, e.g. old "TKA" type
		}
		examsBySubject[e.SubjectID] = append(examsBySubject[e.SubjectID], e.ID)
		examCategory[e.ID] = classifyMapping(m)
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
		rc, isNew, err := h.d.Store.GetOrCreateReportCard(ctx, schoolID, academicYearID, termID, classroomID, studentID)
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

		for _, subj := range subjects {
			var computed, stsScore, sasScore *float64
			count := 0
			var sErr *string
			examIDs := examsBySubject[subj.SubjectID]
			if len(examIDs) == 0 {
				if exams == nil {
					e := "exam-service unavailable"
					sErr = &e
				}
			} else {
				sum := 0.0
				stsSum, sasSum := 0.0, 0.0
				stsCount, sasCount := 0, 0
				for _, eid := range examIDs {
					scoresForExam, ok := scoresByExam[eid]
					if !ok {
						continue
					}
					if sc, ok := scoresForExam[studentID]; ok && sc != nil {
						sum += *sc
						count++
						switch examCategory[eid] {
						case "sts":
							stsSum += *sc
							stsCount++
						case "sas":
							sasSum += *sc
							sasCount++
						}
					}
				}
				if count > 0 {
					avg := sum / float64(count)
					computed = &avg
				}
				if stsCount > 0 {
					avg := stsSum / float64(stsCount)
					stsScore = &avg
				}
				if sasCount > 0 {
					avg := sasSum / float64(sasCount)
					sasScore = &avg
				}
			}
			formativeSum, formativeCount := h.d.Store.SumFormativeGrades(ctx, termID, subj.ID, studentID)
			if formativeCount > 0 {
				sum := formativeSum + (func() float64 {
					if computed != nil {
						return *computed * float64(count)
					}
					return 0
				}())
				totalCount := count + formativeCount
				avg := sum / float64(totalCount)
				computed = &avg
				count = totalCount
			}
			name, code := subj.Name, subj.Code
			_ = h.d.Store.UpsertSubjectComputed(ctx, rc.ID, schoolID, subj.SubjectID, &name, &code, computed, stsScore, sasScore, count, nowPtr(), sErr)
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
	for i := range subjects {
		if f := subjects[i].Final(); f != nil {
			if label := model.LabelFor(*f, bands); label != "" {
				subjects[i].GradeLabel = &label
			}
		}
	}
	rc.Subjects = subjects
	return rc, nil
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

	subjects, err := h.d.Store.ListSubjects(r.Context(), id)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "failed to fetch subjects")
		return
	}
	bands, _ := h.d.Store.GetGradeBands(r.Context(), rc.SchoolID)
	for i := range subjects {
		if f := subjects[i].Final(); f != nil {
			label := model.LabelFor(*f, bands)
			if label != "" {
				subjects[i].GradeLabel = &label
			}
		}
	}
	rc.Subjects = subjects
	common.WriteJSON(w, http.StatusOK, rc, nil)
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
		subjects, err := h.d.Store.ListSubjects(r.Context(), rc.ID)
		if err == nil {
			bands, _ := h.d.Store.GetGradeBands(r.Context(), rc.SchoolID)
			for i := range subjects {
				if f := subjects[i].Final(); f != nil {
					if label := model.LabelFor(*f, bands); label != "" {
						subjects[i].GradeLabel = &label
					}
				}
			}
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
