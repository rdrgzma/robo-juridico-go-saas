// Adapted from the authorized legacy contract; Finance is the single schedule implementation.
package calculator

import (
	"fmt"
	"github.com/shopspring/decimal"
	"strings"
)

func CalcularRevisaoFinanciamento(r CalculoFinanciamentoRequest) (*CalculoFinanciamentoResponse, error) {
	for _, d := range []decimal.Decimal{r.ValorFinanciado, r.TaxaContratadaMensal, r.TaxaMercadoMensal, r.TaxaMensalContratada, r.TaxaMensalMercado} {
		if err := ValidateDecimal(d); err != nil {
			return nil, err
		}
	}
	if r.NumeroParcelas == 0 {
		r.NumeroParcelas = r.QuantidadeParcelas
	}
	if r.TaxaContratadaMensal.IsZero() && !r.TaxaMensalContratada.IsZero() {
		r.TaxaContratadaMensal = r.TaxaMensalContratada
	}
	if r.TaxaMercadoMensal.IsZero() && !r.TaxaMensalMercado.IsZero() {
		r.TaxaMercadoMensal = r.TaxaMensalMercado
	}
	system := strings.ToUpper(strings.TrimSpace(string(r.SistemaAmortizacao)))
	if system == "" {
		system = "PRICE"
	}
	contract, err := Finance(FinancingRequest{r.ValorFinanciado.String(), r.TaxaContratadaMensal.String(), r.NumeroParcelas, "PRICE"})
	if err != nil {
		return nil, err
	}
	revised, err := Finance(FinancingRequest{r.ValorFinanciado.String(), r.TaxaMercadoMensal.String(), r.NumeroParcelas, system})
	if err != nil {
		return nil, err
	}
	out := &CalculoFinanciamentoResponse{Sucesso: true, SistemaAmortizacao: SistemaAmortizacao(system), ValorFinanciado: r.ValorFinanciado, NumeroParcelas: r.NumeroParcelas, TaxaContratadaMensal: r.TaxaContratadaMensal, TaxaMercadoMensal: r.TaxaMercadoMensal, PrestacaoContratada: decimal.RequireFromString(contract[0].Payment), PrestacaoRevisadaInicial: decimal.RequireFromString(revised[0].Payment)}
	for k, row := range revised {
		cp := decimal.RequireFromString(contract[k].Payment)
		rp := decimal.RequireFromString(row.Payment)
		delta := cp.Sub(rp)
		out.TotalPagoContratado = out.TotalPagoContratado.Add(cp)
		out.TotalDevidoRevisado = out.TotalDevidoRevisado.Add(rp)
		out.DiferencaTotalSimples = out.DiferencaTotalSimples.Add(delta)
		out.TabelaEvolucao = append(out.TabelaEvolucao, ParcelaFinanciamentoItem{NumeroParcela: k + 1, PrestacaoContratada: cp, PrestacaoRevisada: rp, DiferencaParcela: delta, JurosRevisado: decimal.RequireFromString(row.Interest), AmortizacaoRevisada: decimal.RequireFromString(row.Amortization), SaldoDevedor: decimal.RequireFromString(row.Balance)})
	}
	out.IndebitoCobrado = decimal.Max(decimal.Zero, out.DiferencaTotalSimples)
	if r.RestituicaoEmDobro || r.RepeticaoEmDobro {
		out.IndebitoCobrado = out.IndebitoCobrado.Mul(decimal.NewFromInt(2))
	}
	out.NotasMetodologicas = []string{fmt.Sprintf("Simulação %s; valores decimais e ajuste da última prestação. Contrato comparado por PRICE. Aplicabilidade e eventual restituição exigem validação específica.", system)}
	return out, nil
}
