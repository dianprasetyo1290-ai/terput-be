package handler

import (
	"context"
	"log"
	"time"

	"skci/attendance/internal/model"
)

// FR-41-004 / AC-41-01: when a student is recorded alpa (tanpa_keterangan)
// on any write path (guru marking, admin import, daily input, approved
// correction), each of their guardians gets one in-app notification —
// deduplicated per student per day via alpa_notifications, so re-saving the
// same day (or two paths writing alpa) never notifies twice.

type alpaClaimStore interface {
	ClaimAlpaNotification(ctx context.Context, schoolID, studentID, date string) (bool, error)
	ReleaseAlpaNotification(ctx context.Context, studentID, date string) error
}

type guardianNotifier interface {
	StudentGuardianIDs(ctx context.Context, studentID, schoolID string) ([]string, error)
	Notify(ctx context.Context, schoolID, userID, typ, title, body, idemKey string) error
}

type alpaEvent struct {
	SchoolID, StudentID, Date string
}

// alpaCollector gathers alpa writes during one request.
type alpaCollector []alpaEvent

func (c *alpaCollector) add(schoolID, studentID, date, status string) {
	if status == string(model.StatusTanpaKeterangan) {
		*c = append(*c, alpaEvent{SchoolID: schoolID, StudentID: studentID, Date: model.DateOnly(date)})
	}
}

// notifyAlpaAsync sends after the response, off the request's context —
// an import of a whole month can produce many events and the guardian
// lookups must not hold the HTTP request open.
func (h *Handler) notifyAlpaAsync(events []alpaEvent) {
	if len(events) == 0 || h.alpaClaims == nil || h.alpaNotify == nil {
		return
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		defer cancel()
		h.notifyAlpa(ctx, events)
	}()
}

// notifyAlpa is the synchronous core (tests call it directly). Returns how
// many (student, day) notifications were actually sent.
func (h *Handler) notifyAlpa(ctx context.Context, events []alpaEvent) int {
	if h.alpaClaims == nil || h.alpaNotify == nil {
		return 0
	}
	sent := 0
	seen := map[string]bool{}
	for _, ev := range events {
		key := ev.StudentID + "|" + ev.Date
		if seen[key] || ev.SchoolID == "" {
			continue
		}
		seen[key] = true
		claimed, err := h.alpaClaims.ClaimAlpaNotification(ctx, ev.SchoolID, ev.StudentID, ev.Date)
		if err != nil {
			log.Printf("alpa notification claim %s: %v", key, err)
			continue
		}
		if !claimed {
			continue // already notified for this student/day
		}
		guardians, err := h.alpaNotify.StudentGuardianIDs(ctx, ev.StudentID, ev.SchoolID)
		if err != nil {
			log.Printf("alpa notification guardians %s: %v", key, err)
			_ = h.alpaClaims.ReleaseAlpaNotification(ctx, ev.StudentID, ev.Date)
			continue
		}
		who := "Putra/putri Anda"
		if name := h.studentName(ctx, ev.StudentID); name != "" {
			who += " " + name
		}
		body := who + " tercatat alpa (tanpa keterangan) pada " + ev.Date +
			". Hubungi wali kelas bila ada kekeliruan atau ajukan izin melalui portal."
		failed := false
		for _, g := range guardians {
			if err := h.alpaNotify.Notify(ctx, ev.SchoolID, g, "attendance.alpa", "Ketidakhadiran tanpa keterangan",
				body, "alpa-"+ev.StudentID+"-"+ev.Date+"-"+g); err != nil {
				log.Printf("alpa notification %s -> %s: %v", key, g, err)
				failed = true
			}
		}
		if failed {
			// Idempotency keys make a later retry safe for guardians that
			// already got it.
			_ = h.alpaClaims.ReleaseAlpaNotification(ctx, ev.StudentID, ev.Date)
			continue
		}
		sent++
	}
	return sent
}

func (h *Handler) studentName(ctx context.Context, studentID string) string {
	if h.Client != nil {
		if u, err := h.Client.GetUser(ctx, studentID); err == nil && u != nil && u.Name != "" {
			return u.Name
		}
	}
	return ""
}
