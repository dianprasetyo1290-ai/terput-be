package model

import (
	"errors"
	"strings"
)

// Reconciliation exception lifecycle (menu 31): open -> pending_approval
// (resolution submitted to the Approval Center) -> resolved, or back to open
// when the checker rejects it.
const (
	ReconOpen            = "open"
	ReconPendingApproval = "pending_approval"
	ReconResolved        = "resolved"
)

// ValidateReconResolution checks a proposed resolution: a note is always
// required; adjustment lines are optional (an explained timing difference
// needs none) but, when given, must form a balanced journal. It returns the
// adjustment total (sum of debits).
func ValidateReconResolution(note string, lines []JournalLine) (int64, error) {
	if strings.TrimSpace(note) == "" {
		return 0, errors.New("catatan penyelesaian (resolution_note) wajib diisi")
	}
	if len(lines) == 0 {
		return 0, nil
	}
	if err := ValidateJournal(lines); err != nil {
		return 0, errors.New("jurnal penyesuaian tidak valid: " + err.Error())
	}
	var total int64
	for _, l := range lines {
		total += l.DebitIDR
	}
	return total, nil
}

// ReconDecision maps an Approval Center decision on a pending resolution to
// the exception's next status. changed is false for a replayed callback.
func ReconDecision(current, decision, decidedBy, requestedBy string) (next string, changed bool, err error) {
	if current != ReconPendingApproval {
		return current, false, nil
	}
	switch decision {
	case "approved":
		next = ReconResolved
	case "rejected":
		next = ReconOpen
	default:
		return current, false, errors.New("keputusan harus approved atau rejected")
	}
	if strings.TrimSpace(decidedBy) == "" {
		return current, false, errors.New("decided_by wajib diisi")
	}
	if decidedBy == requestedBy {
		return current, false, errors.New("penyetuju harus berbeda dari pengaju penyelesaian")
	}
	return next, true, nil
}
