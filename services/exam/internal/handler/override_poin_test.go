package handler

import "testing"

func pf(v float64) *float64 { return &v }

// Yadika13 follow-up: a guru grading an essay by hand must be able to award
// 7 of 8, not just all-or-nothing. points_awarded is what the attempt score
// sums, so a clamp slip here writes a nilai the guru never typed.
func TestResolveAwardedPoints(t *testing.T) {
	const max = 8

	// No `points` field: the old Tandai Benar / Tandai Salah buttons.
	if p, ok, err := resolveAwardedPoints(overrideAnswerReq{IsCorrect: true}, max); p != 8 || !ok || err != nil {
		t.Errorf("Tandai Benar: want 8/true/nil, got %v/%v/%v", p, ok, err)
	}
	if p, ok, err := resolveAwardedPoints(overrideAnswerReq{IsCorrect: false}, max); p != 0 || ok || err != nil {
		t.Errorf("Tandai Salah: want 0/false/nil, got %v/%v/%v", p, ok, err)
	}

	// Partial credit moves the nilai but is not "Jawaban Benar".
	for _, v := range []float64{0, 4, 5, 6, 7} {
		p, ok, err := resolveAwardedPoints(overrideAnswerReq{Points: pf(v)}, max)
		if p != v || ok || err != nil {
			t.Errorf("poin %v: want %v/false/nil, got %v/%v/%v", v, v, p, ok, err)
		}
	}
	// Full marks typed into the box still counts as benar.
	if p, ok, err := resolveAwardedPoints(overrideAnswerReq{Points: pf(8)}, max); p != 8 || !ok || err != nil {
		t.Errorf("poin 8: want 8/true/nil, got %v/%v/%v", p, ok, err)
	}

	// `points` wins over a stale is_correct the client also sent.
	if p, ok, _ := resolveAwardedPoints(overrideAnswerReq{IsCorrect: true, Points: pf(5)}, max); p != 5 || ok {
		t.Errorf("points must win over is_correct, got %v/%v", p, ok)
	}

	// Out of range is rejected, never silently clamped -- a guru who types
	// 80 into an 8-poin box meant something, and must be told.
	for _, bad := range []float64{-1, 8.5, 80} {
		if _, _, err := resolveAwardedPoints(overrideAnswerReq{Points: pf(bad)}, max); err == nil {
			t.Errorf("poin %v must be rejected", bad)
		}
	}

	// Rescaled soal carry fractional poin (exam totals 100), so decimals
	// inside the range must pass.
	if p, ok, err := resolveAwardedPoints(overrideAnswerReq{Points: pf(3.75)}, 4.5); p != 3.75 || ok || err != nil {
		t.Errorf("fractional max: want 3.75/false/nil, got %v/%v/%v", p, ok, err)
	}
}
