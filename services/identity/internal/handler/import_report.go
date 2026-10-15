package handler

import (
	"context"
	"log"
	"net/http"
	"strings"

	"skci/identity/internal/common"
)

// Import failures that leave data half-done or unsaved (system errors, not
// rows the user must fix) go to core's exception queue (kind import, CORE
// handoff §9), so the operator sees them even when the uploader closes the
// page. Retrying the same file of the same kind in the same unit bumps one
// queue item: key import:identity:<kind>:<school>:<file>.

// importJobID identifies one import "job" for the exception queue.
func importJobID(kind, schoolID, fileName string) string {
	name := strings.TrimSpace(fileName)
	if name == "" {
		name = "-"
	}
	return kind + ":" + schoolID + ":" + name
}

// reportImportFailure is best effort: a reporter that is not configured or
// a core that is down is only logged; the HTTP answer already carries the
// failure to the user.
func (a *App) reportImportFailure(r *http.Request, kind, schoolID, fileName, message string, summary map[string]any) {
	if a.Exceptions == nil || schoolID == "" {
		return
	}
	if summary == nil {
		summary = map[string]any{}
	}
	summary["kind"], summary["file"] = kind, fileName
	jobID := importJobID(kind, schoolID, fileName)
	ctx := context.WithoutCancel(r.Context())
	go func() {
		if err := a.Exceptions.ReportImportFailure(ctx, schoolID, jobID, message, summary); err != nil {
			log.Printf("import %s: report exception: %v", jobID, err)
		}
	}()
}

// StampCorrelationHeader copies the id CorrelationMiddleware put in the
// context onto the request header when the caller sent none, so handlers
// that read X-Correlation-ID (security audit) always stamp one.
func StampCorrelationHeader(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get(common.CorrelationHeader) == "" {
			if id := common.CorrelationID(r.Context()); id != "" {
				r.Header.Set(common.CorrelationHeader, id)
			}
		}
		next.ServeHTTP(w, r)
	})
}
