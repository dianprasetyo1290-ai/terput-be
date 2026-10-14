package handler

import (
	"net/http"
	"testing"
)

const (
	testUserStudent = "88888888-8888-8888-8888-888888888888"
	testUserKepala2 = "99999999-9999-9999-9999-999999999999"
	testUserUnknown = "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa"
)

// BUG-002 / FR-79-005: delegation only to a user with an equivalent approver
// role in the same school.
func TestCreateDelegation_PeerRoleRequired(t *testing.T) {
	_, router, _, _ := newWiredServer(t, map[string][]string{
		testUserKepala:  {"kepala_sekolah"},
		testUserKepala2: {"kepala_sekolah", "guru"},
		testUserStudent: {"siswa"},
		testUserFinance: {"finance_manager"},
	})
	tok := token(t, testUserKepala, testSchoolA, "kepala_sekolah")
	body := func(to string) string {
		return `{"to_user_id":"` + to + `","valid_from":"2026-10-15T00:00:00Z","valid_to":"2026-10-20T00:00:00Z"}`
	}
	tests := []struct {
		name string
		to   string
		want int
	}{
		{"diri sendiri ditolak", testUserKepala, http.StatusBadRequest},
		{"ke siswa ditolak", testUserStudent, http.StatusUnprocessableEntity},
		{"ke peran lain (finance) ditolak", testUserFinance, http.StatusUnprocessableEntity},
		{"ke pengguna tak dikenal ditolak", testUserUnknown, http.StatusUnprocessableEntity},
		{"bukan UUID ditolak", "budi", http.StatusBadRequest},
		{"ke kepsek lain diterima", testUserKepala2, http.StatusCreated},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := do(router, http.MethodPost, "/approval-delegations", tok, nil, body(tt.to))
			if rec.Code != tt.want {
				t.Fatalf("want %d, got %d (%s)", tt.want, rec.Code, rec.Body.String())
			}
		})
	}
}

// Without identity the delegate cannot be verified: refuse, never accept blind.
func TestCreateDelegation_IdentityUnavailable(t *testing.T) {
	_, router, _ := newTestServer(t)
	tok := token(t, testUserKepala, testSchoolA, "kepala_sekolah")
	rec := do(router, http.MethodPost, "/approval-delegations", tok, nil,
		`{"to_user_id":"`+testUserKepala2+`","valid_from":"2026-10-15T00:00:00Z","valid_to":"2026-10-20T00:00:00Z"}`)
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("want 503, got %d (%s)", rec.Code, rec.Body.String())
	}
}
