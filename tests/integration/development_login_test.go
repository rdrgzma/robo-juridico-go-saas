package integration

import (
	"bytes"
	"io"
	"log/slog"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/rdrgzma/robo-juridico-go-saas/internal/auth"
	"github.com/rdrgzma/robo-juridico-go-saas/internal/juridico"
	"github.com/rdrgzma/robo-juridico-go-saas/internal/transport"
)

func TestDevelopmentShortcutsUseNormalAuthentication(t *testing.T) {
	f := identityDatabase(t)
	admin, _ := f.user(t, "shortcut-admin", true)
	accounts := map[string]auth.DevelopmentLogin{"platform_admin": {Organization: admin.Organization, Email: admin.Email, Password: fixturePassword}}
	for _, role := range []string{"owner", "admin", "lawyer", "viewer"} {
		p, err := f.service.CreateDevelopmentMember(f.ctx, admin.Organization, role+"@example.test", fixturePassword, role)
		if err != nil {
			t.Fatal(err)
		}
		accounts[role] = auth.DevelopmentLogin{Organization: p.Organization, Email: p.Email, Password: fixturePassword}
	}
	for _, enabled := range []bool{false, true} {
		srv := httptest.NewUnstartedServer(nil)
		origin := "https://" + srv.Listener.Addr().String()
		options := auth.Options{Origin: origin, CookieSecure: true}
		if enabled {
			options.DevelopmentLogins = accounts
		}
		service, err := auth.New(f.pool, bytes.Repeat([]byte{9}, 32), options)
		if err != nil {
			t.Fatal(err)
		}
		srv.Config.Handler = transport.NewWithIdentity([32]byte{}, slog.New(slog.NewTextHandler(io.Discard, nil)), service)
		srv.StartTLS()
		t.Cleanup(srv.Close)
		client := srv.Client()
		client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
		for profile := range accounts {
			client.Jar, _ = cookiejar.New(nil)
			response, err := client.Get(origin + "/login")
			if err != nil {
				t.Fatal(err)
			}
			body := httpBody(t, response)
			if strings.Contains(body, fixturePassword) {
				t.Fatal("password leaked into HTML")
			}
			if strings.Contains(body, "Atalhos de desenvolvimento") != enabled {
				t.Fatal("wrong shortcut visibility")
			}
			csrf := csrfFrom(t, body)
			fetchSite := "same-origin"
			post := func(token string) *http.Response {
				req, _ := http.NewRequest("POST", origin+"/login/development", strings.NewReader(url.Values{"csrf": {token}, "profile": {profile}}.Encode()))
				req.Header.Set("Origin", "null")
				req.Header.Set("Sec-Fetch-Site", fetchSite)
				req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
				res, err := client.Do(req)
				if err != nil {
					t.Fatal(err)
				}
				return res
			}
			if !enabled {
				res := post(csrf)
				if res.StatusCode != 404 {
					t.Fatal("disabled route accessible", res.StatusCode)
				}
				httpBody(t, res)
				break
			}
			fetchSite = "cross-site"
			res := post(csrf)
			if res.StatusCode != 403 {
				t.Fatal("cross-site request accepted", res.StatusCode)
			}
			httpBody(t, res)
			fetchSite = "same-origin"
			res = post("forged")
			if res.StatusCode != 403 {
				t.Fatal("CSRF bypass", res.StatusCode)
			}
			httpBody(t, res)
			res = post(csrf)
			if res.StatusCode != 303 {
				t.Fatal("shortcut login failed", res.StatusCode)
			}
			httpBody(t, res)
			res, err = client.Get(origin + "/account")
			if err != nil {
				t.Fatal(err)
			}
			body = httpBody(t, res)
			if profile == "platform_admin" {
				if res.StatusCode != 303 || res.Header.Get("Location") != "/account/mfa" {
					t.Fatal("MFA bypass")
				}
			} else if res.StatusCode != 200 || !strings.Contains(body, "Papel: "+profile) {
				t.Fatal("incorrect role", profile, res.StatusCode)
			}
		}
	}
}

func TestDevelopmentMFADisablementPreservesSecretsPermissionsAndSessionTruth(t *testing.T) {
	f := identityDatabase(t)
	if _, err := auth.New(f.pool, bytes.Repeat([]byte{9}, 32), auth.Options{Origin: "https://robojuridico.example", CookieSecure: true, DisableMFA: true}); err == nil {
		t.Fatal("non-loopback origin accepted DisableMFA")
	}
	admin, _ := f.user(t, "dev-mfa-admin", true)
	enrolled, enrolledInitialToken := f.user(t, "dev-mfa-enrolled", false)
	secret, err := f.service.MFASetup(f.ctx, enrolled, enrolledInitialToken)
	if err != nil {
		t.Fatal(err)
	}
	code, _ := auth.TOTP(secret, time.Unix(0, f.clock.Load()), 6)
	if _, _, err := f.service.VerifyMFA(f.ctx, enrolled, enrolledInitialToken, code); err != nil {
		t.Fatal(err)
	}
	viewer, err := f.service.CreateDevelopmentMember(f.ctx, admin.Organization, "viewer-mfa@example.test", fixturePassword, "viewer")
	if err != nil {
		t.Fatal(err)
	}
	accounts := map[string]auth.DevelopmentLogin{
		"platform_admin": {Organization: admin.Organization, Email: admin.Email, Password: fixturePassword},
		"owner":          {Organization: enrolled.Organization, Email: enrolled.Email, Password: fixturePassword},
		"viewer":         {Organization: admin.Organization, Email: viewer.Email, Password: fixturePassword},
	}

	srv := httptest.NewUnstartedServer(nil)
	origin := "https://" + srv.Listener.Addr().String()
	disabledService, err := auth.New(f.pool, bytes.Repeat([]byte{9}, 32), auth.Options{
		Origin:            origin,
		CookieSecure:      true,
		DisableMFA:        true,
		DevelopmentLogins: accounts,
		Clock:             func() time.Time { return time.Unix(0, f.clock.Load()) },
	})
	if err != nil {
		t.Fatal(err)
	}
	workspace := juridico.New(f.pool, disabledService, bytes.Repeat([]byte{4}, 32), nil)
	srv.Config.Handler = transport.NewWithWorkspace([32]byte{}, slog.New(slog.NewTextHandler(io.Discard, nil)), disabledService, workspace)
	srv.StartTLS()
	t.Cleanup(srv.Close)

	client := srv.Client()
	client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }

	loginAs := func(org, email string) {
		client.Jar, _ = cookiejar.New(nil)
		res, err := client.Get(origin + "/login")
		if err != nil {
			t.Fatal(err)
		}
		body := httpBody(t, res)
		if !strings.Contains(body, "A exigência de MFA está desativada neste ambiente local.") || strings.Contains(body, "O administrador global continua exigindo MFA.") {
			t.Fatal("unexpected login MFA notice", body)
		}
		csrf := csrfFrom(t, body)
		req, _ := http.NewRequest("POST", origin+"/login", strings.NewReader(url.Values{"csrf": {csrf}, "organization": {org}, "email": {email}, "password": {fixturePassword}}.Encode()))
		req.Header.Set("Origin", origin)
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		res, err = client.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		if res.StatusCode != 303 || res.Header.Get("Location") != "/account" {
			t.Fatal("login failed", res.StatusCode, httpBody(t, res))
		}
		httpBody(t, res)
	}

	// 1. Global administrator logs in without MFA, session lasts > 5 minutes, MFAVerified remains false, accesses /admin, /app, /portal, /account/members, /account/keys.
	loginAs(admin.Organization, admin.Email)
	f.clock.Add(int64(10 * time.Minute))
	for _, path := range []string{"/account", "/admin", "/portal", "/app", "/account/members", "/account/keys", "/account/mfa"} {
		res, err := client.Get(origin + path)
		if err != nil {
			t.Fatal(err)
		}
		body := httpBody(t, res)
		if res.StatusCode != 200 {
			t.Fatalf("expected 200 on %s with MFA disabled, got %d (%s)", path, res.StatusCode, res.Header.Get("Location"))
		}
		if path == "/account" && (!strings.Contains(body, "MFA verificado nesta sessão: não.") || !strings.Contains(body, "Exigência de MFA dispensada no ambiente local.")) {
			t.Fatal("session falsely marked MFA verified or missing local bypass notice", body)
		}
		if path == "/account/mfa" && !strings.Contains(body, "A exigência de segundo fator está desativada neste ambiente local.") {
			t.Fatal("missing MFA page notice", body)
		}
	}

	// 2. Enrolled MFA user logs in without MFA prompt, existing secret is preserved, can issue keys and run workspace calculations after > 5 minutes.
	loginAs(enrolled.Organization, enrolled.Email)
	f.clock.Add(int64(10 * time.Minute))
	res, err := client.Get(origin + "/account")
	if err != nil {
		t.Fatal(err)
	}
	accountBody := httpBody(t, res)
	if res.StatusCode != 200 || !strings.Contains(accountBody, "MFA configurado: sim. MFA verificado nesta sessão: não.") {
		t.Fatal("enrolled user MFA state corrupted", res.StatusCode, accountBody)
	}
	res, err = client.Get(origin + "/admin")
	if err != nil {
		t.Fatal(err)
	}
	httpBody(t, res)
	if res.StatusCode != 403 {
		t.Fatal("non-global admin accessed /admin when MFA was disabled", res.StatusCode)
	}
	res, err = client.Get(origin + "/account/keys")
	if err != nil {
		t.Fatal(err)
	}
	keysCSRF := csrfFrom(t, httpBody(t, res))
	req, _ := http.NewRequest("POST", origin+"/account/keys", strings.NewReader(url.Values{"csrf": {keysCSRF}, "name": {"dev-key"}, "scope": {"account:read"}}.Encode()))
	req.Header.Set("Origin", origin)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	res, err = client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	httpBody(t, res)
	if res.StatusCode != 201 {
		t.Fatal("enrolled user could not issue key with MFA disabled", res.StatusCode)
	}
	res, err = client.Get(origin + "/app/liquidacao")
	if err != nil {
		t.Fatal(err)
	}
	calcCSRF := csrfFrom(t, httpBody(t, res))
	calcValues := url.Values{"csrf": {calcCSRF}, "request_id": {auth.RandomToken()}, "title": {"Dispensa MFA"}, "principal": {"1000.00"}, "fator_correcao": {"1"}, "taxa_mensal": {"1"}, "meses": {"2"}, "pagamento": {"0"}, "multa_percentual": {"0"}, "honorarios_percentual": {"0"}}
	req, _ = http.NewRequest("POST", origin+"/app/liquidacao", strings.NewReader(calcValues.Encode()))
	req.Header.Set("Origin", origin)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	res, err = client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	httpBody(t, res)
	if res.StatusCode != 303 {
		t.Fatal("enrolled user could not run calculation with MFA disabled", res.StatusCode)
	}

	// 3. Viewer still blocked from /app and /admin.
	loginAs(admin.Organization, viewer.Email)
	for _, restricted := range []string{"/app", "/admin", "/account/keys", "/account/members"} {
		res, err := client.Get(origin + restricted)
		if err != nil {
			t.Fatal(err)
		}
		httpBody(t, res)
		if res.StatusCode != 403 {
			t.Fatalf("viewer accessed %s (status %d)", restricted, res.StatusCode)
		}
	}

	// 4. Re-enabling MFA immediately requires MFA again on existing unverified sessions and preserves enrolled secret.
	var storedSecret []byte
	if err := f.pool.QueryRow(f.ctx, "SELECT mfa_secret FROM users WHERE id=$1", enrolled.UserID).Scan(&storedSecret); err != nil || len(storedSecret) == 0 {
		t.Fatal("existing MFA secret was lost", err)
	}
	token, err := disabledService.Login(f.ctx, enrolled.Organization, enrolled.Email, fixturePassword, "re-enable-ip")
	if err != nil {
		t.Fatal(err)
	}
	strictPrincipal, err := f.service.Session(f.ctx, token)
	if err != nil {
		t.Fatal(err)
	}
	if !strictPrincipal.MFAEnabled || strictPrincipal.MFAVerified || !strictPrincipal.RequiresMFA() {
		t.Fatal("re-enabled MFA service did not require MFA for unverified session", strictPrincipal)
	}
}
