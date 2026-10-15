package main

import (
	"go/ast"
	"go/parser"
	"go/token"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"

	"skci/billing/internal/config"
	"skci/billing/internal/handler"
)

// TestRouterBuilds fails when chi rejects a pattern (it panics on
// conflicting wildcards) and checks the T12 wallet/canteen routes exist.
func TestRouterBuilds(t *testing.T) {
	r := newRouter(config.Config{JWTSecret: "x", InternalKey: "k"}, handler.New(nil, config.Config{}, nil, nil))
	want := [][2]string{
		{http.MethodGet, "/admin/wallets"},
		{http.MethodPost, "/admin/wallets/w1/close"},
		{http.MethodGet, "/admin/students/s1/wallets"},
		{http.MethodGet, "/admin/students/s1/ledger"},
		{http.MethodPost, "/admin/student-cards"},
		{http.MethodPost, "/admin/savings-requests/r1/complete"},
		{http.MethodPost, "/cashier/wallet-topups"},
		{http.MethodGet, "/admin/wallet-reconciliations"},
		{http.MethodGet, "/parent/children/s1/wallets"},
		{http.MethodPut, "/parent/children/s1/wallets/allowance/limit"},
		{http.MethodPost, "/parent/children/s1/wallets/allowance/topups"},
		{http.MethodPost, "/parent/savings-requests/r1/decision"},
		{http.MethodGet, "/me/wallets/savings/movements"},
		{http.MethodGet, "/me/student-card"},
		{http.MethodGet, "/canteen/tenants/t1/menu-items"},
		{http.MethodDelete, "/canteen/tenants/t1/operators/u1"},
		{http.MethodPost, "/pos/sales/sync"},
		{http.MethodGet, "/pos/sales/x/receipt"},
		{http.MethodGet, "/canteen/reports/sales"},
		{http.MethodPost, "/canteen/settlements/x/pay"},
		{http.MethodPost, "/internal/wallets/transfer-callback"},
		{http.MethodPost, "/internal/canteen/settlements/x/callback"},
		{http.MethodPost, "/internal/student-cards/resolve"},
		{http.MethodPost, "/internal/bills/adhoc"},
		{http.MethodPost, "/internal/bills/b1/void"},
		{http.MethodGet, "/internal/bills/by-source"},
	}
	rctx := chi.NewRouteContext()
	for _, w := range want {
		rctx.Reset()
		if !r.Match(rctx, w[0], w[1]) {
			t.Errorf("route not mounted: %s %s", w[0], w[1])
		}
	}
}

// TestEveryHandlerMounted: an exported App handler that main never mounts is
// dead code the UI cannot reach (QA-PAY round 4: T12 was never mounted).
func TestEveryHandlerMounted(t *testing.T) {
	src, err := os.ReadFile("routes.go")
	if err != nil {
		t.Fatal(err)
	}
	// Handlers used only by tests or other wiring, not as HTTP routes.
	notRoutes := map[string]bool{}
	fset := token.NewFileSet()
	files, _ := filepath.Glob("../../internal/handler/*.go")
	for _, f := range files {
		if strings.HasSuffix(f, "_test.go") {
			continue
		}
		file, err := parser.ParseFile(fset, f, nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		for _, d := range file.Decls {
			fn, ok := d.(*ast.FuncDecl)
			if !ok || fn.Recv == nil || !fn.Name.IsExported() || fn.Type.Params.NumFields() != 2 || fn.Type.Results != nil {
				continue
			}
			if p := fn.Type.Params.List; len(p) != 2 || exprString(p[0].Type) != "http.ResponseWriter" || exprString(p[1].Type) != "*http.Request" {
				continue
			}
			if notRoutes[fn.Name.Name] {
				continue
			}
			if !strings.Contains(string(src), "app."+fn.Name.Name+")") {
				t.Errorf("handler %s (%s) is not mounted in routes.go", fn.Name.Name, filepath.Base(f))
			}
		}
	}
}

func exprString(e ast.Expr) string {
	switch x := e.(type) {
	case *ast.SelectorExpr:
		return exprString(x.X) + "." + x.Sel.Name
	case *ast.StarExpr:
		return "*" + exprString(x.X)
	case *ast.Ident:
		return x.Name
	}
	return ""
}
