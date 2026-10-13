package discipline

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"skci/studentlife/internal/client"
	"skci/studentlife/internal/common"
	"skci/studentlife/internal/config"
	"skci/studentlife/internal/store"
)

type Handler struct {
	DB     *sql.DB
	Store  *Store
	Client *client.Client
	Cfg    config.Config
}

func NewHandler(db *sql.DB, cl *client.Client, cfg config.Config) *Handler {
	return &Handler{
		DB:     db,
		Store:  NewStore(db),
		Client: cl,
		Cfg:    cfg,
	}
}

// Types
func (h *Handler) ListTypes(w http.ResponseWriter, r *http.Request) {
	claims := common.GetClaims(r.Context())
	list, err := h.Store.ListTypes(r.Context(), claims.SchoolID)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "Gagal memuat jenis kedisiplinan")
		return
	}
	common.WriteSuccess(w, list, nil)
}

func (h *Handler) CreateType(w http.ResponseWriter, r *http.Request) {
	claims := common.GetClaims(r.Context())
	if claims.ActiveRole != common.RoleKesiswaan && claims.ActiveRole != common.RoleAdmin && claims.ActiveRole != common.RoleSuperadmin {
		common.WriteError(w, http.StatusForbidden, "Akses ditolak")
		return
	}

	var req struct {
		Code   string `json:"code"`
		Name   string `json:"name"`
		Kind   string `json:"kind"`
		Points int    `json:"points"`
		Active *bool  `json:"active"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Code == "" || req.Name == "" || req.Points <= 0 {
		common.WriteError(w, http.StatusBadRequest, "Kode, nama, dan poin (>0) wajib diisi")
		return
	}
	if req.Kind != TypeKindViolation && req.Kind != TypeKindMerit {
		req.Kind = TypeKindViolation
	}
	active := true
	if req.Active != nil {
		active = *req.Active
	}

	created, err := h.Store.CreateType(r.Context(), Type{
		SchoolID: claims.SchoolID,
		Code:     strings.TrimSpace(req.Code),
		Name:     strings.TrimSpace(req.Name),
		Kind:     req.Kind,
		Points:   req.Points,
		Active:   active,
	})
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "Gagal membuat jenis kedisiplinan")
		return
	}
	common.WriteSuccess(w, created, nil)
}

func (h *Handler) UpdateType(w http.ResponseWriter, r *http.Request) {
	claims := common.GetClaims(r.Context())
	id := chi.URLParam(r, "id")

	var req struct {
		Name   string `json:"name"`
		Points int    `json:"points"`
		Active bool   `json:"active"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Name == "" || req.Points <= 0 {
		common.WriteError(w, http.StatusBadRequest, "Nama dan poin (>0) wajib diisi")
		return
	}

	updated, err := h.Store.UpdateType(r.Context(), Type{
		ID:       id,
		SchoolID: claims.SchoolID,
		Name:     strings.TrimSpace(req.Name),
		Points:   req.Points,
		Active:   req.Active,
	})
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "Gagal memperbarui jenis kedisiplinan")
		return
	}
	common.WriteSuccess(w, updated, nil)
}

func (h *Handler) DeleteType(w http.ResponseWriter, r *http.Request) {
	claims := common.GetClaims(r.Context())
	id := chi.URLParam(r, "id")
	if err := h.Store.DeleteType(r.Context(), claims.SchoolID, id); err != nil {
		common.WriteError(w, http.StatusInternalServerError, "Gagal menghapus jenis kedisiplinan")
		return
	}
	common.WriteSuccess(w, map[string]string{"status": "deleted"}, nil)
}

// Thresholds
func (h *Handler) ListThresholds(w http.ResponseWriter, r *http.Request) {
	claims := common.GetClaims(r.Context())
	list, err := h.Store.ListThresholds(r.Context(), claims.SchoolID)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "Gagal memuat ambang poin")
		return
	}
	common.WriteSuccess(w, list, nil)
}

func (h *Handler) CreateThreshold(w http.ResponseWriter, r *http.Request) {
	claims := common.GetClaims(r.Context())
	if claims.ActiveRole != common.RoleKesiswaan && claims.ActiveRole != common.RoleAdmin && claims.ActiveRole != common.RoleSuperadmin {
		common.WriteError(w, http.StatusForbidden, "Akses ditolak")
		return
	}

	var req struct {
		Points        int     `json:"points"`
		ActionName    string  `json:"action_name"`
		NotifyParent  *bool   `json:"notify_parent"`
		EffectiveFrom string  `json:"effective_from"`
		EffectiveTo   *string `json:"effective_to"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Points <= 0 || req.ActionName == "" || req.EffectiveFrom == "" {
		common.WriteError(w, http.StatusBadRequest, "Poin (>0), tindakan, dan tanggal mulai berlaku wajib diisi")
		return
	}
	notify := true
	if req.NotifyParent != nil {
		notify = *req.NotifyParent
	}

	created, err := h.Store.CreateThreshold(r.Context(), Threshold{
		SchoolID:      claims.SchoolID,
		Points:        req.Points,
		ActionName:    strings.TrimSpace(req.ActionName),
		NotifyParent:  notify,
		EffectiveFrom: req.EffectiveFrom,
		EffectiveTo:   req.EffectiveTo,
	})
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "Gagal membuat ambang poin")
		return
	}
	common.WriteSuccess(w, created, nil)
}

func (h *Handler) UpdateThreshold(w http.ResponseWriter, r *http.Request) {
	claims := common.GetClaims(r.Context())
	id := chi.URLParam(r, "id")

	var req struct {
		Points        int     `json:"points"`
		ActionName    string  `json:"action_name"`
		NotifyParent  bool    `json:"notify_parent"`
		EffectiveFrom string  `json:"effective_from"`
		EffectiveTo   *string `json:"effective_to"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Points <= 0 || req.ActionName == "" || req.EffectiveFrom == "" {
		common.WriteError(w, http.StatusBadRequest, "Data tidak valid")
		return
	}

	updated, err := h.Store.UpdateThreshold(r.Context(), Threshold{
		ID:            id,
		SchoolID:      claims.SchoolID,
		Points:        req.Points,
		ActionName:    strings.TrimSpace(req.ActionName),
		NotifyParent:  req.NotifyParent,
		EffectiveFrom: req.EffectiveFrom,
		EffectiveTo:   req.EffectiveTo,
	})
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "Gagal memperbarui ambang poin")
		return
	}
	common.WriteSuccess(w, updated, nil)
}

func (h *Handler) DeleteThreshold(w http.ResponseWriter, r *http.Request) {
	claims := common.GetClaims(r.Context())
	id := chi.URLParam(r, "id")
	if err := h.Store.DeleteThreshold(r.Context(), claims.SchoolID, id); err != nil {
		common.WriteError(w, http.StatusInternalServerError, "Gagal menghapus ambang poin")
		return
	}
	common.WriteSuccess(w, map[string]string{"status": "deleted"}, nil)
}

// Settings
func (h *Handler) GetSettings(w http.ResponseWriter, r *http.Request) {
	claims := common.GetClaims(r.Context())
	st, err := h.Store.GetSettings(r.Context(), claims.SchoolID)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "Gagal memuat pengaturan kedisiplinan")
		return
	}
	common.WriteSuccess(w, st, nil)
}

func (h *Handler) PutSettings(w http.ResponseWriter, r *http.Request) {
	claims := common.GetClaims(r.Context())
	if claims.ActiveRole != common.RoleKesiswaan && claims.ActiveRole != common.RoleAdmin && claims.ActiveRole != common.RoleSuperadmin {
		common.WriteError(w, http.StatusForbidden, "Akses ditolak")
		return
	}

	var req struct {
		AccumulationScope string `json:"accumulation_scope"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		common.WriteError(w, http.StatusBadRequest, "Permintaan tidak valid")
		return
	}
	if req.AccumulationScope != ScopeAcademicYear && req.AccumulationScope != ScopeAllTime {
		req.AccumulationScope = ScopeAcademicYear
	}

	st, err := h.Store.PutSettings(r.Context(), Settings{
		SchoolID:          claims.SchoolID,
		AccumulationScope: req.AccumulationScope,
	})
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "Gagal menyimpan pengaturan")
		return
	}
	common.WriteSuccess(w, st, nil)
}

// Records (FR-131-002, AC-131-01)
func (h *Handler) ListRecords(w http.ResponseWriter, r *http.Request) {
	claims := common.GetClaims(r.Context())
	studentID := r.URL.Query().Get("student_id")
	kind := r.URL.Query().Get("kind")
	status := r.URL.Query().Get("status")

	list, err := h.Store.ListRecords(r.Context(), claims.SchoolID, studentID, kind, status)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "Gagal memuat catatan poin")
		return
	}

	// Mark can_appeal for guardians
	if claims.ActiveRole == common.RoleParent {
		for i := range list {
			cnt, _ := h.Store.PendingAppealCount(r.Context(), list[i].ID)
			list[i].CanAppeal = CanAppeal(list[i], cnt)
		}
	}

	common.WriteSuccess(w, list, nil)
}

func (h *Handler) CreateRecord(w http.ResponseWriter, r *http.Request) {
	claims := common.GetClaims(r.Context())
	if claims.ActiveRole != common.RoleKesiswaan && claims.ActiveRole != common.RoleWalikelas && claims.ActiveRole != common.RoleGuru && claims.ActiveRole != common.RoleAdmin && claims.ActiveRole != common.RoleSuperadmin {
		common.WriteError(w, http.StatusForbidden, "Akses ditolak")
		return
	}

	// Support multipart or JSON
	var studentID, typeID, occurredOn, description string
	var evidenceItems []EvidenceItem
	idempKey := r.Header.Get("Idempotency-Key")

	contentType := r.Header.Get("Content-Type")
	if strings.HasPrefix(contentType, "multipart/form-data") {
		_ = r.ParseMultipartForm(32 << 20)
		studentID = r.FormValue("student_id")
		typeID = r.FormValue("type_id")
		occurredOn = r.FormValue("occurred_on")
		description = r.FormValue("description")

		// Handle file uploads
		if r.MultipartForm != nil && r.MultipartForm.File != nil {
			files := r.MultipartForm.File["evidence"]
			for _, fh := range files {
				file, err := fh.Open()
				if err == nil {
					objKey := fmt.Sprintf("discipline/%s/%s-%s", claims.SchoolID, store.NewID(), filepath.Base(fh.Filename))
					storageDir := filepath.Join(h.Cfg.StoragePath, filepath.Dir(objKey))
					_ = os.MkdirAll(storageDir, 0755)
					dstPath := filepath.Join(h.Cfg.StoragePath, objKey)
					dst, err := os.Create(dstPath)
					if err == nil {
						_, _ = io.Copy(dst, file)
						dst.Close()
						evidenceItems = append(evidenceItems, EvidenceItem{
							ObjectKey: objKey,
							FileName:  fh.Filename,
							Mime:      fh.Header.Get("Content-Type"),
							Size:      fh.Size,
						})
					}
					file.Close()
				}
			}
		}
	} else {
		var req struct {
			StudentID   string         `json:"student_id"`
			TypeID      string         `json:"type_id"`
			OccurredOn  string         `json:"occurred_on"`
			Description string         `json:"description"`
			Evidence    []EvidenceItem `json:"evidence"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			common.WriteError(w, http.StatusBadRequest, "Permintaan tidak valid")
			return
		}
		studentID = req.StudentID
		typeID = req.TypeID
		occurredOn = req.OccurredOn
		description = req.Description
		evidenceItems = req.Evidence
	}

	if studentID == "" || typeID == "" || occurredOn == "" {
		common.WriteError(w, http.StatusBadRequest, "Siswa, jenis pelanggaran/prestasi, dan tanggal wajib diisi")
		return
	}

	// Look up type
	types, err := h.Store.ListTypes(r.Context(), claims.SchoolID)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "Gagal memverifikasi jenis poin")
		return
	}
	var selectedType *Type
	for _, t := range types {
		if t.ID == typeID {
			selectedType = &t
			break
		}
	}
	if selectedType == nil {
		common.WriteError(w, http.StatusBadRequest, "Jenis poin tidak ditemukan")
		return
	}

	// Fetch thresholds and settings
	thresholds, _ := h.Store.ListThresholds(r.Context(), claims.SchoolID)
	settings, _ := h.Store.GetSettings(r.Context(), claims.SchoolID)

	var idemp *string
	if strings.TrimSpace(idempKey) != "" {
		trimmed := strings.TrimSpace(idempKey)
		idemp = &trimmed
	}

	rec := Record{
		SchoolID:       claims.SchoolID,
		StudentID:      studentID,
		TypeID:         typeID,
		Kind:           selectedType.Kind,
		Points:         selectedType.Points,
		OccurredOn:     occurredOn,
		Description:    strings.TrimSpace(description),
		Evidence:       evidenceItems,
		ReporterUserID: claims.UserID,
		IdempotencyKey: idemp,
	}

	created, totalViolation, totalMerit, actions, err := h.Store.CreateRecordWithActions(r.Context(), rec, thresholds, settings.AccumulationScope, nil)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "Gagal mencatat poin kedisiplinan")
		return
	}

	// After-commit notifications to student's guardians (AC-131-01, FR-131-002)
	guardians, _ := h.Client.StudentGuardians(r.Context(), studentID, claims.SchoolID)
	for _, g := range guardians {
		// Notify record
		_ = h.Client.Notify(r.Context(), client.Notification{
			SchoolID: claims.SchoolID,
			UserID:   g.GuardianUserID,
			Type:     "discipline_record",
			Title:    fmt.Sprintf("Catatan %s: %s (%d Poin)", selectedType.Kind, selectedType.Name, selectedType.Points),
			Body:     fmt.Sprintf("Tercatat %s untuk Ananda: %s. Total poin pelanggaran saat ini: %d.", selectedType.Kind, description, totalViolation),
			Data:     map[string]any{"record_id": created.ID, "student_id": studentID, "points": selectedType.Points},
		}, fmt.Sprintf("studentlife:discipline:%s:%s", created.ID, g.GuardianUserID))

		// Notify newly triggered actions with notify_parent=true
		for _, act := range actions {
			_ = h.Client.Notify(r.Context(), client.Notification{
				SchoolID: claims.SchoolID,
				UserID:   g.GuardianUserID,
				Type:     "discipline_action",
				Title:    fmt.Sprintf("Tindakan Pembinaan: %s", act.ActionName),
				Body:     fmt.Sprintf("Akumulasi poin telah mencapai ambang pembinaan: %s. Pihak sekolah akan segera menindaklanjuti.", act.ActionName),
				Data:     map[string]any{"action_id": act.ID, "student_id": studentID, "action_name": act.ActionName},
			}, fmt.Sprintf("studentlife:discipline_action:%s:%s", act.ID, g.GuardianUserID))
		}
	}

	// Forward audit
	actorUser := claims.UserID
	actorRole := claims.ActiveRole
	_ = store.Audit(r.Context(), h.DB, store.AuditEntry{
		SchoolID:    claims.SchoolID,
		Module:      "discipline",
		EntityType:  "record",
		EntityID:    created.ID,
		Action:      "create",
		ActorUserID: &actorUser,
		ActorRole:   &actorRole,
		After:       map[string]any{"id": created.ID, "points": created.Points, "kind": created.Kind},
	})

	common.WriteSuccess(w, map[string]any{
		"record":                 created,
		"total_violation_points": totalViolation,
		"total_merit_points":     totalMerit,
		"triggered_actions":      actions,
	}, nil)
}

func (h *Handler) GetEvidenceFile(w http.ResponseWriter, r *http.Request) {
	claims := common.GetClaims(r.Context())
	recID := chi.URLParam(r, "id")
	indexStr := chi.URLParam(r, "n")

	rec, err := h.Store.GetRecord(r.Context(), claims.SchoolID, recID)
	if err != nil {
		common.WriteError(w, http.StatusNotFound, "Catatan tidak ditemukan")
		return
	}

	// Evidence privacy: not visible to parents (photos may leak other students)
	if claims.ActiveRole == common.RoleParent {
		common.WriteError(w, http.StatusForbidden, "Akses bukti dibatasi untuk privasi siswa")
		return
	}

	idx, err := strconv.Atoi(indexStr)
	if err != nil || idx < 0 || idx >= len(rec.Evidence) {
		common.WriteError(w, http.StatusNotFound, "Berkas bukti tidak ditemukan")
		return
	}

	item := rec.Evidence[idx]
	filePath := filepath.Join(h.Cfg.StoragePath, item.ObjectKey)
	http.ServeFile(w, r, filePath)
}

// Student Summary
func (h *Handler) GetStudentSummary(w http.ResponseWriter, r *http.Request) {
	claims := common.GetClaims(r.Context())
	studentID := chi.URLParam(r, "id")

	// Parent access check
	if claims.ActiveRole == common.RoleParent {
		ok, err := h.Client.GuardianHasAccess(r.Context(), claims.UserID, studentID, claims.SchoolID)
		if err != nil || !ok {
			common.WriteError(w, http.StatusForbidden, "Akses ditolak")
			return
		}
	}

	settings, _ := h.Store.GetSettings(r.Context(), claims.SchoolID)
	summary, err := h.Store.GetStudentSummary(r.Context(), claims.SchoolID, studentID, settings.AccumulationScope, nil)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "Gagal memuat ringkasan poin")
		return
	}
	common.WriteSuccess(w, summary, nil)
}

// Actions (FR-131-003)
func (h *Handler) ListActions(w http.ResponseWriter, r *http.Request) {
	claims := common.GetClaims(r.Context())
	status := r.URL.Query().Get("status")
	studentID := r.URL.Query().Get("student_id")

	list, err := h.Store.ListActions(r.Context(), claims.SchoolID, status, studentID)
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "Gagal memuat tindakan pembinaan")
		return
	}
	common.WriteSuccess(w, list, nil)
}

func (h *Handler) UpdateAction(w http.ResponseWriter, r *http.Request) {
	claims := common.GetClaims(r.Context())
	if claims.ActiveRole != common.RoleKesiswaan && claims.ActiveRole != common.RoleAdmin && claims.ActiveRole != common.RoleSuperadmin {
		common.WriteError(w, http.StatusForbidden, "Akses ditolak")
		return
	}
	id := chi.URLParam(r, "id")

	var req struct {
		Status         string  `json:"status"`
		AssigneeUserID *string `json:"assignee_user_id"`
		Notes          *string `json:"notes"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		common.WriteError(w, http.StatusBadRequest, "Permintaan tidak valid")
		return
	}

	var doneAt *time.Time
	if req.Status == ActionStatusDone {
		now := time.Now()
		doneAt = &now
	}

	if err := h.Store.UpdateAction(r.Context(), claims.SchoolID, id, req.Status, req.AssigneeUserID, req.Notes, doneAt); err != nil {
		common.WriteError(w, http.StatusInternalServerError, "Gagal memperbarui tindakan pembinaan")
		return
	}
	common.WriteSuccess(w, map[string]string{"status": "updated"}, nil)
}

// Appeals (FR-131-004)
func (h *Handler) CreateAppeal(w http.ResponseWriter, r *http.Request) {
	claims := common.GetClaims(r.Context())
	recID := chi.URLParam(r, "id")

	rec, err := h.Store.GetRecord(r.Context(), claims.SchoolID, recID)
	if err != nil {
		common.WriteError(w, http.StatusNotFound, "Catatan poin tidak ditemukan")
		return
	}

	// Verify guardian access
	ok, err := h.Client.GuardianHasAccess(r.Context(), claims.UserID, rec.StudentID, claims.SchoolID)
	if err != nil || !ok {
		common.WriteError(w, http.StatusForbidden, "Hanya orang tua/wali siswa yang dapat mengajukan keberatan")
		return
	}

	// Check can appeal
	pendingCount, _ := h.Store.PendingAppealCount(r.Context(), recID)
	if !CanAppeal(*rec, pendingCount) {
		common.WriteError(w, http.StatusUnprocessableEntity, "Catatan tidak dalam status aktif atau telah memiliki keberatan yang sedang diproses")
		return
	}

	var req struct {
		Reason string `json:"reason"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || strings.TrimSpace(req.Reason) == "" {
		common.WriteError(w, http.StatusBadRequest, "Alasan keberatan wajib diisi")
		return
	}

	appealID := store.NewID()
	idempKey := r.Header.Get("Idempotency-Key")
	var idemp *string
	if strings.TrimSpace(idempKey) != "" {
		trimmed := strings.TrimSpace(idempKey)
		idemp = &trimmed
	}

	// Trigger Core Approval: KnownRequestTypes 'discipline_appeal'
	approvalReq := client.ApprovalRequest{
		SchoolID:       claims.SchoolID,
		RequestType:    "discipline_appeal",
		SourceID:       appealID,
		MakerUserID:    claims.UserID,
		SubjectUserID:  rec.StudentID,
		IdempotencyKey: fmt.Sprintf("discipline_appeal:%s", appealID),
		Summary: map[string]any{
			"record_id":    rec.ID,
			"student_id":   rec.StudentID,
			"type_code":    rec.TypeCode,
			"type_name":    rec.TypeName,
			"points":       rec.Points,
			"reason":       req.Reason,
			"appeal_id":    appealID,
		},
	}
	approvalIDStr, err := h.Client.CreateApproval(r.Context(), approvalReq)
	var approvalID *string
	if err == nil && approvalIDStr != "" {
		approvalID = &approvalIDStr
	}

	appeal, err := h.Store.CreateAppeal(r.Context(), Appeal{
		ID:             appealID,
		SchoolID:       claims.SchoolID,
		RecordID:       recID,
		GuardianUserID: claims.UserID,
		Reason:         strings.TrimSpace(req.Reason),
		ApprovalID:     approvalID,
		IdempotencyKey: idemp,
	})
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "Gagal mengajukan keberatan")
		return
	}

	common.WriteSuccess(w, appeal, nil)
}

func (h *Handler) ListMyAppeals(w http.ResponseWriter, r *http.Request) {
	claims := common.GetClaims(r.Context())
	list, err := h.Store.ListAppeals(r.Context(), claims.SchoolID, claims.UserID, "")
	if err != nil {
		common.WriteError(w, http.StatusInternalServerError, "Gagal memuat daftar keberatan")
		return
	}
	common.WriteSuccess(w, list, nil)
}

// ApplyApprovalDecision processes core approval callbacks for discipline_appeal (FR-131-004)
func (h *Handler) ApplyApprovalDecision(ctx context.Context, tx *sql.Tx, sourceID, decision, decidedBy, note string) error {
	var appeal Appeal
	err := tx.QueryRowContext(ctx, `
		SELECT id, school_id, record_id, guardian_user_id, status
		FROM discipline_appeals
		WHERE id = $1`, sourceID).Scan(
		&appeal.ID, &appeal.SchoolID, &appeal.RecordID, &appeal.GuardianUserID, &appeal.Status,
	)
	if err != nil {
		return err
	}

	if appeal.Status != AppealStatusPending {
		// Idempotent callback
		return nil
	}

	if decision == "approved" {
		// Annul the record
		_, err = tx.ExecContext(ctx, `
			UPDATE discipline_records
			SET status = $1, annulled_reason = $2, updated_at = now()
			WHERE id = $3`,
			RecordStatusAnnulled, "Keberatan disetujui: "+note, appeal.RecordID)
		if err != nil {
			return err
		}

		// Update appeal status
		_, err = tx.ExecContext(ctx, `
			UPDATE discipline_appeals
			SET status = $1, updated_at = now()
			WHERE id = $2`, AppealStatusApproved, appeal.ID)
		if err != nil {
			return err
		}

		// Notify guardian
		_ = h.Client.Notify(ctx, client.Notification{
			SchoolID: appeal.SchoolID,
			UserID:   appeal.GuardianUserID,
			Type:     "discipline_appeal_result",
			Title:    "Keberatan Catatan Poin Disetujui",
			Body:     "Pengajuan keberatan Anda telah disetujui dan catatan poin telah dibatalkan.",
			Data:     map[string]any{"appeal_id": appeal.ID, "status": "approved"},
		}, fmt.Sprintf("studentlife:appeal:%s:approved", appeal.ID))
	} else {
		// Restore the record to active
		_, err = tx.ExecContext(ctx, `
			UPDATE discipline_records
			SET status = $1, updated_at = now()
			WHERE id = $2`, RecordStatusActive, appeal.RecordID)
		if err != nil {
			return err
		}

		_, err = tx.ExecContext(ctx, `
			UPDATE discipline_appeals
			SET status = $1, updated_at = now()
			WHERE id = $2`, AppealStatusRejected, appeal.ID)
		if err != nil {
			return err
		}

		_ = h.Client.Notify(ctx, client.Notification{
			SchoolID: appeal.SchoolID,
			UserID:   appeal.GuardianUserID,
			Type:     "discipline_appeal_result",
			Title:    "Keberatan Catatan Poin Ditolak",
			Body:     "Pengajuan keberatan Anda telah ditinjau dan ditolak. Catatan poin tetap berlaku.",
			Data:     map[string]any{"appeal_id": appeal.ID, "status": "rejected"},
		}, fmt.Sprintf("studentlife:appeal:%s:rejected", appeal.ID))
	}

	return nil
}
