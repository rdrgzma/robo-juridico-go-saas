// Package indices applies complete, explicitly supplied monthly series.
// This package has no fabricated seed, network fetch or implicit fallback.
package indices

import (
	"fmt"
	"time"

	"github.com/rdrgzma/robo-juridico-go-saas/internal/calculator"
	"github.com/shopspring/decimal"
)

type Point struct {
	Month   string
	Percent string
	Source  string
}

// Factor includes both reference months; it is not a pro-rata legal policy.
func Factor(start, end string, points []Point, allowDeflation bool) (decimal.Decimal, error) {
	first, err := time.Parse("2006-01", start)
	if err != nil {
		return decimal.Zero, fmt.Errorf("mês inicial inválido")
	}
	last, err := time.Parse("2006-01", end)
	if err != nil || first.After(last) || last.Year()-first.Year() > 100 {
		return decimal.Zero, fmt.Errorf("intervalo inválido")
	}
	if len(points) > 1212 {
		return decimal.Zero, fmt.Errorf("série excede limite")
	}
	series := map[string]decimal.Decimal{}
	for _, p := range points {
		if _, err := time.Parse("2006-01", p.Month); err != nil || p.Source == "" {
			return decimal.Zero, fmt.Errorf("ponto sem mês ou fonte válidos")
		}
		if _, ok := series[p.Month]; ok {
			return decimal.Zero, fmt.Errorf("mês duplicado: %s", p.Month)
		}
		rate, err := calculator.ParseDecimal(p.Percent)
		if err != nil || rate.LessThanOrEqual(decimal.NewFromInt(-100)) || rate.GreaterThan(decimal.NewFromInt(1000)) {
			return decimal.Zero, fmt.Errorf("variação inválida")
		}
		series[p.Month] = rate
	}
	factor := decimal.NewFromInt(1)
	for month := first; !month.After(last); month = month.AddDate(0, 1, 0) {
		rate, ok := series[month.Format("2006-01")]
		if !ok {
			return decimal.Zero, fmt.Errorf("série incompleta em %s", month.Format("2006-01"))
		}
		if !allowDeflation && rate.IsNegative() {
			rate = decimal.Zero
		}
		factor = factor.Mul(decimal.NewFromInt(1).Add(rate.Div(decimal.NewFromInt(100)))).Round(16)
	}
	return factor, nil
}
