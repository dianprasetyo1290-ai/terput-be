package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"skci/core/internal/client"
	"skci/core/internal/common"
	"skci/core/internal/config"
	"skci/core/internal/model"
)

// srFakeStore returns one fixed service request (the shared fake has none).
type srFakeStore struct {
	*fakeStore
	sr model.ServiceRequest
}

func (f *srFakeStore) GetServiceRequest(_ context.Context, schoolID, id string) (*model.ServiceRequest, error) {
	if schoolID != f.sr.SchoolID || id != f.sr.ID {
		return f.fakeStore.GetServiceRequest(context.Background(), schoolID, id)
	}
	cp := f.sr
	return &cp, nil
}

func (f *srFakeStore) UpdateServiceRequestStatus(_ context.Context, _, _, to, _, _ string) (*model.ServiceRequest, error) {
	cp := f.sr
	cp.Status = to
	return &cp, nil
}

func (f *srFakeStore) UpdateServiceRequestResult(_ context.Context, _, _ string, res []byte) (*model.ServiceRequest, error) {
	cp := f.sr
	cp.ResultAttachments = res
	return &cp, nil
}

// TestServiceRequestHandlerOnly: status and result changes are limited to
// admin or the category's handler role (QA-PAY round 2, menu 81).
func TestServiceRequestHandlerOnly(t *testing.T) {
	const (
		parent = "77777777-7777-7777-7777-777777777777"
		staff  = "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa"
		guru   = "bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb"
		admin  = "cccccccc-cccc-cccc-cccc-cccccccccccc"
	)
	fs := &srFakeStore{fakeStore: newFakeStore(), sr: model.ServiceRequest{
		ID: "sr-1", SchoolID: testSchoolA, RequesterUserID: parent, HandlerRole: "tata_usaha", Status: "submitted",
		Attachments: json.RawMessage("[]"), ResultAttachments: json.RawMessage("[]"),
	}}
	cfg := config.Config{JWTSecret: testJWTSecret, InternalKey: testInternalKey}
	router := NewRouter(New(fs, cfg, client.New("", "", "", testInternalKey)), cfg)
	status := `{"to":"accepted","note":"ok"}`

	cases := []struct {
		name string
		tok  string
		want int
	}{
		{"parent requester", token(t, parent, testSchoolA, common.RoleParent), http.StatusForbidden},
		{"other staff", token(t, guru, testSchoolA, common.RoleGuru), http.StatusForbidden},
		{"handler role", token(t, staff, testSchoolA, "tata_usaha"), http.StatusOK},
		{"school admin", token(t, admin, testSchoolA, common.RoleAdmin), http.StatusOK},
	}
	for _, c := range cases {
		if rec := do(router, http.MethodPost, "/service-requests/sr-1/status", c.tok, nil, status); rec.Code != c.want {
			t.Fatalf("%s: status change want %d, got %d %s", c.name, c.want, rec.Code, rec.Body.String())
		}
	}

	parentTok := token(t, parent, testSchoolA, common.RoleParent)
	if rec := do(router, http.MethodPost, "/service-requests/sr-1/result", parentTok, nil, `{"result_attachments":[]}`); rec.Code != http.StatusForbidden {
		t.Fatalf("parent result: want 403, got %d", rec.Code)
	}
	staffTok := token(t, staff, testSchoolA, "tata_usaha")
	foreign := `{"result_attachments":[{"name":"x.pdf","key":"service-requests/` + testSchoolA + `/` + parent + `/abc-x.pdf"}]}`
	if rec := do(router, http.MethodPost, "/service-requests/sr-1/result", staffTok, nil, foreign); rec.Code != http.StatusBadRequest {
		t.Fatalf("result with someone else's file: want 400, got %d %s", rec.Code, rec.Body.String())
	}
	own := `{"result_attachments":[{"name":"x.pdf","key":"service-requests/` + testSchoolA + `/` + staff + `/abc-x.pdf"}]}`
	if rec := do(router, http.MethodPost, "/service-requests/sr-1/result", staffTok, nil, own); rec.Code != http.StatusOK {
		t.Fatalf("result with own upload: want 200, got %d %s", rec.Code, rec.Body.String())
	}

	// Download: requester or handler only, and only keys of this request.
	if rec := do(router, http.MethodGet, "/service-requests/sr-1/files?key=service-requests/x", token(t, guru, testSchoolA, common.RoleGuru), nil, ""); rec.Code != http.StatusForbidden {
		t.Fatalf("download by other staff: want 403, got %d", rec.Code)
	}
	if rec := do(router, http.MethodGet, "/service-requests/sr-1/files?key=service-requests/x", parentTok, nil, ""); rec.Code != http.StatusNotFound {
		t.Fatalf("download of unknown key: want 404, got %d", rec.Code)
	}
}

func TestNormalizeAttachments(t *testing.T) {
	const school, user = "s1", "u1"
	if out, err := normalizeAttachments(nil, school, user); err != nil || string(out) != "[]" {
		t.Fatalf("empty: got %s %v", out, err)
	}
	bad := []string{
		`[{"name":"a","key":"uploads/a.pdf"}]`,
		`[{"name":"a","key":"service-requests/s2/u1/a.pdf"}]`,
		`[{"name":"a","key":"service-requests/s1/u2/a.pdf"}]`,
		`[{"name":"a","key":"service-requests/s1/u1/../u2/a.pdf"}]`,
		`{"name":"a"}`,
	}
	for _, b := range bad {
		if _, err := normalizeAttachments(json.RawMessage(b), school, user); err == nil {
			t.Fatalf("%s: want rejection", b)
		}
	}
	out, err := normalizeAttachments(json.RawMessage(`[{"name":"a","key":"service-requests/s1/u1/x-a.pdf","url":"http://evil"}]`), school, user)
	if err != nil {
		t.Fatalf("own key rejected: %v", err)
	}
	var list []model.Attachment
	_ = json.Unmarshal(out, &list)
	if len(list) != 1 || list[0].URL != "" {
		t.Fatalf("url must be dropped: %s", out)
	}
}
