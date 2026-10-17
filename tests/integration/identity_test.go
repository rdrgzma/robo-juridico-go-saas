package integration

import (
	"bytes"
	"context"
	"errors"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rdrgzma/robo-juridico-go-saas/internal/auth"
	"github.com/rdrgzma/robo-juridico-go-saas/internal/storage"
	"github.com/rdrgzma/robo-juridico-go-saas/internal/tenancy"
	"github.com/rdrgzma/robo-juridico-go-saas/internal/transport"
	"github.com/rdrgzma/robo-juridico-go-saas/migrations"
	"io"
	"log/slog"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"os"
	"regexp"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

const fixturePassword = "senha-sintetica-para-testes-123"

type identityFixture struct {
	ctx     context.Context
	pool    *pgxpool.Pool
	service *auth.Service
	clock   atomic.Int64
}

func identityDatabase(t *testing.T) *identityFixture {
	t.Helper()
	ctx, conn, schema := testDatabase(t)
	if _, err := storage.Migrate(ctx, conn, migrations.Files, true); err != nil {
		t.Fatal(err)
	}
	cfg, err := pgxpool.ParseConfig(os.Getenv("TEST_DATABASE_URL"))
	if err != nil {
		t.Fatal(err)
	}
	cfg.ConnConfig.RuntimeParams["search_path"] = schema
	cfg.MaxConns = 8
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	f := &identityFixture{ctx: ctx, pool: pool}
	f.clock.Store(time.Now().UTC().UnixNano())
	f.service, err = auth.New(pool, bytes.Repeat([]byte{9}, 32), auth.Options{Origin: "http://localhost:8081", Clock: func() time.Time { return time.Unix(0, f.clock.Load()) }})
	if err != nil {
		t.Fatal(err)
	}
	return f
}
func (f *identityFixture) user(t *testing.T, slug string, admin bool) (tenancy.Principal, string) {
	t.Helper()
	_, err := f.service.CreateOrganization(f.ctx, slug, "Organização sintética "+slug, slug+"@example.test", fixturePassword, admin)
	if err != nil {
		t.Fatal(err)
	}
	token, err := f.service.Login(f.ctx, slug, slug+"@example.test", fixturePassword, "fixture-ip")
	if err != nil {
		t.Fatal(err)
	}
	p, err := f.service.Session(f.ctx, token)
	if err != nil {
		t.Fatal(err)
	}
	return p, token
}
func TestSessionIsolationRolesRevocationAndExpiry(t *testing.T) {
	f := identityDatabase(t)
	a, tokenA := f.user(t, "org-a", false)
	b, _ := f.user(t, "org-b", false)
	if _, err := f.service.Login(f.ctx, "org-b", a.Email, fixturePassword, "fixture-ip"); !errors.Is(err, auth.ErrCredentials) {
		t.Fatal("login entre organizações", err)
	}
	members, err := f.service.Members(f.ctx, a)
	if err != nil || len(members) != 1 || members[0].UserID != a.UserID {
		t.Fatal(members, err)
	}
	if err := f.service.SetRole(f.ctx, a, b.UserID, "viewer"); !errors.Is(err, auth.ErrNotFound) {
		t.Fatal("ID de outra organização aceito", err)
	}
	if err := f.service.SetRole(f.ctx, a, a.UserID, "viewer"); !errors.Is(err, auth.ErrConflict) {
		t.Fatal("último owner removido", err)
	}
	keyB, _, err := f.service.IssueKey(f.ctx, b, "B", []string{"account:read"})
	if err != nil {
		t.Fatal(err)
	}
	if err := f.service.RevokeKey(f.ctx, a, keyB.ID); !errors.Is(err, auth.ErrNotFound) {
		t.Fatal("chave de outra organização revogada", err)
	}
	keys, err := f.service.Keys(f.ctx, a)
	if err != nil || len(keys) != 0 {
		t.Fatal(keys, err)
	}
	member, err := f.service.CreateDevelopmentMember(f.ctx, a.Organization, "viewer@example.test", fixturePassword, "viewer")
	if err != nil {
		t.Fatal(err)
	}
	viewerToken, err := f.service.Login(f.ctx, a.Organization, member.Email, fixturePassword, "fixture-ip")
	if err != nil {
		t.Fatal(err)
	}
	viewer, err := f.service.Session(f.ctx, viewerToken)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.service.Members(f.ctx, viewer); !errors.Is(err, auth.ErrDenied) {
		t.Fatal("viewer leu membros", err)
	}
	if _, _, err := f.service.IssueKey(f.ctx, viewer, "proibida", []string{"account:read"}); !errors.Is(err, auth.ErrDenied) {
		t.Fatal("viewer criou chave", err)
	}
	if _, err := f.pool.Exec(f.ctx, "UPDATE memberships SET active=false WHERE organization_id=$1 AND user_id=$2", a.OrganizationID, viewer.UserID); err != nil {
		t.Fatal(err)
	}
	if _, err := f.service.Session(f.ctx, viewerToken); !errors.Is(err, auth.ErrDenied) {
		t.Fatal("membership suspensa manteve acesso", err)
	}
	if err := f.service.Logout(f.ctx, tokenA); err != nil {
		t.Fatal(err)
	}
	if _, err := f.service.Session(f.ctx, tokenA); !errors.Is(err, auth.ErrDenied) {
		t.Fatal("logout manteve sessão", err)
	}
	fresh, err := f.service.Login(f.ctx, a.Organization, a.Email, fixturePassword, "fixture-ip")
	if err != nil {
		t.Fatal(err)
	}
	f.clock.Add(int64(2*time.Hour + time.Second))
	if _, err := f.service.Session(f.ctx, fresh); !errors.Is(err, auth.ErrDenied) {
		t.Fatal("sessão ociosa válida", err)
	}
}
func TestAbsoluteSessionExpiryDespiteActivity(t *testing.T) {
	f := identityDatabase(t)
	_, token := f.user(t, "absolute", false)
	for i := 0; i < 7; i++ {
		f.clock.Add(int64(90 * time.Minute))
		if _, err := f.service.Session(f.ctx, token); err != nil {
			t.Fatal(i, err)
		}
	}
	f.clock.Add(int64(91 * time.Minute))
	if _, err := f.service.Session(f.ctx, token); !errors.Is(err, auth.ErrDenied) {
		t.Fatal("expiração absoluta não respeitada", err)
	}
}
func TestAdminMFAReplayRecoveryAndSessionRotation(t *testing.T) {
	f := identityDatabase(t)
	p, old := f.user(t, "mfa-admin", true)
	if !p.RequiresMFA() {
		t.Fatal("admin sem MFA autorizado")
	}
	secret, err := f.service.MFASetup(f.ctx, p, old)
	if err != nil {
		t.Fatal(err)
	}
	code, _ := auth.TOTP(secret, time.Unix(0, f.clock.Load()), 6)
	upgraded, codes, err := f.service.VerifyMFA(f.ctx, p, old, code)
	if err != nil || len(codes) != 10 || upgraded == old {
		t.Fatal(err, len(codes))
	}
	if _, err := f.service.Session(f.ctx, old); !errors.Is(err, auth.ErrDenied) {
		t.Fatal("sessão anterior ao MFA válida")
	}
	current, err := f.service.Session(f.ctx, upgraded)
	if err != nil || !current.MFAVerified || current.RequiresMFA() {
		t.Fatal(current, err)
	}
	var encrypted []byte
	if err = f.pool.QueryRow(f.ctx, "SELECT mfa_secret FROM users WHERE id=$1", p.UserID).Scan(&encrypted); err != nil || bytes.Contains(encrypted, []byte(secret)) {
		t.Fatal("segredo não cifrado", err)
	}
	pending, err := f.service.Login(f.ctx, p.Organization, p.Email, fixturePassword, "fixture-ip")
	if err != nil {
		t.Fatal(err)
	}
	principal, _ := f.service.Session(f.ctx, pending)
	if _, _, err := f.service.VerifyMFA(f.ctx, principal, pending, code); !errors.Is(err, auth.ErrDenied) {
		t.Fatal("TOTP repetido aceito", err)
	}
	recovered, _, err := f.service.VerifyMFA(f.ctx, principal, pending, codes[0])
	if err != nil {
		t.Fatal(err)
	}
	principal, _ = f.service.Session(f.ctx, recovered)
	if _, _, err := f.service.VerifyMFA(f.ctx, principal, recovered, codes[0]); !errors.Is(err, auth.ErrDenied) {
		t.Fatal("código de recuperação reutilizado", err)
	}
}
func TestPasswordRecoveryIsSingleUseAndRevokesAllOrganizations(t *testing.T) {
	f := identityDatabase(t)
	a, oldA := f.user(t, "reset-a", false)
	b, _ := f.user(t, "reset-b", false)
	if _, err := f.pool.Exec(f.ctx, "INSERT INTO memberships(organization_id,user_id,role_code) VALUES($1,$2,'lawyer')", b.OrganizationID, a.UserID); err != nil {
		t.Fatal(err)
	}
	oldB, err := f.service.Login(f.ctx, b.Organization, a.Email, fixturePassword, "fixture-ip")
	if err != nil {
		t.Fatal(err)
	}
	if err := f.service.ForgotPassword(f.ctx, a.Organization, "ausente@example.test", "reset-ip"); err != nil {
		t.Fatal(err)
	}
	if err := f.service.ForgotPassword(f.ctx, a.Organization, a.Email, "reset-ip"); err != nil {
		t.Fatal(err)
	}
	messages, err := f.service.DevelopmentOutbox(f.ctx)
	if err != nil || len(messages) != 1 {
		t.Fatal(messages, err)
	}
	u, _ := url.Parse(messages[0].URL)
	token := u.Query().Get("token")
	var hash, payload []byte
	if err := f.pool.QueryRow(f.ctx, "SELECT token_hash FROM password_resets").Scan(&hash); err != nil || bytes.Equal(hash, []byte(token)) {
		t.Fatal(err)
	}
	if err := f.pool.QueryRow(f.ctx, "SELECT encrypted_payload FROM mail_outbox").Scan(&payload); err != nil || bytes.Contains(payload, []byte(token)) {
		t.Fatal("outbox plaintext", err)
	}
	newPassword := "nova-senha-segura-de-fixture-6789"
	if err := f.service.ResetPassword(f.ctx, token, newPassword); err != nil {
		t.Fatal(err)
	}
	if err := f.service.ResetPassword(f.ctx, token, newPassword); !errors.Is(err, auth.ErrDenied) {
		t.Fatal("token reutilizado", err)
	}
	for _, session := range []string{oldA, oldB} {
		if _, err := f.service.Session(f.ctx, session); !errors.Is(err, auth.ErrDenied) {
			t.Fatal("sessão antiga não revogada", err)
		}
	}
	if _, err := f.service.Login(f.ctx, a.Organization, a.Email, fixturePassword, "fixture-ip"); !errors.Is(err, auth.ErrCredentials) {
		t.Fatal("senha antiga aceita", err)
	}
	if _, err := f.service.Login(f.ctx, a.Organization, a.Email, newPassword, "fixture-ip"); err != nil {
		t.Fatal(err)
	}
	if err := f.service.ForgotPassword(f.ctx, a.Organization, a.Email, "reset-ip"); err != nil {
		t.Fatal(err)
	}
	messages, _ = f.service.DevelopmentOutbox(f.ctx)
	u, _ = url.Parse(messages[0].URL)
	f.clock.Add(int64(16 * time.Minute))
	if err := f.service.ResetPassword(f.ctx, u.Query().Get("token"), newPassword); !errors.Is(err, auth.ErrDenied) {
		t.Fatal("token expirado aceito", err)
	}
}
func TestQuotaReservationConcurrencyRefundAndRecovery(t *testing.T) {
	f := identityDatabase(t)
	p, _ := f.user(t, "quota", false)
	_, raw, err := f.service.IssueKey(f.ctx, p, "integração", []string{"account:read", "calculations:run"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.pool.Exec(f.ctx, "UPDATE organization_limits SET monthly_limit=3 WHERE organization_id=$1", p.OrganizationID); err != nil {
		t.Fatal(err)
	}
	results := make(chan auth.APIAccess, 20)
	failures := make(chan error, 20)
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			access, err := f.service.BeginAPI(f.ctx, raw, "calculations:run", "api-fixture", true)
			if err == nil {
				results <- access
			} else if !errors.Is(err, auth.ErrQuota) {
				failures <- err
			}
		}()
	}
	wg.Wait()
	close(results)
	close(failures)
	for err := range failures {
		t.Fatal(err)
	}
	success := []auth.APIAccess{}
	for a := range results {
		success = append(success, a)
	}
	if len(success) != 3 {
		t.Fatal("reserva concorrente", len(success))
	}
	if err := f.service.FinishAPI(f.ctx, success[0].EventID, 200); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		if err := f.service.FinishAPI(f.ctx, success[1].EventID, 500); err != nil {
			t.Fatal(err)
		}
	}
	again, err := f.service.BeginAPI(f.ctx, raw, "calculations:run", "api-fixture", true)
	if err != nil {
		t.Fatal("estorno não liberou cota", err)
	}
	f.clock.Add(int64(11 * time.Minute))
	if err := f.service.Reconcile(f.ctx); err != nil {
		t.Fatal(err)
	}
	var used int
	if err := f.pool.QueryRow(f.ctx, "SELECT used FROM quota_periods WHERE organization_id=$1", p.OrganizationID).Scan(&used); err != nil || used != 1 {
		t.Fatal("saldo após reconciliação", used, err)
	}
	if err := f.service.FinishAPI(f.ctx, again.EventID, 500); err != nil {
		t.Fatal(err)
	} // idempotent after reconciliation.
}
func TestAPIScopesRateAndRevocation(t *testing.T) {
	f := identityDatabase(t)
	p, _ := f.user(t, "api-roles", false)
	key, raw, err := f.service.IssueKey(f.ctx, p, "somente conta", []string{"account:read"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.service.BeginAPI(f.ctx, raw, "calculations:run", "api-ip", true); !errors.Is(err, auth.ErrDenied) {
		t.Fatal("escopo ignorado", err)
	}
	if _, err := f.pool.Exec(f.ctx, "UPDATE organization_limits SET requests_per_minute=2 WHERE organization_id=$1", p.OrganizationID); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		a, err := f.service.BeginAPI(f.ctx, raw, "account:read", "api-ip", false)
		if err != nil {
			t.Fatal(err)
		}
		if err = f.service.FinishAPI(f.ctx, a.EventID, 200); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := f.service.BeginAPI(f.ctx, raw, "account:read", "api-ip", false); !errors.Is(err, auth.ErrRate) {
		t.Fatal("RPM ignorado", err)
	}
	f.clock.Add(int64(time.Minute))
	if err := f.service.RevokeKey(f.ctx, p, key.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := f.service.BeginAPI(f.ctx, raw, "account:read", "api-ip", false); !errors.Is(err, auth.ErrCredentials) {
		t.Fatal("chave revogada aceita", err)
	}
}

func TestLoginBruteForceLimitPersistsAcrossServiceInstances(t *testing.T) {
	f := identityDatabase(t)
	p, _ := f.user(t, "brute-force", false)
	for i := 0; i < 9; i++ {
		if _, err := f.service.Login(f.ctx, p.Organization, p.Email, "senha-errada", "same-ip"); !errors.Is(err, auth.ErrCredentials) {
			t.Fatal(i, err)
		}
	}
	other, err := auth.New(f.pool, bytes.Repeat([]byte{9}, 32), auth.Options{Origin: "http://localhost:8081", Clock: func() time.Time { return time.Unix(0, f.clock.Load()) }})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := other.Login(f.ctx, p.Organization, p.Email, fixturePassword, "another-ip"); !errors.Is(err, auth.ErrRate) {
		t.Fatal("limite não persistiu", err)
	}
}
func TestBillingPeriodUsesSaoPauloAndSuspensionIsImmediate(t *testing.T) {
	f := identityDatabase(t)
	p, session := f.user(t, "billing", false)
	_, raw, err := f.service.IssueKey(f.ctx, p, "billing", []string{"account:read", "calculations:run"})
	if err != nil {
		t.Fatal(err)
	}
	f.clock.Store(time.Date(2026, 11, 1, 2, 59, 0, 0, time.UTC).UnixNano()) // still October in Sao Paulo.
	a, err := f.service.BeginAPI(f.ctx, raw, "calculations:run", "billing-ip", true)
	if err != nil {
		t.Fatal(err)
	}
	if err = f.service.FinishAPI(f.ctx, a.EventID, 200); err != nil {
		t.Fatal(err)
	}
	f.clock.Add(int64(2 * time.Minute))
	a, err = f.service.BeginAPI(f.ctx, raw, "calculations:run", "billing-ip", true)
	if err != nil {
		t.Fatal(err)
	}
	if err = f.service.FinishAPI(f.ctx, a.EventID, 200); err != nil {
		t.Fatal(err)
	}
	var periods int
	if err = f.pool.QueryRow(f.ctx, "SELECT count(*) FROM quota_periods WHERE organization_id=$1 AND used=1", p.OrganizationID).Scan(&periods); err != nil || periods != 2 {
		t.Fatal(periods, err)
	}
	if _, err = f.pool.Exec(f.ctx, "UPDATE organizations SET active=false WHERE id=$1", p.OrganizationID); err != nil {
		t.Fatal(err)
	}
	if _, err = f.service.BeginAPI(f.ctx, raw, "account:read", "billing-ip", false); !errors.Is(err, auth.ErrCredentials) {
		t.Fatal("organização suspensa acessou API", err)
	}
	if _, err = f.service.Session(f.ctx, session); !errors.Is(err, auth.ErrDenied) {
		t.Fatal("sessão suspensa permaneceu válida", err)
	}
}

func TestRoleChangesImmediatelyRestrictExistingAPIKeys(t *testing.T) {
	f := identityDatabase(t)
	owner, _ := f.user(t, "roles-live", false)
	member, err := f.service.CreateDevelopmentMember(f.ctx, owner.Organization, "admin-local@example.test", fixturePassword, "admin")
	if err != nil {
		t.Fatal(err)
	}
	token, err := f.service.Login(f.ctx, owner.Organization, member.Email, fixturePassword, "role-ip")
	if err != nil {
		t.Fatal(err)
	}
	admin, err := f.service.Session(f.ctx, token)
	if err != nil {
		t.Fatal(err)
	}
	_, raw, err := f.service.IssueKey(f.ctx, admin, "antes da mudança", []string{"calculations:run"})
	if err != nil {
		t.Fatal(err)
	}
	if err = f.service.SetRole(f.ctx, admin, admin.UserID, "owner"); !errors.Is(err, auth.ErrDenied) {
		t.Fatal("admin elevou-se a owner", err)
	}
	if err = f.service.SetRole(f.ctx, owner, admin.UserID, "viewer"); err != nil {
		t.Fatal(err)
	}
	if _, err = f.service.BeginAPI(f.ctx, raw, "calculations:run", "role-ip", true); !errors.Is(err, auth.ErrDenied) {
		t.Fatal("chave manteve permissão revogada", err)
	}
	p, err := f.service.Session(f.ctx, token)
	if err != nil || p.Role != "viewer" || p.Can("keys:manage") {
		t.Fatal(p, err)
	}
}

func TestCompositeOwnershipConstraintsAndLastOwnerConcurrency(t *testing.T) {
	f := identityDatabase(t)
	a, _ := f.user(t, "ownership-a", false)
	b, _ := f.user(t, "ownership-b", false)
	if _, err := f.pool.Exec(f.ctx, "INSERT INTO api_keys(organization_id,created_by,name,prefix,token_hash,scopes) VALUES($1,$2,'invalid','fixture',$3,ARRAY['account:read'])", a.OrganizationID, b.UserID, auth.Digest("cross-organization-fixture")); err == nil {
		t.Fatal("FK permitiu chave de usuário de outra organização")
	}
	second, err := f.service.CreateDevelopmentMember(f.ctx, a.Organization, "second-owner@example.test", fixturePassword, "owner")
	if err != nil {
		t.Fatal(err)
	}
	token, err := f.service.Login(f.ctx, a.Organization, second.Email, fixturePassword, "owner-ip")
	if err != nil {
		t.Fatal(err)
	}
	second, err = f.service.Session(f.ctx, token)
	if err != nil {
		t.Fatal(err)
	}
	results := make(chan error, 2)
	go func() { results <- f.service.SetRole(f.ctx, a, second.UserID, "viewer") }()
	go func() { results <- f.service.SetRole(f.ctx, second, a.UserID, "viewer") }()
	succeeded := 0
	for i := 0; i < 2; i++ {
		err := <-results
		if err == nil {
			succeeded++
		} else if !errors.Is(err, auth.ErrDenied) && !errors.Is(err, auth.ErrConflict) {
			t.Fatal(err)
		}
	}
	var owners int
	if err = f.pool.QueryRow(f.ctx, "SELECT count(*) FROM memberships WHERE organization_id=$1 AND role_code='owner' AND active", a.OrganizationID).Scan(&owners); err != nil || owners != 1 || succeeded != 1 {
		t.Fatal(owners, succeeded, err)
	}
}

func TestWebMFAAndScopedAPIIdentity(t *testing.T) {
	f := identityDatabase(t)
	admin, _ := f.user(t, "web-mfa", true)
	other, _ := f.user(t, "web-key", false)
	key, raw, err := f.service.IssueKey(f.ctx, other, "outra organização", []string{"account:read"})
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewUnstartedServer(nil)
	origin := "https://" + srv.Listener.Addr().String()
	identity, err := auth.New(f.pool, bytes.Repeat([]byte{9}, 32), auth.Options{Origin: origin, CookieSecure: true, Clock: func() time.Time { return time.Unix(0, f.clock.Load()) }})
	if err != nil {
		t.Fatal(err)
	}
	srv.Config.Handler = transport.NewWithIdentity([32]byte{}, slog.New(slog.NewTextHandler(io.Discard, nil)), identity)
	srv.StartTLS()
	t.Cleanup(srv.Close)
	client := srv.Client()
	client.Jar, _ = cookiejar.New(nil)
	client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	get := func(path string) *http.Response {
		r, err := client.Get(origin + path)
		if err != nil {
			t.Fatal(err)
		}
		return r
	}
	post := func(path string, v url.Values) *http.Response {
		req, _ := http.NewRequest("POST", origin+path, strings.NewReader(v.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req.Header.Set("Origin", origin)
		r, err := client.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		return r
	}
	csrf := csrfFrom(t, httpBody(t, get("/login")))
	res := post("/login", url.Values{"csrf": {csrf}, "organization": {admin.Organization}, "email": {admin.Email}, "password": {fixturePassword}})
	if res.StatusCode != 303 {
		t.Fatal(res.StatusCode, httpBody(t, res))
	}
	_ = httpBody(t, res)
	res = get("/admin")
	if res.StatusCode != 303 || res.Header.Get("Location") != "/account/mfa" {
		t.Fatal("admin acessou sem MFA", res.StatusCode)
	}
	_ = httpBody(t, res)
	csrf = csrfFrom(t, httpBody(t, get("/account/mfa")))
	res = post("/account/mfa/setup", url.Values{"csrf": {csrf}})
	body := httpBody(t, res)
	pattern := regexp.MustCompile(`<code>([A-Z2-7]{32})</code>`)
	match := pattern.FindStringSubmatch(body)
	if len(match) != 2 {
		t.Fatal("segredo de configuração ausente", res.StatusCode)
	}
	code, _ := auth.TOTP(match[1], time.Unix(0, f.clock.Load()), 6)
	res = post("/account/mfa/verify", url.Values{"csrf": {csrf}, "code": {code}})
	body = httpBody(t, res)
	if res.StatusCode != 200 || !strings.Contains(body, "Cada código funciona uma única vez") {
		t.Fatal("confirmação MFA", res.StatusCode)
	}
	res = get("/admin")
	if res.StatusCode != 200 {
		t.Fatal("admin verificado bloqueado", res.StatusCode)
	}
	_ = httpBody(t, res)
	req, _ := http.NewRequest("GET", origin+"/api/v1/me", nil)
	req.Header.Set("X-API-Key", raw)
	res, err = client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	body = httpBody(t, res)
	if res.StatusCode != 200 || !strings.Contains(body, other.OrganizationID) || strings.Contains(body, admin.OrganizationID) {
		t.Fatal("sessão confundida com identidade da API", res.StatusCode)
	}
	csrf = csrfFrom(t, httpBody(t, get("/account")))
	res = post("/account/keys/"+key.ID+"/revoke", url.Values{"csrf": {csrf}})
	if res.StatusCode != 404 {
		t.Fatal("IDOR de chave via HTTP", res.StatusCode)
	}
	_ = httpBody(t, res)
}

var csrfPattern = regexp.MustCompile(`name="csrf" value="([^"]+)"`)

func httpBody(t *testing.T, response *http.Response) string {
	t.Helper()
	defer response.Body.Close()
	b, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}
func csrfFrom(t *testing.T, body string) string {
	t.Helper()
	match := csrfPattern.FindStringSubmatch(body)
	if len(match) != 2 {
		t.Fatal("CSRF ausente no formulário")
	}
	return match[1]
}
func TestHTTPSLoginCSRFAndOrganizationAuthorization(t *testing.T) {
	f := identityDatabase(t)
	owner, _ := f.user(t, "web-owner", false)
	other, _ := f.user(t, "web-other", false)
	srv := httptest.NewUnstartedServer(nil)
	origin := "https://" + srv.Listener.Addr().String()
	identity, err := auth.New(f.pool, bytes.Repeat([]byte{9}, 32), auth.Options{Origin: origin, CookieSecure: true})
	if err != nil {
		t.Fatal(err)
	}
	srv.Config.Handler = transport.NewWithIdentity([32]byte{}, slog.New(slog.NewTextHandler(io.Discard, nil)), identity)
	srv.StartTLS()
	t.Cleanup(srv.Close)
	client := srv.Client()
	client.Jar, _ = cookiejar.New(nil)
	client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	post := func(path string, values url.Values, originValue string) *http.Response {
		req, _ := http.NewRequest("POST", origin+path, strings.NewReader(values.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req.Header.Set("Origin", originValue)
		res, err := client.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		return res
	}
	response, err := client.Get(origin + "/app")
	if err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != 303 {
		t.Fatal(response.StatusCode)
	}
	_ = httpBody(t, response)
	response, err = client.Get(origin + "/login")
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range response.Cookies() {
		if !c.Secure || !c.HttpOnly || c.SameSite != http.SameSiteStrictMode || c.Path != "/" {
			t.Fatal("cookie inseguro", c.Name)
		}
	}
	csrf := csrfFrom(t, httpBody(t, response))
	values := url.Values{"organization": {owner.Organization}, "email": {owner.Email}, "password": {fixturePassword}, "csrf": {csrf}}
	response = post("/login", values, "https://evil.example")
	if response.StatusCode != 403 {
		t.Fatal("Origin ignorada", response.StatusCode)
	}
	_ = httpBody(t, response)
	values.Set("csrf", "forged")
	response = post("/login", values, origin)
	if response.StatusCode != 403 {
		t.Fatal("CSRF ignorado", response.StatusCode)
	}
	_ = httpBody(t, response)
	values.Set("csrf", csrf)
	response = post("/login", values, origin)
	if response.StatusCode != 303 {
		t.Fatal("login", response.StatusCode, httpBody(t, response))
	}
	for _, c := range response.Cookies() {
		if c.Name == "rj_session" && (!c.Secure || !c.HttpOnly || c.SameSite != http.SameSiteStrictMode) {
			t.Fatal("sessão insegura")
		}
	}
	_ = httpBody(t, response)
	response, err = client.Get(origin + "/account/members")
	if err != nil {
		t.Fatal(err)
	}
	body := httpBody(t, response)
	if response.StatusCode != 200 || strings.Contains(body, other.Email) || !strings.Contains(body, owner.Email) {
		t.Fatal("vazamento de organização", response.StatusCode)
	}
	csrf = csrfFrom(t, body)
	response = post("/account/members/"+other.UserID+"/role", url.Values{"csrf": {csrf}, "role": {"viewer"}}, origin)
	if response.StatusCode != 404 {
		t.Fatal("IDOR de usuário", response.StatusCode)
	}
	_ = httpBody(t, response)
	response, err = client.Get(origin + "/admin")
	if err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != 403 {
		t.Fatal("owner ganhou administração global", response.StatusCode)
	}
	_ = httpBody(t, response)
	response, err = client.Get(origin + "/v1/me")
	if err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != 401 {
		t.Fatal("cookie substituiu API key", response.StatusCode)
	}
	_ = httpBody(t, response)
	response = post("/logout", url.Values{"csrf": {csrf}}, origin)
	if response.StatusCode != 303 {
		t.Fatal(response.StatusCode)
	}
	_ = httpBody(t, response)
	response, err = client.Get(origin + "/portal")
	if err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != 303 {
		t.Fatal("logout web falhou", response.StatusCode)
	}
	_ = httpBody(t, response)
}
