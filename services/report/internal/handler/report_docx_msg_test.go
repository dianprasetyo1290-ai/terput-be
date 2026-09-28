package handler

import (
	"strings"
	"testing"

	"skci/report/internal/model"
)

// Report Sekoci V15 item 2: the guru's error must name the Import Template
// slot that is actually empty. A "semester" rapor needs akhir_semester, and
// pointing at the tengah_semester template the admin already uploaded is what
// made the bug look like the system ignoring its own upload.
func TestMissingTemplateMsgNamesTheSlot(t *testing.T) {
	for _, tc := range []struct{ reportType, want string }{
		{"semester", "akhir semester"},
		{"bayangan", "tengah semester"},
	} {
		msg := missingTemplateMsg(&model.ReportCard{ReportType: tc.reportType})
		if !strings.Contains(strings.ToLower(msg), tc.want) {
			t.Errorf("report_type %q: msg %q does not name %q", tc.reportType, msg, tc.want)
		}
	}
}

// Kolom 2 must reach the printed rapor: no school's uploaded .docx mentions
// capaian2, so both boxes have to merge into the one placeholder they do have.
func TestJoinNotesMergesBothCapaianBoxes(t *testing.T) {
	p := func(s string) *string { return &s }
	for _, tc := range []struct {
		a, b *string
		want string
	}{
		{p("Sudah mampu membaca."), p("Perlu latihan menulis."), "Sudah mampu membaca. Perlu latihan menulis."},
		{p("Hanya kolom 1."), nil, "Hanya kolom 1."},
		{nil, p("Hanya kolom 2."), "Hanya kolom 2."},
		{p("  "), nil, ""},
	} {
		if got := joinNotes(tc.a, tc.b); got != tc.want {
			t.Errorf("joinNotes = %q, want %q", got, tc.want)
		}
	}
}
