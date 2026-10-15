package activity

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"skci/studentlife/internal/client"
	"skci/studentlife/internal/common"
	"skci/studentlife/internal/config"
	"skci/studentlife/internal/consent"
	"skci/studentlife/internal/store"
)

type Handler struct {
	DB          *sql.DB
	Store       *Store
	Client      *client.Client
	Cfg         config.Config
	Consent     *consent.Handler
	StoragePath string
}

const ConsentSubjectEventRegistration = "event_registration"

func (h *Handler) ConsentSubject() consent.Subject {
	return consent.Subject{
		Feature: common.FeatureEvent,
		Describe: func(ctx context.Context, schoolID, subjectID string) (map[string]any, error) {
			reg, err := h.Store.GetRegistration(ctx, schoolID, subjectID)
			if err != nil {
				return map[string]any{"title": "Pendaftaran Kegiatan", "details": "Persetujuan orang tua untuk kegiatan sekolah"}, nil
			}
			event, _ := h.Store.GetEvent(ctx, schoolID, reg.EventID)
			eventName := reg.EventID
			fee := int64(0)
			consentText := ""
			dateStr := ""
			if event != nil {
				eventName = event.Name
				fee = event.FeeIDR
				if event.ConsentText != nil {
					consentText = *event.ConsentText
				}
				dateStr = event.StartsAt.Format("02-01-2006 15:04")
			}
			return map[string]any{
				"title":        fmt.Sprintf("Persetujuan Kegiatan: %s", eventName),
				"event_name":   eventName,
				"date":         dateStr,
				"fee_idr":      fee,
				"consent_text": consentText,
			}, nil
		},
		InUse: func(ctx context.Context, schoolID, consentID string) (bool, error) {
			return false, nil
		},
		OnDecided: func(ctx context.Context, tx *sql.Tx, schoolID, subjectID, decision string) error {
			newStatus := RegStatusDeclined
			if decision == "granted" {
				newStatus = RegStatusConfirmed
			}
			_, err := tx.ExecContext(ctx, `
				UPDATE act_event_registrations
				SET status = $1, updated_at = now()
				WHERE id = $2 AND school_id = $3`, newStatus, subjectID, schoolID)
			if err != nil {
				return err
			}

			// If confirmed and incurs fee, issue ad-hoc bill (AC-134-01)
			if decision == "granted" {
				var eventID, studentID string
				var feeIDR int64
				var eventName string
				var startsAt time.Time
				err := tx.QueryRowContext(ctx, `
					SELECT r.event_id, r.student_user_id, e.fee_idr, e.name, e.starts_at
					FROM act_event_registrations r
					JOIN act_events e ON r.event_id = e.id
					WHERE r.id = $1 AND r.school_id = $2`, subjectID, schoolID).Scan(&eventID, &studentID, &feeIDR, &eventName, &startsAt)
				if err == nil && feeIDR > 0 {
					billID, bErr := h.Client.CreateAdhocBill(ctx, client.AdhocBillRequest{
						SchoolID:      schoolID,
						StudentUserID: studentID,
						SourceModule:  "event",
						SourceRef:     subjectID,
						Items: []client.AdhocBillItem{
							{Name: fmt.Sprintf("Biaya Kegiatan: %s", eventName), AmountIDR: feeIDR},
						},
						DueDate: startsAt.Format("2006-01-02"),
					}, fmt.Sprintf("event:%s", subjectID))
					if bErr == nil && billID != "" {
						_, _ = tx.ExecContext(ctx, `UPDATE act_event_registrations SET bill_id = $1 WHERE id = $2`, billID, subjectID)
					}
				}
			}
			return nil
		},
	}
}

func (h *Handler) ApplyApprovalDecision(ctx context.Context, tx *sql.Tx, eventID, decision string, decidedBy string) error {
	newStatus := EventStatusRejected
	if decision == "approved" {
		newStatus = EventStatusApproved
	}

	res, err := tx.ExecContext(ctx, `
		UPDATE act_events
		SET status = $1, updated_at = now()
		WHERE id = $2 AND status = 'pending_approval'`, newStatus, eventID)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return nil
	}

	if decision == "rejected" {
		var bookingID *string
		var schoolID string
		_ = tx.QueryRowContext(ctx, `SELECT booking_id, school_id FROM act_events WHERE id = $1`, eventID).Scan(&bookingID, &schoolID)
		if bookingID != nil && *bookingID != "" {
			_ = h.Client.CancelFacilityBooking(ctx, *bookingID, schoolID, "Event approval rejected", decidedBy)
		}
	}
	return nil
}

func NewHandler(db *sql.DB, s *Store, c *client.Client, cfg config.Config, cons *consent.Handler) *Handler {
	storagePath := cfg.StoragePath
	if storagePath == "" {
		storagePath = "./data/storage"
	}
	_ = os.MkdirAll(filepath.Join(storagePath, "achievements"), 0755)

	return &Handler{
		DB:          db,
		Store:       s,
		Client:      c,
		Cfg:         cfg,
		Consent:     cons,
		StoragePath: storagePath,
	}
}

// -------------------------------------------------------------
// 1. Event Kinds
// -------------------------------------------------------------

func (h *Handler) ListEventKinds(w http.ResponseWriter, r *http.Request) {
	claims := common.GetClaims(r.Context())
	kinds, err := h.Store.ListEventKinds(r.Context(), claims.SchoolID)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "Gagal memuat jenis kegiatan")
		return
	}
	common.WriteSuccess(w, kinds, nil)
}

func (h *Handler) CreateEventKind(w http.ResponseWriter, r *http.Request) {
	claims := common.GetClaims(r.Context())
	var body struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || strings.TrimSpace(body.Name) == "" {
		common.WriteError(w, http.StatusBadRequest, "Nama jenis kegiatan wajib diisi")
		return
	}

	created, err := h.Store.CreateEventKind(r.Context(), EventKind{
		SchoolID: claims.SchoolID,
		Name:     strings.TrimSpace(body.Name),
	})
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "Gagal menyimpan jenis kegiatan")
		return
	}
	common.WriteJSON(w, http.StatusCreated, created, nil)
}

func (h *Handler) UpdateEventKind(w http.ResponseWriter, r *http.Request) {
	claims := common.GetClaims(r.Context())
	id := chi.URLParam(r, "id")
	var body struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || strings.TrimSpace(body.Name) == "" {
		common.WriteError(w, http.StatusBadRequest, "Nama jenis kegiatan wajib diisi")
		return
	}

	updated, err := h.Store.UpdateEventKind(r.Context(), claims.SchoolID, id, strings.TrimSpace(body.Name))
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			common.WriteError(w, http.StatusNotFound, "Jenis kegiatan tidak ditemukan")
			return
		}
		common.WriteError(w, http.StatusInternalServerError, "Gagal memperbarui jenis kegiatan")
		return
	}
	common.WriteSuccess(w, updated, nil)
}

func (h *Handler) DeleteEventKind(w http.ResponseWriter, r *http.Request) {
	claims := common.GetClaims(r.Context())
	id := chi.URLParam(r, "id")

	err := h.Store.DeleteEventKind(r.Context(), claims.SchoolID, id)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			common.WriteError(w, http.StatusNotFound, "Jenis kegiatan tidak ditemukan")
			return
		}
		common.WriteError(w, http.StatusInternalServerError, "Gagal menghapus jenis kegiatan")
		return
	}
	common.WriteSuccess(w, map[string]string{"id": id, "status": "deleted"}, nil)
}

// -------------------------------------------------------------
// 2. Events
// -------------------------------------------------------------

func (h *Handler) ListEvents(w http.ResponseWriter, r *http.Request) {
	claims := common.GetClaims(r.Context())
	status := r.URL.Query().Get("status")
	from := r.URL.Query().Get("from")
	to := r.URL.Query().Get("to")

	events, err := h.Store.ListEvents(r.Context(), claims.SchoolID, status, from, to)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "Gagal memuat daftar kegiatan")
		return
	}
	common.WriteSuccess(w, events, nil)
}

func (h *Handler) GetEvent(w http.ResponseWriter, r *http.Request) {
	claims := common.GetClaims(r.Context())
	id := chi.URLParam(r, "id")

	event, err := h.Store.GetEvent(r.Context(), claims.SchoolID, id)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			common.WriteError(w, http.StatusNotFound, "Kegiatan tidak ditemukan")
			return
		}
		common.WriteError(w, http.StatusInternalServerError, "Gagal memuat detail kegiatan")
		return
	}
	common.WriteSuccess(w, event, nil)
}

type CreateEventInput struct {
	Name                  string      `json:"name"`
	KindID                string      `json:"kind_id"`
	Description           *string     `json:"description"`
	StartsAt              string      `json:"starts_at"`
	EndsAt                string      `json:"ends_at"`
	RoomID                *string     `json:"room_id"`
	LocationText          *string     `json:"location_text"`
	Target                EventTarget `json:"target"`
	Quota                 *int        `json:"quota"`
	FeeIDR                int64       `json:"fee_idr"`
	RequiresParentConsent bool        `json:"requires_parent_consent"`
	ConsentText           *string     `json:"consent_text"`
	BudgetLineID          *string     `json:"budget_line_id"`
	BudgetAmountIDR       *int64      `json:"budget_amount_idr"`
	RegistrationOpensAt   *string     `json:"registration_opens_at"`
	RegistrationClosesAt  *string     `json:"registration_closes_at"`
	CommitteeUserIDs      []string    `json:"committee_user_ids"`
}

func (h *Handler) CreateEvent(w http.ResponseWriter, r *http.Request) {
	claims := common.GetClaims(r.Context())
	var in CreateEventInput
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		common.WriteError(w, http.StatusBadRequest, "Format payload tidak valid")
		return
	}

	startsAt, errS := time.Parse(time.RFC3339, in.StartsAt)
	endsAt, errE := time.Parse(time.RFC3339, in.EndsAt)
	if errS != nil || errE != nil || !endsAt.After(startsAt) {
		common.WriteError(w, http.StatusUnprocessableEntity, "Waktu mulai dan selesai kegiatan tidak valid")
		return
	}

	eventID := store.NewID()
	var bookingID *string

	// Room booking via T9 facility integration (FR-133-002, AC-133-01)
	if in.RoomID != nil && *in.RoomID != "" {
		participants := 30
		if in.Quota != nil && *in.Quota > 0 {
			participants = *in.Quota
		}

		idemKey := fmt.Sprintf("event:%s:%s", eventID, startsAt.Format(time.RFC3339))
		bkRes, conflictRej, err := h.Client.CreateFacilityBooking(r.Context(), client.BookingRequest{
			SchoolID:        claims.SchoolID,
			RoomID:          *in.RoomID,
			StartsAt:        in.StartsAt,
			EndsAt:          in.EndsAt,
			Purpose:         in.Name,
			Participants:    participants,
			RequesterUserID: claims.UserID,
			SourceService:   "studentlife",
			SourceRef:       eventID,
			IdempotencyKey:  idemKey,
		})
		if err != nil {
			if conflictRej != nil {
				// AC-133-01: return 409 + nearest free slots, DO NOT save event
				common.WriteJSON(w, http.StatusConflict, map[string]any{
					"error":         conflictRej.Error,
					"conflict":      true,
					"nearest_slots": conflictRej.NearestSlots,
				}, nil)
				return
			}
			var he *client.HTTPError
			if errors.As(err, &he) {
				common.WriteError(w, he.Status, he.Message())
				return
			}
			common.WriteError(w, http.StatusInternalServerError, "Gagal memproses pemesanan ruang")
			return
		}
		if bkRes != nil && bkRes.ID != "" {
			bookingID = &bkRes.ID
		}
	}

	var regOpensAt, regClosesAt *time.Time
	if in.RegistrationOpensAt != nil {
		if t, err := time.Parse(time.RFC3339, *in.RegistrationOpensAt); err == nil {
			regOpensAt = &t
		}
	}
	if in.RegistrationClosesAt != nil {
		if t, err := time.Parse(time.RFC3339, *in.RegistrationClosesAt); err == nil {
			regClosesAt = &t
		}
	}

	ev := Event{
		ID:                    eventID,
		SchoolID:              claims.SchoolID,
		Name:                  strings.TrimSpace(in.Name),
		KindID:                in.KindID,
		Description:           in.Description,
		StartsAt:              startsAt,
		EndsAt:                endsAt,
		RoomID:                in.RoomID,
		BookingID:             bookingID,
		LocationText:          in.LocationText,
		Target:                in.Target,
		Quota:                 in.Quota,
		FeeIDR:                in.FeeIDR,
		RequiresParentConsent: in.RequiresParentConsent,
		ConsentText:           in.ConsentText,
		BudgetLineID:          in.BudgetLineID,
		BudgetAmountIDR:       in.BudgetAmountIDR,
		RegistrationOpensAt:   regOpensAt,
		RegistrationClosesAt:  regClosesAt,
		Status:                EventStatusDraft,
		CreatedBy:             claims.UserID,
	}

	created, err := h.Store.CreateEvent(r.Context(), ev, in.CommitteeUserIDs)
	if err != nil {
		// Compensation: cancel booking if event creation fails
		if bookingID != nil {
			_ = h.Client.CancelFacilityBooking(r.Context(), *bookingID, claims.SchoolID, "Rollback event failure", claims.UserID)
		}
		common.WriteError(w, http.StatusInternalServerError, "Gagal menyimpan kegiatan")
		return
	}

	common.WriteJSON(w, http.StatusCreated, created, nil)
}

func (h *Handler) UpdateEvent(w http.ResponseWriter, r *http.Request) {
	claims := common.GetClaims(r.Context())
	id := chi.URLParam(r, "id")

	var in CreateEventInput
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		common.WriteError(w, http.StatusBadRequest, "Format payload tidak valid")
		return
	}

	startsAt, errS := time.Parse(time.RFC3339, in.StartsAt)
	endsAt, errE := time.Parse(time.RFC3339, in.EndsAt)
	if errS != nil || errE != nil || !endsAt.After(startsAt) {
		common.WriteError(w, http.StatusUnprocessableEntity, "Waktu kegiatan tidak valid")
		return
	}

	var regOpensAt, regClosesAt *time.Time
	if in.RegistrationOpensAt != nil {
		if t, err := time.Parse(time.RFC3339, *in.RegistrationOpensAt); err == nil {
			regOpensAt = &t
		}
	}
	if in.RegistrationClosesAt != nil {
		if t, err := time.Parse(time.RFC3339, *in.RegistrationClosesAt); err == nil {
			regClosesAt = &t
		}
	}

	ev := Event{
		ID:                    id,
		SchoolID:              claims.SchoolID,
		Name:                  strings.TrimSpace(in.Name),
		KindID:                in.KindID,
		Description:           in.Description,
		StartsAt:              startsAt,
		EndsAt:                endsAt,
		RoomID:                in.RoomID,
		LocationText:          in.LocationText,
		Target:                in.Target,
		Quota:                 in.Quota,
		FeeIDR:                in.FeeIDR,
		RequiresParentConsent: in.RequiresParentConsent,
		ConsentText:           in.ConsentText,
		BudgetLineID:          in.BudgetLineID,
		BudgetAmountIDR:       in.BudgetAmountIDR,
		RegistrationOpensAt:   regOpensAt,
		RegistrationClosesAt:  regClosesAt,
	}

	if err := h.Store.UpdateEvent(r.Context(), ev); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			common.WriteError(w, http.StatusNotFound, "Kegiatan draft tidak ditemukan")
			return
		}
		common.WriteError(w, http.StatusInternalServerError, "Gagal memperbarui kegiatan")
		return
	}

	common.WriteSuccess(w, map[string]string{"id": id, "status": "updated"}, nil)
}

func (h *Handler) DeleteEvent(w http.ResponseWriter, r *http.Request) {
	claims := common.GetClaims(r.Context())
	id := chi.URLParam(r, "id")

	existing, err := h.Store.GetEvent(r.Context(), claims.SchoolID, id)
	if err != nil {
		common.WriteError(w, http.StatusNotFound, "Kegiatan tidak ditemukan")
		return
	}

	if err := h.Store.DeleteEvent(r.Context(), claims.SchoolID, id); err != nil {
		common.WriteError(w, http.StatusBadRequest, "Hanya kegiatan draft yang dapat dihapus")
		return
	}

	if existing.BookingID != nil {
		_ = h.Client.CancelFacilityBooking(r.Context(), *existing.BookingID, claims.SchoolID, "Event deleted", claims.UserID)
	}

	common.WriteSuccess(w, map[string]string{"id": id, "status": "deleted"}, nil)
}

func (h *Handler) SubmitEvent(w http.ResponseWriter, r *http.Request) {
	claims := common.GetClaims(r.Context())
	id := chi.URLParam(r, "id")

	event, err := h.Store.GetEvent(r.Context(), claims.SchoolID, id)
	if err != nil {
		common.WriteError(w, http.StatusNotFound, "Kegiatan tidak ditemukan")
		return
	}
	if event.Status != EventStatusDraft {
		common.WriteError(w, http.StatusBadRequest, "Hanya kegiatan draft yang dapat diajukan")
		return
	}

	// Check if approval is needed (FR-133-003)
	if !NeedsApproval(*event) {
		// No fee & no budget -> can directly publish or mark approved
		_ = h.Store.UpdateEventStatus(r.Context(), claims.SchoolID, id, EventStatusApproved, nil, nil)
		common.WriteSuccess(w, map[string]string{"id": id, "status": EventStatusApproved, "message": "Disetujui otomatis (tanpa biaya/anggaran)"}, nil)
		return
	}

	// Budget check if BUDGET_CHECK_URL configured (T17 hook)
	var budgetRes *BudgetCheckResult
	if h.Cfg.BudgetCheckURL != "" && event.BudgetLineID != nil {
		bAmt := int64(0)
		if event.BudgetAmountIDR != nil {
			bAmt = *event.BudgetAmountIDR
		}
		res, err := h.Client.CheckBudget(r.Context(), h.Cfg.BudgetCheckURL, client.BudgetCheckRequest{
			SchoolID:      claims.SchoolID,
			BudgetLineID:  *event.BudgetLineID,
			AmountIDR:     bAmt,
			SourceService: "studentlife",
			SourceType:    "event",
			SourceID:      id,
		})
		if err != nil {
			common.WriteError(w, http.StatusBadGateway, "Gagal verifikasi anggaran")
			return
		}
		if res != nil {
			budgetRes = &BudgetCheckResult{
				AvailableIDR: res.AvailableIDR,
				OverBudget:   res.OverBudget,
				Policy:       res.Policy,
			}
			if res.OverBudget && res.Policy == "reject" {
				common.WriteError(w, http.StatusUnprocessableEntity, "Melebihi sisa anggaran kegiatan")
				return
			}
		}
	}

	flags := ApprovalFlags(*event, budgetRes)
	approvalAmount := event.FeeIDR
	if event.BudgetAmountIDR != nil && *event.BudgetAmountIDR > 0 {
		approvalAmount = *event.BudgetAmountIDR
	}

	approvalID, err := h.Client.CreateApproval(r.Context(), client.ApprovalRequest{
		SchoolID:      claims.SchoolID,
		RequestType:   "event",
		SourceService: "studentlife",
		SourceID:      id,
		Title:         fmt.Sprintf("Pengajuan Kegiatan: %s", event.Name),
		Summary: map[string]any{
			"kegiatan":        event.Name,
			"waktu":           event.StartsAt.Format("02-01-2006 15:04"),
			"fee_idr":         event.FeeIDR,
			"budget_line_id":  event.BudgetLineID,
			"budget_amount":   event.BudgetAmountIDR,
			"approval_amount": approvalAmount,
			"approval_flags":  flags,
		},
		MakerUserID:    claims.UserID,
		CallbackURL:    h.Cfg.PublicInternalURL + "/internal/approval-callbacks",
		CorrelationID:  id,
		IdempotencyKey: fmt.Sprintf("event:%s", id),
	})
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "Gagal membuat permohonan persetujuan ke core")
		return
	}

	_ = h.Store.UpdateEventStatus(r.Context(), claims.SchoolID, id, EventStatusPendingApproval, &approvalID, nil)
	common.WriteSuccess(w, map[string]string{"id": id, "status": EventStatusPendingApproval, "approval_id": approvalID}, nil)
}

func (h *Handler) PublishEvent(w http.ResponseWriter, r *http.Request) {
	claims := common.GetClaims(r.Context())
	id := chi.URLParam(r, "id")

	event, err := h.Store.GetEvent(r.Context(), claims.SchoolID, id)
	if err != nil {
		common.WriteError(w, http.StatusNotFound, "Kegiatan tidak ditemukan")
		return
	}

	// Allowed if approved, or draft if free without budget
	if event.Status != EventStatusApproved && (event.Status != EventStatusDraft || NeedsApproval(*event)) {
		common.WriteError(w, http.StatusBadRequest, "Kegiatan belum disetujui untuk dipublikasi")
		return
	}

	now := time.Now()
	if err := h.Store.UpdateEventStatus(r.Context(), claims.SchoolID, id, EventStatusPublished, nil, &now); err != nil {
		common.WriteError(w, http.StatusInternalServerError, "Gagal mempublikasikan kegiatan")
		return
	}

	// Notify targeted students / staff
	_ = h.Client.Notify(r.Context(), client.Notification{
		SchoolID: claims.SchoolID,
		UserID:   claims.UserID, // broadcast/self fallback
		Type:     "event_published",
		Title:    fmt.Sprintf("Kegiatan Baru: %s", event.Name),
		Body:     fmt.Sprintf("Kegiatan %s akan berlangsung pada %s. Silakan mendaftar!", event.Name, event.StartsAt.Format("02 Jan 2006")),
		Data:     map[string]any{"event_id": id},
	}, fmt.Sprintf("event:%s:published", id))

	common.WriteSuccess(w, map[string]string{"id": id, "status": EventStatusPublished}, nil)
}

func (h *Handler) CancelEvent(w http.ResponseWriter, r *http.Request) {
	claims := common.GetClaims(r.Context())
	id := chi.URLParam(r, "id")

	event, err := h.Store.GetEvent(r.Context(), claims.SchoolID, id)
	if err != nil {
		common.WriteError(w, http.StatusNotFound, "Kegiatan tidak ditemukan")
		return
	}

	_ = h.Store.UpdateEventStatus(r.Context(), claims.SchoolID, id, EventStatusCancelled, nil, nil)
	if event.BookingID != nil {
		_ = h.Client.CancelFacilityBooking(r.Context(), *event.BookingID, claims.SchoolID, "Event cancelled", claims.UserID)
	}

	common.WriteSuccess(w, map[string]string{"id": id, "status": EventStatusCancelled}, nil)
}

// -------------------------------------------------------------
// 3. Event Registrations & Attendance
// -------------------------------------------------------------

func (h *Handler) ListEventRegistrations(w http.ResponseWriter, r *http.Request) {
	claims := common.GetClaims(r.Context())
	id := chi.URLParam(r, "id")

	regs, err := h.Store.ListEventRegistrations(r.Context(), claims.SchoolID, id)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "Gagal memuat peserta kegiatan")
		return
	}
	common.WriteSuccess(w, regs, nil)
}

func (h *Handler) RegisterStudent(w http.ResponseWriter, r *http.Request) {
	eventID := chi.URLParam(r, "id")

	var body struct {
		StudentID string `json:"student_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || strings.TrimSpace(body.StudentID) == "" {
		common.WriteError(w, http.StatusBadRequest, "student_id wajib diisi")
		return
	}

	h.registerCore(w, r, eventID, strings.TrimSpace(body.StudentID), "event_coordinator", true)
}

func (h *Handler) RegisterSelf(w http.ResponseWriter, r *http.Request) {
	claims := common.GetClaims(r.Context())
	eventID := chi.URLParam(r, "id")

	h.registerCore(w, r, eventID, claims.UserID, "siswa", false)
}

func (h *Handler) RegisterByParent(w http.ResponseWriter, r *http.Request) {
	claims := common.GetClaims(r.Context())
	eventID := chi.URLParam(r, "id")

	var body struct {
		StudentID string `json:"student_id"`
		Consent   bool   `json:"consent"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || strings.TrimSpace(body.StudentID) == "" {
		common.WriteError(w, http.StatusBadRequest, "student_id wajib diisi")
		return
	}

	// Verify parent access
	hasAccess, err := h.Client.GuardianHasAccess(r.Context(), claims.UserID, body.StudentID, claims.SchoolID)
	if err != nil || !hasAccess {
		common.WriteError(w, http.StatusForbidden, "Anda bukan wali sah dari siswa ini")
		return
	}

	h.registerCore(w, r, eventID, strings.TrimSpace(body.StudentID), "orang_tua", body.Consent)
}

func (h *Handler) registerCore(w http.ResponseWriter, r *http.Request, eventID, studentID, via string, consentGiven bool) {
	claims := common.GetClaims(r.Context())

	event, err := h.Store.GetEvent(r.Context(), claims.SchoolID, eventID)
	if err != nil {
		common.WriteError(w, http.StatusNotFound, "Kegiatan tidak ditemukan")
		return
	}
	if event.Status != EventStatusPublished {
		common.WriteError(w, http.StatusBadRequest, "Pendaftaran hanya terbuka untuk kegiatan yang telah dipublikasikan")
		return
	}

	// Seat decision check
	taken, _ := h.Store.CountActiveRegistrations(r.Context(), claims.SchoolID, eventID)
	allowed, reason := EventSeatDecision(event.Quota, taken)
	if !allowed {
		common.WriteError(w, http.StatusUnprocessableEntity, reason)
		return
	}

	// Check if already registered
	existing, _ := h.Store.GetRegistrationByStudent(r.Context(), claims.SchoolID, eventID, studentID)
	if existing != nil {
		common.WriteError(w, http.StatusBadRequest, "Siswa sudah terdaftar pada kegiatan ini")
		return
	}

	initialStatus := RegistrationStatus(*event, via, consentGiven)
	var consentID *string
	var consentTextSnapshot *string

	regID := store.NewID()

	// If pending consent, create consent record (FR-134-002, BR-EVT-01)
	if initialStatus == RegStatusPendingConsent {
		cID := store.NewID()
		consentID = &cID
		consentTextSnapshot = event.ConsentText

		if _, err := h.DB.ExecContext(r.Context(), `
			INSERT INTO parent_consents (id, school_id, subject_type, subject_id, student_id, status, requested_by, note)
			VALUES ($1, $2, 'event_registration', $3, $4, 'pending', $5, $6)`,
			cID, claims.SchoolID, regID, studentID, claims.UserID, event.Name); err != nil {
			common.WriteError(w, http.StatusInternalServerError, "Gagal membuat permintaan persetujuan orang tua")
			return
		}

		// Notify guardians
		guardians, _ := h.Client.StudentGuardians(r.Context(), studentID, claims.SchoolID)
		for _, g := range guardians {
			_ = h.Client.Notify(r.Context(), client.Notification{
				SchoolID: claims.SchoolID,
				UserID:   g.GuardianUserID,
				Type:     "event_consent_request",
				Title:    fmt.Sprintf("Persetujuan Kegiatan: %s", event.Name),
				Body:     fmt.Sprintf("Ananda mendaftar kegiatan %s. Mohon berikan persetujuan Anda.", event.Name),
				Data:     map[string]any{"consent_id": cID, "event_id": event.ID},
			}, fmt.Sprintf("studentlife:consent:%s:%s", cID, g.GuardianUserID))
		}
	}

	var billID *string
	// If confirmed and incurs fee, issue ad-hoc bill immediately (AC-134-01)
	if canIssue, amount := CanIssueEventBill(EventRegistration{Status: initialStatus}, *event); canIssue {
		bID, err := h.Client.CreateAdhocBill(r.Context(), client.AdhocBillRequest{
			SchoolID:      claims.SchoolID,
			StudentUserID: studentID,
			SourceModule:  "event",
			SourceRef:     regID,
			Items: []client.AdhocBillItem{
				{Name: fmt.Sprintf("Biaya Kegiatan: %s", event.Name), AmountIDR: amount},
			},
			DueDate: event.StartsAt.Format("2006-01-02"),
		}, fmt.Sprintf("event:%s", regID))
		if err == nil && bID != "" {
			billID = &bID
		}
	}

	created, err := h.Store.CreateRegistration(r.Context(), EventRegistration{
		ID:                  regID,
		SchoolID:            claims.SchoolID,
		EventID:             eventID,
		StudentUserID:       studentID,
		RegisteredBy:        claims.UserID,
		RegisteredVia:       via,
		Status:              initialStatus,
		ConsentID:           consentID,
		ConsentTextSnapshot: consentTextSnapshot,
		BillID:              billID,
	})
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "Gagal mendaftarkan siswa")
		return
	}

	common.WriteJSON(w, http.StatusCreated, created, nil)
}

func (h *Handler) CancelRegistration(w http.ResponseWriter, r *http.Request) {
	claims := common.GetClaims(r.Context())
	regID := chi.URLParam(r, "id")

	reg, err := h.Store.GetRegistration(r.Context(), claims.SchoolID, regID)
	if err != nil {
		common.WriteError(w, http.StatusNotFound, "Pendaftaran tidak ditemukan")
		return
	}
	if reg.StudentUserID != claims.UserID && claims.ActiveRole != "event_coordinator" && claims.ActiveRole != "admin" {
		common.WriteError(w, http.StatusForbidden, "Akses ditolak")
		return
	}

	_ = h.Store.UpdateRegistrationStatus(r.Context(), claims.SchoolID, regID, RegStatusCancelled, nil, nil, nil)
	common.WriteSuccess(w, map[string]string{"id": regID, "status": RegStatusCancelled}, nil)
}

func (h *Handler) MarkAttendance(w http.ResponseWriter, r *http.Request) {
	claims := common.GetClaims(r.Context())
	regID := chi.URLParam(r, "id")

	reg, err := h.Store.GetRegistration(r.Context(), claims.SchoolID, regID)
	if err != nil {
		common.WriteError(w, http.StatusNotFound, "Pendaftaran tidak ditemukan")
		return
	}

	event, err := h.Store.GetEvent(r.Context(), claims.SchoolID, reg.EventID)
	if err != nil {
		common.WriteError(w, http.StatusNotFound, "Kegiatan tidak ditemukan")
		return
	}

	now := time.Now()
	canMark, reason := CanMarkAttendance(*reg, *event, now)
	if !canMark {
		common.WriteError(w, http.StatusUnprocessableEntity, reason)
		return
	}

	if err := h.Store.MarkAttendance(r.Context(), claims.SchoolID, regID, claims.UserID, now); err != nil {
		common.WriteError(w, http.StatusInternalServerError, "Gagal mencatat kehadiran")
		return
	}

	common.WriteSuccess(w, map[string]string{"id": regID, "status": "attended"}, nil)
}

// Calendar view
func (h *Handler) Calendar(w http.ResponseWriter, r *http.Request) {
	claims := common.GetClaims(r.Context())
	from := r.URL.Query().Get("from")
	to := r.URL.Query().Get("to")

	events, err := h.Store.ListPublishedEvents(r.Context(), claims.SchoolID, from, to)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "Gagal memuat kalender")
		return
	}
	common.WriteSuccess(w, events, nil)
}

// -------------------------------------------------------------
// 4. Extracurriculars & Periods
// -------------------------------------------------------------

func (h *Handler) ListPeriods(w http.ResponseWriter, r *http.Request) {
	claims := common.GetClaims(r.Context())
	periods, err := h.Store.ListPeriods(r.Context(), claims.SchoolID)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "Gagal memuat periode ekskul")
		return
	}
	common.WriteSuccess(w, periods, nil)
}

func (h *Handler) CreatePeriod(w http.ResponseWriter, r *http.Request) {
	claims := common.GetClaims(r.Context())
	var in struct {
		Name           string  `json:"name"`
		AcademicYearID *string `json:"academic_year_id"`
		OpensAt        string  `json:"opens_at"`
		ClosesAt       string  `json:"closes_at"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		common.WriteError(w, http.StatusBadRequest, "Payload tidak valid")
		return
	}
	opensAt, errO := time.Parse(time.RFC3339, in.OpensAt)
	closesAt, errC := time.Parse(time.RFC3339, in.ClosesAt)
	if errO != nil || errC != nil || !closesAt.After(opensAt) {
		common.WriteError(w, http.StatusUnprocessableEntity, "Rentang tanggal periode tidak valid")
		return
	}

	p, err := h.Store.CreatePeriod(r.Context(), EccPeriod{
		SchoolID:       claims.SchoolID,
		AcademicYearID: in.AcademicYearID,
		Name:           strings.TrimSpace(in.Name),
		OpensAt:        opensAt,
		ClosesAt:       closesAt,
	})
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "Gagal menyimpan periode")
		return
	}
	common.WriteJSON(w, http.StatusCreated, p, nil)
}

func (h *Handler) ListExtracurriculars(w http.ResponseWriter, r *http.Request) {
	claims := common.GetClaims(r.Context())
	activeOnly := r.URL.Query().Get("active") == "true"

	list, err := h.Store.ListExtracurriculars(r.Context(), claims.SchoolID, activeOnly)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "Gagal memuat ekstrakurikuler")
		return
	}
	common.WriteSuccess(w, list, nil)
}

func (h *Handler) GetExtracurricular(w http.ResponseWriter, r *http.Request) {
	claims := common.GetClaims(r.Context())
	id := chi.URLParam(r, "id")

	ecc, err := h.Store.GetExtracurricular(r.Context(), claims.SchoolID, id)
	if err != nil {
		common.WriteError(w, http.StatusNotFound, "Ekstrakurikuler tidak ditemukan")
		return
	}
	common.WriteSuccess(w, ecc, nil)
}

func (h *Handler) CreateExtracurricular(w http.ResponseWriter, r *http.Request) {
	claims := common.GetClaims(r.Context())
	var in struct {
		Name         string          `json:"name"`
		Description  *string         `json:"description"`
		Schedule     json.RawMessage `json:"schedule"`
		RoomID       *string         `json:"room_id"`
		LocationText *string         `json:"location_text"`
		Quota        *int            `json:"quota"`
		FeeIDR       int64           `json:"fee_idr"`
		FullPolicy   string          `json:"full_policy"`
		Active       bool            `json:"active"`
		Coaches      []string        `json:"coaches"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil || strings.TrimSpace(in.Name) == "" {
		common.WriteError(w, http.StatusBadRequest, "Nama ekskul wajib diisi")
		return
	}

	if in.FullPolicy == "" {
		in.FullPolicy = EccFullPolicyReject
	}

	ecc, err := h.Store.CreateExtracurricular(r.Context(), Extracurricular{
		SchoolID:     claims.SchoolID,
		Name:         strings.TrimSpace(in.Name),
		Description:  in.Description,
		Schedule:     in.Schedule,
		RoomID:       in.RoomID,
		LocationText: in.LocationText,
		Quota:        in.Quota,
		FeeIDR:       in.FeeIDR,
		FullPolicy:   in.FullPolicy,
		Active:       in.Active,
	}, in.Coaches)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "Gagal membuat ekstrakurikuler")
		return
	}
	common.WriteJSON(w, http.StatusCreated, ecc, nil)
}

func (h *Handler) UpdateExtracurricular(w http.ResponseWriter, r *http.Request) {
	claims := common.GetClaims(r.Context())
	id := chi.URLParam(r, "id")

	var in struct {
		Name         string          `json:"name"`
		Description  *string         `json:"description"`
		Schedule     json.RawMessage `json:"schedule"`
		RoomID       *string         `json:"room_id"`
		LocationText *string         `json:"location_text"`
		Quota        *int            `json:"quota"`
		FeeIDR       int64           `json:"fee_idr"`
		FullPolicy   string          `json:"full_policy"`
		Active       bool            `json:"active"`
		Coaches      []string        `json:"coaches"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		common.WriteError(w, http.StatusBadRequest, "Payload tidak valid")
		return
	}

	err := h.Store.UpdateExtracurricular(r.Context(), Extracurricular{
		ID:           id,
		SchoolID:     claims.SchoolID,
		Name:         strings.TrimSpace(in.Name),
		Description:  in.Description,
		Schedule:     in.Schedule,
		RoomID:       in.RoomID,
		LocationText: in.LocationText,
		Quota:        in.Quota,
		FeeIDR:       in.FeeIDR,
		FullPolicy:   in.FullPolicy,
		Active:       in.Active,
	}, in.Coaches)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "Gagal memperbarui ekstrakurikuler")
		return
	}
	common.WriteSuccess(w, map[string]string{"id": id, "status": "updated"}, nil)
}

func (h *Handler) DeleteExtracurricular(w http.ResponseWriter, r *http.Request) {
	claims := common.GetClaims(r.Context())
	id := chi.URLParam(r, "id")

	if err := h.Store.DeleteExtracurricular(r.Context(), claims.SchoolID, id); err != nil {
		common.WriteError(w, http.StatusInternalServerError, "Gagal menghapus ekstrakurikuler")
		return
	}
	common.WriteSuccess(w, map[string]string{"id": id, "status": "deleted"}, nil)
}

// Members
func (h *Handler) ListMembers(w http.ResponseWriter, r *http.Request) {
	claims := common.GetClaims(r.Context())
	eccID := chi.URLParam(r, "id")
	periodID := r.URL.Query().Get("period_id")

	members, err := h.Store.ListMembers(r.Context(), claims.SchoolID, eccID, periodID)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "Gagal memuat anggota ekskul")
		return
	}
	common.WriteSuccess(w, members, nil)
}

func (h *Handler) UpdateMemberStatus(w http.ResponseWriter, r *http.Request) {
	claims := common.GetClaims(r.Context())
	mid := chi.URLParam(r, "mid")

	var body struct {
		Status string `json:"status"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		common.WriteError(w, http.StatusBadRequest, "Payload tidak valid")
		return
	}

	err := h.Store.UpdateMemberStatus(r.Context(), claims.SchoolID, mid, body.Status, nil)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "Gagal memperbarui status anggota")
		return
	}
	common.WriteSuccess(w, map[string]string{"id": mid, "status": body.Status}, nil)
}

func (h *Handler) JoinEccSelf(w http.ResponseWriter, r *http.Request) {
	claims := common.GetClaims(r.Context())
	eccID := chi.URLParam(r, "id")

	var body struct {
		PeriodID string `json:"period_id"`
	}
	_ = json.NewDecoder(r.Body).Decode(&body)

	h.joinEccCore(w, r, eccID, claims.UserID, body.PeriodID)
}

func (h *Handler) JoinEccByParent(w http.ResponseWriter, r *http.Request) {
	claims := common.GetClaims(r.Context())
	eccID := chi.URLParam(r, "id")

	var body struct {
		StudentID string `json:"student_id"`
		PeriodID  string `json:"period_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || strings.TrimSpace(body.StudentID) == "" {
		common.WriteError(w, http.StatusBadRequest, "student_id wajib diisi")
		return
	}

	hasAccess, err := h.Client.GuardianHasAccess(r.Context(), claims.UserID, body.StudentID, claims.SchoolID)
	if err != nil || !hasAccess {
		common.WriteError(w, http.StatusForbidden, "Akses ditolak")
		return
	}

	h.joinEccCore(w, r, eccID, strings.TrimSpace(body.StudentID), body.PeriodID)
}

func (h *Handler) joinEccCore(w http.ResponseWriter, r *http.Request, eccID, studentID, periodID string) {
	claims := common.GetClaims(r.Context())

	ecc, err := h.Store.GetExtracurricular(r.Context(), claims.SchoolID, eccID)
	if err != nil {
		common.WriteError(w, http.StatusNotFound, "Ekstrakurikuler tidak ditemukan")
		return
	}

	// Verify period (FR-135-002)
	period, err := h.Store.GetPeriod(r.Context(), claims.SchoolID, periodID)
	if err != nil {
		common.WriteError(w, http.StatusBadRequest, "Periode pendaftaran tidak valid")
		return
	}
	now := time.Now()
	if !InPeriod(*period, now) {
		common.WriteError(w, http.StatusUnprocessableEntity, "Pendaftaran ekskul di luar periode aktif")
		return
	}

	// Check if already registered
	existing, _ := h.Store.GetMemberByStudent(r.Context(), claims.SchoolID, eccID, periodID, studentID)
	if existing != nil && existing.Status != EccMemberStatusLeft && existing.Status != EccMemberStatusRejected {
		common.WriteError(w, http.StatusBadRequest, "Siswa sudah terdaftar di ekskul ini")
		return
	}

	activeCount, _ := h.Store.CountActiveMembers(r.Context(), claims.SchoolID, eccID, periodID)
	joinDecision := EccJoinDecision(ecc.Quota, activeCount, ecc.FullPolicy)

	if joinDecision == EccMemberStatusRejected {
		// AC-135-01: quota full and policy is reject -> 422
		common.WriteError(w, http.StatusUnprocessableEntity, "Kuota ekstrakurikuler sudah penuh")
		return
	}

	memberID := store.NewID()
	var waitlistedAt, joinedAt *time.Time
	if joinDecision == EccMemberStatusWaitlisted {
		waitlistedAt = &now
	} else if joinDecision == EccMemberStatusActive {
		joinedAt = &now
	}

	var billID *string
	// If active and fee > 0, issue adhoc bill once per period (FR-135-002, AC-135-01)
	if joinDecision == EccMemberStatusActive && ecc.FeeIDR > 0 {
		bID, err := h.Client.CreateAdhocBill(r.Context(), client.AdhocBillRequest{
			SchoolID:      claims.SchoolID,
			StudentUserID: studentID,
			SourceModule:  "extracurricular",
			SourceRef:     memberID,
			Items: []client.AdhocBillItem{
				{Name: fmt.Sprintf("Iuran Ekskul: %s (%s)", ecc.Name, period.Name), AmountIDR: ecc.FeeIDR},
			},
			DueDate: period.ClosesAt.Format("2006-01-02"),
		}, fmt.Sprintf("extracurricular:%s", memberID))
		if err == nil && bID != "" {
			billID = &bID
		}
	}

	m, err := h.Store.CreateMember(r.Context(), EccMember{
		ID:            memberID,
		SchoolID:      claims.SchoolID,
		EccID:         eccID,
		PeriodID:      periodID,
		StudentUserID: studentID,
		Status:        joinDecision,
		WaitlistedAt:  waitlistedAt,
		JoinedAt:      joinedAt,
		BillID:        billID,
	})
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "Gagal mendaftar ekskul")
		return
	}

	common.WriteJSON(w, http.StatusCreated, m, nil)
}

func (h *Handler) LeaveEccSelf(w http.ResponseWriter, r *http.Request) {
	claims := common.GetClaims(r.Context())
	eccID := chi.URLParam(r, "id")

	members, err := h.Store.ListMyMemberships(r.Context(), claims.SchoolID, claims.UserID)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "Gagal memproses pengunduran diri")
		return
	}

	var target *EccMember
	for _, m := range members {
		if m.EccID == eccID && m.Status == EccMemberStatusActive {
			target = &m
			break
		}
	}
	if target == nil {
		common.WriteError(w, http.StatusNotFound, "Keanggotaan aktif tidak ditemukan")
		return
	}

	// Update left status
	_ = h.Store.UpdateMemberStatus(r.Context(), claims.SchoolID, target.ID, EccMemberStatusLeft, nil)

	// Promote waitlist if exists (AC-135-01)
	allMembers, _ := h.Store.ListMembers(r.Context(), claims.SchoolID, eccID, target.PeriodID)
	promotedID, _ := PromoteWaitlist(allMembers)
	if promotedID != nil {
		_ = h.Store.UpdateMemberStatus(r.Context(), claims.SchoolID, *promotedID, EccMemberStatusActive, nil)
		// Check fee for promoted member
		ecc, _ := h.Store.GetExtracurricular(r.Context(), claims.SchoolID, eccID)
		if ecc != nil && ecc.FeeIDR > 0 {
			promotedMem, _ := h.Store.GetMember(r.Context(), claims.SchoolID, *promotedID)
			if promotedMem != nil {
				_, _ = h.Client.CreateAdhocBill(r.Context(), client.AdhocBillRequest{
					SchoolID:      claims.SchoolID,
					StudentUserID: promotedMem.StudentUserID,
					SourceModule:  "extracurricular",
					SourceRef:     promotedMem.ID,
					Items: []client.AdhocBillItem{
						{Name: fmt.Sprintf("Iuran Ekskul: %s", ecc.Name), AmountIDR: ecc.FeeIDR},
					},
					DueDate: time.Now().AddDate(0, 0, 14).Format("2006-01-02"),
				}, fmt.Sprintf("extracurricular:%s", promotedMem.ID))
			}
		}
	}

	common.WriteSuccess(w, map[string]string{"id": target.ID, "status": EccMemberStatusLeft}, nil)
}

// Sessions & Attendance
func (h *Handler) ListSessions(w http.ResponseWriter, r *http.Request) {
	claims := common.GetClaims(r.Context())
	eccID := chi.URLParam(r, "id")

	sessions, err := h.Store.ListSessions(r.Context(), claims.SchoolID, eccID)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "Gagal memuat sesi ekskul")
		return
	}
	common.WriteSuccess(w, sessions, nil)
}

func (h *Handler) CreateSession(w http.ResponseWriter, r *http.Request) {
	claims := common.GetClaims(r.Context())
	eccID := chi.URLParam(r, "id")

	var in struct {
		HeldOn     string            `json:"held_on"`
		Note       *string           `json:"note"`
		Attendance []AttendanceInput `json:"attendance"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil || in.HeldOn == "" {
		common.WriteError(w, http.StatusBadRequest, "Tanggal sesi (held_on) wajib diisi")
		return
	}

	ses, err := h.Store.CreateSessionWithAttendance(r.Context(), EccSession{
		SchoolID:  claims.SchoolID,
		EccID:     eccID,
		HeldOn:    in.HeldOn,
		Note:      in.Note,
		CreatedBy: claims.UserID,
	}, in.Attendance)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "Gagal menyimpan sesi dan presensi")
		return
	}
	common.WriteJSON(w, http.StatusCreated, ses, nil)
}

// Assessments
func (h *Handler) ListAssessments(w http.ResponseWriter, r *http.Request) {
	claims := common.GetClaims(r.Context())
	eccID := chi.URLParam(r, "id")
	periodID := r.URL.Query().Get("period_id")

	assessments, err := h.Store.ListAssessments(r.Context(), claims.SchoolID, eccID, periodID)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "Gagal memuat penilaian")
		return
	}
	common.WriteSuccess(w, assessments, nil)
}

func (h *Handler) UpsertAssessment(w http.ResponseWriter, r *http.Request) {
	claims := common.GetClaims(r.Context())
	eccID := chi.URLParam(r, "id")

	var in struct {
		PeriodID    string  `json:"period_id"`
		MemberID    string  `json:"member_id"`
		Predicate   string  `json:"predicate"`
		Description *string `json:"description"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil || in.Predicate == "" || in.MemberID == "" {
		common.WriteError(w, http.StatusBadRequest, "Predikat dan ID anggota wajib diisi")
		return
	}

	res, err := h.Store.UpsertAssessment(r.Context(), EccAssessment{
		SchoolID:    claims.SchoolID,
		EccID:       eccID,
		PeriodID:    in.PeriodID,
		MemberID:    in.MemberID,
		Predicate:   strings.TrimSpace(in.Predicate),
		Description: in.Description,
		AssessedBy:  claims.UserID,
	})
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "Gagal menyimpan penilaian")
		return
	}
	common.WriteSuccess(w, res, nil)
}

// -------------------------------------------------------------
// 5. Achievements
// -------------------------------------------------------------

func (h *Handler) ListAchievements(w http.ResponseWriter, r *http.Request) {
	claims := common.GetClaims(r.Context())
	status := r.URL.Query().Get("status")
	studentID := r.URL.Query().Get("student_id")

	list, err := h.Store.ListAchievements(r.Context(), claims.SchoolID, status, studentID)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "Gagal memuat daftar prestasi")
		return
	}
	common.WriteSuccess(w, list, nil)
}

func (h *Handler) CreateAchievement(w http.ResponseWriter, r *http.Request) {
	claims := common.GetClaims(r.Context())

	// Supports both multipart/form-data with evidence file uploads and JSON
	if strings.Contains(r.Header.Get("Content-Type"), "multipart/form-data") {
		if err := r.ParseMultipartForm(15 << 20); err != nil {
			common.WriteError(w, http.StatusBadRequest, "Gagal membaca form data")
			return
		}

		studentID := r.FormValue("student_user_id")
		if studentID == "" {
			studentID = claims.UserID
		}
		activityName := r.FormValue("activity_name")
		level := r.FormValue("level")
		rankLabel := r.FormValue("rank_label")
		achievedOn := r.FormValue("achieved_on")

		if activityName == "" || level == "" || rankLabel == "" || achievedOn == "" {
			common.WriteError(w, http.StatusBadRequest, "Nama kegiatan, tingkat, peringkat, dan tanggal wajib diisi")
			return
		}

		var evidenceList []string
		files := r.MultipartForm.File["evidence"]
		for _, fh := range files {
			f, err := fh.Open()
			if err != nil {
				continue
			}
			defer f.Close()

			fname := fmt.Sprintf("%d_%s", time.Now().UnixNano(), fh.Filename)
			dstPath := filepath.Join(h.StoragePath, "achievements", fname)
			dst, err := os.Create(dstPath)
			if err == nil {
				_, _ = io.Copy(dst, f)
				dst.Close()
				evidenceList = append(evidenceList, "/data/storage/achievements/"+fname)
			}
		}

		evJSON, _ := json.Marshal(evidenceList)
		ach, err := h.Store.CreateAchievement(r.Context(), Achievement{
			SchoolID:      claims.SchoolID,
			StudentUserID: studentID,
			ActivityName:  activityName,
			Level:         level,
			RankLabel:     rankLabel,
			AchievedOn:    achievedOn,
			Evidence:      evJSON,
			SubmittedBy:   claims.UserID,
			SubmittedVia:  claims.ActiveRole,
			Status:        AchStatusPending,
		})
		if err != nil {
			common.WriteError(w, http.StatusInternalServerError, "Gagal menyimpan pengajuan prestasi")
			return
		}
		common.WriteJSON(w, http.StatusCreated, ach, nil)
		return
	}

	// JSON payload
	var in struct {
		StudentUserID string          `json:"student_user_id"`
		ActivityName  string          `json:"activity_name"`
		Organizer     *string         `json:"organizer"`
		Level         string          `json:"level"`
		RankLabel     string          `json:"rank_label"`
		AchievedOn    string          `json:"achieved_on"`
		Evidence      json.RawMessage `json:"evidence"`
		EventID       *string         `json:"event_id"`
		EccID         *string         `json:"ecc_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil || in.ActivityName == "" || in.Level == "" || in.RankLabel == "" {
		common.WriteError(w, http.StatusBadRequest, "Nama kegiatan, tingkat, dan peringkat wajib diisi")
		return
	}

	studentID := in.StudentUserID
	if studentID == "" {
		studentID = claims.UserID
	}

	ach, err := h.Store.CreateAchievement(r.Context(), Achievement{
		SchoolID:      claims.SchoolID,
		StudentUserID: studentID,
		ActivityName:  in.ActivityName,
		Organizer:     in.Organizer,
		Level:         in.Level,
		RankLabel:     in.RankLabel,
		AchievedOn:    in.AchievedOn,
		Evidence:      in.Evidence,
		EventID:       in.EventID,
		EccID:         in.EccID,
		SubmittedBy:   claims.UserID,
		SubmittedVia:  claims.ActiveRole,
		Status:        AchStatusPending,
	})
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "Gagal menyimpan prestasi")
		return
	}
	common.WriteJSON(w, http.StatusCreated, ach, nil)
}

func (h *Handler) VerifyAchievement(w http.ResponseWriter, r *http.Request) {
	claims := common.GetClaims(r.Context())
	id := chi.URLParam(r, "id")

	ach, err := h.Store.GetAchievement(r.Context(), claims.SchoolID, id)
	if err != nil {
		common.WriteError(w, http.StatusNotFound, "Prestasi tidak ditemukan")
		return
	}

	// Enforce maker-checker rule (BR-GEN-09)
	if !CanVerify(*ach, claims.UserID) {
		common.WriteError(w, http.StatusForbidden, "Pengaju tidak dapat memverifikasi prestasinya sendiri")
		return
	}

	if err := h.Store.VerifyAchievement(r.Context(), claims.SchoolID, id, claims.UserID, AchStatusVerified, nil); err != nil {
		common.WriteError(w, http.StatusInternalServerError, "Gagal memverifikasi prestasi")
		return
	}

	common.WriteSuccess(w, map[string]string{"id": id, "status": AchStatusVerified}, nil)
}

func (h *Handler) RejectAchievement(w http.ResponseWriter, r *http.Request) {
	claims := common.GetClaims(r.Context())
	id := chi.URLParam(r, "id")

	var body struct {
		Reason string `json:"reason"`
	}
	_ = json.NewDecoder(r.Body).Decode(&body)

	ach, err := h.Store.GetAchievement(r.Context(), claims.SchoolID, id)
	if err != nil {
		common.WriteError(w, http.StatusNotFound, "Prestasi tidak ditemukan")
		return
	}

	if !CanVerify(*ach, claims.UserID) {
		common.WriteError(w, http.StatusForbidden, "Pengaju tidak dapat menolak prestasinya sendiri")
		return
	}

	reason := body.Reason
	if err := h.Store.VerifyAchievement(r.Context(), claims.SchoolID, id, claims.UserID, AchStatusRejected, &reason); err != nil {
		common.WriteError(w, http.StatusInternalServerError, "Gagal menolak prestasi")
		return
	}

	common.WriteSuccess(w, map[string]string{"id": id, "status": AchStatusRejected}, nil)
}

// Summary endpoint: only returns verified achievements (AC-136-01, FR-136-003)
func (h *Handler) AchievementSummary(w http.ResponseWriter, r *http.Request) {
	claims := common.GetClaims(r.Context())
	from := r.URL.Query().Get("from")
	to := r.URL.Query().Get("to")

	verified, err := h.Store.ListAllVerifiedAchievements(r.Context(), claims.SchoolID, from, to)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "Gagal memuat rekap prestasi")
		return
	}

	res := AchievementSummary(verified)
	common.WriteSuccess(w, res, nil)
}

// Internal summary endpoint for reporting and BI analytics (T24)
func (h *Handler) InternalAchievementSummary(w http.ResponseWriter, r *http.Request) {
	schoolID := r.URL.Query().Get("school_id")
	from := r.URL.Query().Get("from")
	to := r.URL.Query().Get("to")

	verified, err := h.Store.ListAllVerifiedAchievements(r.Context(), schoolID, from, to)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "Gagal memuat rekap internal prestasi")
		return
	}

	res := AchievementSummary(verified)
	common.WriteSuccess(w, res, nil)
}
