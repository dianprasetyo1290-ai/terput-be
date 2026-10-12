package handler

import (
	"bytes"
	"context"
	"net/http"
	"strings"
	"testing"

	"skci/studentlife/internal/common"
	"skci/studentlife/internal/health"
)

// people creates a classroom with a homeroom teacher, one student with one
// guardian, plus a second classroom/teacher.
type people struct {
	uks, wali, waliLain, kepsek, admin, parent, parent2, student, student2, kelas, kelasLain string
}

func (e *env) people() people {
	p := people{uks: newID(e.t, e.db), wali: newID(e.t, e.db), waliLain: newID(e.t, e.db), kepsek: newID(e.t, e.db),
		admin: newID(e.t, e.db), parent: newID(e.t, e.db), parent2: newID(e.t, e.db), student: newID(e.t, e.db),
		student2: newID(e.t, e.db), kelas: newID(e.t, e.db), kelasLain: newID(e.t, e.db)}
	w := e.world
	w.homeroom[p.kelas], w.homeroom[p.kelasLain] = p.wali, p.waliLain
	w.roster[p.kelas], w.roster[p.kelasLain] = []string{p.student}, []string{p.student2}
	w.grade[p.kelas], w.grade[p.kelasLain] = "7", "8"
	w.guardians[p.student] = []string{p.parent, p.parent2}
	w.names[p.student], w.names[p.student2] = "Budi", "Sari"
	return p
}

var peanutProfile = map[string]any{
	"blood_type": "O", "rhesus": "+",
	"allergies":           []map[string]any{{"name": "Kacang", "reaction": "sesak napas", "is_alert": true}},
	"conditions":          []map[string]any{{"name": "Riwayat asma", "note": "kambuh saat dingin", "is_alert": false}},
	"routine_medications": []map[string]any{{"name": "Salbutamol", "dose": "2 hirup", "schedule": "bila perlu"}},
	"emergency_contacts":  []map[string]any{{"name": "Ibu Budi", "relationship": "ibu", "phone": "0812-3456-789"}},
}

func TestFeatureOff(t *testing.T) {
	e := setup(t)
	p := e.people()
	e.world.features[e.school] = false
	if r := e.do(http.MethodGet, "/health/visits", e.token(p.uks, common.RolePetugasUKS), nil); r.Code != http.StatusForbidden {
		t.Fatalf("feature off: %d", r.Code)
	}
	if r := e.do(http.MethodGet, "/health/visits", "", nil); r.Code != http.StatusUnauthorized {
		t.Fatalf("no token: %d", r.Code)
	}
}

// AC-127-01 + FR-127-001/003 + OQ-31 + NFR-OPS-03.
func TestProfileAccess_AC12701(t *testing.T) {
	e := setup(t)
	p := e.people()
	uks := e.token(p.uks, common.RolePetugasUKS)

	if r := e.do(http.MethodPut, "/health/students/"+p.student+"/profile", uks, map[string]any{"blood_type": "C"}); r.Code != http.StatusUnprocessableEntity {
		t.Fatalf("invalid blood type: %d", r.Code)
	}
	if r := e.do(http.MethodPut, "/health/students/"+p.student+"/profile", e.token(p.wali, common.RoleWalikelas), peanutProfile); r.Code != http.StatusForbidden {
		t.Fatalf("walikelas PUT: %d", r.Code)
	}
	r := e.do(http.MethodPut, "/health/students/"+p.student+"/profile", uks, peanutProfile)
	if r.Code != http.StatusOK {
		t.Fatalf("PUT profile: %d %s", r.Code, r.Body)
	}
	// NFR-OPS-03: raw columns are ciphertext.
	var profEnc, alertEnc []byte
	if err := e.db.QueryRow(`SELECT profile_enc, alert_enc FROM health_profiles WHERE school_id = $1`, e.school).Scan(&profEnc, &alertEnc); err != nil {
		t.Fatal(err)
	}
	for _, s := range []string{"Kacang", "asma", "Salbutamol"} {
		if bytes.Contains(profEnc, []byte(s)) || bytes.Contains(alertEnc, []byte(s)) {
			t.Fatalf("plaintext %q in DB", s)
		}
	}
	// Audit outbox never carries plaintext.
	var after string
	_ = e.db.QueryRow(`SELECT after::text FROM audit_events WHERE school_id = $1 AND entity_type = 'health_profiles'`, e.school).Scan(&after)
	if strings.Contains(after, "Kacang") || !strings.Contains(after, "allergies") {
		t.Fatalf("audit after: %s", after)
	}

	// Homeroom teacher: only alert + emergency contact.
	logsBefore := e.count(`SELECT count(*) FROM sensitive_access_logs WHERE school_id = $1`, e.school)
	r = e.do(http.MethodGet, "/health/alerts?classroom_id="+p.kelas, e.token(p.wali, common.RoleWalikelas), nil)
	if r.Code != http.StatusOK {
		t.Fatalf("alerts: %d %s", r.Code, r.Body)
	}
	if strings.Contains(string(r.Body), "asma") || strings.Contains(string(r.Body), "Salbutamol") || strings.Contains(string(r.Body), "blood_type") {
		t.Fatalf("alert view leaks non-alert data: %s", r.Body)
	}
	var alerts []health.StudentAlert
	r.data(t, &alerts)
	if len(alerts) != 1 || len(alerts[0].Items) != 1 || alerts[0].Items[0].Name != "Kacang" || len(alerts[0].EmergencyContacts) != 1 || alerts[0].StudentName != "Budi" {
		t.Fatalf("alerts: %+v", alerts)
	}
	if n := e.count(`SELECT count(*) FROM sensitive_access_logs WHERE school_id = $1`, e.school); n != logsBefore+1 {
		t.Fatalf("alert view not logged: %d → %d", logsBefore, n)
	}
	// Homeroom of another class → 404 + deny log.
	if r := e.do(http.MethodGet, "/health/alerts?classroom_id="+p.kelas, e.token(p.waliLain, common.RoleWalikelas), nil); r.Code != http.StatusNotFound {
		t.Fatalf("other walikelas alerts: %d", r.Code)
	}
	if n := e.count(`SELECT count(*) FROM sensitive_access_logs WHERE school_id = $1 AND action = 'deny' AND actor_user_id = $2`, e.school, p.waliLain); n != 1 {
		t.Fatalf("deny log rows = %d", n)
	}
	// Homeroom cannot open the full profile.
	if r := e.do(http.MethodGet, "/health/students/"+p.student+"/profile", e.token(p.wali, common.RoleWalikelas), nil); r.Code != http.StatusNotFound {
		t.Fatalf("walikelas full profile: %d", r.Code)
	}
	// OQ-31: kepala sekolah without policy → 404; superadmin too.
	kepsek := e.token(p.kepsek, common.RoleKepalaSekolah)
	if r := e.do(http.MethodGet, "/health/students/"+p.student+"/profile", kepsek, nil); r.Code != http.StatusNotFound {
		t.Fatalf("kepsek without policy: %d", r.Code)
	}
	if r := e.do(http.MethodGet, "/health/students/"+p.student+"/profile", e.token(p.admin, common.RoleAdmin), nil); r.Code != http.StatusNotFound {
		t.Fatalf("admin: %d", r.Code)
	}
	// Admin grants kepala_sekolah → 200.
	admin := e.token(p.admin, common.RoleAdmin)
	r = e.do(http.MethodPost, "/sensitive/access-policies", admin, map[string]string{"module": "health", "role": common.RoleKepalaSekolah})
	if r.Code != http.StatusCreated {
		t.Fatalf("grant: %d %s", r.Code, r.Body)
	}
	if r := e.do(http.MethodPost, "/sensitive/access-policies", admin, map[string]string{"module": "health", "role": common.RoleKepalaSekolah}); r.Code != http.StatusConflict {
		t.Fatalf("duplicate grant: %d", r.Code)
	}
	if r := e.do(http.MethodPost, "/sensitive/access-policies", admin, map[string]string{"module": "health", "role": common.RoleParent}); r.Code != http.StatusUnprocessableEntity {
		t.Fatalf("grant parent: %d", r.Code)
	}
	if r := e.do(http.MethodPost, "/sensitive/access-policies", kepsek, map[string]string{"module": "health", "role": common.RoleKepalaSekolah}); r.Code != http.StatusForbidden {
		t.Fatalf("kepsek grant: %d", r.Code)
	}
	r = e.do(http.MethodGet, "/health/students/"+p.student+"/profile", kepsek, nil)
	if r.Code != http.StatusOK {
		t.Fatalf("kepsek with policy: %d", r.Code)
	}
	var view health.ProfileView
	r.data(t, &view)
	if view.Profile.BloodType != "O" || len(view.Profile.Conditions) != 1 || view.Version != 1 || view.VerifiedBy == nil {
		t.Fatalf("profile view: %+v", view)
	}
	// Guardian sees full profile; unrelated parent does not.
	if r := e.do(http.MethodGet, "/health/students/"+p.student+"/profile", e.token(p.parent, common.RoleParent), nil); r.Code != http.StatusOK {
		t.Fatalf("guardian: %d", r.Code)
	}
	if r := e.do(http.MethodGet, "/health/students/"+p.student+"/profile", e.token(newID(t, e.db), common.RoleParent), nil); r.Code != http.StatusNotFound {
		t.Fatalf("stranger parent: %d", r.Code)
	}
	// Access log is readable by auditor, without health content.
	r = e.do(http.MethodGet, "/sensitive/access-logs?module=health", e.token(newID(t, e.db), common.RoleAuditor), nil)
	if r.Code != http.StatusOK || strings.Contains(string(r.Body), "Kacang") {
		t.Fatalf("access logs: %d %s", r.Code, r.Body)
	}
	if r := e.do(http.MethodGet, "/sensitive/access-logs", kepsek, nil); r.Code != http.StatusForbidden {
		t.Fatalf("kepsek logs: %d", r.Code)
	}
	// Cross-school: another school's token cannot see this student.
	other := *e
	other.school = newID(t, e.db)
	e.world.features[other.school] = true
	if r := other.do(http.MethodGet, "/health/students/"+p.student+"/profile", other.token(p.uks, common.RolePetugasUKS), nil); r.Code != http.StatusOK {
		t.Fatalf("other school uks: %d", r.Code)
	} else {
		var v health.ProfileView
		r.data(t, &v)
		if v.Exists {
			t.Fatal("cross-school profile visible")
		}
	}
}

// FR-128-001/002/003 + AC-128-01.
func TestVisit_AC12801(t *testing.T) {
	e := setup(t)
	p := e.people()
	uks := e.token(p.uks, common.RolePetugasUKS)
	var plain, stocked health.Medicine
	r := e.do(http.MethodPost, "/health/medicines", uks, map[string]any{"name": "Parasetamol", "unit": "tablet"})
	if r.Code != http.StatusCreated {
		t.Fatalf("medicine: %d %s", r.Code, r.Body)
	}
	r.data(t, &plain)
	opsItem := newID(t, e.db)
	e.do(http.MethodPost, "/health/medicines", uks, map[string]any{"name": "Kasa", "unit": "lembar", "ops_item_id": opsItem}).data(t, &stocked)
	if r := e.do(http.MethodPost, "/health/medicines", uks, map[string]any{"name": "parasetamol", "unit": "tablet"}); r.Code != http.StatusConflict {
		t.Fatalf("duplicate medicine: %d", r.Code)
	}

	body := map[string]any{"student_id": p.student, "complaint": "Demam tinggi", "examination": "Suhu 38,9", "action": "Kompres",
		"outcome": "sent_home", "medications": []map[string]any{{"medicine_id": plain.ID, "qty": 1}, {"medicine_id": stocked.ID, "qty": 2}}}
	if r := e.do(http.MethodPost, "/health/visits", uks, body); r.Code != http.StatusBadRequest {
		t.Fatalf("missing idempotency key: %d", r.Code)
	}
	r = e.do(http.MethodPost, "/health/visits", uks, body, "Idempotency-Key", "visit-form-1")
	if r.Code != http.StatusCreated {
		t.Fatalf("visit: %d %s", r.Code, r.Body)
	}
	var v health.VisitView
	r.data(t, &v)
	if v.AttendanceSync != health.SyncDone || v.ParentNotifiedAt == nil || v.Details == nil || v.Details.Complaint != "Demam tinggi" || len(v.Medications) != 2 {
		t.Fatalf("visit view: %+v", v)
	}
	if len(e.world.attendance) != 1 || e.world.attendance[0]["source_ref"] != v.ID || !strings.HasPrefix(e.world.attendance[0]["reason"].(string), "Dipulangkan dari UKS pukul") {
		t.Fatalf("attendance calls: %+v", e.world.attendance)
	}
	for _, g := range []string{p.parent, p.parent2} {
		if e.world.notifications["studentlife:visit:"+v.ID+":"+g] != 1 {
			t.Fatalf("notification to %s: %+v", g, e.world.notifications)
		}
	}
	// Repeat with same key: same visit, no extra side effects.
	r = e.do(http.MethodPost, "/health/visits", uks, body, "Idempotency-Key", "visit-form-1")
	var again health.VisitView
	r.data(t, &again)
	if r.Code != http.StatusOK || again.ID != v.ID || len(e.world.attendance) != 1 || len(e.world.notifications) != 2 {
		t.Fatalf("repeat: %d id=%s att=%d notif=%d", r.Code, again.ID, len(e.world.attendance), len(e.world.notifications))
	}
	if n := e.count(`SELECT count(*) FROM clinic_visits WHERE school_id = $1`, e.school); n != 1 {
		t.Fatalf("visits = %d", n)
	}
	// Encrypted at rest.
	var det []byte
	_ = e.db.QueryRow(`SELECT details_enc FROM clinic_visits WHERE id = $1`, v.ID).Scan(&det)
	if bytes.Contains(det, []byte("Demam")) {
		t.Fatal("visit details stored in plaintext")
	}
	// Addendum: original unchanged.
	if r := e.do(http.MethodPost, "/health/visits/"+v.ID+"/addenda", uks, map[string]string{"note": "Orang tua menjemput 10.40"}); r.Code != http.StatusCreated {
		t.Fatalf("addendum: %d %s", r.Code, r.Body)
	}
	var detail health.VisitView
	e.do(http.MethodGet, "/health/visits/"+v.ID, uks, nil).data(t, &detail)
	if detail.Details.Complaint != "Demam tinggi" || len(detail.Addenda) != 1 || detail.Addenda[0].Note != "Orang tua menjemput 10.40" {
		t.Fatalf("detail after addendum: %+v", detail)
	}
	// Back to class: no attendance, no notification.
	r = e.do(http.MethodPost, "/health/visits", uks, map[string]any{"student_id": p.student, "complaint": "Pusing", "outcome": "back_to_class"}, "Idempotency-Key", "visit-form-2")
	if r.Code != http.StatusCreated || len(e.world.attendance) != 1 || len(e.world.notifications) != 2 {
		t.Fatalf("back to class side effects: %d att=%d", r.Code, len(e.world.attendance))
	}
	if r := e.do(http.MethodPost, "/health/visits", uks, map[string]any{"student_id": p.student, "complaint": "x", "outcome": "referred"}, "Idempotency-Key", "k3"); r.Code != http.StatusUnprocessableEntity {
		t.Fatalf("referral without destination: %d", r.Code)
	}
	// Guardian reads child's visits; walikelas cannot.
	if r := e.do(http.MethodGet, "/health/students/"+p.student+"/visits", e.token(p.parent, common.RoleParent), nil); r.Code != http.StatusOK {
		t.Fatalf("guardian visits: %d", r.Code)
	}
	if r := e.do(http.MethodGet, "/health/students/"+p.student+"/visits", e.token(p.wali, common.RoleWalikelas), nil); r.Code != http.StatusNotFound {
		t.Fatalf("walikelas visits: %d", r.Code)
	}

	// FR-128-003 outbox: no ops item → skipped; OPS_URL empty → pending.
	if n := e.count(`SELECT count(*) FROM inventory_issue_outbox WHERE school_id = $1 AND status = 'skipped'`, e.school); n != 1 {
		t.Fatalf("skipped rows = %d", n)
	}
	ctx := context.Background()
	if err := e.app.Health.JobInventoryIssues(ctx, e.db); err != nil {
		t.Fatal(err)
	}
	if n := e.count(`SELECT count(*) FROM inventory_issue_outbox WHERE school_id = $1 AND status = 'pending'`, e.school); n != 1 {
		t.Fatalf("pending rows with OPS_URL empty = %d", n)
	}
	e.app.Client.URLs.Ops = e.server.URL
	if err := e.app.Health.JobInventoryIssues(ctx, e.db); err != nil {
		t.Fatal(err)
	}
	if n := e.count(`SELECT count(*) FROM inventory_issue_outbox WHERE school_id = $1 AND status = 'sent'`, e.school); n != 1 {
		t.Fatalf("sent rows = %d", n)
	}
	// Audit forwarding drains the outbox to core.
	if err := e.app.JobForwardAudit(ctx, e.db); err != nil {
		t.Fatal(err)
	}
	if n := e.count(`SELECT count(*) FROM audit_events WHERE school_id = $1 AND forwarded_at IS NULL`, e.school); n != 0 {
		t.Fatalf("unforwarded audit rows = %d", n)
	}
}

// FR-129-001/002/003 + AC-129-01.
func TestProgramConsent_AC12901(t *testing.T) {
	e := setup(t)
	p := e.people()
	uks := e.token(p.uks, common.RolePetugasUKS)
	prog := map[string]any{"kind": "immunization", "name": "BIAS Campak", "starts_on": "2026-11-02", "ends_on": "2026-11-03",
		"requires_consent": true, "consent_deadline": "2026-10-30",
		"targets": []map[string]string{{"target_kind": "grade_level", "target_ref": "7"}, {"target_kind": "student", "target_ref": p.student}}}
	var created health.Program
	r := e.do(http.MethodPost, "/health/programs", uks, prog)
	if r.Code != http.StatusCreated {
		t.Fatalf("program: %d %s", r.Code, r.Body)
	}
	r.data(t, &created)
	r = e.do(http.MethodPost, "/health/programs/"+created.ID+"/publish", uks, nil)
	if r.Code != http.StatusOK {
		t.Fatalf("publish: %d %s", r.Code, r.Body)
	}
	if r := e.do(http.MethodPost, "/health/programs/"+created.ID+"/publish", uks, nil); r.Code != http.StatusConflict {
		t.Fatalf("republish: %d", r.Code)
	}
	var parts []health.ParticipantView
	e.do(http.MethodGet, "/health/programs/"+created.ID+"/participants", uks, nil).data(t, &parts)
	if len(parts) != 1 || parts[0].StudentID != p.student || parts[0].ConsentStatus == nil || *parts[0].ConsentStatus != "pending" || parts[0].CanRecord {
		t.Fatalf("participants: %+v", parts)
	}
	for _, g := range []string{p.parent, p.parent2} {
		if e.world.notifications["studentlife:consent:"+*parts[0].ConsentID+":"+g] != 1 {
			t.Fatalf("consent request to %s missing: %+v", g, e.world.notifications)
		}
	}
	resultURL := "/health/programs/" + created.ID + "/participants/" + parts[0].ID + "/result"
	result := map[string]any{"result_status": "done", "result_note": "Vaksin diberikan", "needs_follow_up": true, "follow_up_note": "Pantau demam"}
	r = e.do(http.MethodPut, resultURL, uks, result)
	if r.Code != http.StatusUnprocessableEntity || !strings.Contains(string(r.Body), "Persetujuan orang tua belum tersedia") {
		t.Fatalf("AC-129-01 without consent: %d %s", r.Code, r.Body)
	}
	// Parent sees and grants; second guardian then gets 409.
	parent := e.token(p.parent, common.RoleParent)
	var mine []map[string]any
	e.do(http.MethodGet, "/consents/my", parent, nil).data(t, &mine)
	if len(mine) != 1 || mine[0]["subject"].(map[string]any)["name"] != "BIAS Campak" || mine[0]["student_name"] != "Budi" {
		t.Fatalf("consents/my: %+v", mine)
	}
	if r := e.do(http.MethodPost, "/consents/"+*parts[0].ConsentID+"/decide", e.token(newID(t, e.db), common.RoleParent), map[string]string{"decision": "granted"}); r.Code != http.StatusNotFound {
		t.Fatalf("stranger decide: %d", r.Code)
	}
	if r := e.do(http.MethodPost, "/consents/"+*parts[0].ConsentID+"/decide", parent, map[string]string{"decision": "granted"}); r.Code != http.StatusOK {
		t.Fatalf("grant: %d %s", r.Code, r.Body)
	}
	if r := e.do(http.MethodPost, "/consents/"+*parts[0].ConsentID+"/decide", e.token(p.parent2, common.RoleParent), map[string]string{"decision": "declined"}); r.Code != http.StatusConflict {
		t.Fatalf("second guardian: %d", r.Code)
	}
	if r := e.do(http.MethodPut, resultURL, uks, result); r.Code != http.StatusOK {
		t.Fatalf("record after consent: %d %s", r.Code, r.Body)
	}
	// Used consent cannot be revoked.
	if r := e.do(http.MethodPost, "/consents/"+*parts[0].ConsentID+"/decide", parent, map[string]string{"decision": "revoked"}); r.Code != http.StatusConflict {
		t.Fatalf("revoke used consent: %d", r.Code)
	}
	var recap struct {
		Recap health.Recap `json:"recap"`
	}
	e.do(http.MethodGet, "/health/programs/"+created.ID+"/recap", uks, nil).data(t, &recap)
	if recap.Recap.Total != 1 || recap.Recap.ByResult["done"] != 1 || recap.Recap.ByConsent["granted"] != 1 || recap.Recap.FollowUpOpen != 1 {
		t.Fatalf("recap: %+v", recap)
	}
	if r := e.do(http.MethodGet, "/health/programs/"+created.ID+"/recap", e.token(p.kepsek, common.RoleKepalaSekolah), nil); r.Code != http.StatusForbidden {
		t.Fatalf("kepsek recap without policy: %d", r.Code)
	}
	if r := e.do(http.MethodPost, "/health/programs/"+created.ID+"/participants/"+parts[0].ID+"/follow-up-done", uks, nil); r.Code != http.StatusOK {
		t.Fatalf("follow-up done: %d", r.Code)
	}
	var enc []byte
	_ = e.db.QueryRow(`SELECT result_enc FROM health_program_participants WHERE id = $1`, parts[0].ID).Scan(&enc)
	if len(enc) == 0 || bytes.Contains(enc, []byte("Vaksin")) {
		t.Fatal("result not encrypted")
	}
}

// FR-127-002 + §4.5.
func TestProfileChangeApproval_FR127002(t *testing.T) {
	e := setup(t)
	p := e.people()
	parent := e.token(p.parent, common.RoleParent)
	path := "/health/students/" + p.student + "/profile-changes"
	if r := e.do(http.MethodPost, path, parent, peanutProfile, "Idempotency-Key", "chg-1"); r.Code != http.StatusUnprocessableEntity {
		t.Fatalf("no flow configured must be 422: %d %s", r.Code, r.Body)
	}
	if n := e.count(`SELECT count(*) FROM health_profile_changes WHERE school_id = $1`, e.school); n != 0 {
		t.Fatalf("change kept without approval: %d", n)
	}
	e.world.flowConfigured = true
	if r := e.do(http.MethodPost, path, e.token(newID(t, e.db), common.RoleParent), peanutProfile, "Idempotency-Key", "x"); r.Code != http.StatusNotFound {
		t.Fatalf("stranger proposal: %d", r.Code)
	}
	r := e.do(http.MethodPost, path, parent, peanutProfile, "Idempotency-Key", "chg-1")
	if r.Code != http.StatusCreated {
		t.Fatalf("proposal: %d %s", r.Code, r.Body)
	}
	var ch health.ChangeView
	r.data(t, &ch)
	ap := e.world.approvals[0]
	if ap["request_type"] != "health_profile_update" || ap["idempotency_key"] != "health_profile_update:"+ch.ID || strings.Contains(stringify(ap["summary"]), "Kacang") {
		t.Fatalf("approval request: %+v", ap)
	}
	// Same key → same proposal, no new approval.
	if r := e.do(http.MethodPost, path, parent, peanutProfile, "Idempotency-Key", "chg-1"); r.Code != http.StatusOK || len(e.world.approvals) != 1 {
		t.Fatalf("repeat proposal: %d approvals=%d", r.Code, len(e.world.approvals))
	}
	// UKS sees proposal vs current.
	var detail health.ChangeView
	e.do(http.MethodGet, "/health/profile-changes/"+ch.ID, e.token(p.uks, common.RolePetugasUKS), nil).data(t, &detail)
	if detail.Proposed == nil || len(detail.Proposed.Allergies) != 1 || detail.Current != nil {
		t.Fatalf("change detail: %+v", detail)
	}
	if r := e.do(http.MethodGet, "/health/profile-changes/"+ch.ID, e.token(p.parent2, common.RoleParent), nil); r.Code != http.StatusNotFound {
		t.Fatalf("other guardian sees proposal: %d", r.Code)
	}
	cb := func(id, decision string) resp {
		return e.do(http.MethodPost, "/internal/approval-callbacks", "", map[string]string{"approval_id": "a", "source_id": id,
			"request_type": "health_profile_update", "decision": decision, "decided_by": p.uks}, "X-Internal-Key", testInternal)
	}
	if r := e.do(http.MethodPost, "/internal/approval-callbacks", "", map[string]string{}, "X-Internal-Key", "bad"); r.Code != http.StatusUnauthorized {
		t.Fatalf("callback bad key: %d", r.Code)
	}
	if r := cb(ch.ID, "approved"); r.Code != http.StatusOK {
		t.Fatalf("callback: %d %s", r.Code, r.Body)
	}
	var view health.ProfileView
	e.do(http.MethodGet, "/health/students/"+p.student+"/profile", parent, nil).data(t, &view)
	if view.Version != 1 || len(view.Profile.Allergies) != 1 || view.VerifiedBy == nil || *view.VerifiedBy != p.uks {
		t.Fatalf("applied profile: %+v", view)
	}
	// Idempotent repeat.
	if r := cb(ch.ID, "approved"); r.Code != http.StatusOK {
		t.Fatalf("repeat callback: %d", r.Code)
	}
	e.do(http.MethodGet, "/health/students/"+p.student+"/profile", parent, nil).data(t, &view)
	if view.Version != 1 {
		t.Fatalf("repeat callback re-applied: v%d", view.Version)
	}
	// Proposal on v1, UKS edits to v2, approval → superseded.
	var ch2 health.ChangeView
	e.do(http.MethodPost, path, parent, peanutProfile, "Idempotency-Key", "chg-2").data(t, &ch2)
	if r := e.do(http.MethodPut, "/health/students/"+p.student+"/profile", e.token(p.uks, common.RolePetugasUKS), peanutProfile); r.Code != http.StatusOK {
		t.Fatalf("uks edit: %d", r.Code)
	}
	var res map[string]string
	cb(ch2.ID, "approved").data(t, &res)
	if res["status"] != health.ChangeSuperseded {
		t.Fatalf("stale approval: %+v", res)
	}
	var ch3 health.ChangeView
	e.do(http.MethodPost, path, parent, peanutProfile, "Idempotency-Key", "chg-3").data(t, &ch3)
	cb(ch3.ID, "rejected").data(t, &res)
	if res["status"] != health.ChangeRejected {
		t.Fatalf("reject: %+v", res)
	}
	if e.world.notifications["studentlife:change:"+ch3.ID+":rejected"] != 1 {
		t.Fatalf("rejection notice missing: %+v", e.world.notifications)
	}
}

func stringify(v any) string { return fmtAny(v) }
