package calculator

import "fmt"

import "github.com/shopspring/decimal"

func ValidateDecimal(d decimal.Decimal) error {
	if d.Exponent() < -8 || d.Exponent() > 8 || d.Coefficient().BitLen() > 80 {
		return fmt.Errorf("decimal excede precisão ou magnitude permitida")
	}
	return nil
}

func validarCalculo(r CalculoJudicialRequest) error {
	for _, d := range []decimal.Decimal{r.JurosMora.PercentualMensal, r.JurosCompensatorios.PercentualMensal, r.JurosCompensatorios.TaxaMensal, r.FaseExecucao.MultaMoratoriaPercentual, r.FaseExecucao.HonorariosSucumbenciaisPercentual, r.FaseExecucao.HonorariosSucumbenciaisValorFixo} {
		if err := ValidateDecimal(d); err != nil {
			return err
		}
	}
	if len(r.Parcelas) == 0 || len(r.Parcelas) > 1000 {
		return fmt.Errorf("informe entre 1 e 1000 parcelas")
	}
	end, err := ParseDataFlexivel(r.DataAtualizacao)
	if err != nil || end.Year() < 1900 || end.Year() > 2100 {
		return fmt.Errorf("data de atualização inválida")
	}
	switch r.JurosMora.Tipo {
	case "", JurosNenhum, JurosConvencionalMensal, JurosTaxaLegal, JurosLei14905, JurosSelic:
	default:
		return fmt.Errorf("tipo de juros inválido")
	}
	for _, mode := range []ModoCapitalizacao{r.JurosMora.Modo, r.JurosCompensatorios.Modo} {
		if mode != "" && mode != CapitalizacaoSimples && mode != CapitalizacaoComposta {
			return fmt.Errorf("capitalização inválida")
		}
	}
	for _, v := range []string{r.JurosMora.PercentualMensal.String(), r.JurosCompensatorios.PercentualMensal.String(), r.JurosCompensatorios.TaxaMensal.String(), r.FaseExecucao.MultaMoratoriaPercentual.String(), r.FaseExecucao.HonorariosSucumbenciaisPercentual.String(), r.FaseExecucao.HonorariosSucumbenciaisValorFixo.String()} {
		d, err := ParseDecimal(v)
		if err != nil || d.IsNegative() {
			return fmt.Errorf("taxa ou valor fora dos limites")
		}
	}
	for _, s := range []string{r.JurosMora.DataInicioGeral, r.JurosCompensatorios.DataInicio, r.JurosCompensatorios.DataFim} {
		if s != "" {
			d, err := ParseDataFlexivel(s)
			if err != nil || d.After(end) || d.Year() < 1900 {
				return fmt.Errorf("data de juros inválida")
			}
		}
	}
	for _, p := range r.Parcelas {
		if err := ValidateDecimal(p.ValorOriginal); err != nil {
			return err
		}
		date := p.DataVencimento
		if date == "" {
			date = p.Data
		}
		d, err := ParseDataFlexivel(date)
		if err != nil || d.Year() < 1900 || d.After(end) {
			return fmt.Errorf("vencimento inválido")
		}
		if p.DataInicioJuros != "" {
			j, err := ParseDataFlexivel(p.DataInicioJuros)
			if err != nil || j.After(end) || j.Year() < 1900 {
				return fmt.Errorf("início de juros inválido")
			}
		}
		switch p.Tipo {
		case "", ParcelaDebito, ParcelaCusta, ParcelaDespesa:
			if p.ValorOriginal.IsNegative() {
				return fmt.Errorf("débito negativo")
			}
		case ParcelaPagamentoParcial:
			if !d.Equal(end) {
				return fmt.Errorf("pagamentos intermediários exigem motor cronológico homologado; use pagamento na data final nesta versão")
			}
		default:
			return fmt.Errorf("tipo de parcela inválido")
		}
		if _, err := ParseDecimal(p.ValorOriginal.String()); err != nil || len(p.Descricao) > 2000 {
			return fmt.Errorf("parcela fora dos limites")
		}
	}
	return nil
}
