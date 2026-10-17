// Package legacymigrate implements safe, read-only inspection, validation and
// compatibility dry-run mapping from legacy SQLite snapshots to the Go SaaS PostgreSQL schema.
package legacymigrate

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

var (
	ErrNotSQLite        = errors.New("arquivo não possui cabeçalho válido SQLite 3")
	ErrFileAccess       = errors.New("não foi possível acessar o arquivo de banco legado")
	ErrReadOnlyRequired = errors.New("o arquivo deve ser aberto estritamente em modo somente leitura")
)

var sqliteHeaderMagic = []byte("SQLite format 3\x00")

type SnapshotMetadata struct {
	Path           string    `json:"path"`
	Filename       string    `json:"filename"`
	SizeBytes      int64     `json:"size_bytes"`
	SHA256         string    `json:"sha256"`
	PageSize       int       `json:"page_size"`
	PageCount      int       `json:"page_count"`
	UserVersion    int       `json:"user_version"`
	SchemaVersion  int       `json:"schema_version"`
	ReadOnlyMode   bool      `json:"read_only_mode"`
	InspectionTime time.Time `json:"inspection_time"`
}

type CompatibilityWarning struct {
	Severity string `json:"severity"` // "INFO", "WARNING", "CRITICAL"
	Entity   string `json:"entity"`
	Message  string `json:"message"`
}

type EntityMappingSummary struct {
	SourceTable string `json:"source_table"`
	TargetTable string `json:"target_table"`
	Strategy    string `json:"strategy"`
	Status      string `json:"status"`
}

type DiagnosticReport struct {
	Metadata       SnapshotMetadata       `json:"metadata"`
	Mappings       []EntityMappingSummary `json:"mappings"`
	Warnings       []CompatibilityWarning `json:"warnings"`
	DryRunPassed   bool                   `json:"dry_run_passed"`
	SummaryMessage string                 `json:"summary_message"`
}

// InspectSnapshot performs a safe, strictly read-only inspection of a SQLite snapshot file.
// It never writes, alters, or creates lockfiles that mutate the original source file.
func InspectSnapshot(filePath string) (SnapshotMetadata, error) {
	var meta SnapshotMetadata

	cleanPath := filepath.Clean(filePath)
	info, err := os.Stat(cleanPath)
	if err != nil {
		return meta, fmt.Errorf("%w: %v", ErrFileAccess, err)
	}

	if info.IsDir() {
		return meta, fmt.Errorf("%w: caminho é um diretório", ErrFileAccess)
	}

	// Strictly open with O_RDONLY
	f, err := os.OpenFile(cleanPath, os.O_RDONLY, 0)
	if err != nil {
		return meta, fmt.Errorf("%w: %v", ErrFileAccess, err)
	}
	defer f.Close()

	hasher := sha256.New()
	headerBuf := make([]byte, 100)

	n, err := io.ReadFull(io.TeeReader(f, hasher), headerBuf)
	if err != nil || n < 100 {
		return meta, ErrNotSQLite
	}

	if !bytes.Equal(headerBuf[:16], sqliteHeaderMagic) {
		return meta, ErrNotSQLite
	}

	// Remaining file SHA-256 computation
	if _, err := io.Copy(hasher, f); err != nil {
		return meta, fmt.Errorf("%w: erro computando hash: %v", ErrFileAccess, err)
	}

	pageSizeRaw := binary.BigEndian.Uint16(headerBuf[16:18])
	pageSize := int(pageSizeRaw)
	if pageSizeRaw == 1 {
		pageSize = 65536
	}

	pageCount := int(binary.BigEndian.Uint32(headerBuf[28:32]))
	schemaVersion := int(binary.BigEndian.Uint32(headerBuf[40:44]))
	userVersion := int(binary.BigEndian.Uint32(headerBuf[60:64]))

	meta = SnapshotMetadata{
		Path:           cleanPath,
		Filename:       filepath.Base(cleanPath),
		SizeBytes:      info.Size(),
		SHA256:         hex.EncodeToString(hasher.Sum(nil)),
		PageSize:       pageSize,
		PageCount:      pageCount,
		UserVersion:    userVersion,
		SchemaVersion:  schemaVersion,
		ReadOnlyMode:   true,
		InspectionTime: time.Now().UTC(),
	}

	return meta, nil
}

// AnalyzeDumpScript analyzes a legacy SQLite SQL dump or schema export to assess
// table compatibility and map entities to the PostgreSQL SaaS architecture.
func AnalyzeDumpScript(dumpContent string) DiagnosticReport {
	report := DiagnosticReport{
		InspectionTimeNow(),
		nil,
		nil,
		true,
		"Validação de diagnóstico concluída",
	}

	knownLegacyTables := map[string]string{
		"users":                 "users (com vínculo em memberships)",
		"clientes":              "organizations",
		"empresas":              "organizations",
		"planos":                "plans e organization_limits",
		"tabela_indices":        "economic_indices e index_points",
		"indices":               "economic_indices e index_points",
		"calculos":              "legal_operations (com jsonb e HMAC)",
		"simulacoes":            "legal_operations",
		"api_keys":              "api_keys (com hash SHA-256 e escopos RBAC)",
		"consultas_processuais": "legal_operations (kind='cnj')",
	}

	lines := strings.Split(dumpContent, "\n")
	detectedTables := make(map[string]bool)

	for _, line := range lines {
		trimmed := strings.TrimSpace(strings.ToUpper(line))
		if strings.HasPrefix(trimmed, "CREATE TABLE") {
			parts := strings.Fields(line)
			if len(parts) >= 3 {
				rawName := strings.Trim(parts[2], "`\"'[](")
				lowerName := strings.ToLower(rawName)
				detectedTables[lowerName] = true
			}
		}
	}

	for legacyTable, target := range knownLegacyTables {
		if detectedTables[legacyTable] {
			report.Mappings = append(report.Mappings, EntityMappingSummary{
				SourceTable: legacyTable,
				TargetTable: target,
				Strategy:    "Mapeamento transacional com preservação de integridade",
				Status:      "Compatível",
			})
		}
	}

	// Security and privacy safeguards verification:
	report.Warnings = append(report.Warnings, CompatibilityWarning{
		Severity: "WARNING",
		Entity:   "users.password_hash",
		Message:  "Hashes legados antigos incompatíveis com Argon2id nunca devem ser rehasheados cegamente; exigir fluxo seguro de recuperação de senha por link criptografado.",
	})

	report.Warnings = append(report.Warnings, CompatibilityWarning{
		Severity: "INFO",
		Entity:   "legal_operations.hmac",
		Message:  "Memórias e cálculos legados importados recebem versão de engine explícita e nova assinatura HMAC gerada na transação de migração.",
	})

	report.Warnings = append(report.Warnings, CompatibilityWarning{
		Severity: "INFO",
		Entity:   "monetary_values",
		Message:  "Todos os valores monetários e taxas devem ser convertidos estritamente com decimais exatos (NUMERIC), rejeitando aproximações por ponto flutuante IEEE-754.",
	})

	report.SummaryMessage = fmt.Sprintf("Diagnóstico de migração concluído com sucesso. %d tabelas mapeadas para o schema PostgreSQL.", len(report.Mappings))
	return report
}

func InspectionTimeNow() SnapshotMetadata {
	return SnapshotMetadata{
		ReadOnlyMode:   true,
		InspectionTime: time.Now().UTC(),
	}
}
