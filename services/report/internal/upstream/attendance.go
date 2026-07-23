package upstream

import (
	"context"
	"fmt"
	"time"
)

// AttendanceClient talks to attendance-service (port 8083). The exact
// endpoint shape here is given verbatim by the report-service spec:
// GET /students/{id}/attendance-summary?term_id=&academic_year_id= ->
// hadir/sakit/izin/tanpa_keterangan. Confirmed against attendance-service's
// own implementation (internal/handler/report.go there): it's reachable
// either via a normal user JWT or X-Internal-Key (see
// attendance's common.InternalOrJWTMiddleware) — when called with
// X-Internal-Key, attendance-service does NOT re-scope by school_id,
// trusting the internal caller already knows which student it's asking
// about.
type AttendanceClient struct{ c *client }

func NewAttendanceClient(baseURL, internalKey string, timeout time.Duration) *AttendanceClient {
	return &AttendanceClient{c: newClient(baseURL, internalKey, "attendance", timeout)}
}

type AttendanceSummary struct {
	Hadir           int `json:"hadir"`
	Sakit           int `json:"sakit"`
	Izin            int `json:"izin"`
	TanpaKeterangan int `json:"tanpa_keterangan"`
}

func (a *AttendanceClient) GetSummary(ctx context.Context, studentID, termID, academicYearID string) (*AttendanceSummary, error) {
	var out AttendanceSummary
	path := fmt.Sprintf("/students/%s/attendance-summary?term_id=%s&academic_year_id=%s", studentID, termID, academicYearID)
	if err := a.c.getJSON(ctx, "GetSummary", path, &out); err != nil {
		return nil, err
	}
	return &out, nil
}
