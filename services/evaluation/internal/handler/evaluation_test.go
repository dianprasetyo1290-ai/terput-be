package handler

import (
	"testing"

	"skci/evaluation/internal/common"
	"skci/evaluation/internal/model"
)

// TestAuthorizeEvaluation_GuruCannotRateAtasan is the runnable check for the
// client's hierarchy rule (2026-08, translated):
//   - guru CANNOT rate an atasan (kepala sekolah / waka kurikulum / waka
//     kesiswaan) under any evaluator_type, including the old upward-feedback
//     guru_to_* types.
//   - guru CAN rate a fellow guru via the peer track -- INCLUDING a kepsek/
//     waka who also teaches as guru, as long as they're rated in that guru
//     capacity (i.e. present in the period's peer pool like any other guru),
//     never in their kepsek/waka capacity.
//
// No DB needed -- authorizeEvaluation is a pure function of (claims, period,
// request).
func TestAuthorizeEvaluation_GuruCannotRateAtasan(t *testing.T) {
	kepsekID := "kepsek-1"
	wakaKurikulumID := "waka-kurikulum-1"
	guruID := "guru-1"
	kepsekWhoTeachesID := "kepsek-who-teaches-1" // holds kepala_sekolah jabatan AND is a subject teacher

	p := &model.Period{
		ID:                 "period-1",
		HeadmasterUserID:   &kepsekID,
		CurriculumVPUserID: &wakaKurikulumID,
		TargetTeacherIDs:   []string{guruID},
		// kepsekWhoTeachesID is in the peer pool -- i.e. admin has configured
		// them as a rateable guru for this period, same as any other teacher.
		PeerEvaluateeIDs: []string{kepsekWhoTeachesID},
		PeerEvaluatorIDs: []string{guruID},
	}

	guruClaims := &common.Claims{UserID: guruID, Roles: []string{"guru"}}

	// Negative cases: guru attempting to rate an atasan, every angle.
	negativeCases := []struct {
		name string
		req  submitEvaluationRequest
	}{
		{"guru submits kepala_sekolah-category eval", submitEvaluationRequest{EvaluatorType: model.EvalTypeKepalaSekolah, TargetTeacherID: guruID}},
		{"guru submits waka_kurikulum-category eval", submitEvaluationRequest{EvaluatorType: model.EvalTypeWakaKurikulum, TargetTeacherID: guruID}},
		{"guru submits upward guru_to_kepsek", submitEvaluationRequest{EvaluatorType: model.EvalTypeGuruToKepsek, TargetTeacherID: kepsekID}},
		{"guru submits upward guru_to_waka_kurikulum", submitEvaluationRequest{EvaluatorType: model.EvalTypeGuruToWakaKurikulum, TargetTeacherID: wakaKurikulumID}},
		{"guru submits upward guru_to_waka_kesiswaan", submitEvaluationRequest{EvaluatorType: model.EvalTypeGuruToWakaKesiswaan, TargetTeacherID: "waka-kesiswaan-1"}},
	}
	for _, tc := range negativeCases {
		t.Run(tc.name, func(t *testing.T) {
			if err := authorizeEvaluation(guruClaims, p, tc.req); err == nil {
				t.Fatalf("expected guru-rates-atasan to be rejected, got nil error")
			}
		})
	}

	// Positive case: guru peer-rates the kepsek-who-also-teaches, in their
	// guru/peer capacity (via the ordinary peer pool) -- must be allowed.
	t.Run("guru peer-rates kepsek-who-teaches as guru", func(t *testing.T) {
		req := submitEvaluationRequest{EvaluatorType: model.EvalTypePeer, TargetTeacherID: kepsekWhoTeachesID}
		if err := authorizeEvaluation(guruClaims, p, req); err != nil {
			t.Fatalf("expected peer-rating a kepsek-who-teaches (in guru capacity) to be allowed, got error: %v", err)
		}
	})
}
