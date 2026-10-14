package client

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestCreateApproval(t *testing.T) {
	cases := []struct {
		name       string
		status     int
		body       any
		wantID     string
		wantStatus int
		wantMsg    string
	}{
		{"created", http.StatusCreated, map[string]any{"data": map[string]any{"id": "ap-1", "status": "pending"}}, "ap-1", 0, ""},
		{"flow missing is 422", http.StatusUnprocessableEntity, map[string]any{"message": "alur approval discount_special belum dikonfigurasi"}, "", http.StatusUnprocessableEntity, "Approval Center menolak pengajuan: alur approval discount_special belum dikonfigurasi"},
		{"core down is 502", http.StatusInternalServerError, map[string]any{"message": "boom"}, "", http.StatusBadGateway, "Approval Center tidak dapat dihubungi, pengajuan belum tersimpan"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var got ApprovalRequest
			var idem string
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				idem = r.Header.Get("Idempotency-Key")
				_ = json.NewDecoder(r.Body).Decode(&got)
				w.WriteHeader(c.status)
				_ = json.NewEncoder(w).Encode(c.body)
			}))
			defer srv.Close()
			id, err := New(srv.URL, "k").CreateApproval(context.Background(), ApprovalRequest{
				SchoolID: "s", RequestType: ReqTypeDiscountSpecial, SourceID: "md-1", Title: "t", MakerUserID: "u", CallbackURL: "http://x/cb",
			})
			if got.SourceService != "admission" || idem != "discount_special:md-1" || got.IdempotencyKey != idem {
				t.Fatalf("request not normalised: %+v idem=%s", got, idem)
			}
			if c.wantStatus == 0 {
				if err != nil || id != c.wantID {
					t.Fatalf("id=%q err=%v", id, err)
				}
				return
			}
			status, msg := ApprovalErrorMessage(err)
			if status != c.wantStatus || msg != c.wantMsg {
				t.Fatalf("status=%d msg=%q", status, msg)
			}
		})
	}
	if status, _ := ApprovalErrorMessage(errors.New("dial tcp: refused")); status != http.StatusBadGateway {
		t.Fatal("network error must map to 502")
	}
}
