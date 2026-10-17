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
	"github.com/rdrgzma/robo-juridico-go-saas/internal/portal"
	"github.com/rdrgzma/robo-juridico-go-saas/internal/transport"
)

func TestPortalTenantIsolationAndIDORProtection(t *testing.T) {
	f := identityDatabase(t)
	globalAdmin, _ := f.user(t, "portal-admin", true)
	globalAdmin.MFAVerified = true

	sources := juridico.NewSources("", false)
	ws := juridico.New(f.pool, f.service, bytes.Repeat([]byte{5}, 32), sources)
	adm := admin.New(f.pool, f.service, ws)
	ptl := portal.New(f.pool, f.service, ws)

	// Create test plan with API and all calculator features
	if err := adm.SavePlan(f.ctx, globalAdmin, admin.PlanInput{
		Code:              "plano_completo_portal",
		Name:              "Plano Completo Portal",
		MonthlyPrice:      "590.00",
		MonthlyLimit:      100,
		RequestsPerMinute: 60,
		Features:          []string{"cnj", "liquidacao", "prazos", "judicial", "api"},
		Active:            true,
	}); err != nil {
		t.Fatal(err)
	}

	// Create Organization A
	orgAOwner, _ := f.user(t, "org-alpha-portal", false)
	orgAOwner.MFAVerified = true

	// Create Organization B
	orgBOwner, _ := f.user(t, "org-beta-portal", false)
	orgBOwner.MFAVerified = true

	// Org A creates a legal operation (calculation)
	calcInput := []byte(`{"principal":"1000.00","fator_correcao":"1","taxa_mensal":"1","meses":2,"compostos":false,"pagamento":"0","imputar_juros_primeiro":true,"multa_percentual":"0","honorarios_percentual":"0"}`)
	opA, err := ws.Run(f.ctx, orgAOwner, "liquidacao", "Execução Alpha 100", auth.RandomToken(), calcInput)
	if err != nil {
		t.Fatal(err)
	}

	// Verify Org A sees it in portal dashboard and history
	dashA, err := ptl.Dashboard(f.ctx, orgAOwner)
	if err != nil {
		t.Fatal(err)
	}
	if dashA.TotalOperations < 1 || dashA.MonthCalculations < 1 {
		t.Fatalf("expected Org A to have operations, got %+v", dashA)
	}
	if dashA.HomologatedReports != 0 {
		t.Fatalf("expected honest 0 homologated reports, got %d", dashA.HomologatedReports)
	}

	historyA, _, err := ptl.ListOperations(f.ctx, orgAOwner, "", time.Time{}, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(historyA) == 0 || historyA[0].ID != opA.ID {
		t.Fatalf("expected Org A to see op %s, got %+v", opA.ID, historyA)
	}

	// Verify Org B does NOT see Org A's operation in history
	historyB, _, err := ptl.ListOperations(f.ctx, orgBOwner, "", time.Time{}, "")
	if err != nil {
		t.Fatal(err)
	}
	for _, op := range historyB {
		if op.ID == opA.ID {
			t.Fatalf("cross-tenant leak: Org B saw Org A's operation %s", opA.ID)
		}
	}

	// IDOR Protection: Org B requesting Org A's operation directly must return ErrNotFound
	if _, _, err := ptl.GetOperation(f.ctx, orgBOwner, opA.ID); !errors.Is(err, auth.ErrNotFound) {
		t.Fatalf("expected ErrNotFound when Org B accesses Org A's op, got %v", err)
	}

	// IDOR Protection via HTTP routes
	handler := transport.NewWithPortal([32]byte{}, slog.New(slog.NewTextHandler(io.Discard, nil)), f.service, ws, adm, ptl)

	// Simulate Org B browser session
	jar, _ := cookiejar.New(nil)
	clientB := &http.Client{
		Jar: jar,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	server := httptest.NewServer(handler)
	defer server.Close()

	tokenB, err := f.service.Login(f.ctx, "org-beta-portal", "org-beta-portal@example.test", fixturePassword, "127.0.0.1")
	if err != nil {
		t.Fatal(err)
	}
	serverURL, _ := url.Parse(server.URL)
	jar.SetCookies(serverURL, []*http.Cookie{{
		Name:  "rj_session",
		Value: tokenB,
		Path:  "/",
	}})

	// Org B requests Org A's operation detail via HTTP -> must be 404
	respDetail, err := clientB.Get(server.URL + "/portal/history/" + opA.ID)
	if err != nil {
		t.Fatal(err)
	}
	_ = respDetail.Body.Close()
	if respDetail.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404 for cross-tenant history view, got %d", respDetail.StatusCode)
	}

	// Org B requests download of Org A's operation -> must be 404
	for _, fmt := range []string{"csv", "json", "html", "xlsx"} {
		respDownload, err := clientB.Get(server.URL + "/portal/history/" + opA.ID + "/download/" + fmt)
		if err != nil {
			t.Fatal(err)
		}
		_ = respDownload.Body.Close()
		if respDownload.StatusCode != http.StatusNotFound {
			t.Fatalf("expected 404 for cross-tenant download %s, got %d", fmt, respDownload.StatusCode)
		}
	}

	// Org B requests recalculation of Org A's operation -> must be 404
	respRecalc, err := clientB.Get(server.URL + "/portal/history/" + opA.ID + "/recalculate")
	if err != nil {
		t.Fatal(err)
	}
	_ = respRecalc.Body.Close()
	if respRecalc.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404 for cross-tenant recalculate, got %d", respRecalc.StatusCode)
	}
}

func TestPortalPlanAndKeyPermissions(t *testing.T) {
	f := identityDatabase(t)
	globalAdmin, _ := f.user(t, "keys-admin", true)
	globalAdmin.MFAVerified = true

	sources := juridico.NewSources("", false)
	ws := juridico.New(f.pool, f.service, bytes.Repeat([]byte{5}, 32), sources)
	adm := admin.New(f.pool, f.service, ws)
	ptl := portal.New(f.pool, f.service, ws)

	// Plan without API feature
	if err := adm.SavePlan(f.ctx, globalAdmin, admin.PlanInput{
		Code:              "plano_sem_api",
		Name:              "Plano Sem API",
		MonthlyPrice:      "199.00",
		MonthlyLimit:      50,
		RequestsPerMinute: 20,
		Features:          []string{"cnj", "liquidacao"},
		Active:            true,
	}); err != nil {
		t.Fatal(err)
	}

	// Create Organization on plan without API
	orgNoAPIID, err := adm.CreateClientOrganization(f.ctx, globalAdmin, admin.CreateOrganizationInput{
		Slug:              "escritorio-no-api",
		Name:              "Escritório No API",
		LegalName:         "Escritório No API Sociedade",
		Document:          "99.888.777/0001-11",
		ContactName:       "Dr. Silva",
		ContactEmail:      "silva@noapi.test",
		OwnerEmail:        "owner@noapi.test",
		OwnerPassword:     "SenhaValidaOwner123!2026",
		PlanCode:          "plano_sem_api",
		ContractedPrice:   "199.00",
		MonthlyLimit:      50,
		RequestsPerMinute: 20,
		EffectiveFrom:     time.Now().AddDate(0, 0, -1).Format("2006-01-02"),
	})
	if err != nil {
		t.Fatal(err)
	}
	_ = orgNoAPIID

	noAPIToken, err := f.service.Login(f.ctx, "escritorio-no-api", "owner@noapi.test", "SenhaValidaOwner123!2026", "127.0.0.1")
	if err != nil {
		t.Fatal(err)
	}
	ownerNoAPI, err := f.service.Session(f.ctx, noAPIToken)
	if err != nil {
		t.Fatal(err)
	}
	ownerNoAPI.MFAVerified = true

	// Check KeysData: APIAllowed must be false
	kd, err := ptl.KeysData(f.ctx, ownerNoAPI)
	if err != nil {
		t.Fatal(err)
	}
	if kd.APIAllowed {
		t.Fatalf("expected APIAllowed to be false for plan without api feature, got true")
	}

	// Attempting to issue key on plan without API must fail with ErrDenied
	if _, _, err := ptl.IssueKey(f.ctx, ownerNoAPI, "Chave Proibida", []string{"calculations:run"}); !errors.Is(err, auth.ErrDenied) {
		t.Fatalf("expected ErrDenied when issuing key on plan without API, got %v", err)
	}

	// Now test organization on default starter plan (which includes API)
	orgOwner, _ := f.user(t, "org-api-allowed", false)
	orgOwner.MFAVerified = true

	kdAllowed, err := ptl.KeysData(f.ctx, orgOwner)
	if err != nil {
		t.Fatal(err)
	}
	if !kdAllowed.APIAllowed {
		t.Fatalf("expected APIAllowed to be true on starter plan")
	}

	// Issue key
	key, rawSecret, err := ptl.IssueKey(f.ctx, orgOwner, "Integração CRM", []string{"calculations:run"})
	if err != nil {
		t.Fatal(err)
	}
	if rawSecret == "" || key.Prefix == "" || !strings.HasPrefix(rawSecret, "rj_") {
		t.Fatalf("invalid raw secret or key: %+v, raw: %s", key, rawSecret)
	}

	// Rotate key
	rotatedKey, newRawSecret, err := ptl.RotateKey(f.ctx, orgOwner, key.ID)
	if err != nil {
		t.Fatal(err)
	}
	if newRawSecret == "" || newRawSecret == rawSecret {
		t.Fatalf("expected new secret on rotation, got %s", newRawSecret)
	}

	// Revoke key
	if err := ptl.RevokeKey(f.ctx, orgOwner, rotatedKey.ID); err != nil {
		t.Fatal(err)
	}
}

func TestPortalTeamAndOrganizationSettings(t *testing.T) {
	f := identityDatabase(t)
	orgOwner, _ := f.user(t, "escritorio-team", false)
	orgOwner.MFAVerified = true

	sources := juridico.NewSources("", false)
	ws := juridico.New(f.pool, f.service, bytes.Repeat([]byte{5}, 32), sources)
	ptl := portal.New(f.pool, f.service, ws)

	// Update organization settings
	err := ptl.UpdateOrganizationSettings(f.ctx, orgOwner, portal.OrganizationSettingsInput{
		Name:         "Novo Nome Escritório Team",
		LegalName:    "Novo Nome Sociedade de Advogados",
		Document:     "11.222.333/0001-44",
		ContactName:  "Dra. Joana",
		ContactEmail: "joana@team.test",
	})
	if err != nil {
		t.Fatal(err)
	}

	dash, err := ptl.Dashboard(f.ctx, orgOwner)
	if err != nil {
		t.Fatal(err)
	}
	if dash.OrgName != "Novo Nome Escritório Team" || dash.Document != "11.222.333/0001-44" || dash.ContactEmail != "joana@team.test" {
		t.Fatalf("organization settings not reflected: %+v", dash)
	}

	// Provision new member (lawyer)
	lawyerUserID, err := f.service.ProvisionMember(f.ctx, orgOwner, "advogado@team.test", "SenhaAdvogado123!2026", "lawyer")
	if err != nil {
		t.Fatal(err)
	}

	// Provision new member (viewer)
	viewerUserID, err := f.service.ProvisionMember(f.ctx, orgOwner, "estagiario@team.test", "SenhaEstagiario123!2026", "viewer")
	if err != nil {
		t.Fatal(err)
	}

	members, err := f.service.Members(f.ctx, orgOwner)
	if err != nil {
		t.Fatal(err)
	}
	if len(members) != 3 {
		t.Fatalf("expected 3 members, got %d", len(members))
	}

	// Change role of viewer to lawyer
	if err := f.service.SetRole(f.ctx, orgOwner, viewerUserID, "lawyer"); err != nil {
		t.Fatal(err)
	}

	// Change status (deactivate) lawyer member
	if err := f.service.SetMemberStatus(f.ctx, orgOwner, lawyerUserID, false); err != nil {
		t.Fatal(err)
	}

	// Verify owner cannot demote itself if it is the only owner
	if err := f.service.SetRole(f.ctx, orgOwner, orgOwner.UserID, "lawyer"); err == nil {
		t.Fatalf("expected error when demoting sole organization owner, got nil")
	}

	// Verify viewer/lawyer cannot manage members
	viewerToken, err := f.service.Login(f.ctx, "escritorio-team", "estagiario@team.test", "SenhaEstagiario123!2026", "127.0.0.1")
	if err != nil {
		t.Fatal(err)
	}
	viewerPrincipal, err := f.service.Session(f.ctx, viewerToken)
	if err != nil {
		t.Fatal(err)
	}
	viewerPrincipal.MFAVerified = true

	// Lawyer/viewer updating org settings must return ErrDenied
	if err := ptl.UpdateOrganizationSettings(f.ctx, viewerPrincipal, portal.OrganizationSettingsInput{Name: "Tentativa Proibida"}); !errors.Is(err, auth.ErrDenied) {
		t.Fatalf("expected ErrDenied for non-admin org update, got %v", err)
	}
}

func TestPortalSecurityAndSessionManagement(t *testing.T) {
	f := identityDatabase(t)
	orgOwner, token1 := f.user(t, "escritorio-sec", false)
	orgOwner.MFAVerified = true

	sources := juridico.NewSources("", false)
	ws := juridico.New(f.pool, f.service, bytes.Repeat([]byte{5}, 32), sources)
	adm := admin.New(f.pool, f.service, ws)
	ptl := portal.New(f.pool, f.service, ws)

	// Create a second session for the same user
	token2, err := f.service.Login(f.ctx, "escritorio-sec", "escritorio-sec@example.test", fixturePassword, "192.168.1.100")
	if err != nil {
		t.Fatal(err)
	}

	// List sessions via UserSessions
	sessions, err := f.service.UserSessions(f.ctx, orgOwner, token1)
	if err != nil {
		t.Fatal(err)
	}
	if len(sessions) < 2 {
		t.Fatalf("expected at least 2 active sessions, got %d", len(sessions))
	}
	var currentFound, otherFound bool
	var otherID string
	for _, s := range sessions {
		if s.Current {
			currentFound = true
		} else {
			otherFound = true
			otherID = s.ID
		}
	}
	if !currentFound || !otherFound || otherID == "" {
		t.Fatalf("unexpected sessions breakdown: %+v", sessions)
	}

	// Revoke the other session
	if err := f.service.RevokeSession(f.ctx, orgOwner, otherID); err != nil {
		t.Fatal(err)
	}

	// Verify token2 is now invalid
	if _, err := f.service.Session(f.ctx, token2); !errors.Is(err, auth.ErrDenied) {
		t.Fatalf("expected revoked session to return ErrDenied, got %v", err)
	}

	// Verify token1 remains valid
	if _, err := f.service.Session(f.ctx, token1); err != nil {
		t.Fatalf("current session should remain valid, got %v", err)
	}

	// Change password: wrong current password must return ErrCredentials
	if _, err := f.service.ChangePassword(f.ctx, orgOwner, "SenhaErradaTotal!2026", "NovaSenhaSegura123!2026"); !errors.Is(err, auth.ErrCredentials) {
		t.Fatalf("expected ErrCredentials on wrong password, got %v", err)
	}

	// Change password: correct current password
	newToken, err := f.service.ChangePassword(f.ctx, orgOwner, fixturePassword, "NovaSenhaSegura123!2026")
	if err != nil {
		t.Fatal(err)
	}

	// Old token1 must now be revoked (due to password_version increment)
	if _, err := f.service.Session(f.ctx, token1); !errors.Is(err, auth.ErrDenied) {
		t.Fatalf("expected old session to be revoked after password change, got %v", err)
	}

	// New token must be active
	newPrincipal, err := f.service.Session(f.ctx, newToken)
	if err != nil {
		t.Fatal(err)
	}
	if newPrincipal.UserID != orgOwner.UserID {
		t.Fatalf("principal mismatch: %s vs %s", newPrincipal.UserID, orgOwner.UserID)
	}

	// Login with new password must succeed
	loginToken, err := f.service.Login(f.ctx, "escritorio-sec", "escritorio-sec@example.test", "NovaSenhaSegura123!2026", "127.0.0.1")
	if err != nil {
		t.Fatal(err)
	}
	if loginToken == "" {
		t.Fatalf("expected non-empty login token")
	}

	// Verify HTTP views render correctly
	handler := transport.NewWithPortal([32]byte{}, slog.New(slog.NewTextHandler(io.Discard, nil)), f.service, ws, adm, ptl)
	jar, _ := cookiejar.New(nil)
	client := &http.Client{Jar: jar}
	server := httptest.NewServer(handler)
	defer server.Close()

	serverURL, _ := url.Parse(server.URL)
	jar.SetCookies(serverURL, []*http.Cookie{{
		Name:  "rj_session",
		Value: loginToken,
		Path:  "/",
	}})

	// Test GET all portal views
	views := []string{
		"/portal",
		"/portal/usage",
		"/portal/keys",
		"/portal/history",
		"/portal/team",
		"/portal/security",
	}
	for _, v := range views {
		r, err := client.Get(server.URL + v)
		if err != nil {
			t.Fatalf("GET %s failed: %v", v, err)
		}
		body, _ := io.ReadAll(r.Body)
		_ = r.Body.Close()
		if r.StatusCode != http.StatusOK {
			t.Fatalf("GET %s returned status %d, body: %s", v, r.StatusCode, string(body))
		}
		if !strings.Contains(string(body), "Portal do cliente") && !strings.Contains(string(body), "Robô Jurídico") {
			t.Fatalf("GET %s missing expected header content", v)
		}
	}
}
