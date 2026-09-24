package handler

import "testing"

// Yadika13 item 1 asked for a per-soal essay poin bounded 1-10. The cap was
// lifted 2026-09-24 on request (soal KK kelas XII needed a wider spread so
// manual partial credit has room -- resolveAwardedPoints clamps to this
// poin). Only the floor survives: a 0-poin essay gives manual grading
// nothing to award, and publish rescales the exam to 100 anyway, so the
// absolute number never reaches a rapor -- only its ratio to other soal.
func TestEssayPoinBound(t *testing.T) {
	essay := func(poin float64) questionReq {
		return questionReq{
			AcademicYearID: "y1",
			QuestionType:   "essay",
			Content:        "Jelaskan fotosintesis.",
			Poin:           poin,
			EssayOptions:   []string{"klorofil"},
		}
	}

	// The whole point of the lift: 10 is no longer a ceiling.
	for _, poin := range []float64{1, 5, 10, 11, 25, 100} {
		if msg, ok := validateQuestionShape(essay(poin)); !ok {
			t.Errorf("essay poin %v should be accepted, got %q", poin, msg)
		}
	}
	for _, poin := range []float64{0, 0.5, -1} {
		if _, ok := validateQuestionShape(essay(poin)); ok {
			t.Errorf("essay poin %v should be rejected", poin)
		}
	}

	pg := questionReq{
		AcademicYearID: "y1",
		QuestionType:   "pilihan_ganda",
		Content:        "2+2?",
		Poin:           25,
		AnswerOptions: []answerOptionReq{
			{Content: strptr("3")}, {Content: strptr("4"), IsCorrect: true},
			{Content: strptr("5")}, {Content: strptr("6")}, {Content: strptr("7")},
		},
	}
	if msg, ok := validateQuestionShape(pg); !ok {
		t.Errorf("pilihan_ganda poin 25 should still be accepted, got %q", msg)
	}
}

// Import used to clamp essay poin to 10; with the cap gone a big poin must
// survive the spreadsheet round-trip untouched, for every type.
func TestImportKeepsEssayPoin(t *testing.T) {
	idx := map[string]int{"poin": 0, "content": 1, "opsi_jawaban": 2}

	req, msg := rowToQuestionReq("essay", []string{"50", "Jelaskan fotosintesis.", "klorofil"}, idx, "y1", "")
	if msg != "" {
		t.Fatalf("unexpected parse error: %s", msg)
	}
	if req.Poin != 50 {
		t.Errorf("essay poin 50 should be kept as-is, got %v", req.Poin)
	}
	if _, ok := validateQuestionShape(req); !ok {
		t.Error("an imported 50-poin essay must pass validation, otherwise import aborts mid-file")
	}

	// Unparseable or non-positive poin still falls back to 1 rather than
	// aborting a bulk import halfway through (rows are written as the loop goes).
	if req, _ := rowToQuestionReq("essay", []string{"abc", "Soal?", "jawab"}, idx, "y1", ""); req.Poin != 1 {
		t.Errorf("unparseable poin should default to 1, got %v", req.Poin)
	}
	if req, _ := rowToQuestionReq("pilihan_ganda", []string{"50", "2+2?", ""}, idx, "y1", ""); req.Poin != 50 {
		t.Errorf("pilihan_ganda poin should not be clamped, got %v", req.Poin)
	}
}

func strptr(s string) *string { return &s }
