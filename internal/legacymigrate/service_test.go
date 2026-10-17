package legacymigrate_test

import (
	"encoding/binary"
	"os"
	"path/filepath"
	"testing"

	"github.com/rdrgzma/robo-juridico-go-saas/internal/legacymigrate"
)

func createSyntheticSQLiteHeader(t *testing.T, dir string, valid bool) string {
	t.Helper()
	buf := make([]byte, 100)
	if valid {
		copy(buf[:16], []byte("SQLite format 3\x00"))
		binary.BigEndian.PutUint16(buf[16:18], 4096) // page size
		binary.BigEndian.PutUint32(buf[28:32], 12)   // page count
		binary.BigEndian.PutUint32(buf[40:44], 4)    // schema version
		binary.BigEndian.PutUint32(buf[60:64], 1)    // user version
	} else {
		copy(buf[:16], []byte("Not a valid DB!\x00"))
	}

	tmpFile := filepath.Join(dir, "synthetic_test.db")
	if err := os.WriteFile(tmpFile, buf, 0o600); err != nil {
		t.Fatal(err)
	}
	return tmpFile
}

func TestInspectSnapshotValidAndInvalidHeaders(t *testing.T) {
	tmpDir := t.TempDir()

	validDB := createSyntheticSQLiteHeader(t, tmpDir, true)
	meta, err := legacymigrate.InspectSnapshot(validDB)
	if err != nil {
		t.Fatalf("expected valid header inspection, got error: %v", err)
	}
	if meta.PageSize != 4096 || meta.PageCount != 12 || meta.SchemaVersion != 4 || meta.UserVersion != 1 {
		t.Fatalf("unexpected metadata extracted: %+v", meta)
	}
	if !meta.ReadOnlyMode || meta.SHA256 == "" {
		t.Fatalf("expected read-only flag and valid SHA-256, got %+v", meta)
	}

	invalidDB := createSyntheticSQLiteHeader(t, tmpDir, false)
	if _, err := legacymigrate.InspectSnapshot(invalidDB); err != legacymigrate.ErrNotSQLite {
		t.Fatalf("expected ErrNotSQLite for invalid file, got %v", err)
	}
}

func TestAnalyzeDumpScriptMapping(t *testing.T) {
	syntheticDump := `
CREATE TABLE users (id integer primary key, name text, email text, password text);
CREATE TABLE clientes (id integer primary key, razao_social text, cnpj text);
CREATE TABLE tabela_indices (id integer primary key, nome text, data text, valor real);
CREATE TABLE calculos (id integer primary key, titulo text, payload text);
`
	report := legacymigrate.AnalyzeDumpScript(syntheticDump)
	if len(report.Mappings) < 4 {
		t.Fatalf("expected at least 4 mappings detected, got %d", len(report.Mappings))
	}
	if !report.DryRunPassed {
		t.Fatalf("expected dry run passed")
	}
	if len(report.Warnings) == 0 {
		t.Fatalf("expected security warnings recorded")
	}
}
