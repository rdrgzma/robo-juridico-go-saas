package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"

	"github.com/rdrgzma/robo-juridico-go-saas/internal/legacymigrate"
)

func main() {
	dbPath := flag.String("db", "", "Caminho do arquivo SQLite legado (somente-leitura)")
	dumpPath := flag.String("dump", "", "Caminho de arquivo SQL dump legado para análise")
	jsonOutput := flag.Bool("json", false, "Exibir diagnóstico em formato JSON")
	dryRun := flag.Bool("dry-run", true, "Executar validação em modo simulação sem modificações")
	flag.Parse()

	if *dbPath == "" && *dumpPath == "" {
		fmt.Fprintln(os.Stderr, "Uso: rj-legacymigrate -db <caminho.db> [-dry-run] [-json]")
		fmt.Fprintln(os.Stderr, "     rj-legacymigrate -dump <caminho.sql> [-dry-run] [-json]")
		os.Exit(2)
	}

	report := legacymigrate.DiagnosticReport{
		DryRunPassed: true,
	}

	if *dbPath != "" {
		meta, err := legacymigrate.InspectSnapshot(*dbPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Erro inspecionando arquivo SQLite legado: %v\n", err)
			os.Exit(1)
		}
		report.Metadata = meta
	}

	if *dumpPath != "" {
		content, err := os.ReadFile(*dumpPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Erro lendo dump SQL: %v\n", err)
			os.Exit(1)
		}
		dumpReport := legacymigrate.AnalyzeDumpScript(string(content))
		report.Mappings = dumpReport.Mappings
		report.Warnings = append(report.Warnings, dumpReport.Warnings...)
		report.SummaryMessage = dumpReport.SummaryMessage
	}

	if *dryRun {
		report.SummaryMessage += " [MODO DRY-RUN CONFIRMADO: nenhum arquivo ou banco foi alterado]"
	}

	if *jsonOutput {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		if err := enc.Encode(report); err != nil {
			fmt.Fprintf(os.Stderr, "Erro gerando JSON: %v\n", err)
			os.Exit(1)
		}
		return
	}

	fmt.Println("=================================================================")
	fmt.Println("Robô Jurídico SaaS — Diagnóstico de Migração Legada (Dry-Run)")
	fmt.Println("=================================================================")
	if report.Metadata.Filename != "" {
		fmt.Printf("Arquivo:       %s\n", report.Metadata.Filename)
		fmt.Printf("Tamanho:       %d bytes\n", report.Metadata.SizeBytes)
		fmt.Printf("SHA-256:       %s\n", report.Metadata.SHA256)
		fmt.Printf("Page Size:     %d bytes\n", report.Metadata.PageSize)
		fmt.Printf("Páginas:       %d\n", report.Metadata.PageCount)
		fmt.Printf("Somente-Leitura: %v\n", report.Metadata.ReadOnlyMode)
	}
	if len(report.Mappings) > 0 {
		fmt.Println("\nMapeamentos de Tabelas Identificados:")
		for _, m := range report.Mappings {
			fmt.Printf("  • %-18s -> %-30s [%s]\n", m.SourceTable, m.TargetTable, m.Status)
		}
	}
	if len(report.Warnings) > 0 {
		fmt.Println("\nAdvertências e Diretrizes de Segurança:")
		for _, w := range report.Warnings {
			fmt.Printf("  [%s] %s: %s\n", w.Severity, w.Entity, w.Message)
		}
	}
	fmt.Printf("\nResultado: %s\n", report.SummaryMessage)
}
