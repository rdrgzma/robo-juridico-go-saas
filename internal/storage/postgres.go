package storage

import (
	"context"
	"crypto/sha256"
	"fmt"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rdrgzma/robo-juridico-go-saas/migrations"
	"strings"
	"time"
)

func Connect(ctx context.Context, url string) (*pgxpool.Pool, error) {
	cfg, err := pgxpool.ParseConfig(url)
	if err != nil {
		return nil, fmt.Errorf("DATABASE_URL inválida")
	}
	cfg.MaxConns = 10
	cfg.MinConns = 1
	cfg.ConnConfig.ConnectTimeout = 5 * time.Second
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("configuração de pool inválida")
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("PostgreSQL indisponível")
	}
	return pool, nil
}

// Ready verifies connectivity and applied checksums without changing the database.
func Ready(ctx context.Context, pool *pgxpool.Pool) error {
	if err := pool.Ping(ctx); err != nil {
		return fmt.Errorf("PostgreSQL indisponível")
	}
	entries, _ := migrations.Files.ReadDir(".")
	for _, entry := range entries {
		if !strings.HasSuffix(entry.Name(), ".sql") {
			continue
		}
		b, _ := migrations.Files.ReadFile(entry.Name())
		var checksum string
		if err := pool.QueryRow(ctx, "SELECT checksum FROM schema_migrations WHERE name=$1", entry.Name()).Scan(&checksum); err != nil || checksum != fmt.Sprintf("%x", sha256.Sum256(b)) {
			return fmt.Errorf("migrations pendentes ou alteradas")
		}
	}
	return nil
}
