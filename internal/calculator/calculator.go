// Package calculator contains deterministic arithmetic, independent of HTTP/storage.
// Rates, factors and applicability must be supplied by a validated business policy.
package calculator

import (
	"fmt"
	"regexp"

	"github.com/shopspring/decimal"
)

var fixed = regexp.MustCompile(`^-?[0-9]{1,16}(\.[0-9]{1,8})?$`)

// ParseDecimal bounds work and precision; scientific notation and float64 are not used.
func ParseDecimal(s string) (decimal.Decimal, error) {
	if !fixed.MatchString(s) {
		return decimal.Zero, fmt.Errorf("decimal inválido: use ponto e até 8 casas, sem expoente")
	}
	return decimal.NewFromString(s)
}

type Request struct {
	Principal        string `json:"principal"`
	CorrectionFactor string `json:"fator_correcao"`
	MonthlyRate      string `json:"taxa_mensal"`
	Months           int    `json:"meses"`
	Compound         bool   `json:"compostos"`
	Payment          string `json:"pagamento"`
	InterestFirst    bool   `json:"imputar_juros_primeiro"`
	PenaltyRate      string `json:"multa_percentual"`
	FeeRate          string `json:"honorarios_percentual"`
}

type Result struct {
	Corrected     string `json:"principal_corrigido"`
	Interest      string `json:"juros"`
	PaidInterest  string `json:"pagamento_juros"`
	PaidPrincipal string `json:"pagamento_principal"`
	Balance       string `json:"saldo"`
	Penalty       string `json:"multa"`
	Fees          string `json:"honorarios"`
	Total         string `json:"total"`
	Method        string `json:"metodo"`
}

func Calculate(r Request) (Result, error) {
	var out Result
	values := []string{r.Principal, r.CorrectionFactor, r.MonthlyRate, r.Payment, r.PenaltyRate, r.FeeRate}
	v := make([]decimal.Decimal, len(values))
	for i, s := range values {
		d, err := ParseDecimal(s)
		if err != nil {
			return out, err
		}
		if d.IsNegative() {
			return out, fmt.Errorf("valores negativos não permitidos")
		}
		v[i] = d
	}
	if !v[0].IsPositive() || !v[1].IsPositive() || r.Months < 0 || r.Months > 1200 || v[2].GreaterThan(decimal.NewFromInt(100)) || v[4].GreaterThan(decimal.NewFromInt(100)) || v[5].GreaterThan(decimal.NewFromInt(100)) {
		return out, fmt.Errorf("principal, fator, meses ou percentuais fora dos limites")
	}
	principal := v[0].Mul(v[1]).Round(2)
	rate := v[2].Div(decimal.NewFromInt(100))
	interest := principal.Mul(rate).Mul(decimal.NewFromInt(int64(r.Months))).Round(2)
	if r.Compound {
		interest = principal.Mul(decimal.NewFromInt(1).Add(rate).Pow(decimal.NewFromInt(int64(r.Months))).Sub(decimal.NewFromInt(1))).Round(2)
	}
	payment := v[3].Round(2)
	if payment.GreaterThan(principal.Add(interest)) {
		return out, fmt.Errorf("pagamento superior ao saldo; crédito excedente não suportado nesta versão")
	}
	paidI, paidP := decimal.Zero, decimal.Zero
	if r.InterestFirst {
		paidI = decimal.Min(payment, interest)
		paidP = payment.Sub(paidI)
	} else {
		paidP = decimal.Min(payment, principal)
		paidI = payment.Sub(paidP)
	}
	balance := principal.Sub(paidP).Add(interest.Sub(paidI))
	penalty := balance.Mul(v[4]).Div(decimal.NewFromInt(100)).Round(2)
	fees := balance.Mul(v[5]).Div(decimal.NewFromInt(100)).Round(2)
	return Result{principal.StringFixed(2), interest.StringFixed(2), paidI.StringFixed(2), paidP.StringFixed(2), balance.StringFixed(2), penalty.StringFixed(2), fees.StringFixed(2), balance.Add(penalty).Add(fees).StringFixed(2), "Fator informado; meses inteiros; pagamento na data final; encargos sobre saldo sem cascata; arredondamento por componente"}, nil
}
