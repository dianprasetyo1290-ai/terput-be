package store

import "testing"

import "skci/identity/internal/model"

func role(name, school string) model.UserSchoolRole {
	return model.UserSchoolRole{Role: name, SchoolID: &school}
}

// TestPickActiveRoleNonPriorityRoles pins the login bug that shipped once:
// priority was treated as an allowlist, so an account holding only
// pengawas (or walikelas / kepala_sekolah / kepala_yayasan) could not log
// in at all -- Login returned "account has no assigned role" and the login
// screen rendered it as "Email atau password salah."
func TestPickActiveRoleNonPriorityRoles(t *testing.T) {
	for _, name := range []string{"pengawas", "walikelas", "kepala_sekolah", "kepala_yayasan"} {
		got, sid, all, ok := PickActiveRole([]model.UserSchoolRole{role(name, "s1")})
		if !ok {
			t.Fatalf("%s-only account rejected at login", name)
		}
		if got != name || sid != "s1" || len(all) != 1 {
			t.Fatalf("%s: got role=%q school=%q all=%v", name, got, sid, all)
		}
	}
}

// Priority still wins when an account holds several roles, and an account
// with no role at all is still rejected.
func TestPickActiveRolePriorityAndEmpty(t *testing.T) {
	got, _, _, ok := PickActiveRole([]model.UserSchoolRole{role("pengawas", "s1"), role("admin", "s1")})
	if !ok || got != "admin" {
		t.Fatalf("priority lost: got %q ok=%v", got, ok)
	}
	if _, _, _, ok := PickActiveRole(nil); ok {
		t.Fatal("account with no roles must not log in")
	}
}
