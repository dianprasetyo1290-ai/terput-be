package handler

import (
	"net/http"
	"testing"

	"github.com/go-chi/chi/v5"

	"skci/identity/internal/common"
)

func oversightRouter(app *App) http.Handler {
	r := chi.NewRouter()
	r.Group(func(pr chi.Router) {
		pr.Use(claimsMiddleware)
		pr.Get("/students", app.ListStudents)
		pr.Get("/users", app.ListUsers)
		pr.Get("/users/{id}", app.GetUser)
	})
	r.Group(func(in chi.Router) {
		in.Use(common.InternalOnlyMiddleware("internal-key"))
		in.Get("/internal/users", app.InternalUsersByRole)
	})
	return r
}

// QA-BILL hand-off: finance/oversight roles read students of their own unit
// (identifiers only); other roles still get 403; other units' students 404.
func TestOversightRolesReadStudents(t *testing.T) {
	db := openTestDB(t)
	app := newTestApp(db)
	h := oversightRouter(app)
	admin := seedUser(t, db, common.RoleAdmin)
	other := seedUser(t, db, common.RoleAdmin)
	student := seedAccount(t, app, admin.schoolID, common.RoleSiswa)
	mustExec(t, db, `UPDATE users SET nisn='0012345678', nik='3201010101010001', phone='0811' WHERE id=$1`, student)
	foreign := seedAccount(t, app, other.schoolID, common.RoleSiswa)

	cases := []struct {
		role       string
		listCode   int
		detailCode int
	}{
		{common.RoleFinanceManager, http.StatusOK, http.StatusOK},
		{common.RoleKepalaYayasan, http.StatusOK, http.StatusOK},
		{common.RoleKepalaSekolah, http.StatusOK, http.StatusOK},
		{common.RoleAuditor, http.StatusOK, http.StatusOK},
		{common.RolePegawai, http.StatusForbidden, http.StatusForbidden},
		{common.RoleSiswa, http.StatusForbidden, http.StatusForbidden},
	}
	for _, c := range cases {
		t.Run(c.role, func(t *testing.T) {
			caller := seedAccount(t, app, admin.schoolID, c.role)
			cl := claimsFor(caller, c.role, admin.schoolID)
			code, body := govCall(t, h, http.MethodGet, "/students?per_page=100", cl, nil)
			if code != c.listCode {
				t.Fatalf("list %d %v", code, body)
			}
			if code == http.StatusOK {
				found := false
				for _, it := range dataList(body) {
					m := it.(map[string]any)
					if m["id"] == foreign {
						t.Fatal("student of another unit listed")
					}
					if m["id"] == student {
						found = true
						if m["nisn"] != "0012345678" || m["nik"] != nil || m["phone"] != nil {
							t.Fatalf("fields: nisn=%v nik=%v phone=%v", m["nisn"], m["nik"], m["phone"])
						}
					}
				}
				if !found {
					t.Fatal("own student missing")
				}
				if code, _ := govCall(t, h, http.MethodGet, "/users?role=siswa", cl, nil); code != http.StatusOK {
					t.Fatalf("users?role=siswa %d", code)
				}
			}
			code, body = govCall(t, h, http.MethodGet, "/users/"+student, cl, nil)
			if code != c.detailCode {
				t.Fatalf("detail %d %v", code, body)
			}
			if code == http.StatusOK {
				m := dataMap(body)
				if m["nik"] != nil || m["phone"] != nil || m["default_password"] != nil {
					t.Fatalf("detail leaks private fields: %v", m)
				}
				if code, _ := govCall(t, h, http.MethodGet, "/users/"+foreign, cl, nil); code != http.StatusNotFound {
					t.Fatalf("foreign detail %d", code)
				}
				if code, _ := govCall(t, h, http.MethodGet, "/users/"+admin.userID, cl, nil); code != http.StatusNotFound {
					t.Fatalf("non-student detail %d", code)
				}
			}
		})
	}
}

func TestInternalUsersByRole(t *testing.T) {
	db := openTestDB(t)
	app := newTestApp(db)
	h := oversightRouter(app)
	admin := seedUser(t, db, common.RoleAdmin)
	acc := seedAccount(t, app, admin.schoolID, common.RoleAkuntan)
	inactive := seedAccount(t, app, admin.schoolID, common.RoleAkuntan)
	mustExec(t, db, `UPDATE user_school_roles SET status='inactive' WHERE user_id=$1`, inactive)

	cases := []struct {
		name, query, claims string
		code                int
		want                int
	}{
		{"no key", "?role=akuntan&school_id=" + admin.schoolID, "", http.StatusUnauthorized, -1},
		{"bad role", "?role=nope&school_id=" + admin.schoolID, "internal", http.StatusBadRequest, -1},
		{"no school", "?role=akuntan", "internal", http.StatusBadRequest, -1},
		{"akuntan", "?role=akuntan&school_id=" + admin.schoolID, "internal", http.StatusOK, 1},
		{"finance none", "?role=finance_manager&school_id=" + admin.schoolID, "internal", http.StatusOK, 0},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			code, body := govCall(t, h, http.MethodGet, "/internal/users"+c.query, c.claims, nil)
			if code != c.code && !(c.code == http.StatusUnauthorized && code == http.StatusForbidden) {
				t.Fatalf("code %d %v", code, body)
			}
			if c.want >= 0 {
				l := dataList(body)
				if len(l) != c.want {
					t.Fatalf("got %d users: %v", len(l), l)
				}
				if c.want == 1 && l[0].(map[string]any)["id"] != acc {
					t.Fatal("wrong user")
				}
			}
		})
	}
}
