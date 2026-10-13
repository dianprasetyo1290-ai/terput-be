package sensitive

import (
	"bytes"
	"testing"

	"skci/studentlife/internal/common"
)

func TestCanAccess(t *testing.T) {
	cases := []struct {
		name     string
		module   string
		role     string
		facts    Facts
		policies []string
		want     Level
	}{
		{"petugas uks full", ModuleHealth, common.RolePetugasUKS, Facts{}, nil, Full},
		{"wali terverifikasi full", ModuleHealth, common.RoleParent, Facts{IsGuardian: true}, nil, Full},
		{"orang tua bukan wali none", ModuleHealth, common.RoleParent, Facts{}, nil, None},
		{"wali kelas siswa alert only (AC-127-01)", ModuleHealth, common.RoleWalikelas, Facts{IsHomeroom: true}, nil, AlertOnly},
		{"wali kelas kelas lain none", ModuleHealth, common.RoleWalikelas, Facts{}, nil, None},
		{"kepala sekolah tanpa kebijakan none (OQ-31)", ModuleHealth, common.RoleKepalaSekolah, Facts{}, nil, None},
		{"kepala sekolah dengan kebijakan full", ModuleHealth, common.RoleKepalaSekolah, Facts{}, []string{common.RoleKepalaSekolah}, Full},
		{"superadmin tanpa kebijakan none", ModuleHealth, common.RoleSuperadmin, Facts{}, nil, None},
		{"admin none", ModuleHealth, common.RoleAdmin, Facts{}, []string{common.RoleKepalaSekolah}, None},
		{"guru none", ModuleHealth, common.RoleGuru, Facts{IsHomeroom: true}, nil, None},
		{"modul konseling: wali tidak otomatis", ModuleCounseling, common.RoleParent, Facts{IsGuardian: true}, nil, None},
		{"konseling anggota kasus full", ModuleCounseling, common.RoleKonselor, Facts{IsCaseMember: true}, nil, Full},
		{"konseling konselor bukan anggota none", ModuleCounseling, common.RoleKonselor, Facts{IsCaseMember: false}, nil, None},
		{"konseling kebijakan konselor full", ModuleCounseling, common.RoleKonselor, Facts{}, []string{common.RoleKonselor}, Full},
		{"konseling walikelas disclose true status only (AC-130-01)", ModuleCounseling, common.RoleWalikelas, Facts{IsHomeroom: true, HasDisclosedCase: true}, nil, StatusOnly},
		{"konseling walikelas disclose false none (AC-130-01)", ModuleCounseling, common.RoleWalikelas, Facts{IsHomeroom: true, HasDisclosedCase: false}, nil, None},
		{"konseling kepala sekolah tanpa kebijakan none (OQ-31)", ModuleCounseling, common.RoleKepalaSekolah, Facts{}, nil, None},
		{"konseling kepala sekolah dengan kebijakan full", ModuleCounseling, common.RoleKepalaSekolah, Facts{}, []string{common.RoleKepalaSekolah}, Full},
	}
	for _, tc := range cases {
		if got := CanAccess(tc.module, tc.role, tc.facts, tc.policies); got != tc.want {
			t.Errorf("%s: got %v want %v", tc.name, got, tc.want)
		}
	}
}

func TestSealJSONBindsRow(t *testing.T) {
	svc, err := New(bytes.Repeat([]byte{0x11}, 32), "test-secret-blind-key-12345")
	if err != nil {
		t.Fatal(err)
	}
	in := map[string]string{"blood_type": "O"}
	aadA := AAD("health_profiles", "school-1", "row-a")
	ct, err := svc.SealJSON(in, aadA)
	if err != nil || len(ct) == 0 {
		t.Fatalf("seal: %v", err)
	}
	if bytes.Contains(ct, []byte("blood_type")) {
		t.Fatal("ciphertext leaks plaintext")
	}
	var out map[string]string
	if err := svc.OpenJSON(ct, aadA, &out); err != nil || out["blood_type"] != "O" {
		t.Fatalf("open: %v %v", err, out)
	}
	// AAD of another row (ciphertext moved) must fail.
	if err := svc.OpenJSON(ct, AAD("health_profiles", "school-1", "row-b"), &out); err == nil {
		t.Fatal("decrypt with another row's AAD must fail")
	}
	if err := svc.OpenJSON(ct, AAD("health_profiles", "school-2", "row-a"), &out); err == nil {
		t.Fatal("decrypt with another school's AAD must fail")
	}
	// Empty plaintext → nil ciphertext.
	if ct, err := svc.Encrypt("", aadA); err != nil || ct != nil {
		t.Fatalf("empty plaintext: %v %v", ct, err)
	}
	if ct, err := svc.SealJSON(nil, aadA); err != nil || ct != nil {
		t.Fatalf("nil value: %v %v", ct, err)
	}
}
