package handler

import (
	"strings"
	"testing"

	"skci/exam/internal/model"
)

// TestQuestionRowRoundTrip is the runnable check that export really is the
// inverse of import: the point of the feature is that an admin can download
// a wrongly-uploaded exam's soal and upload the file again somewhere else,
// which only holds if every column survives the trip. Fails the moment
// csvColumns gains a field questionToRow forgets to write.
func TestQuestionRowRoundTrip(t *testing.T) {
	s := func(v string) *string { return &v }
	cases := map[string]*model.Question{
		"pilihan_ganda": {
			QuestionType: "pilihan_ganda", Content: "Berapakah 1/2 + 1/4?", Poin: 10,
			NoKD: s("3.1"), KontenKD: s("Memahami pecahan"),
			IndikatorPencapaian: s("Siswa dapat menjumlahkan pecahan"), LevelKognitif: s("C2"),
			AnswerOptions: []model.AnswerOption{
				{Content: s("1/2")}, {Content: s("3/4"), IsCorrect: true},
				{Content: s("1")}, {Content: s("5/4")}, {Content: s("2")},
			},
		},
		"benar_salah": {
			QuestionType: "benar_salah", Content: "-5 lebih besar dari 3", Poin: 5,
			NoKD: s("3.2"), LevelKognitif: s("C1"),
			AnswerOptions: []model.AnswerOption{
				{Content: s("Benar")}, {Content: s("Salah"), IsCorrect: true},
			},
		},
		"essay": {
			QuestionType: "essay", Content: "Apa ibukota Indonesia?", Poin: 10,
			EssayOptions: []model.EssayAnswerOption{{AnswerText: "Jakarta"}, {AnswerText: "DKI Jakarta"}},
		},
		"tebak_gambar": {
			QuestionType: "tebak_gambar", Content: "Bendera negara apakah ini?", Poin: 10,
			ImageURL:     s("https://example.com/flag.png"),
			EssayOptions: []model.EssayAnswerOption{{AnswerText: "Indonesia"}},
		},
	}

	for qType, q := range cases {
		row := questionToRow(qType, q)
		cols := csvColumns[qType]
		if len(row) != len(cols) {
			t.Fatalf("%s: wrote %d fields for %d columns %v", qType, len(row), len(cols), cols)
		}
		idx := map[string]int{}
		for i, col := range cols {
			idx[col] = i
		}
		back, msg := rowToQuestionReq(qType, row, idx, "ay", "")
		if msg != "" {
			t.Fatalf("%s: re-import rejected exported row: %s (row=%v)", qType, msg, row)
		}
		if back.Content != q.Content || back.Poin != q.Poin {
			t.Errorf("%s: content/poin lost: %q/%v vs %q/%v", qType, back.Content, back.Poin, q.Content, q.Poin)
		}
		if q.NoKD != nil && (back.NoKD == nil || *back.NoKD != *q.NoKD) {
			t.Errorf("%s: no_kd lost", qType)
		}
		switch qType {
		case "pilihan_ganda":
			if len(back.AnswerOptions) != 5 {
				t.Fatalf("pilihan_ganda: got %d options", len(back.AnswerOptions))
			}
			for i, o := range back.AnswerOptions {
				if o.IsCorrect != q.AnswerOptions[i].IsCorrect {
					t.Errorf("pilihan_ganda: option %d correctness flipped", i)
				}
				if *o.Content != *q.AnswerOptions[i].Content {
					t.Errorf("pilihan_ganda: option %d text lost", i)
				}
			}
		case "benar_salah":
			// Import writes Benar first, Salah second, and only the correct
			// one carries the flag -- so "Salah" must come back as index 1.
			if !back.AnswerOptions[1].IsCorrect || back.AnswerOptions[0].IsCorrect {
				t.Errorf("benar_salah: correct answer flipped: %v", row)
			}
		case "essay", "tebak_gambar":
			want := len(q.EssayOptions)
			if len(back.EssayOptions) != want {
				t.Errorf("%s: %d opsi jawaban, want %d", qType, len(back.EssayOptions), want)
			}
			if qType == "tebak_gambar" && (back.ImageURL == nil || *back.ImageURL != *q.ImageURL) {
				t.Errorf("tebak_gambar: image_url lost")
			}
		}
	}
}

func TestSafeFilename(t *testing.T) {
	for in, want := range map[string]string{
		"Sumatif Tengah Semester - Koding": "Sumatif_Tengah_Semester_-_Koding",
		`a"b` + "\r\n" + "/../c":           "abc",
		"":                                 "soal",
		"±§":                               "soal",
	} {
		if got := safeFilename(in); got != want {
			t.Errorf("safeFilename(%q) = %q, want %q", in, got, want)
		}
	}
	if got := safeFilename(strings.Repeat("x", 200)); len(got) != 60 {
		t.Errorf("long title not truncated: %d chars", len(got))
	}
}
