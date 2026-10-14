package model

import "strings"

// StudentEnrollment is what academic-service says about one bill recipient.
type StudentEnrollment struct {
	Enrolled   bool   // active class enrollment in the active academic year
	GradeLevel string // grade level of that class
}

// RejectedRecipient is a student a generation run refused, with the reason
// shown to the operator.
type RejectedRecipient struct {
	StudentUserID string `json:"student_user_id"`
	Reason        string `json:"reason"`
}

// ScreenRecipients keeps only active students whose class grade matches the
// scheme's grade (when the scheme is tied to one). Users without an active
// enrollment (staff accounts, unplaced or inactive students) are rejected.
func ScreenRecipients(students []string, enrollments map[string]StudentEnrollment, schemeGrade string) ([]string, []RejectedRecipient) {
	schemeGrade = strings.TrimSpace(schemeGrade)
	var ok []string
	var rejected []RejectedRecipient
	for _, s := range students {
		e := enrollments[s]
		switch {
		case !e.Enrolled:
			rejected = append(rejected, RejectedRecipient{s, "bukan siswa aktif: tidak terdaftar di kelas aktif tahun ajaran berjalan"})
		case schemeGrade != "" && !strings.EqualFold(strings.TrimSpace(e.GradeLevel), schemeGrade):
			grade := e.GradeLevel
			if grade == "" {
				grade = "-"
			}
			rejected = append(rejected, RejectedRecipient{s, "tingkat kelas siswa (" + grade + ") tidak sesuai skema (" + schemeGrade + ")"})
		default:
			ok = append(ok, s)
		}
	}
	return ok, rejected
}

// DepositToApply is how much of a student's deposit balance settles a newly
// issued bill: everything available, capped at the bill's net amount.
func DepositToApply(balanceIDR, netIDR int64) int64 {
	if balanceIDR <= 0 || netIDR <= 0 {
		return 0
	}
	return min(balanceIDR, netIDR)
}

// SchemeIssueMapping checks that every scheme component can be journaled
// when the bill is issued (revenue and receivable accounts mapped). It uses
// the same rule as the bill-issued journal so issuance and posting agree.
func SchemeIssueMapping(set JournalSettings, comps map[string]ComponentAccount, items []BillItem) error {
	amounts := make([]ComponentAmount, 0, len(items))
	for _, it := range items {
		// A positive placeholder amount: only the account mapping matters here.
		amounts = append(amounts, ComponentAmount{Component: it.ComponentCode, Name: it.Name, AmountIDR: 1})
	}
	_, err := BillIssuedJournal(set, comps, amounts, 0)
	return err
}
