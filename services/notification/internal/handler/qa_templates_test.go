package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"sync"
	"testing"

	"skci/notification/internal/client"
	"skci/notification/internal/model"
)

// BUG-088: admins manage event templates from the app; unknown variables are
// rejected; the newest version is what the next notification uses.
func TestAdminTemplates(t *testing.T) {
	fs, router, h, p, _ := newDeliveryServer(t, map[string][2]string{userA: {"wali@example.com", "081234567890"}})
	var mu sync.Mutex
	var audits []client.AuditEvent
	h.Audit = func(_ context.Context, e client.AuditEvent) { mu.Lock(); audits = append(audits, e); mu.Unlock() }
	admin := token(t, userB, schoolA, roleAdmin)

	cases := []struct {
		name, tok, body string
		want            int
	}{
		{"guru ditolak", token(t, userB, schoolA, "guru"), `{"key":"bill_issued","channel":"in_app","subject":"x","body":"y"}`, http.StatusForbidden},
		{"variabel tidak dikenal", admin, `{"key":"bill_issued","channel":"in_app","subject":"Tagihan","body":"Halo {{nama_siswa}}"}`, http.StatusUnprocessableEntity},
		{"kurung tidak seimbang", admin, `{"key":"bill_issued","channel":"in_app","subject":"Tagihan","body":"Halo {{title"}`, http.StatusUnprocessableEntity},
		{"kanal push ditolak", admin, `{"key":"bill_issued","channel":"push","subject":"x","body":"y"}`, http.StatusBadRequest},
		{"email tanpa subjek", admin, `{"key":"bill_issued","channel":"email","body":"y"}`, http.StatusBadRequest},
		{"versi 1", admin, `{"key":"bill_issued","channel":"email","subject":"Tagihan v1","body":"{{body}}"}`, http.StatusCreated},
		{"versi 2", admin, `{"key":"bill_issued","channel":"email","subject":"Tagihan baru: {{title}}","body":"{{body}} Bayar di {{url}}"}`, http.StatusCreated},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec := do(router, http.MethodPost, "/notification-templates", tc.tok, nil, tc.body)
			if rec.Code != tc.want {
				t.Fatalf("want %d, got %d %s", tc.want, rec.Code, rec.Body.String())
			}
		})
	}
	if len(audits) != 2 || audits[0].EntityType != "notification_template" || audits[0].Action != "create" {
		t.Fatalf("audits = %+v", audits)
	}

	rec := do(router, http.MethodGet, "/notification-templates?key=bill_issued", admin, nil, "")
	var list struct{ Data []model.NotificationTemplate }
	_ = json.Unmarshal(rec.Body.Bytes(), &list)
	if rec.Code != http.StatusOK || len(list.Data) != 2 {
		t.Fatalf("list: %d %s", rec.Code, rec.Body.String())
	}

	rec = do(router, http.MethodGet, "/notification-templates/variables", admin, nil, "")
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"bill_issued":["bill_id","body","title","url"]`) {
		t.Fatalf("variables: %d %s", rec.Code, rec.Body.String())
	}

	send(router, "tpl-1", `{"school_id":"`+schoolA+`","user_id":"`+userA+`","type":"bill_issued","title":"Okt","body":"Tagihan SPP Okt","data":{"url":"https://app.example/tagihan/1"}}`)
	h.Wait()
	var email string
	for _, m := range p.sent {
		if m.Channel == model.ChannelEmail {
			email = m.Subject + "|" + m.Body
		}
	}
	if email != "Tagihan baru: Okt|Tagihan SPP Okt Bayar di https://app.example/tagihan/1" {
		t.Fatalf("email not rendered from newest template: %q", email)
	}

	var v2 string
	for _, tpl := range list.Data {
		if tpl.Version == 2 {
			v2 = tpl.ID
		}
	}
	if rec := do(router, http.MethodDelete, "/notification-templates/"+v2, admin, nil, ""); rec.Code != http.StatusOK {
		t.Fatalf("delete: %d %s", rec.Code, rec.Body.String())
	}
	if rec := do(router, http.MethodDelete, "/notification-templates/"+v2, token(t, userB, schoolB, roleAdmin), nil, ""); rec.Code != http.StatusNotFound {
		t.Fatalf("other school delete: want 404, got %d", rec.Code)
	}
	if got, _ := fs.FindNotificationTemplate(context.Background(), schoolA, "bill_issued", model.ChannelEmail); got == nil || got.Version != 1 {
		t.Fatalf("after deleting v2 the active version must be v1, got %+v", got)
	}
}
