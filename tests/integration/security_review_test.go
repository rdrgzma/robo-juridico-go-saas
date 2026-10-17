package integration

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/rdrgzma/robo-juridico-go-saas/internal/admin"
	"github.com/rdrgzma/robo-juridico-go-saas/internal/auth"
	"github.com/rdrgzma/robo-juridico-go-saas/internal/juridico"
	"github.com/rdrgzma/robo-juridico-go-saas/internal/portal"
)

func TestSecuritySQLInjectionResistance(t *testing.T) {
	f := identityDatabase(t)
	globalAdmin, _ := f.user(t, "sec-admin", true)
	globalAdmin.MFAVerified = true

	sources := juridico.NewSources("", false)
	ws := juridico.New(f.pool, f.service, bytes.Repeat([]byte{6}, 32), sources)
	adm := admin.New(f.pool, f.service, ws)
	ptl := portal.New(f.pool, f.service, ws)

	// Injections in search query parameters
	sqlPayloads := []string{
		"' OR 1=1 --",
		"'; DROP TABLE organizations; --",
		"' UNION SELECT id, email, password_hash FROM users --",
		"1' AND SLEEP(5) --",
		"admin'--",
		"{\"key\": \"'; DROP TABLE legal_operations; --\"}",
	}

	for _, payload := range sqlPayloads {
		// Admin organization search with SQL injection payload
		orgs, err := adm.ListOrganizations(f.ctx, globalAdmin, payload, "all", "")
		if err != nil {
			t.Fatalf("unexpected database crash on injection payload %q: %v", payload, err)
		}
		// Must not return all organizations due to injection
		for _, o := range orgs {
			if strings.Contains(o.Name, "'; DROP") {
				t.Fatalf("unexpected match for %q", payload)
			}
		}

		// Portal usage history with SQL injection in filter
		_, _, err = ptl.UsageHistory(f.ctx, globalAdmin, payload, payload, time.Time{})
		if err != nil {
			t.Fatalf("unexpected database crash on portal injection %q: %v", payload, err)
		}

		// Portal operations history with SQL injection in filter
		_, _, err = ptl.ListOperations(f.ctx, globalAdmin, payload, time.Time{}, "")
		if err != nil {
			t.Fatalf("unexpected database crash on operations injection %q: %v", payload, err)
		}
	}
}

func TestSecurityHMACIntegrityAndTamperResistance(t *testing.T) {
	f := identityDatabase(t)
	user, _ := f.user(t, "hmac-org", false)
	user.MFAVerified = true

	ws := juridico.New(f.pool, f.service, bytes.Repeat([]byte{7}, 32), nil)

	calcInput := []byte(`{"principal":"1000.00","fator_correcao":"1","taxa_mensal":"1","meses":2,"compostos":false,"pagamento":"0","imputar_juros_primeiro":true,"multa_percentual":"0","honorarios_percentual":"0"}`)
	op, err := ws.Run(f.ctx, user, "liquidacao", "Laudo Pericial Teste", auth.RandomToken(), calcInput)
	if err != nil {
		t.Fatal(err)
	}

	sig := ws.Integrity(op)
	if sig == "" {
		t.Fatalf("expected non-empty HMAC signature")
	}

	// Signature must verify on unmodified operation
	if !ws.Verify(op, sig) {
		t.Fatalf("expected valid signature verification on original operation")
	}

	// Single byte mutation in result must immediately fail verification
	tamperedResult := bytes.Replace(op.Result, []byte("1020.00"), []byte("9999.00"), 1)
	tamperedOp := op
	tamperedOp.Result = json.RawMessage(tamperedResult)

	if ws.Verify(tamperedOp, sig) {
		t.Fatalf("HMAC verification failed to detect tampered result payload")
	}

	// Single byte mutation in input must immediately fail verification
	tamperedInput := bytes.Replace(op.Input, []byte("1000.00"), []byte("2000.00"), 1)
	tamperedInputOp := op
	tamperedInputOp.Input = json.RawMessage(tamperedInput)

	if ws.Verify(tamperedInputOp, sig) {
		t.Fatalf("HMAC verification failed to detect tampered input payload")
	}
}

func TestSecurityOriginAndCSRFProtection(t *testing.T) {
	f := identityDatabase(t)
	user, sessionToken := f.user(t, "csrf-test-org", false)
	_ = user

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !f.service.ValidateWebForm(w, r) {
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("OK"))
	})

	// 1. Request with untrusted Origin must be rejected with 403
	req := httptest.NewRequest("POST", "http://localhost:8081/test", strings.NewReader("name=novo"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Origin", "http://evil-attacker.com")
	req.AddCookie(&http.Cookie{Name: "rj_session", Value: sessionToken})

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for untrusted Origin, got %d", rec.Code)
	}

	// 2. Request without CSRF token must be rejected with 403
	req2 := httptest.NewRequest("POST", "http://localhost:8081/test", strings.NewReader("name=novo"))
	req2.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req2.Header.Set("Origin", "http://localhost:8081")
	req2.AddCookie(&http.Cookie{Name: "rj_session", Value: sessionToken})

	rec2 := httptest.NewRecorder()
	handler.ServeHTTP(rec2, req2)

	if rec2.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for missing CSRF, got %d", rec2.Code)
	}

	// 3. Request with forged CSRF token must be rejected with 403
	formData := url.Values{
		"name": {"novo"},
		"csrf": {"forged_invalid_csrf_token_value"},
	}
	req3 := httptest.NewRequest("POST", "http://localhost:8081/test", strings.NewReader(formData.Encode()))
	req3.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req3.Header.Set("Origin", "http://localhost:8081")
	req3.AddCookie(&http.Cookie{Name: "rj_session", Value: sessionToken})

	rec3 := httptest.NewRecorder()
	handler.ServeHTTP(rec3, req3)

	if rec3.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for forged CSRF, got %d", rec3.Code)
	}
}
