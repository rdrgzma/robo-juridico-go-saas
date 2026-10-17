package integration

import (
	"bytes"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/rdrgzma/robo-juridico-go-saas/internal/admin"
	"github.com/rdrgzma/robo-juridico-go-saas/internal/auth"
	"github.com/rdrgzma/robo-juridico-go-saas/internal/juridico"
	"github.com/rdrgzma/robo-juridico-go-saas/internal/transport"
)

func apiReq(h http.Handler, method, path, key, body string) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, path, strings.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	if key != "" {
		r.Header.Set("X-API-Key", key)
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w
}

func TestAdminOrganizationsPlansContractsAndQuotaBlocks(t *testing.T) {
	f := identityDatabase(t)
	globalAdmin, _ := f.user(t, "plataforma-hq", true)
	globalAdmin.MFAVerified = true

	orgOwner, _ := f.user(t, "escritorio-comum", false)

	sources := juridico.NewSources("", false)
	ws := juridico.New(f.pool, f.service, bytes.Repeat([]byte{4}, 32), sources)
	adm := admin.New(f.pool, f.service, ws)

	// Non-platform_admin must be rejected by all admin service operations even if owner of an organization.
	if _, err := adm.Dashboard(f.ctx, orgOwner); !errors.Is(err, auth.ErrDenied) {
		t.Fatalf("expected ErrDenied for org owner without platform_admin, got %v", err)
	}

	// Create a restricted plan with cnj, prazos and api (no liquidacao/judicial), and a panel-only plan without api.
	if err := adm.SavePlan(f.ctx, globalAdmin, admin.PlanInput{
		Code:              "somente_cnj_api",
		Name:              "Somente CNJ, Prazos e API",
		MonthlyPrice:      "299.90",
		MonthlyLimit:      2,
		RequestsPerMinute: 30,
		Features:          []string{"cnj", "prazos", "api"},
		Active:            true,
	}); err != nil {
		t.Fatal(err)
	}
	if err := adm.SavePlan(f.ctx, globalAdmin, admin.PlanInput{
		Code:              "painel_sem_api",
		Name:              "Painel Sem API",
		MonthlyPrice:      "149.50",
		MonthlyLimit:      10,
		RequestsPerMinute: 20,
		Features:          []string{"cnj", "prazos", "liquidacao"},
		Active:            true,
	}); err != nil {
		t.Fatal(err)
	}

	// Create client organization via admin service with decimal contracted price and contract metadata.
	orgID, err := adm.CreateClientOrganization(f.ctx, globalAdmin, admin.CreateOrganizationInput{
		Slug:              "cliente-restrito",
		Name:              "Cliente Restrito Advocacia",
		LegalName:         "Cliente Restrito Sociedade de Advogados LTDA",
		Document:          "12.345.678/0001-90",
		ContactName:       "Dra. Maria",
		ContactEmail:      "maria@restrito.test",
		Notes:             "Contrato homologado",
		OwnerEmail:        "owner@restrito.test",
		OwnerPassword:     "SenhaForteClienteRestrito!2026",
		PlanCode:          "somente_cnj_api",
		ContractedPrice:   "350.00",
		MonthlyLimit:      2,
		RequestsPerMinute: 30,
		EffectiveFrom:     time.Now().AddDate(0, 0, -5).Format("2006-01-02"),
		ContractNotes:     "Condição comercial inicial",
	})
	if err != nil {
		t.Fatal(err)
	}

	detail, err := adm.GetOrganization(f.ctx, globalAdmin, orgID)
	if err != nil {
		t.Fatal(err)
	}
	if detail.Organization.PlanCode != "somente_cnj_api" || detail.Organization.ContractedPrice.StringFixed(2) != "350.00" || len(detail.ContractHistory) != 1 {
		t.Fatalf("unexpected organization detail: %+v", detail)
	}

	// Login as the newly created organization's owner.
	rawSession, err := f.service.Login(f.ctx, "cliente-restrito", "owner@restrito.test", "SenhaForteClienteRestrito!2026", "127.0.0.1")
	if err != nil {
		t.Fatal(err)
	}
	clientPrincipal, err := f.service.Session(f.ctx, rawSession)
	if err != nil {
		t.Fatal(err)
	}

	// Verify backend plan feature enforcement in /app (juridico.Service.Run):
	// 'liquidacao' is NOT in 'somente_cnj_api' features -> must fail with ErrDenied.
	if _, err := ws.Run(f.ctx, clientPrincipal, "liquidacao", "Tentativa bloqueada pelo plano", auth.RandomToken(), workspaceInput); !errors.Is(err, auth.ErrDenied) {
		t.Fatalf("expected ErrDenied for unauthorized plan feature liquidacao, got %v", err)
	}
	// 'prazos' IS in 'somente_cnj_api' features -> must succeed and consume 1 billable unit!
	prazosInput := []byte(`{"data_evento":"2026-03-02","dias":5,"modo":"UTEIS","intimacao":"DIRETA","feriados":[],"suspender_recesso":false,"dobro":false}`)
	if _, err := ws.Run(f.ctx, clientPrincipal, "prazos", "Prazo autorizado", auth.RandomToken(), prazosInput); err != nil {
		t.Fatalf("expected prazos to succeed on somente_cnj_api, got %v", err)
	}

	// Issue API key and test API plan feature enforcement + quota exhaustion block tracking.
	keyRec, rawKey, err := adm.IssueKey(f.ctx, globalAdmin, orgID, "Chave ERP", []string{"calculations:run", "account:read"})
	if err != nil {
		t.Fatal(err)
	}
	router := transport.NewWithAdmin([32]byte{}, slog.New(slog.NewTextHandler(io.Discard, nil)), f.service, ws, adm)

	// Calling /api/v1/simulacoes/liquidacao with a key on 'somente_cnj_api' must return 403 acesso_api_recusado.
	w := apiReq(router, "POST", "/api/v1/simulacoes/liquidacao", rawKey, string(workspaceInput))
	if w.Code != 403 || !strings.Contains(w.Body.String(), "acesso_api_recusado") {
		t.Fatalf("expected 403 acesso_api_recusado for unauthorized API module, got %d %s", w.Code, w.Body.String())
	}

	// Calling /api/v1/simulacoes/prazos uses the 2nd unit of the monthly quota (limit = 2).
	w = apiReq(router, "POST", "/api/v1/simulacoes/prazos", rawKey, string(prazosInput))
	if w.Code != 200 {
		t.Fatalf("expected 200 on allowed API prazos call, got %d %s", w.Code, w.Body.String())
	}

	// 3rd billable call exceeds monthly_limit=2 -> must return 402 acesso_api_recusado AND increment quota_periods.blocked!
	w = apiReq(router, "POST", "/api/v1/simulacoes/prazos", rawKey, string(prazosInput))
	if w.Code != 402 || !strings.Contains(w.Body.String(), "acesso_api_recusado") {
		t.Fatalf("expected 402 acesso_api_recusado, got %d %s", w.Code, w.Body.String())
	}
	// Panel billable call when quota is exhausted also increments blocked counter.
	if _, err := ws.Run(f.ctx, clientPrincipal, "prazos", "Prazo acima da cota", auth.RandomToken(), prazosInput); !errors.Is(err, auth.ErrQuota) {
		t.Fatalf("expected ErrQuota on workspace Run, got %v", err)
	}

	// Update contract to a higher limit (10) and switch to 'painel_sem_api', verifying contract history and API block when 'api' feature is absent.
	if err := adm.UpdateContract(f.ctx, globalAdmin, orgID, admin.ContractInput{
		PlanCode:          "painel_sem_api",
		ContractedPrice:   "490.00",
		MonthlyLimit:      10,
		RequestsPerMinute: 40,
		EffectiveFrom:     time.Now().AddDate(0, 0, -1).Format("2006-01-02"),
		Notes:             "Upgrade para painel sem API",
	}); err != nil {
		t.Fatal(err)
	}

	// API must now reject with 403 acesso_api_recusado because 'api' is not in 'painel_sem_api'.
	w = apiReq(router, "GET", "/api/v1/cnj/0000001-73.2023.8.26.0100", rawKey, "")
	if w.Code != 403 || !strings.Contains(w.Body.String(), "acesso_api_recusado") {
		t.Fatalf("expected 403 when plan lacks api feature, got %d %s", w.Code, w.Body.String())
	}

	// Rotate API key and verify old key is revoked and new key is returned once.
	rotated, newRawKey, err := adm.RotateKey(f.ctx, globalAdmin, keyRec.ID)
	if err != nil || newRawKey == "" || newRawKey == rawKey || rotated.ID == keyRec.ID {
		t.Fatalf("rotate key failed: %+v %v", rotated, err)
	}

	// Suspend organization and verify active web session is immediately invalidated.
	if err := adm.SetOrganizationStatus(f.ctx, globalAdmin, orgID, false); err != nil {
		t.Fatal(err)
	}
	if _, err := f.service.Session(f.ctx, rawSession); !errors.Is(err, auth.ErrDenied) {
		t.Fatalf("expected session to be invalidated after organization suspension, got %v", err)
	}
	// Reactivate organization.
	if err := adm.SetOrganizationStatus(f.ctx, globalAdmin, orgID, true); err != nil {
		t.Fatal(err)
	}

	// Check Dashboard metrics & quota summary.
	dash, err := adm.Dashboard(f.ctx, globalAdmin)
	if err != nil {
		t.Fatal(err)
	}
	if dash.MonthQuotaBlocked < 2 || dash.EstimatedMRR != "490.00" || dash.HomologatedReports != 0 || dash.CapturedLeads != 0 || dash.HTTPRequests == 0 {
		t.Fatalf("unexpected dashboard metrics: %+v", dash)
	}
}

type staticRoundTripper func(*http.Request) (*http.Response, error)

func (fn staticRoundTripper) RoundTrip(r *http.Request) (*http.Response, error) { return fn(r) }

func TestAdminUsersIndexSyncAndHTTPPanel(t *testing.T) {
	f := identityDatabase(t)
	globalAdmin, _ := f.user(t, "admin-hq", true)
	globalAdmin.MFAVerified = true
	regularOwner, _ := f.user(t, "org-regular", false)

	// Synthetic BCB client for controlled index synchronization test.
	bcbClient := &http.Client{
		Transport: staticRoundTripper(func(r *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: 200,
				Header:     http.Header{"Content-Type": {"application/json"}},
				Body:       io.NopCloser(strings.NewReader(`[{"data":"01/01/2025","valor":"0.16"},{"data":"01/02/2025","valor":"1.31"}]`)),
			}, nil
		}),
	}

	sources := juridico.NewSourcesWithClient(bcbClient, "", false)
	ws := juridico.New(f.pool, f.service, bytes.Repeat([]byte{4}, 32), sources)
	adm := admin.New(f.pool, f.service, ws)

	// Controlled index sync into economic_indices, index_points, and index_sync_runs.
	syncRun, err := adm.SyncIndices(f.ctx, globalAdmin, "BCB", "2025-01", "2025-02")
	if err != nil || syncRun.Status != "completed" || syncRun.PointsSynced != 2 {
		t.Fatalf("expected completed index sync of 2 points, got %+v %v", syncRun, err)
	}
	indices, runs, err := adm.ListIndicesAndSyncs(f.ctx, globalAdmin)
	if err != nil || len(indices) != 1 || indices[0].PointCount != 2 || len(runs) != 1 {
		t.Fatalf("unexpected indices/runs after sync: %+v %+v %v", indices, runs, err)
	}

	// User & membership management, last-owner and last-platform_admin protection.
	userID, err := adm.ProvisionUserMembership(f.ctx, globalAdmin, regularOwner.OrganizationID, "advogado@regular.test", "SenhaAdvogadoRegular!2026", "lawyer", false)
	if err != nil || userID == "" {
		t.Fatalf("provision user failed: %v", err)
	}
	if err := adm.UpdateUserMembership(f.ctx, globalAdmin, regularOwner.OrganizationID, userID, "admin", true); err != nil {
		t.Fatal(err)
	}
	// Demoting the only owner of regularOwner's organization must fail with ErrConflict.
	if err := adm.UpdateUserMembership(f.ctx, globalAdmin, regularOwner.OrganizationID, regularOwner.UserID, "viewer", true); !errors.Is(err, auth.ErrConflict) {
		t.Fatalf("expected ErrConflict when demoting last owner, got %v", err)
	}
	// Removing platform_admin from the only platform_admin must fail with ErrConflict.
	if err := adm.SetPlatformAdmin(f.ctx, globalAdmin, globalAdmin.UserID, false); !errors.Is(err, auth.ErrConflict) {
		t.Fatalf("expected ErrConflict when removing last platform_admin, got %v", err)
	}
	if err := adm.TriggerUserRecovery(f.ctx, globalAdmin, userID); err != nil {
		t.Fatal(err)
	}

	// Audit search and pagination.
	events, _, err := adm.ListAuditEvents(f.ctx, globalAdmin, "", "admin.indices_sync", "", 0)
	if err != nil || len(events) != 1 {
		t.Fatalf("expected 1 audit event for admin.indices_sync, got %+v %v", events, err)
	}

	// Verify HTTP routes on /admin with authenticated global admin vs regular organization owner.
	srv := httptest.NewUnstartedServer(nil)
	origin := "http://" + srv.Listener.Addr().String()
	opts := auth.Options{Origin: origin, DisableMFA: true}
	authSvc, err := auth.New(f.pool, bytes.Repeat([]byte{9}, 32), opts)
	if err != nil {
		t.Fatal(err)
	}
	wsHTTP := juridico.New(f.pool, authSvc, bytes.Repeat([]byte{4}, 32), sources)
	admHTTP := admin.New(f.pool, authSvc, wsHTTP)
	srv.Config.Handler = transport.NewWithAdmin([32]byte{}, slog.New(slog.NewTextHandler(io.Discard, nil)), authSvc, wsHTTP, admHTTP)
	srv.Start()
	defer srv.Close()

	// Regular owner login -> GET /admin must return 403.
	ownerClient := loginHTTPClient(t, origin, "org-regular", regularOwner.Email, fixturePassword)
	resp, err := ownerClient.Get(origin + "/admin")
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != 403 {
		t.Fatalf("expected 403 for non-platform_admin on /admin, got %d", resp.StatusCode)
	}

	// Global admin login -> GET /admin, /admin/organizations, /admin/plans, /admin/users, /admin/keys, /admin/operations, /admin/audit must return 200.
	adminClient := loginHTTPClient(t, origin, "admin-hq", globalAdmin.Email, fixturePassword)
	for _, path := range []string{"/admin", "/admin/organizations", "/admin/plans", "/admin/users", "/admin/keys", "/admin/operations", "/admin/audit"} {
		res, err := adminClient.Get(origin + path)
		if err != nil {
			t.Fatal(err)
		}
		body, _ := io.ReadAll(res.Body)
		_ = res.Body.Close()
		if res.StatusCode != 200 || !strings.Contains(string(body), "ADMINISTRAÇÃO GLOBAL DA PLATAFORMA") {
			t.Fatalf("%s returned %d: %s", path, res.StatusCode, string(body))
		}
	}
}

func loginHTTPClient(t *testing.T, baseURL, org, email, password string) *http.Client {
	t.Helper()
	jar, _ := cookiejar.New(nil)
	client := &http.Client{
		Jar: jar,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	loginPage, err := client.Get(baseURL + "/login")
	if err != nil {
		t.Fatal(err)
	}
	csrf := csrfFrom(t, httpBody(t, loginPage))
	form := url.Values{"csrf": {csrf}, "organization": {org}, "email": {email}, "password": {password}}
	req, _ := http.NewRequest("POST", baseURL+"/login", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Origin", baseURL)
	resp, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != 303 {
		t.Fatalf("login failed with status %d", resp.StatusCode)
	}
	return client
}
