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
// V21: a school template may carry BOTH {{kemajuan}} and {{kemajuan2}}, so
// each capaian placeholder must hold only its own box -- merging kolom 1+2
// into the first one printed kolom 2 twice.
func TestMergeDataKeepsCapaianBoxesApart(t *testing.T) {
	p := func(s string) *string { return &s }
	rc := &model.ReportCard{Subjects: []model.ReportCardSubject{{
		MidNote: p("STS kolom 1"), MidNote2: p("STS kolom 2"),
		FinalNote: p("SAS kolom 1"), FinalNote2: p("SAS kolom 2"),
	}}}
	item := buildMergeData(rc).Loops["mapel"][0]
	for field, want := range map[string]string{
		"kemajuan": "STS kolom 1", "kemajuan2": "STS kolom 2",
		"capaian": "SAS kolom 1", "capaian2": "SAS kolom 2",
	} {
		if item[field] != want {
			t.Errorf("%s = %q, want %q", field, item[field], want)
		}
	}
}
