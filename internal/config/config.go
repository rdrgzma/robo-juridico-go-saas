package config

import (
	"encoding/hex"
	"fmt"
	"net"
	"os"
)

type Config struct {
	Address    string
	APIKeyHash [32]byte
}

func Load() (Config, error) {
	var c Config
	c.Address = os.Getenv("HTTP_ADDR")
	if c.Address == "" {
		c.Address = "127.0.0.1:8080"
	}
	host, _, err := net.SplitHostPort(c.Address)
	if err != nil || net.ParseIP(host) == nil || !net.ParseIP(host).IsLoopback() {
		return c, fmt.Errorf("Sprint 1 aceita somente endereço IP de loopback; deploy público bloqueado até autenticação multiempresa")
	}
	b, err := hex.DecodeString(os.Getenv("API_KEY_SHA256"))
	if err != nil || len(b) != 32 {
		return c, fmt.Errorf("configure API_KEY_SHA256 com SHA-256 de chave aleatória (go run ./cmd/keygen)")
	}
	copy(c.APIKeyHash[:], b)
	return c, nil
}
