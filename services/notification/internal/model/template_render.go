package model

import (
	"encoding/json"
	"fmt"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"
)

// placeholderRe matches {{ name }} placeholders (BUG-088).
var placeholderRe = regexp.MustCompile(`\{\{\s*([^{}]*?)\s*\}\}`)

var varNameRe = regexp.MustCompile(`^[a-z][a-z0-9_]*$`)

// CommonTemplateVars are available in every event template: the caller's own
// title and body, so an admin can wrap or rephrase them.
var CommonTemplateVars = []string{"title", "body"}

// EventTemplateVars lists, per event type (the template key), the data fields
// the sending service puts in the notification and a template may use. A key
// not listed here only gets CommonTemplateVars.
var EventTemplateVars = map[string][]string{
	"bill_issued":           {"bill_id", "url"},
	"receipt":               {"url"},
	"approval":              {"approval_id", "source_service", "source_id", "url", "correlation_id"},
	"integration_exception": {"exception_id", "correlation_id", "url"},
}

// AllowedTemplateVars returns the variables a template for key may use.
func AllowedTemplateVars(key string) []string {
	out := append(slices.Clone(CommonTemplateVars), EventTemplateVars[key]...)
	sort.Strings(out)
	return out
}

// TemplateVars extracts the names used in a template text, sorted, unique.
func TemplateVars(text string) []string {
	seen := map[string]bool{}
	for _, m := range placeholderRe.FindAllStringSubmatch(text, -1) {
		seen[m[1]] = true
	}
	out := make([]string, 0, len(seen))
	for v := range seen {
		out = append(out, v)
	}
	sort.Strings(out)
	return out
}

// ValidateTemplate rejects malformed or unknown placeholders for key.
func ValidateTemplate(key string, texts ...string) error {
	allowed := AllowedTemplateVars(key)
	for _, text := range texts {
		if strings.Count(text, "{{") != strings.Count(text, "}}") {
			return fmt.Errorf("penanda variabel tidak seimbang: setiap {{ harus ditutup }}")
		}
		for _, v := range TemplateVars(text) {
			if !varNameRe.MatchString(v) {
				return fmt.Errorf("nama variabel {{%s}} tidak valid", v)
			}
			if !slices.Contains(allowed, v) {
				return fmt.Errorf("variabel {{%s}} tidak dikenal untuk %q; yang tersedia: %s", v, key, strings.Join(allowed, ", "))
			}
		}
	}
	return nil
}

// TemplateValues builds the render values from a notification's data object
// (scalar top-level fields) plus its title and body.
func TemplateValues(title, body string, data json.RawMessage) map[string]string {
	vals := map[string]string{"title": title, "body": body}
	var obj map[string]any
	if len(data) > 0 && json.Unmarshal(data, &obj) == nil {
		for k, v := range obj {
			switch x := v.(type) {
			case string:
				vals[k] = x
			case float64:
				vals[k] = strconv.FormatFloat(x, 'f', -1, 64)
			case bool:
				vals[k] = strconv.FormatBool(x)
			}
		}
	}
	return vals
}

// RenderTemplate substitutes every placeholder. ok is false when a value is
// missing, so the caller can fall back to the original text instead of
// sending a message with holes.
func RenderTemplate(text string, vals map[string]string) (string, bool) {
	ok := true
	out := placeholderRe.ReplaceAllStringFunc(text, func(m string) string {
		name := placeholderRe.FindStringSubmatch(m)[1]
		v, found := vals[name]
		if !found {
			ok = false
		}
		return v
	})
	return out, ok
}
