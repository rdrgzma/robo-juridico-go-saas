package config

import (
	"encoding/hex"
	"fmt"
	"net"
	"net/url"
	"os"
	"strings"
)

type Config struct {
	Address      string
	APIKeyHash   [32]byte
	Environment  string
	DatabaseURL  string
	AuthKey      []byte
	PublicOrigin string
	CookieSecure bool
	DisableMFA   bool
}

func Load() (Config, error) {
	var c Config
	c.Environment = os.Getenv("APP_ENV")
	if c.Environment != "development" && c.Environment != "test" && c.Environment != "production" {
		return c, fmt.Errorf("APP_ENV obrigatório: development, test ou production")
	}
	if c.Environment != "development" {
		return c, fmt.Errorf("servidor habilitado apenas em development; test é exclusivo da suíte e production exige homologação operacional")
	}
	c.DatabaseURL = os.Getenv("DATABASE_URL")
	if c.DatabaseURL == "" {
		return c, fmt.Errorf("DATABASE_URL obrigatória")
	}
	c.Address = os.Getenv("HTTP_ADDR")
	if c.Address == "" {
		c.Address = "0.0.0.0:8080"
	}
	host, _, err := net.SplitHostPort(c.Address)
	if err != nil || net.ParseIP(host) == nil {
		return c, fmt.Errorf("HTTP_ADDR inválido")
	}
	b, err := hex.DecodeString(os.Getenv("API_KEY_SHA256"))
	if err != nil || len(b) != 32 {
		return c, fmt.Errorf("configure API_KEY_SHA256 com SHA-256 de chave aleatória (go run ./cmd/keygen)")
	}
	copy(c.APIKeyHash[:], b)
	c.AuthKey, err = hex.DecodeString(os.Getenv("AUTH_ENCRYPTION_KEY"))
	if err != nil || len(c.AuthKey) != 32 {
		return c, fmt.Errorf("AUTH_ENCRYPTION_KEY obrigatória: 32 bytes em hexadecimal")
	}
	c.PublicOrigin = os.Getenv("PUBLIC_ORIGIN")
	origin, err := url.Parse(c.PublicOrigin)
	if err != nil || origin.Host == "" || (origin.Scheme != "http" && origin.Scheme != "https") || (origin.Hostname() != "localhost" && origin.Hostname() != "127.0.0.1" && origin.Hostname() != "::1") {
		return c, fmt.Errorf("PUBLIC_ORIGIN local explícita obrigatória")
	}
	c.CookieSecure = origin.Scheme == "https"
	switch os.Getenv("AUTH_DISABLE_MFA") {
	case "", "false":
	case "true":
		dbURL, err := url.Parse(c.DatabaseURL)
		if err != nil || dbURL.User == nil || dbURL.User.Username() != "rj_app" || strings.TrimPrefix(dbURL.Path, "/") != "rj_saas_dev" {
			return c, fmt.Errorf("AUTH_DISABLE_MFA permitido apenas no banco rj_saas_dev com role rj_app")
		}
		c.DisableMFA = true
	default:
		return c, fmt.Errorf("AUTH_DISABLE_MFA inválido: use true ou false")
	}
	return c, nil
}
