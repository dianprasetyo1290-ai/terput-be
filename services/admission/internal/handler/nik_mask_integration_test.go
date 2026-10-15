package handler

import (
	"net/http/httptest"
	"testing"
)

// QA-XSVC (QA-IDN §9.5): admission lists never carry the full NIK. The
// guardian's own applicant detail keeps it (the edit form needs it); staff
// see the masked value everywhere (BUG-121).
func TestNIKMaskedInLists(t *testing.T) {
	a := testApp(t)
	s := seedSchool(t, a.DB, 10)
	const nik = "3201000000000001"
	const masked = "3201********0001"
	applicant := mustID(t, a.DB, `INSERT INTO applicants(school_id,guardian_id,full_name,nik) VALUES($1,$2,'Ananda',$3) RETURNING id`, s.school, s.guardian, nik)
	app := mustID(t, a.DB, `INSERT INTO applications(school_id,applicant_id,period_id,wave_id,track_id,registration_number,status)
		VALUES($1,$2,$3,$4,$5,'REG-'||substr(md5(random()::text),1,10),'submitted') RETURNING id`, s.school, applicant, s.period, s.wave, s.track)
	req := httptest.NewRequest("GET", "/", nil)

	list, err := a.listApplicants(req, s.school, s.guardian)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 || list[0]["nik"] != masked {
		t.Fatalf("guardian list nik = %v, want %s", list, masked)
	}

	staff, err := a.applicationDetail(req, s.school, app, "admin", "")
	if err != nil {
		t.Fatal(err)
	}
	if got := staff["applicant"].(map[string]any)["nik"]; got != masked {
		t.Fatalf("staff detail nik = %v, want %s", got, masked)
	}

	own, err := a.applicationDetail(req, s.school, app, "guardian", s.guardian)
	if err != nil {
		t.Fatal(err)
	}
	if got := own["applicant"].(map[string]any)["nik"]; got != nik {
		t.Fatalf("guardian detail nik = %v, want full", got)
	}
}
