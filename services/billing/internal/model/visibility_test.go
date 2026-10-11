package model

import (
	"testing"
	"time"
)

func TestIsGradeBillVisible(t *testing.T) {
	enabledMap := map[string]bool{
		"10":  true,
		"11":  true,
		"12":  true,
		"SD":  false,
		"SMP": false,
		"SMK": true,
	}

	tests := []struct {
		name       string
		gradeLevel string
		m          map[string]bool
		want       bool
	}{
		{"nil map", "10", nil, false},
		{"empty grade", "", enabledMap, false},
		{"disabled SD (AC-82-01)", "SD", enabledMap, false},
		{"enabled SMK (AC-82-02)", "SMK", enabledMap, true},
		{"unconfigured grade defaults to false", "9", enabledMap, false},
		{"grade 10 enabled", "10", enabledMap, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := IsGradeBillVisible(tt.gradeLevel, tt.m)
			if got != tt.want {
				t.Errorf("IsGradeBillVisible(%q) = %v, want %v", tt.gradeLevel, got, tt.want)
			}
		})
	}
}

func TestCategorizeBillOutstanding(t *testing.T) {
	now := time.Date(2026, 7, 10, 12, 0, 0, 0, time.UTC)

	past := time.Date(2026, 7, 5, 0, 0, 0, 0, time.UTC)
	future := time.Date(2026, 7, 20, 0, 0, 0, 0, time.UTC)

	// Due in past -> open
	isOpen, isUnmatured := CategorizeBillOutstanding(&past, now)
	if !isOpen || isUnmatured {
		t.Errorf("past due date should be open, got isOpen=%v, isUnmatured=%v", isOpen, isUnmatured)
	}

	// Due in future -> unmatured
	isOpen, isUnmatured = CategorizeBillOutstanding(&future, now)
	if isOpen || !isUnmatured {
		t.Errorf("future due date should be unmatured, got isOpen=%v, isUnmatured=%v", isOpen, isUnmatured)
	}

	// Nil due date -> open
	isOpen, isUnmatured = CategorizeBillOutstanding(nil, now)
	if !isOpen || isUnmatured {
		t.Errorf("nil due date should be open, got isOpen=%v, isUnmatured=%v", isOpen, isUnmatured)
	}
}

func TestShouldVoidExitBill(t *testing.T) {
	effectiveDate := time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)

	before := time.Date(2026, 6, 25, 0, 0, 0, 0, time.UTC)
	after := time.Date(2026, 7, 10, 0, 0, 0, 0, time.UTC)

	// Policy keep -> never void
	if ShouldVoidExitBill(&after, effectiveDate, "keep") {
		t.Errorf("policy keep should not void bill")
	}

	// Policy void, due after -> void
	if !ShouldVoidExitBill(&after, effectiveDate, "void") {
		t.Errorf("policy void with bill due after effective date should void")
	}

	// Policy void, due before -> do not void
	if ShouldVoidExitBill(&before, effectiveDate, "void") {
		t.Errorf("policy void with bill due before effective date should not void")
	}
}
