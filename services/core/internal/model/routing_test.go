package model

import (
	"errors"
	"testing"
)

func TestRerouteMakerOnlyStep(t *testing.T) {
	admin, kepsek := "admin", "kepala_sekolah"
	const maker, other = "u-maker", "u-other"
	roleStep := ApprovalStep{Seq: 1, ApproverKind: ApproverKindRole, ApproverRole: &admin}
	roleStepEsc := ApprovalStep{Seq: 1, ApproverKind: ApproverKindRole, ApproverRole: &admin, EscalateToRole: &kepsek}
	m := maker
	userStepEsc := ApprovalStep{Seq: 1, ApproverKind: ApproverKindUser, ApproverUserID: &m, EscalateToRole: &kepsek}

	tests := []struct {
		name        string
		step        ApprovalStep
		approvers   []string
		escalate    []string
		wantErr     bool
		wantReroute bool
	}{
		{"ada penyetuju lain: tetap", roleStep, []string{maker, other}, nil, false, false},
		{"BUG-111: pembuat satu-satunya, tanpa eskalasi: ditolak", roleStep, []string{maker}, nil, true, false},
		{"pembuat satu-satunya, eskalasi ada pemegang lain: dialihkan", roleStepEsc, []string{maker}, []string{other}, false, true},
		{"pembuat satu-satunya, eskalasi hanya pembuat: ditolak", roleStepEsc, []string{maker}, []string{maker}, true, false},
		{"tahap user = pembuat, eskalasi tersedia: dialihkan", userStepEsc, []string{maker}, []string{other}, false, true},
		{"tidak ada pemegang peran sama sekali, eskalasi tersedia: dialihkan", roleStepEsc, nil, []string{other}, false, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, rerouted, err := RerouteMakerOnlyStep(tt.step, maker, tt.approvers, tt.escalate)
			if (err != nil) != tt.wantErr || (err != nil && !errors.Is(err, ErrNoIndependentApprover)) {
				t.Fatalf("err = %v, wantErr %v", err, tt.wantErr)
			}
			if rerouted != tt.wantReroute {
				t.Fatalf("rerouted = %v, want %v", rerouted, tt.wantReroute)
			}
			if rerouted {
				if got.ApproverKind != ApproverKindRole || got.ApproverRole == nil || *got.ApproverRole != kepsek || got.ApproverUserID != nil || got.EscalateToRole != nil {
					t.Errorf("rerouted step = %+v", got)
				}
			}
		})
	}
}
