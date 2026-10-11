package model

import (
	"testing"
	"time"
)

func TestValidateCreateStatusRequest(t *testing.T) {
	tests := []struct {
		name            string
		kind            string
		reason          string
		fromClassroomID string
		toClassroomID   string
		toSchoolID      string
		schoolID        string
		effectiveDate   string
		wantErr         bool
		errMsg          string
	}{
		{
			name:            "valid class transfer",
			kind:            KindClassTransfer,
			reason:          "Pindah jurusan IPA ke IPS",
			fromClassroomID: "cls-1",
			toClassroomID:   "cls-2",
			schoolID:        "sch-1",
			effectiveDate:   "2026-07-01",
			wantErr:         false,
		},
		{
			name:            "class transfer same classroom",
			kind:            KindClassTransfer,
			reason:          "Pindah kelas",
			fromClassroomID: "cls-1",
			toClassroomID:   "cls-1",
			schoolID:        "sch-1",
			effectiveDate:   "2026-07-01",
			wantErr:         true,
			errMsg:          "kelas tujuan harus berbeda dari kelas asal",
		},
		{
			name:            "class transfer missing to classroom",
			kind:            KindClassTransfer,
			reason:          "Pindah kelas",
			fromClassroomID: "cls-1",
			toClassroomID:   "",
			schoolID:        "sch-1",
			effectiveDate:   "2026-07-01",
			wantErr:         true,
			errMsg:          "kelas tujuan wajib dipilih",
		},
		{
			name:            "valid unit transfer",
			kind:            KindUnitTransfer,
			reason:          "Pindah dari SMP ke SMK",
			fromClassroomID: "cls-1",
			toSchoolID:      "sch-smk",
			schoolID:        "sch-smp",
			effectiveDate:   "2026-07-01",
			wantErr:         false,
		},
		{
			name:            "unit transfer same school",
			kind:            KindUnitTransfer,
			reason:          "Pindah",
			fromClassroomID: "cls-1",
			toSchoolID:      "sch-1",
			schoolID:        "sch-1",
			effectiveDate:   "2026-07-01",
			wantErr:         true,
			errMsg:          "unit tujuan harus berbeda",
		},
		{
			name:            "valid withdrawal",
			kind:            KindWithdrawal,
			reason:          "Pindah domisili ke luar kota",
			fromClassroomID: "cls-1",
			schoolID:        "sch-1",
			effectiveDate:   "2026-08-01",
			wantErr:         false,
		},
		{
			name:            "withdrawal without active classroom",
			kind:            KindWithdrawal,
			reason:          "Pindah domisili",
			fromClassroomID: "",
			schoolID:        "sch-1",
			effectiveDate:   "2026-08-01",
			wantErr:         true,
			errMsg:          "siswa belum terdaftar di kelas aktif",
		},
		{
			name:            "empty reason",
			kind:            KindWithdrawal,
			reason:          "   ",
			fromClassroomID: "cls-1",
			schoolID:        "sch-1",
			effectiveDate:   "2026-08-01",
			wantErr:         true,
			errMsg:          "alasan pengajuan wajib diisi",
		},
		{
			name:            "invalid date format",
			kind:            KindWithdrawal,
			reason:          "Alasan",
			fromClassroomID: "cls-1",
			schoolID:        "sch-1",
			effectiveDate:   "01-08-2026",
			wantErr:         true,
			errMsg:          "format tanggal efektif harus YYYY-MM-DD",
		},
		{
			name:            "invalid kind",
			kind:            "unknown_kind",
			reason:          "Alasan",
			fromClassroomID: "cls-1",
			schoolID:        "sch-1",
			effectiveDate:   "2026-08-01",
			wantErr:         true,
			errMsg:          "jenis pengajuan tidak valid",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateCreateStatusRequest(tt.kind, tt.reason, tt.fromClassroomID, tt.toClassroomID, tt.toSchoolID, tt.schoolID, tt.effectiveDate)
			if (err != nil) != tt.wantErr {
				t.Fatalf("ValidateCreateStatusRequest() error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantErr && err != nil && tt.errMsg != "" {
				if !contains(err.Error(), tt.errMsg) {
					t.Errorf("expected error message to contain %q, got %q", tt.errMsg, err.Error())
				}
			}
		})
	}
}

func TestIsValidStatusTransition(t *testing.T) {
	tests := []struct {
		current string
		target  string
		want    bool
	}{
		{StatusPendingApproval, StatusApproved, true},
		{StatusPendingApproval, StatusRejected, true},
		{StatusPendingApproval, StatusCancelled, true},
		{StatusPendingApproval, StatusApplied, false},
		{StatusApproved, StatusApplied, true},
		{StatusApproved, StatusCancelled, false},
		{StatusApproved, StatusRejected, false},
		{StatusApplied, StatusCancelled, false},
		{StatusRejected, StatusApproved, false},
	}

	for _, tt := range tests {
		t.Run(tt.current+"->"+tt.target, func(t *testing.T) {
			got := IsValidStatusTransition(tt.current, tt.target)
			if got != tt.want {
				t.Errorf("IsValidStatusTransition(%q, %q) = %v, want %v", tt.current, tt.target, got, tt.want)
			}
		})
	}
}

func TestDetermineApprovalFlags(t *testing.T) {
	snap := &FinanceSnapshot{
		TotalOutstandingIDR: 800000,
		DepositIDR:          200000,
	}
	amount, flags := DetermineApprovalFlags(snap)
	if amount != 800000 {
		t.Errorf("expected amount 800000, got %d", amount)
	}
	if len(flags) != 2 || flags[0] != "has_outstanding" || flags[1] != "has_deposit" {
		t.Errorf("expected flags [has_outstanding, has_deposit], got %v", flags)
	}

	amountZero, flagsZero := DetermineApprovalFlags(nil)
	if amountZero != 0 || len(flagsZero) != 0 {
		t.Errorf("expected zero amount and empty flags for nil snapshot")
	}
}

func TestShouldApplyImmediately(t *testing.T) {
	loc := time.FixedZone("WIB", 7*3600)
	now := time.Date(2026, 7, 10, 14, 0, 0, 0, loc)

	// Effective today
	apply, err := ShouldApplyImmediately("2026-07-10", now, loc)
	if err != nil || !apply {
		t.Errorf("effective today should apply immediately, got apply=%v, err=%v", apply, err)
	}

	// Effective past
	applyPast, err := ShouldApplyImmediately("2026-07-01", now, loc)
	if err != nil || !applyPast {
		t.Errorf("effective past date should apply immediately, got apply=%v, err=%v", applyPast, err)
	}

	// Effective future
	applyFuture, err := ShouldApplyImmediately("2026-07-15", now, loc)
	if err != nil || applyFuture {
		t.Errorf("effective future date should NOT apply immediately, got apply=%v, err=%v", applyFuture, err)
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 || (len(s) > 0 && len(substr) > 0 && (s[:len(substr)] == substr || (len(s) > len(substr) && stringContains(s, substr)))))
}

func stringContains(s, substr string) bool {
	for i := 0; i+len(substr) <= len(s); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
