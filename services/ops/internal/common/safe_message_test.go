package common

import "testing"

func TestSafeMessage(t *testing.T) {
	cases := []struct {
		status int
		in     string
		want   string
	}{
		{400, "periode wajib diisi", "periode wajib diisi"},
		{404, "data tidak ditemukan", "data tidak ditemukan"},
		{500, "gagal memuat data", "gagal memuat data"},
		{500, "gagal memuat data: ERROR: relation \"x\" does not exist (SQLSTATE 42P01)", "gagal memuat data"},
		{500, "context deadline exceeded", genericServerMessage},
		{500, "gagal simpan: unexpected thing", "gagal simpan"},
		{400, "ERROR: invalid input syntax for type uuid: \"abc\" (SQLSTATE 22P02)", genericRequestMessage},
		{422, "data tidak valid: pq: duplicate key value violates unique constraint", "data tidak valid"},
		{500, "", genericServerMessage},
	}
	for _, c := range cases {
		if got := SafeMessage(c.status, c.in); got != c.want {
			t.Errorf("SafeMessage(%d, %q) = %q, want %q", c.status, c.in, got, c.want)
		}
	}
}
