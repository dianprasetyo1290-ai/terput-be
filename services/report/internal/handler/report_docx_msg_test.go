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
