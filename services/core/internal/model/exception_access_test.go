package model

import (
	"reflect"
	"slices"
	"testing"
)

func TestExceptionKindsFor(t *testing.T) {
	tests := []struct {
		role        string
		sees        []string
		doesNotSee  []string
		wantNoneAll bool
	}{
		{role: "admin", sees: []string{"webhook", "notification", "import", "dead_letter", "callback"}, doesNotSee: []string{"journal_posting"}},
		{role: "finance_manager", sees: []string{"webhook", "journal_posting"}, doesNotSee: []string{"import", "notification"}},
		{role: "akuntan", sees: []string{"webhook", "journal_posting"}, doesNotSee: []string{"import", "notification", "dead_letter"}},
		{role: "payroll_officer", sees: []string{"journal_posting"}, doesNotSee: []string{"webhook"}},
		{role: "superadmin", sees: ExceptionKinds},
		{role: "auditor", sees: ExceptionKinds},
		{role: "guru", wantNoneAll: true},
		{role: "siswa", wantNoneAll: true},
	}
	for _, tt := range tests {
		t.Run(tt.role, func(t *testing.T) {
			got := ExceptionKindsFor(tt.role)
			if tt.wantNoneAll {
				if len(got) != 0 {
					t.Fatalf("want no access, got %v", got)
				}
				return
			}
			for _, k := range tt.sees {
				if !slices.Contains(got, k) {
					t.Errorf("%s should see %s", tt.role, k)
				}
			}
			for _, k := range tt.doesNotSee {
				if slices.Contains(got, k) {
					t.Errorf("BUG-114: %s must not see %s", tt.role, k)
				}
			}
		})
	}
}

func TestExceptionResponsibleRoles(t *testing.T) {
	tests := map[string][]string{
		"journal_posting": {"akuntan", "finance_manager", "payroll_officer", "superadmin"},
		"webhook":         {"admin", "akuntan", "finance_manager", "superadmin"},
		"import":          {"admin", "superadmin"},
	}
	for kind, want := range tests {
		if got := ExceptionResponsibleRoles(kind); !reflect.DeepEqual(got, want) {
			t.Errorf("%s: got %v, want %v", kind, got, want)
		}
	}
}
