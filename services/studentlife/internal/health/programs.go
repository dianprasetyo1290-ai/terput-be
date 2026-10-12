package health

import (
	"context"
	"errors"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"skci/studentlife/internal/client"
	"skci/studentlife/internal/common"
	"skci/studentlife/internal/consent"
	"skci/studentlife/internal/sensitive"
	"skci/studentlife/internal/store"
)

func validateProgramInput(in ProgramInput) error {
	starts, err := ParseDate(in.StartsOn)
	if err != nil {
		return invalid("Tanggal mulai tidak valid")
	}
	ends, err := ParseDate(in.EndsOn)
	if err != nil {
		return invalid("Tanggal selesai tidak valid")
	}
	var deadline *time.Time
	if in.ConsentDeadline != nil && *in.ConsentDeadline != "" {
		d, err := ParseDate(*in.ConsentDeadline)
		if err != nil {
			return invalid("Batas persetujuan tidak valid")
		}
		deadline = &d
	}
	return ValidateProgram(in.Kind, in.Name, starts, ends, in.RequiresConsent, deadline, in.Targets)
}

func normalizeProgram(in *ProgramInput) {
	if in.ConsentDeadline != nil && *in.ConsentDeadline == "" {
		in.ConsentDeadline = nil
	}
	if !in.RequiresConsent {
		in.ConsentDeadline = nil
	}
}

// ListPrograms: GET /health/programs?status=&kind=
func (h *Handler) ListPrograms(w http.ResponseWriter, r *http.Request) {
	_, school := common.RequireRole(w, r, common.RolePetugasUKS)
	if school == "" {
		return
	}
	page, perPage := common.Page(r)
	list, total, err := ListPrograms(r.Context(), h.DB, school, r.URL.Query().Get("status"), r.URL.Query().Get("kind"), page, perPage)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "Gagal memuat program")
		return
	}
	common.WriteSuccess(w, list, common.Meta{Page: page, PerPage: perPage, Total: total})
}

// GetProgram: GET /health/programs/{id}
func (h *Handler) GetProgram(w http.ResponseWriter, r *http.Request) {
	_, school := common.RequireRole(w, r, common.RolePetugasUKS)
	if school == "" {
		return
	}
	p, err := GetProgram(r.Context(), h.DB, school, chi.URLParam(r, "id"))
	if errors.Is(err, store.ErrNotFound) {
		common.WriteError(w, http.StatusNotFound, "Program tidak ditemukan")
		return
	}
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "Gagal memuat program")
		return
	}
	common.WriteSuccess(w, p, nil)
}

func (h *Handler) saveProgram(w http.ResponseWriter, r *http.Request, id string) {
	c, school := common.RequireRole(w, r, common.RolePetugasUKS)
	if school == "" {
		return
	}
	var in ProgramInput
	if !common.Decode(w, r, &in) {
		return
	}
	normalizeProgram(&in)
	if err := validateProgramInput(in); err != nil {
		writeValidation(w, err)
		return
	}
	tx, err := h.DB.BeginTx(r.Context(), nil)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "Gagal menyimpan program")
		return
	}
	defer tx.Rollback()
	status := http.StatusOK
	if id == "" {
		id, err = InsertProgram(r.Context(), tx, school, c.UserID, in)
		status = http.StatusCreated
	} else {
		err = UpdateProgram(r.Context(), tx, school, id, in)
	}
	if errors.Is(err, store.ErrNotFound) {
		common.WriteError(w, http.StatusConflict, "Program tidak ditemukan atau sudah dipublikasikan")
		return
	}
	if err == nil {
		err = ReplaceTargets(r.Context(), tx, school, id, in.Targets)
	}
	if err == nil {
		err = h.audit(r.Context(), tx, c, school, "health_program", id, map[bool]string{true: "create", false: "update"}[status == http.StatusCreated],
			map[string]any{"kind": in.Kind, "name": in.Name, "starts_on": in.StartsOn, "ends_on": in.EndsOn,
				"requires_consent": in.RequiresConsent, "targets": len(in.Targets)})
	}
	if err == nil {
		err = tx.Commit()
	}
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "Gagal menyimpan program")
		return
	}
	p, err := GetProgram(r.Context(), h.DB, school, id)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "Gagal memuat program")
		return
	}
	common.WriteJSON(w, status, p, nil)
}

// CreateProgram: POST /health/programs
func (h *Handler) CreateProgram(w http.ResponseWriter, r *http.Request) { h.saveProgram(w, r, "") }

// UpdateProgram: PUT /health/programs/{id} (hanya draft)
func (h *Handler) UpdateProgram(w http.ResponseWriter, r *http.Request) {
	h.saveProgram(w, r, chi.URLParam(r, "id"))
}

// transition moves a program between statuses (cancel/complete).
func (h *Handler) transition(w http.ResponseWriter, r *http.Request, from []string, to, action string) {
	c, school := common.RequireRole(w, r, common.RolePetugasUKS)
	if school == "" {
		return
	}
	id := chi.URLParam(r, "id")
	p, err := GetProgram(r.Context(), h.DB, school, id)
	if errors.Is(err, store.ErrNotFound) {
		common.WriteError(w, http.StatusNotFound, "Program tidak ditemukan")
		return
	}
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "Gagal memuat program")
		return
	}
	if !slices.Contains(from, p.Status) {
		common.WriteError(w, http.StatusConflict, "Status program tidak memungkinkan aksi ini")
		return
	}
	tx, err := h.DB.BeginTx(r.Context(), nil)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "Gagal memperbarui program")
		return
	}
	defer tx.Rollback()
	err = SetProgramStatus(r.Context(), tx, school, id, p.Status, to)
	if errors.Is(err, store.ErrNotFound) {
		common.WriteError(w, http.StatusConflict, "Status program sudah berubah")
		return
	}
	if err == nil {
		err = h.audit(r.Context(), tx, c, school, "health_program", id, action, map[string]any{"from": p.Status, "status": to})
	}
	if err == nil {
		err = tx.Commit()
	}
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "Gagal memperbarui program")
		return
	}
	p.Status = to
	common.WriteSuccess(w, p, nil)
}

// CancelProgram: DELETE /health/programs/{id} (soft: status cancelled)
func (h *Handler) CancelProgram(w http.ResponseWriter, r *http.Request) {
	h.transition(w, r, []string{ProgramDraft, ProgramPublished}, ProgramCancelled, "cancel")
}

// CompleteProgram: POST /health/programs/{id}/complete
func (h *Handler) CompleteProgram(w http.ResponseWriter, r *http.Request) {
	h.transition(w, r, []string{ProgramPublished}, ProgramCompleted, "complete")
}

// rosters loads the academic data needed by ResolveTargets.
func (h *Handler) rosters(ctx context.Context, school string, targets []Target) (Rosters, error) {
	r := Rosters{ByClassroom: map[string][]string{}, GradeOf: map[string]string{}}
	need := map[string]bool{}
	needGrades := false
	for _, t := range targets {
		switch t.Kind {
		case TargetClassroom:
			need[t.Ref] = true
		case TargetGradeLevel:
			needGrades = true
		}
	}
	if needGrades {
		cls, err := h.Client.ListClassrooms(ctx, school)
		if err != nil {
			return r, err
		}
		for _, cl := range cls {
			if cl.GradeLevel != nil {
				r.GradeOf[cl.ID] = *cl.GradeLevel
				for _, t := range targets {
					if t.Kind == TargetGradeLevel && t.Ref == *cl.GradeLevel {
						need[cl.ID] = true
					}
				}
			}
		}
	}
	for id := range need {
		ids, err := h.Client.ClassroomStudentIDs(ctx, id, school)
		if err != nil {
			return r, err
		}
		r.ByClassroom[id] = ids
	}
	return r, nil
}

// PublishProgram: POST /health/programs/{id}/publish → bentuk peserta (+ consent).
func (h *Handler) PublishProgram(w http.ResponseWriter, r *http.Request) {
	c, school := common.RequireRole(w, r, common.RolePetugasUKS)
	if school == "" {
		return
	}
	p, err := GetProgram(r.Context(), h.DB, school, chi.URLParam(r, "id"))
	if errors.Is(err, store.ErrNotFound) {
		common.WriteError(w, http.StatusNotFound, "Program tidak ditemukan")
		return
	}
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "Gagal memuat program")
		return
	}
	if p.Status != ProgramDraft {
		common.WriteError(w, http.StatusConflict, "Program sudah dipublikasikan atau ditutup")
		return
	}
	rs, err := h.rosters(r.Context(), school, p.Targets)
	if err != nil {
		common.WriteError(w, http.StatusBadGateway, "Gagal memuat data kelas untuk sasaran program")
		return
	}
	students := ResolveTargets(p.Targets, rs)
	if len(students) == 0 {
		common.WriteError(w, http.StatusUnprocessableEntity, "Sasaran program belum memiliki peserta")
		return
	}
	tx, err := h.DB.BeginTx(r.Context(), nil)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "Gagal mempublikasikan program")
		return
	}
	defer tx.Rollback()
	newConsents := []consent.Consent{}
	for _, sid := range students {
		var consentID *string
		if p.RequiresConsent {
			cs, created, cerr := consent.Request(r.Context(), tx, school, ConsentSubjectProgram, p.ID, sid, c.UserID)
			if cerr != nil {
				err = cerr
				break
			}
			consentID = &cs.ID
			if created {
				newConsents = append(newConsents, cs)
			}
		}
		if err = InsertParticipant(r.Context(), tx, school, p.ID, sid, consentID); err != nil {
			break
		}
	}
	if err == nil {
		err = SetProgramStatus(r.Context(), tx, school, p.ID, ProgramDraft, ProgramPublished)
	}
	if err == nil {
		err = h.audit(r.Context(), tx, c, school, "health_program", p.ID, "publish",
			map[string]any{"participants": len(students), "consents": len(newConsents)})
	}
	if err == nil {
		err = tx.Commit()
	}
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "Gagal mempublikasikan program")
		return
	}
	for _, cs := range newConsents {
		h.notifyConsent(r.Context(), school, p, cs, "studentlife:consent:"+cs.ID, "Permintaan persetujuan: "+p.Name)
	}
	p, err = GetProgram(r.Context(), h.DB, school, p.ID)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "Gagal memuat program")
		return
	}
	common.WriteSuccess(w, p, nil)
}

// notifyConsent asks every active guardian of the consent's student to decide.
func (h *Handler) notifyConsent(ctx context.Context, school string, p Program, cs consent.Consent, keyPrefix, title string) {
	guardians, err := h.Client.StudentGuardians(ctx, cs.StudentID, school)
	if err != nil {
		return
	}
	body := "Sekolah meminta persetujuan Anda untuk program " + p.Name + " (" + p.StartsOn + " s.d. " + p.EndsOn + ")."
	if p.ConsentDeadline != nil {
		body += " Batas persetujuan " + *p.ConsentDeadline + "."
	}
	for _, g := range guardians {
		_ = h.Client.Notify(ctx, client.Notification{SchoolID: school, UserID: g.GuardianUserID, Type: "health_consent",
			Title: title, Body: body, Data: map[string]any{"consent_id": cs.ID, "program_id": p.ID,
				"link": h.Cfg.PortalURL + "/orang-tua/kesehatan"}}, keyPrefix+":"+g.GuardianUserID)
	}
}

// ParticipantView is a participant row as shown to the officer.
type ParticipantView struct {
	ID             string     `json:"id"`
	StudentID      string     `json:"student_id"`
	StudentName    string     `json:"student_name"`
	ConsentID      *string    `json:"consent_id"`
	ConsentStatus  *string    `json:"consent_status"`
	CanRecord      bool       `json:"can_record"`
	ResultStatus   string     `json:"result_status"`
	ResultNote     string     `json:"result_note"`
	NeedsFollowUp  bool       `json:"needs_follow_up"`
	FollowUpNote   string     `json:"follow_up_note"`
	FollowUpDoneAt *time.Time `json:"follow_up_done_at"`
	RecordedAt     *time.Time `json:"recorded_at"`
}

func (h *Handler) participantView(p ParticipantRow, school string, prog Program, names map[string]string) (ParticipantView, error) {
	aad := sensitive.AAD(tblParticipants, school, p.ID)
	v := ParticipantView{ID: p.ID, StudentID: p.StudentID, StudentName: names[p.StudentID], ConsentID: p.ConsentID,
		ConsentStatus: p.ConsentStatus, ResultStatus: p.ResultStatus, NeedsFollowUp: p.NeedsFollowUp,
		FollowUpDoneAt: p.FollowUpDoneAt, RecordedAt: p.RecordedAt}
	cs := ""
	if p.ConsentStatus != nil {
		cs = *p.ConsentStatus
	}
	v.CanRecord = CanRecordResult(prog.RequiresConsent, prog.Status, cs) == nil
	if err := h.Crypto.OpenJSON(p.ResultEnc, aad, &v.ResultNote); err != nil {
		return v, err
	}
	return v, h.Crypto.OpenJSON(p.FollowUpEnc, aad, &v.FollowUpNote)
}

// ListParticipants: GET /health/programs/{id}/participants
func (h *Handler) ListParticipants(w http.ResponseWriter, r *http.Request) {
	c, school := common.RequireRole(w, r, common.RolePetugasUKS)
	if school == "" {
		return
	}
	prog, err := GetProgram(r.Context(), h.DB, school, chi.URLParam(r, "id"))
	if errors.Is(err, store.ErrNotFound) {
		common.WriteError(w, http.StatusNotFound, "Program tidak ditemukan")
		return
	}
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "Gagal memuat peserta")
		return
	}
	rows, err := Participants(r.Context(), h.DB, school, prog.ID)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "Gagal memuat peserta")
		return
	}
	ids := []string{}
	for _, p := range rows {
		ids = append(ids, p.StudentID)
	}
	names := h.names(r.Context(), school, ids...)
	out := make([]ParticipantView, 0, len(rows))
	for _, p := range rows {
		v, err := h.participantView(p, school, prog, names)
		if err != nil {
			common.WriteError(w, http.StatusInternalServerError, "Gagal membuka hasil peserta")
			return
		}
		out = append(out, v)
	}
	if !h.viewed(w, r, c, school, "health_program", prog.ID, "") {
		return
	}
	common.WriteSuccess(w, out, nil)
}

// RecordResult: PUT /health/programs/{id}/participants/{pid}/result (AC-129-01).
func (h *Handler) RecordResult(w http.ResponseWriter, r *http.Request) {
	c, school := common.RequireRole(w, r, common.RolePetugasUKS)
	if school == "" {
		return
	}
	var body struct {
		ResultStatus  string `json:"result_status"`
		ResultNote    string `json:"result_note"`
		NeedsFollowUp bool   `json:"needs_follow_up"`
		FollowUpNote  string `json:"follow_up_note"`
	}
	if !common.Decode(w, r, &body) {
		return
	}
	if body.ResultStatus == ResultNotRecorded || !slices.Contains(ResultStatuses, body.ResultStatus) {
		common.WriteError(w, http.StatusUnprocessableEntity, "Status hasil tidak valid")
		return
	}
	if body.NeedsFollowUp && strings.TrimSpace(body.FollowUpNote) == "" {
		common.WriteError(w, http.StatusUnprocessableEntity, "Catatan tindak lanjut wajib diisi")
		return
	}
	prog, err := GetProgram(r.Context(), h.DB, school, chi.URLParam(r, "id"))
	if errors.Is(err, store.ErrNotFound) {
		common.WriteError(w, http.StatusNotFound, "Program tidak ditemukan")
		return
	}
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "Gagal menyimpan hasil")
		return
	}
	tx, err := h.DB.BeginTx(r.Context(), nil)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "Gagal menyimpan hasil")
		return
	}
	defer tx.Rollback()
	p, err := GetParticipant(r.Context(), tx, school, prog.ID, chi.URLParam(r, "pid"))
	if errors.Is(err, store.ErrNotFound) {
		common.WriteError(w, http.StatusNotFound, "Peserta tidak ditemukan")
		return
	}
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "Gagal menyimpan hasil")
		return
	}
	cs := ""
	if p.ConsentStatus != nil {
		cs = *p.ConsentStatus
	}
	if err := CanRecordResult(prog.RequiresConsent, prog.Status, cs); err != nil {
		common.WriteError(w, http.StatusUnprocessableEntity, err.Error())
		return
	}
	aad := sensitive.AAD(tblParticipants, school, p.ID)
	var resEnc, fuEnc []byte
	if note := strings.TrimSpace(body.ResultNote); note != "" {
		resEnc, err = h.Crypto.SealJSON(note, aad)
	}
	if err == nil && body.NeedsFollowUp {
		fuEnc, err = h.Crypto.SealJSON(strings.TrimSpace(body.FollowUpNote), aad)
	}
	if err == nil {
		err = SaveResult(r.Context(), tx, p.ID, body.ResultStatus, resEnc, body.NeedsFollowUp, fuEnc, c.UserID)
	}
	if err == nil {
		err = h.audit(r.Context(), tx, c, school, tblParticipants, p.ID, "record_result",
			map[string]any{"program_id": prog.ID, "student_id": p.StudentID, "result_status": body.ResultStatus,
				"needs_follow_up": body.NeedsFollowUp, "changed": []string{"result"}})
	}
	if err == nil {
		err = h.logAccess(r.Context(), tx, r, c, school, tblParticipants, p.ID, p.StudentID, sensitive.ActionUpdate)
	}
	if err == nil {
		err = tx.Commit()
	}
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "Gagal menyimpan hasil")
		return
	}
	common.WriteSuccess(w, map[string]any{"id": p.ID, "result_status": body.ResultStatus, "needs_follow_up": body.NeedsFollowUp}, nil)
}

// FollowUpDone: POST /health/programs/{id}/participants/{pid}/follow-up-done
func (h *Handler) FollowUpDone(w http.ResponseWriter, r *http.Request) {
	c, school := common.RequireRole(w, r, common.RolePetugasUKS)
	if school == "" {
		return
	}
	programID := chi.URLParam(r, "id")
	tx, err := h.DB.BeginTx(r.Context(), nil)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "Gagal memperbarui tindak lanjut")
		return
	}
	defer tx.Rollback()
	p, err := GetParticipant(r.Context(), tx, school, programID, chi.URLParam(r, "pid"))
	if err == nil {
		err = MarkFollowUpDone(r.Context(), tx, p.ID)
	}
	if errors.Is(err, store.ErrNotFound) {
		common.WriteError(w, http.StatusNotFound, "Peserta tidak memerlukan tindak lanjut atau sudah selesai")
		return
	}
	if err == nil {
		err = h.audit(r.Context(), tx, c, school, tblParticipants, p.ID, "follow_up_done", map[string]any{"program_id": programID})
	}
	if err == nil {
		err = tx.Commit()
	}
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "Gagal memperbarui tindak lanjut")
		return
	}
	common.WriteSuccess(w, map[string]string{"id": p.ID}, nil)
}

// Recap: GET /health/programs/{id}/recap (petugas UKS, peran berkebijakan) — angka saja.
func (h *Handler) Recap(w http.ResponseWriter, r *http.Request) {
	c := common.FromCtx(r)
	school := common.EffectiveSchoolID(r)
	if c == nil || school == "" {
		common.WriteError(w, http.StatusForbidden, "Peran Anda tidak memiliki akses ke fitur ini")
		return
	}
	if c.ActiveRole != common.RolePetugasUKS {
		roles, err := sensitive.ActivePolicyRoles(r.Context(), h.DB, school, sensitive.ModuleHealth)
		if err != nil || !slices.Contains(roles, c.ActiveRole) {
			common.WriteError(w, http.StatusForbidden, "Peran Anda tidak memiliki akses ke fitur ini")
			return
		}
	}
	prog, err := GetProgram(r.Context(), h.DB, school, chi.URLParam(r, "id"))
	if errors.Is(err, store.ErrNotFound) {
		common.WriteError(w, http.StatusNotFound, "Program tidak ditemukan")
		return
	}
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "Gagal memuat rekap")
		return
	}
	rows, err := Participants(r.Context(), h.DB, school, prog.ID)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "Gagal memuat rekap")
		return
	}
	in := make([]RecapInput, 0, len(rows))
	for _, p := range rows {
		ri := RecapInput{ResultStatus: p.ResultStatus, NeedsFollowUp: p.NeedsFollowUp, FollowUpDone: p.FollowUpDoneAt != nil}
		if p.ConsentStatus != nil {
			ri.ConsentStatus = *p.ConsentStatus
		}
		in = append(in, ri)
	}
	common.WriteSuccess(w, map[string]any{"program_id": prog.ID, "recap": ProgramRecap(in), "generated_at": time.Now()}, nil)
}

// StudentProgramView is one program row for a student's health page.
type StudentProgramView struct {
	ProgramID   string `json:"program_id"`
	ProgramName string `json:"program_name"`
	Kind        string `json:"kind"`
	StartsOn    string `json:"starts_on"`
	ParticipantView
}

// StudentPrograms: GET /health/students/{id}/programs (petugas, wali, peran berkebijakan).
func (h *Handler) StudentPrograms(w http.ResponseWriter, r *http.Request) {
	c, school := common.RequireRole(w, r, common.RolePetugasUKS, common.RoleParent, common.RoleKepalaSekolah,
		common.RoleAdmin, common.RoleSuperadmin, common.RoleWalikelas, common.RoleGuru)
	if school == "" {
		return
	}
	studentID := chi.URLParam(r, "id")
	if h.level(r.Context(), c, school, studentID) != sensitive.Full {
		h.deny(w, r, c, school, tblParticipants, "", studentID)
		return
	}
	rows, err := StudentParticipations(r.Context(), h.DB, school, studentID)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "Gagal memuat program siswa")
		return
	}
	out := []StudentProgramView{}
	for _, p := range rows {
		prog, err := GetProgram(r.Context(), h.DB, school, p.ProgramID)
		if err != nil {
			common.WriteError(w, http.StatusInternalServerError, "Gagal memuat program siswa")
			return
		}
		v, err := h.participantView(p, school, prog, map[string]string{})
		if err != nil {
			common.WriteError(w, http.StatusInternalServerError, "Gagal membuka hasil program")
			return
		}
		out = append(out, StudentProgramView{ProgramID: prog.ID, ProgramName: prog.Name, Kind: prog.Kind, StartsOn: prog.StartsOn, ParticipantView: v})
	}
	if !h.viewed(w, r, c, school, tblParticipants, "", studentID) {
		return
	}
	common.WriteSuccess(w, out, nil)
}

// ConsentSubject registers health_program in the generic consent registry.
func (h *Handler) ConsentSubject() consent.Subject {
	return consent.Subject{
		Feature: common.FeatureHealth,
		Describe: func(ctx context.Context, schoolID, subjectID string) (map[string]any, error) {
			return ProgramSummary(ctx, h.DB, schoolID, subjectID)
		},
		InUse: func(ctx context.Context, schoolID, consentID string) (bool, error) {
			return ConsentInUse(ctx, h.DB, schoolID, consentID)
		},
	}
}
