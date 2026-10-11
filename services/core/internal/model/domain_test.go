package model

import (
	"reflect"
	"testing"
)

func ptr[T any](v T) *T { return &v }

func TestResolveSteps(t *testing.T) {
	steps := []FlowStep{
		{
			Seq:          1,
			ApproverKind: ApproverKindRole,
			ApproverRole: ptr("finance_manager"),
			// No min amount, applies to all
		},
		{
			Seq:          2,
			ApproverKind: ApproverKindRole,
			ApproverRole: ptr("kepala_sekolah"),
			MinAmountIDR: ptr(int64(5_000_000)), // AC-79-02: > 5 jt needs Kepala Unit
		},
		{
			Seq:           3,
			ApproverKind:  ApproverKindRole,
			ApproverRole:  ptr("hr_manager"),
			ConditionFlag: ptr("exceeds_headcount"), // AC-88-01
		},
		{
			Seq:           4,
			ApproverKind:  ApproverKindRole,
			ApproverRole:  ptr("kepala_yayasan"),
			ConditionFlag: ptr("salary_out_of_range"), // AC-92-01
		},
	}

	tests := []struct {
		name      string
		amountIDR *int64
		flags     []string
		wantSeqs  []int
		wantErr   bool
	}{
		{
			name:      "AC-79-02: refund 1 jt -> only step 1",
			amountIDR: ptr(int64(1_000_000)),
			flags:     nil,
			wantSeqs:  []int{1},
		},
		{
			name:      "AC-79-02: refund 6 jt -> step 1 and step 2",
			amountIDR: ptr(int64(6_000_000)),
			flags:     nil,
			wantSeqs:  []int{1, 2},
		},
		{
			name:      "AC-88-01: requisition with exceeds_headcount flag",
			amountIDR: nil,
			flags:     []string{"exceeds_headcount"},
			wantSeqs:  []int{1, 3},
		},
		{
			name:      "AC-92-01: salary_out_of_range flag",
			amountIDR: nil,
			flags:     []string{"salary_out_of_range"},
			wantSeqs:  []int{1, 4},
		},
		{
			name:      "Combination of flags and amount",
			amountIDR: ptr(int64(10_000_000)),
			flags:     []string{"exceeds_headcount", "salary_out_of_range"},
			wantSeqs:  []int{1, 2, 3, 4},
		},
		{
			name:      "No applicable steps returns error",
			amountIDR: ptr(int64(100)),
			flags:     nil,
			wantErr:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			inputSteps := steps
			if tt.wantErr {
				// Only steps requiring flags or higher amount
				inputSteps = []FlowStep{
					{Seq: 1, ApproverKind: ApproverKindRole, ApproverRole: ptr("admin"), MinAmountIDR: ptr(int64(5_000_000))},
				}
			}

			resolved, err := ResolveSteps(inputSteps, tt.amountIDR, tt.flags)
			if (err != nil) != tt.wantErr {
				t.Fatalf("ResolveSteps() error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantErr {
				return
			}

			var gotSeqs []int
			for _, s := range resolved {
				gotSeqs = append(gotSeqs, s.Seq)
			}
			if !reflect.DeepEqual(gotSeqs, tt.wantSeqs) {
				t.Errorf("ResolveSteps() got seqs %v, want %v", gotSeqs, tt.wantSeqs)
			}
		})
	}
}

func TestCanDecide(t *testing.T) {
	const (
		makerID      = "user-maker"
		otherUser    = "user-other"
		supervisorID = "user-supervisor"
		delegateID   = "user-delegate"
	)

	stepRole := ApproverStepInfo{Kind: ApproverKindRole, Role: "finance_manager"}
	stepSupervisor := ApproverStepInfo{Kind: ApproverKindDirectSupervisor, SupervisorID: supervisorID}
	stepUser := ApproverStepInfo{Kind: ApproverKindUser, UserID: otherUser}

	tests := []struct {
		name         string
		makerID      string
		step         ApproverStepInfo
		user         UserInfo
		delegations  []ActiveDelegation
		wantCan      bool
		wantDelegate *string
	}{
		{
			name:    "AC-79-01: Maker cannot decide own request (role match)",
			makerID: makerID,
			step:    stepRole,
			user:    UserInfo{UserID: makerID, ActiveRole: "finance_manager"},
			wantCan: false,
		},
		{
			name:    "AC-79-01: Maker cannot decide own request (user match)",
			makerID: makerID,
			step:    ApproverStepInfo{Kind: ApproverKindUser, UserID: makerID},
			user:    UserInfo{UserID: makerID},
			wantCan: false,
		},
		{
			name:    "AC-79-01: Maker cannot decide own request even via delegation",
			makerID: makerID,
			step:    stepRole,
			user:    UserInfo{UserID: delegateID},
			delegations: []ActiveDelegation{
				{FromUserID: makerID, ToUserID: delegateID, FromUserRoles: []string{"finance_manager"}},
			},
			wantCan: false,
		},
		{
			name:    "Role match: user with active_role",
			makerID: makerID,
			step:    stepRole,
			user:    UserInfo{UserID: otherUser, ActiveRole: "finance_manager"},
			wantCan: true,
		},
		{
			name:    "Role match: user with role in Roles list",
			makerID: makerID,
			step:    stepRole,
			user:    UserInfo{UserID: otherUser, ActiveRole: "guru", Roles: []string{"guru", "finance_manager"}},
			wantCan: true,
		},
		{
			name:    "Role mismatch",
			makerID: makerID,
			step:    stepRole,
			user:    UserInfo{UserID: otherUser, ActiveRole: "admin"},
			wantCan: false,
		},
		{
			name:    "Direct supervisor match",
			makerID: makerID,
			step:    stepSupervisor,
			user:    UserInfo{UserID: supervisorID},
			wantCan: true,
		},
		{
			name:    "Direct supervisor mismatch",
			makerID: makerID,
			step:    stepSupervisor,
			user:    UserInfo{UserID: otherUser},
			wantCan: false,
		},
		{
			name:    "User match",
			makerID: makerID,
			step:    stepUser,
			user:    UserInfo{UserID: otherUser},
			wantCan: true,
		},
		{
			name:    "Delegation match for role",
			makerID: makerID,
			step:    stepRole,
			user:    UserInfo{UserID: delegateID, ActiveRole: "guru"},
			delegations: []ActiveDelegation{
				{FromUserID: otherUser, ToUserID: delegateID, FromUserRoles: []string{"finance_manager"}},
			},
			wantCan:      true,
			wantDelegate: ptr(otherUser),
		},
		{
			name:    "Delegation match for supervisor",
			makerID: makerID,
			step:    stepSupervisor,
			user:    UserInfo{UserID: delegateID},
			delegations: []ActiveDelegation{
				{FromUserID: supervisorID, ToUserID: delegateID},
			},
			wantCan:      true,
			wantDelegate: ptr(supervisorID),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotCan, gotDelegate := CanDecide(tt.makerID, tt.step, tt.user, tt.delegations)
			if gotCan != tt.wantCan {
				t.Errorf("CanDecide() gotCan = %v, want %v", gotCan, tt.wantCan)
			}
			if tt.wantDelegate != nil {
				if gotDelegate == nil || *gotDelegate != *tt.wantDelegate {
					t.Errorf("CanDecide() gotDelegate = %v, want %v", gotDelegate, *tt.wantDelegate)
				}
			} else {
				if gotDelegate != nil {
					t.Errorf("CanDecide() gotDelegate = %v, want nil", gotDelegate)
				}
			}
		})
	}
}

func TestMaskSensitive(t *testing.T) {
	input := map[string]any{
		"name":            "Budi Santoso",
		"nik":             "3275012345678901",
		"npwp":            "09.123.456.7-890.000",
		"phone":           "08123456789",
		"email":           "budi.santoso@example.com",
		"account_number":  "1234567890",
		"salary":          15000000,
		"base_salary_idr": 12000000,
		"nested": map[string]any{
			"bank_account": "987654321",
			"net_idr":      14000000,
			"safe_note":    "Bonus tahunan",
		},
		"items": []any{
			map[string]any{
				"nik":   "12345",
				"label": "Keluarga",
			},
		},
	}

	masked := MaskSensitive(input)

	if masked["name"] != "Budi Santoso" {
		t.Errorf("name should not be masked, got %v", masked["name"])
	}
	if masked["nik"] != "••••8901" {
		t.Errorf("nik should end in 8901, got %v", masked["nik"])
	}
	if masked["phone"] != "••••6789" {
		t.Errorf("phone should end in 6789, got %v", masked["phone"])
	}
	if masked["email"] != "b•••@example.com" {
		t.Errorf("email masked incorrectly, got %v", masked["email"])
	}
	if masked["salary"] != "•••" {
		t.Errorf("salary should be •••, got %v", masked["salary"])
	}
	if masked["base_salary_idr"] != "•••" {
		t.Errorf("base_salary_idr should be •••, got %v", masked["base_salary_idr"])
	}

	nested, ok := masked["nested"].(map[string]any)
	if !ok {
		t.Fatalf("nested should be map, got %T", masked["nested"])
	}
	if nested["bank_account"] != "••••4321" {
		t.Errorf("nested bank_account should end in 4321, got %v", nested["bank_account"])
	}
	if nested["net_idr"] != "•••" {
		t.Errorf("nested net_idr should be •••, got %v", nested["net_idr"])
	}
	if nested["safe_note"] != "Bonus tahunan" {
		t.Errorf("safe_note should not be masked, got %v", nested["safe_note"])
	}

	items, ok := masked["items"].([]any)
	if !ok || len(items) == 0 {
		t.Fatalf("items slice invalid: %v", masked["items"])
	}
	item0 := items[0].(map[string]any)
	if item0["nik"] != "••••2345" {
		t.Errorf("array item nik should be ••••2345, got %v", item0["nik"])
	}
	if item0["label"] != "Keluarga" {
		t.Errorf("label should not be masked, got %v", item0["label"])
	}
}
