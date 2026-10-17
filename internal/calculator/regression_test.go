package calculator

import (
	"archive/zip"
	"bytes"
	"github.com/shopspring/decimal"
	"strings"
	"testing"
	"time"
)

func TestEngineKnownFinalPaymentAndInputOrder(t *testing.T) {
	debt := ParcelaItem{Tipo: ParcelaDebito, DataVencimento: "2026-01-01", ValorOriginal: decimal.NewFromInt(1000)}
	payment := ParcelaItem{Tipo: ParcelaPagamentoParcial, DataVencimento: "2026-03-01", ValorOriginal: decimal.NewFromInt(100)}
	for _, items := range [][]ParcelaItem{{debt, payment}, {payment, debt}} {
		r := CalculoJudicialRequest{DataAtualizacao: "2026-03-01", IndexadorCodigo: "SEM_CORRECAO", ImputarArt354CC: true, JurosMora: JurosMoraConfig{Tipo: JurosConvencionalMensal, PercentualMensal: decimal.NewFromInt(1)}, Parcelas: items}
		out, err := NovoEngineCalculo(nil).Executar(r)
		if err != nil {
			t.Fatal(err)
		}
		if !out.OK || out.Resumo.TotalGeral.StringFixed(2) != "920.00" || out.Resumo.ImputadoJurosArt354.StringFixed(2) != "20.00" || out.Resumo.ImputadoPrincipalArt354.StringFixed(2) != "80.00" || !out.Resumo.TotalCorrecaoMonetaria.IsZero() {
			t.Fatalf("%+v", out.Resumo)
		}
		r.Parcelas = []ParcelaItem{debt, {Tipo: ParcelaPagamentoParcial, DataVencimento: "2026-02-01", ValorOriginal: decimal.NewFromInt(100)}}
		if _, err := NovoEngineCalculo(nil).Executar(r); err == nil {
			t.Fatal("historical payment silently approximated")
		}
		r.ImputarArt354CC = false
		r.Parcelas = []ParcelaItem{debt, {Tipo: ParcelaPagamentoParcial, DataVencimento: "2026-03-01", ValorOriginal: decimal.NewFromInt(10000)}}
		if _, err := NovoEngineCalculo(nil).Executar(r); err == nil {
			t.Fatal("negative balance accepted")
		}
	}
}

func TestCommercialProRataWithinSameMonth(t *testing.T) {
	repo := NovoRepositorioIndices()
	repo.RegistrarIndice(IndiceInfo{Codigo: "FIXTURE", Fonte: "fixture"})
	start, _ := time.Parse("2006-01-02", "2026-01-01")
	end, _ := time.Parse("2006-01-02", "2026-01-16")
	repo.AdicionarPonto("FIXTURE", start, decimal.NewFromInt(1), decimal.Zero)
	factor, err := repo.ObterFatorAcumuladoComOpcoes("FIXTURE", start, end, true, true)
	if err != nil || factor.String() != "1.005" {
		t.Fatal(factor, err)
	}
}

func TestEngineMissingIndicesAndInterestFailClosed(t *testing.T) {
	repo := NovoRepositorioIndices()
	repo.RegistrarIndice(IndiceInfo{Codigo: "FIXTURE", Fonte: "fixture"})
	start, _ := time.Parse("2006-01-02", "2026-01-01")
	end, _ := time.Parse("2006-01-02", "2026-03-01")
	repo.AdicionarPonto("FIXTURE", start, decimal.NewFromInt(1), decimal.Zero)
	repo.AdicionarPonto("FIXTURE", end, decimal.NewFromInt(1), decimal.Zero)
	if _, err := repo.ObterFatorAcumulado("FIXTURE", start, end, true); err == nil {
		t.Fatal("missing intermediate month accepted")
	}
	req := CalculoJudicialRequest{DataAtualizacao: "2026-03-01", IndexadorCodigo: "FIXTURE", Parcelas: []ParcelaItem{{DataVencimento: "2026-01-01", ValorOriginal: decimal.NewFromInt(1000)}}}
	if _, err := NovoEngineCalculo(repo).Executar(req); err == nil {
		t.Fatal("missing correction replaced with one")
	}
	if _, _, _, err := CalcularJurosMoratorios(repo, JurosMoraConfig{Tipo: JurosSelic}, start, end, decimal.NewFromInt(1000)); err == nil {
		t.Fatal("missing interest series ignored")
	}
	_, interest, _, err := CalcularJurosMoratorios(repo, JurosMoraConfig{Tipo: JurosConvencionalMensal}, start, end, decimal.NewFromInt(1000))
	if err != nil || !interest.IsZero() {
		t.Fatal("explicit zero became default rate")
	}
}

func TestBoundedImportsAndCSVFormulaProtection(t *testing.T) {
	if _, err := ImportarPlanilha("x.csv", bytes.Repeat([]byte("x"), (2<<20)+1)); err == nil {
		t.Fatal("oversized upload")
	}
	if _, err := ImportarPlanilha("x.csv", []byte("data;descricao;valor\ninvalid;item;10\n")); err == nil {
		t.Fatal("invalid row skipped")
	}
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	w, _ := zw.Create("xl/worksheets/sheet1.xml")
	_, _ = w.Write(bytes.Repeat([]byte("x"), (8<<20)+1))
	_ = zw.Close()
	if _, err := ImportarPlanilha("x.xlsx", buf.Bytes()); err == nil {
		t.Fatal("expanded zip accepted")
	}
	csv, err := ExportarMemoriaCSV(&CalculoJudicialResponse{ItensLiquidados: []ItemLiquidado{{Descricao: "=HYPERLINK(1)"}}})
	if err != nil || !strings.Contains(string(csv), "'=HYPERLINK(1)") {
		t.Fatal("formula injection not neutralized", err)
	}
}

func TestExportedXLSXImportsInlineStringsAndSparseCells(t *testing.T) {
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	w, _ := zw.Create("xl/worksheets/sheet1.xml")
	_, _ = w.Write([]byte(`<worksheet><sheetData><row><c r="A1" t="inlineStr"><is><t>data</t></is></c><c r="C1" t="inlineStr"><is><t>descricao</t></is></c><c r="D1" t="inlineStr"><is><t>valor</t></is></c></row><row><c r="A2" t="inlineStr"><is><t>2026-01-01</t></is></c><c r="C2" t="inlineStr"><is><t>Fixture</t></is></c><c r="D2"><v>100</v></c></row></sheetData></worksheet>`))
	_ = zw.Close()
	items, err := ImportarPlanilha("x.xlsx", buf.Bytes())
	if err != nil || len(items) != 1 || items[0].ValorOriginal.String() != "100" || items[0].Descricao != "Fixture" {
		t.Fatal(items, err)
	}
}
