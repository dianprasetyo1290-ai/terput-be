package model

import (
	"errors"
	"testing"
)

func TestValidateHelpArticle(t *testing.T) {
	ok := HelpArticle{Audience: HelpAudienceParent, Kind: HelpKindGuide, Title: "Cara bayar", Body: "Langkah"}
	tests := []struct {
		name string
		edit func(*HelpArticle)
		want error
	}{
		{"valid", func(*HelpArticle) {}, nil},
		{"audiens salah", func(a *HelpArticle) { a.Audience = "guru" }, ErrHelpAudience},
		{"jenis salah", func(a *HelpArticle) { a.Kind = "blog" }, ErrHelpKind},
		{"judul kosong", func(a *HelpArticle) { a.Title = "  " }, ErrHelpTitle},
		{"isi kosong", func(a *HelpArticle) { a.Body = "" }, ErrHelpBody},
	}
	for _, tt := range tests {
		a := ok
		tt.edit(&a)
		if err := ValidateHelpArticle(a); !errors.Is(err, tt.want) {
			t.Errorf("%s: got %v want %v", tt.name, err, tt.want)
		}
	}
}

func TestMergeHelpArticles(t *testing.T) {
	school := "s1"
	d1, d2 := "d1", "d2"
	rows := []HelpArticle{
		{ID: d1, Title: "Default bayar", SortOrder: 10, Published: true},
		{ID: d2, Title: "Default FAQ", SortOrder: 20, Published: true},
		{ID: "s-replace", SchoolID: &school, ReplacesID: &d1, Title: "Bayar versi sekolah", SortOrder: 5, Published: true},
		{ID: "s-hide", SchoolID: &school, ReplacesID: &d2, Title: "Disembunyikan", SortOrder: 20, Published: false},
		{ID: "s-draft", SchoolID: &school, Title: "Draf", SortOrder: 1, Published: false},
		{ID: "s-own", SchoolID: &school, Title: "Kontak keuangan", SortOrder: 30, Published: true},
	}
	reader := MergeHelpArticles(rows, true)
	var ids []string
	for _, a := range reader {
		ids = append(ids, a.ID)
	}
	if len(ids) != 2 || ids[0] != "s-replace" || ids[1] != "s-own" {
		t.Fatalf("reader view = %v", ids)
	}
	admin := MergeHelpArticles(rows, false)
	if len(admin) != 6 {
		t.Fatalf("admin view has %d rows", len(admin))
	}
	for _, a := range admin {
		if a.ID == d1 && (a.ReplacedBy == nil || *a.ReplacedBy != "s-replace" || !a.IsDefault) {
			t.Errorf("default d1 = %+v", a)
		}
		if a.ID == "s-own" && a.IsDefault {
			t.Error("school article flagged default")
		}
	}
}
