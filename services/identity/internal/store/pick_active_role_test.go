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
	got, sid, _, ok := PickActiveRole([]model.UserSchoolRole{role("orang_tua", "parent-school")})
	if !ok || got != "orang_tua" || sid != "parent-school" {
		t.Fatalf("parent role not selected safely: role=%q school=%q ok=%v", got, sid, ok)
	}
	got, _, _, ok = PickActiveRole([]model.UserSchoolRole{role("orang_tua", "s1"), role("admin", "s1")})
	if !ok || got != "admin" {
		t.Fatalf("priority lost: got %q ok=%v", got, ok)
	}
	// A guru who also holds a jabatan must land on the jabatan, not the
	// classroom -- otherwise Kepala Sekolah / Wali Kelas open the app on the
	// identical guru UI and look like a plain teacher (they still reach guru
	// screens via "Beralih peran").
	for _, name := range []string{"kepala_sekolah", "kepala_yayasan", "walikelas"} {
		got, _, all, ok := PickActiveRole([]model.UserSchoolRole{role("guru", "s1"), role(name, "s1")})
		if !ok || got != name {
			t.Fatalf("jabatan %s must win over guru: got %q all=%v ok=%v", name, got, all, ok)
		}
	}
	// ...but teaching still outranks the student/parent dashboards.
	got, _, _, ok = PickActiveRole([]model.UserSchoolRole{role("siswa", "s1"), role("guru", "s1")})
	if !ok || got != "guru" {
		t.Fatalf("guru must win over siswa: got %q ok=%v", got, ok)
	}
	if _, _, _, ok := PickActiveRole(nil); ok {
		t.Fatal("account with no roles must not log in")
	}
}
