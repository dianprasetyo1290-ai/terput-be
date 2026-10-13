package transport

import (
	"testing"
	"time"
)

func TestStudentsByStop_AC11901(t *testing.T) {
	today := time.Date(2026, 11, 10, 0, 0, 0, 0, time.UTC)
	yesterday := today.AddDate(0, 0, -1)

	stops := []Stop{
		{ID: "stop-1", Seq: 1, Name: "Perumahan A"},
		{ID: "stop-2", Seq: 2, Name: "Perumahan B"},
	}

	var allocations []Allocation
	// 6 active allocations on Stop 1
	for i := 1; i <= 6; i++ {
		allocations = append(allocations, Allocation{
			ID:            "alloc-active-" + string(rune('a'+i)),
			StudentUserID: "student-" + string(rune('a'+i)),
			StopID:        "stop-1",
			StartDate:     today.AddDate(0, -1, 0),
			EndDate:       nil,
		})
	}

	// 1 allocation that ended yesterday on Stop 1
	allocations = append(allocations, Allocation{
		ID:            "alloc-ended-yesterday",
		StudentUserID: "student-ended",
		StopID:        "stop-1",
		StartDate:     today.AddDate(0, -2, 0),
		EndDate:       &yesterday,
	})

	roster := StudentsByStop(stops, allocations, today)
	if len(roster) != 2 {
		t.Fatalf("expected 2 stops, got %d", len(roster))
	}
	if roster[0].Stop.Name != "Perumahan A" {
		t.Errorf("expected first stop Perumahan A, got %s", roster[0].Stop.Name)
	}
	if len(roster[0].Students) != 6 {
		t.Errorf("AC-119-01: expected 6 active students at Perumahan A, got %d", len(roster[0].Students))
	}
	if len(roster[1].Students) != 0 {
		t.Errorf("expected 0 students at Perumahan B, got %d", len(roster[1].Students))
	}
}

func TestCanAllocate_AC12101(t *testing.T) {
	// Capacity 15, active 15, add 1 -> error
	err := CanAllocate(15, 15, 1)
	if err == nil {
		t.Errorf("AC-121-01: expected capacity full error, got nil")
	}

	// Capacity 15, active 14, add 1 -> ok
	if err := CanAllocate(15, 14, 1); err != nil {
		t.Errorf("AC-121-01: expected allowed allocation, got error: %v", err)
	}

	// Capacity 15, active 14, add 2 -> error
	if err := CanAllocate(15, 14, 2); err == nil {
		t.Errorf("AC-121-01: expected error adding 2 to 14/15, got nil")
	}
}

func TestDocReminderDue_AC12001(t *testing.T) {
	validUntil := time.Date(2026, 11, 30, 0, 0, 0, 0, time.UTC)
	today31Oct := time.Date(2026, 10, 31, 0, 0, 0, 0, time.UTC)
	today29Oct := time.Date(2026, 10, 29, 0, 0, 0, 0, time.UTC)

	// AC-120-01: 30 Nov vs 31 Okt, days 30 -> due=true, daysLeft=30
	due, daysLeft := DocReminderDue(validUntil, today31Oct, 30)
	if !due || daysLeft != 30 {
		t.Errorf("AC-120-01: expected due=true, daysLeft=30; got due=%v, daysLeft=%d", due, daysLeft)
	}

	// 29 Okt -> due=false, daysLeft=32
	due29, daysLeft29 := DocReminderDue(validUntil, today29Oct, 30)
	if due29 || daysLeft29 != 32 {
		t.Errorf("AC-120-01: expected due=false, daysLeft=32; got due=%v, daysLeft=%d", due29, daysLeft29)
	}

	// Expired: 1 Dec vs 30 Nov
	today1Dec := time.Date(2026, 12, 1, 0, 0, 0, 0, time.UTC)
	dueExp, daysLeftExp := DocReminderDue(validUntil, today1Dec, 30)
	if !dueExp || daysLeftExp >= 0 {
		t.Errorf("expected due=true and negative daysLeft for expired doc; got due=%v, daysLeft=%d", dueExp, daysLeftExp)
	}
}

func TestProrateIDR_AC12201(t *testing.T) {
	// AC-122-01: 600.000, 15/30 -> 300.000
	p1 := ProrateIDR(600000, 15, 30)
	if p1 != 300000 {
		t.Errorf("AC-122-01: expected 300000, got %d", p1)
	}

	// 600.000, 30/30 -> 600.000
	p2 := ProrateIDR(600000, 30, 30)
	if p2 != 600000 {
		t.Errorf("expected 600000, got %d", p2)
	}

	// 0 days -> 0
	p3 := ProrateIDR(600000, 0, 30)
	if p3 != 0 {
		t.Errorf("expected 0, got %d", p3)
	}
}

func TestFareFor_Precedence(t *testing.T) {
	routeID := "route-1"
	zoneID := "zone-1"
	date := time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)

	fares := []Fare{
		{
			ID:               "f-zone",
			ZoneID:           &zoneID,
			MonthlyAmountIDR: 600000,
			EffectiveFrom:    time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		},
		{
			ID:               "f-route",
			RouteID:          &routeID,
			MonthlyAmountIDR: 650000,
			EffectiveFrom:    time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		},
	}

	// Route fare wins over zone fare
	amount, err := FareFor(fares, &routeID, &zoneID, date)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if amount != 650000 {
		t.Errorf("expected route fare 650000, got %d", amount)
	}

	// Zone fare when no route fare
	otherRoute := "route-2"
	zoneAmount, err := FareFor(fares, &otherRoute, &zoneID, date)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if zoneAmount != 600000 {
		t.Errorf("expected zone fare 600000, got %d", zoneAmount)
	}
}

func TestComputeCharge_DailyAndFullMonth(t *testing.T) {
	routeAID := "route-a"
	routeBID := "route-b"
	fares := []Fare{
		{RouteID: &routeAID, MonthlyAmountIDR: 600000, EffectiveFrom: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)},
		{RouteID: &routeBID, MonthlyAmountIDR: 800000, EffectiveFrom: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)},
	}

	// November 2026 has 30 days
	// Route A from Nov 1 to Nov 15 (15 days)
	// Route B from Nov 16 to Nov 30 (15 days)
	endA := time.Date(2026, 11, 15, 0, 0, 0, 0, time.UTC)
	startB := time.Date(2026, 11, 16, 0, 0, 0, 0, time.UTC)
	endB := time.Date(2026, 11, 30, 0, 0, 0, 0, time.UTC)

	segments := []AllocationSegment{
		{RouteID: routeAID, RouteName: "Rute A", StartDate: time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC), EndDate: &endA},
		{RouteID: routeBID, RouteName: "Rute B", StartDate: startB, EndDate: &endB},
	}

	// Daily policy -> 300.000 + 400.000 = 700.000
	items, total, err := ComputeCharge(segments, fares, "daily", 2026, 11)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("expected 2 items, got %d", len(items))
	}
	if total != 700000 {
		t.Errorf("expected total 700000, got %d", total)
	}
	if items[0].AmountIDR != 300000 || items[1].AmountIDR != 400000 {
		t.Errorf("expected items 300000 and 400000, got %d and %d", items[0].AmountIDR, items[1].AmountIDR)
	}

	// Full month policy -> tie break selects last segment (Route B: 800.000)
	itemsFM, totalFM, err := ComputeCharge(segments, fares, "full_month", 2026, 11)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(itemsFM) != 1 || totalFM != 800000 {
		t.Errorf("expected 1 item with 800000, got len %d, total %d", len(itemsFM), totalFM)
	}
}

func TestManifestAndEvents(t *testing.T) {
	roster := []Allocation{
		{StudentUserID: "student-1", StudentName: "Nara", StopID: "stop-1", StopName: "Halte Rumah"},
		{StudentUserID: "student-2", StudentName: "Budi", StopID: "stop-1", StopName: "Halte Rumah"},
	}

	notices := []AbsenceNotice{
		{StudentUserID: "student-2", Direction: "both", Reason: strPtr("Sakit demam")},
	}

	evTime := time.Now()
	events := []TripEvent{
		{StudentUserID: "student-1", Event: "board", RecordedAt: evTime, ClientEventID: "ev-1"},
	}

	m := Manifest(roster, events, notices, "jemput")
	if len(m) != 2 {
		t.Fatalf("expected 2 manifest entries, got %d", len(m))
	}
	if m[0].Status != "naik" {
		t.Errorf("expected student-1 status 'naik', got '%s'", m[0].Status)
	}
	if m[1].Status != "tidak_ikut" {
		t.Errorf("expected student-2 status 'tidak_ikut', got '%s'", m[1].Status)
	}
	if m[1].NoticeReason == nil || *m[1].NoticeReason != "Sakit demam" {
		t.Errorf("expected notice reason 'Sakit demam', got %v", m[1].NoticeReason)
	}

	// ValidateTripEvent
	errAlight := ValidateTripEvent(events, TripEvent{StudentUserID: "student-2", Event: "alight"})
	if errAlight == nil {
		t.Errorf("expected error when alighting without boarding, got nil")
	}

	errIdemp := ValidateTripEvent(events, TripEvent{StudentUserID: "student-1", Event: "board", ClientEventID: "ev-1"})
	if errIdemp != nil {
		t.Errorf("expected idempotent retry to succeed, got %v", errIdemp)
	}
}

func strPtr(s string) *string {
	return &s
}
