package model

import (
	"encoding/json"
	"testing"
)

func TestValidateTemplate(t *testing.T) {
	tests := []struct {
		name, key, text string
		wantErr         bool
	}{
		{"variabel dikenal", "bill_issued", "Tagihan {{ bill_id }}: {{body}}", false},
		{"variabel tidak dikenal", "bill_issued", "Halo {{nama_ibu}}", true},
		{"kunci tak terdaftar hanya title/body", "custom_event", "{{title}} - {{body}}", false},
		{"kunci tak terdaftar, variabel lain ditolak", "custom_event", "{{url}}", true},
		{"kurung tidak seimbang", "approval", "Halo {{title", true},
		{"nama variabel tidak valid", "approval", "{{Title-1}}", true},
		{"tanpa variabel", "approval", "Teks tetap", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := ValidateTemplate(tt.key, tt.text); (err != nil) != tt.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestRenderTemplate(t *testing.T) {
	vals := TemplateValues("Tagihan baru", "Tagihan Okt Rp100.000", json.RawMessage(`{"bill_id":"b-1","amount":100000,"nested":{"x":1}}`))
	tests := []struct {
		text, want string
		ok         bool
	}{
		{"{{title}}: {{ body }} ({{bill_id}})", "Tagihan baru: Tagihan Okt Rp100.000 (b-1)", true},
		{"Nominal {{amount}}", "Nominal 100000", true},
		{"{{nested}}", "", false},
		{"{{missing}}", "", false},
	}
	for _, tt := range tests {
		got, ok := RenderTemplate(tt.text, vals)
		if ok != tt.ok || (tt.ok && got != tt.want) {
			t.Errorf("%q -> %q ok=%v, want %q ok=%v", tt.text, got, ok, tt.want, tt.ok)
		}
	}
}
