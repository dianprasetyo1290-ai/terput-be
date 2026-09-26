package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"skci/report/internal/model"
	"skci/report/internal/upstream"
)

func f(v float64) *float64 { return &v }

func show(v *float64) interface{} {
	if v == nil {
		return "nil"
	}
	return *v
}

// computeSubjectScore is now shared by GenerateReportCards and
// refreshExamScores (Yadika13 items 4 & 5). A change here silently moves
// every Nilai Akhir on every rapor, so pin the two shapes that matter:
// all buckets present, and a missing bucket renormalizing rather than
// counting as zero.
func TestComputeSubjectScore(t *testing.T) {
	w := model.WeightSettings{
		AkhirFormatifWeight: 30, AkhirStsWeight: 30, AkhirSasWeight: 40,
		SementaraFormatifWeight: 30, SementaraPraktikWeight: 40, SementaraStsWeight: 30,
	}

	// 0.3*80 + 0.3*70 + 0.4*90 = 81
	if got := computeSubjectScore("semester", w, f(80), nil, f(70), f(90)); got == nil || *got != 81 {
		t.Errorf("semester with all buckets: want 81, got %v", show(got))
	}

	// No SAS yet: the remaining 30/30 renormalize to 50/50, so 80 and 70
	// average to 75 -- NOT (0.3*80 + 0.3*70) = 45.
	if got := computeSubjectScore("semester", w, f(80), nil, f(70), nil); got == nil || *got != 75 {
		t.Errorf("semester without SAS must renormalize: want 75, got %v", show(got))
	}

	// Bayangan uses praktik and ignores SAS entirely.
	// 0.3*80 + 0.4*90 + 0.3*60 = 78; the SAS argument is ignored entirely.
	if got := computeSubjectScore("bayangan", w, f(80), f(90), f(60), f(10)); got == nil || *got != 78 {
		t.Errorf("bayangan: want 78, got %v", show(got))
	}
}

// sameScore decides whether a refreshed exam score is worth a write; a
// too-strict comparison would rewrite every row on every page load, and a
// too-loose one would keep showing 70 after a koreksi made it 80.
func TestSameScore(t *testing.T) {
	if !sameScore(nil, nil) {
		t.Error("two missing scores are the same")
	}
	if sameScore(f(70), nil) || sameScore(nil, f(70)) {
		t.Error("missing vs present must differ")
	}
	if sameScore(f(70), f(80)) {
		t.Error("70 and 80 must differ -- this is the koreksi case")
	}
	// An average of 3 exams re-derived from the same numbers lands a few
	// ULPs off; that must not count as a change.
	if !sameScore(f((70+80+90)/3.0), f(80.0000000001)) {
		t.Error("float noise must not count as a change")
	}
}

func TestMaybeRound(t *testing.T) {
	if got := maybeRound(79.5, true); got == nil || *got != 80 {
		t.Errorf("whole-score grades round half up: want 80, got %v", got)
	}
	if got := maybeRound(79.5, false); got == nil || *got != 79.5 {
		t.Errorf("other grades keep the decimal: want 79.5, got %v", got)
	}
}

// BUG SEKOCI 2026-09-27: Penilaian Formatif showed a lower number than
// Pantau Ujian Langsung because examBucketScores averaged every exam that
// shared a subject + bucket. Real prod data from XI.ANM / Matematika: a
// "penjas" exam left tagged as Matematika (Alfito 30) got blended with the
// real STS (78) into 54 -- exactly what the guru screenshotted.
//
// Exams are served newest-first, the way exam-service's ORDER BY
// created_at DESC does, so the newest sitting must be the one that wins.
func TestExamBucketScoresTakesNewestExamNotAverage(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		write := func(v any) { b, _ := json.Marshal(v); w.Write([]byte(`{"data":` + string(b) + `}`)) }

		switch {
		case strings.HasPrefix(r.URL.Path, "/exam-types"):
			write([]upstream.ExamType{{ID: "t-sts", Name: "STS I", MappingRapor: "sts"}})
		case r.URL.Path == "/exams":
			// Newest first, as exam-service orders them.
			write([]upstream.Exam{
				{ID: "susulan", SubjectID: "mtk", ExamTypeID: "t-sts"},
				{ID: "psts", SubjectID: "mtk", ExamTypeID: "t-sts"},
				{ID: "penjas", SubjectID: "mtk", ExamTypeID: "t-sts"},
				{ID: "ipa", SubjectID: "ipa", ExamTypeID: "t-sts"},
			})
		case strings.HasSuffix(r.URL.Path, "/scores"):
			id := strings.Split(strings.Trim(r.URL.Path, "/"), "/")[1]
			n := func(f float64) *float64 { return &f }
			scores := map[string][]upstream.Score{
				"penjas":  {{StudentID: "alfito", Score: n(30)}, {StudentID: "dewi", Score: n(45)}},
				"psts":    {{StudentID: "alfito", Score: n(78)}, {StudentID: "dewi", Score: n(49)}},
				"susulan": {{StudentID: "dewi", Score: n(60)}},
				"ipa":     {{StudentID: "alfito", Score: n(99)}},
			}[id]
			b, _ := json.Marshal(scores)
			w.Write([]byte(`{"data":{"scores":` + string(b) + `}}`))
		default:
			t.Errorf("unexpected request %s", r.URL)
		}
	}))
	defer srv.Close()

	h := New(&Deps{Exam: upstream.NewExamClient(srv.URL, "k", 5*time.Second)})
	got := h.examBucketScores(context.Background(), "c1", "term1", "ay1", "mtk")

	for student, want := range map[string]float64{
		"alfito": 78, // NOT (30+78)/2 = 54, the number in the bug report
		"dewi":   60, // the susulan supersedes both earlier sittings
	} {
		if v, ok := got["sts"][student]; !ok || v != want {
			t.Errorf("%s: got %v (present=%v), want %v -- scores are being blended again", student, v, ok, want)
		}
	}
	if _, ok := got["sts"]["alfito"]; ok && got["sts"]["alfito"] == 99 {
		t.Error("another subject's exam leaked into this subject's bucket")
	}
}
