package handler

import (
	"encoding/json"
	"net/http"
	"testing"

	"skci/core/internal/model"
)

// BUG-165 (TC-50-001..003): parents read the guide & FAQ; school admins
// replace or hide defaults and add their own entries, each change audited.
func TestHelpArticles(t *testing.T) {
	fs, router, _ := newTestServer(t)
	fs.helpArticles = []model.HelpArticle{
		{ID: "default-pay", Audience: model.HelpAudienceParent, Kind: model.HelpKindGuide, Title: "Cara membayar tagihan", Body: "umum", SortOrder: 10, Published: true},
		{ID: "default-faq", Audience: model.HelpAudienceParent, Kind: model.HelpKindFAQ, Title: "FAQ umum", Body: "jawaban", SortOrder: 20, Published: true},
	}
	parent := token(t, testUserMaker, testSchoolA, "orang_tua")
	admin := token(t, testUserAdmin, testSchoolA, "admin")
	titles := func(tok, path string) []string {
		rec := do(router, http.MethodGet, path, tok, nil, "")
		if rec.Code != http.StatusOK {
			t.Fatalf("%s: %d %s", path, rec.Code, rec.Body.String())
		}
		var resp struct{ Data []model.HelpArticle }
		_ = json.Unmarshal(rec.Body.Bytes(), &resp)
		var out []string
		for _, a := range resp.Data {
			out = append(out, a.Title)
		}
		return out
	}
	if got := titles(parent, "/help-articles"); len(got) != 2 {
		t.Fatalf("parent sees defaults: %v", got)
	}

	cases := []struct {
		name, tok, method, path, body string
		want                          int
	}{
		{"orang tua tidak bisa menulis", parent, http.MethodPost, "/help-articles", `{"audience":"orang_tua","kind":"faq","title":"x","body":"y","published":true}`, http.StatusForbidden},
		{"judul kosong", admin, http.MethodPost, "/help-articles", `{"audience":"orang_tua","kind":"faq","title":"","body":"y"}`, http.StatusUnprocessableEntity},
		{"ganti panduan bayar", admin, http.MethodPost, "/help-articles", `{"audience":"orang_tua","kind":"guide","title":"Cara membayar di sekolah kami","body":"VA bank X","sort_order":5,"published":true,"replaces_id":"default-pay"}`, http.StatusCreated},
		{"pengganti kedua ditolak", admin, http.MethodPost, "/help-articles", `{"audience":"orang_tua","kind":"guide","title":"lagi","body":"y","replaces_id":"default-pay"}`, http.StatusConflict},
		{"sembunyikan FAQ bawaan", admin, http.MethodPost, "/help-articles", `{"audience":"orang_tua","kind":"faq","title":"FAQ umum","body":"-","published":false,"replaces_id":"default-faq"}`, http.StatusCreated},
		{"tambah FAQ sekolah", admin, http.MethodPost, "/help-articles", `{"audience":"orang_tua","kind":"faq","title":"Kontak keuangan","body":"Ruang TU","sort_order":30,"published":true}`, http.StatusCreated},
		{"bawaan tidak bisa diubah", admin, http.MethodPut, "/help-articles/default-faq", `{"audience":"orang_tua","kind":"faq","title":"x","body":"y"}`, http.StatusUnprocessableEntity},
		{"bawaan tidak bisa dihapus", admin, http.MethodDelete, "/help-articles/default-faq", "", http.StatusUnprocessableEntity},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if rec := do(router, tc.method, tc.path, tc.tok, nil, tc.body); rec.Code != tc.want {
				t.Fatalf("want %d, got %d %s", tc.want, rec.Code, rec.Body.String())
			}
		})
	}
	got := titles(parent, "/help-articles")
	if len(got) != 2 || got[0] != "Cara membayar di sekolah kami" || got[1] != "Kontak keuangan" {
		t.Fatalf("parent view after edits: %v", got)
	}
	if got := titles(admin, "/help-articles/manage"); len(got) != 5 {
		t.Fatalf("admin manage view: %v", got)
	}
	if rec := do(router, http.MethodGet, "/help-articles/manage", parent, nil, ""); rec.Code != http.StatusForbidden {
		t.Fatalf("parent manage: want 403, got %d", rec.Code)
	}
	var created int
	for _, a := range fs.audits {
		if a.Module == "help_center" && a.Action == "create" {
			created++
		}
	}
	if created != 3 {
		t.Fatalf("audit create events = %d", created)
	}
}
