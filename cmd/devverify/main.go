// Devverify exercises the running development server, including bind-mounted reload.
package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

var client = &http.Client{Timeout: 3 * time.Second}

func get(path string) ([]byte, error) {
	r, err := client.Get("http://127.0.0.1:8080" + path)
	if err != nil {
		return nil, err
	}
	defer r.Body.Close()
	b, err := io.ReadAll(r.Body)
	if r.StatusCode != 200 {
		return nil, fmt.Errorf("HTTP %d em %s", r.StatusCode, path)
	}
	return b, err
}
func wait(check func() bool) error {
	deadline := time.Now().Add(50 * time.Second)
	for time.Now().Before(deadline) {
		if check() {
			return nil
		}
		time.Sleep(300 * time.Millisecond)
	}
	return fmt.Errorf("verificação excedeu prazo")
}
func generation() string {
	b, err := get("/health/live")
	if err != nil {
		return ""
	}
	var result map[string]string
	_ = json.Unmarshal(b, &result)
	return result["started_at"]
}
func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
func run() error {
	if os.Getenv("APP_ENV") != "development" {
		return fmt.Errorf("somente development")
	}
	if err := wait(func() bool { _, err := get("/health/ready"); return err == nil }); err != nil {
		return err
	}
	for _, path := range []string{"/", "/docs", "/health/live", "/health/ready", "/openapi.json"} {
		if _, err := get(path); err != nil {
			return err
		}
	}
	key, err := os.ReadFile(".dev-api-key")
	if err != nil {
		return err
	}
	req, _ := http.NewRequest("POST", "http://127.0.0.1:8080/api/v1/calculos/judicial", strings.NewReader(`{"data_atualizacao":"2026-03-01","indexador_codigo":"SEM_CORRECAO","juros_mora":{"tipo":"CONVENCIONAL_MENSAL","percentual_mensal":"1"},"parcelas":[{"data_vencimento":"2026-01-01","valor_original":"1000"}]}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-API-Key", strings.TrimSpace(string(key)))
	response, err := client.Do(req)
	if err != nil {
		return err
	}
	b, err := io.ReadAll(response.Body)
	_ = response.Body.Close()
	if err != nil || response.StatusCode != 200 || !strings.Contains(string(b), `"total_geral":"1020"`) {
		return fmt.Errorf("smoke da liquidação falhou (status %d)", response.StatusCode)
	}
	for _, path := range []string{"cmd/server/main.go", "web/templates/public/index.html", "web/static/css/style.css"} {
		original, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		before := generation()
		if before == "" {
			if err := wait(func() bool { return generation() != "" }); err != nil {
				return err
			}
			before = generation()
		}
		marker := "SPRINT1_RELOAD_PROOF"
		addition := "\n// " + marker + "\n"
		if strings.HasSuffix(path, "html") {
			addition = "\n<span hidden>" + marker + "</span>\n"
		}
		if strings.HasSuffix(path, "css") {
			addition = "\n/* " + marker + " */\n"
		}
		if err := func() error {
			defer func() { _ = os.WriteFile(path, original, 0644) }()
			if err := os.WriteFile(path, append(append([]byte(nil), original...), []byte(addition)...), 0644); err != nil {
				return err
			}
			return wait(func() bool {
				after := generation()
				if after == "" || after == before {
					return false
				}
				if strings.HasSuffix(path, "html") {
					b, err := get("/")
					return err == nil && strings.Contains(string(b), marker)
				}
				if strings.HasSuffix(path, "css") {
					b, err := get("/static/style.css")
					return err == nil && strings.Contains(string(b), marker)
				}
				return true
			})
		}(); err != nil {
			return fmt.Errorf("reload %s: %w", path, err)
		}
		if err := wait(func() bool {
			b, err := get("/health/ready")
			if err != nil || !strings.Contains(string(b), "ready") {
				return false
			}
			if strings.HasSuffix(path, "html") {
				b, _ = get("/")
				return !strings.Contains(string(b), marker)
			}
			if strings.HasSuffix(path, "css") {
				b, _ = get("/static/style.css")
				return !strings.Contains(string(b), marker)
			}
			return true
		}); err != nil {
			return err
		}
		fmt.Println("Hot reload confirmado:", path)
	}
	fmt.Println("Smoke HTTP, liquidação 1020.00, hot reload Go/HTML/CSS e restauração dos arquivos: OK")
	return nil
}
