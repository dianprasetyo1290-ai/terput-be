package health

import (
	"database/sql"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"skci/studentlife/internal/client"
	"skci/studentlife/internal/common"
	"skci/studentlife/internal/sensitive"
	"skci/studentlife/internal/store"
)

// ProfileView is the Full projection returned to authorized viewers.
type ProfileView struct {
	StudentID         string     `json:"student_id"`
	StudentName       string     `json:"student_name"`
	Exists            bool       `json:"exists"`
	Profile           Profile    `json:"profile"`
	EmergencyContacts []Contact  `json:"emergency_contacts"`
	Version           int        `json:"version"`
	VerifiedBy        *string    `json:"verified_by"`
	VerifiedByName    string     `json:"verified_by_name,omitempty"`
	VerifiedAt        *time.Time `json:"verified_at"`
	UpdatedAt         *time.Time `json:"updated_at"`
	PendingChanges    int        `json:"pending_changes"`
}

func emptyInput() ProfileInput {
	return ProfileInput{Profile: Profile{BloodType: "tidak_diketahui", Allergies: []Allergy{}, Conditions: []Condition{},
		RoutineMedications: []Medication{}}, EmergencyContacts: []Contact{}}
}

// openProfile decrypts a stored profile row.
func (h *Handler) openProfile(p ProfileRow) (ProfileInput, error) {
	in := emptyInput()
	aad := sensitive.AAD(tblProfiles, p.SchoolID, p.ID)
	if err := h.Crypto.OpenJSON(p.ProfileEnc, aad, &in.Profile); err != nil {
		return in, err
	}
	if err := h.Crypto.OpenJSON(p.EmergencyEnc, aad, &in.EmergencyContacts); err != nil {
		return in, err
	}
	return in, nil
}

// sealProfile encrypts profile, derived alert and contacts for row p.
func (h *Handler) sealProfile(p *ProfileRow, in ProfileInput) error {
	aad := sensitive.AAD(tblProfiles, p.SchoolID, p.ID)
	var err error
	if p.ProfileEnc, err = h.Crypto.SealJSON(in.Profile, aad); err != nil {
		return err
	}
	if p.AlertEnc, err = h.Crypto.SealJSON(AlertView(in.Profile, in.EmergencyContacts), aad); err != nil {
		return err
	}
	p.EmergencyEnc, err = h.Crypto.SealJSON(in.EmergencyContacts, aad)
	return err
}

// GetProfile: GET /health/students/{id}/profile
func (h *Handler) GetProfile(w http.ResponseWriter, r *http.Request) {
	c, school := common.RequireRole(w, r, common.RolePetugasUKS, common.RoleParent, common.RoleKepalaSekolah,
		common.RoleAdmin, common.RoleSuperadmin, common.RoleWalikelas, common.RoleGuru)
	if school == "" {
		return
	}
	studentID := chi.URLParam(r, "id")
	if h.level(r.Context(), c, school, studentID) != sensitive.Full {
		h.deny(w, r, c, school, tblProfiles, "", studentID)
		return
	}
	view := ProfileView{StudentID: studentID, Profile: emptyInput().Profile, EmergencyContacts: []Contact{}}
	row, err := GetProfile(r.Context(), h.DB, school, studentID, false)
	if err != nil && !errors.Is(err, store.ErrNotFound) {
		common.WriteError(w, http.StatusInternalServerError, "Gagal memuat profil kesehatan")
		return
	}
	entityID := ""
	if err == nil {
		in, derr := h.openProfile(row)
		if derr != nil {
			common.WriteError(w, http.StatusInternalServerError, "Gagal membuka profil kesehatan")
			return
		}
		entityID = row.ID
		view.Exists, view.Profile, view.EmergencyContacts = true, in.Profile, in.EmergencyContacts
		view.Version, view.VerifiedBy, view.VerifiedAt, view.UpdatedAt = row.Version, row.VerifiedBy, row.VerifiedAt, &row.UpdatedAt
	}
	submitter := ""
	if c.ActiveRole == common.RoleParent {
		submitter = c.UserID
	}
	if _, n, err := ListChanges(r.Context(), h.DB, school, ChangePending, submitter, studentID, 1, 1); err == nil {
		view.PendingChanges = n
	}
	verifier := ""
	if view.VerifiedBy != nil {
		verifier = *view.VerifiedBy
	}
	names := h.names(r.Context(), school, studentID, verifier)
	view.StudentName, view.VerifiedByName = names[studentID], names[verifier]
	if !h.viewed(w, r, c, school, tblProfiles, entityID, studentID) {
		return
	}
	common.WriteSuccess(w, view, nil)
}

// PutProfile: PUT /health/students/{id}/profile (petugas UKS; langsung berlaku + verified).
func (h *Handler) PutProfile(w http.ResponseWriter, r *http.Request) {
	c, school := common.RequireRole(w, r, common.RolePetugasUKS)
	if school == "" {
		return
	}
	studentID := chi.URLParam(r, "id")
	var in ProfileInput
	if !common.Decode(w, r, &in) {
		return
	}
	if err := ValidateProfile(&in); err != nil {
		writeValidation(w, err)
		return
	}
	tx, err := h.DB.BeginTx(r.Context(), nil)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "Gagal menyimpan profil kesehatan")
		return
	}
	defer tx.Rollback()
	row, err := GetProfile(r.Context(), tx, school, studentID, true)
	before := emptyInput()
	switch {
	case errors.Is(err, store.ErrNotFound):
		row = ProfileRow{SchoolID: school, StudentID: studentID}
		row.ID, err = NewID(r.Context(), tx)
	case err == nil:
		before, err = h.openProfile(row)
	}
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "Gagal menyimpan profil kesehatan")
		return
	}
	now := time.Now()
	row.Version++
	row.VerifiedBy, row.VerifiedAt = &c.UserID, &now
	err = h.sealProfile(&row, in)
	if err == nil {
		err = SaveProfile(r.Context(), tx, row)
	}
	if err == nil {
		err = h.audit(r.Context(), tx, c, school, tblProfiles, row.ID, "update",
			map[string]any{"student_id": studentID, "version": row.Version, "changed": ChangedFields(before, in)})
	}
	if err == nil {
		err = h.logAccess(r.Context(), tx, r, c, school, tblProfiles, row.ID, studentID, sensitive.ActionUpdate)
	}
	if err == nil {
		err = tx.Commit()
	}
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "Gagal menyimpan profil kesehatan")
		return
	}
	common.WriteSuccess(w, ProfileView{StudentID: studentID, Exists: true, Profile: in.Profile, EmergencyContacts: in.EmergencyContacts,
		Version: row.Version, VerifiedBy: row.VerifiedBy, VerifiedAt: row.VerifiedAt, UpdatedAt: &now}, nil)
}

// StudentAlert is one row of the homeroom alert view (AC-127-01).
type StudentAlert struct {
	StudentID   string `json:"student_id"`
	StudentName string `json:"student_name"`
	Alert
}

// ListAlerts: GET /health/alerts?classroom_id= (walikelas kelas itu, petugas UKS).
func (h *Handler) ListAlerts(w http.ResponseWriter, r *http.Request) {
	c, school := common.RequireRole(w, r, common.RoleWalikelas, common.RolePetugasUKS)
	if school == "" {
		return
	}
	classroomID := strings.TrimSpace(r.URL.Query().Get("classroom_id"))
	if classroomID == "" {
		common.WriteError(w, http.StatusBadRequest, "classroom_id wajib diisi")
		return
	}
	if c.ActiveRole == common.RoleWalikelas && !h.isHomeroomOf(r.Context(), c.UserID, school, classroomID) {
		h.deny(w, r, c, school, "health_alerts", classroomID, "")
		return
	}
	studentIDs, err := h.Client.ClassroomStudentIDs(r.Context(), classroomID, school)
	if err != nil {
		common.WriteError(w, http.StatusBadGateway, "Gagal memuat siswa kelas")
		return
	}
	rows, err := ProfilesForStudents(r.Context(), h.DB, school, studentIDs)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "Gagal memuat peringatan medis")
		return
	}
	names := h.names(r.Context(), school, studentIDs...)
	out := []StudentAlert{}
	tx, err := h.DB.BeginTx(r.Context(), nil)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "Gagal memuat peringatan medis")
		return
	}
	defer tx.Rollback()
	for _, p := range rows {
		a := Alert{Items: []AlertItem{}, EmergencyContacts: []Contact{}}
		if err := h.Crypto.OpenJSON(p.AlertEnc, sensitive.AAD(tblProfiles, p.SchoolID, p.ID), &a); err != nil {
			common.WriteError(w, http.StatusInternalServerError, "Gagal membuka peringatan medis")
			return
		}
		if len(a.Items) == 0 && len(a.EmergencyContacts) == 0 {
			continue
		}
		if err := h.logAccess(r.Context(), tx, r, c, school, "health_alerts", p.ID, p.StudentID, sensitive.ActionView); err != nil {
			common.WriteError(w, http.StatusInternalServerError, "Gagal mencatat akses data sensitif")
			return
		}
		out = append(out, StudentAlert{StudentID: p.StudentID, StudentName: names[p.StudentID], Alert: a})
	}
	if err := tx.Commit(); err != nil {
		common.WriteError(w, http.StatusInternalServerError, "Gagal mencatat akses data sensitif")
		return
	}
	common.WriteSuccess(w, out, nil)
}

// ---- usulan perubahan dari wali (FR-127-002) -------------------------------

// ChangeView is a proposal; Proposed/Current are only filled on detail.
type ChangeView struct {
	ChangeRow
	StudentName   string        `json:"student_name"`
	SubmitterName string        `json:"submitter_name"`
	Proposed      *ProfileInput `json:"proposed,omitempty"`
	Current       *ProfileInput `json:"current,omitempty"`
	CurrentVer    int           `json:"current_version"`
}

// CreateChange: POST /health/students/{id}/profile-changes (wali; Idempotency-Key wajib).
func (h *Handler) CreateChange(w http.ResponseWriter, r *http.Request) {
	c, school := common.RequireRole(w, r, common.RoleParent)
	if school == "" {
		return
	}
	studentID := chi.URLParam(r, "id")
	key := idempotencyKey(r)
	if key == "" {
		common.WriteError(w, http.StatusBadRequest, "Header Idempotency-Key wajib diisi")
		return
	}
	var in ProfileInput
	if !common.Decode(w, r, &in) {
		return
	}
	if h.level(r.Context(), c, school, studentID) != sensitive.Full {
		h.deny(w, r, c, school, tblChanges, "", studentID)
		return
	}
	if err := ValidateProfile(&in); err != nil {
		writeValidation(w, err)
		return
	}
	tx, err := h.DB.BeginTx(r.Context(), nil)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "Gagal menyimpan usulan")
		return
	}
	defer tx.Rollback()
	base := 0
	var profileID *string
	if p, err := GetProfile(r.Context(), tx, school, studentID, false); err == nil {
		base, profileID = p.Version, &p.ID
	} else if !errors.Is(err, store.ErrNotFound) {
		common.WriteError(w, http.StatusInternalServerError, "Gagal menyimpan usulan")
		return
	}
	row := ChangeRow{SchoolID: school, StudentID: studentID, SubmittedBy: c.UserID, BaseVersion: base}
	if row.ID, err = NewID(r.Context(), tx); err == nil {
		row.ProposedEnc, err = h.Crypto.SealJSON(in, sensitive.AAD(tblChanges, school, row.ID))
	}
	var created bool
	if err == nil {
		row, created, err = InsertChange(r.Context(), tx, row, "guardian:"+c.UserID+":"+key, profileID)
	}
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "Gagal menyimpan usulan")
		return
	}
	if !created {
		common.WriteSuccess(w, ChangeView{ChangeRow: row}, nil)
		return
	}
	names := h.names(r.Context(), school, studentID)
	approvalID, err := h.Client.CreateApproval(r.Context(), client.ApprovalRequest{
		SchoolID: school, RequestType: RequestTypeProfileUpdate, SourceService: "studentlife", SourceID: row.ID,
		Title: "Perubahan profil kesehatan siswa", Summary: map[string]any{"student_name": names[studentID]},
		MakerUserID: c.UserID, SubjectUserID: studentID, CallbackURL: h.Cfg.PublicInternalURL + "/internal/approval-callbacks",
		CorrelationID: row.ID, IdempotencyKey: RequestTypeProfileUpdate + ":" + row.ID,
	})
	if err != nil {
		if client.StatusOf(err) == http.StatusUnprocessableEntity {
			common.WriteError(w, http.StatusUnprocessableEntity, "Verifikasi perubahan belum dapat diajukan: alur persetujuan belum dikonfigurasi sekolah. Hubungi admin sekolah.")
			return
		}
		common.WriteError(w, http.StatusBadGateway, "Gagal mengajukan verifikasi ke petugas UKS")
		return
	}
	row.ApprovalID = &approvalID
	err = SetChangeApproval(r.Context(), tx, row.ID, approvalID)
	if err == nil {
		err = h.audit(r.Context(), tx, c, school, tblChanges, row.ID, "create",
			map[string]any{"student_id": studentID, "base_version": base, "approval_id": approvalID})
	}
	if err == nil {
		err = h.logAccess(r.Context(), tx, r, c, school, tblChanges, row.ID, studentID, sensitive.ActionCreate)
	}
	if err == nil {
		err = tx.Commit()
	}
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "Gagal menyimpan usulan")
		return
	}
	common.WriteJSON(w, http.StatusCreated, ChangeView{ChangeRow: row, StudentName: names[studentID]}, nil)
}

// ListChanges: GET /health/profile-changes?status=&student_id= (wali: miliknya; petugas: antrian).
func (h *Handler) ListChanges(w http.ResponseWriter, r *http.Request) {
	c, school := common.RequireRole(w, r, common.RolePetugasUKS, common.RoleParent)
	if school == "" {
		return
	}
	submitter := ""
	if c.ActiveRole == common.RoleParent {
		submitter = c.UserID
	}
	page, perPage := common.Page(r)
	q := r.URL.Query()
	list, total, err := ListChanges(r.Context(), h.DB, school, q.Get("status"), submitter, q.Get("student_id"), page, perPage)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "Gagal memuat usulan perubahan")
		return
	}
	ids := []string{}
	for _, ch := range list {
		ids = append(ids, ch.StudentID, ch.SubmittedBy)
	}
	names := h.names(r.Context(), school, ids...)
	out := make([]ChangeView, 0, len(list))
	for _, ch := range list {
		out = append(out, ChangeView{ChangeRow: ch, StudentName: names[ch.StudentID], SubmitterName: names[ch.SubmittedBy]})
	}
	common.WriteSuccess(w, out, common.Meta{Page: page, PerPage: perPage, Total: total})
}

// GetChange: GET /health/profile-changes/{id} (isi usulan vs data saat ini).
func (h *Handler) GetChange(w http.ResponseWriter, r *http.Request) {
	c, school := common.RequireRole(w, r, common.RolePetugasUKS, common.RoleParent)
	if school == "" {
		return
	}
	ch, err := GetChange(r.Context(), h.DB, school, chi.URLParam(r, "id"), false)
	if errors.Is(err, store.ErrNotFound) {
		common.WriteError(w, http.StatusNotFound, "Data tidak ditemukan")
		return
	}
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "Gagal memuat usulan")
		return
	}
	if (c.ActiveRole == common.RoleParent && ch.SubmittedBy != c.UserID) || h.level(r.Context(), c, school, ch.StudentID) != sensitive.Full {
		h.deny(w, r, c, school, tblChanges, ch.ID, ch.StudentID)
		return
	}
	view := ChangeView{ChangeRow: ch, Proposed: &ProfileInput{}}
	if err := h.Crypto.OpenJSON(ch.ProposedEnc, sensitive.AAD(tblChanges, school, ch.ID), view.Proposed); err != nil {
		common.WriteError(w, http.StatusInternalServerError, "Gagal membuka usulan")
		return
	}
	if p, err := GetProfile(r.Context(), h.DB, school, ch.StudentID, false); err == nil {
		cur, derr := h.openProfile(p)
		if derr != nil {
			common.WriteError(w, http.StatusInternalServerError, "Gagal membuka profil kesehatan")
			return
		}
		view.Current, view.CurrentVer = &cur, p.Version
	} else if !errors.Is(err, store.ErrNotFound) {
		common.WriteError(w, http.StatusInternalServerError, "Gagal memuat profil kesehatan")
		return
	}
	names := h.names(r.Context(), school, ch.StudentID, ch.SubmittedBy)
	view.StudentName, view.SubmitterName = names[ch.StudentID], names[ch.SubmittedBy]
	if !h.viewed(w, r, c, school, tblChanges, ch.ID, ch.StudentID) {
		return
	}
	common.WriteSuccess(w, view, nil)
}

// ApplyApprovalDecision handles the core callback for health_profile_update
// (idempotent): approved → ApplyChange (or superseded), rejected → status.
// Returns the guardian to notify and the final status ("" when nothing changed).
func (h *Handler) ApplyApprovalDecision(r *http.Request, tx *sql.Tx, changeID, decision, decidedBy string) (ChangeRow, string, error) {
	ctx := r.Context()
	ch, err := GetChangeByID(ctx, tx, changeID)
	if err != nil {
		return ch, "", err
	}
	if ch.Status != ChangePending {
		return ch, "", nil
	}
	actor := decidedBy
	role := "approval"
	entry := func(action string, after map[string]any) error {
		var a *string
		if actor != "" {
			a = &actor
		}
		return store.Audit(ctx, tx, store.AuditEntry{SchoolID: ch.SchoolID, Module: sensitive.ModuleHealth, EntityType: tblChanges,
			EntityID: ch.ID, Action: action, ActorUserID: a, ActorRole: &role, After: after})
	}
	if decision != ChangeApproved {
		if err := DecideChange(ctx, tx, ch.ID, ChangeRejected); err != nil {
			return ch, "", err
		}
		return ch, ChangeRejected, entry("reject", map[string]any{"status": ChangeRejected})
	}
	var proposed ProfileInput
	if err := h.Crypto.OpenJSON(ch.ProposedEnc, sensitive.AAD(tblChanges, ch.SchoolID, ch.ID), &proposed); err != nil {
		return ch, "", err
	}
	row, err := GetProfile(ctx, tx, ch.SchoolID, ch.StudentID, true)
	before := emptyInput()
	switch {
	case errors.Is(err, store.ErrNotFound):
		row = ProfileRow{SchoolID: ch.SchoolID, StudentID: ch.StudentID}
		row.ID, err = NewID(ctx, tx)
	case err == nil:
		before, err = h.openProfile(row)
	}
	if err != nil {
		return ch, "", err
	}
	version, _, err := ApplyChange(row.Version, ch.BaseVersion, proposed)
	if errors.Is(err, ErrSuperseded) {
		if err := DecideChange(ctx, tx, ch.ID, ChangeSuperseded); err != nil {
			return ch, "", err
		}
		return ch, ChangeSuperseded, entry("supersede", map[string]any{"status": ChangeSuperseded, "current_version": row.Version})
	}
	now := time.Now()
	row.Version = version
	if decidedBy != "" {
		row.VerifiedBy = &decidedBy
	}
	row.VerifiedAt = &now
	if err := h.sealProfile(&row, proposed); err != nil {
		return ch, "", err
	}
	if err := SaveProfile(ctx, tx, row); err != nil {
		return ch, "", err
	}
	if err := DecideChange(ctx, tx, ch.ID, ChangeApproved); err != nil {
		return ch, "", err
	}
	return ch, ChangeApproved, entry("approve", map[string]any{"status": ChangeApproved, "version": version, "changed": ChangedFields(before, proposed)})
}
