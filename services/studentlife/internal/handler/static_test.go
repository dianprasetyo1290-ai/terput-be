package handler

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

func fmtAny(v any) string { return fmt.Sprintf("%v", v) }

// NFR-OPS-03: application logs never mention health content fields.
func TestNoHealthPlaintextInLogs(t *testing.T) {
	logCall := regexp.MustCompile(`(log\.|slog\.)`)
	sensitive := regexp.MustCompile(`(?i)allerg|complaint|diagnos`)
	_ = filepath.Walk("..", func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		raw, _ := os.ReadFile(path)
		for i, line := range strings.Split(string(raw), "\n") {
			if logCall.MatchString(line) && sensitive.MatchString(line) {
				t.Errorf("%s:%d logs health content: %s", path, i+1, strings.TrimSpace(line))
			}
		}
		return nil
	})
}

// NFR-OPS-03 "tidak muncul di ekspor umum": the general exports of
// identity, report and academic never reach studentlife.
func TestGeneralExportsDoNotTouchStudentlife(t *testing.T) {
	for _, svc := range []string{"identity", "report", "academic"} {
		root := filepath.Join("..", "..", "..", svc)
		if _, err := os.Stat(root); err != nil {
			t.Fatalf("service %s not found: %v", svc, err)
		}
		_ = filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
			if err != nil || info.IsDir() || !strings.HasSuffix(path, ".go") {
				return nil
			}
			raw, _ := os.ReadFile(path)
			s := strings.ToLower(string(raw))
			if strings.Contains(s, "studentlife") || strings.Contains(s, "health_profiles") || strings.Contains(s, "clinic_visits") {
				t.Errorf("%s references studentlife data", path)
			}
			return nil
		})
	}
}
