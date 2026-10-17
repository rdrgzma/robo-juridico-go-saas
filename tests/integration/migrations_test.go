package integration

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rdrgzma/robo-juridico-go-saas/internal/storage"
	"github.com/rdrgzma/robo-juridico-go-saas/internal/transport"
	"github.com/rdrgzma/robo-juridico-go-saas/migrations"
	"io"
	"log/slog"
	"net/http/httptest"
	"os"
	"testing"
	"testing/fstest"
	"time"
)

func testDatabase(t *testing.T) (context.Context, *pgx.Conn, string) {
	t.Helper()
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("TEST_DATABASE_URL ausente; execute a suíte pelo Docker Compose")
	}
	cfg, err := pgx.ParseConfig(url)
	if err != nil || cfg.Database != "rj_saas_test" || cfg.User != "rj_test" {
		t.Fatal("testes recusados: banco/role devem ser rj_saas_test/rj_test")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)
	conn, err := pgx.ConnectConfig(ctx, cfg)
	if err != nil {
		t.Fatal("banco de testes indisponível")
	}
	t.Cleanup(func() { _ = conn.Close(context.Background()) })
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	schema := "rj_test_" + hex.EncodeToString(b)
	quoted := pgx.Identifier{schema}.Sanitize()
	if _, err := conn.Exec(ctx, "CREATE SCHEMA "+quoted); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = conn.Exec(context.Background(), "DROP SCHEMA "+quoted+" CASCADE") })
	if _, err := conn.Exec(ctx, "SET search_path TO "+quoted); err != nil {
		t.Fatal(err)
	}
	return ctx, conn, schema
}

func TestMigrationsOnEmptyDatabaseAndIdempotency(t *testing.T) {
	ctx, conn, _ := testDatabase(t)
	for i := 0; i < 2; i++ {
		status, err := storage.Migrate(ctx, conn, migrations.Files, true)
		if err != nil || len(status) != 4 || !status[0].Applied || !status[1].Applied || !status[2].Applied || !status[3].Applied {
			t.Fatalf("migration: %+v %v", status, err)
		}
	}
	var count int
	if err := conn.QueryRow(ctx, "SELECT count(*) FROM schema_migrations").Scan(&count); err != nil || count != 4 {
		t.Fatal(count, err)
	}
	if _, err := conn.Exec(ctx, "INSERT INTO organizations(slug,name) VALUES('fixture',$1)", "Fixture persistente na transação de teste"); err != nil {
		t.Fatal(err)
	}
	if err := conn.QueryRow(ctx, "SELECT count(*) FROM organizations").Scan(&count); err != nil || count != 1 {
		t.Fatal(count, err)
	}
}

func TestFailedMigrationRollsBackAndIsNotMarked(t *testing.T) {
	ctx, conn, _ := testDatabase(t)
	files := fstest.MapFS{"000001_fail.sql": {Data: []byte("CREATE TABLE rollback_probe(id int); SELECT invalid_column FROM rollback_probe;")}}
	if _, err := storage.Migrate(ctx, conn, files, true); err == nil {
		t.Fatal("SQL inválido aceito")
	}
	var exists bool
	if err := conn.QueryRow(ctx, "SELECT to_regclass('rollback_probe') IS NOT NULL").Scan(&exists); err != nil || exists {
		t.Fatal("DDL parcial persistiu", err)
	}
	var count int
	if err := conn.QueryRow(ctx, "SELECT count(*) FROM schema_migrations").Scan(&count); err != nil || count != 0 {
		t.Fatal("falha registrada como sucesso")
	}
}

func TestMigrationTamperIsRejected(t *testing.T) {
	ctx, conn, _ := testDatabase(t)
	files := fstest.MapFS{"000001.sql": {Data: []byte("CREATE TABLE original(id int);")}}
	if _, err := storage.Migrate(ctx, conn, files, true); err != nil {
		t.Fatal(err)
	}
	files["000001.sql"] = &fstest.MapFile{Data: []byte("CREATE TABLE altered(id int);")}
	if _, err := storage.Migrate(ctx, conn, files, true); err == nil {
		t.Fatal("checksum alterado aceito")
	}
}

func TestTestRoleCannotConnectToDevelopmentDatabase(t *testing.T) {
	ctx, _, _ := testDatabase(t)
	cfg, _ := pgx.ParseConfig(os.Getenv("TEST_DATABASE_URL"))
	cfg.Database = "rj_saas_dev"
	conn, err := pgx.ConnectConfig(ctx, cfg)
	if err == nil {
		_ = conn.Close(ctx)
		t.Fatal("role de teste acessou banco principal")
	}
}

func TestConcurrentMigrationRunners(t *testing.T) {
	ctx, first, schema := testDatabase(t)
	cfg, _ := pgx.ParseConfig(os.Getenv("TEST_DATABASE_URL"))
	cfg.RuntimeParams["search_path"] = schema
	second, err := pgx.ConnectConfig(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer second.Close(context.Background())
	results := make(chan error, 2)
	for _, conn := range []*pgx.Conn{first, second} {
		go func(c *pgx.Conn) { _, err := storage.Migrate(ctx, c, migrations.Files, true); results <- err }(conn)
	}
	for i := 0; i < 2; i++ {
		if err := <-results; err != nil {
			t.Fatal(err)
		}
	}
	var count int
	if err := first.QueryRow(ctx, "SELECT count(*) FROM schema_migrations").Scan(&count); err != nil || count != 4 {
		t.Fatal(count, err)
	}
}

func TestRealReadinessRejectsUnavailableDependency(t *testing.T) {
	ctx, _, _ := testDatabase(t)
	cfg, err := pgxpool.ParseConfig(os.Getenv("TEST_DATABASE_URL"))
	if err != nil {
		t.Fatal(err)
	}
	cfg.ConnConfig.Port = 1
	cfg.ConnConfig.ConnectTimeout = time.Second
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	h := transport.New([32]byte{}, slog.New(slog.NewTextHandler(io.Discard, nil)), func(ctx context.Context) error { return storage.Ready(ctx, pool) })
	for _, tc := range []struct {
		path   string
		status int
	}{{"/health/live", 200}, {"/health/ready", 503}} {
		w := httptest.NewRecorder()
		h.ServeHTTP(w, httptest.NewRequest("GET", tc.path, nil))
		if w.Code != tc.status {
			t.Fatal(tc.path, w.Code)
		}
	}
}
