package auth

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDevelopmentLoginLoaderFailsClosed(t *testing.T) {
	path := filepath.Join(t.TempDir(), "accounts.json")
	if err := os.WriteFile(path, []byte(`{"owner":{"password":"should-not-load"}}`), 0600); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct{ environment, url string }{
		{"production", "postgres://rj_app:unused@localhost/rj_saas_dev"},
		{"development", "postgres://rj_app:unused@localhost/production"},
		{"development", "postgres://rj_test:unused@localhost/rj_saas_dev"},
	} {
		accounts, err := LoadDevelopmentLogins(tc.environment, tc.url, path)
		if err != nil || len(accounts) != 0 {
			t.Fatal("loaded outside development", err)
		}
	}
	if _, err := LoadDevelopmentLogins("development", "postgres://rj_app:unused@localhost/rj_saas_dev", path); err == nil {
		t.Fatal("accepted incomplete file")
	}
}
