package model

import "strings"

// severityAliases maps the severities other services already send
// (ops: "warning", studentlife: "low"/"medium") onto the two-level scale of
// FR-80. Before this, those reports were rejected with 422 and never reached
// the queue (BUG-113).
var severityAliases = map[string]string{
	"":         SeverityNormal,
	"low":      SeverityNormal,
	"info":     SeverityNormal,
	"medium":   SeverityNormal,
	"warning":  SeverityNormal,
	"normal":   SeverityNormal,
	"high":     SeverityCritical,
	"error":    SeverityCritical,
	"critical": SeverityCritical,
}

// NormalizeSeverity returns the canonical severity and whether the input was
// recognised.
func NormalizeSeverity(s string) (string, bool) {
	v, ok := severityAliases[strings.ToLower(strings.TrimSpace(s))]
	return v, ok
}
