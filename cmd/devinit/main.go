// Devinit creates local development secrets without requiring Go on the host.
package main

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
)

func token() string {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return hex.EncodeToString(b)
}
func main() {
	if env := os.Getenv("APP_ENV"); env != "" && env != "development" {
		fmt.Fprintln(os.Stderr, "devinit permitido apenas em development")
		os.Exit(1)
	}
	for _, p := range []string{".env", ".dev-api-key"} {
		if _, err := os.Stat(p); !os.IsNotExist(err) {
			fmt.Fprintln(os.Stderr, "Inicialização recusada: arquivo local já existe; não sobrescrever segredos/volumes")
			os.Exit(1)
		}
	}
	key := "rj_" + token()
	content := fmt.Sprintf("# Exclusivo de desenvolvimento; não versionar.\nPOSTGRES_PASSWORD=%s\nAPP_DB_PASSWORD=%s\nTEST_DB_PASSWORD=%s\nAPI_KEY_SHA256=%x\n", token(), token(), token(), sha256.Sum256([]byte(key)))
	if err := os.WriteFile(".dev-api-key", []byte(key+"\n"), 0600); err != nil {
		panic(err)
	}
	f, err := os.OpenFile(".env", os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		panic(err)
	}
	if _, err = f.WriteString(content); err != nil {
		panic(err)
	}
	if err = f.Close(); err != nil {
		panic(err)
	}
	fmt.Println(".env e .dev-api-key gerados. Segredos não foram exibidos. Execute docker compose up -d --build.")
}
