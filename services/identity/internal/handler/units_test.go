package handler

import (
	"testing"

	"skci/identity/internal/model"
)

func TestUnitRole(t *testing.T) {
	sd, smp := "sd", "smp"
	rows := []model.UserSchoolRole{
		{SchoolID: &smp, Role: "admin"},
		{SchoolID: &smp, Role: "finance_manager"},
		{SchoolID: &sd, Role: "finance_manager"},
		{SchoolID: nil, Role: "superadmin"},
	}
	// Keeps the active role when it is held in the target unit too.
	if got := unitRole(rows, sd, "finance_manager"); got != "finance_manager" {
		t.Fatalf("keep active role: got %q", got)
	}
	// Otherwise the first role held in that unit.
	if got := unitRole(rows, smp, "kepala_sekolah"); got != "admin" {
		t.Fatalf("first role in unit: got %q", got)
	}
	// No role in the unit: no access.
	if got := unitRole(rows, "smk", "admin"); got != "" {
		t.Fatalf("unit without role: got %q", got)
	}
}
