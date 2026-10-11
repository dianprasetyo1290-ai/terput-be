package model

import "testing"

// TestValidParticipantType pins the closed two-value union. participant_type
// arrives from a token claim / request and ends up in a SQL predicate, so the
// only safe answers are the exact two literals — a prefix, a padded value, a
// differently-cased one, or anything longer than ParticipantTypeLen must be
// rejected rather than trimmed or prefix-matched into a real kind.
func TestValidParticipantType(t *testing.T) {
	valid := map[string]bool{
		ParticipantStudent:   true,
		ParticipantApplicant: true,
	}
	invalid := []string{
		"", " ", "Applicant", "APPLICANT", "applicant ", " applicant",
		"applicantt", "applicants", "students", "student1", "applic", "st",
		"applicant\x00", "applicant\n", "ânapplicant", "applicat", "app",
	}
	for v := range valid {
		if !ValidParticipantType(v) {
			t.Errorf("ValidParticipantType(%q) = false, want true", v)
		}
		if len(v) > ParticipantTypeLen {
			t.Errorf("%q is longer than the ParticipantTypeLen cap %d", v, ParticipantTypeLen)
		}
	}
	for _, v := range invalid {
		if ValidParticipantType(v) {
			t.Errorf("ValidParticipantType(%q) = true, want false", v)
		}
	}
	if ParticipantTypeLen != len(ParticipantApplicant) {
		t.Errorf("ParticipantTypeLen = %d, want the length of %q", ParticipantTypeLen, ParticipantApplicant)
	}
}
