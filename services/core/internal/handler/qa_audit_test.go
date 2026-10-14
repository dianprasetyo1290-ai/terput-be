package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"skci/core/internal/model"
)

func grantAuditor(t *testing.T, fs *fakeStore, userID string) {
	t.Helper()
	if _, err := fs.CreateAuditGrant(context.Background(), model.AuditAccessGrant{SchoolID: testSchoolA, UserID: userID,
		ValidFrom: time.Now().Add(-time.Hour), ValidTo: time.Now().Add(time.Hour), Reason: "audit tahunan", GrantedBy: testUserAdmin}); err != nil {
		t.Fatal(err)
	}
}

// BUG-152 (TC-06-006): reversed / malformed date ranges are refused.
func TestListAudit_DateRange(t *testing.T) {
	_, router, _ := newTestServer(t)
	tok := token(t, testUserAdmin, testSchoolA, "superadmin")
	tests := []struct {
		query string
		want  int
	}{
		{"from=2026-10-14&to=2026-10-01", http.StatusUnprocessableEntity},
		{"from=14-10-2026", http.StatusBadRequest},
		{"from=2026-10-01&to=2026-10-14", http.StatusOK},
		{"to=2026-10-14", http.StatusOK},
	}
	for _, tt := range tests {
		if rec := do(router, http.MethodGet, "/audit?"+tt.query, tok, nil, ""); rec.Code != tt.want {
			t.Errorf("%s: want %d, got %d (%s)", tt.query, tt.want, rec.Code, rec.Body.String())
		}
	}
}

// BUG-087 (AC-RBAC-10): auditor access is time-boxed.
func TestAuditorAccessWindow(t *testing.T) {
	fs, router, _ := newTestServer(t)
	tokAud := token(t, testUserAuditor, testSchoolA, "auditor")
	for _, path := range []string{"/audit", "/exceptions"} {
		if rec := do(router, http.MethodGet, path, tokAud, nil, ""); rec.Code != http.StatusForbidden {
			t.Fatalf("%s without grant: want 403, got %d", path, rec.Code)
		}
	}
	// expired grant does not count
	_, _ = fs.CreateAuditGrant(context.Background(), model.AuditAccessGrant{SchoolID: testSchoolA, UserID: testUserAuditor,
		ValidFrom: time.Now().Add(-48 * time.Hour), ValidTo: time.Now().Add(-24 * time.Hour), Reason: "lama", GrantedBy: testUserAdmin})
	if rec := do(router, http.MethodGet, "/audit", tokAud, nil, ""); rec.Code != http.StatusForbidden {
		t.Fatalf("expired grant: want 403, got %d", rec.Code)
	}
	grantAuditor(t, fs, testUserAuditor)
	for _, path := range []string{"/audit", "/exceptions"} {
		if rec := do(router, http.MethodGet, path, tokAud, nil, ""); rec.Code != http.StatusOK {
			t.Fatalf("%s with grant: want 200, got %d", path, rec.Code)
		}
	}
	// revoking closes access again
	grants, _ := fs.ListAuditGrants(context.Background(), testSchoolA)
	tokSA := token(t, testUserAdmin, testSchoolA, "superadmin")
	for _, g := range grants {
		if g.RevokedAt == nil && g.Active(time.Now()) {
			if rec := do(router, http.MethodDelete, "/audit/access-grants/"+g.ID, tokSA, nil, ""); rec.Code != http.StatusOK {
				t.Fatalf("revoke: %d %s", rec.Code, rec.Body.String())
			}
		}
	}
	if rec := do(router, http.MethodGet, "/audit", tokAud, nil, ""); rec.Code != http.StatusForbidden {
		t.Fatalf("after revoke: want 403, got %d", rec.Code)
	}
}

func TestCreateAuditGrant(t *testing.T) {
	_, router, h, _ := newWiredServer(t, map[string][]string{testUserAuditor: {"auditor"}, testUserFinance: {"finance_manager"}})
	now := time.Now().UTC()
	body := func(user string, from, to time.Time, reason string) string {
		b, _ := json.Marshal(map[string]any{"user_id": user, "valid_from": from, "valid_to": to, "reason": reason})
		return string(b)
	}
	tests := []struct {
		name, role, body string
		want             int
	}{
		{"admin ditolak", "admin", body(testUserAuditor, now, now.Add(24*time.Hour), "audit"), http.StatusForbidden},
		{"bukan auditor ditolak", "superadmin", body(testUserFinance, now, now.Add(24*time.Hour), "audit"), http.StatusUnprocessableEntity},
		{"rentang terbalik ditolak", "superadmin", body(testUserAuditor, now, now.Add(-time.Hour), "audit"), http.StatusUnprocessableEntity},
		{"lebih dari 180 hari ditolak", "superadmin", body(testUserAuditor, now, now.Add(200*24*time.Hour), "audit"), http.StatusUnprocessableEntity},
		{"tanpa alasan ditolak", "superadmin", body(testUserAuditor, now, now.Add(24*time.Hour), " "), http.StatusUnprocessableEntity},
		{"valid", "superadmin", body(testUserAuditor, now, now.Add(30*24*time.Hour), "audit semester"), http.StatusCreated},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := do(router, http.MethodPost, "/audit/access-grants", token(t, testUserAdmin, testSchoolA, tt.role), nil, tt.body)
			if rec.Code != tt.want {
				t.Fatalf("want %d, got %d (%s)", tt.want, rec.Code, rec.Body.String())
			}
		})
	}
	h.Wait()
}

// BUG-117: the request correlation id is echoed and stamped on audit events.
func TestCorrelationID_StampedOnAudit(t *testing.T) {
	_, router, _ := newTestServer(t)
	body := `{"school_id":"` + testSchoolA + `","service":"billing","module":"security","entity_type":"user","entity_id":"u1","action":"login_failed"}`
	rec := do(router, http.MethodPost, "/internal/audit", "", map[string]string{"X-Internal-Key": testInternalKey, "X-Correlation-ID": "flow-abc"}, body)
	if rec.Code != http.StatusAccepted || rec.Header().Get("X-Correlation-ID") != "flow-abc" {
		t.Fatalf("code %d header %q", rec.Code, rec.Header().Get("X-Correlation-ID"))
	}
	tok := token(t, testUserAdmin, testSchoolA, "superadmin")
	rec = do(router, http.MethodGet, "/audit?correlation_id=flow-abc", tok, nil, "")
	var resp struct{ Data []model.AuditEvent }
	_ = json.Unmarshal(rec.Body.Bytes(), &resp)
	if len(resp.Data) != 1 || resp.Data[0].CorrelationID == nil || *resp.Data[0].CorrelationID != "flow-abc" {
		t.Fatalf("audit by correlation: %s", rec.Body.String())
	}
}
