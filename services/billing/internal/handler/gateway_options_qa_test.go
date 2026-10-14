package handler

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"

	"skci/billing/internal/common"
)

// TestProviderOptionsShowReadiness (QA-PAY round 2): the options list used
// by the admin gateway settings page says whether a provider's credentials
// are installed, so the UI can show "belum dikonfigurasi" instead of a
// provider that silently cannot charge; credentials themselves never leak.
func TestProviderOptionsShowReadiness(t *testing.T) {
	f := newExtFixture(t)
	secret := "SECRET-" + genUUID()[:8]
	prefix := "QAOPT" + strings.ToUpper(uniqueCode("x"))[1:7]
	t.Setenv(prefix+"_CLIENT_SECRET", secret)
	code := uniqueCode("np")
	if st, _, raw := f.do(http.MethodPost, "/platform/payment-providers", common.RoleSuperadmin, genUUID(),
		map[string]any{"code": code, "adapter": "nicepay", "label": "NICEPay Cadangan", "enabled": true, "credential_ref": prefix, "capabilities": []string{"va"}}); st != http.StatusCreated {
		t.Fatalf("create provider %d %s", st, raw)
	}

	r := chi.NewRouter()
	r.Use(common.JWTMiddleware(testSecret))
	r.Get("/payment-providers/options", f.app.ListPaymentProviderOptions)
	srv := httptest.NewServer(r)
	defer srv.Close()

	req, _ := http.NewRequest(http.MethodGet, srv.URL+"/payment-providers/options", nil)
	req.Header.Set("Authorization", "Bearer "+signToken(t, genUUID(), f.schoolID, common.RoleFinanceManager))
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	body := string(raw)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("options %d %s", resp.StatusCode, body)
	}
	if !strings.Contains(body, `"code":"`+code+`"`) || !strings.Contains(body, `"credentials_installed":false`) {
		t.Fatalf("options must list %s with credentials_installed=false: %s", code, body)
	}
	if strings.Contains(body, secret) || strings.Contains(body, prefix) {
		t.Fatalf("options must not expose credentials or env prefix: %s", body)
	}
}
