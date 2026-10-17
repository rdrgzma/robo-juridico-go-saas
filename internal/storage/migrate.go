package storage

import (
	"context"
	"crypto/sha256"
	"fmt"
	"github.com/jackc/pgx/v5"
	"io/fs"
	"sort"
	"strings"
)

type MigrationStatus struct {
	Name    string `json:"name"`
	Applied bool   `json:"applied"`
}

// Migrate serializes runners. SQL and checksum are committed in the same transaction.
func Migrate(ctx context.Context, conn *pgx.Conn, files fs.FS, apply bool) ([]MigrationStatus, error) {
	if _, err := conn.Exec(ctx, "SELECT pg_advisory_lock(726574863019)"); err != nil {
		return nil, err
	}
	defer func() { _, _ = conn.Exec(context.Background(), "SELECT pg_advisory_unlock(726574863019)") }()
	if _, err := conn.Exec(ctx, `CREATE TABLE IF NOT EXISTS schema_migrations (name text PRIMARY KEY, checksum text NOT NULL, applied_at timestamptz NOT NULL DEFAULT now())`); err != nil {
		return nil, err
	}
	names, err := fs.Glob(files, "*.sql")
	if err != nil {
		return nil, err
	}
	sort.Strings(names)
	known := map[string]string{}
	for _, name := range names {
		b, err := fs.ReadFile(files, name)
		if err != nil {
			return nil, err
		}
		known[name] = fmt.Sprintf("%x", sha256.Sum256(b))
	}
	rows, err := conn.Query(ctx, "SELECT name,checksum FROM schema_migrations")
	if err != nil {
		return nil, err
	}
	applied := map[string]bool{}
	for rows.Next() {
		var name, checksum string
		if err := rows.Scan(&name, &checksum); err != nil {
			rows.Close()
			return nil, err
		}
		if known[name] != checksum {
			rows.Close()
			return nil, fmt.Errorf("migration alterada ou ausente: %s", name)
		}
		applied[name] = true
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	status := make([]MigrationStatus, 0, len(names))
	for _, name := range names {
		if !applied[name] && apply {
			b, _ := fs.ReadFile(files, name)
			if strings.TrimSpace(string(b)) == "" {
				return nil, fmt.Errorf("migration vazia: %s", name)
			}
			tx, err := conn.Begin(ctx)
			if err != nil {
				return nil, err
			}
			if _, err = tx.Exec(ctx, string(b)); err == nil {
				_, err = tx.Exec(ctx, "INSERT INTO schema_migrations(name,checksum) VALUES($1,$2)", name, known[name])
			}
			if err != nil {
				_ = tx.Rollback(context.Background())
				return nil, fmt.Errorf("migration %s falhou: %w", name, err)
			}
			if err = tx.Commit(ctx); err != nil {
				return nil, err
			}
			applied[name] = true
		}
		status = append(status, MigrationStatus{name, applied[name]})
	}
	return status, nil
}
