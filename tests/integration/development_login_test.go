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

	"github.com/rdrgzma/robo-juridico-go-saas/internal/auth"
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
