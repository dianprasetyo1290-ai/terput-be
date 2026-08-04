package handler

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	"skci/exam/internal/common"
	"skci/exam/internal/httpclient"
	"skci/exam/internal/model"
)

type rosterRow struct {
	No           int      `json:"no"`
	AttemptID    string   `json:"attempt_id"`
	UserID       string   `json:"user_id"`
	NamaSiswa    string   `json:"nama_siswa"`
	WaktuSelesai *string  `json:"waktu_selesai"`
	JawabanBenar string   `json:"jawaban_benar"` // e.g. "15/20"
	Nilai        *float64 `json:"nilai"`
	Status       string   `json:"status"`
}

// Monitor: live roster for a published exam, across EVERY classroom it
// targets.
//
// Item 10 fix (V3) originally pinned this to the exam's single classroom_id,
// since back then an exam only ever had one. Migration 0005 made exams
// many-to-many with classrooms (exam_classrooms) so a guru creating one exam
// for both 1-A and 1-B no longer gets two duplicate exam rows — but that
// means Monitor pinning to exam.ClassroomID (just the "primary" one) was
// silently hiding every student in the exam's OTHER classrooms from the
// roster/counters. classroomIDs is now the full target-audience set
// (exam.ClassroomIDs, populated by GetExam from exam_classrooms); the roster
// unions each classroom's full class list (fetched from academic-service),
// left-joined with exam_attempts across the WHOLE exam (not filtered to one
// classroom) — a student who was issued a voucher but has never opened the
// exam yet still shows up as "belum_mulai" instead of being invisible.
func (h *Handler) Monitor(w http.ResponseWriter, r *http.Request) {
	c := common.FromCtx(r)
	if !requireRole(w, c, "admin", "superadmin", "guru") {
		return
	}
	examID := chi.URLParam(r, "id")
	exam, err := h.Store.GetExam(r.Context(), examID, common.EffectiveSchoolID(r, c))
	if err != nil {
		writeStoreErr(w, err)
		return
	}
	if exam == nil {
		common.WriteError(w, http.StatusNotFound, "exam not found")
		return
	}
	if c.ActiveRole == "guru" && exam.TeacherID != c.UserID {
		common.WriteError(w, http.StatusForbidden, "you may only monitor your own exams")
		return
	}
	classroomIDs := exam.ClassroomIDs
	if len(classroomIDs) == 0 {
		classroomIDs = []string{exam.ClassroomID}
	}

	attempts, err := h.Store.ListAttemptsByExam(r.Context(), examID)
	if err != nil {
		writeStoreErr(w, err)
		return
	}
	examQuestions, err := h.Store.ListExamQuestions(r.Context(), examID)
	if err != nil {
		writeStoreErr(w, err)
		return
	}
	questionCount := len(examQuestions)

	attemptByUser := make(map[string]model.ExamAttempt, len(attempts))
	for _, a := range attempts {
		attemptByUser[a.UserID] = a
	}

	// Best-effort full roster, unioned across every classroom this exam
	// targets; if academic-service is unreachable for one, the others still
	// come through rather than the whole roster going empty. addRow's `seen`
	// guard also protects against a student double-counted if a classroom
	// lookup ever returned them more than once.
	var roster []httpclient.ClassroomStudentDTO
	for _, cid := range classroomIDs {
		roster = append(roster, h.Internal.ClassroomStudents(cid)...)
	}
	seen := make(map[string]bool, len(roster))
	rows := make([]rosterRow, 0, len(roster)+len(attempts))
	counts := &monitorCounters{}

	addRow := func(userID string) {
		if seen[userID] {
			return
		}
		seen[userID] = true
		a, hasAttempt := attemptByUser[userID]

		// Item 13 / Filter Constraint: If the student doesn't have an attempt yet,
		// only show them in the monitor list if they are actually enrolled in this
		// specific subject. This perfectly handles Agama exams (e.g. Agama Kristen
		// only shows Christian students) because ActiveSubjectIDs already resolves
		// religion exclusivity. We fail-open (include them) if academic-service
		// is unreachable (nil). Students who ALREADY have an attempt are never hidden.
		if !hasAttempt {
			activeSubjects := h.Internal.ActiveSubjectIDs(exam.SchoolID, userID, exam.AcademicYearID)
			if activeSubjects != nil && !activeSubjects[exam.SubjectID] {
				return
			}
		}

		row := rosterRow{
			No:           len(rows) + 1,
			UserID:       userID,
			NamaSiswa:    h.Internal.UserName(userID), // best-effort; "" if identity-service unreachable, see README
			Status:       model.StatusBelumMulai,
			JawabanBenar: "-/" + itoaSimple(questionCount),
		}
		if hasAttempt {
			row.AttemptID = a.ID
			row.Status = a.Status
			if a.SubmittedAt != nil {
				s := a.SubmittedAt.Format("2006-01-02T15:04:05Z07:00")
				row.WaktuSelesai = &s
			}
			if a.CorrectCount != nil {
				row.JawabanBenar = itoaSimple(*a.CorrectCount) + "/" + itoaSimple(questionCount)
			}
			row.Nilai = a.Score
		}
		rows = append(rows, row)
		countStatus(counts, row.Status)
	}

	for _, s := range roster {
		addRow(s.StudentUserID)
	}
	// Any attempt whose student wasn't in the academic roster (roster call
	// failed, or the student was later removed from the class) still shows
	// up — we never want to silently drop a real in-progress attempt.
	for _, a := range attempts {
		addRow(a.UserID)
	}

	common.WriteJSON(w, http.StatusOK, map[string]interface{}{
		"classroom_id":  exam.ClassroomID, // primary classroom, kept for backward compat
		"classroom_ids": classroomIDs,
		"roster":        rows,
		"counters":      counts,
	}, nil)
}

type monitorCounters struct {
	BelumMengerjakan int `json:"belum_mengerjakan"`
	SedangUjian      int `json:"sedang_ujian"`
	Selesai          int `json:"selesai"`
	DilarangUjian    int `json:"dilarang_ujian"`
	Total            int `json:"total"`
}

func countStatus(c *monitorCounters, status string) {
	switch status {
	case model.StatusBelumMulai:
		c.BelumMengerjakan++
	case model.StatusAktif, model.StatusBlokir:
		c.SedangUjian++
	case model.StatusSelesai:
		c.Selesai++
	case model.StatusDilarang:
		c.DilarangUjian++
	}
	c.Total++
}

func itoaSimple(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var b []byte
	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}
	if neg {
		b = append([]byte{'-'}, b...)
	}
	return string(b)
}

// AttemptDetail: admin/guru view a student's live/completed answers. For
// essay questions this response doubles as the correction UI payload.
func (h *Handler) AttemptDetail(w http.ResponseWriter, r *http.Request) {
	c := common.FromCtx(r)
	if !requireRole(w, c, "admin", "superadmin", "guru") {
		return
	}
	examID := chi.URLParam(r, "id")
	attemptID := chi.URLParam(r, "attemptId")
	exam, err := h.Store.GetExam(r.Context(), examID, common.EffectiveSchoolID(r, c))
	if err != nil {
		writeStoreErr(w, err)
		return
	}
	if exam == nil {
		common.WriteError(w, http.StatusNotFound, "exam not found")
		return
	}
	if c.ActiveRole == "guru" && exam.TeacherID != c.UserID {
		common.WriteError(w, http.StatusForbidden, "you may only view your own exam's attempts")
		return
	}
	attempt, err := h.Store.GetAttemptByID(r.Context(), attemptID, exam.SchoolID)
	if err != nil {
		writeStoreErr(w, err)
		return
	}
	if attempt == nil || attempt.ExamID != examID {
		common.WriteError(w, http.StatusNotFound, "attempt not found")
		return
	}
	examQuestions, err := h.Store.ListExamQuestions(r.Context(), examID)
	if err != nil {
		writeStoreErr(w, err)
		return
	}
	answers, err := h.Store.ListAnswers(r.Context(), attemptID)
	if err != nil {
		writeStoreErr(w, err)
		return
	}
	answerByEQ := map[string]model.ExamAttemptAnswer{}
	for _, a := range answers {
		answerByEQ[a.ExamQuestionID] = a
	}

	type detailRow struct {
		No             int                      `json:"no"`
		ExamQuestionID string                   `json:"exam_question_id"`
		QuestionType   string                   `json:"question_type"`
		Content        string                   `json:"content"`
		Points         float64                  `json:"points"`
		Answer         *model.ExamAttemptAnswer `json:"answer"`
		CanOverride    bool                     `json:"can_override"` // essay only, per spec
	}
	var rows []detailRow
	for i, eq := range examQuestions {
		ans, has := answerByEQ[eq.ID]
		row := detailRow{No: i + 1, ExamQuestionID: eq.ID, Points: eq.Points, CanOverride: false}
		if eq.Question != nil {
			row.QuestionType = eq.Question.QuestionType
			row.Content = eq.Question.Content
			row.CanOverride = eq.Question.QuestionType == "essay"
		}
		if has {
			row.Answer = &ans
		}
		rows = append(rows, row)
	}

	common.WriteJSON(w, http.StatusOK, map[string]interface{}{
		"attempt": attempt,
		"answers": rows,
	}, nil)
}

// --- state-machine actions (admin/guru) ---

type reasonReq struct {
	Alasan string `json:"alasan"`
}

// LarangUjian: admin only, belum_mulai -> dilarang, required Alasan.
func (h *Handler) LarangUjian(w http.ResponseWriter, r *http.Request) {
	c := common.FromCtx(r)
	if !requireRole(w, c, "admin", "superadmin") {
		return
	}
	attemptID, exam, ok := h.loadAttemptForAction(w, r, c)
	if !ok {
		return
	}
	var req reasonReq
	if !common.DecodeJSON(w, r, &req) {
		return
	}
	if req.Alasan == "" {
		common.WriteError(w, http.StatusBadRequest, "alasan is required")
		return
	}
	if err := h.Store.LarangUjian(r.Context(), attemptID, c.UserID, req.Alasan); err != nil {
		writeTransitionErr(w, err)
		return
	}
	common.WriteJSON(w, http.StatusOK, map[string]string{
		"attempt_id": attemptID,
		"message":    "Anda akan melarang siswa dari ujian ini. Siswa tidak akan bisa mengikuti ujian sampai larangan dibuka.",
		"exam_id":    exam.ID,
	}, nil)
}

// LarangUjianByUser: same action as LarangUjian, but for a student who has
// never entered the exam yet (no attempt_id exists for the monitor row to
// key on) — the client's primary scenario for this feature. Route is keyed
// by userId instead of attemptId for exactly that reason.
func (h *Handler) LarangUjianByUser(w http.ResponseWriter, r *http.Request) {
	c := common.FromCtx(r)
	if !requireRole(w, c, "admin", "superadmin") {
		return
	}
	examID := chi.URLParam(r, "id")
	userID := chi.URLParam(r, "userId")
	exam, err := h.Store.GetExam(r.Context(), examID, common.EffectiveSchoolID(r, c))
	if err != nil {
		writeStoreErr(w, err)
		return
	}
	if exam == nil {
		common.WriteError(w, http.StatusNotFound, "exam not found")
		return
	}
	if c.ActiveRole == "guru" && exam.TeacherID != c.UserID {
		common.WriteError(w, http.StatusForbidden, "you may only act on your own exam's attempts")
		return
	}
	var req reasonReq
	if !common.DecodeJSON(w, r, &req) {
		return
	}
	if req.Alasan == "" {
		common.WriteError(w, http.StatusBadRequest, "alasan is required")
		return
	}
	studentClassroomID := h.Internal.StudentClassroomID(userID, exam.SchoolID, exam.AcademicYearID)
	attemptID, err := h.Store.LarangUjianPreEntry(r.Context(), examID, userID, c.UserID, req.Alasan, studentClassroomID)
	if err != nil {
		writeTransitionErr(w, err)
		return
	}
	common.WriteJSON(w, http.StatusOK, map[string]string{
		"attempt_id": attemptID,
		"message":    "Anda akan melarang siswa dari ujian ini. Siswa tidak akan bisa mengikuti ujian sampai larangan dibuka.",
		"exam_id":    exam.ID,
	}, nil)
}

func (h *Handler) BukaLarangan(w http.ResponseWriter, r *http.Request) {
	c := common.FromCtx(r)
	if !requireRole(w, c, "admin", "superadmin") {
		return
	}
	attemptID, _, ok := h.loadAttemptForAction(w, r, c)
	if !ok {
		return
	}
	if err := h.Store.BukaLarangan(r.Context(), attemptID); err != nil {
		writeTransitionErr(w, err)
		return
	}
	common.WriteJSON(w, http.StatusOK, map[string]string{"attempt_id": attemptID, "status": model.StatusBelumMulai}, nil)
}

func (h *Handler) Blokir(w http.ResponseWriter, r *http.Request) {
	c := common.FromCtx(r)
	if !requireRole(w, c, "admin", "superadmin", "guru") {
		return
	}
	attemptID, _, ok := h.loadAttemptForAction(w, r, c)
	if !ok {
		return
	}
	var req reasonReq
	_ = common.DecodeJSON(w, r, &req)
	var reason *string
	if req.Alasan != "" {
		reason = &req.Alasan
	}
	if err := h.Store.Blokir(r.Context(), attemptID, c.UserID, reason); err != nil {
		writeTransitionErr(w, err)
		return
	}
	common.WriteJSON(w, http.StatusOK, map[string]string{"attempt_id": attemptID, "status": model.StatusBlokir}, nil)
}

// BukaBlokir ("Ulangi"): resumes without resetting progress.
func (h *Handler) BukaBlokir(w http.ResponseWriter, r *http.Request) {
	c := common.FromCtx(r)
	if !requireRole(w, c, "admin", "superadmin", "guru") {
		return
	}
	attemptID, _, ok := h.loadAttemptForAction(w, r, c)
	if !ok {
		return
	}
	if err := h.Store.BukaBlokir(r.Context(), attemptID); err != nil {
		writeTransitionErr(w, err)
		return
	}
	common.WriteJSON(w, http.StatusOK, map[string]string{"attempt_id": attemptID, "status": model.StatusAktif}, nil)
}

type overrideAnswerReq struct {
	IsCorrect bool `json:"is_correct"`
}

// OverrideAnswer: guru manually flips an essay answer's correctness. NOT
// permitted for tebak_gambar per spec.
func (h *Handler) OverrideAnswer(w http.ResponseWriter, r *http.Request) {
	c := common.FromCtx(r)
	if !requireRole(w, c, "admin", "superadmin", "guru") {
		return
	}
	attemptID, exam, ok := h.loadAttemptForAction(w, r, c)
	if !ok {
		return
	}
	answerID := chi.URLParam(r, "answerId")
	var req overrideAnswerReq
	if !common.DecodeJSON(w, r, &req) {
		return
	}

	answers, err := h.Store.ListAnswers(r.Context(), attemptID)
	if err != nil {
		writeStoreErr(w, err)
		return
	}
	var target *model.ExamAttemptAnswer
	for i := range answers {
		if answers[i].ID == answerID {
			target = &answers[i]
			break
		}
	}
	if target == nil {
		common.WriteError(w, http.StatusNotFound, "answer not found")
		return
	}
	eq, err := h.Store.GetExamQuestion(r.Context(), target.ExamQuestionID)
	if err != nil {
		writeStoreErr(w, err)
		return
	}
	if eq == nil || eq.Question == nil {
		common.WriteError(w, http.StatusNotFound, "question not found")
		return
	}
	if eq.Question.QuestionType != "essay" {
		common.WriteError(w, http.StatusBadRequest, "manual correctness override is only permitted for essay answers")
		return
	}
	points := 0.0
	if req.IsCorrect {
		points = eq.Points
	}
	if err := h.Store.OverrideAnswerCorrectness(r.Context(), answerID, attemptID, req.IsCorrect, points); err != nil {
		writeStoreErr(w, err)
		return
	}
	_ = exam
	common.WriteJSON(w, http.StatusOK, map[string]interface{}{"answer_id": answerID, "is_correct": req.IsCorrect}, nil)
}

func (h *Handler) loadAttemptForAction(w http.ResponseWriter, r *http.Request, c *common.Claims) (string, *model.Exam, bool) {
	examID := chi.URLParam(r, "id")
	attemptID := chi.URLParam(r, "attemptId")
	exam, err := h.Store.GetExam(r.Context(), examID, common.EffectiveSchoolID(r, c))
	if err != nil {
		writeStoreErr(w, err)
		return "", nil, false
	}
	if exam == nil {
		common.WriteError(w, http.StatusNotFound, "exam not found")
		return "", nil, false
	}
	if c.ActiveRole == "guru" && exam.TeacherID != c.UserID {
		common.WriteError(w, http.StatusForbidden, "you may only act on your own exam's attempts")
		return "", nil, false
	}
	attempt, err := h.Store.GetAttemptByID(r.Context(), attemptID, exam.SchoolID)
	if err != nil {
		writeStoreErr(w, err)
		return "", nil, false
	}
	if attempt == nil || attempt.ExamID != examID {
		common.WriteError(w, http.StatusNotFound, "attempt not found")
		return "", nil, false
	}
	return attemptID, exam, true
}

func writeTransitionErr(w http.ResponseWriter, err error) {
	common.WriteError(w, http.StatusConflict, "invalid state transition: "+err.Error())
}
