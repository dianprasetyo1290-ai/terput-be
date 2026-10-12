// Package consent holds generic digital parent consent (persetujuan orang
// tua) keyed by (subject_type, subject_id, student_id). Used by health
// programs (T10), counseling referrals (T20) and optionally events (T21).
package consent

import (
	"errors"
	"time"
)

// Status values (CHECK di migrasi 0001).
const (
	StatusPending  = "pending"
	StatusGranted  = "granted"
	StatusDeclined = "declined"
	StatusRevoked  = "revoked"
)

var (
	// ErrInvalidDecision: decision is not granted/declined/revoked.
	ErrInvalidDecision = errors.New("keputusan tidak valid")
	// ErrFinal: the consent was already decided (first guardian wins) or is
	// final; only the officer can re-create it.
	ErrFinal = errors.New("persetujuan sudah diputuskan")
	// ErrInUse: a granted consent can no longer be revoked once used.
	ErrInUse = errors.New("persetujuan sudah dipakai sehingga tidak dapat dicabut")
)

// Consent is one parent_consents row.
type Consent struct {
	ID                  string     `json:"id"`
	SchoolID            string     `json:"school_id"`
	SubjectType         string     `json:"subject_type"`
	SubjectID           string     `json:"subject_id"`
	StudentID           string     `json:"student_id"`
	Status              string     `json:"status"`
	DecidedByGuardianID *string    `json:"decided_by_guardian_id"`
	DecidedAt           *time.Time `json:"decided_at"`
	Note                *string    `json:"note"`
	RequestedBy         string     `json:"requested_by"`
	CreatedAt           time.Time  `json:"created_at"`
}

// Decide applies a guardian decision. Only pending consents can be granted
// or declined; a granted consent can be revoked while not yet used.
// declined/revoked are final.
func Decide(c Consent, decision, guardianID string, used bool, now time.Time) (Consent, error) {
	switch decision {
	case StatusGranted, StatusDeclined:
		if c.Status != StatusPending {
			return c, ErrFinal
		}
	case StatusRevoked:
		if c.Status != StatusGranted {
			return c, ErrFinal
		}
		if used {
			return c, ErrInUse
		}
	default:
		return c, ErrInvalidDecision
	}
	c.Status = decision
	c.DecidedByGuardianID = &guardianID
	c.DecidedAt = &now
	return c, nil
}
