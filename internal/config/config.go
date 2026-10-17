package config

import (
	"encoding/hex"
	"fmt"
	"net"
	"os"
)

type Config struct {
	Address     string
	APIKeyHash  [32]byte
	Environment string
	DatabaseURL string
}

func Load() (Config, error) {
	var c Config
	c.Environment = os.Getenv("APP_ENV")
	if c.Environment != "development" && c.Environment != "test" && c.Environment != "production" {
		return c, fmt.Errorf("APP_ENV obrigatório: development, test ou production")
	}
	if c.Environment != "development" {
		return c, fmt.Errorf("servidor habilitado apenas em development até autenticação multiempresa; test é exclusivo da suíte e production exige homologação")
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
	return c, nil
}
