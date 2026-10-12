package health

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"time"

	"skci/studentlife/internal/client"
	"skci/studentlife/internal/consent"
	"skci/studentlife/internal/store"
)

const jobBatch = 100

// JobSyncVisits retries pending attendance syncs and parent notifications.
func (h *Handler) JobSyncVisits(ctx context.Context, db *sql.DB) error {
	visits, err := PendingSyncVisits(ctx, db, jobBatch)
	if err != nil {
		return err
	}
	for _, v := range visits {
		h.ProcessVisitEffects(ctx, v.ID, v.SchoolID)
	}
	return nil
}

// JobInventoryIssues delivers pending medicine stock issues to ops (FR-128-003).
// OPS_URL empty or ops 404 (T15 belum ada) → stays pending; stock shortage
// (422) → failed; other errors retried until JOB_MAX_ATTEMPTS.
func (h *Handler) JobInventoryIssues(ctx context.Context, db *sql.DB) error {
	rows, err := PendingOutbox(ctx, db, jobBatch)
	if err != nil {
		return err
	}
	for _, o := range rows {
		err := h.Client.IssueInventory(ctx, client.InventoryIssue{SchoolID: o.SchoolID, ItemID: o.OpsItemID, Qty: o.Qty,
			RefType: "clinic_visit", RefID: o.VisitID, IdempotencyKey: "studentlife:med:" + o.VisitMedicationID})
		status := client.StatusOf(err)
		switch {
		case err == nil:
			_ = SetOutbox(ctx, db, o.ID, "sent", "", true)
		case errors.Is(err, client.ErrNotConfigured) || status == http.StatusNotFound:
			_ = SetOutbox(ctx, db, o.ID, "pending", "Inventory belum tersedia", false)
		case status == http.StatusUnprocessableEntity:
			_ = SetOutbox(ctx, db, o.ID, "failed", "Stok tidak mencukupi", true)
		case o.Attempts+1 >= h.Cfg.JobMaxAttempts:
			_ = SetOutbox(ctx, db, o.ID, "failed", "Pengiriman ke inventory gagal", true)
			_ = h.Client.ReportException(ctx, client.IntegrationException{SchoolID: o.SchoolID, SourceService: "studentlife",
				Kind: "callback", Severity: "low", CorrelationID: o.ID, ErrorMessage: "Pengeluaran stok obat UKS gagal dikirim",
				PayloadSummary: map[string]any{"outbox_id": o.ID, "target": "ops"}, MaxAttempts: h.Cfg.JobMaxAttempts,
				IdempotencyKey: "studentlife:med:" + o.VisitMedicationID})
		default:
			_ = SetOutbox(ctx, db, o.ID, "pending", "Pengiriman ke inventory belum berhasil", true)
		}
	}
	return nil
}

// JobConsentReminders reminds guardians who have not decided on the day of
// consent_deadline (school zone); the idempotency key carries the date so
// it is sent once.
func (h *Handler) JobConsentReminders(ctx context.Context, db *sql.DB) error {
	cands, deadlines, err := ConsentReminderCandidates(ctx, db, 500)
	if err != nil {
		return err
	}
	for _, cd := range cands {
		loc := store.SchoolLocation(ctx, db, cd.SchoolID, h.Cfg.DefaultTimezone)
		if deadlines[cd.ProgramID] != time.Now().In(loc).Format("2006-01-02") {
			continue
		}
		p, err := GetProgram(ctx, db, cd.SchoolID, cd.ProgramID)
		if err != nil {
			continue
		}
		cs := consent.Consent{ID: cd.ConsentID, StudentID: cd.StudentID}
		h.notifyConsent(ctx, cd.SchoolID, p, cs, "studentlife:consent:"+cd.ConsentID+":reminder:"+deadlines[cd.ProgramID],
			"Pengingat: persetujuan "+p.Name+" berakhir hari ini")
	}
	return nil
}
