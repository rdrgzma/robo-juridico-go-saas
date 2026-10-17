package calculator

import (
	"archive/zip"
	"bytes"
	"encoding/csv"
	"encoding/xml"
	"fmt"
	"strings"
)

// ExportarMemoriaCSV exporta a memória de cálculo analítica e sintética em formato CSV (UTF-8 com BOM para Excel)
func ExportarMemoriaCSV(resp *CalculoJudicialResponse) ([]byte, error) {
	if resp == nil {
		return nil, fmt.Errorf("dados do calculo nao informados")
	}

	var buf bytes.Buffer
	// BOM UTF-8 para que o Microsoft Excel abra acentos corretamente
	buf.Write([]byte{0xEF, 0xBB, 0xBF})

	w := csv.NewWriter(&buf)
	w.Comma = ';'

	// Cabeçalho do Resumo
	_ = w.Write([]string{"MEMORIA DE LIQUIDACAO JUDICIAL - CALCULO DO SERVIDOR"})
	_ = w.Write([]string{"Data Base de Atualizacao", resp.Resumo.DataAtualizacao})
	_ = w.Write([]string{"Indexador Monetario", resp.Resumo.IndexadorNome})
	_ = w.Write([]string{""})

	// Tabela Analítica de Parcelas
	_ = w.Write([]string{
		"Item",
		"Descricao",
		"Natureza",
		"Data Vencimento",
		"Valor Original (R$)",
		"Fator Correcao",
		"Valor Corrigido (R$)",
		"Taxa Juros (%)",
		"Juros Mora (R$)",
		"Juros Compensatorios (R$)",
		"Multa Item (R$)",
		"Subtotal Item (R$)",
		"Observacoes",
	})

	itens := resp.ItensLiquidados
	if len(itens) == 0 {
		itens = resp.Itens
	}

	for _, it := range itens {
		_ = w.Write([]string{
			fmt.Sprintf("%d", it.NumeroItem),
			protegerCSV(it.Descricao),
			string(it.Tipo),
			it.DataVencimento,
			formatarBR(it.ValorOriginal.StringFixed(2)),
			strings.ReplaceAll(it.FatorCorrecao.StringFixed(6), ".", ","),
			formatarBR(it.ValorCorrigido.StringFixed(2)),
			formatarBR(it.TaxaJurosAcumulada.StringFixed(4)),
			formatarBR(it.ValorJurosMora.StringFixed(2)),
			formatarBR(it.ValorJurosCompensatorios.StringFixed(2)),
			formatarBR(it.ValorMultaMoratoria.StringFixed(2)),
			formatarBR(it.TotalItem.StringFixed(2)),
			protegerCSV(it.Observacoes),
		})
	}

	_ = w.Write([]string{""})
	_ = w.Write([]string{"QUADRO RESUMO DA CONDENACAO", "VALOR (R$)"})
	_ = w.Write([]string{"Principal Original", formatarBR(resp.Resumo.TotalPrincipalOriginal.StringFixed(2))})
	_ = w.Write([]string{"Principal Corrigido", formatarBR(resp.Resumo.TotalPrincipalCorrigido.StringFixed(2))})
	_ = w.Write([]string{"Correcao Monetaria Acumulada", formatarBR(resp.Resumo.TotalCorrecaoMonetaria.StringFixed(2))})
	_ = w.Write([]string{"Total Juros de Mora", formatarBR(resp.Resumo.TotalJurosMora.StringFixed(2))})
	_ = w.Write([]string{"Total Juros Compensatorios", formatarBR(resp.Resumo.TotalJurosCompensatorios.StringFixed(2))})
	_ = w.Write([]string{"Total Multas", formatarBR(resp.Resumo.TotalMultas.StringFixed(2))})
	_ = w.Write([]string{"Total Honorarios", formatarBR(resp.Resumo.TotalHonorarios.StringFixed(2))})
	_ = w.Write([]string{"Total Custas e Despesas", formatarBR(resp.Resumo.TotalCustasEDespesas.StringFixed(2))})
	_ = w.Write([]string{"Total Abatimentos / Pagamentos Parciais", formatarBR(resp.Resumo.TotalAbatimentos.StringFixed(2))})
	_ = w.Write([]string{"TOTAL GERAL LIQUIDO DEVIDO", formatarBR(resp.Resumo.TotalGeralDevido.StringFixed(2))})

	w.Flush()
	return buf.Bytes(), w.Error()
}

// ExportarMemoriaXLSX gera um arquivo Excel (.xlsx) válido nativamente via archive/zip
func ExportarMemoriaXLSX(resp *CalculoJudicialResponse) ([]byte, error) {
	if resp == nil {
		return nil, fmt.Errorf("dados do calculo nao informados")
	}

	itens := resp.ItensLiquidados
	if len(itens) == 0 {
		itens = resp.Itens
	}

	linhas := [][]string{
		{"Item", "Descricao", "Natureza", "Data Vencimento", "Valor Original", "Fator Correcao", "Valor Corrigido", "Juros", "Multa", "Subtotal"},
	}

	for _, it := range itens {
		linhas = append(linhas, []string{
			fmt.Sprintf("%d", it.NumeroItem),
			it.Descricao,
			string(it.Tipo),
			it.DataVencimento,
			it.ValorOriginal.StringFixed(2),
			it.FatorCorrecao.StringFixed(6),
			it.ValorCorrigido.StringFixed(2),
			it.ValorJurosMora.Add(it.ValorJurosCompensatorios).StringFixed(2),
			it.ValorMultaMoratoria.StringFixed(2),
			it.TotalItem.StringFixed(2),
		})
	}

	linhas = append(linhas,
		[]string{"", "", "", "", "", "", "", "", "", ""},
		[]string{"", "RESUMO", "Indexador", resp.Resumo.IndexadorNome, "", "", "", "", "Principal Corrigido", resp.Resumo.TotalPrincipalCorrigido.StringFixed(2)},
		[]string{"", "RESUMO", "Data Base", resp.Resumo.DataAtualizacao, "", "", "", "", "Total Juros", resp.Resumo.TotalJuros.StringFixed(2)},
		[]string{"", "RESUMO", "", "", "", "", "", "", "Total Multas + Hon.", resp.Resumo.TotalMultas.Add(resp.Resumo.TotalHonorarios).StringFixed(2)},
		[]string{"", "RESUMO", "", "", "", "", "", "", "TOTAL GERAL DEVIDO", resp.Resumo.TotalGeralDevido.StringFixed(2)},
	)

	return construirXLSXSimples(linhas)
}

func construirXLSXSimples(linhas [][]string) ([]byte, error) {
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)

	arquivos := map[string]string{
		"[Content_Types].xml": `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types">
  <Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/>
  <Default Extension="xml" ContentType="application/xml"/>
  <Override PartName="/xl/workbook.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.sheet.main+xml"/>
  <Override PartName="/xl/worksheets/sheet1.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.worksheet+xml"/>
</Types>`,
		"_rels/.rels": `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">
  <Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/officeDocument" Target="xl/workbook.xml"/>
</Relationships>`,
		"xl/_rels/workbook.xml.rels": `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">
  <Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/worksheet" Target="worksheets/sheet1.xml"/>
</Relationships>`,
		"xl/workbook.xml": `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<workbook xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main" xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships">
  <sheets>
    <sheet name="Memoria de Calculo" sheetId="1" r:id="rId1"/>
  </sheets>
</workbook>`,
	}

	for nome, conteudo := range arquivos {
		f, err := zw.Create(nome)
		if err != nil {
			return nil, err
		}
		if _, err := f.Write([]byte(conteudo)); err != nil {
			return nil, err
		}
	}

	var sheet bytes.Buffer
	sheet.WriteString(`<?xml version="1.0" encoding="UTF-8" standalone="yes"?>`)
	sheet.WriteString(`<worksheet xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main"><sheetData>`)

	for rIdx, linha := range linhas {
		sheet.WriteString(fmt.Sprintf(`<row r="%d">`, rIdx+1))
		for cIdx, val := range linha {
			colLetra := string(rune('A' + cIdx))
			var escaped bytes.Buffer
			_ = xml.EscapeText(&escaped, []byte(val))
			sheet.WriteString(fmt.Sprintf(`<c r="%s%d" t="inlineStr"><is><t>%s</t></is></c>`, colLetra, rIdx+1, escaped.String()))
		}
		sheet.WriteString(`</row>`)
	}

	sheet.WriteString(`</sheetData></worksheet>`)

	fSheet, err := zw.Create("xl/worksheets/sheet1.xml")
	if err != nil {
		return nil, err
	}
	if _, err := fSheet.Write(sheet.Bytes()); err != nil {
		return nil, err
	}

	if err := zw.Close(); err != nil {
		return nil, err
	}

	return buf.Bytes(), nil
}

func formatarBR(valor string) string {
	return strings.ReplaceAll(valor, ".", ",")
}

func protegerCSV(s string) string {
	trim := strings.TrimLeft(s, " \t\r\n")
	if trim != "" && strings.ContainsAny(trim[:1], "=+-@") {
		return "'" + s
	}
	return s
}
