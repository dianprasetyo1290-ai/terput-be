package common

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestExceptionKeys(t *testing.T) {
	tests := []struct{ got, want string }{
		{ImportFailureKey("academic", "job-1"), "import:academic:job-1"},
		{DeadLetterKey("ops", "procurement", "m-9"), "dead_letter:ops:procurement:m-9"},
	}
	for _, tt := range tests {
		if tt.got != tt.want {
			t.Errorf("got %q, want %q", tt.got, tt.want)
		}
	}
}

func TestNormalizeExceptionReport(t *testing.T) {
	ctx := WithCorrelationID(context.Background(), "corr-1")
	valid := ExceptionReport{SchoolID: "s", Kind: ExceptionKindImport, ErrorMessage: "gagal", IdempotencyKey: "k"}
	tests := []struct {
		name    string
		in      ExceptionReport
		ctx     context.Context
		wantErr bool
		check   func(t *testing.T, e ExceptionReport)
	}{
		{name: "defaults filled", in: valid, ctx: ctx, check: func(t *testing.T, e ExceptionReport) {
			if e.SourceService != "svc" || e.Severity != ExceptionSeverityNormal || e.CorrelationID != "corr-1" ||
				e.PayloadSummary == nil || e.MaxAttempts != DefaultExceptionMaxAttempts {
				t.Errorf("defaults not applied: %+v", e)
			}
		}},
		{name: "minted correlation", in: valid, ctx: context.Background(), check: func(t *testing.T, e ExceptionReport) {
			if !ValidCorrelationID(e.CorrelationID) {
				t.Errorf("correlation %q not minted", e.CorrelationID)
			}
		}},
		{name: "explicit values kept", in: ExceptionReport{SchoolID: "s", SourceService: "x", Kind: "k", Severity: ExceptionSeverityCritical,
			ErrorMessage: "e", IdempotencyKey: "i", CorrelationID: "c", MaxAttempts: 7}, ctx: ctx, check: func(t *testing.T, e ExceptionReport) {
			if e.SourceService != "x" || e.Severity != ExceptionSeverityCritical || e.CorrelationID != "c" || e.MaxAttempts != 7 {
				t.Errorf("explicit values overwritten: %+v", e)
			}
		}},
		{name: "missing school", in: ExceptionReport{Kind: "k", ErrorMessage: "e", IdempotencyKey: "i"}, ctx: ctx, wantErr: true},
		{name: "blank message", in: ExceptionReport{SchoolID: "s", Kind: "k", ErrorMessage: "  ", IdempotencyKey: "i"}, ctx: ctx, wantErr: true},
		{name: "missing key", in: ExceptionReport{SchoolID: "s", Kind: "k", ErrorMessage: "e"}, ctx: ctx, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := normalizeExceptionReport(tt.ctx, "svc", tt.in)
			if (err != nil) != tt.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.check != nil {
				tt.check(t, got)
			}
		})
	}
}

func TestExceptionReporter_Report(t *testing.T) {
	var gotHeaders http.Header
	var gotBody ExceptionReport
	status := http.StatusCreated
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/internal/exceptions" || r.Method != http.MethodPost {
			t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
		}
		gotHeaders = r.Header.Clone()
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.WriteHeader(status)
		_, _ = w.Write([]byte(`{"error":"x"}`))
	}))
	defer srv.Close()

	rep := NewExceptionReporter(srv.URL+"/", "secret", "ops")
	ctx := WithCorrelationID(context.Background(), "corr-9")
	if err := rep.ReportDeadLetter(ctx, "school-1", "procurement", "m-1", "habis percobaan", 5, nil); err != nil {
		t.Fatalf("report: %v", err)
	}
	if gotHeaders.Get("X-Internal-Key") != "secret" || gotHeaders.Get("Idempotency-Key") != "dead_letter:ops:procurement:m-1" ||
		gotHeaders.Get(CorrelationHeader) != "corr-9" {
		t.Errorf("headers = %v", gotHeaders)
	}
	if gotBody.SourceService != "ops" || gotBody.Kind != ExceptionKindDeadLetter || gotBody.Severity != ExceptionSeverityCritical ||
		gotBody.MaxAttempts != 5 || gotBody.PayloadSummary["topic"] != "procurement" {
		t.Errorf("body = %+v", gotBody)
	}

	if err := rep.ReportImportFailure(ctx, "school-1", "job-2", "baris 3 tidak valid", map[string]any{"rows_failed": 1}); err != nil {
		t.Fatalf("import report: %v", err)
	}
	if gotBody.Kind != ExceptionKindImport || gotBody.IdempotencyKey != "import:ops:job-2" || gotBody.PayloadSummary["job_id"] != "job-2" {
		t.Errorf("import body = %+v", gotBody)
	}

	status = http.StatusUnprocessableEntity
	if err := rep.ReportImportFailure(ctx, "school-1", "job-3", "x", nil); err == nil {
		t.Error("non-2xx must be an error")
	}

	if err := NewExceptionReporter("", "", "ops").Report(ctx, ExceptionReport{}); !errors.Is(err, ErrReporterNotConfigured) {
		t.Errorf("unconfigured err = %v", err)
	}
}
