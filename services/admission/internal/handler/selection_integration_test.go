package handler

import (
	"context"
	"testing"
	"time"
)

func appStatus(t *testing.T, a *App, id string) string {
	t.Helper()
	var s string
	if err := a.DB.QueryRow(`SELECT status FROM applications WHERE id=$1`, id).Scan(&s); err != nil {
		t.Fatal(err)
	}
	return s
}

func countRows(t *testing.T, a *App, q string, args ...any) int {
	t.Helper()
	var n int
	if err := a.DB.QueryRow(q, args...).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

// BUG-034/035/036/037/038/039: selection results need committee approval,
// stay hidden until the scheduled announcement, then publish with the
// re-registration bill and notifications; a waitlist promotion approved by
// core fills a released seat; duplicate callbacks are no-ops.
func TestSelectionApprovalAnnouncementAndWaitlist(t *testing.T) {
	a := testApp(t)
	ctx := context.Background()
	s := seedSchool(t, a.DB, 1)
	offered := seedApplication(t, a.DB, s, s.guardian, "ranked")
	waitlisted := seedApplication(t, a.DB, s, s.guardian, "ranked")
	rejected := seedApplication(t, a.DB, s, s.guardian, "ranked")

	approval := mustID(t, a.DB, `SELECT gen_random_uuid()::text`)
	sel := mustID(t, a.DB, `INSERT INTO selection_approvals(school_id,period_id,wave_id,status,approval_id,decision_count,created_by)
		VALUES($1,$2,$3,'pending',$4,3,'admin') RETURNING id`, s.school, s.period, s.wave, approval)
	for i, d := range []struct{ app, decision string }{{offered, "offered"}, {waitlisted, "waitlisted"}, {rejected, "rejected"}} {
		mustExec(t, a.DB, `INSERT INTO decisions(school_id,application_id,decision,actor_id,version,status,selection_approval_id)
			VALUES($1,$2,$3,'admin',$4,'pending_approval',$5)`, s.school, d.app, d.decision, i+1, sel)
	}

	cb := approvalCallback{ApprovalID: approval, SourceID: sel, RequestType: "selection_result", Decision: "approved", DecidedBy: "ketua-panitia", DecidedAt: time.Now()}
	res, err := a.applyApprovalCallback(ctx, cb, time.Now())
	if err != nil || res["applied"] != "selection" {
		t.Fatalf("callback: %v %v", res, err)
	}
	if res, err = a.applyApprovalCallback(ctx, cb, time.Now()); err != nil || res["duplicate"] != true {
		t.Fatalf("duplicate callback must be a no-op: %v %v", res, err)
	}
	if n := countRows(t, a, `SELECT count(*) FROM decisions WHERE selection_approval_id=$1 AND status='approved'`, sel); n != 3 {
		t.Fatalf("approved decisions=%d", n)
	}

	publishAt := time.Now().Add(2 * time.Hour)
	ann := mustID(t, a.DB, `INSERT INTO ppdb_announcements(school_id,period_id,wave_id,title,message,publish_at,status,created_by,selection_approval_id)
		VALUES($1,$2,$3,'Hasil seleksi','',$4,'approved','admin',$5) RETURNING id`, s.school, s.period, s.wave, publishAt, sel)

	// Before the schedule nothing is visible and nothing moves.
	if err := a.publishDueAnnouncements(ctx, time.Now()); err != nil {
		t.Fatal(err)
	}
	if st := appStatus(t, a, offered); st != "ranked" {
		t.Fatalf("result leaked before the announcement: %s", st)
	}
	if n := countRows(t, a, `SELECT count(*) FROM decisions WHERE application_id=$1 AND status='published'`, offered); n != 0 {
		t.Fatal("decision published early")
	}

	if err := a.publishDueAnnouncements(ctx, publishAt.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	for id, want := range map[string]string{offered: "offered", waitlisted: "waitlisted", rejected: "rejected"} {
		if st := appStatus(t, a, id); st != want {
			t.Fatalf("after publish %s: %s, want %s", id, st, want)
		}
		if n := countRows(t, a, `SELECT count(*) FROM guardian_notifications WHERE application_id=$1`, id); n != 1 {
			t.Fatalf("notifications for %s: %d", want, n)
		}
	}
	if n := countRows(t, a, `SELECT count(*) FROM charges WHERE application_id=$1 AND stage='reregistration' AND cancelled_at IS NULL`, offered); n != 1 {
		t.Fatalf("re-registration bill not issued: %d", n)
	}
	if n := countRows(t, a, `SELECT count(*) FROM applications WHERE id=$1 AND reregistration_due_at IS NOT NULL`, offered); n != 1 {
		t.Fatal("re-registration deadline not set")
	}
	if n := countRows(t, a, `SELECT count(*) FROM ppdb_announcements WHERE id=$1 AND status='published' AND notified_count=3`, ann); n != 1 {
		t.Fatal("announcement not marked published")
	}

	// The track (quota 1) is full: a promotion approved now fails.
	promoApproval := mustID(t, a.DB, `SELECT gen_random_uuid()::text`)
	promo := mustID(t, a.DB, `INSERT INTO waitlist_promotions(school_id,application_id,period_id,wave_id,track_id,status,approval_id,created_by)
		VALUES($1,$2,$3,$4,$5,'pending',$6,'admin') RETURNING id`, s.school, waitlisted, s.period, s.wave, s.track, promoApproval)
	if _, err := a.applyApprovalCallback(ctx, approvalCallback{ApprovalID: promoApproval, SourceID: promo, RequestType: "selection_result", Decision: "approved", DecidedAt: time.Now()}, time.Now()); err != nil {
		t.Fatal(err)
	}
	if n := countRows(t, a, `SELECT count(*) FROM waitlist_promotions WHERE id=$1 AND status='failed'`, promo); n != 1 || appStatus(t, a, waitlisted) != "waitlisted" {
		t.Fatal("promotion into a full track must fail")
	}

	// The offered applicant misses the re-registration minimum: dropped,
	// bill cancelled, seat released (BR-PPDB-08).
	mustExec(t, a.DB, `UPDATE applications SET reregistration_due_at=now()-interval '1 hour' WHERE id=$1`, offered)
	if err := a.dropUnpaidReregistration(ctx, time.Now()); err != nil {
		t.Fatal(err)
	}
	if st := appStatus(t, a, offered); st != "dropped" {
		t.Fatalf("unpaid re-registration not dropped: %s", st)
	}
	if n := countRows(t, a, `SELECT count(*) FROM charges WHERE application_id=$1 AND stage='reregistration' AND cancelled_at IS NULL`, offered); n != 0 {
		t.Fatal("charge of a dropped applicant still active")
	}

	// Now a promotion succeeds and the promoted applicant gets an offer.
	promoApproval2 := mustID(t, a.DB, `SELECT gen_random_uuid()::text`)
	promo2 := mustID(t, a.DB, `INSERT INTO waitlist_promotions(school_id,application_id,period_id,wave_id,track_id,status,approval_id,created_by)
		VALUES($1,$2,$3,$4,$5,'pending',$6,'admin') RETURNING id`, s.school, waitlisted, s.period, s.wave, s.track, promoApproval2)
	if _, err := a.applyApprovalCallback(ctx, approvalCallback{ApprovalID: promoApproval2, SourceID: promo2, RequestType: "selection_result", Decision: "approved", DecidedAt: time.Now()}, time.Now()); err != nil {
		t.Fatal(err)
	}
	if st := appStatus(t, a, waitlisted); st != "offered" {
		t.Fatalf("promoted applicant: %s", st)
	}
	if n := countRows(t, a, `SELECT count(*) FROM decisions WHERE application_id=$1 AND status='published' AND decision='offered'`, waitlisted); n != 1 {
		t.Fatal("promotion decision not published")
	}
}

// BR-PPDB-07: an unpaid form fee past its deadline drops the application,
// unless a proof transferred on time is still under review.
func TestDropUnpaidForms(t *testing.T) {
	a := testApp(t)
	ctx := context.Background()
	s := seedSchool(t, a.DB, 10)
	unpaid := seedApplication(t, a.DB, s, s.guardian, "submitted")
	pending := seedApplication(t, a.DB, s, s.guardian, "submitted")
	for _, app := range []string{unpaid, pending} {
		if _, err := a.issueCharge(ctx, s.school, app, stageRegistration, issueOptions{}); err != nil {
			t.Fatal(err)
		}
		mustExec(t, a.DB, `UPDATE charges SET due_at=now()-interval '1 day' WHERE application_id=$1`, app)
	}
	mustExec(t, a.DB, `INSERT INTO payment_submissions(school_id,application_id,charge_id,amount_idr,object_key,mime,size_bytes,sha256,status,uploader_type,uploader_id,idempotency_key,transfer_at)
		SELECT school_id,application_id,id,250000,'k','application/pdf',1,'x','submitted','guardian','g',gen_random_uuid()::text,now()-interval '2 day'
		FROM charges WHERE application_id=$1`, pending)

	if err := a.dropUnpaidForms(ctx, time.Now()); err != nil {
		t.Fatal(err)
	}
	if st := appStatus(t, a, unpaid); st != "dropped" {
		t.Fatalf("unpaid form: %s", st)
	}
	if st := appStatus(t, a, pending); st != "submitted" {
		t.Fatalf("on-time proof under review must not drop: %s", st)
	}
	if n := countRows(t, a, `SELECT count(*) FROM guardian_notifications WHERE application_id=$1 AND type='dropped'`, unpaid); n != 1 {
		t.Fatal("dropped guardian not notified")
	}
	// Running the job again changes nothing.
	if err := a.dropUnpaidForms(ctx, time.Now()); err != nil {
		t.Fatal(err)
	}
	if n := countRows(t, a, `SELECT count(*) FROM guardian_notifications WHERE application_id=$1`, unpaid); n != 1 {
		t.Fatal("job re-notified")
	}
}
