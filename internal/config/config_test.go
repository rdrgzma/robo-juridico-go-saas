package config

import (
	"strings"
	"testing"
)

func TestFailClosedConfiguration(t *testing.T) {
	t.Setenv("HTTP_ADDR", "0.0.0.0:8080")
	t.Setenv("APP_ENV", "production")
	t.Setenv("DATABASE_URL", "postgres://db/test")
	t.Setenv("AUTH_ENCRYPTION_KEY", strings.Repeat("b", 64))
	t.Setenv("PUBLIC_ORIGIN", "http://localhost:8081")
	t.Setenv("API_KEY_SHA256", strings.Repeat("a", 64))
	t.Setenv("AUTH_DISABLE_MFA", "")
	if _, err := Load(); err == nil {
		t.Fatal("public listener accepted")
	}
	t.Setenv("HTTP_ADDR", "127.0.0.1:8080")
	t.Setenv("APP_ENV", "development")
	t.Setenv("API_KEY_SHA256", "")
	if _, err := Load(); err == nil {
		t.Fatal("missing key accepted")
	}
	t.Setenv("API_KEY_SHA256", strings.Repeat("a", 64))
	if _, err := Load(); err != nil {
		t.Fatal(err)
	}
	t.Setenv("AUTH_DISABLE_MFA", "true")
	if _, err := Load(); err == nil {
		t.Fatal("MFA bypass accepted outside rj_saas_dev/rj_app")
	}
	t.Setenv("DATABASE_URL", "postgres://rj_app:secret@db:5432/rj_saas_dev?sslmode=disable")
	t.Setenv("AUTH_DISABLE_MFA", "invalid")
	if _, err := Load(); err == nil {
		t.Fatal("invalid AUTH_DISABLE_MFA accepted")
	}
	t.Setenv("AUTH_DISABLE_MFA", "true")
	cfg, err := Load()
	if err != nil || !cfg.DisableMFA {
		t.Fatal("expected DisableMFA in local dev config", err)
	}
	t.Setenv("AUTH_DISABLE_MFA", "false")
	cfg, err = Load()
	if err != nil || cfg.DisableMFA {
		t.Fatal("expected DisableMFA=false", err)
	}
}
