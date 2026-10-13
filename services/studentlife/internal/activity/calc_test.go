package activity

import (
	"testing"
	"time"
)

func TestNeedsApproval_FR133003(t *testing.T) {
	// 1. Free event, no budget -> false
	e1 := Event{FeeIDR: 0}
	if NeedsApproval(e1) {
		t.Fatalf("expected NeedsApproval=false for free event without budget")
	}

	// 2. Fee > 0 -> true
	e2 := Event{FeeIDR: 50000}
	if !NeedsApproval(e2) {
		t.Fatalf("expected NeedsApproval=true for fee > 0")
	}

	// 3. Free event with budget line -> true
	budID := "bud-001"
	e3 := Event{FeeIDR: 0, BudgetLineID: &budID}
	if !NeedsApproval(e3) {
		t.Fatalf("expected NeedsApproval=true for budget-funded event")
	}
}

func TestApprovalFlags(t *testing.T) {
	budID := "bud-123"
	e := Event{FeeIDR: 10000, BudgetLineID: &budID}
	bRes := &BudgetCheckResult{AvailableIDR: 5000, OverBudget: true}
	flags := ApprovalFlags(e, bRes)

	if flags["has_fee"] != true || flags["uses_budget"] != true || flags["over_budget"] != true {
		t.Fatalf("expected all flags true, got: %+v", flags)
	}
}

func TestInTarget_FR134001(t *testing.T) {
	// Unrestricted
	eUnrestricted := Event{Target: EventTarget{}}
	if !InTarget(eUnrestricted, 10, "class-1") {
		t.Fatalf("expected unrestricted event to accept all students")
	}

	// Target grade 11 and 12
	eGrade := Event{Target: EventTarget{GradeLevels: []int{11, 12}}}
	if InTarget(eGrade, 10, "class-1") {
		t.Fatalf("grade 10 should not match target [11, 12]")
	}
	if !InTarget(eGrade, 11, "class-1") {
		t.Fatalf("grade 11 should match target [11, 12]")
	}

	// Target specific classroom
	eClass := Event{Target: EventTarget{ClassroomIDs: []string{"class-A"}}}
	if InTarget(eClass, 10, "class-B") {
		t.Fatalf("classroom B should not match class-A")
	}
	if !InTarget(eClass, 10, "class-A") {
		t.Fatalf("classroom A should match")
	}
}

func TestEventSeatDecision_Quota(t *testing.T) {
	// Unlimited
	allowed, _ := EventSeatDecision(nil, 500)
	if !allowed {
		t.Fatalf("nil quota should allow unlimited seats")
	}

	// Quota 30
	quota := 30
	// 29 taken (n-1) -> allowed
	allowed, _ = EventSeatDecision(&quota, 29)
	if !allowed {
		t.Fatalf("29/30 should be allowed")
	}

	// 30 taken (n) -> rejected
	allowed, reason := EventSeatDecision(&quota, 30)
	if allowed || reason != "Kuota penuh" {
		t.Fatalf("30/30 should be rejected with 'Kuota penuh', got allowed=%v, reason=%s", allowed, reason)
	}
}

func TestRegistrationStatus_BR_EVT_01(t *testing.T) {
	// Event does not require consent
	eNoConsent := Event{RequiresParentConsent: false}
	st := RegistrationStatus(eNoConsent, "siswa", false)
	if st != RegStatusConfirmed {
		t.Fatalf("expected confirmed for event without consent, got %s", st)
	}

	// Event requires consent, registered by student
	eConsent := Event{RequiresParentConsent: true}
	st = RegistrationStatus(eConsent, "siswa", false)
	if st != RegStatusPendingConsent {
		t.Fatalf("expected pending_consent for student registration, got %s", st)
	}

	// Registered by parent with consent
	st = RegistrationStatus(eConsent, "orang_tua", true)
	if st != RegStatusConfirmed {
		t.Fatalf("expected confirmed for parent registration with consent, got %s", st)
	}
}

func TestCanIssueEventBill_AC13401(t *testing.T) {
	event := Event{FeeIDR: 750000}

	// pending_consent -> false
	regPending := EventRegistration{Status: RegStatusPendingConsent}
	canIssue, amt := CanIssueEventBill(regPending, event)
	if canIssue || amt != 0 {
		t.Fatalf("pending_consent should not issue bill, got canIssue=%v, amt=%d", canIssue, amt)
	}

	// confirmed -> true, 750.000
	regConfirmed := EventRegistration{Status: RegStatusConfirmed}
	canIssue, amt = CanIssueEventBill(regConfirmed, event)
	if !canIssue || amt != 750000 {
		t.Fatalf("confirmed should issue bill 750000, got canIssue=%v, amt=%d", canIssue, amt)
	}
}

func TestCanMarkAttendance_AC13401_FR134004(t *testing.T) {
	loc, _ := time.LoadLocation("Asia/Jakarta")
	eventDay := time.Date(2026, 10, 15, 8, 0, 0, 0, loc)
	event := Event{StartsAt: eventDay}

	regPending := EventRegistration{Status: RegStatusPendingConsent}
	canMark, _ := CanMarkAttendance(regPending, event, eventDay)
	if canMark {
		t.Fatalf("pending_consent must not be marked present")
	}

	regConfirmed := EventRegistration{Status: RegStatusConfirmed}
	// Wrong day (yesterday)
	yesterday := eventDay.AddDate(0, 0, -1)
	canMark, reason := CanMarkAttendance(regConfirmed, event, yesterday)
	if canMark {
		t.Fatalf("must not mark attendance on wrong date, got allowed with reason: %s", reason)
	}

	// Correct day
	sameDayLater := eventDay.Add(5 * time.Hour)
	canMark, _ = CanMarkAttendance(regConfirmed, event, sameDayLater)
	if !canMark {
		t.Fatalf("must allow attendance on event date")
	}
}

func TestEccJoinDecision_AC13501(t *testing.T) {
	quota := 20

	// 19 active -> active
	st := EccJoinDecision(&quota, 19, EccFullPolicyReject)
	if st != EccMemberStatusActive {
		t.Fatalf("expected active, got %s", st)
	}

	// 20 active with policy reject -> rejected
	st = EccJoinDecision(&quota, 20, EccFullPolicyReject)
	if st != EccMemberStatusRejected {
		t.Fatalf("expected rejected for full quota with reject policy, got %s", st)
	}

	// 20 active with policy waitlist -> waitlisted
	st = EccJoinDecision(&quota, 20, EccFullPolicyWaitlist)
	if st != EccMemberStatusWaitlisted {
		t.Fatalf("expected waitlisted, got %s", st)
	}
}

func TestPromoteWaitlist(t *testing.T) {
	t1 := time.Date(2026, 8, 1, 10, 0, 0, 0, time.UTC)
	t2 := time.Date(2026, 8, 2, 10, 0, 0, 0, time.UTC)

	members := []EccMember{
		{ID: "m1", Status: EccMemberStatusActive},
		{ID: "m2", Status: EccMemberStatusWaitlisted, CreatedAt: t2},
		{ID: "m3", Status: EccMemberStatusWaitlisted, CreatedAt: t1}, // oldest waitlisted
	}

	promotedID, updated := PromoteWaitlist(members)
	if promotedID == nil || *promotedID != "m3" {
		t.Fatalf("expected m3 to be promoted, got %v", promotedID)
	}

	for _, m := range updated {
		if m.ID == "m3" && m.Status != EccMemberStatusActive {
			t.Fatalf("m3 should now have active status")
		}
	}
}

func TestInPeriod_FR135002(t *testing.T) {
	p := EccPeriod{
		OpensAt:  time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC),
		ClosesAt: time.Date(2026, 8, 15, 23, 59, 59, 0, time.UTC),
	}

	before := time.Date(2026, 7, 31, 0, 0, 0, 0, time.UTC)
	if InPeriod(p, before) {
		t.Fatalf("before opens_at should be false")
	}

	during := time.Date(2026, 8, 5, 0, 0, 0, 0, time.UTC)
	if !InPeriod(p, during) {
		t.Fatalf("during period should be true")
	}

	after := time.Date(2026, 8, 16, 0, 0, 0, 0, time.UTC)
	if InPeriod(p, after) {
		t.Fatalf("after closes_at should be false")
	}
}

func TestVisibleAchievement_AC13601(t *testing.T) {
	aPending := Achievement{Status: AchStatusPending}
	if VisibleAchievement(aPending) {
		t.Fatalf("pending achievement must not be visible")
	}

	aRejected := Achievement{Status: AchStatusRejected}
	if VisibleAchievement(aRejected) {
		t.Fatalf("rejected achievement must not be visible")
	}

	aVerified := Achievement{Status: AchStatusVerified}
	if !VisibleAchievement(aVerified) {
		t.Fatalf("verified achievement must be visible")
	}
}

func TestCanVerify_BR_GEN_09(t *testing.T) {
	a := Achievement{SubmittedBy: "user-100"}

	// Self-verify -> false
	if CanVerify(a, "user-100") {
		t.Fatalf("submitter cannot verify own achievement (maker-checker)")
	}

	// Other user -> true
	if !CanVerify(a, "user-200") {
		t.Fatalf("another user should be permitted to verify")
	}
}

func TestAchievementSummary_AC13601_FR136003(t *testing.T) {
	list := []Achievement{
		{ID: "a1", Status: AchStatusPending, Level: "nasional", AchievedOn: "2026-10-01"},
		{ID: "a2", Status: AchStatusVerified, Level: "nasional", AchievedOn: "2026-10-02", StudentName: "Siswa A"},
		{ID: "a3", Status: AchStatusVerified, Level: "provinsi", AchievedOn: "2026-09-15", StudentName: "Siswa B"},
		{ID: "a4", Status: AchStatusRejected, Level: "internasional", AchievedOn: "2026-10-05"},
	}

	res := AchievementSummary(list)

	// Only a2 and a3 should be counted
	if res.Total != 2 {
		t.Fatalf("expected 2 verified achievements, got %d", res.Total)
	}

	if res.PerLevel["nasional"] != 1 || res.PerLevel["provinsi"] != 1 || res.PerLevel["internasional"] != 0 {
		t.Fatalf("unexpected level counts: %+v", res.PerLevel)
	}

	if res.PerMonth["2026-10"] != 1 || res.PerMonth["2026-09"] != 1 {
		t.Fatalf("unexpected month counts: %+v", res.PerMonth)
	}

	if len(res.RecentVerified) != 2 {
		t.Fatalf("expected 2 recent verified, got %d", len(res.RecentVerified))
	}
	// a2 is more recent than a3
	if res.RecentVerified[0].ID != "a2" {
		t.Fatalf("expected a2 first (more recent date), got %s", res.RecentVerified[0].ID)
	}
}
