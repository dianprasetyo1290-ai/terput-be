package health

import (
	"context"
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

// ---- obat UKS (FR-128-003) -------------------------------------------------

type medicineBody struct {
	Name      string  `json:"name"`
	Unit      string  `json:"unit"`
	OpsItemID *string `json:"ops_item_id"`
	Active    *bool   `json:"active"`
}

func (b medicineBody) toMedicine() (Medicine, error) {
	m := Medicine{Name: strings.TrimSpace(b.Name), Unit: strings.TrimSpace(b.Unit), Active: b.Active == nil || *b.Active}
	if b.OpsItemID != nil && strings.TrimSpace(*b.OpsItemID) != "" {
		v := strings.TrimSpace(*b.OpsItemID)
		m.OpsItemID = &v
	}
	if m.Name == "" || m.Unit == "" {
		return m, invalid("Nama obat dan satuan wajib diisi")
	}
	return m, nil
}

// ListMedicines: GET /health/medicines?active=true
func (h *Handler) ListMedicines(w http.ResponseWriter, r *http.Request) {
	_, school := common.RequireRole(w, r, common.RolePetugasUKS)
	if school == "" {
		return
	}
	list, err := ListMedicines(r.Context(), h.DB, school, r.URL.Query().Get("active") == "true")
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "Gagal memuat daftar obat")
		return
	}
	common.WriteSuccess(w, list, nil)
}

func (h *Handler) saveMedicine(w http.ResponseWriter, r *http.Request, id string) {
	c, school := common.RequireRole(w, r, common.RolePetugasUKS)
	if school == "" {
		return
	}
	var body medicineBody
	if !common.Decode(w, r, &body) {
		return
	}
	m, err := body.toMedicine()
	if err != nil {
		writeValidation(w, err)
		return
	}
	tx, err := h.DB.BeginTx(r.Context(), nil)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "Gagal menyimpan obat")
		return
	}
	defer tx.Rollback()
	var out Medicine
	status := http.StatusOK
	if id == "" {
		out, err = CreateMedicine(r.Context(), tx, school, m)
		status = http.StatusCreated
	} else {
		out, err = UpdateMedicine(r.Context(), tx, school, id, m)
	}
	switch {
	case errors.Is(err, ErrDuplicateName):
		common.WriteError(w, http.StatusConflict, "Nama obat sudah terdaftar")
		return
	case errors.Is(err, store.ErrNotFound):
		common.WriteError(w, http.StatusNotFound, "Obat tidak ditemukan")
		return
	}
	if err == nil {
		err = h.audit(r.Context(), tx, c, school, "uks_medicine", out.ID, map[bool]string{true: "create", false: "update"}[id == ""],
			map[string]any{"name": out.Name, "unit": out.Unit, "ops_item_id": out.OpsItemID, "active": out.Active})
	}
	if err == nil {
		err = tx.Commit()
	}
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "Gagal menyimpan obat")
		return
	}
	common.WriteJSON(w, status, out, nil)
}

// CreateMedicine: POST /health/medicines
func (h *Handler) CreateMedicine(w http.ResponseWriter, r *http.Request) { h.saveMedicine(w, r, "") }

// UpdateMedicine: PUT /health/medicines/{id}
func (h *Handler) UpdateMedicine(w http.ResponseWriter, r *http.Request) {
	h.saveMedicine(w, r, chi.URLParam(r, "id"))
}

// DeleteMedicine: DELETE /health/medicines/{id} (soft delete)
func (h *Handler) DeleteMedicine(w http.ResponseWriter, r *http.Request) {
	c, school := common.RequireRole(w, r, common.RolePetugasUKS)
	if school == "" {
		return
	}
	id := chi.URLParam(r, "id")
	tx, err := h.DB.BeginTx(r.Context(), nil)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "Gagal menghapus obat")
		return
	}
	defer tx.Rollback()
	err = DeleteMedicine(r.Context(), tx, school, id)
	if errors.Is(err, store.ErrNotFound) {
		common.WriteError(w, http.StatusNotFound, "Obat tidak ditemukan")
		return
	}
	if err == nil {
		err = h.audit(r.Context(), tx, c, school, "uks_medicine", id, "delete", nil)
	}
	if err == nil {
		err = tx.Commit()
	}
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "Gagal menghapus obat")
		return
	}
	common.WriteSuccess(w, map[string]string{"id": id}, nil)
}

// ---- kunjungan UKS (FR-128-001/002/003) -----------------------------------

type visitBody struct {
	StudentID   string     `json:"student_id"`
	VisitedAt   *time.Time `json:"visited_at"`
	Complaint   string     `json:"complaint"`
	Examination string     `json:"examination"`
	Action      string     `json:"action"`
	Note        string     `json:"note"`
	Outcome     string     `json:"outcome"`
	ReferredTo  string     `json:"referred_to"`
	Medications []struct {
		MedicineID string  `json:"medicine_id"`
		Qty        float64 `json:"qty"`
	} `json:"medications"`
}

// AddendumView is a decrypted addendum.
type AddendumView struct {
	ID            string    `json:"id"`
	Note          string    `json:"note"`
	CreatedBy     string    `json:"created_by"`
	CreatedByName string    `json:"created_by_name"`
	CreatedAt     time.Time `json:"created_at"`
}

// VisitView is a visit as returned by the API. Details/ReferredTo are only
// set on detail reads (list rows never carry decrypted content).
type VisitView struct {
	ID               string            `json:"id"`
	StudentID        string            `json:"student_id"`
	StudentName      string            `json:"student_name"`
	VisitedAt        time.Time         `json:"visited_at"`
	LeftAt           *time.Time        `json:"left_at"`
	Outcome          string            `json:"outcome"`
	Details          *VisitDetails     `json:"details,omitempty"`
	ReferredTo       *string           `json:"referred_to,omitempty"`
	HandledBy        string            `json:"handled_by"`
	HandledByName    string            `json:"handled_by_name"`
	AttendanceSync   string            `json:"attendance_sync"`
	AttendanceError  *string           `json:"attendance_error,omitempty"`
	ParentNotifiedAt *time.Time        `json:"parent_notified_at"`
	Effects          Effects           `json:"effects"`
	Medications      []VisitMedication `json:"medications,omitempty"`
	Addenda          []AddendumView    `json:"addenda,omitempty"`
	CreatedAt        time.Time         `json:"created_at"`
}

func baseVisitView(v VisitRow, names map[string]string) VisitView {
	return VisitView{ID: v.ID, StudentID: v.StudentID, StudentName: names[v.StudentID], VisitedAt: v.VisitedAt, LeftAt: v.LeftAt,
		Outcome: v.Outcome, HandledBy: v.HandledBy, HandledByName: names[v.HandledBy], AttendanceSync: v.AttendanceSync,
		AttendanceError: v.AttendanceError, ParentNotifiedAt: v.ParentNotifiedAt, Effects: VisitEffects(v.Outcome), CreatedAt: v.CreatedAt}
}

// visitDetail builds the full decrypted view of one visit.
func (h *Handler) visitDetail(ctx context.Context, v VisitRow) (VisitView, error) {
	aad := sensitive.AAD(tblVisits, v.SchoolID, v.ID)
	var d VisitDetails
	if err := h.Crypto.OpenJSON(v.DetailsEnc, aad, &d); err != nil {
		return VisitView{}, err
	}
	var ref *string
	if len(v.ReferredToEnc) > 0 {
		var s string
		if err := h.Crypto.OpenJSON(v.ReferredToEnc, aad, &s); err != nil {
			return VisitView{}, err
		}
		ref = &s
	}
	meds, err := VisitMedications(ctx, h.DB, v.SchoolID, v.ID)
	if err != nil {
		return VisitView{}, err
	}
	adds, err := Addenda(ctx, h.DB, v.SchoolID, v.ID)
	if err != nil {
		return VisitView{}, err
	}
	ids := []string{v.StudentID, v.HandledBy}
	for _, a := range adds {
		ids = append(ids, a.CreatedBy)
	}
	names := h.names(ctx, v.SchoolID, ids...)
	view := baseVisitView(v, names)
	view.Details, view.ReferredTo, view.Medications, view.Addenda = &d, ref, meds, []AddendumView{}
	for _, a := range adds {
		var note string
		if err := h.Crypto.OpenJSON(a.NoteEnc, sensitive.AAD(tblAddenda, v.SchoolID, a.ID), &note); err != nil {
			return VisitView{}, err
		}
		view.Addenda = append(view.Addenda, AddendumView{ID: a.ID, Note: note, CreatedBy: a.CreatedBy, CreatedByName: names[a.CreatedBy], CreatedAt: a.CreatedAt})
	}
	return view, nil
}

// CreateVisit: POST /health/visits (petugas UKS; Idempotency-Key wajib).
func (h *Handler) CreateVisit(w http.ResponseWriter, r *http.Request) {
	c, school := common.RequireRole(w, r, common.RolePetugasUKS)
	if school == "" {
		return
	}
	key := idempotencyKey(r)
	if key == "" {
		common.WriteError(w, http.StatusBadRequest, "Header Idempotency-Key wajib diisi")
		return
	}
	var body visitBody
	if !common.Decode(w, r, &body) {
		return
	}
	if existing, err := VisitByIdempotencyKey(r.Context(), h.DB, school, key); err == nil {
		h.respondVisit(w, r, c, existing, http.StatusOK)
		return
	} else if !errors.Is(err, store.ErrNotFound) {
		common.WriteError(w, http.StatusInternalServerError, "Gagal menyimpan kunjungan")
		return
	}
	details := VisitDetails{Complaint: strings.TrimSpace(body.Complaint), Examination: strings.TrimSpace(body.Examination),
		Action: strings.TrimSpace(body.Action), Note: strings.TrimSpace(body.Note)}
	if strings.TrimSpace(body.StudentID) == "" {
		common.WriteError(w, http.StatusUnprocessableEntity, "Siswa wajib dipilih")
		return
	}
	if err := ValidateVisit(body.Outcome, details, body.ReferredTo); err != nil {
		writeValidation(w, err)
		return
	}
	now := time.Now()
	visitedAt := now
	if body.VisitedAt != nil {
		if body.VisitedAt.After(now) {
			common.WriteError(w, http.StatusUnprocessableEntity, "Jam datang tidak boleh di masa depan")
			return
		}
		visitedAt = *body.VisitedAt
	}
	medIDs := []string{}
	for _, m := range body.Medications {
		if m.Qty <= 0 {
			common.WriteError(w, http.StatusUnprocessableEntity, "Jumlah obat harus lebih dari 0")
			return
		}
		medIDs = append(medIDs, m.MedicineID)
	}
	tx, err := h.DB.BeginTx(r.Context(), nil)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "Gagal menyimpan kunjungan")
		return
	}
	defer tx.Rollback()
	meds, err := MedicinesByID(r.Context(), tx, school, medIDs)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "Gagal menyimpan kunjungan")
		return
	}
	for _, id := range medIDs {
		if _, ok := meds[id]; !ok {
			common.WriteError(w, http.StatusUnprocessableEntity, "Obat tidak ditemukan atau nonaktif")
			return
		}
	}
	eff := VisitEffects(body.Outcome)
	v := VisitRow{SchoolID: school, StudentID: body.StudentID, VisitedAt: visitedAt, Outcome: body.Outcome, HandledBy: c.UserID,
		AttendanceSync: SyncNotNeeded}
	if eff.MarkSick {
		v.AttendanceSync, v.LeftAt = SyncPending, &now
	}
	if v.ID, err = NewID(r.Context(), tx); err == nil {
		aad := sensitive.AAD(tblVisits, school, v.ID)
		if v.DetailsEnc, err = h.Crypto.SealJSON(details, aad); err == nil && body.Outcome == OutcomeReferred {
			v.ReferredToEnc, err = h.Crypto.SealJSON(strings.TrimSpace(body.ReferredTo), aad)
		}
	}
	created := false
	if err == nil {
		created, err = InsertVisit(r.Context(), tx, v, key)
	}
	if err == nil && !created {
		tx.Rollback()
		existing, err := VisitByIdempotencyKey(r.Context(), h.DB, school, key)
		if err != nil {
			common.WriteError(w, http.StatusInternalServerError, "Gagal menyimpan kunjungan")
			return
		}
		h.respondVisit(w, r, c, existing, http.StatusOK)
		return
	}
	for _, m := range body.Medications {
		if err == nil {
			err = InsertVisitMedication(r.Context(), tx, school, v.ID, meds[m.MedicineID], m.Qty)
		}
	}
	if err == nil {
		err = h.audit(r.Context(), tx, c, school, tblVisits, v.ID, "create",
			map[string]any{"student_id": v.StudentID, "outcome": v.Outcome, "medications": len(body.Medications)})
	}
	if err == nil {
		err = h.logAccess(r.Context(), tx, r, c, school, tblVisits, v.ID, v.StudentID, sensitive.ActionCreate)
	}
	if err == nil {
		err = tx.Commit()
	}
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "Gagal menyimpan kunjungan")
		return
	}
	// Efek setelah tersimpan: presensi sakit + notifikasi wali. Gagal → job ulang.
	h.ProcessVisitEffects(r.Context(), v.ID, school)
	saved, err := GetVisit(r.Context(), h.DB, school, v.ID)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "Gagal memuat kunjungan")
		return
	}
	h.respondVisit(w, r, nil, saved, http.StatusCreated)
}

// respondVisit writes the decrypted visit; c != nil logs a view first.
func (h *Handler) respondVisit(w http.ResponseWriter, r *http.Request, c *common.Claims, v VisitRow, status int) {
	view, err := h.visitDetail(r.Context(), v)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "Gagal membuka kunjungan")
		return
	}
	if c != nil && !h.viewed(w, r, c, v.SchoolID, tblVisits, v.ID, v.StudentID) {
		return
	}
	common.WriteJSON(w, status, view, nil)
}

// ListVisits: GET /health/visits?student_id=&outcome=&from=&to= (tanpa isi terenkripsi).
func (h *Handler) ListVisits(w http.ResponseWriter, r *http.Request) {
	_, school := common.RequireRole(w, r, common.RolePetugasUKS)
	if school == "" {
		return
	}
	q := r.URL.Query()
	page, perPage := common.Page(r)
	list, total, err := ListVisits(r.Context(), h.DB, VisitFilter{SchoolID: school, StudentID: q.Get("student_id"),
		Outcome: q.Get("outcome"), From: q.Get("from"), To: q.Get("to")}, page, perPage)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "Gagal memuat kunjungan")
		return
	}
	ids := []string{}
	for _, v := range list {
		ids = append(ids, v.StudentID, v.HandledBy)
	}
	names := h.names(r.Context(), school, ids...)
	out := make([]VisitView, 0, len(list))
	for _, v := range list {
		out = append(out, baseVisitView(v, names))
	}
	common.WriteSuccess(w, out, common.Meta{Page: page, PerPage: perPage, Total: total})
}

// GetVisit: GET /health/visits/{id}
func (h *Handler) GetVisit(w http.ResponseWriter, r *http.Request) {
	c, school := common.RequireRole(w, r, common.RolePetugasUKS)
	if school == "" {
		return
	}
	v, err := GetVisit(r.Context(), h.DB, school, chi.URLParam(r, "id"))
	if errors.Is(err, store.ErrNotFound) {
		common.WriteError(w, http.StatusNotFound, "Data tidak ditemukan")
		return
	}
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "Gagal memuat kunjungan")
		return
	}
	h.respondVisit(w, r, c, v, http.StatusOK)
}

// AddAddendum: POST /health/visits/{id}/addenda {note}
func (h *Handler) AddAddendum(w http.ResponseWriter, r *http.Request) {
	c, school := common.RequireRole(w, r, common.RolePetugasUKS)
	if school == "" {
		return
	}
	var body struct {
		Note string `json:"note"`
	}
	if !common.Decode(w, r, &body) {
		return
	}
	if strings.TrimSpace(body.Note) == "" {
		common.WriteError(w, http.StatusUnprocessableEntity, "Catatan addendum wajib diisi")
		return
	}
	v, err := GetVisit(r.Context(), h.DB, school, chi.URLParam(r, "id"))
	if errors.Is(err, store.ErrNotFound) {
		common.WriteError(w, http.StatusNotFound, "Data tidak ditemukan")
		return
	}
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "Gagal menyimpan addendum")
		return
	}
	tx, err := h.DB.BeginTx(r.Context(), nil)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "Gagal menyimpan addendum")
		return
	}
	defer tx.Rollback()
	id, err := NewID(r.Context(), tx)
	var enc []byte
	if err == nil {
		enc, err = h.Crypto.SealJSON(strings.TrimSpace(body.Note), sensitive.AAD(tblAddenda, school, id))
	}
	if err == nil {
		_, err = InsertAddendum(r.Context(), tx, id, school, v.ID, enc, c.UserID)
	}
	if err == nil {
		err = h.audit(r.Context(), tx, c, school, tblVisits, v.ID, "addendum", map[string]any{"addendum_id": id})
	}
	if err == nil {
		err = h.logAccess(r.Context(), tx, r, c, school, tblVisits, v.ID, v.StudentID, sensitive.ActionUpdate)
	}
	if err == nil {
		err = tx.Commit()
	}
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "Gagal menyimpan addendum")
		return
	}
	h.respondVisit(w, r, nil, v, http.StatusCreated)
}

// StudentVisits: GET /health/students/{id}/visits (petugas, wali anak sendiri, peran berkebijakan).
func (h *Handler) StudentVisits(w http.ResponseWriter, r *http.Request) {
	c, school := common.RequireRole(w, r, common.RolePetugasUKS, common.RoleParent, common.RoleKepalaSekolah,
		common.RoleAdmin, common.RoleSuperadmin, common.RoleWalikelas, common.RoleGuru)
	if school == "" {
		return
	}
	studentID := chi.URLParam(r, "id")
	if h.level(r.Context(), c, school, studentID) != sensitive.Full {
		h.deny(w, r, c, school, tblVisits, "", studentID)
		return
	}
	page, perPage := common.Page(r)
	list, total, err := ListVisits(r.Context(), h.DB, VisitFilter{SchoolID: school, StudentID: studentID}, page, perPage)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "Gagal memuat kunjungan")
		return
	}
	out := make([]VisitView, 0, len(list))
	for _, v := range list {
		view, err := h.visitDetail(r.Context(), v)
		if err != nil {
			common.WriteError(w, http.StatusInternalServerError, "Gagal membuka kunjungan")
			return
		}
		out = append(out, view)
	}
	if !h.viewed(w, r, c, school, tblVisits, "", studentID) {
		return
	}
	common.WriteSuccess(w, out, common.Meta{Page: page, PerPage: perPage, Total: total})
}

// ---- efek kunjungan: presensi + notifikasi (dipakai handler dan job) -------

// ProcessVisitEffects pushes a visit's pending attendance sync and parent
// notifications. Failures stay pending for the job; after JOB_MAX_ATTEMPTS
// attendance sync becomes 'failed' and an exception is raised in core
// (payload tanpa data kesehatan).
func (h *Handler) ProcessVisitEffects(ctx context.Context, visitID, schoolID string) {
	v, err := GetVisit(ctx, h.DB, schoolID, visitID)
	if err != nil {
		return
	}
	eff := VisitEffects(v.Outcome)
	since := v.VisitedAt
	if v.LeftAt != nil {
		since = *v.LeftAt
	}
	loc := store.SchoolLocation(ctx, h.DB, schoolID, h.Cfg.DefaultTimezone)
	if v.AttendanceSync == SyncPending {
		res, err := h.Client.ApplyHealthAbsence(ctx, client.HealthAbsence{SchoolID: schoolID, StudentID: v.StudentID,
			Date: since.In(loc).Format("2006-01-02"), SickSince: since, SourceRef: v.ID, Outcome: v.Outcome,
			ActorUserID: v.HandledBy, Reason: SickNote(v.Outcome, since, loc)})
		switch {
		case err == nil && res.Applied:
			_ = SetAttendanceSync(ctx, h.DB, v.ID, SyncDone, "")
		case err == nil:
			_ = SetAttendanceSync(ctx, h.DB, v.ID, SyncSkipped, res.Reason)
		case v.AttendanceTries+1 >= h.Cfg.JobMaxAttempts:
			_ = SetAttendanceSync(ctx, h.DB, v.ID, SyncFailed, "sinkron presensi gagal")
			_ = h.Client.ReportException(ctx, client.IntegrationException{SchoolID: schoolID, SourceService: "studentlife",
				Kind: "callback", Severity: "medium", CorrelationID: v.ID, ErrorMessage: "Sinkron presensi sakit dari UKS gagal",
				PayloadSummary: map[string]any{"visit_id": v.ID, "student_id": v.StudentID, "target": "attendance"},
				MaxAttempts: h.Cfg.JobMaxAttempts, IdempotencyKey: "studentlife:absence:" + v.ID})
		default:
			_ = SetAttendanceSync(ctx, h.DB, v.ID, SyncPending, "sinkron presensi belum berhasil")
		}
	}
	if eff.NotifyParent && v.ParentNotifiedAt == nil {
		guardians, err := h.Client.StudentGuardians(ctx, v.StudentID, schoolID)
		if err != nil {
			return
		}
		name := h.names(ctx, schoolID, v.StudentID)[v.StudentID]
		if name == "" {
			name = "siswa"
		}
		title := "Ananda " + name + " " + OutcomeLabel(v.Outcome) + " dari UKS"
		body := "Ananda " + name + " " + OutcomeLabel(v.Outcome) + " dari UKS pukul " + since.In(loc).Format("15.04") + " " +
			zoneLabel(since.In(loc)) + ". Detail dapat dilihat di Portal Orang Tua."
		ok := true
		for _, g := range guardians {
			err := h.Client.Notify(ctx, client.Notification{SchoolID: schoolID, UserID: g.GuardianUserID, Type: "health_visit",
				Title: title, Body: body, Data: map[string]any{"visit_id": v.ID, "student_id": v.StudentID,
					"link": h.Cfg.PortalURL + "/orang-tua/kesehatan"}}, "studentlife:visit:"+v.ID+":"+g.GuardianUserID)
			ok = ok && err == nil
		}
		if ok {
			_ = MarkParentNotified(ctx, h.DB, v.ID)
		}
	}
}
