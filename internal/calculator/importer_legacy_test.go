package calculator

import (
	"strings"
	"testing"
)

func TestImportarPlanilha_CSV(t *testing.T) {
	csvData := `Vencimento;Descrição;Valor;Tipo
15/01/2023;Primeira Parcela de Débito;10.000,00;DEBITO
20/01/2023;Custas do Cartório;350,50;CUSTA
10/05/2024;Pagamento Parcial Acordo;-2.000,00;PAGAMENTO_PARCIAL
`

	parcelas, err := ImportarPlanilha("teste.csv", []byte(csvData))
	if err != nil {
		t.Fatalf("Erro ao importar CSV: %v", err)
	}

	if len(parcelas) != 3 {
		t.Fatalf("Esperado 3 parcelas, obtido %d", len(parcelas))
	}

	p1 := parcelas[0]
	if p1.DataVencimento != "2023-01-15" {
		t.Errorf("Data esperada '2023-01-15', obtido '%s'", p1.DataVencimento)
	}
	if p1.ValorOriginal.String() != "10000" {
		t.Errorf("Valor esperado 10000, obtido %s", p1.ValorOriginal.String())
	}
	if p1.Tipo != ParcelaDebito {
		t.Errorf("Tipo esperado DEBITO, obtido %s", p1.Tipo)
	}

	p2 := parcelas[1]
	if p2.Tipo != ParcelaCusta {
		t.Errorf("Tipo esperado CUSTA_PROCESSUAL, obtido %s", p2.Tipo)
	}

	p3 := parcelas[2]
	if p3.Tipo != ParcelaPagamentoParcial {
		t.Errorf("Tipo esperado PAGAMENTO_PARCIAL, obtido %s", p3.Tipo)
	}

	t.Logf("Parcelas importadas com sucesso: %d", len(parcelas))
}

func TestImportarPlanilha_CSVVirgula(t *testing.T) {
	csvData := `data,descricao,valor
2024-01-15,Honorarios Periciais,1500.00
2024-02-15,Despesas Postais,85.20
`
	parcelas, err := ImportarPlanilha("dados.csv", []byte(csvData))
	if err != nil {
		t.Fatalf("Erro ao importar CSV com virgula: %v", err)
	}

	if len(parcelas) != 2 {
		t.Fatalf("Esperado 2 parcelas, obtido %d", len(parcelas))
	}

	if parcelas[0].DataVencimento != "2024-01-15" {
		t.Errorf("Data incorreta: %s", parcelas[0].DataVencimento)
	}
}

func TestImportarPlanilha_TextoColadoAreaTransferencia(t *testing.T) {
	// Exemplo de texto copiado de linhas do Excel (separado por TABs)
	tabData := "15/01/2023\tAluguel Janeiro\t5.500,00\n15/02/2023\tAluguel Fevereiro\t5.500,00\n"

	parcelas, err := ImportarPlanilha("clipboard.txt", []byte(tabData))
	if err != nil {
		t.Fatalf("Erro ao importar dados tabulados: %v", err)
	}

	if len(parcelas) != 2 {
		t.Fatalf("Esperado 2 parcelas de clipboard, obtido %d", len(parcelas))
	}

	if !strings.Contains(parcelas[0].Descricao, "Aluguel") {
		t.Errorf("Descricao esperada conter 'Aluguel', obtido: %s", parcelas[0].Descricao)
	}
}
