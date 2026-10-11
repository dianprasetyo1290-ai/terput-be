package handler

import (
	"testing"

	"skci/report/internal/common"
)

func TestClassifyMapping(t *testing.T) {
	cases := map[string]string{
		"STS":                      "sts",
		"PTS":                      "sts",
		"uts":                      "sts",
		"mid_exam":                 "sts",
		"Sumatif Tengah Semester":  "sts",
		"SAS":                      "sas",
		"PAS":                      "sas",
		"UAS":                      "sas",
		"final_exam":               "sas",
		"Penilaian Akhir Semester": "sas",
		"Sumatif Akhir Jenjang":    "sas",
		"Pasar Malam":              "other", // "pas" only as a whole word
		"Hiragana":                 "other",
		"":                         "other",
	}
	for in, want := range cases {
		if got := classifyMapping(in); got != want {
			t.Errorf("classifyMapping(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestRoundsToWholeScores(t *testing.T) {
	// BUG SEKOCI 2026-09-24 item B: no tingkat keeps decimals any more,
	// Kelas XII included (it used to be the only one that did).
	for _, in := range []string{"10", "11", "X", "xi", "10 mipa", "XI-2", "12", "XII", "LULUS", ""} {
		v := in
		if !roundsToWholeScores(&v) {
			t.Errorf("roundsToWholeScores(%q) = false, want true", in)
		}
	}
	if !roundsToWholeScores(nil) {
		t.Error("an unknown grade_level must still round")
	}
	// Worked examples straight out of the bug report, plus the half-up
	// boundary it implies (>= .50 up, < .50 down).
	for in, want := range map[float64]float64{
		88.90: 89, 92.90: 93, 83.35: 83, 60.80: 61, 88: 88, 63.80: 64, 76.80: 77, 75.30: 75,
		85.84: 86, 43.5: 44, 69.5: 70, 69.49: 69, 100: 100, (69.0*0.3 + 70*0.7) / 1: 70} {
		v := in
		if got := *roundScore(&v); got != want {
			t.Errorf("roundScore(%v) = %v, want %v", in, got, want)
		}
	}
	if roundScore(nil) != nil {
		t.Error("nil score must stay nil")
	}
}

// TestFaseOf pins the Kurikulum Merdeka grade->fase mapping. grade_level is
// free text, so the parsing matters as much as the table.
func TestFaseOf(t *testing.T) {
	s := func(v string) *string { return &v }
	cases := map[string]string{
		"1": "A", "II": "A",
		"3": "B", "IV": "B",
		"5": "C", "VI": "C",
		"7": "D", "VIII": "D", "9": "D",
		"10": "E", "X": "E", "10 mipa": "E", "x-tkj-1": "E",
		"11": "F", "XI": "F", "12": "F", "XII DKV": "F",
		// Unrecognized falls back to blank, which prints as "-" like before.
		"": "", "kelas unggulan": "", "13": "",
	}
	for in, want := range cases {
		if got := faseOf(s(in)); got != want {
			t.Errorf("faseOf(%q) = %q, want %q", in, got, want)
		}
	}
	if got := faseOf(nil); got != "" {
		t.Errorf("faseOf(nil) = %q, want empty", got)
	}
}

// Real classroom names pulled from production, where 60 of 132 rows have no
// grade_level at all -- resolveFase falls back to the name for these.
func TestFaseOfClassroomNames(t *testing.T) {
	s := func(v string) *string { return &v }
	cases := map[string]string{
		"2-A": "A", "3 A": "B", "VII 1": "D", "VIII A": "D", "IX": "D",
		"X TKJ 1": "E", "X MPLB 2": "E", "X. ANM": "E", "X.TKRO.1": "E",
		"XI AKL 1": "F", "XI.MP.2": "F", "XII AKL": "F", "XII. ANM": "F",
		"10 TKJ 1": "E", "12 MIPA": "F",
		// No level to read: still blank, still prints "-".
		"TKR": "", "ips 2": "", "IT 1": "", "kelas ipa 1 1": "",
	}
	for in, want := range cases {
		if got := faseOf(s(in)); got != want {
			t.Errorf("faseOf(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestSiswaSeesFinalizedOwnRaporOnly(t *testing.T) {
	siswa := &common.Claims{UserID: "s1", ActiveRole: "siswa"}
	if authorizeReportCardAccess(siswa, "x", "s1", "draft") {
		t.Fatal("siswa must not see a draft rapor")
	}
	if !authorizeReportCardAccess(siswa, "x", "s1", "finalized") {
		t.Fatal("siswa must see own finalized rapor")
	}
	if authorizeReportCardAccess(siswa, "x", "s2", "finalized") {
		t.Fatal("siswa must not see another student's rapor")
	}
	guru := &common.Claims{UserID: "g", SchoolID: "x", ActiveRole: "guru"}
	if !authorizeReportCardAccess(guru, "x", "s1", "draft") {
		t.Fatal("staff still sees drafts")
	}
}
