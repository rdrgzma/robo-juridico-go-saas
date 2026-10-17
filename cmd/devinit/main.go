// Devinit creates local development secrets without requiring Go on the host.
package main

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"strings"
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
	if len(os.Args) == 2 && os.Args[1] == "extend" {
		b, err := os.ReadFile(".env")
		if err != nil {
			panic(err)
		}
		for _, line := range strings.Split(string(b), "\n") {
			if strings.HasPrefix(strings.TrimSpace(line), "AUTH_ENCRYPTION_KEY=") && strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(line), "AUTH_ENCRYPTION_KEY=")) != "" {
				fmt.Println("Chave de autenticação existente preservada.")
				return
			}
		}
		f, err := os.OpenFile(".env", os.O_APPEND|os.O_WRONLY, 0600)
		if err != nil {
			panic(err)
		}
		_, err = f.WriteString("\nAUTH_ENCRYPTION_KEY=" + token() + "\n")
		if err != nil {
			panic(err)
		}
		if err = f.Close(); err != nil {
			panic(err)
		}
		fmt.Println("AUTH_ENCRYPTION_KEY adicionada sem alterar os segredos existentes.")
		return
	}
	for _, p := range []string{".env", ".dev-api-key"} {
		if _, err := os.Stat(p); !os.IsNotExist(err) {
			fmt.Fprintln(os.Stderr, "Inicialização recusada: arquivo local já existe; não sobrescrever segredos/volumes")
			os.Exit(1)
		}
	}
	key := "rj_" + token()
	content := fmt.Sprintf("# Exclusivo de desenvolvimento; não versionar.\nPOSTGRES_PASSWORD=%s\nAPP_DB_PASSWORD=%s\nTEST_DB_PASSWORD=%s\nAPI_KEY_SHA256=%x\nAUTH_ENCRYPTION_KEY=%s\n", token(), token(), token(), sha256.Sum256([]byte(key)), token())
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
