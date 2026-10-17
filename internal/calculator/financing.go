package calculator

import (
	"fmt"

	"github.com/shopspring/decimal"
)

type FinancingRequest struct {
	Principal    string `json:"principal"`
	MonthlyRate  string `json:"taxa_mensal"`
	Installments int    `json:"parcelas"`
	System       string `json:"sistema"`
}

type Installment struct {
	Number       int    `json:"numero"`
	Payment      string `json:"prestacao"`
	Interest     string `json:"juros"`
	Amortization string `json:"amortizacao"`
	Balance      string `json:"saldo"`
}

func Finance(r FinancingRequest) ([]Installment, error) {
	p, err := ParseDecimal(r.Principal)
	if err != nil {
		return nil, err
	}
	i, err := ParseDecimal(r.MonthlyRate)
	if err != nil {
		return nil, err
	}
	if !p.IsPositive() || !p.Equal(p.Round(2)) || i.IsNegative() || i.GreaterThan(decimal.NewFromInt(100)) || r.Installments < 1 || r.Installments > 600 {
		return nil, fmt.Errorf("financiamento fora dos limites; principal deve ter centavos exatos")
	}
	if r.System != "PRICE" && r.System != "SAC" {
		return nil, fmt.Errorf("sistema deve ser PRICE ou SAC; GAUSS aguarda homologação")
	}
	i = i.Div(decimal.NewFromInt(100))
	n := decimal.NewFromInt(int64(r.Installments))
	amort := p.Div(n).Round(2)
	payment := amort
	if !i.IsZero() {
		factor := decimal.NewFromInt(1).Add(i).Pow(n)
		payment = p.Mul(i).Mul(factor).Div(factor.Sub(decimal.NewFromInt(1))).Round(2)
	}
	balance := p
	rows := make([]Installment, 0, r.Installments)
	for k := 1; k <= r.Installments; k++ {
		interest := balance.Mul(i).Round(2)
		a := amort
		if r.System == "PRICE" {
			a = payment.Sub(interest)
		}
		if a.IsNegative() {
			return nil, fmt.Errorf("precisão insuficiente para amortização")
		}
		// The final payment itself is adjusted, keeping payment = interest + amortization.
		if k == r.Installments || a.GreaterThan(balance) {
			a = balance
		}
		balance = balance.Sub(a)
		rows = append(rows, Installment{k, a.Add(interest).StringFixed(2), interest.StringFixed(2), a.StringFixed(2), balance.StringFixed(2)})
	}
	return rows, nil
}
