package handler

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"

	"skci/billing/internal/client"
	"skci/billing/internal/common"
	"skci/billing/internal/config"
)

// TestWalletAdminRoles (QA-PAY round 4): PRD menu 125 lists Finance Admin,
// which is finance_manager in this system (identity issues no finance_admin
// role), so finance_manager may read the wallet list and a student's
// wallets, while closing a wallet stays admin-only.
func TestWalletAdminRoles(t *testing.T) {
	db := getTestDB(t)
	defer db.Close()
	identity := chi.NewRouter()
	identity.Get("/schools/{id}/features", func(w http.ResponseWriter, r *http.Request) {
		common.WriteJSON(w, http.StatusOK, []map[string]any{{"feature_key": featureWallet, "enabled": true}}, nil)
	})
	idSrv := httptest.NewServer(identity)
	defer idSrv.Close()
	app := New(db, config.Config{}, stubIdentity{}, nil)
	app.IdentitySvc = client.New(idSrv.URL, "k")
	r := chi.NewRouter()
	r.Group(func(pr chi.Router) {
		pr.Use(common.JWTMiddleware(testSecret))
		pr.Get("/admin/wallets", app.AdminListWallets)
		pr.Get("/admin/students/{studentUserId}/wallets", app.AdminStudentWallets)
		pr.Post("/admin/wallets/{id}/close", app.AdminCloseWallet)
	})
	schoolID, studentID, walletID := genUUID(), genUUID(), genUUID()
	mustExec(t, db, `INSERT INTO wallets (id, school_id, student_user_id, kind, balance_idr) VALUES ($1,$2,$3,'allowance',5000)`,
		walletID, schoolID, studentID)
	as := func(role string) apiCaller {
		return apiCaller{t: t, r: r, token: signToken(t, genUUID(), schoolID, role)}
	}
	for _, role := range []string{common.RoleAdmin, common.RoleCanteenAdmin, common.RoleFinanceManager} {
		if code, env := as(role).do(http.MethodGet, "/admin/wallets", nil); code != http.StatusOK {
			t.Errorf("%s list: %d %v", role, code, env)
		}
		if code, env := as(role).do(http.MethodGet, "/admin/students/"+studentID+"/wallets", nil); code != http.StatusOK {
			t.Errorf("%s student wallets: %d %v", role, code, env)
		}
	}
	for _, role := range []string{common.RoleKasir, common.RoleCanteenOperator, common.RoleParent} {
		if code, _ := as(role).do(http.MethodGet, "/admin/wallets", nil); code != http.StatusForbidden {
			t.Errorf("%s list: want 403, got %d", role, code)
		}
	}
	if code, _ := as(common.RoleFinanceManager).do(http.MethodPost, "/admin/wallets/"+walletID+"/close",
		map[string]any{"disposition": "refund", "reason": "x"}); code != http.StatusForbidden {
		t.Errorf("finance_manager close: want 403, got %d", code)
	}
}
