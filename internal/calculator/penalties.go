package calculator

import (
	"github.com/shopspring/decimal"
)

// CalcularEncargosExecucao calcula as multas e honorários aplicáveis à liquidação
func CalcularEncargosExecucao(
	cfg FaseExecucaoConfig,
	subtotalDebito decimal.Decimal,
) (multaArt523, honorariosArt523, honorariosSucumbenciais decimal.Decimal) {

	cento := decimal.NewFromInt(100)
	dez := decimal.NewFromInt(10)

	// 1. Multa do Art. 523 § 1º do CPC (10% pelo não pagamento voluntário)
	if cfg.AplicarArt523Multa {
		multaArt523 = subtotalDebito.Mul(dez).Div(cento).Round(2)
	}

	// 2. Honorários da fase de cumprimento de sentença (10% pelo Art. 523 § 1º do CPC)
	if cfg.AplicarArt523Honorarios {
		honorariosArt523 = subtotalDebito.Mul(dez).Div(cento).Round(2)
	}

	// 3. Honorários sucumbenciais gerais (Art. 85 CPC)
	if !cfg.HonorariosSucumbenciaisPercentual.IsZero() {
		honorariosSucumbenciais = subtotalDebito.Mul(cfg.HonorariosSucumbenciaisPercentual).Div(cento).Round(2)
	} else if !cfg.HonorariosSucumbenciaisValorFixo.IsZero() {
		honorariosSucumbenciais = cfg.HonorariosSucumbenciaisValorFixo
	}

	return multaArt523, honorariosArt523, honorariosSucumbenciais
}
