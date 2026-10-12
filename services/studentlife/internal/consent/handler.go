package consent

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"skci/studentlife/internal/client"
	"skci/studentlife/internal/common"
	"skci/studentlife/internal/store"
)

// Subject is a consent subject type registered by its owning module in main
// (health_program → health; T20 registers referral → counseling).
type Subject struct {
	Feature string
	// Describe returns a parent-facing summary of the subject (never health data).
	Describe func(ctx context.Context, schoolID, subjectID string) (map[string]any, error)
	// InUse reports whether a granted consent has already been relied upon.
	InUse func(ctx context.Context, schoolID, consentID string) (bool, error)
}

// Handler serves the generic /consents/* endpoints for orang_tua.
type Handler struct {
	DB       *sql.DB
	Client   *client.Client
	Subjects map[string]Subject
}

func (h *Handler) subjectEnabled(ctx context.Context, schoolID, subjectType string) bool {
	s, ok := h.Subjects[subjectType]
	if !ok {
		return false
	}
	on, err := h.Client.FeatureEnabled(ctx, schoolID, s.Feature)
	return err == nil && on
}

// MyConsent is a consent row enriched for the parent portal.
type MyConsent struct {
	Consent
	StudentName string         `json:"student_name"`
	Subject     map[string]any `json:"subject"`
}

// ListMine: GET /consents/my?subject_type=
func (h *Handler) ListMine(w http.ResponseWriter, r *http.Request) {
	c, school := common.RequireRole(w, r, common.RoleParent)
	if school == "" {
		return
	}
	children, err := h.Client.GuardianChildren(r.Context(), c.UserID, school)
	if err != nil {
		common.WriteError(w, http.StatusBadGateway, "Gagal memuat data anak")
		return
	}
	ids := make([]string, 0, len(children))
	names := map[string]string{}
	for _, ch := range children {
		ids = append(ids, ch.ID)
		names[ch.ID] = ch.Name
	}
	list, err := ListForStudents(r.Context(), h.DB, school, ids, r.URL.Query().Get("subject_type"))
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "Gagal memuat persetujuan")
		return
	}
	out := []MyConsent{}
	enabled := map[string]bool{}
	for _, cs := range list {
		on, seen := enabled[cs.SubjectType]
		if !seen {
			on = h.subjectEnabled(r.Context(), school, cs.SubjectType)
			enabled[cs.SubjectType] = on
		}
		if !on {
			continue
		}
		summary, err := h.Subjects[cs.SubjectType].Describe(r.Context(), school, cs.SubjectID)
		if err != nil {
			common.WriteError(w, http.StatusInternalServerError, "Gagal memuat persetujuan")
			return
		}
		out = append(out, MyConsent{Consent: cs, StudentName: names[cs.StudentID], Subject: summary})
	}
	common.WriteSuccess(w, out, nil)
}

// Decide: POST /consents/{id}/decide {decision, note}
func (h *Handler) Decide(w http.ResponseWriter, r *http.Request) {
	c, school := common.RequireRole(w, r, common.RoleParent)
	if school == "" {
		return
	}
	var body struct {
		Decision string `json:"decision"`
		Note     string `json:"note"`
	}
	if !common.Decode(w, r, &body) {
		return
	}
	cs, err := Get(r.Context(), h.DB, school, chi.URLParam(r, "id"))
	if errors.Is(err, store.ErrNotFound) {
		common.WriteError(w, http.StatusNotFound, "Persetujuan tidak ditemukan")
		return
	}
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "Gagal memuat persetujuan")
		return
	}
	if !h.subjectEnabled(r.Context(), school, cs.SubjectType) {
		common.WriteError(w, http.StatusNotFound, "Persetujuan tidak ditemukan")
		return
	}
	ok, err := h.Client.GuardianHasAccess(r.Context(), c.UserID, cs.StudentID, school)
	if err != nil || !ok {
		common.WriteError(w, http.StatusNotFound, "Persetujuan tidak ditemukan")
		return
	}
	used := false
	if body.Decision == StatusRevoked {
		if used, err = h.Subjects[cs.SubjectType].InUse(r.Context(), school, cs.ID); err != nil {
			common.WriteError(w, http.StatusInternalServerError, "Gagal memeriksa persetujuan")
			return
		}
	}
	from := cs.Status
	next, err := Decide(cs, body.Decision, c.UserID, used, time.Now())
	if errors.Is(err, ErrInvalidDecision) {
		common.WriteError(w, http.StatusUnprocessableEntity, err.Error())
		return
	}
	if err != nil {
		common.WriteError(w, http.StatusConflict, err.Error())
		return
	}
	tx, err := h.DB.BeginTx(r.Context(), nil)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "Gagal menyimpan keputusan")
		return
	}
	defer tx.Rollback()
	err = SaveDecision(r.Context(), tx, next, from, body.Note)
	if errors.Is(err, ErrFinal) {
		common.WriteError(w, http.StatusConflict, err.Error())
		return
	}
	if err == nil {
		err = store.Audit(r.Context(), tx, store.AuditEntry{SchoolID: school, Module: "consent", EntityType: "parent_consent",
			EntityID: cs.ID, Action: "decide", ActorUserID: &c.UserID, ActorRole: &c.ActiveRole,
			Before: map[string]any{"status": from}, After: map[string]any{"status": next.Status, "subject_type": cs.SubjectType}})
	}
	if err == nil {
		err = tx.Commit()
	}
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "Gagal menyimpan keputusan")
		return
	}
	if body.Note != "" {
		next.Note = &body.Note
	}
	common.WriteSuccess(w, next, nil)
}
