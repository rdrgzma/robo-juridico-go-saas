package transport

import (
	"crypto/sha256"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func handler() http.Handler {
	return New(sha256.Sum256([]byte("test-key")), slog.New(slog.NewTextHandler(io.Discard, nil)))
}
func request(h http.Handler, method, path, key, body string) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, path, strings.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set("X-API-Key", key)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w
}

func TestPrefixesAndAuthentication(t *testing.T) {
	h := handler()
	for _, prefix := range []string{"/v1", "/api/v1"} {
		path := prefix + "/cnj/0000001-73.2023.8.26.0100"
		if w := request(h, "GET", path, "", ""); w.Code != 401 {
			t.Fatal(w.Code, w.Body.String())
		}
		if w := request(h, "GET", path, "test-key", ""); w.Code != 200 || !strings.Contains(w.Body.String(), `"valido":true`) {
			t.Fatal(w.Code, w.Body.String())
		}
	}
}

func TestInputBoundaryAndPanelGates(t *testing.T) {
	h := handler()
	for _, body := range []string{`{"unknown":1}`, `{} {}`, `{"principal":"1e99999"}`, strings.Repeat(" ", 65537) + `{}`} {
		w := request(h, "POST", "/v1/simulacoes/liquidacao", "test-key", body)
		if w.Code < 400 || w.Header().Get("X-Request-ID") == "" {
			t.Fatal(w.Code)
		}
	}
	for _, path := range []string{"/app", "/portal", "/admin"} {
		if w := request(h, "GET", path, "", ""); w.Code != 503 {
			t.Fatal(path, w.Code)
		}
	}
	w := request(h, "GET", "/saude", "", "")
	if w.Code != 200 || w.Header().Get("Access-Control-Allow-Origin") != "" {
		t.Fatal(w.Code)
	}
	w = request(h, "GET", "/", "", "")
	if w.Code != 200 || !strings.Contains(w.Body.String(), "Robô Jurídico") {
		t.Fatal(w.Code)
	}
}

func TestGlobalLocalRateLimit(t *testing.T) {
	h := handler()
	for i := 0; i < 60; i++ {
		if w := request(h, "GET", "/v1/cnj/x", "bad", ""); w.Code != 401 {
			t.Fatal(i, w.Code)
		}
	}
	if w := request(h, "GET", "/v1/cnj/x", "bad", ""); w.Code != 429 || w.Header().Get("Retry-After") == "" {
		t.Fatal(w.Code)
	}
}

func TestSimulationPrefixes(t *testing.T) {
	body := `{"principal":"1000","fator_correcao":"1.1","taxa_mensal":"1","meses":2,"compostos":false,"pagamento":"100","imputar_juros_primeiro":true,"multa_percentual":"10","honorarios_percentual":"10"}`
	h := handler()
	a := request(h, "POST", "/v1/simulacoes/liquidacao", "test-key", body)
	b := request(h, "POST", "/api/v1/simulacoes/liquidacao", "test-key", body)
	if a.Code != 200 || b.Code != 200 || a.Body.String() != b.Body.String() || !strings.Contains(a.Body.String(), `"total":"1226.40"`) {
		t.Fatal(a.Code, a.Body.String(), b.Code, b.Body.String())
	}
}
