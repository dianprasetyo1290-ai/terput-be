package counseling

import (
	"testing"
	"time"
)

func TestHomeroomStatus_AC13001(t *testing.T) {
	// Case with disclose_to_homeroom = true
	disclosed := []Case{
		{Status: CaseStatusOpen, DiscloseToHomeroom: true},
	}
	if got := HomeroomStatus(disclosed); got != HomeroomStatusInCare {
		t.Errorf("expected %s, got %s", HomeroomStatusInCare, got)
	}

	// Case with disclose_to_homeroom = false
	undisclosed := []Case{
		{Status: CaseStatusOpen, DiscloseToHomeroom: false},
	}
	if got := HomeroomStatus(undisclosed); got != HomeroomStatusNoInfo {
		t.Errorf("expected %s, got %s", HomeroomStatusNoInfo, got)
	}

	// Closed case with disclose_to_homeroom = true -> still tidak_ada_informasi
	closed := []Case{
		{Status: CaseStatusClosed, DiscloseToHomeroom: true},
	}
	if got := HomeroomStatus(closed); got != HomeroomStatusNoInfo {
		t.Errorf("expected %s, got %s", HomeroomStatusNoInfo, got)
	}

	// Empty cases
	if got := HomeroomStatus(nil); got != HomeroomStatusNoInfo {
		t.Errorf("expected %s, got %s", HomeroomStatusNoInfo, got)
	}
}

func TestValidateSession_FR13001(t *testing.T) {
	now := time.Now()

	// Group with 1 student -> error
	if err := ValidateSession(SessionKindGroup, []string{"s1"}, SessionStatusScheduled, nil); err == nil {
		t.Errorf("expected error for group session with 1 student, got nil")
	}

	// Group with 2 students -> ok
	if err := ValidateSession(SessionKindGroup, []string{"s1", "s2"}, SessionStatusScheduled, nil); err != nil {
		t.Errorf("unexpected error for group session: %v", err)
	}

	// Individual with 2 students -> error
	if err := ValidateSession(SessionKindIndividual, []string{"s1", "s2"}, SessionStatusScheduled, nil); err == nil {
		t.Errorf("expected error for individual session with 2 students, got nil")
	}

	// Individual with 1 student -> ok
	if err := ValidateSession(SessionKindIndividual, []string{"s1"}, SessionStatusScheduled, nil); err != nil {
		t.Errorf("unexpected error for individual session: %v", err)
	}

	// Status held without heldAt -> error
	if err := ValidateSession(SessionKindIndividual, []string{"s1"}, SessionStatusHeld, nil); err == nil {
		t.Errorf("expected error for held session without heldAt, got nil")
	}

	// Status held with heldAt -> ok
	if err := ValidateSession(SessionKindIndividual, []string{"s1"}, SessionStatusHeld, &now); err != nil {
		t.Errorf("unexpected error for held session with heldAt: %v", err)
	}
}

func TestReminderDue_FR13003(t *testing.T) {
	loc := time.FixedZone("WIB", 7*3600)
	sessionDate := time.Date(2026, 10, 15, 10, 0, 0, 0, loc)

	// reminder_days_before = 0 (Hari H)
	todayH := time.Date(2026, 10, 15, 6, 0, 0, 0, loc)
	if !ReminderDue(sessionDate, 0, nil, SessionStatusScheduled, todayH, loc) {
		t.Errorf("expected ReminderDue to be true on hari H")
	}

	todayHMinus1 := time.Date(2026, 10, 14, 6, 0, 0, 0, loc)
	if ReminderDue(sessionDate, 0, nil, SessionStatusScheduled, todayHMinus1, loc) {
		t.Errorf("expected ReminderDue to be false on H-1 when setting is 0")
	}

	// reminder_days_before = 1 (H-1)
	if !ReminderDue(sessionDate, 1, nil, SessionStatusScheduled, todayHMinus1, loc) {
		t.Errorf("expected ReminderDue to be true on H-1 when setting is 1")
	}

	// Already sent -> false
	now := time.Now()
	if ReminderDue(sessionDate, 0, &now, SessionStatusScheduled, todayH, loc) {
		t.Errorf("expected ReminderDue to be false if already sent")
	}

	// Not scheduled status -> false
	if ReminderDue(sessionDate, 0, nil, SessionStatusHeld, todayH, loc) {
		t.Errorf("expected ReminderDue to be false if status != scheduled")
	}
}

func TestReferralInitialStatus_AC13201(t *testing.T) {
	if got := ReferralInitialStatus("granted"); got != ReferralStatusActive {
		t.Errorf("expected %s, got %s", ReferralStatusActive, got)
	}
	if got := ReferralInitialStatus("declined"); got != ReferralStatusDeclined {
		t.Errorf("expected %s, got %s", ReferralStatusDeclined, got)
	}
	if got := ReferralInitialStatus("pending"); got != ReferralStatusAwaitingConsent {
		t.Errorf("expected %s, got %s", ReferralStatusAwaitingConsent, got)
	}
	if got := ReferralInitialStatus(""); got != ReferralStatusAwaitingConsent {
		t.Errorf("expected %s, got %s", ReferralStatusAwaitingConsent, got)
	}
}

func TestCanClose_FR13203(t *testing.T) {
	now := time.Now()
	pendingTasks := []Task{
		{ID: "t1", DoneAt: nil},
	}
	completedTasks := []Task{
		{ID: "t1", DoneAt: &now},
	}

	// Pending tasks without reason -> cannot close
	if CanClose(pendingTasks, "") {
		t.Errorf("expected CanClose to be false with pending tasks and empty reason")
	}

	// Pending tasks with reason -> can close
	if !CanClose(pendingTasks, "Siswa telah pindah sekolah") {
		t.Errorf("expected CanClose to be true with pending tasks when reason provided")
	}

	// Completed tasks without reason -> can close
	if !CanClose(completedTasks, "") {
		t.Errorf("expected CanClose to be true when all tasks done")
	}
}

func TestTaskDisplayStatus_FR13202(t *testing.T) {
	now := time.Now()
	today := "2026-10-15"

	doneTask := Task{DoneAt: &now, DueDate: "2026-10-10"}
	if got := TaskDisplayStatus(doneTask, today); got != TaskStatusDone {
		t.Errorf("expected %s, got %s", TaskStatusDone, got)
	}

	lateTask := Task{DoneAt: nil, DueDate: "2026-10-14"}
	if got := TaskDisplayStatus(lateTask, today); got != TaskStatusLate {
		t.Errorf("expected %s, got %s", TaskStatusLate, got)
	}

	pendingTask := Task{DoneAt: nil, DueDate: "2026-10-16"}
	if got := TaskDisplayStatus(pendingTask, today); got != TaskStatusPending {
		t.Errorf("expected %s, got %s", TaskStatusPending, got)
	}
}
