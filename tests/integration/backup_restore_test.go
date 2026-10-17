package integration

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/rdrgzma/robo-juridico-go-saas/internal/storage"
	"github.com/rdrgzma/robo-juridico-go-saas/migrations"
)

func TestPostgreSQLBackupAndRestoreIntegrity(t *testing.T) {
	ctx := context.Background()
	cfg, err := pgx.ParseConfig(os.Getenv("TEST_DATABASE_URL"))
	if err != nil {
		t.Fatal(err)
	}

	srcSchema := fmt.Sprintf("src_backup_%d", time.Now().UnixNano())
	dstSchema := fmt.Sprintf("dst_restore_%d", time.Now().UnixNano())

	conn, err := pgx.ConnectConfig(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close(ctx)

	// Create and isolate source schema
	if _, err := conn.Exec(ctx, "CREATE SCHEMA "+srcSchema); err != nil {
		t.Fatal(err)
	}
	defer func() {
		_, _ = conn.Exec(context.Background(), "DROP SCHEMA IF EXISTS "+srcSchema+" CASCADE")
		_, _ = conn.Exec(context.Background(), "DROP SCHEMA IF EXISTS "+dstSchema+" CASCADE")
	}()

	if _, err := conn.Exec(ctx, "SET search_path TO "+srcSchema); err != nil {
		t.Fatal(err)
	}

	// Apply migrations on source schema
	status, err := storage.Migrate(ctx, conn, migrations.Files, true)
	if err != nil || len(status) != 5 {
		t.Fatalf("failed applying migrations to source schema: %v", err)
	}

	// Seed source schema with representative entities
	var orgID, userID string
	err = conn.QueryRow(ctx, `INSERT INTO organizations(slug, name, legal_name, document)
		VALUES('backup-test-org', 'Escritório Backup Teste', 'Sociedade Teste LTDA', '12.345.678/0001-90')
		RETURNING id::text`).Scan(&orgID)
	if err != nil {
		t.Fatal(err)
	}

	err = conn.QueryRow(ctx, `INSERT INTO users(email, password_hash)
		VALUES('admin@backuptest.org', '$argon2id$v=19$m=65536,t=3,p=2$synthetichash$synthetichash')
		RETURNING id::text`).Scan(&userID)
	if err != nil {
		t.Fatal(err)
	}

	if _, err = conn.Exec(ctx, `INSERT INTO memberships(organization_id, user_id, role_code)
		VALUES($1, $2, 'owner')`, orgID, userID); err != nil {
		t.Fatal(err)
	}

	if _, err = conn.Exec(ctx, `INSERT INTO legal_operations(organization_id, user_id, idempotency_key, kind, title, input, result, engine_version)
		VALUES($1, $2, 'idempotency-key-for-backup-test-12345678901', 'liquidacao', 'Memória de Cálculo 100', '{"principal":"1000.00"}'::jsonb, '{"total":"1020.00"}'::jsonb, '1.0.0')`, orgID, userID); err != nil {
		t.Fatal(err)
	}

	// Count source rows
	var srcOrgs, srcUsers, srcMemberships, srcOperations int
	_ = conn.QueryRow(ctx, "SELECT count(*) FROM organizations").Scan(&srcOrgs)
	_ = conn.QueryRow(ctx, "SELECT count(*) FROM users").Scan(&srcUsers)
	_ = conn.QueryRow(ctx, "SELECT count(*) FROM memberships").Scan(&srcMemberships)
	_ = conn.QueryRow(ctx, "SELECT count(*) FROM legal_operations").Scan(&srcOperations)

	if srcOrgs != 1 || srcUsers != 1 || srcMemberships != 1 || srcOperations != 1 {
		t.Fatalf("unexpected seed counts: orgs=%d users=%d memberships=%d ops=%d", srcOrgs, srcUsers, srcMemberships, srcOperations)
	}

	// Create and initialize destination schema with fresh migration
	if _, err := conn.Exec(ctx, "CREATE SCHEMA "+dstSchema); err != nil {
		t.Fatal(err)
	}
	if _, err := conn.Exec(ctx, "SET search_path TO "+dstSchema); err != nil {
		t.Fatal(err)
	}
	if _, err := storage.Migrate(ctx, conn, migrations.Files, true); err != nil {
		t.Fatal(err)
	}

	// Simulate data transfer (logical restore of table records preserving IDs and timestamps)
	tables := []string{"organizations", "users", "memberships", "legal_operations"}
	for _, tbl := range tables {
		query := fmt.Sprintf("INSERT INTO %s.%s SELECT * FROM %s.%s", dstSchema, tbl, srcSchema, tbl)
		if _, err := conn.Exec(ctx, query); err != nil {
			t.Fatalf("failed restoring table %s: %v", tbl, err)
		}
	}

	// Verify restored destination schema integrity
	var dstOrgs, dstUsers, dstMemberships, dstOperations int
	_ = conn.QueryRow(ctx, fmt.Sprintf("SELECT count(*) FROM %s.organizations", dstSchema)).Scan(&dstOrgs)
	_ = conn.QueryRow(ctx, fmt.Sprintf("SELECT count(*) FROM %s.users", dstSchema)).Scan(&dstUsers)
	_ = conn.QueryRow(ctx, fmt.Sprintf("SELECT count(*) FROM %s.memberships", dstSchema)).Scan(&dstMemberships)
	_ = conn.QueryRow(ctx, fmt.Sprintf("SELECT count(*) FROM %s.legal_operations", dstSchema)).Scan(&dstOperations)

	if dstOrgs != srcOrgs || dstUsers != srcUsers || dstMemberships != srcMemberships || dstOperations != srcOperations {
		t.Fatalf("restore count mismatch: src(%d,%d,%d,%d) vs dst(%d,%d,%d,%d)",
			srcOrgs, srcUsers, srcMemberships, srcOperations,
			dstOrgs, dstUsers, dstMemberships, dstOperations)
	}

	// Verify content integrity of restored calculation operation
	var restoredTitle, restoredTotal string
	err = conn.QueryRow(ctx, fmt.Sprintf("SELECT title, result->>'total' FROM %s.legal_operations WHERE organization_id=$1", dstSchema), orgID).Scan(&restoredTitle, &restoredTotal)
	if err != nil || restoredTitle != "Memória de Cálculo 100" || restoredTotal != "1020.00" {
		t.Fatalf("restored calculation data corrupted: title=%s total=%s err=%v", restoredTitle, restoredTotal, err)
	}
}
