package handler

import (
	"context"
	"database/sql"
	"os"
	"testing"

	"skci/admission/internal/config"
	"skci/admission/internal/store"
)

// Integration tests run against a migrated Postgres (TEST_PG_DSN). Each test
// seeds its own school id, so runs never collide.

func testApp(t *testing.T) *App {
	t.Helper()
	dsn := os.Getenv("TEST_PG_DSN")
	if dsn == "" {
		t.Skip("TEST_PG_DSN not set")
	}
	db, err := store.Open(dsn)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return &App{DB: db, Config: config.Config{SchoolName: "Sekolah Uji"}}
}

type seed struct {
	school, period, wave, track, scheme, guardian string
}

func mustExec(t *testing.T, db *sql.DB, q string, args ...any) {
	t.Helper()
	if _, err := db.Exec(q, args...); err != nil {
		t.Fatalf("exec %q: %v", q, err)
	}
}

func mustID(t *testing.T, db *sql.DB, q string, args ...any) string {
	t.Helper()
	var id string
	if err := db.QueryRow(q, args...).Scan(&id); err != nil {
		t.Fatalf("query %q: %v", q, err)
	}
	return id
}

// seedSchool creates an open period/wave/track with a fee scheme:
// formulir 250.000, daftar ulang 1.000.000 + uang pangkal 5.000.000 in 3
// installments with a 40% minimum.
func seedSchool(t *testing.T, db *sql.DB, quota int) seed {
	t.Helper()
	var s seed
	s.school = mustID(t, db, `SELECT gen_random_uuid()::text`)
	s.period = mustID(t, db, `INSERT INTO admission_periods(school_id,name,academic_year,status) VALUES($1,'PPDB','2027/2028','open') RETURNING id`, s.school)
	s.wave = mustID(t, db, `INSERT INTO admission_waves(school_id,period_id,name,registration_start,registration_end,payment_end,reregistration_end)
		VALUES($1,$2,'Gelombang 1',now()-interval '1 day',now()+interval '10 day',now()+interval '12 day',now()+interval '40 day') RETURNING id`, s.school, s.period)
	s.track = mustID(t, db, `INSERT INTO admission_tracks(school_id,period_id,code,name,quota) VALUES($1,$2,'REG','Reguler',$3) RETURNING id`, s.school, s.period, quota)
	s.scheme = mustID(t, db, `INSERT INTO fee_schemes(school_id,period_id,name,revision,status,registration_fee_idr,reregistration_fee_idr,entrance_fee_idr,installment_count,min_reregistration_pct)
		VALUES($1,$2,'Biaya 2027',1,'published',250000,1000000,5000000,3,40) RETURNING id`, s.school, s.period)
	for _, it := range []struct {
		code   string
		amount int64
	}{{"registration", 250000}, {"reregistration", 1000000}, {"entrance", 5000000}} {
		mustExec(t, db, `INSERT INTO fee_items(school_id,fee_scheme_id,code,name,amount_idr) VALUES($1,$2,$3,$3,$4)`, s.school, s.scheme, it.code, it.amount)
	}
	s.guardian = mustID(t, db, `INSERT INTO guardians(school_id,name) VALUES($1,'Wali Uji') RETURNING id`, s.school)
	return s
}

func seedApplication(t *testing.T, db *sql.DB, s seed, guardian, status string) string {
	t.Helper()
	applicant := mustID(t, db, `INSERT INTO applicants(school_id,guardian_id,full_name) VALUES($1,$2,'Ananda') RETURNING id`, s.school, guardian)
	return mustID(t, db, `INSERT INTO applications(school_id,applicant_id,period_id,wave_id,track_id,registration_number,status)
		VALUES($1,$2,$3,$4,$5,'REG-'||substr(md5(random()::text),1,10),$6) RETURNING id`, s.school, applicant, s.period, s.wave, s.track, status)
}

func addRule(t *testing.T, db *sql.DB, s seed, name, kind string, value int64, appliesTo, conditions string, stackable bool) string {
	t.Helper()
	return mustID(t, db, `INSERT INTO discount_rules(school_id,period_id,name,kind,value,priority,conditions,applies_to,stackable)
		VALUES($1,$2,$3,$4,$5,10,$6::jsonb,$7,$8) RETURNING id`, s.school, s.period, name, kind, value, conditions, appliesTo, stackable)
}

func chargeTotals(t *testing.T, db *sql.DB, chargeID string) (gross, discount, net int64, steps int) {
	t.Helper()
	if err := db.QueryRow(`SELECT gross_idr,discount_idr,net_idr,(SELECT count(*) FROM discount_applications WHERE charge_id=c.id)
		FROM charges c WHERE id=$1`, chargeID).Scan(&gross, &discount, &net, &steps); err != nil {
		t.Fatal(err)
	}
	return
}

// BUG-011: a promo only reaches applicants matching its conditions, appears
// once, and re-issuing or rebuilding never duplicates it.
func TestChargeDiscountsConditionalAndNotDuplicated(t *testing.T) {
	a := testApp(t)
	ctx := context.Background()
	s := seedSchool(t, a.DB, 10)
	addRule(t, a.DB, s, "Diskon saudara kandung", "percent", 1000, "reregistration", `{"sibling":true}`, true)
	addRule(t, a.DB, s, "Early bird", "fixed", 500000, "reregistration", `{}`, true)

	lone := seedApplication(t, a.DB, s, s.guardian, "offered")
	chargeID, err := a.issueCharge(ctx, s.school, lone, stageReregistration, issueOptions{})
	if err != nil {
		t.Fatal(err)
	}
	gross, discount, net, steps := chargeTotals(t, a.DB, chargeID)
	if gross != 6000000 || discount != 500000 || net != 5500000 || steps != 1 {
		t.Fatalf("lone applicant: gross=%d discount=%d net=%d steps=%d", gross, discount, net, steps)
	}

	again, err := a.issueCharge(ctx, s.school, lone, stageReregistration, issueOptions{})
	if err != nil || again != chargeID {
		t.Fatalf("re-issue must be idempotent: %v %s vs %s", err, again, chargeID)
	}
	rebuilt, err := a.issueCharge(ctx, s.school, lone, stageReregistration, issueOptions{Rebuild: true})
	if err != nil {
		t.Fatal(err)
	}
	if _, d, n, st := chargeTotals(t, a.DB, rebuilt); d != 500000 || n != 5500000 || st != 1 {
		t.Fatalf("rebuild duplicated discounts: discount=%d net=%d steps=%d", d, n, st)
	}

	// The sibling (same guardian, already offered) gets both promos.
	sibling := seedApplication(t, a.DB, s, s.guardian, "offered")
	sibCharge, err := a.issueCharge(ctx, s.school, sibling, stageReregistration, issueOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if _, d, _, st := chargeTotals(t, a.DB, sibCharge); st != 2 || d != 600000+500000 { // 10% of 6.000.000 first (older rule), then the fixed promo
		t.Fatalf("sibling: discount=%d steps=%d", d, st)
	}

	// The registration stage is untouched by re-registration promos.
	regCharge, err := a.issueCharge(ctx, s.school, seedApplication(t, a.DB, s, s.guardian, "submitted"), stageRegistration, issueOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if g, d, n, _ := chargeTotals(t, a.DB, regCharge); g != 250000 || d != 0 || n != 250000 {
		t.Fatalf("registration charge: gross=%d discount=%d net=%d", g, d, n)
	}
}

// BUG-003: installments belong to each applicant's own charge, and one
// applicant's payment never settles another applicant's installment.
func TestInstallmentsArePerApplicant(t *testing.T) {
	a := testApp(t)
	ctx := context.Background()
	s := seedSchool(t, a.DB, 10)
	other := mustID(t, a.DB, `INSERT INTO guardians(school_id,name) VALUES($1,'Wali Lain') RETURNING id`, s.school)
	appA := seedApplication(t, a.DB, s, s.guardian, "offered")
	appB := seedApplication(t, a.DB, s, other, "offered")
	chargeA, err := a.issueCharge(ctx, s.school, appA, stageReregistration, issueOptions{})
	if err != nil {
		t.Fatal(err)
	}
	chargeB, err := a.issueCharge(ctx, s.school, appB, stageReregistration, issueOptions{})
	if err != nil {
		t.Fatal(err)
	}
	var minA int64
	if err := a.DB.QueryRow(`SELECT min_payment_idr FROM charges WHERE id=$1`, chargeA).Scan(&minA); err != nil {
		t.Fatal(err)
	}
	if minA != 2400000 {
		t.Fatalf("40%% minimum of 6.000.000 = %d", minA)
	}

	// Applicant A pays the first installment (verified, allocated).
	open, _, err := openInstallments(ctx, a.DB, s.school, appA, stageReregistration)
	if err != nil || len(open) != 3 {
		t.Fatalf("open installments A: %v %d", err, len(open))
	}
	pay := mustID(t, a.DB, `INSERT INTO payment_submissions(school_id,application_id,charge_id,amount_idr,object_key,mime,size_bytes,sha256,status,uploader_type,uploader_id,idempotency_key)
		VALUES($1,$2,$3,2400000,'k','application/pdf',1,'x','verified','guardian',$4,gen_random_uuid()::text) RETURNING id`, s.school, appA, chargeA, s.guardian)
	mustExec(t, a.DB, `INSERT INTO payment_allocations(school_id,payment_id,charge_installment_id,amount_idr) VALUES($1,$2,$3,2400000)`, s.school, pay, open[0].ID)

	viewA, err := a.chargeView(ctx, s.school, appA, chargeA)
	if err != nil {
		t.Fatal(err)
	}
	viewB, err := a.chargeView(ctx, s.school, appB, chargeB)
	if err != nil {
		t.Fatal(err)
	}
	if viewA["paid"].(int64) != 2400000 || viewA["min_payment_met"] != true {
		t.Fatalf("A paid=%v min_met=%v", viewA["paid"], viewA["min_payment_met"])
	}
	if viewB["paid"].(int64) != 0 || viewB["outstanding"].(int64) != 6000000 {
		t.Fatalf("B affected by A's payment: paid=%v outstanding=%v", viewB["paid"], viewB["outstanding"])
	}
	instB := viewB["installments"].([]map[string]any)
	instA := viewA["installments"].([]map[string]any)
	if instA[0]["id"] == instB[0]["id"] {
		t.Fatal("installments are shared between applicants")
	}
	if instA[0]["status"] != "paid" || instB[0]["status"] == "paid" {
		t.Fatalf("installment status A=%v B=%v", instA[0]["status"], instB[0]["status"])
	}
}

// BUG-031: the last seat of a track can only be taken once.
func TestSubmitEnforcesTrackQuota(t *testing.T) {
	a := testApp(t)
	ctx := context.Background()
	s := seedSchool(t, a.DB, 1)
	first := seedApplication(t, a.DB, s, s.guardian, "draft")
	second := seedApplication(t, a.DB, s, s.guardian, "draft")
	if err := a.submit(ctx, s.school, s.guardian, first, 1); err != nil {
		t.Fatalf("first submit: %v", err)
	}
	err := a.submit(ctx, s.school, s.guardian, second, 1)
	he, ok := err.(*httpError)
	if !ok || he.status != 422 {
		t.Fatalf("second submit must be refused as quota full, got %v", err)
	}
	var status string
	if err := a.DB.QueryRow(`SELECT status FROM applications WHERE id=$1`, second).Scan(&status); err != nil || status != "draft" {
		t.Fatalf("refused application changed: %s %v", status, err)
	}
	// The submitted application has its form fee charge and a notification.
	var charges, notes int
	_ = a.DB.QueryRow(`SELECT count(*) FROM charges WHERE application_id=$1 AND stage='registration'`, first).Scan(&charges)
	_ = a.DB.QueryRow(`SELECT count(*) FROM guardian_notifications WHERE application_id=$1`, first).Scan(&notes)
	if charges != 1 || notes != 1 {
		t.Fatalf("submit side effects: charges=%d notifications=%d", charges, notes)
	}
}

// BUG-033/107: rejected required documents or an unpaid form block the test stage.
func TestEligibilityIssues(t *testing.T) {
	a := testApp(t)
	ctx := context.Background()
	s := seedSchool(t, a.DB, 5)
	req := mustID(t, a.DB, `INSERT INTO document_requirements(school_id,period_id,code,name,required,max_bytes) VALUES($1,$2,'KK','Kartu Keluarga',true,1048576) RETURNING id`, s.school, s.period)
	app := seedApplication(t, a.DB, s, s.guardian, "submitted")
	if _, err := a.issueCharge(ctx, s.school, app, stageRegistration, issueOptions{}); err != nil {
		t.Fatal(err)
	}
	doc := mustID(t, a.DB, `INSERT INTO application_documents(school_id,application_id,requirement_id,object_key,mime,size_bytes,sha256,uploaded_by)
		VALUES($1,$2,$3,'k','application/pdf',1,'x','g') RETURNING id`, s.school, app, req)
	mustExec(t, a.DB, `INSERT INTO document_reviews(school_id,document_id,status,notes,reviewer_id) VALUES($1,$2,'rejected','buram','admin')`, s.school, doc)

	issues, err := eligibilityIssues(ctx, a.DB, s.school, app)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"Biaya formulir belum lunas.", "Dokumen wajib ditolak: Kartu Keluarga."}
	if len(issues) != len(want) || issues[0] != want[0] || issues[1] != want[1] {
		t.Fatalf("issues=%v", issues)
	}
}
