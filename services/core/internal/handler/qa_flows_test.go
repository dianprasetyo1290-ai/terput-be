package handler

import (
	"net/http"
	"testing"
)

// BUG-112 (TC-79-021): approval flow configuration is Super Admin only.
func TestApprovalFlows_SuperadminOnly(t *testing.T) {
	_, router, _ := newTestServer(t)
	body := `{"request_type":"refund","name":"Refund","active":true,"steps":[{"seq":1,"approver_kind":"role","approver_role":"finance_manager"}]}`
	tests := []struct {
		role       string
		wantList   int
		wantCreate int
	}{
		{"admin", http.StatusForbidden, http.StatusForbidden},
		{"finance_manager", http.StatusForbidden, http.StatusForbidden},
		{"superadmin", http.StatusOK, http.StatusCreated},
	}
	for _, tt := range tests {
		t.Run(tt.role, func(t *testing.T) {
			tok := token(t, testUserAdmin, testSchoolA, tt.role)
			if rec := do(router, http.MethodGet, "/approval-flows", tok, nil, ""); rec.Code != tt.wantList {
				t.Fatalf("list: want %d, got %d", tt.wantList, rec.Code)
			}
			if rec := do(router, http.MethodPost, "/approval-flows", tok, nil, body); rec.Code != tt.wantCreate {
				t.Fatalf("create: want %d, got %d (%s)", tt.wantCreate, rec.Code, rec.Body.String())
			}
		})
	}
}

// BUG-112: a unit admin no longer reads arbitrary requests by id.
func TestGetApproval_AdminNoBypass(t *testing.T) {
	fs, router, _ := newTestServer(t)
	seedRefundFlow(t, fs)
	res := createRefund(router, testUserMaker, "v1")
	if res.code != http.StatusCreated {
		t.Fatalf("create: %d %s", res.code, res.body)
	}
	if rec := do(router, http.MethodGet, "/approvals/"+res.id, token(t, testUserAdmin, testSchoolA, "admin"), nil, ""); rec.Code == http.StatusOK {
		t.Fatalf("admin read a request it is not part of: %d", rec.Code)
	}
	if rec := do(router, http.MethodGet, "/approvals/"+res.id, token(t, testUserAdmin, testSchoolA, "superadmin"), nil, ""); rec.Code != http.StatusOK {
		t.Fatalf("superadmin: want 200, got %d", rec.Code)
	}
}

// BUG-166 (TC-79-013): overlapping thresholds are refused on save.
func TestCreateFlow_OverlappingThresholds(t *testing.T) {
	_, router, _ := newTestServer(t)
	tok := token(t, testUserAdmin, testSchoolA, "superadmin")
	body := `{"request_type":"refund","name":"Refund","active":true,"steps":[
		{"seq":1,"approver_kind":"role","approver_role":"finance_manager","min_amount_idr":4000000},
		{"seq":2,"approver_kind":"role","approver_role":"kepala_sekolah","min_amount_idr":4000000}]}`
	rec := do(router, http.MethodPost, "/approval-flows", tok, nil, body)
	if rec.Code != http.StatusUnprocessableEntity && rec.Code != http.StatusBadRequest {
		t.Fatalf("want 400/422, got %d (%s)", rec.Code, rec.Body.String())
	}
}
