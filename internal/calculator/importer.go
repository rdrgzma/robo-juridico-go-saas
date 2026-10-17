package calculator

import (
	"bytes"
	"encoding/csv"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/shopspring/decimal"
)

// ImportarPlanilha extrai parcelas de um arquivo Excel (.xlsx) ou CSV (.csv)
func ImportarPlanilha(nomeArquivo string, conteudo []byte) ([]ParcelaItem, error) {
	if len(conteudo) == 0 || len(conteudo) > 2<<20 {
		return nil, fmt.Errorf("arquivo deve ter até 2 MiB")
	}
	nomeLower := strings.ToLower(nomeArquivo)

	if strings.HasSuffix(nomeLower, ".xlsx") {
		return importarXLSX(conteudo)
	}

	return importarCSV(conteudo)
}

// importarCSV processa arquivos delimitados por ponto-e-vírgula, vírgula ou tabulação
func importarCSV(conteudo []byte) ([]ParcelaItem, error) {
	// Detecta delimitador comum
	delimitador := ';'
	primeiraLinha := string(bytes.Split(conteudo, []byte("\n"))[0])
	if strings.Count(primeiraLinha, "\t") > strings.Count(primeiraLinha, ";") {
		delimitador = '\t'
	} else if strings.Count(primeiraLinha, ",") > strings.Count(primeiraLinha, ";") {
		delimitador = ','
	}

	r := csv.NewReader(bytes.NewReader(conteudo))
	r.Comma = delimitador
	r.LazyQuotes = false
	r.FieldsPerRecord = -1

	linhas, err := r.ReadAll()
	if err != nil {
		return nil, fmt.Errorf("erro ao ler arquivo CSV: %w", err)
	}

	if len(linhas) > 1001 {
		return nil, fmt.Errorf("planilha excede 1000 linhas de dados")
	}
	for _, row := range linhas {
		if len(row) > 64 {
			return nil, fmt.Errorf("mais de 64 colunas")
		}
		for _, cell := range row {
			if len(cell) > 8192 {
				return nil, fmt.Errorf("célula excede limite")
			}
		}
	}
	if len(linhas) == 0 {
		return nil, fmt.Errorf("arquivo CSV vazio")
	}

	return processarLinhasTabela(linhas)
}

func processarLinhasTabela(linhas [][]string) ([]ParcelaItem, error) {
	if len(linhas) == 0 {
		return nil, fmt.Errorf("planilha sem linhas de dados")
	}

	// Identifica cabeçalhos e colunas
	idxData := -1
	idxDescricao := -1
	idxValor := -1
	idxTipo := -1

	startRow := 0
	cabecalho := linhas[0]

	for i, c := range cabecalho {
		cNorm := strings.ToLower(strings.TrimSpace(c))
		if strings.Contains(cNorm, "data") || strings.Contains(cNorm, "vencto") || strings.Contains(cNorm, "vencimento") {
			idxData = i
		} else if strings.Contains(cNorm, "desc") || strings.Contains(cNorm, "historico") || strings.Contains(cNorm, "parcela") {
			idxDescricao = i
		} else if strings.Contains(cNorm, "valor") || strings.Contains(cNorm, "original") || strings.Contains(cNorm, "nominal") {
			idxValor = i
		} else if strings.Contains(cNorm, "tipo") || strings.Contains(cNorm, "natureza") {
			idxTipo = i
		}
	}

	// Se encontrou cabeçalho, começa na linha 1; senão assume posições padrão (0: Data, 1: Descrição, 2: Valor)
	if idxData != -1 || idxValor != -1 {
		startRow = 1
	}

	if idxData == -1 {
		idxData = 0
	}
	if idxDescricao == -1 {
		idxDescricao = 1
	}
	if idxValor == -1 {
		idxValor = 2
	}

	var parcelas []ParcelaItem
	numItem := 1

	for r := startRow; r < len(linhas); r++ {
		linha := linhas[r]
		if len(linha) == 0 {
			continue
		}

		// Garante índices válidos
		strData := ""
		if idxData < len(linha) {
			strData = strings.TrimSpace(linha[idxData])
		}

		strDesc := fmt.Sprintf("Item %d", numItem)
		if idxDescricao < len(linha) && strings.TrimSpace(linha[idxDescricao]) != "" {
			strDesc = strings.TrimSpace(linha[idxDescricao])
		}

		strValor := ""
		if idxValor < len(linha) {
			strValor = strings.TrimSpace(linha[idxValor])
		}

		strTipo := "DEBITO"
		if idxTipo != -1 && idxTipo < len(linha) {
			t := strings.ToUpper(strings.TrimSpace(linha[idxTipo]))
			if strings.Contains(t, "CUSTA") {
				strTipo = "CUSTA_PROCESSUAL"
			} else if strings.Contains(t, "DESPESA") {
				strTipo = "DESPESA_PROCESSUAL"
			} else if strings.Contains(t, "PAG") || strings.Contains(t, "ABAT") {
				strTipo = "PAGAMENTO_PARCIAL"
			}
		}

		if strData == "" && strValor == "" {
			continue
		}

		dtFormatada := normalizarDataString(strData)
		if dtFormatada == "" {
			return nil, fmt.Errorf("data inválida na linha %d", r+1)
		}

		valDecimal, err := normalizarValorString(strValor)
		if err != nil || valDecimal.IsZero() {
			return nil, fmt.Errorf("valor inválido na linha %d", r+1)
		}

		parcelas = append(parcelas, ParcelaItem{
			NumeroItem:     numItem,
			Descricao:      strDesc,
			Tipo:           TipoParcela(strTipo),
			DataVencimento: dtFormatada,
			Data:           dtFormatada,
			ValorOriginal:  valDecimal,
		})
		numItem++
	}

	if len(parcelas) == 0 {
		return nil, fmt.Errorf("nenhuma parcela valida identificada na planilha")
	}

	return parcelas, nil
}

// normalizarDataString converte DD/MM/AAAA, AAAA-MM-DD ou dias seriais do Excel para AAAA-MM-DD
func normalizarDataString(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}

	// Caso serial do Excel (ex: "44941" para 15/01/2023)
	if num, err := strconv.Atoi(s); err == nil && num > 30000 && num < 60000 {
		// Marco inicial do Excel: 30/12/1899
		base := time.Date(1899, 12, 30, 0, 0, 0, 0, time.UTC)
		d := base.AddDate(0, 0, num)
		return d.Format("2006-01-02")
	}

	dt, err := ParseDataFlexivel(s)
	if err == nil && !dt.IsZero() {
		return dt.Format("2006-01-02")
	}

	return ""
}

// normalizarValorString limpa R$, pontos e converte vírgula para ponto decimal
func normalizarValorString(s string) (decimal.Decimal, error) {
	s = strings.TrimSpace(s)
	s = strings.ReplaceAll(s, "R$", "")
	s = strings.ReplaceAll(s, " ", "")

	// Se tiver formato brasileiro 1.500,50
	if strings.Contains(s, ",") {
		s = strings.ReplaceAll(s, ".", "")
		s = strings.ReplaceAll(s, ",", ".")
	}

	return ParseDecimal(s)
}
