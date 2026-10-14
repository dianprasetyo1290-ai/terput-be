package model

import (
	"reflect"
	"testing"
)

func TestResolveChannels(t *testing.T) {
	waOn := map[string]bool{ChannelWhatsApp: true}
	none := map[string]bool{}
	tests := []struct {
		name      string
		requested []string
		typ       string
		mandatory bool
		enabled   map[string]bool
		want      []string
	}{
		{"BUG-029: tagihan in_app saja + WA aktif -> WA dicoba", []string{"in_app"}, "receipt", false, waOn, []string{"in_app", "whatsapp"}},
		{"BUG-090: tagihan wajib, email off -> tetap email+WA", []string{"in_app"}, "bill_issued", true, waOn, []string{"in_app", "email", "whatsapp"}},
		{"diminta email tapi pengguna mematikan -> dibuang", []string{"in_app", "email"}, "approval", false, none, []string{"in_app"}},
		{"tanpa permintaan, default hanya in_app", nil, "approval", false, none, []string{"in_app"}},
		{"push diteruskan apa adanya", []string{"push"}, "approval", false, none, []string{"in_app", "push"}},
		{"kampanye tidak diubah", []string{"email"}, "campaign", true, waOn, []string{"email"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ResolveChannels(tt.requested, tt.typ, tt.mandatory, tt.enabled); !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("got %v, want %v", got, tt.want)
			}
		})
	}
}

func TestIsMandatoryType(t *testing.T) {
	for typ, want := range map[string]bool{"bill_issued": true, "invoice_overdue": true, "security_login_failed": true,
		"receipt": false, "approval": false, "campaign": false, "billing": false} {
		if got := IsMandatoryType(typ); got != want {
			t.Errorf("%s: got %v want %v", typ, got, want)
		}
	}
}
