package handler

import (
	"context"
	"os"
	"testing"

	"skci/evaluation/internal/client"
	"skci/evaluation/internal/model"
	"skci/evaluation/internal/store"
)

// TestMyTasks_SelfPeerAndLeadership is the one runnable check for the
// guru-facing "all applicable assessment types" surface: GET /periods/{id}/my-tasks
// (backed by TaskStore.ListForEvaluator, called from IsiPenilaianGuruView.vue)
// must return every task where the caller is the evaluator, across all
// evaluator_types -- not just "self".
//
// Real direction confirmed from migrations/000001_init.up.sql line 39
// ("J: guru in-scope for this period (evaluated by kepsek/waka/self/siswa)"),
// store/default_forms.go item content (e.g. A-1 "Kehadiran ... dalam semua
// kewajiban sekolah" is clearly rating a TEACHER, not a principal), and
// PeriodeFormView.vue (kepsek/waka fields are single-select "who holds this
// role", not a target list) -- kepala_sekolah/waka_kurikulum/waka_kesiswaan
// are EVALUATOR categories (the role-holder rates each target guru), the
// reverse of "guru rates the principal". A guru who happens to also hold
// one/more of those jabatan for a period (common in small schools) gets
// those tasks too, alongside self + peer, via the same MyTasks call --
// already wired end-to-end, this test just proves it.
//
// Scenario: guruMain is the period's sole target teacher (so each
// leadership role yields exactly 1 task) AND is designated as all 3
// leadership roles AND is assigned N peer-evaluatees. Since guruMain holds
// every leadership role AND is the only target teacher, he'd otherwise be
// asked to fill out "kepsek/waka rates guruMain" and its upward-feedback
// mirror "guruMain rates kepsek/waka" targeting himself both ways -- the
// exact "guru struktural dipaksa menilai dirinya sendiri" bug from the V6
// gap report. upsert()'s target==evaluator guard (period.go) blocks all 6
// of those (3 leadership + 3 upward), so they must NOT appear here; only
// self (which IS allowed to target the caller) and peer remain. Expected
// MyTasks count for guruMain = 1 (self) + N (peer), with no self/peer
// duplication (peer evaluatees are distinct guru, not guruMain).
func TestMyTasks_SelfPeerAndLeadership(t *testing.T) {
	dbURL := os.Getenv("EVALUATION_TEST_DATABASE_URL")
	if dbURL == "" {
		dbURL = "postgres://sekoci:change-me@localhost:5480/skci_evaluation?sslmode=disable"
	}
	ctx := context.Background()
	pool, err := store.NewPool(ctx, dbURL)
	if err != nil {
		t.Skipf("skipping: dev postgres not reachable at %s: %v", dbURL, err)
	}
	// Registered before the row-cleanup below so it runs LAST (t.Cleanup is
	// LIFO) -- otherwise the pool closes before the delete runs and the test
	// rows leak into the dev DB.
	t.Cleanup(pool.Close)

	newID := func() string {
		var id string
		if err := pool.QueryRow(ctx, "SELECT gen_random_uuid()").Scan(&id); err != nil {
			t.Fatalf("gen_random_uuid: %v", err)
		}
		return id
	}

	schoolID := newID()
	academicYearID := newID()
	guruMain := newID()
	const nPeers = 2
	peers := []string{newID(), newID()}

	h := New(nil, store.NewPeriodStore(pool), store.NewFormStore(pool), store.NewTaskStore(pool), &client.Clients{})

	p := &model.Period{
		SchoolID: schoolID, AcademicYearID: academicYearID, Title: "test period",
		PublishStartDate: "2026-01-01", PublishEndDate: "2026-01-31",
		HeadmasterUserID: &guruMain, CurriculumVPUserID: &guruMain, StudentAffairsVPUserID: &guruMain,
		TargetTeacherIDs: []string{guruMain}, // sole target -> exactly 1 task per leadership role
		PeerEvaluateeIDs: peers, PeerEvaluatorIDs: []string{guruMain},
	}
	if err := h.Periods.Create(ctx, p); err != nil {
		t.Fatalf("create period: %v", err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(ctx, "DELETE FROM teacher_evaluation_periods WHERE id=$1", p.ID) // cascades tasks/answers
		_, _ = pool.Exec(ctx, "DELETE FROM teacher_evaluation_forms WHERE school_id=$1", schoolID) // cascades items
	})

	if err := h.regenerateTasks(ctx, p); err != nil {
		t.Fatalf("regenerateTasks: %v", err)
	}

	tasks, err := h.Tasks.ListForEvaluator(ctx, p.ID, guruMain)
	if err != nil {
		t.Fatalf("ListForEvaluator: %v", err)
	}

	// guruMain also holds all 3 leadership roles AND is the sole target
	// teacher, so every leadership-rates-you and upward-feedback task would
	// target himself -- all 6 must be suppressed by the self-assignment
	// guard, leaving only self + peer. See
	// TestUpwardEvalTasks_GeneratedPerGuruAndAdditive for the realistic case
	// of a guru rating a distinct leader (where upward tasks DO appear).
	want := 1 + nPeers
	if len(tasks) != want {
		t.Fatalf("got %d tasks for guruMain, want %d (self=1 + peer=%d, leadership+upward all self-blocked): %+v", len(tasks), want, nPeers, tasks)
	}

	counts := map[string]int{}
	peerTargets := map[string]bool{}
	for _, tk := range tasks {
		counts[tk.EvaluatorType]++
		if tk.EvaluatorType == model.EvalTypePeer {
			peerTargets[tk.TargetTeacherUserID] = true
			if tk.TargetTeacherUserID == guruMain {
				t.Fatalf("peer task must not target the evaluator themselves (no self/peer duplication), got task %+v", tk)
			}
		}
	}
	if counts[model.EvalTypeSelf] != 1 {
		t.Errorf("self tasks = %d, want 1", counts[model.EvalTypeSelf])
	}
	if counts[model.EvalTypePeer] != nPeers {
		t.Errorf("peer tasks = %d, want %d", counts[model.EvalTypePeer], nPeers)
	}
	// Self-targeting leadership tasks (guruMain would be asked to evaluate
	// himself as kepsek/waka rating a target teacher) must be suppressed.
	if counts[model.EvalTypeKepalaSekolah] != 0 || counts[model.EvalTypeWakaKurikulum] != 0 || counts[model.EvalTypeWakaKesiswaan] != 0 {
		t.Errorf("leadership tasks = kepsek:%d waka_kurikulum:%d waka_kesiswaan:%d, want 0 each (self-assignment must be blocked)",
			counts[model.EvalTypeKepalaSekolah], counts[model.EvalTypeWakaKurikulum], counts[model.EvalTypeWakaKesiswaan])
	}
	// Self-targeting upward-feedback tasks (guruMain rating himself as
	// kepsek/waka) must likewise be suppressed -- the "guru struktural
	// dipaksa menilai dirinya sendiri" bug this guard exists to prevent.
	if counts[model.EvalTypeGuruToKepsek] != 0 || counts[model.EvalTypeGuruToWakaKurikulum] != 0 || counts[model.EvalTypeGuruToWakaKesiswaan] != 0 {
		t.Errorf("upward tasks = guru_to_kepsek:%d guru_to_waka_kurikulum:%d guru_to_waka_kesiswaan:%d, want 0 each (self-assignment must be blocked)",
			counts[model.EvalTypeGuruToKepsek], counts[model.EvalTypeGuruToWakaKurikulum], counts[model.EvalTypeGuruToWakaKesiswaan])
	}
	if len(peerTargets) != nPeers {
		t.Errorf("expected %d distinct peer targets, got %d", nPeers, len(peerTargets))
	}
}

// TestUpwardEvalTasks_GeneratedPerGuruAndAdditive is the runnable check for
// the new upward-feedback track (guru rates kepala sekolah / waka kurikulum
// / waka kesiswaan -- the opposite direction of the existing
// kepsek/waka-rates-guru categories, NOT blended into the weighted teacher
// score, see model.EvalTypeGuruToKepsek doc comment).
//
// Scenario: 2 guru in TargetTeacherIDs, distinct leader IDs (kepsek != waka
// kurikulum != waka kesiswaan != either guru), plus the same self/peer setup
// as TestMyTasks_SelfPeerAndLeadership. Expected per guru: self=1,
// peer=nPeers (only for the guru actually assigned as peer evaluator),
// leadership-rates-you=0 (neither guru holds a leadership role here), and
// upward=3 (one guru_to_kepsek/guru_to_waka_kurikulum/guru_to_waka_kesiswaan
// task each, targeting the real current role-holder). This proves both:
//  1. task generation for the 3 upward types happens per guru, targeting the
//     actual current kepsek/waka-kurikulum/waka-kesiswaan holder, and
//  2. it's additive -- the pre-existing self+peer+leadership-rates-you count
//     for each guru is unchanged by adding the upward track.
func TestUpwardEvalTasks_GeneratedPerGuruAndAdditive(t *testing.T) {
	dbURL := os.Getenv("EVALUATION_TEST_DATABASE_URL")
	if dbURL == "" {
		dbURL = "postgres://sekoci:change-me@localhost:5480/skci_evaluation?sslmode=disable"
	}
	ctx := context.Background()
	pool, err := store.NewPool(ctx, dbURL)
	if err != nil {
		t.Skipf("skipping: dev postgres not reachable at %s: %v", dbURL, err)
	}
	t.Cleanup(pool.Close)

	newID := func() string {
		var id string
		if err := pool.QueryRow(ctx, "SELECT gen_random_uuid()").Scan(&id); err != nil {
			t.Fatalf("gen_random_uuid: %v", err)
		}
		return id
	}

	schoolID := newID()
	academicYearID := newID()
	guru1 := newID()
	guru2 := newID()
	kepsek := newID()
	wakaKurikulum := newID()
	wakaKesiswaan := newID()

	h := New(nil, store.NewPeriodStore(pool), store.NewFormStore(pool), store.NewTaskStore(pool), &client.Clients{})

	p := &model.Period{
		SchoolID: schoolID, AcademicYearID: academicYearID, Title: "test period upward eval",
		PublishStartDate: "2026-01-01", PublishEndDate: "2026-01-31",
		HeadmasterUserID: &kepsek, CurriculumVPUserID: &wakaKurikulum, StudentAffairsVPUserID: &wakaKesiswaan,
		TargetTeacherIDs: []string{guru1, guru2},
		PeerEvaluateeIDs: []string{guru1}, PeerEvaluatorIDs: []string{guru2},
	}
	if err := h.Periods.Create(ctx, p); err != nil {
		t.Fatalf("create period: %v", err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(ctx, "DELETE FROM teacher_evaluation_periods WHERE id=$1", p.ID) // cascades tasks/answers
		_, _ = pool.Exec(ctx, "DELETE FROM teacher_evaluation_forms WHERE school_id=$1", schoolID) // cascades items
	})

	if err := h.regenerateTasks(ctx, p); err != nil {
		t.Fatalf("regenerateTasks: %v", err)
	}

	for _, guruID := range []string{guru1, guru2} {
		tasks, err := h.Tasks.ListForEvaluator(ctx, p.ID, guruID)
		if err != nil {
			t.Fatalf("ListForEvaluator(%s): %v", guruID, err)
		}

		counts := map[string]int{}
		upwardTargets := map[string]string{} // evaluator_type -> target_teacher_user_id
		for _, tk := range tasks {
			counts[tk.EvaluatorType]++
			if model.IsUpwardEvalType(tk.EvaluatorType) {
				upwardTargets[tk.EvaluatorType] = tk.TargetTeacherUserID
			}
		}

		// Additive check: pre-existing self+peer+leadership-rates-you counts
		// are exactly what TestMyTasks_SelfPeerAndLeadership already proves
		// for this shape (self=1 always; peer=1 only for guru2, who is the
		// sole peer evaluator here; leadership=0, neither guru holds a role).
		if counts[model.EvalTypeSelf] != 1 {
			t.Errorf("guru %s: self tasks = %d, want 1", guruID, counts[model.EvalTypeSelf])
		}
		wantPeer := 0
		if guruID == guru2 {
			wantPeer = 1
		}
		if counts[model.EvalTypePeer] != wantPeer {
			t.Errorf("guru %s: peer tasks = %d, want %d", guruID, counts[model.EvalTypePeer], wantPeer)
		}
		if counts[model.EvalTypeKepalaSekolah] != 0 || counts[model.EvalTypeWakaKurikulum] != 0 || counts[model.EvalTypeWakaKesiswaan] != 0 {
			t.Errorf("guru %s: leadership-rates-you tasks should be 0 (guru holds no leadership role), got kepsek:%d waka_kurikulum:%d waka_kesiswaan:%d",
				guruID, counts[model.EvalTypeKepalaSekolah], counts[model.EvalTypeWakaKurikulum], counts[model.EvalTypeWakaKesiswaan])
		}

		// New behavior: exactly 3 upward tasks, each targeting the real
		// current role-holder.
		if counts[model.EvalTypeGuruToKepsek] != 1 || counts[model.EvalTypeGuruToWakaKurikulum] != 1 || counts[model.EvalTypeGuruToWakaKesiswaan] != 1 {
			t.Fatalf("guru %s: upward tasks = guru_to_kepsek:%d guru_to_waka_kurikulum:%d guru_to_waka_kesiswaan:%d, want 1 each",
				guruID, counts[model.EvalTypeGuruToKepsek], counts[model.EvalTypeGuruToWakaKurikulum], counts[model.EvalTypeGuruToWakaKesiswaan])
		}
		if upwardTargets[model.EvalTypeGuruToKepsek] != kepsek {
			t.Errorf("guru %s: guru_to_kepsek task targets %s, want current kepsek %s", guruID, upwardTargets[model.EvalTypeGuruToKepsek], kepsek)
		}
		if upwardTargets[model.EvalTypeGuruToWakaKurikulum] != wakaKurikulum {
			t.Errorf("guru %s: guru_to_waka_kurikulum task targets %s, want current waka kurikulum %s", guruID, upwardTargets[model.EvalTypeGuruToWakaKurikulum], wakaKurikulum)
		}
		if upwardTargets[model.EvalTypeGuruToWakaKesiswaan] != wakaKesiswaan {
			t.Errorf("guru %s: guru_to_waka_kesiswaan task targets %s, want current waka kesiswaan %s", guruID, upwardTargets[model.EvalTypeGuruToWakaKesiswaan], wakaKesiswaan)
		}

		wantTotal := 1 + wantPeer + 3
		if len(tasks) != wantTotal {
			t.Errorf("guru %s: total tasks = %d, want %d", guruID, len(tasks), wantTotal)
		}
	}

	// computeResults for either guru must NOT include the upward-eval forms
	// (they're a separate, unblended track) -- categories should still be
	// exactly the original 6.
	res, err := h.computeResults(ctx, p, guru1)
	if err != nil {
		t.Fatalf("computeResults: %v", err)
	}
	if len(res.Categories) != 6 {
		t.Errorf("computeResults categories = %d, want 6 (upward-eval forms must be excluded from the blended score)", len(res.Categories))
	}
	for _, c := range res.Categories {
		if model.IsUpwardEvalType(c.EvaluatorType) {
			t.Errorf("computeResults leaked an upward-eval category into the blended score: %+v", c)
		}
	}

	// computeLeadershipResults must report 3 rows (one per assigned role),
	// each with expected_count = len(TargetTeacherIDs) = 2, submitted_count = 0
	// (nobody has submitted yet).
	lr, err := h.computeLeadershipResults(ctx, p)
	if err != nil {
		t.Fatalf("computeLeadershipResults: %v", err)
	}
	if len(lr.Categories) != 3 {
		t.Fatalf("leadership results categories = %d, want 3", len(lr.Categories))
	}
	for _, c := range lr.Categories {
		if c.ExpectedCount != 2 {
			t.Errorf("leadership category %s: expected_count = %d, want 2", c.EvaluatorType, c.ExpectedCount)
		}
		if c.SubmittedCount != 0 || c.AvgScore != nil || c.Complete {
			t.Errorf("leadership category %s: expected zero-submission state, got %+v", c.EvaluatorType, c)
		}
	}
}

// TestPeerEval_FullOverlap_ReciprocalMesh proves the "Pilih Semua Guru" fix:
// when admin sets BOTH "guru yang ingin dinilai" (K-a/PeerEvaluateeIDs) and
// "yang menilai" (K-b/PeerEvaluatorIDs) to the SAME full set of guru (e.g.
// Aji + Kunto), every guru must end up with a peer task rating every OTHER
// guru in that set -- Aji sees Kunto as a teman sejawat he can rate, and
// Kunto sees Aji, but neither gets a peer task targeting themselves.
//
// Before this fix, period creation with overlapping K-a/K-b outright failed
// validation (validatePeerCrossFilter rejected it with a 400), so this
// reciprocal mesh could never be created at all.
func TestPeerEval_FullOverlap_ReciprocalMesh(t *testing.T) {
	dbURL := os.Getenv("EVALUATION_TEST_DATABASE_URL")
	if dbURL == "" {
		dbURL = "postgres://sekoci:change-me@localhost:5480/skci_evaluation?sslmode=disable"
	}
	ctx := context.Background()
	pool, err := store.NewPool(ctx, dbURL)
	if err != nil {
		t.Skipf("skipping: dev postgres not reachable at %s: %v", dbURL, err)
	}
	t.Cleanup(pool.Close)

	newID := func() string {
		var id string
		if err := pool.QueryRow(ctx, "SELECT gen_random_uuid()").Scan(&id); err != nil {
			t.Fatalf("gen_random_uuid: %v", err)
		}
		return id
	}

	schoolID := newID()
	academicYearID := newID()
	aji := newID()
	kunto := newID()
	all := []string{aji, kunto}

	h := New(nil, store.NewPeriodStore(pool), store.NewFormStore(pool), store.NewTaskStore(pool), &client.Clients{})

	p := &model.Period{
		SchoolID: schoolID, AcademicYearID: academicYearID, Title: "test period full overlap peer",
		PublishStartDate: "2026-01-01", PublishEndDate: "2026-01-31",
		TargetTeacherIDs: all,
		// K(a) and K(b) are the SAME set -- "Pilih Semua Guru" on both fields.
		PeerEvaluateeIDs: all, PeerEvaluatorIDs: all,
	}
	if err := h.Periods.Create(ctx, p); err != nil {
		t.Fatalf("create period with overlapping peer_evaluatee_ids/peer_evaluator_ids: %v", err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(ctx, "DELETE FROM teacher_evaluation_periods WHERE id=$1", p.ID)
		_, _ = pool.Exec(ctx, "DELETE FROM teacher_evaluation_forms WHERE school_id=$1", schoolID)
	})

	if err := h.regenerateTasks(ctx, p); err != nil {
		t.Fatalf("regenerateTasks: %v", err)
	}

	// Aji's tasks must include a peer task where he evaluates Kunto, and NOT
	// one where he evaluates himself.
	ajiTasks, err := h.Tasks.ListForEvaluator(ctx, p.ID, aji)
	if err != nil {
		t.Fatalf("ListForEvaluator(aji): %v", err)
	}
	ajiRatesKunto, ajiRatesSelf := false, false
	for _, tk := range ajiTasks {
		if tk.EvaluatorType != model.EvalTypePeer {
			continue
		}
		if tk.TargetTeacherUserID == kunto {
			ajiRatesKunto = true
		}
		if tk.TargetTeacherUserID == aji {
			ajiRatesSelf = true
		}
	}
	if !ajiRatesKunto {
		t.Errorf("expected aji to have a peer task rating kunto (teman sejawat), got tasks: %+v", ajiTasks)
	}
	if ajiRatesSelf {
		t.Errorf("aji must not have a peer task targeting himself, got tasks: %+v", ajiTasks)
	}

	// And the reverse: Kunto rates Aji.
	kuntoTasks, err := h.Tasks.ListForEvaluator(ctx, p.ID, kunto)
	if err != nil {
		t.Fatalf("ListForEvaluator(kunto): %v", err)
	}
	kuntoRatesAji := false
	for _, tk := range kuntoTasks {
		if tk.EvaluatorType == model.EvalTypePeer && tk.TargetTeacherUserID == aji {
			kuntoRatesAji = true
		}
	}
	if !kuntoRatesAji {
		t.Errorf("expected kunto to have a peer task rating aji (teman sejawat), got tasks: %+v", kuntoTasks)
	}
}
