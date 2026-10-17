package main

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/jackc/pgx/v5"
	"github.com/rdrgzma/robo-juridico-go-saas/internal/storage"
	"github.com/rdrgzma/robo-juridico-go-saas/migrations"
	"os"
	"time"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
func run() error {
	if len(os.Args) != 2 || (os.Args[1] != "up" && os.Args[1] != "status") {
		return fmt.Errorf("uso: migrate up|status")
	}
	url := os.Getenv("DATABASE_URL")
	if os.Getenv("APP_ENV") == "test" {
		url = os.Getenv("TEST_DATABASE_URL")
		cfg, err := pgx.ParseConfig(url)
		if err != nil || cfg.Database != "rj_saas_test" || cfg.User != "rj_test" {
			return fmt.Errorf("test exige banco rj_saas_test e role rj_test")
		}
	}
	if url == "" {
		return fmt.Errorf("configure DATABASE_URL (TEST_DATABASE_URL em test)")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	conn, err := pgx.Connect(ctx, url)
	if err != nil {
		return fmt.Errorf("não foi possível conectar ao banco de migrations")
	}
	defer conn.Close(context.Background())
	status, err := storage.Migrate(ctx, conn, migrations.Files, os.Args[1] == "up")
	if err != nil {
		return err
	}
	return json.NewEncoder(os.Stdout).Encode(status)
}
