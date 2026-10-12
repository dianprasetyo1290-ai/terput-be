package health

import (
	"errors"
	"reflect"
	"testing"
	"time"
)

func TestAlertView_AC12701(t *testing.T) {
	p := Profile{
		BloodType:          "O",
		Allergies:          []Allergy{{Name: "Kacang", Reaction: "sesak napas", IsAlert: true}, {Name: "Debu", IsAlert: false}},
		Conditions:         []Condition{{Name: "Riwayat asma", Note: "kambuh saat dingin", IsAlert: false}},
		RoutineMedications: []Medication{{Name: "Salbutamol"}},
	}
	contacts := []Contact{{Name: "Ibu", Relationship: "ibu", Phone: "081234567"}}
	got := AlertView(p, contacts)
	want := Alert{Items: []AlertItem{{Kind: "allergy", Name: "Kacang", Detail: "sesak napas"}}, EmergencyContacts: contacts}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %+v want %+v", got, want)
	}
	empty := AlertView(Profile{}, nil)
	if empty.Items == nil || empty.EmergencyContacts == nil || len(empty.Items) != 0 {
		t.Fatalf("empty alert must be non-nil empty slices: %+v", empty)
	}
	cond := AlertView(Profile{Conditions: []Condition{{Name: "Epilepsi", Note: "kejang", IsAlert: true}}}, nil)
	if len(cond.Items) != 1 || cond.Items[0].Kind != "condition" {
		t.Fatalf("alert condition missing: %+v", cond)
	}
}

func TestValidateProfile(t *testing.T) {
	ok := func() ProfileInput {
		return ProfileInput{Profile: Profile{BloodType: "AB", Rhesus: "+", Allergies: []Allergy{{Name: " Kacang "}}},
			EmergencyContacts: []Contact{{Name: "Ayah", Phone: "0812-3456 789"}}}
	}
	cases := []struct {
		name   string
		mutate func(*ProfileInput)
		ok     bool
	}{
		{"valid", func(*ProfileInput) {}, true},
		{"tidak diketahui", func(p *ProfileInput) { p.BloodType = "tidak_diketahui"; p.Rhesus = "" }, true},
		{"golongan darah asing", func(p *ProfileInput) { p.BloodType = "C" }, false},
		{"golongan darah kosong", func(p *ProfileInput) { p.BloodType = "" }, false},
		{"rhesus asing", func(p *ProfileInput) { p.Rhesus = "x" }, false},
		{"nama alergi kosong", func(p *ProfileInput) { p.Allergies[0].Name = "  " }, false},
		{"nama kondisi kosong", func(p *ProfileInput) { p.Conditions = []Condition{{Name: ""}} }, false},
		{"nama obat kosong", func(p *ProfileInput) { p.RoutineMedications = []Medication{{Name: ""}} }, false},
		{"telepon huruf", func(p *ProfileInput) { p.EmergencyContacts[0].Phone = "08abc" }, false},
		{"telepon terlalu pendek", func(p *ProfileInput) { p.EmergencyContacts[0].Phone = "12345" }, false},
		{"telepon +62", func(p *ProfileInput) { p.EmergencyContacts[0].Phone = "+62 812 3456 789" }, true},
		{"nama kontak kosong", func(p *ProfileInput) { p.EmergencyContacts[0].Name = "" }, false},
	}
	for _, tc := range cases {
		p := ok()
		tc.mutate(&p)
		err := ValidateProfile(&p)
		if (err == nil) != tc.ok {
			t.Errorf("%s: err=%v", tc.name, err)
		}
	}
	p := ok()
	_ = ValidateProfile(&p)
	if p.Allergies[0].Name != "Kacang" || p.EmergencyContacts[0].Phone != "08123456789" {
		t.Fatalf("not normalized: %+v", p)
	}
}

func TestApplyChange_FR127002(t *testing.T) {
	proposed := ProfileInput{Profile: Profile{BloodType: "A", Allergies: []Allergy{{Name: "Udang", IsAlert: true}}}}
	v, alert, err := ApplyChange(3, 3, proposed)
	if err != nil || v != 4 || len(alert.Items) != 1 || alert.Items[0].Name != "Udang" {
		t.Fatalf("apply: v=%d alert=%+v err=%v", v, alert, err)
	}
	if v, _, err := ApplyChange(0, 0, proposed); err != nil || v != 1 {
		t.Fatalf("first profile: v=%d err=%v", v, err)
	}
	if v, _, err := ApplyChange(4, 3, proposed); !errors.Is(err, ErrSuperseded) || v != 4 {
		t.Fatalf("stale proposal must be superseded: v=%d err=%v", v, err)
	}
}

func TestChangedFields(t *testing.T) {
	a := ProfileInput{Profile: Profile{BloodType: "A"}}
	b := ProfileInput{Profile: Profile{BloodType: "A", Allergies: []Allergy{{Name: "Kacang"}}}, EmergencyContacts: []Contact{{Name: "x"}}}
	if got := ChangedFields(a, b); !reflect.DeepEqual(got, []string{"allergies", "emergency_contacts"}) {
		t.Fatalf("got %v", got)
	}
	if got := ChangedFields(a, a); len(got) != 0 {
		t.Fatalf("got %v", got)
	}
}

func TestVisitEffects_AC12801(t *testing.T) {
	cases := map[string]Effects{
		OutcomeBackToClass: {false, false},
		OutcomeSentHome:    {true, true},
		OutcomeReferred:    {true, true},
		"unknown":          {false, false},
	}
	for outcome, want := range cases {
		if got := VisitEffects(outcome); got != want {
			t.Errorf("%s: got %+v want %+v", outcome, got, want)
		}
	}
}

func TestSickNote(t *testing.T) {
	jkt, _ := time.LoadLocation("Asia/Jakarta")
	mks, _ := time.LoadLocation("Asia/Makassar")
	since := time.Date(2026, 10, 12, 3, 15, 0, 0, time.UTC) // 10.15 WIB
	cases := []struct {
		outcome string
		loc     *time.Location
		want    string
	}{
		{OutcomeSentHome, jkt, "Dipulangkan dari UKS pukul 10.15 WIB"},
		{OutcomeReferred, jkt, "Dirujuk dari UKS pukul 10.15 WIB"},
		{OutcomeSentHome, mks, "Dipulangkan dari UKS pukul 11.15 WITA"},
	}
	for _, tc := range cases {
		if got := SickNote(tc.outcome, since, tc.loc); got != tc.want {
			t.Errorf("got %q want %q", got, tc.want)
		}
	}
}

func TestValidateVisit(t *testing.T) {
	d := VisitDetails{Complaint: "Demam"}
	if err := ValidateVisit(OutcomeSentHome, d, ""); err != nil {
		t.Fatal(err)
	}
	if err := ValidateVisit("pulang", d, ""); err == nil {
		t.Fatal("unknown outcome accepted")
	}
	if err := ValidateVisit(OutcomeBackToClass, VisitDetails{}, ""); err == nil {
		t.Fatal("empty complaint accepted")
	}
	if err := ValidateVisit(OutcomeReferred, d, " "); err == nil {
		t.Fatal("referral without destination accepted")
	}
	if err := ValidateVisit(OutcomeReferred, d, "RSUD"); err != nil {
		t.Fatal(err)
	}
}

func TestCanRecordResult_AC12901(t *testing.T) {
	cases := []struct {
		name     string
		requires bool
		status   string
		consent  string
		wantErr  error
		anyErr   bool
	}{
		{"butuh consent, belum ada", true, ProgramPublished, "", ErrConsentMissing, true},
		{"butuh consent, pending", true, ProgramPublished, "pending", ErrConsentMissing, true},
		{"butuh consent, declined", true, ProgramPublished, "declined", ErrConsentMissing, true},
		{"butuh consent, revoked", true, ProgramPublished, "revoked", ErrConsentMissing, true},
		{"butuh consent, granted", true, ProgramPublished, "granted", nil, false},
		{"tanpa consent", false, ProgramPublished, "", nil, false},
		{"program draft", false, ProgramDraft, "", nil, true},
		{"program dibatalkan", true, ProgramCancelled, "granted", nil, true},
	}
	for _, tc := range cases {
		err := CanRecordResult(tc.requires, tc.status, tc.consent)
		if (err != nil) != tc.anyErr || (tc.wantErr != nil && !errors.Is(err, tc.wantErr)) {
			t.Errorf("%s: err=%v", tc.name, err)
		}
	}
	if ErrConsentMissing.Error() != "Persetujuan orang tua belum tersedia" {
		t.Fatal("pesan AC-129-01 berubah")
	}
}

func TestResolveTargets_FR129001(t *testing.T) {
	r := Rosters{
		ByClassroom: map[string][]string{"c7a": {"s1", "s2"}, "c7b": {"s3"}, "c8a": {"s4", "s1"}},
		GradeOf:     map[string]string{"c7a": "7", "c7b": "7", "c8a": "8"},
	}
	cases := []struct {
		name    string
		targets []Target
		want    []string
	}{
		{"satu kelas", []Target{{TargetClassroom, "c7a"}}, []string{"s1", "s2"}},
		{"satu tingkat", []Target{{TargetGradeLevel, "7"}}, []string{"s1", "s2", "s3"}},
		{"siswa di dua target → satu peserta", []Target{{TargetClassroom, "c7a"}, {TargetClassroom, "c8a"}, {TargetStudent, "s1"}}, []string{"s1", "s2", "s4"}},
		{"siswa individual", []Target{{TargetStudent, "s9"}}, []string{"s9"}},
		{"kelas tak dikenal", []Target{{TargetClassroom, "zz"}}, []string{}},
		{"tanpa sasaran", nil, []string{}},
	}
	for _, tc := range cases {
		if got := ResolveTargets(tc.targets, r); !reflect.DeepEqual(got, tc.want) {
			t.Errorf("%s: got %v want %v", tc.name, got, tc.want)
		}
	}
}

func TestValidateProgram(t *testing.T) {
	d := func(s string) time.Time { v, _ := time.Parse("2006-01-02", s); return v }
	dl := d("2026-10-20")
	late := d("2026-11-05")
	tg := []Target{{TargetClassroom, "c1"}}
	cases := []struct {
		name string
		err  bool
		f    func() error
	}{
		{"valid", false, func() error {
			return ValidateProgram(KindImmunization, "BIAS", d("2026-11-01"), d("2026-11-03"), true, &dl, tg)
		}},
		{"jenis asing", true, func() error { return ValidateProgram("vaksin", "x", d("2026-11-01"), d("2026-11-01"), false, nil, tg) }},
		{"nama kosong", true, func() error {
			return ValidateProgram(KindScreening, " ", d("2026-11-01"), d("2026-11-01"), false, nil, tg)
		}},
		{"selesai sebelum mulai", true, func() error {
			return ValidateProgram(KindScreening, "x", d("2026-11-02"), d("2026-11-01"), false, nil, tg)
		}},
		{"butuh consent tanpa batas", true, func() error {
			return ValidateProgram(KindScreening, "x", d("2026-11-01"), d("2026-11-01"), true, nil, tg)
		}},
		{"batas setelah mulai", true, func() error {
			return ValidateProgram(KindScreening, "x", d("2026-11-01"), d("2026-11-09"), true, &late, tg)
		}},
		{"sasaran asing", true, func() error {
			return ValidateProgram(KindScreening, "x", d("2026-11-01"), d("2026-11-01"), false, nil, []Target{{"sekolah", "x"}})
		}},
	}
	for _, tc := range cases {
		if err := tc.f(); (err != nil) != tc.err {
			t.Errorf("%s: err=%v", tc.name, err)
		}
	}
}

func TestProgramRecap_FR129003(t *testing.T) {
	got := ProgramRecap([]RecapInput{
		{ResultStatus: ResultDone, ConsentStatus: "granted", NeedsFollowUp: true},
		{ResultStatus: ResultDone, ConsentStatus: "granted", NeedsFollowUp: true, FollowUpDone: true},
		{ResultStatus: ResultNotRecorded, ConsentStatus: "pending"},
		{ResultStatus: ResultExcluded, ConsentStatus: "declined"},
		{ResultStatus: ResultAbsent},
	})
	if got.Total != 5 || got.ByResult[ResultDone] != 2 || got.ByResult[ResultNotRecorded] != 1 || got.ByResult[ResultExcluded] != 1 || got.ByResult[ResultAbsent] != 1 {
		t.Fatalf("by result: %+v", got)
	}
	if got.ByConsent["granted"] != 2 || got.ByConsent["pending"] != 1 || got.ByConsent["declined"] != 1 || got.ByConsent["revoked"] != 0 {
		t.Fatalf("by consent: %+v", got.ByConsent)
	}
	if got.NeedsFollowUp != 2 || got.FollowUpOpen != 1 || got.FollowUpFinished != 1 {
		t.Fatalf("follow-up: %+v", got)
	}
	empty := ProgramRecap(nil)
	if empty.Total != 0 || len(empty.ByResult) != len(ResultStatuses) {
		t.Fatalf("empty: %+v", empty)
	}
}
