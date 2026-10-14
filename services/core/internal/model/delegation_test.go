package model

import (
	"reflect"
	"testing"
)

func TestSharedApproverRoles(t *testing.T) {
	approvers := []string{"admin", "kepala_sekolah", "finance_manager", "guru"}
	tests := []struct {
		name     string
		from, to []string
		want     []string
	}{
		{"BUG-002: kepsek ke siswa ditolak", []string{"kepala_sekolah"}, []string{"siswa"}, nil},
		{"kepsek ke orang tua ditolak", []string{"kepala_sekolah"}, []string{"orang_tua"}, nil},
		{"kepsek ke guru (peran tidak setara) ditolak", []string{"kepala_sekolah"}, []string{"guru"}, nil},
		{"kepsek ke kepsek lain diterima", []string{"kepala_sekolah"}, []string{"kepala_sekolah", "guru"}, []string{"kepala_sekolah"}},
		{"multi peran: hanya irisan", []string{"finance_manager", "guru"}, []string{"guru", "finance_manager"}, []string{"finance_manager", "guru"}},
		{"peran bukan penyetuju tidak dihitung", []string{"siswa"}, []string{"siswa"}, nil},
		{"duplikat tidak berulang", []string{"admin", "admin"}, []string{"admin"}, []string{"admin"}},
		{"delegasi tanpa peran", nil, []string{"admin"}, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := SharedApproverRoles(tt.from, tt.to, approvers)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("got %v, want %v", got, tt.want)
			}
		})
	}
}
