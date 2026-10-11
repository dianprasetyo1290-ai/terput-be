package model

import "time"

// IsGradeBillVisible checks if bill visibility is enabled for the student's grade level.
// If gradeLevel is empty or not in the enabled map, defaults to false (FR-82-001 / AC-82-01).
func IsGradeBillVisible(gradeLevel string, enabledMap map[string]bool) bool {
	if enabledMap == nil || gradeLevel == "" {
		return false
	}
	return enabledMap[gradeLevel]
}

// CategorizeBillOutstanding categorizes an outstanding bill amount into open (due/overdue)
// or unmatured (due in the future) based on due date.
func CategorizeBillOutstanding(dueAt *time.Time, now time.Time) (isOpen bool, isUnmatured bool) {
	if dueAt == nil || !dueAt.After(now) {
		return true, false
	}
	return false, true
}

// ShouldVoidExitBill determines whether a bill should be voided when a student exits.
// Per OQ-07 and FR-78-005, if policy == 'void', bills due after the effective date are voided.
func ShouldVoidExitBill(dueAt *time.Time, effectiveDate time.Time, policy string) bool {
	if policy != "void" || dueAt == nil {
		return false
	}
	return dueAt.After(effectiveDate)
}
