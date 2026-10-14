package model

import (
	"testing"
	"time"
)

func tp(t time.Time) *time.Time { return &t }

func TestDiscountConditions(t *testing.T) {
	cases := []struct {
		name  string
		raw   string
		facts ApplicantFacts
		want  bool
		err   bool
	}{
		{"empty matches all", `{}`, ApplicantFacts{TrackID: "t1"}, true, false},
		{"null matches all", `null`, ApplicantFacts{}, true, false},
		{"track match", `{"track_ids":["tahfidz"]}`, ApplicantFacts{TrackID: "tahfidz"}, true, false},
		{"track mismatch", `{"track_ids":["tahfidz"]}`, ApplicantFacts{TrackID: "reguler"}, false, false},
		{"wave mismatch", `{"wave_ids":["w1"]}`, ApplicantFacts{WaveID: "w2"}, false, false},
		{"sibling required and present (TC-09-038 A)", `{"sibling":true}`, ApplicantFacts{HasSibling: true}, true, false},
		{"sibling required but absent (TC-09-038 B)", `{"sibling":true}`, ApplicantFacts{}, false, false},
		{"all conditions", `{"sibling":true,"track_ids":["a"],"wave_ids":["w"]}`, ApplicantFacts{TrackID: "a", WaveID: "w", HasSibling: true}, true, false},
		{"unknown key fails closed", `{"hafalan_juz":5}`, ApplicantFacts{}, false, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			cond, err := ParseDiscountConditions([]byte(c.raw))
			if (err != nil) != c.err {
				t.Fatalf("err=%v want err=%v", err, c.err)
			}
			if err == nil && cond.Matches(c.facts) != c.want {
				t.Fatalf("Matches=%v want %v", !c.want, c.want)
			}
		})
	}
}

func TestPromoActiveBoundary(t *testing.T) {
	wib := time.FixedZone("WIB", 7*3600)
	until := time.Date(2026, 10, 31, 23, 59, 59, 0, wib)
	from := time.Date(2026, 10, 1, 0, 0, 0, 0, wib)
	cases := []struct {
		name string
		at   time.Time
		want bool
	}{
		{"31 Okt 23.58 WIB gets promo (TC-09-028)", time.Date(2026, 10, 31, 23, 58, 0, 0, wib), true},
		{"1 Nov 00.01 WIB does not", time.Date(2026, 11, 1, 0, 1, 0, 0, wib), false},
		{"same instant expressed in UTC", time.Date(2026, 10, 31, 16, 58, 0, 0, time.UTC), true},
		{"before start", time.Date(2026, 9, 30, 23, 0, 0, 0, wib), false},
		{"exactly at end", until, true},
	}
	for _, c := range cases {
		if got := PromoActive(c.at, &from, &until); got != c.want {
			t.Errorf("%s: got %v", c.name, got)
		}
	}
	if !PromoActive(time.Now(), nil, nil) {
		t.Error("open window must be active")
	}
}

func TestPromoQuotaAndWindow(t *testing.T) {
	if !PromoQuotaAvailable(0, 1000) || !PromoQuotaAvailable(2, 1) || PromoQuotaAvailable(2, 2) {
		t.Fatal("quota semantics")
	}
	a := time.Date(2026, 10, 31, 0, 0, 0, 0, time.UTC)
	b := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	if ValidatePromoWindow(&a, &b) == nil {
		t.Fatal("inverted window must fail (TC-09-027)")
	}
	if ValidatePromoWindow(&b, &a) != nil || ValidatePromoWindow(nil, &a) != nil {
		t.Fatal("valid window rejected")
	}
}

func TestApplyDiscountPolicyExclusive(t *testing.T) {
	now := time.Now()
	early := DiscountCandidate{ID: "eb", Kind: DiscountFixed, Value: 500_000, Priority: 1, CreatedAt: now}
	sibling := DiscountCandidate{ID: "sib", Kind: DiscountPercent, Value: 1000, Priority: 2, CreatedAt: now}
	scholar := DiscountCandidate{ID: "tahfidz", Kind: DiscountFixed, Value: 1_500_000, Priority: 3, CreatedAt: now, Exclusive: true}
	cases := []struct {
		name    string
		in      []DiscountCandidate
		mode    string
		max     int64
		wantNet int64
		wantIDs []string
	}{
		{"no candidates", nil, "stack", 0, 9_000_000, nil},
		{"stack stackables", []DiscountCandidate{early, sibling}, "stack", 0, 7_650_000, []string{"eb", "sib"}},
		{"exclusive wins when larger", []DiscountCandidate{early, scholar}, "stack", 0, 7_500_000, []string{"tahfidz"}},
		{"stack wins when larger", []DiscountCandidate{early, sibling, {ID: "x", Kind: DiscountFixed, Value: 100_000, Exclusive: true}}, "stack", 0, 7_650_000, []string{"eb", "sib"}},
		{"max picks single best", []DiscountCandidate{early, sibling}, "max", 0, 8_100_000, []string{"sib"}},
		{"cap per applicant", []DiscountCandidate{early, sibling}, "stack", 1_000_000, 8_000_000, []string{"eb", "sib"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			net, steps, err := ApplyDiscountPolicy(9_000_000, c.in, c.mode, c.max)
			if err != nil {
				t.Fatal(err)
			}
			if net != c.wantNet {
				t.Fatalf("net=%d want %d", net, c.wantNet)
			}
			if len(steps) != len(c.wantIDs) {
				t.Fatalf("steps=%v want %v", steps, c.wantIDs)
			}
			for i, s := range steps {
				if s.ID != c.wantIDs[i] {
					t.Fatalf("step %d=%s want %s", i, s.ID, c.wantIDs[i])
				}
			}
		})
	}
}

func TestPlanInstallments(t *testing.T) {
	due := time.Date(2027, 2, 15, 16, 59, 0, 0, time.UTC)
	cases := []struct {
		name  string
		net   int64
		count int
		pct   int
		want  []int64
	}{
		{"single", 9_000_000, 1, 50, []int64{9_000_000}},
		{"first is minimum 50%", 9_000_000, 3, 50, []int64{4_500_000, 2_250_000, 2_250_000}},
		{"even split at 100%", 9_000_000, 3, 100, []int64{3_000_000, 3_000_000, 3_000_000}},
		{"remainder on last", 1_000, 3, 100, []int64{333, 333, 334}},
		{"rounded-up minimum", 9_999, 2, 50, []int64{5_000, 4_999}},
		{"zero net", 0, 4, 50, []int64{0}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			lines := PlanInstallments(c.net, c.count, c.pct, due)
			var sum int64
			if len(lines) != len(c.want) {
				t.Fatalf("lines=%v", lines)
			}
			for i, l := range lines {
				if l.Amount != c.want[i] || l.Sequence != i+1 {
					t.Fatalf("line %d=%+v want %d", i, l, c.want[i])
				}
				sum += l.Amount
			}
			if sum != c.net {
				t.Fatalf("sum %d != net %d", sum, c.net)
			}
			if !lines[0].DueAt.Equal(due) {
				t.Fatal("first installment must be due at the deadline")
			}
		})
	}
}

func TestSettleInstallmentsPerApplicant(t *testing.T) {
	now := time.Date(2027, 3, 1, 0, 0, 0, 0, time.UTC)
	lines := []InstallmentLine{
		{Sequence: 1, Amount: 4_500_000, DueAt: now.AddDate(0, 0, -1)},
		{Sequence: 2, Amount: 4_500_000, DueAt: now.AddDate(0, 1, 0)},
	}
	cases := []struct {
		name       string
		paid       int64
		wantStatus []string
		wantPaid   []int64
	}{
		{"nothing paid, first overdue", 0, []string{"overdue", "unpaid"}, []int64{0, 0}},
		{"first paid", 4_500_000, []string{"paid", "unpaid"}, []int64{4_500_000, 0}},
		{"partial second", 6_000_000, []string{"paid", "partial"}, []int64{4_500_000, 1_500_000}},
		{"overpaid lands on last", 9_500_000, []string{"paid", "paid"}, []int64{4_500_000, 5_000_000}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := SettleInstallments(lines, c.paid, now)
			for i := range got {
				if got[i].Status != c.wantStatus[i] || got[i].Paid != c.wantPaid[i] {
					t.Fatalf("line %d=%+v", i, got[i])
				}
			}
		})
	}
}

func TestAutoAllocate(t *testing.T) {
	cases := []struct {
		name   string
		amount int64
		open   []InstallmentBalance
		want   []Allocation
		err    bool
	}{
		{"fits first", 100, []InstallmentBalance{{"a", 300}, {"b", 300}}, []Allocation{{"a", 100}}, false},
		{"spills", 400, []InstallmentBalance{{"a", 300}, {"b", 300}}, []Allocation{{"a", 300}, {"b", 100}}, false},
		{"skips settled", 50, []InstallmentBalance{{"a", 0}, {"b", 300}}, []Allocation{{"b", 50}}, false},
		{"overpay on last", 700, []InstallmentBalance{{"a", 300}, {"b", 300}}, []Allocation{{"a", 300}, {"b", 400}}, false},
		{"no installments", 10, nil, nil, true},
		{"non-positive", 0, []InstallmentBalance{{"a", 1}}, nil, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := AutoAllocate(c.amount, c.open)
			if (err != nil) != c.err {
				t.Fatalf("err=%v", err)
			}
			if c.err {
				return
			}
			if len(got) != len(c.want) {
				t.Fatalf("got %v want %v", got, c.want)
			}
			for i := range got {
				if got[i] != c.want[i] {
					t.Fatalf("got %v want %v", got, c.want)
				}
			}
			if ValidateAllocations(c.amount, got) != nil {
				t.Fatal("auto allocation must sum to the payment")
			}
		})
	}
}

func TestReregistrationMinimumBoundary(t *testing.T) {
	min := MinimumPayment(9_000_000, 50)
	cases := []struct {
		name string
		paid int64
		done bool
		want bool
	}{
		{"just below minimum (TC-15-012)", 4_499_999, true, false},
		{"exactly minimum", 4_500_000, true, true},
		{"items missing", 9_000_000, false, false},
	}
	for _, c := range cases {
		if got := ReregistrationComplete(c.paid, min, c.done); got != c.want {
			t.Errorf("%s: got %v", c.name, got)
		}
	}
}

func TestDeadlineDrops(t *testing.T) {
	wib := time.FixedZone("WIB", 7*3600)
	deadline := time.Date(2026, 11, 30, 23, 59, 0, 0, wib)
	cases := []struct {
		name    string
		now     time.Time
		settled bool
		pending bool
		want    bool
	}{
		{"before deadline", deadline.Add(-time.Minute), false, false, false},
		{"exactly at deadline", deadline, false, false, false},
		{"after deadline unpaid", deadline.Add(2 * time.Minute), false, false, true},
		{"after deadline but settled", deadline.Add(time.Hour), true, false, false},
		{"proof transferred on time awaiting review", deadline.Add(time.Hour), false, true, false},
	}
	for _, c := range cases {
		if got := ShouldDropUnpaidForm(c.now, &deadline, c.settled, c.pending); got != c.want {
			t.Errorf("form %s: got %v", c.name, got)
		}
	}
	if ShouldDropUnpaidForm(deadline.AddDate(1, 0, 0), nil, false, false) {
		t.Error("no deadline configured must never drop")
	}
	due := deadline
	rcases := []struct {
		name string
		now  time.Time
		paid int64
		want bool
	}{
		{"minimum reached 23.50 then job (TC-15-016)", deadline.Add(time.Minute), 4_500_000, false},
		{"below minimum after deadline", deadline.Add(time.Minute), 4_499_999, true},
		{"below minimum before deadline", deadline.Add(-time.Minute), 0, false},
	}
	for _, c := range rcases {
		if got := ShouldDropReregistration(c.now, &due, c.paid, 4_500_000, false); got != c.want {
			t.Errorf("rereg %s: got %v", c.name, got)
		}
	}
}

func TestQuotaAndWaitlist(t *testing.T) {
	if CheckQuota(0, 0) != ErrQuotaFull {
		t.Fatal("quota 0 accepts nobody")
	}
	if CheckQuota(5, 4) != nil || CheckQuota(5, 5) != ErrQuotaFull {
		t.Fatal("last seat semantics")
	}
	if VacantSeats(10, 12) != 0 || VacantSeats(10, 8) != 2 {
		t.Fatal("vacant seats")
	}
	waitlist := []WaitlistCandidate{{"c78", 3, 78}, {"c82", 1, 82}, {"c80", 2, 80}}
	cases := []struct {
		name  string
		skip  map[string]bool
		seats int
		want  []string
	}{
		{"no seats (TC-13-018)", nil, 0, nil},
		{"one seat picks top", nil, 1, []string{"c82"}},
		{"two seats two distinct (TC-13-017)", nil, 2, []string{"c82", "c80"}},
		{"rejected skipped for next (TC-13-016)", map[string]bool{"c82": true}, 1, []string{"c80"}},
		{"empty waitlist (TC-13-015)", nil, 3, []string{"c82", "c80", "c78"}},
	}
	for _, c := range cases {
		got := PickWaitlist(waitlist, c.skip, c.seats)
		if len(got) != len(c.want) {
			t.Fatalf("%s: got %v", c.name, got)
		}
		for i := range got {
			if got[i].ApplicationID != c.want[i] {
				t.Fatalf("%s: got %v", c.name, got)
			}
		}
	}
	if PickWaitlist(nil, nil, 2) != nil {
		t.Fatal("no waitlist yields no proposal")
	}
}

func TestEligibility(t *testing.T) {
	cfg, err := ParseEligibility([]byte(`{"min_age_years":6,"age_reference_date":"2027-07-01","wave_ids":["w1"]}`))
	if err != nil {
		t.Fatal(err)
	}
	five := time.Date(2022, 1, 1, 0, 0, 0, 0, time.UTC)
	six := time.Date(2021, 7, 1, 0, 0, 0, 0, time.UTC)
	cases := []struct {
		name   string
		birth  *time.Time
		wave   string
		issues int
	}{
		{"age 5 rejected (TC-16-018)", &five, "w1", 1},
		{"turns 6 on reference date", &six, "w1", 0},
		{"wrong wave", &six, "w2", 1},
		{"missing birth date", nil, "w1", 1},
	}
	for _, c := range cases {
		if got := cfg.CheckEligibility(c.birth, c.wave, time.Now()); len(got) != c.issues {
			t.Errorf("%s: %v", c.name, got)
		}
	}
	for _, bad := range []string{`{"min_age_years":-1}`, `{"min_age_years":9,"max_age_years":7}`, `{"age_reference_date":"1 Juli"}`, `{"foo":1}`} {
		if _, err := ParseEligibility([]byte(bad)); err == nil {
			t.Errorf("%s must be rejected", bad)
		}
	}
}

func TestValidateWaveSchedule(t *testing.T) {
	d := func(m, day int) time.Time { return time.Date(2026, time.Month(m), day, 0, 0, 0, 0, time.UTC) }
	start, end := d(10, 1), d(11, 30)
	cases := []struct {
		name string
		s    WaveSchedule
		ok   bool
	}{
		{"valid", WaveSchedule{RegistrationStart: start, RegistrationEnd: end, PaymentEnd: tp(d(12, 7)), TestAt: tp(d(12, 10)), AnnouncementAt: tp(d(12, 20)), ReregistrationEnd: tp(d(12, 31))}, true},
		{"inverted registration", WaveSchedule{RegistrationStart: end, RegistrationEnd: start}, false},
		{"payment far after close (TC-09-036)", WaveSchedule{RegistrationStart: start, RegistrationEnd: end, PaymentEnd: tp(time.Date(2027, 3, 31, 0, 0, 0, 0, time.UTC))}, false},
		{"rereg before announcement (TC-09-035)", WaveSchedule{RegistrationStart: start, RegistrationEnd: end, AnnouncementAt: tp(d(12, 20)), ReregistrationEnd: tp(d(12, 19))}, false},
		{"rereg without announcement", WaveSchedule{RegistrationStart: start, RegistrationEnd: end, ReregistrationEnd: tp(d(12, 19))}, false},
		{"announcement before test", WaveSchedule{RegistrationStart: start, RegistrationEnd: end, TestAt: tp(d(12, 20)), AnnouncementAt: tp(d(12, 10))}, false},
	}
	for _, c := range cases {
		if err := ValidateWaveSchedule(c.s, 30); (err == nil) != c.ok {
			t.Errorf("%s: err=%v", c.name, err)
		}
	}
	if !RangesOverlap(d(10, 1), d(11, 30), d(11, 15), d(12, 30)) {
		t.Error("TC-09-020 overlap not detected")
	}
	if RangesOverlap(d(10, 1), d(10, 31), d(11, 1), d(11, 30)) {
		t.Error("adjacent waves must not overlap")
	}
}

func TestDetectUpload(t *testing.T) {
	pdf := []byte("%PDF-1.7\n%âãÏÓ\n1 0 obj")
	png := []byte("\x89PNG\r\n\x1a\n\x00\x00\x00\rIHDR")
	jpg := []byte("\xff\xd8\xff\xe0\x00\x10JFIF\x00")
	exe := []byte("MZ\x90\x00\x03\x00\x00\x00")
	cases := []struct {
		name string
		file string
		head []byte
		want string
	}{
		{"pdf", "akta.pdf", pdf, "application/pdf"},
		{"png upper ext", "FOTO.PNG", png, "image/png"},
		{"jpeg", "kk.jpeg", jpg, "image/jpeg"},
		{"exe renamed pdf (TC-16-017)", "akta.pdf", exe, ""},
		{"exe extension", "setup.exe", exe, ""},
		{"png content named pdf", "akta.pdf", png, ""},
		{"text named pdf", "akta.pdf", []byte("hello world"), ""},
		{"no extension", "akta", pdf, ""},
	}
	for _, c := range cases {
		got, err := DetectUpload(c.file, c.head)
		if c.want == "" {
			if err == nil {
				t.Errorf("%s: accepted as %s", c.name, got)
			}
			continue
		}
		if err != nil || got != c.want {
			t.Errorf("%s: got %q err %v", c.name, got, err)
		}
	}
	if ValidateAllowedMIMEs([]string{"application/x-msdownload"}) == nil || ValidateAllowedMIMEs(nil) == nil {
		t.Error("TC-09-034 must reject executables / empty list")
	}
	if ValidateAllowedMIMEs([]string{"application/pdf", "image/png"}) != nil {
		t.Error("whitelisted types rejected")
	}
}

func TestNIKAndScore(t *testing.T) {
	for nik, ok := range map[string]bool{"": true, "3216098778901234": true, "32160987789012": false, "32160987789012ab": false} {
		if (ValidateNIK(nik) == nil) != ok {
			t.Errorf("NIK %q", nik)
		}
	}
	if MaskNIK("3216098778901234") != "************1234" || MaskNIK("12") != "**" {
		t.Error("mask")
	}
	cases := []struct {
		score, max int64
		ok         bool
	}{{0, 100, true}, {100, 100, true}, {150, 100, false}, {-1, 100, false}}
	for _, c := range cases {
		if (ValidateScore(c.score, c.max) == nil) != c.ok {
			t.Errorf("score %d/%d", c.score, c.max)
		}
	}
}

func TestDroppedTransitions(t *testing.T) {
	cases := []struct {
		from ApplicationStatus
		ok   bool
	}{
		{StatusSubmitted, true}, {StatusOffered, true}, {StatusReregistration, true},
		{StatusConverted, false}, {StatusDraft, false}, {StatusDropped, false},
	}
	for _, c := range cases {
		if CanTransition(c.from, StatusDropped) != c.ok {
			t.Errorf("%s -> dropped", c.from)
		}
	}
	if ValidateTransition(StatusSubmitted, StatusDropped, "") == nil {
		t.Error("dropping needs a reason")
	}
}

func TestDocumentIssue(t *testing.T) {
	cases := []struct {
		name     string
		uploaded bool
		review   string
		want     string
	}{
		{"missing", false, "", "Dokumen wajib belum diunggah: KK."},
		{"accepted", true, "accepted", ""},
		{"rejected blocks (TC-10 BUG-033)", true, "rejected", "Dokumen wajib ditolak: KK."},
		{"correction blocks", true, "correction", "Dokumen wajib perlu perbaikan: KK."},
		{"unreviewed blocks", true, "", "Dokumen wajib belum diverifikasi: KK."},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := DocumentIssue("KK", tc.uploaded, tc.review); got != tc.want {
				t.Fatalf("got %q want %q", got, tc.want)
			}
		})
	}
}

func TestValidateFeeScheme(t *testing.T) {
	cases := []struct {
		name            string
		reg, rereg, ent int64
		count, pct      int
		ok              bool
	}{
		{"valid", 250000, 1000000, 5000000, 6, 30, true},
		{"negative fee", -1, 0, 0, 1, 100, false},
		{"zero installments", 0, 0, 0, 0, 100, false},
		{"too many installments", 0, 0, 0, 25, 100, false},
		{"pct zero", 0, 0, 0, 1, 0, false},
		{"pct over 100", 0, 0, 0, 1, 101, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidateFeeScheme(tc.reg, tc.rereg, tc.ent, tc.count, tc.pct)
			if (err == nil) != tc.ok {
				t.Fatalf("err=%v ok=%v", err, tc.ok)
			}
		})
	}
}

func TestPaymentStatusOf(t *testing.T) {
	cases := []struct {
		billed, outstanding int64
		want                string
	}{
		{0, 0, "unbilled"},
		{250000, 250000, "unpaid"},
		{250000, 100000, "partial"},
		{250000, 0, "paid"},
	}
	for _, tc := range cases {
		if got := PaymentStatusOf(tc.billed, tc.outstanding); got != tc.want {
			t.Fatalf("PaymentStatusOf(%d,%d)=%q want %q", tc.billed, tc.outstanding, got, tc.want)
		}
	}
}

func TestDocumentUploadAllowed(t *testing.T) {
	cases := []struct {
		name   string
		status string
		exists bool
		review string
		locked bool
		want   bool
	}{
		{"draft new", "draft", false, "", false, true},
		{"draft replace", "draft", true, "", false, true},
		{"submitted new file refused", "submitted", false, "", false, false},
		{"submitted correction re-upload", "submitted", true, "correction", false, true},
		{"review rejected re-upload", "document_review", true, "rejected", false, true},
		{"review accepted refused", "document_review", true, "accepted", false, false},
		{"locked refused", "draft", true, "", true, false},
		{"eligible refused", "eligible_for_test", true, "correction", false, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := DocumentUploadAllowed(tc.status, tc.exists, tc.review, tc.locked); got != tc.want {
				t.Fatalf("got %v want %v", got, tc.want)
			}
		})
	}
}

func TestSafeFileName(t *testing.T) {
	cases := []struct{ in, mime, want string }{
		{"akta.pdf", "application/pdf", "akta.pdf"},
		{`a"b;c.pdf`, "application/pdf", "a_b_c.pdf"},
		{"", "image/png", "dokumen.png"},
		{"\r\n", "application/pdf", "dokumen.pdf"},
	}
	for _, tc := range cases {
		if got := SafeFileName(tc.in, tc.mime); got != tc.want {
			t.Fatalf("SafeFileName(%q)=%q want %q", tc.in, got, tc.want)
		}
	}
}

func TestReregistrationDeadline(t *testing.T) {
	now := time.Date(2027, 3, 1, 10, 0, 0, 0, time.UTC)
	end := time.Date(2027, 3, 20, 17, 0, 0, 0, time.UTC)
	past := time.Date(2027, 2, 1, 0, 0, 0, 0, time.UTC)
	cases := []struct {
		name     string
		waveEnd  *time.Time
		days     int
		waitlist bool
		want     time.Time
	}{
		{"announced uses wave end", &end, 7, false, end},
		{"waitlist gets relative days", &end, 5, true, now.AddDate(0, 0, 5)},
		{"no wave end", nil, 3, false, now.AddDate(0, 0, 3)},
		{"wave end already past", &past, 3, false, now.AddDate(0, 0, 3)},
		{"zero days defaults to 7", nil, 0, true, now.AddDate(0, 0, 7)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := ReregistrationDeadline(now, tc.waveEnd, tc.days, tc.waitlist); !got.Equal(tc.want) {
				t.Fatalf("got %v want %v", got, tc.want)
			}
		})
	}
}

func TestWaveScheduleChanged(t *testing.T) {
	base := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	later := base.AddDate(0, 1, 0)
	s := WaveSchedule{RegistrationStart: base, RegistrationEnd: later}
	same := s
	sameLater := later
	same.RegistrationEnd = sameLater
	withPay := s
	withPay.PaymentEnd = &later
	cases := []struct {
		name string
		a, b WaveSchedule
		want bool
	}{
		{"identical", s, same, false},
		{"payment deadline added", s, withPay, true},
		{"close moved", s, WaveSchedule{RegistrationStart: base, RegistrationEnd: later.Add(time.Hour)}, true},
	}
	for _, c := range cases {
		if got := WaveScheduleChanged(c.a, c.b); got != c.want {
			t.Errorf("%s: got %v", c.name, got)
		}
	}
}

func TestRegistrationNumber(t *testing.T) {
	cases := []struct {
		prefix string
		seq    int
		want   string
	}{
		{"PPDB-2027-G01", 1, "PPDB-2027-G01-0001"},
		{"PPDB-2027-G02", 125, "PPDB-2027-G02-0125"},
		{"PPDB-2027-G01", 12345, "PPDB-2027-G01-12345"},
	}
	for _, c := range cases {
		if got := RegistrationNumber(c.prefix, c.seq); got != c.want {
			t.Errorf("got %s want %s", got, c.want)
		}
	}
}

func TestActorLabel(t *testing.T) {
	cases := []struct{ typ, name, want string }{
		{"admin", "Bu Rina", "Bu Rina"},
		{"guardian", "", "Calon orang tua"},
		{"system", " ", "Sistem PPDB"},
		{"approval", "", "Approval Center"},
		{"admin", "", "Admin PPDB"},
	}
	for _, c := range cases {
		if got := ActorLabel(c.typ, c.name); got != c.want {
			t.Errorf("%s/%q: %s", c.typ, c.name, got)
		}
	}
}

func TestValidateSchedule(t *testing.T) {
	now := time.Date(2026, 10, 14, 8, 0, 0, 0, time.UTC)
	cases := []struct {
		kind string
		at   time.Time
		ok   bool
	}{
		{"test", now.Add(time.Hour), true},
		{"interview", now.Add(24 * time.Hour), true},
		{"test", now, false},
		{"exam", now.Add(time.Hour), false},
	}
	for _, c := range cases {
		if err := ValidateSchedule(c.kind, c.at, now); (err == nil) != c.ok {
			t.Errorf("%s %v: %v", c.kind, c.at, err)
		}
	}
	for status, want := range map[string]bool{"eligible_for_test": true, "document_review": true, "tested": true, "draft": false, "offered": false} {
		if SchedulableStatus(status) != want {
			t.Errorf("schedulable %s", status)
		}
	}
}

func TestParseWIBDate(t *testing.T) {
	cases := []struct {
		in      string
		end     bool
		want    string
		wantErr bool
	}{
		{"", false, "", false},
		{"2026-10-31", false, "2026-10-30T17:00:00Z", false},
		{"2026-10-31", true, "2026-10-31T16:59:59.999999999Z", false},
		{"2026-10-31T10:00:00+07:00", true, "2026-10-31T03:00:00Z", false},
		{"31/10/2026", false, "", true},
	}
	for _, c := range cases {
		got, err := ParseWIBDate(c.in, c.end)
		if (err != nil) != c.wantErr {
			t.Fatalf("%q: err %v", c.in, err)
		}
		if c.want == "" {
			if got != nil && !c.wantErr {
				t.Errorf("%q: want nil", c.in)
			}
			continue
		}
		if got.UTC().Format(time.RFC3339Nano) != c.want {
			t.Errorf("%q end=%v: got %s", c.in, c.end, got.UTC().Format(time.RFC3339Nano))
		}
	}
}

func TestBuildFunnel(t *testing.T) {
	c := FunnelCounts{Registered: 120, Submitted: 100, FormPaid: 90, Verified: 80, Accepted: 70, Reregistered: 60}
	f := BuildFunnel(&c)
	if len(f) != 6 || f[0].Count != 120 || f[5].Count != 60 {
		t.Fatalf("funnel %+v", f)
	}
	if f[1].OfFirstBP != 8333 || f[1].OfPreviousBP != 8333 || f[5].OfPreviousBP != 8571 {
		t.Errorf("rates %+v", f)
	}
	if ConversionBP(c) != 5000 {
		t.Errorf("conversion %d", ConversionBP(c))
	}
	empty := BuildFunnel(nil)
	if empty[3].OfFirstBP != 0 || ConversionBP(FunnelCounts{}) != 0 {
		t.Error("empty funnel must not divide by zero")
	}
}
