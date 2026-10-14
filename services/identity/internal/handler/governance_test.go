package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	"skci/identity/internal/client"
	"skci/identity/internal/common"
	"skci/identity/internal/model"
	"skci/identity/internal/security"
)

// ---------------------------------------------------------------------------
// Pure table tests
// ---------------------------------------------------------------------------

func TestValidateBreakGlass(t *testing.T) {
	user, school, me := newIdentityTestUUID(), newIdentityTestUUID(), newIdentityTestUUID()
	reason := "Bendahara sakit, tagihan jatuh tempo hari ini"
	cases := []struct {
		name  string
		in    breakGlassRequest
		field string
	}{
		{"ok", breakGlassRequest{UserID: user, SchoolID: school, Role: "finance_manager", DurationMinutes: 60, Reason: reason}, ""},
		{"self", breakGlassRequest{UserID: me, SchoolID: school, Role: "finance_manager", DurationMinutes: 60, Reason: reason}, "user_id"},
		{"bad school", breakGlassRequest{UserID: user, SchoolID: "x", Role: "admin", DurationMinutes: 60, Reason: reason}, "school_id"},
		{"superadmin", breakGlassRequest{UserID: user, SchoolID: school, Role: "superadmin", DurationMinutes: 60, Reason: reason}, "role"},
		{"too short", breakGlassRequest{UserID: user, SchoolID: school, Role: "admin", DurationMinutes: 5, Reason: reason}, "duration_minutes"},
		{"over 24h", breakGlassRequest{UserID: user, SchoolID: school, Role: "admin", DurationMinutes: 1441, Reason: reason}, "duration_minutes"},
		{"short reason", breakGlassRequest{UserID: user, SchoolID: school, Role: "admin", DurationMinutes: 60, Reason: "darurat"}, "reason"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			errs := validateBreakGlass(&c.in, me)
			if c.field == "" && len(errs) > 0 {
				t.Fatalf("unexpected errors %v", errs)
			}
			if c.field != "" && errs[c.field] == "" {
				t.Fatalf("want error on %s, got %v", c.field, errs)
			}
		})
	}
}

func TestValidateCorrections(t *testing.T) {
	cases := []struct {
		name  string
		in    map[string]string
		field string
	}{
		{"empty", map[string]string{}, "corrections"},
		{"ok", map[string]string{"name": "Budi Santoso", "nik": "3201010101010001"}, ""},
		{"email not correctable", map[string]string{"email": "x@y"}, "email"},
		{"bad nik", map[string]string{"nik": "123"}, "nik"},
		{"bad nisn", map[string]string{"nisn": "12"}, "nisn"},
		{"bad date", map[string]string{"birth_date": "31-12-2010"}, "birth_date"},
		{"blank name", map[string]string{"name": "  "}, "name"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			errs := validateCorrections(c.in)
			if c.field == "" && len(errs) > 0 {
				t.Fatalf("unexpected %v", errs)
			}
			if c.field != "" && errs[c.field] == "" {
				t.Fatalf("want error on %s, got %v", c.field, errs)
			}
		})
	}
}

func TestWithoutExcepted(t *testing.T) {
	found := []security.RoleConflict{{RoleA: "admin", RoleB: "auditor"}, {RoleA: "auditor", RoleB: "finance_manager"}}
	cases := []struct {
		name     string
		excepted map[string]bool
		want     int
	}{
		{"none", nil, 2},
		{"one pair, reversed order", map[string]bool{security.PairKey("auditor", "admin"): true}, 1},
		{"all", map[string]bool{security.PairKey("admin", "auditor"): true, security.PairKey("finance_manager", "auditor"): true}, 0},
	}
	for _, c := range cases {
		if got := security.WithoutExcepted(found, c.excepted); len(got) != c.want {
			t.Errorf("%s: got %d want %d", c.name, len(got), c.want)
		}
	}
}

func TestValidateAnnouncementRequest(t *testing.T) {
	now := time.Now()
	future := now.Add(2 * time.Hour).Format(time.RFC3339)
	tooFar := now.AddDate(2, 0, 0).Format(time.RFC3339)
	bad := "besok"
	cases := []struct {
		name  string
		in    announcementRequest
		field string
	}{
		{"no audience (BUG-153)", announcementRequest{Title: "A", Message: "B"}, "target_roles"},
		{"all roles", announcementRequest{Title: "A", Message: "B", AudienceAll: true}, ""},
		{"roles", announcementRequest{Title: "A", Message: "B", TargetRoles: []string{"siswa"}}, ""},
		{"unknown role", announcementRequest{Title: "A", Message: "B", TargetRoles: []string{"alien"}}, "target_roles"},
		{"no title", announcementRequest{Message: "B", AudienceAll: true}, "title"},
		{"bad class id", announcementRequest{Title: "A", Message: "B", AudienceAll: true, TargetClassIDs: []string{"x"}}, "target_class_ids"},
		{"scheduled", announcementRequest{Title: "A", Message: "B", AudienceAll: true, PublishAt: &future}, ""},
		{"schedule too far", announcementRequest{Title: "A", Message: "B", AudienceAll: true, PublishAt: &tooFar}, "publish_at"},
		{"bad schedule", announcementRequest{Title: "A", Message: "B", AudienceAll: true, PublishAt: &bad}, "publish_at"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, errs := validateAnnouncementRequest(&c.in, now)
			if c.field == "" && errs != nil {
				t.Fatalf("unexpected %v", errs)
			}
			if c.field != "" && errs[c.field] == "" {
				t.Fatalf("want error on %s, got %v", c.field, errs)
			}
		})
	}
}

func TestNeedsSendNowConfirmation(t *testing.T) {
	now := time.Now()
	past, future := now.Add(-time.Hour), now.Add(time.Hour)
	cases := []struct {
		name                        string
		publish, wasLive, confirmed bool
		at                          *time.Time
		want                        bool
	}{
		{"draft", false, false, false, nil, false},
		{"publish now unconfirmed", true, false, false, nil, true},
		{"publish past unconfirmed", true, false, false, &past, true},
		{"publish now confirmed", true, false, true, nil, false},
		{"scheduled", true, false, false, &future, false},
		{"already live edit", true, true, false, nil, false},
	}
	for _, c := range cases {
		if got := needsSendNowConfirmation(c.publish, c.at, c.wasLive, c.confirmed, now); got != c.want {
			t.Errorf("%s: got %v want %v", c.name, got, c.want)
		}
	}
}

func TestAnnouncementVisibleTo(t *testing.T) {
	now := time.Now()
	unit, other := "u1", "u2"
	future := now.Add(time.Hour)
	mk := func(f func(*model.Announcement)) *model.Announcement {
		an := &model.Announcement{Published: true, SchoolID: &unit, AudienceAll: true}
		f(an)
		return an
	}
	siswa := viewer{SchoolID: unit, Role: "siswa", Classes: map[string]bool{"c1": true}}
	cases := []struct {
		name string
		an   *model.Announcement
		v    viewer
		want bool
	}{
		{"all roles", mk(func(*model.Announcement) {}), siswa, true},
		{"draft", mk(func(a *model.Announcement) { a.Published = false }), siswa, false},
		{"scheduled", mk(func(a *model.Announcement) { a.PublishAt = &future }), siswa, false},
		{"other unit", mk(func(a *model.Announcement) { a.SchoolID = &other }), siswa, false},
		{"role mismatch", mk(func(a *model.Announcement) { a.AudienceAll = false; a.TargetRoles = []string{"guru"} }), siswa, false},
		{"role match", mk(func(a *model.Announcement) { a.AudienceAll = false; a.TargetRoles = []string{"siswa"} }), siswa, true},
		{"class match", mk(func(a *model.Announcement) { a.TargetClassIDs = []string{"c1"} }), siswa, true},
		{"class mismatch", mk(func(a *model.Announcement) { a.TargetClassIDs = []string{"c2"} }), siswa, false},
		{"class unknown membership", mk(func(a *model.Announcement) { a.TargetClassIDs = []string{"c1"} }), viewer{SchoolID: unit, Role: "siswa"}, false},
		{"global limited to units", mk(func(a *model.Announcement) { a.SchoolID = nil; a.TargetSchoolIDs = []string{other} }), siswa, false},
		{"global for my unit", mk(func(a *model.Announcement) { a.SchoolID = nil; a.TargetSchoolIDs = []string{unit} }), siswa, true},
	}
	for _, c := range cases {
		if got := announcementVisibleTo(c.an, c.v, now); got != c.want {
			t.Errorf("%s: got %v want %v", c.name, got, c.want)
		}
	}
}

func TestValidatePermissionUpdate(t *testing.T) {
	cases := []struct {
		name string
		in   permissionUpdate
		ok   bool
	}{
		{"ok", permissionUpdate{Role: "admin", Module: "announcements", Allowed: map[string]bool{"create": false}}, true},
		{"superadmin locked", permissionUpdate{Role: "superadmin", Module: "announcements"}, false},
		{"unknown module", permissionUpdate{Role: "admin", Module: "nuklir"}, false},
		{"action not in module", permissionUpdate{Role: "admin", Module: "references", Allowed: map[string]bool{"delete": true}}, false},
		{"reset", permissionUpdate{Role: "guru", Module: "guardians", Reset: true}, true},
	}
	for _, c := range cases {
		if got := validatePermissionUpdate(c.in) == ""; got != c.ok {
			t.Errorf("%s: ok=%v want %v", c.name, got, c.ok)
		}
	}
}

func TestValidateReference(t *testing.T) {
	cases := []struct {
		name   string
		in     referenceInput
		create bool
		field  string
	}{
		{"ok", referenceInput{Category: "program", Code: "IPA", Label: "IPA"}, true, ""},
		{"bad category", referenceInput{Category: "warna", Code: "x", Label: "x"}, true, "category"},
		{"bad code", referenceInput{Category: "program", Code: "ip a!", Label: "x"}, true, "code"},
		{"empty label", referenceInput{Category: "program", Code: "x", Label: " "}, true, "label"},
		{"update ignores code", referenceInput{Label: "Baru"}, false, ""},
	}
	for _, c := range cases {
		errs := validateReference(&c.in, c.create)
		if (c.field == "" && len(errs) > 0) || (c.field != "" && errs[c.field] == "") {
			t.Errorf("%s: errs=%v want field %q", c.name, errs, c.field)
		}
	}
}

// ---------------------------------------------------------------------------
// PG flows. Claims are injected from the X-Test-Claims header
// ("userID|role|schoolID"), standing in for JWTSessionMiddleware.
// ---------------------------------------------------------------------------

func claimsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if v := r.Header.Get("X-Test-Claims"); v != "" {
			p := strings.SplitN(v, "|", 3)
			c := &common.Claims{UserID: p[0], ActiveRole: p[1], SchoolID: p[2]}
			r = r.WithContext(context.WithValue(r.Context(), common.ClaimsCtxKey, c))
		}
		next.ServeHTTP(w, r)
	})
}

func governanceRouter(app *App) http.Handler {
	r := chi.NewRouter()
	r.Group(func(pr chi.Router) {
		pr.Use(claimsMiddleware)
		pr.Post("/users/{id}/roles", app.AssignUserRole)
		pr.Put("/users/{id}", app.UpdateUser)
		pr.Get("/security/role-conflicts/violations", app.ListRoleConflictViolations)
		pr.Get("/security/role-conflicts/cleanup-log", app.ListSoDCleanupLog)
		pr.Post("/security/role-conflicts/violations/resolve", app.ResolveRoleConflictViolations)
		pr.Get("/security/role-conflict-exceptions", app.ListRoleConflictExceptions)
		pr.Post("/security/role-conflict-exceptions/{id}/revoke", app.RevokeRoleConflictException)
		pr.Get("/security/break-glass", app.ListBreakGlass)
		pr.Post("/security/break-glass", app.CreateBreakGlass)
		pr.Post("/security/break-glass/{id}/revoke", app.RevokeBreakGlass)
		pr.Get("/security/permissions", app.GetPermissionMatrix)
		pr.Put("/security/permissions", app.UpdatePermissionMatrix)
		pr.Get("/me/permissions", app.MyPermissions)
		pr.Get("/privacy/requests", app.ListDataSubjectRequests)
		pr.Post("/privacy/requests", app.CreateDataSubjectRequest)
		pr.Get("/privacy/requests/mine", app.MyDataSubjectRequests)
		pr.Post("/privacy/requests/{id}/start", app.StartDataSubjectRequest)
		pr.Post("/privacy/requests/{id}/complete", app.CompleteDataSubjectRequest)
		pr.Post("/privacy/requests/{id}/reject", app.RejectDataSubjectRequest)
		pr.Get("/privacy/requests/{id}/export", app.DataSubjectRequestExport)
		pr.Get("/references", app.ListReferences)
		pr.Post("/references", app.CreateReference)
		pr.Put("/references/{id}", app.UpdateReference)
		pr.Get("/announcements", app.ListVisibleAnnouncements)
		pr.Get("/announcements/all", app.ListAllAnnouncements)
		pr.Post("/announcements", app.CreateAnnouncement)
		pr.Get("/announcements/{id}", app.GetAnnouncement)
		pr.Put("/announcements/{id}", app.UpdateAnnouncement)
		pr.Post("/announcements/{id}/publish", app.PublishAnnouncement)
		pr.Post("/announcements/{id}/unpublish", app.UnpublishAnnouncement)
	})
	r.Group(func(in chi.Router) {
		in.Use(common.InternalOnlyMiddleware("internal-key"))
		in.Post("/internal/role-conflict-exceptions/approval-callback", app.RoleConflictExceptionCallback)
		in.Get("/internal/permissions", app.InternalPermissions)
	})
	return r
}

func govCall(t *testing.T, h http.Handler, method, path, claims string, body any) (int, map[string]any) {
	t.Helper()
	var buf bytes.Buffer
	if body != nil {
		_ = json.NewEncoder(&buf).Encode(body)
	}
	req := httptest.NewRequest(method, path, &buf)
	req.Header.Set("Content-Type", "application/json")
	if claims == "internal" {
		req.Header.Set("X-Internal-Key", "internal-key")
	} else if claims != "" {
		req.Header.Set("X-Test-Claims", claims)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	out := map[string]any{}
	_ = json.Unmarshal(rec.Body.Bytes(), &out)
	return rec.Code, out
}

func dataList(body map[string]any) []any {
	l, _ := body["data"].([]any)
	return l
}

func claimsFor(userID, role, school string) string { return userID + "|" + role + "|" + school }

func seedAccount(t *testing.T, app *App, schoolID string, roles ...string) string {
	t.Helper()
	id := newIdentityTestUUID()
	mustExec(t, app.DB, `INSERT INTO users (id, name, email, password_hash) VALUES ($1, $2, $3, 'x')`, id, "Akun "+id[:8], "gov-"+id[:8]+"@test.local")
	for _, ro := range roles {
		mustExec(t, app.DB, `INSERT INTO user_school_roles (user_id, school_id, role) VALUES ($1,$2,$3)`, id, schoolID, ro)
	}
	return id
}

// fakeCore records approval submissions and answers like core.
type fakeCore struct {
	mu        sync.Mutex
	submitted []map[string]any
	reject    string // non-empty: answer 422 with this message
}

func (f *fakeCore) server(t *testing.T) *httptest.Server {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Internal-Key") != "internal-key" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		body := map[string]any{}
		_ = json.NewDecoder(r.Body).Decode(&body)
		switch {
		case r.URL.Path == "/internal/approvals":
			f.mu.Lock()
			f.submitted = append(f.submitted, body)
			f.mu.Unlock()
			if f.reject != "" {
				common.WriteError(w, http.StatusUnprocessableEntity, f.reject)
				return
			}
			common.WriteJSON(w, http.StatusCreated, map[string]string{"id": newIdentityTestUUID(), "status": "pending"}, nil)
		case strings.HasSuffix(r.URL.Path, "/cancel"):
			common.WriteJSON(w, http.StatusOK, map[string]string{"status": "cancelled"}, nil)
		default:
			// audit and anything else
			common.WriteJSON(w, http.StatusOK, nil, nil)
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestSoDExceptionApprovalFlow(t *testing.T) {
	db := openTestDB(t)
	app := newTestApp(db)
	core := &fakeCore{}
	app.CoreURL = core.server(t).URL
	app.SelfURL = "http://identity.test"
	h := governanceRouter(app)

	admin := seedUser(t, db, common.RoleAdmin)
	target := seedAccount(t, app, admin.schoolID, common.RoleSiswa)
	asAdmin := claimsFor(admin.userID, common.RoleAdmin, admin.schoolID)

	// Forbidden combination without a reason: 409 that offers an exception.
	code, body := govCall(t, h, http.MethodPost, "/users/"+target+"/roles", asAdmin, map[string]string{"role": common.RoleGuru})
	errs, _ := body["errors"].(map[string]any)
	if code != http.StatusConflict || errs["exception_allowed"] != true {
		t.Fatalf("want 409 with exception_allowed, got %d %v", code, body)
	}
	// Reason too short.
	if code, _ := govCall(t, h, http.MethodPost, "/users/"+target+"/roles", asAdmin, map[string]string{"role": common.RoleGuru, "exception_reason": "singkat"}); code != http.StatusBadRequest {
		t.Fatalf("short reason: got %d", code)
	}
	// Core does not know the request type yet: stored as failed with the
	// readable core message, role not granted.
	core.reject = "request_type tidak dikenal"
	reason := "Unit kecil, hanya satu orang yang paham keuangan sementara audit internal"
	code, body = govCall(t, h, http.MethodPost, "/users/"+target+"/roles", asAdmin, map[string]string{"role": common.RoleGuru, "exception_reason": reason})
	if code != http.StatusBadGateway || !strings.Contains(body["message"].(string), "request_type tidak dikenal") || dataMap(body)["status"] != "failed" {
		t.Fatalf("core 422: got %d %v", code, body)
	}
	// Configured core: 202 pending, then approved callback grants the role.
	core.reject = ""
	code, body = govCall(t, h, http.MethodPost, "/users/"+target+"/roles", asAdmin, map[string]string{"role": common.RoleGuru, "exception_reason": reason, "valid_until": time.Now().AddDate(0, 3, 0).Format("2006-01-02")})
	if code != http.StatusAccepted {
		t.Fatalf("submit: got %d %v", code, body)
	}
	exID := dataMap(body)["id"].(string)
	last := core.submitted[len(core.submitted)-1]
	if last["request_type"] != "role_conflict_exception" || last["source_id"] != exID ||
		last["callback_url"] != "http://identity.test/internal/role-conflict-exceptions/approval-callback" {
		t.Fatalf("core payload %v", last)
	}
	var held int
	_ = db.QueryRow(`SELECT count(*) FROM user_school_roles WHERE user_id=$1 AND role='guru'`, target).Scan(&held)
	if held != 0 {
		t.Fatal("role must not be granted before approval")
	}
	// A second request while one is open: 409.
	if code, _ := govCall(t, h, http.MethodPost, "/users/"+target+"/roles", asAdmin, map[string]string{"role": common.RoleGuru, "exception_reason": reason}); code != http.StatusConflict {
		t.Fatalf("duplicate open request: got %d", code)
	}
	// Callback requires the internal key.
	cb := map[string]string{"approval_id": newIdentityTestUUID(), "source_id": exID, "request_type": "role_conflict_exception", "decision": "approved", "decided_by": newIdentityTestUUID()}
	if code, _ := govCall(t, h, http.MethodPost, "/internal/role-conflict-exceptions/approval-callback", asAdmin, cb); code != http.StatusUnauthorized && code != http.StatusForbidden {
		t.Fatalf("callback without key: %d", code)
	}
	if code, body := govCall(t, h, http.MethodPost, "/internal/role-conflict-exceptions/approval-callback", "internal", cb); code != http.StatusOK || dataMap(body)["status"] != "approved" {
		t.Fatalf("callback: %d %v", code, body)
	}
	// Idempotent replay.
	if code, _ := govCall(t, h, http.MethodPost, "/internal/role-conflict-exceptions/approval-callback", "internal", cb); code != http.StatusOK {
		t.Fatalf("replay: %d", code)
	}
	_ = db.QueryRow(`SELECT count(*) FROM user_school_roles WHERE user_id=$1 AND role='guru'`, target).Scan(&held)
	if held != 1 {
		t.Fatal("approved exception must grant the role")
	}
	// The approved pair is no longer reported as a violation.
	_, body = govCall(t, h, http.MethodGet, "/security/role-conflicts/violations", claimsFor(newIdentityTestUUID(), common.RoleSuperadmin, ""), nil)
	for _, v := range dataList(body) {
		if m, _ := v.(map[string]any); m["user_id"] == target {
			t.Fatalf("excepted pair still listed as violation: %v", m)
		}
	}
	// Expiry ends it automatically.
	mustExec(t, db, `UPDATE role_conflict_exceptions SET valid_until = current_date - 1 WHERE id=$1`, exID)
	if n := app.RunRoleConflictExceptionExpiry(context.Background()); n < 1 {
		t.Fatal("expiry did not run")
	}
	_ = db.QueryRow(`SELECT count(*) FROM user_school_roles WHERE user_id=$1 AND role='guru'`, target).Scan(&held)
	if held != 0 {
		t.Fatal("expired exception must remove the role")
	}

	// Rejected decision: nothing granted. Revoke of a pending one cancels.
	code, body = govCall(t, h, http.MethodPost, "/users/"+target+"/roles", asAdmin, map[string]string{"role": common.RoleOrangTua, "exception_reason": reason})
	if code != http.StatusAccepted {
		t.Fatalf("second submit %d %v", code, body)
	}
	ex2 := dataMap(body)["id"].(string)
	if code, body := govCall(t, h, http.MethodPost, "/security/role-conflict-exceptions/"+ex2+"/revoke", asAdmin, map[string]string{"reason": "Tidak jadi diperlukan"}); code != http.StatusOK || dataMap(body)["status"] != "revoked" {
		t.Fatalf("revoke pending: %d %v", code, body)
	}
	// Other unit's admin cannot see it.
	other := seedUser(t, db, common.RoleAdmin)
	if code, _ := govCall(t, h, http.MethodPost, "/security/role-conflict-exceptions/"+ex2+"/revoke", claimsFor(other.userID, common.RoleAdmin, other.schoolID), map[string]string{"reason": "Tidak jadi diperlukan"}); code != http.StatusNotFound {
		t.Fatalf("foreign admin revoke: %d", code)
	}
}

func TestSoDResolveViolationsCleanup(t *testing.T) {
	db := openTestDB(t)
	app := newTestApp(db)
	h := governanceRouter(app)
	admin := seedUser(t, db, common.RoleAdmin)
	// An account like the production admin holding many conflicting roles.
	many := seedAccount(t, app, admin.schoolID, common.RoleAdmin, common.RoleAuditor, common.RoleSiswa, common.RoleGuru,
		common.RoleFinanceManager, common.RolePayrollOfficer, common.RoleHRManager)
	super := claimsFor(newIdentityTestUUID(), common.RoleSuperadmin, "")
	if code, _ := govCall(t, h, http.MethodPost, "/security/role-conflicts/violations/resolve", claimsFor(admin.userID, common.RoleAdmin, admin.schoolID), map[string]string{"reason": "Pembersihan SoD"}); code != http.StatusForbidden {
		t.Fatalf("admin must not run clean-up: %d", code)
	}
	code, body := govCall(t, h, http.MethodPost, "/security/role-conflicts/violations/resolve", super, map[string]string{"reason": "Pembersihan SoD QA"})
	if code != http.StatusOK {
		t.Fatalf("resolve %d %v", code, body)
	}
	var violations int
	_ = db.QueryRow(`SELECT count(*) FROM user_school_roles a JOIN user_school_roles b ON a.user_id=b.user_id AND a.role<b.role
		JOIN role_conflicts c ON c.role_a=a.role AND c.role_b=b.role WHERE a.user_id=$1`, many).Scan(&violations)
	if violations != 0 {
		t.Fatalf("%d violations remain", violations)
	}
	var kept []string
	rows, _ := db.Query(`SELECT role FROM user_school_roles WHERE user_id=$1 ORDER BY role`, many)
	for rows.Next() {
		var r string
		_ = rows.Scan(&r)
		kept = append(kept, r)
	}
	rows.Close()
	if !contains(kept, common.RoleAdmin) {
		t.Fatalf("admin role should be kept (highest rank), kept %v", kept)
	}
	var logged int
	_ = db.QueryRow(`SELECT count(*) FROM role_sod_cleanup_log WHERE user_id=$1`, many).Scan(&logged)
	if logged == 0 || logged+len(kept) != 7 {
		t.Fatalf("logged %d kept %v", logged, kept)
	}
	_, body = govCall(t, h, http.MethodGet, "/security/role-conflicts/cleanup-log", super, nil)
	if len(dataList(body)) == 0 {
		t.Fatal("cleanup log empty")
	}
}

func TestBreakGlassGrantAndExpiry(t *testing.T) {
	db := openTestDB(t)
	app := newTestApp(db)
	h := governanceRouter(app)
	admin := seedUser(t, db, common.RoleAdmin)
	target := seedAccount(t, app, admin.schoolID, common.RoleGuru)
	superID := seedAccount(t, app, admin.schoolID)
	super := claimsFor(superID, common.RoleSuperadmin, "")
	in := map[string]any{"user_id": target, "school_id": admin.schoolID, "role": common.RoleFinanceManager, "duration_minutes": 60,
		"reason": "Bendahara sakit, tagihan jatuh tempo hari ini", "incident_ref": "INC-1"}
	if code, _ := govCall(t, h, http.MethodPost, "/security/break-glass", claimsFor(admin.userID, common.RoleAdmin, admin.schoolID), in); code != http.StatusForbidden {
		t.Fatalf("admin grant: %d", code)
	}
	code, body := govCall(t, h, http.MethodPost, "/security/break-glass", super, in)
	if code != http.StatusCreated {
		t.Fatalf("grant %d %v", code, body)
	}
	id := dataMap(body)["id"].(string)
	if code, _ := govCall(t, h, http.MethodPost, "/security/break-glass", super, in); code != http.StatusConflict {
		t.Fatalf("second grant while active: %d", code)
	}
	var held int
	_ = db.QueryRow(`SELECT count(*) FROM user_school_roles WHERE user_id=$1 AND role='finance_manager'`, target).Scan(&held)
	if held != 1 {
		t.Fatal("grant must add the role")
	}
	// Activity during the grant shows in the summary.
	mustExec(t, db, `INSERT INTO login_events (user_id, email, success, reason, ip, created_at) VALUES ($1, 'x', true, 'ok', '10.0.0.1', now())`, target)
	// Time passes.
	mustExec(t, db, `UPDATE break_glass_grants SET starts_at = now() - interval '2 hours', expires_at = now() - interval '1 hour' WHERE id=$1`, id)
	if n := app.RunBreakGlassExpiry(context.Background()); n < 1 {
		t.Fatal("expiry did not end the grant")
	}
	_ = db.QueryRow(`SELECT count(*) FROM user_school_roles WHERE user_id=$1 AND role='finance_manager'`, target).Scan(&held)
	if held != 0 {
		t.Fatal("expired grant must remove the role")
	}
	var endReason, summaryStatus string
	var summary []byte
	_ = db.QueryRow(`SELECT end_reason, coalesce(summary_status,''), summary FROM break_glass_grants WHERE id=$1`, id).Scan(&endReason, &summaryStatus, &summary)
	if endReason != "expired" || !strings.Contains(string(summary), "10.0.0.1") || summaryStatus == "" {
		t.Fatalf("end=%s status=%s summary=%s", endReason, summaryStatus, summary)
	}
	// The guru role of the account is untouched.
	_ = db.QueryRow(`SELECT count(*) FROM user_school_roles WHERE user_id=$1 AND role='guru'`, target).Scan(&held)
	if held != 1 {
		t.Fatal("pre-existing role removed")
	}
	// Manual revoke path.
	code, body = govCall(t, h, http.MethodPost, "/security/break-glass", super, in)
	if code != http.StatusCreated {
		t.Fatalf("regrant %d %v", code, body)
	}
	id2 := dataMap(body)["id"].(string)
	if code, body := govCall(t, h, http.MethodPost, "/security/break-glass/"+id2+"/revoke", super, map[string]string{"reason": "Bendahara sudah kembali"}); code != http.StatusOK {
		t.Fatalf("revoke %d %v", code, body)
	}
	_, body = govCall(t, h, http.MethodGet, "/security/break-glass?active=true", super, nil)
	for _, g := range dataList(body) {
		if m := g.(map[string]any); m["id"] == id2 {
			t.Fatal("revoked grant still active")
		}
	}
}

func TestDataSubjectRequestFlow(t *testing.T) {
	db := openTestDB(t)
	app := newTestApp(db)
	h := governanceRouter(app)
	admin := seedUser(t, db, common.RoleAdmin)
	asAdmin := claimsFor(admin.userID, common.RoleAdmin, admin.schoolID)
	subject := seedAccount(t, app, admin.schoolID, common.RoleSiswa)
	asSubject := claimsFor(subject, common.RoleSiswa, admin.schoolID)
	stranger := seedAccount(t, app, admin.schoolID, common.RoleSiswa)

	if code, _ := govCall(t, h, http.MethodPost, "/privacy/requests", asSubject, map[string]any{"request_type": "access", "details": "x"}); code != http.StatusBadRequest {
		t.Fatalf("short details: %d", code)
	}
	if code, _ := govCall(t, h, http.MethodPost, "/privacy/requests", asSubject, map[string]any{"request_type": "access", "details": "Minta salinan data saya", "subject_user_id": stranger}); code != http.StatusForbidden {
		t.Fatalf("filing for a stranger: %d", code)
	}
	code, body := govCall(t, h, http.MethodPost, "/privacy/requests", asSubject, map[string]any{"request_type": "access", "details": "Minta salinan data saya"})
	if code != http.StatusCreated {
		t.Fatalf("create %d %v", code, body)
	}
	accessID := dataMap(body)["id"].(string)
	if code, _ := govCall(t, h, http.MethodPost, "/privacy/requests/"+accessID+"/complete", asSubject, map[string]any{"resolution": "Selesai diproses"}); code != http.StatusForbidden {
		t.Fatalf("subject completing own request: %d", code)
	}
	code, body = govCall(t, h, http.MethodPost, "/privacy/requests/"+accessID+"/complete", asAdmin, map[string]any{"resolution": "Salinan data disiapkan"})
	if code != http.StatusOK || dataMap(body)["has_export"] != true {
		t.Fatalf("complete access %d %v", code, body)
	}
	if code, _ := govCall(t, h, http.MethodGet, "/privacy/requests/"+accessID+"/export", asAdmin, nil); code != http.StatusOK {
		t.Fatalf("export %d", code)
	}
	// Correction with a religion from the reference list.
	code, body = govCall(t, h, http.MethodPost, "/privacy/requests", asSubject, map[string]any{"request_type": "correction", "details": "Nama dan agama salah tulis",
		"corrections": map[string]string{"name": "Nama Benar", "religion": "islam"}})
	if code != http.StatusCreated {
		t.Fatalf("correction create %d %v", code, body)
	}
	corrID := dataMap(body)["id"].(string)
	if code, body := govCall(t, h, http.MethodPost, "/privacy/requests/"+corrID+"/complete", asAdmin, map[string]any{"resolution": "Data sudah dikoreksi"}); code != http.StatusOK {
		t.Fatalf("complete correction %d %v", code, body)
	}
	var name, religion string
	_ = db.QueryRow(`SELECT name, coalesce(religion,'') FROM users WHERE id=$1`, subject).Scan(&name, &religion)
	if name != "Nama Benar" || religion != "Islam" {
		t.Fatalf("corrections not applied: %s/%s", name, religion)
	}
	// Deletion needs confirmation, then anonymizes.
	code, body = govCall(t, h, http.MethodPost, "/privacy/requests", asSubject, map[string]any{"request_type": "deletion", "details": "Hapus data saya karena pindah"})
	if code != http.StatusCreated {
		t.Fatalf("deletion create %d %v", code, body)
	}
	delID := dataMap(body)["id"].(string)
	if code, body := govCall(t, h, http.MethodPost, "/privacy/requests/"+delID+"/complete", asAdmin, map[string]any{"resolution": "Data dianonimkan"}); code != http.StatusConflict || dataMap(body)["requires_confirmation"] != true {
		t.Fatalf("deletion without confirm %d %v", code, body)
	}
	if code, body := govCall(t, h, http.MethodPost, "/privacy/requests/"+delID+"/complete", asAdmin, map[string]any{"resolution": "Data dianonimkan", "confirm": true}); code != http.StatusOK {
		t.Fatalf("deletion %d %v", code, body)
	}
	var email string
	var anonymized bool
	_ = db.QueryRow(`SELECT email, anonymized_at IS NOT NULL FROM users WHERE id=$1`, subject).Scan(&email, &anonymized)
	if !anonymized || !strings.HasPrefix(email, "pdp-deleted-") {
		t.Fatalf("not anonymized: %s %v", email, anonymized)
	}
	_, body = govCall(t, h, http.MethodGet, "/privacy/requests", asAdmin, nil)
	if len(dataList(body)) < 3 {
		t.Fatalf("admin list %v", body)
	}
}

func TestReferencesAndReligionValidation(t *testing.T) {
	db := openTestDB(t)
	app := newTestApp(db)
	h := governanceRouter(app)
	admin := seedUser(t, db, common.RoleAdmin)
	asAdmin := claimsFor(admin.userID, common.RoleAdmin, admin.schoolID)
	asGuru := claimsFor(newIdentityTestUUID(), common.RoleGuru, admin.schoolID)

	_, body := govCall(t, h, http.MethodGet, "/references?category=agama", asGuru, nil)
	if len(dataList(body)) != 7 {
		t.Fatalf("agama seeds: %v", body)
	}
	if code, _ := govCall(t, h, http.MethodPost, "/references", asGuru, map[string]any{"category": "program", "code": "ipa", "label": "IPA"}); code != http.StatusForbidden {
		t.Fatalf("guru create: %d", code)
	}
	code, body := govCall(t, h, http.MethodPost, "/references", asAdmin, map[string]any{"category": "program", "code": "ipa", "label": "IPA"})
	if code != http.StatusCreated {
		t.Fatalf("admin create %d %v", code, body)
	}
	progID := dataMap(body)["id"].(string)
	if code, _ := govCall(t, h, http.MethodPost, "/references", asAdmin, map[string]any{"category": "program", "code": "ipa", "label": "IPA lagi"}); code != http.StatusConflict {
		t.Fatalf("duplicate code: %d", code)
	}
	off := false
	if code, body := govCall(t, h, http.MethodPut, "/references/"+progID, asAdmin, map[string]any{"label": "IPA", "active": off}); code != http.StatusOK || dataMap(body)["active"] != false {
		t.Fatalf("deactivate %d %v", code, body)
	}
	// Global rows are superadmin-only.
	_, body = govCall(t, h, http.MethodGet, "/references?category=agama", asAdmin, nil)
	globalID := dataList(body)[0].(map[string]any)["id"].(string)
	if code, _ := govCall(t, h, http.MethodPut, "/references/"+globalID, asAdmin, map[string]any{"label": "Ubah"}); code != http.StatusForbidden {
		t.Fatalf("admin editing global: %d", code)
	}
	// Religion on a user must come from the list.
	u := seedAccount(t, app, admin.schoolID, common.RoleGuru)
	code, body = govCall(t, h, http.MethodPut, "/users/"+u, asAdmin, map[string]any{"name": "Guru A", "religion": "Jedi"})
	if code != http.StatusBadRequest {
		t.Fatalf("free-text religion: %d %v", code, body)
	}
	if code, body := govCall(t, h, http.MethodPut, "/users/"+u, asAdmin, map[string]any{"name": "Guru A", "religion": "katolik"}); code != http.StatusOK {
		t.Fatalf("valid religion: %d %v", code, body)
	}
	var religion string
	_ = db.QueryRow(`SELECT religion FROM users WHERE id=$1`, u).Scan(&religion)
	if religion != "Katolik" {
		t.Fatalf("stored %q", religion)
	}
}

func TestPermissionMatrixOverridesDefaults(t *testing.T) {
	db := openTestDB(t)
	app := newTestApp(db)
	h := governanceRouter(app)
	admin := seedUser(t, db, common.RoleAdmin)
	asAdmin := claimsFor(admin.userID, common.RoleAdmin, admin.schoolID)
	super := claimsFor(seedAccount(t, app, admin.schoolID), common.RoleSuperadmin, "")
	t.Cleanup(func() { mustExec(t, db, `DELETE FROM role_permissions WHERE school_id=$1`, admin.schoolID) })

	_, body := govCall(t, h, http.MethodGet, "/me/permissions", asAdmin, nil)
	if dataMap(body)["references"].(map[string]any)["create"] != true {
		t.Fatalf("default admin perms %v", body)
	}
	if code, _ := govCall(t, h, http.MethodPut, "/security/permissions", asAdmin, map[string]any{"items": []any{}}); code != http.StatusForbidden {
		t.Fatalf("admin editing matrix: %d", code)
	}
	upd := map[string]any{"school_id": admin.schoolID, "items": []map[string]any{{"role": "admin", "module": "references", "allowed": map[string]bool{"create": false}}}}
	if code, body := govCall(t, h, http.MethodPut, "/security/permissions", super, upd); code != http.StatusOK {
		t.Fatalf("update %d %v", code, body)
	}
	if code, body := govCall(t, h, http.MethodPost, "/references", asAdmin, map[string]any{"category": "program", "code": "ips", "label": "IPS"}); code != http.StatusForbidden || !strings.Contains(body["message"].(string), "Data Referensi") {
		t.Fatalf("override not enforced: %d %v", code, body)
	}
	_, body = govCall(t, h, http.MethodGet, "/security/permissions?role=admin&school_id="+admin.schoolID, super, nil)
	found := false
	for _, row := range dataList(body) {
		m := row.(map[string]any)
		if m["module"] == "references" {
			found = m["overridden"] == true && m["scope"] == "unit" && m["allowed"].(map[string]any)["edit"] == true
		}
	}
	if !found {
		t.Fatalf("matrix row %v", body)
	}
	// Internal read for other services.
	if code, body := govCall(t, h, http.MethodGet, "/internal/permissions?role=admin&school_id="+admin.schoolID, "internal", nil); code != http.StatusOK || len(dataList(body)) == 0 {
		t.Fatalf("internal %d %v", code, body)
	}
	reset := map[string]any{"school_id": admin.schoolID, "items": []map[string]any{{"role": "admin", "module": "references", "reset": true}}}
	govCall(t, h, http.MethodPut, "/security/permissions", super, reset)
	if code, _ := govCall(t, h, http.MethodPost, "/references", asAdmin, map[string]any{"category": "program", "code": "ips", "label": "IPS"}); code != http.StatusCreated {
		t.Fatalf("after reset: %d", code)
	}
}

// fakeAcademic answers ClassroomName and UserClassrooms.
func fakeAcademic(t *testing.T, classes map[string]string, members map[string][]string) *client.AcademicClient {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/internal/user-classrooms" {
			out := []string{}
			for _, u := range strings.Split(r.URL.Query().Get("user_ids"), ",") {
				out = append(out, members[u]...)
			}
			common.WriteJSON(w, http.StatusOK, out, nil)
			return
		}
		id := strings.TrimPrefix(r.URL.Path, "/classrooms/")
		if name, ok := classes[id]; ok {
			common.WriteJSON(w, http.StatusOK, map[string]string{"id": id, "name": name}, nil)
			return
		}
		common.WriteError(w, http.StatusNotFound, "classroom not found")
	}))
	t.Cleanup(srv.Close)
	return &client.AcademicClient{BaseURL: srv.URL, InternalKey: "internal-key", HTTPClient: srv.Client()}
}

func TestAnnouncementTargetingAndSchedule(t *testing.T) {
	db := openTestDB(t)
	app := newTestApp(db)
	admin := seedUser(t, db, common.RoleAdmin)
	t.Cleanup(func() { mustExec(t, db, `DELETE FROM announcements WHERE school_id=$1`, admin.schoolID) })
	classA, classB := newIdentityTestUUID(), newIdentityTestUUID()
	inA, inB := newIdentityTestUUID(), newIdentityTestUUID()
	parent := seedAccount(t, app, admin.schoolID, common.RoleOrangTua)
	mustExec(t, db, `INSERT INTO users (id, name, email, password_hash) VALUES ($1,'Siswa A',$2,'x')`, inA, "sa-"+inA[:8]+"@test.local")
	mustExec(t, db, `INSERT INTO guardian_links (school_id, guardian_user_id, student_user_id, relationship, status) VALUES ($1,$2,$3,'ayah','active')`, admin.schoolID, parent, inA)
	app.Academic = fakeAcademic(t, map[string]string{classA: "X-A", classB: "X-B"}, map[string][]string{inA: {classA}, inB: {classB}})
	h := governanceRouter(app)
	asAdmin := claimsFor(admin.userID, common.RoleAdmin, admin.schoolID)

	base := map[string]any{"title": "Ujian", "message": "Ujian Senin", "type": "pengumuman"}
	with := func(kv ...any) map[string]any {
		m := map[string]any{}
		for k, v := range base {
			m[k] = v
		}
		for i := 0; i < len(kv); i += 2 {
			m[kv[i].(string)] = kv[i+1]
		}
		return m
	}
	// BUG-153: audience required.
	if code, body := govCall(t, h, http.MethodPost, "/announcements", asAdmin, base); code != http.StatusUnprocessableEntity || !strings.Contains(body["message"].(string), "isian") {
		t.Fatalf("no audience: %d %v", code, body)
	}
	// Send-now needs confirmation.
	if code, body := govCall(t, h, http.MethodPost, "/announcements", asAdmin, with("target_roles", []string{"siswa"})); code != http.StatusConflict || dataMap(body)["requires_confirmation"] != true {
		t.Fatalf("send now unconfirmed: %d %v", code, body)
	}
	// Unknown class: 422.
	if code, _ := govCall(t, h, http.MethodPost, "/announcements", asAdmin, with("target_roles", []string{"siswa"}, "target_class_ids", []string{newIdentityTestUUID()}, "confirm_send_now", true)); code != http.StatusUnprocessableEntity {
		t.Fatalf("unknown class: %d", code)
	}
	// Class-targeted, sent now.
	code, body := govCall(t, h, http.MethodPost, "/announcements", asAdmin, with("target_roles", []string{"siswa", "orang_tua"}, "target_class_ids", []string{classA}, "confirm_send_now", true))
	if code != http.StatusCreated || dataMap(body)["status"] != "terbit" {
		t.Fatalf("class announcement %d %v", code, body)
	}
	classAnn := dataMap(body)["id"].(string)
	if names := dataMap(body)["target_class_names"].([]any); len(names) != 1 || names[0] != "X-A" {
		t.Fatalf("class names %v", names)
	}
	// Scheduled one (BUG-154): no confirmation needed, hidden until due.
	future := time.Now().Add(2 * time.Hour).Format(time.RFC3339)
	code, body = govCall(t, h, http.MethodPost, "/announcements", asAdmin, with("audience_all", true, "publish_at", future))
	if code != http.StatusCreated || dataMap(body)["status"] != "terjadwal" {
		t.Fatalf("scheduled %d %v", code, body)
	}
	scheduled := dataMap(body)["id"].(string)

	visible := func(claims string) map[string]bool {
		_, body := govCall(t, h, http.MethodGet, "/announcements", claims, nil)
		out := map[string]bool{}
		for _, a := range dataList(body) {
			out[a.(map[string]any)["id"].(string)] = true
		}
		return out
	}
	if v := visible(claimsFor(inA, common.RoleSiswa, admin.schoolID)); !v[classAnn] || v[scheduled] {
		t.Fatalf("student in class A sees %v", v)
	}
	if v := visible(claimsFor(inB, common.RoleSiswa, admin.schoolID)); v[classAnn] {
		t.Fatal("student in class B must not see class A announcement")
	}
	if v := visible(claimsFor(parent, common.RoleOrangTua, admin.schoolID)); !v[classAnn] {
		t.Fatal("parent of a class A student must see it")
	}
	if v := visible(claimsFor(newIdentityTestUUID(), common.RoleGuru, admin.schoolID)); v[classAnn] {
		t.Fatal("guru not targeted by role")
	}
	if code, _ := govCall(t, h, http.MethodGet, "/announcements/"+classAnn, claimsFor(inB, common.RoleSiswa, admin.schoolID), nil); code != http.StatusNotFound {
		t.Fatalf("direct read by non-audience: %d", code)
	}
	// Schedule comes due.
	mustExec(t, db, `UPDATE announcements SET publish_at = now() - interval '1 minute' WHERE id=$1`, scheduled)
	if v := visible(claimsFor(inB, common.RoleSiswa, admin.schoolID)); !v[scheduled] {
		t.Fatal("due scheduled announcement not visible")
	}
	// Admin list shows status; unpublish then publish needs confirmation.
	if code, _ := govCall(t, h, http.MethodPost, "/announcements/"+classAnn+"/unpublish", asAdmin, nil); code != http.StatusOK {
		t.Fatalf("unpublish %d", code)
	}
	if code, _ := govCall(t, h, http.MethodPost, "/announcements/"+classAnn+"/publish", asAdmin, nil); code != http.StatusConflict {
		t.Fatalf("publish unconfirmed %d", code)
	}
	if code, _ := govCall(t, h, http.MethodPost, "/announcements/"+classAnn+"/publish", asAdmin, map[string]bool{"confirm_send_now": true}); code != http.StatusOK {
		t.Fatalf("publish confirmed %d", code)
	}
	_, body = govCall(t, h, http.MethodGet, "/announcements/all", asAdmin, nil)
	if len(dataList(body)) != 2 {
		t.Fatalf("admin list %v", body)
	}
	if code, _ := govCall(t, h, http.MethodGet, "/announcements/all", claimsFor(inA, common.RoleSiswa, admin.schoolID), nil); code != http.StatusForbidden {
		t.Fatalf("siswa management list: %d", code)
	}
}
