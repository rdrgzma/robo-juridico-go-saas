package config

import (
	"strings"
	"testing"
)

func TestFailClosedConfiguration(t *testing.T) {
	t.Setenv("HTTP_ADDR", "0.0.0.0:8080")
	t.Setenv("API_KEY_SHA256", strings.Repeat("a", 64))
	if _, err := Load(); err == nil {
		t.Fatal("public listener accepted")
	}
	t.Setenv("HTTP_ADDR", "127.0.0.1:8080")
	t.Setenv("API_KEY_SHA256", "")
	if _, err := Load(); err == nil {
		t.Fatal("missing key accepted")
	}
	t.Setenv("API_KEY_SHA256", strings.Repeat("a", 64))
	if _, err := Load(); err != nil {
		t.Fatal(err)
	}
}
