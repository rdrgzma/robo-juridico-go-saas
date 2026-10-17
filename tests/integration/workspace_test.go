package integration

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"mime/multipart"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/rdrgzma/robo-juridico-go-saas/internal/auth"
	"github.com/rdrgzma/robo-juridico-go-saas/internal/juridico"
	"github.com/rdrgzma/robo-juridico-go-saas/internal/transport"
)

var workspaceInput = []byte(`{"principal":"1000.00","fator_correcao":"1","taxa_mensal":"1","meses":2,"compostos":false,"pagamento":"0","imputar_juros_primeiro":true,"multa_percentual":"0","honorarios_percentual":"0"}`)

func TestWorkspaceHistoryQuotaIdempotenceAndIntegrity(t *testing.T) {
	f := identityDatabase(t)
	a, _ := f.user(t, "workspace-a", false)
	b, _ := f.user(t, "workspace-b", false)
	s := juridico.New(f.pool, f.service, bytes.Repeat([]byte{4}, 32), nil)
	request := auth.RandomToken()
	o, err := s.Run(f.ctx, a, "liquidacao", "Memória sintética", request, workspaceInput)
	if err != nil {
		t.Fatal(err)
	}
	var result map[string]any
	json.Unmarshal(o.Result, &result)
	if result["total"] != "1020.00" {
		t.Fatal(result)
	}
	persisted, err := s.Get(f.ctx, a, o.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !s.Verify(persisted, s.Integrity(o)) {
		t.Fatal("JSONB normalization changed integrity")
	}
	tampered := persisted
	tampered.Result = json.RawMessage(`{"total":"1.00"}`)
	if s.Verify(tampered, s.Integrity(persisted)) {
		t.Fatal("tampered full result passed integrity")
	}
	var wg sync.WaitGroup
	var failures atomic.Int32
	for i := 0; i < 12; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			again, err := s.Run(f.ctx, a, "liquidacao", "Duplicate", request, workspaceInput)
			if err != nil || again.ID != o.ID {
				failures.Add(1)
			}
		}()
	}
	wg.Wait()
	if failures.Load() != 0 {
		t.Fatal("idempotence failed", failures.Load())
	}
	if _, err := s.Run(f.ctx, a, "liquidacao", "Changed", request, bytes.Replace(workspaceInput, []byte("1000.00"), []byte("2000.00"), 1)); !errors.Is(err, auth.ErrConflict) {
		t.Fatal("changed payload reused idempotency key", err)
	}
	stats, err := s.Stats(f.ctx, a)
	if err != nil || stats.Total != 1 || stats.MonthUsed != 1 {
		t.Fatal(stats, err)
	}
	if _, err = s.Get(f.ctx, b, o.ID); !errors.Is(err, auth.ErrNotFound) {
		t.Fatal("cross-tenant history accessed", err)
	}
	if _, err = s.Get(f.ctx, a, "------------------------------------"); !errors.Is(err, auth.ErrNotFound) {
		t.Fatal("invalid UUID accepted", err)
	}
	list, err := s.List(f.ctx, b, time.Time{})
	if err != nil || len(list) != 0 {
		t.Fatal("history leaked", err)
	}
	restarted := juridico.New(f.pool, f.service, bytes.Repeat([]byte{4}, 32), nil)
	if _, err = restarted.Get(f.ctx, a, o.ID); err != nil {
		t.Fatal("history lost on new service", err)
	}
	if _, err = s.Run(f.ctx, a, "liquidacao", "Bad", auth.RandomToken(), []byte(`{"principal":"1e1000000"}`)); !errors.Is(err, juridico.ErrInput) {
		t.Fatal("bad input accepted", err)
	}
	// Fresh distinct requests contend for the same organization/API quota lock.
	if _, err = f.pool.Exec(f.ctx, "UPDATE organization_limits SET monthly_limit=3 WHERE organization_id=$1", a.OrganizationID); err != nil {
		t.Fatal(err)
	}
	var success atomic.Int32
	failures.Store(0)
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := s.Run(f.ctx, a, "liquidacao", "Concurrent", auth.RandomToken(), workspaceInput)
			if err == nil {
				success.Add(1)
			} else if !errors.Is(err, auth.ErrQuota) {
				failures.Add(1)
			}
		}()
	}
	wg.Wait()
	if success.Load() != 2 || failures.Load() != 0 {
		t.Fatal(success.Load(), failures.Load())
	}
	stats, err = s.Stats(f.ctx, a)
	if err != nil || stats.Total != 3 || stats.MonthUsed != 3 {
		t.Fatal(stats, err)
	}
	if _, err = f.pool.Exec(f.ctx, "UPDATE memberships SET role_code='viewer' WHERE organization_id=$1", a.OrganizationID); err != nil {
		t.Fatal(err)
	}
	if _, err = s.Run(f.ctx, a, "liquidacao", "Stale principal", auth.RandomToken(), workspaceInput); !errors.Is(err, auth.ErrDenied) {
		t.Fatal("revoked permission reused", err)
	}
}

func TestWorkspaceHTTPSFormsUploadsDownloadsAndTenantIsolation(t *testing.T) {
	f := identityDatabase(t)
	owner, _ := f.user(t, "workspace-web", false)
	other, _ := f.user(t, "workspace-other", false)
	srv := httptest.NewUnstartedServer(nil)
	origin := "https://" + srv.Listener.Addr().String()
	identity, err := auth.New(f.pool, bytes.Repeat([]byte{9}, 32), auth.Options{Origin: origin, CookieSecure: true})
	if err != nil {
		t.Fatal(err)
	}
	s := juridico.New(f.pool, identity, bytes.Repeat([]byte{4}, 32), nil)
	foreign, err := s.Run(f.ctx, other, "liquidacao", "Other organization", auth.RandomToken(), workspaceInput)
	if err != nil {
		t.Fatal(err)
	}
	srv.Config.Handler = transport.NewWithWorkspace([32]byte{}, slog.New(slog.NewTextHandler(io.Discard, nil)), identity, s)
	srv.StartTLS()
	t.Cleanup(srv.Close)
	client := srv.Client()
	client.Jar, _ = cookiejar.New(nil)
	client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	get := func(path string) *http.Response {
		res, err := client.Get(origin + path)
		if err != nil {
			t.Fatal(err)
		}
		return res
	}
	post := func(path string, v url.Values) *http.Response {
		req, _ := http.NewRequest("POST", origin+path, strings.NewReader(v.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req.Header.Set("Origin", origin)
		res, err := client.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		return res
	}
	res := get("/app")
	if res.StatusCode != 303 {
		t.Fatal("unauthenticated workspace accessible")
	}
	httpBody(t, res)
	csrf := csrfFrom(t, httpBody(t, get("/login")))
	res = post("/login", url.Values{"csrf": {csrf}, "organization": {owner.Organization}, "email": {owner.Email}, "password": {fixturePassword}})
	if res.StatusCode != 303 {
		t.Fatal("login failed")
	}
	httpBody(t, res)
	for _, path := range []string{"/app", "/app/cnj", "/app/judicial", "/app/financiamento", "/app/prazos", "/app/indices", "/app/history"} {
		res = get(path)
		body := httpBody(t, res)
		if res.StatusCode != 200 || !strings.Contains(body, "Painel jurídico") {
			t.Fatal("workspace page failed", path, res.StatusCode)
		}
	}
	csrf = csrfFrom(t, httpBody(t, get("/app/liquidacao")))
	v := url.Values{"csrf": {csrf}, "request_id": {auth.RandomToken()}, "title": {"<script>alert(1)</script>"}, "principal": {"1000.00"}, "fator_correcao": {"1"}, "taxa_mensal": {"1"}, "meses": {"2"}, "pagamento": {"0"}, "multa_percentual": {"0"}, "honorarios_percentual": {"0"}}
	v.Set("csrf", "forged")
	res = post("/app/liquidacao", v)
	if res.StatusCode != 403 {
		t.Fatal("CSRF bypass")
	}
	httpBody(t, res)
	v.Set("csrf", csrf)
	res = post("/app/liquidacao", v)
	if res.StatusCode != 303 {
		t.Fatal("calculation failed", res.StatusCode, httpBody(t, res))
	}
	location := res.Header.Get("Location")
	httpBody(t, res)
	res = get(location)
	body := httpBody(t, res)
	if res.StatusCode != 200 || !strings.Contains(body, "1020.00") || strings.Contains(body, "<script>alert(1)</script>") {
		t.Fatal("wrong result or XSS")
	}
	for _, format := range []string{"json", "csv", "html"} {
		res = get(location + "/download/" + format)
		if res.StatusCode != 200 || !strings.Contains(res.Header.Get("Content-Disposition"), "attachment") {
			t.Fatal("download", format, res.StatusCode)
		}
		httpBody(t, res)
	}
	res = get(location + "/recalculate")
	body = httpBody(t, res)
	if res.StatusCode != 200 || !strings.Contains(body, `value="2"`) {
		t.Fatal("recalculation lost input")
	}
	for _, suffix := range []string{"", "/download/json", "/download/csv", "/download/html", "/recalculate"} {
		res = get("/app/history/" + foreign.ID + suffix)
		if res.StatusCode != 404 {
			t.Fatal("IDOR", suffix, res.StatusCode)
		}
		httpBody(t, res)
	}
	var upload bytes.Buffer
	writer := multipart.NewWriter(&upload)
	for k, value := range map[string]string{"csrf": csrf, "request_id": auth.RandomToken(), "title": "CSV synthetic", "data_atualizacao": "2024-02-01", "taxa_mensal": "1", "juros_tipo": "CONVENCIONAL_MENSAL", "capitalizacao": "SIMPLES"} {
		writer.WriteField(k, value)
	}
	file, _ := writer.CreateFormFile("file", "parcelas.csv")
	file.Write([]byte("descricao;data_vencimento;valor_original;tipo\nSynthetic;2024-01-01;1000.00;DEBITO\n"))
	writer.Close()
	req, _ := http.NewRequest("POST", origin+"/app/judicial/import", &upload)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	req.Header.Set("Origin", origin)
	res, err = client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	if res.StatusCode != 303 {
		t.Fatal("import failed", res.StatusCode, httpBody(t, res))
	}
	location = res.Header.Get("Location")
	httpBody(t, res)
	importLocation := location
	res = get(importLocation + "/recalculate")
	recalculateBody := httpBody(t, res)
	if !strings.Contains(recalculateBody, `name="source_id"`) {
		t.Fatal("immutable source missing")
	}
	sourceID := strings.TrimPrefix(importLocation, "/app/history/")
	recalc := url.Values{"csrf": {csrf}, "request_id": {auth.RandomToken()}, "title": {"Recalculated import"}, "source_id": {sourceID}, "data_atualizacao": {"2024-03-01"}, "taxa_mensal": {"1"}, "juros_tipo": {"CONVENCIONAL_MENSAL"}, "capitalizacao": {"SIMPLES"}}
	res = post("/app/judicial", recalc)
	if res.StatusCode != 303 {
		t.Fatal("immutable-source recalc", res.StatusCode, httpBody(t, res))
	}
	httpBody(t, res)
	recalc.Set("source_id", foreign.ID)
	recalc.Set("request_id", auth.RandomToken())
	res = post("/app/judicial", recalc)
	if res.StatusCode != 404 {
		t.Fatal("cross-tenant recalc source accessible", res.StatusCode)
	}
	httpBody(t, res)
	res = get(location + "/download/xlsx")
	if res.StatusCode != 200 {
		t.Fatal("XLSX failed")
	}
	data, err := io.ReadAll(res.Body)
	res.Body.Close()
	if err != nil || !bytes.HasPrefix(data, []byte("PK")) {
		t.Fatal("invalid XLSX")
	}
	if _, err = f.pool.Exec(f.ctx, "UPDATE memberships SET role_code='viewer' WHERE organization_id=$1 AND user_id=$2", owner.OrganizationID, owner.UserID); err != nil {
		t.Fatal(err)
	}
	res = get("/app")
	if res.StatusCode != 403 {
		t.Fatal("viewer accessed workspace")
	}
	httpBody(t, res)
}
