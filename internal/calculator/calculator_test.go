package calculator

import (
	"testing"

	"github.com/shopspring/decimal"
)

func baseRequest() Request {
	return Request{Principal: "1000", CorrectionFactor: "1.1", MonthlyRate: "1", Months: 2, Payment: "100", InterestFirst: true, PenaltyRate: "10", FeeRate: "10"}
}

func TestKnownLiquidation(t *testing.T) {
	// 1000*1.1=1100; simple interest 1100*.01*2=22; payment 22+78;
	// remaining 1022, each charge 102.20, total 1226.40.
	r, err := Calculate(baseRequest())
	if err != nil {
		t.Fatal(err)
	}
	if r.Corrected != "1100.00" || r.Interest != "22.00" || r.PaidInterest != "22.00" || r.PaidPrincipal != "78.00" || r.Balance != "1022.00" || r.Total != "1226.40" {
		t.Fatalf("%+v", r)
	}
}

func TestCompoundAndZeroRate(t *testing.T) {
	r := baseRequest()
	r.Compound = true
	out, err := Calculate(r)
	if err != nil || out.Interest != "22.11" || out.Total != "1226.53" {
		t.Fatalf("%+v %v", out, err)
	}
	r.MonthlyRate = "0"
	out, err = Calculate(r)
	if err != nil || out.Interest != "0.00" {
		t.Fatalf("%+v %v", out, err)
	}
}

func TestRejectInvalidLiquidation(t *testing.T) {
	for _, modify := range []func(*Request){
		func(r *Request) { r.Principal = "1e1000000" }, func(r *Request) { r.CorrectionFactor = "" }, func(r *Request) { r.MonthlyRate = "-1" }, func(r *Request) { r.Payment = "9999" }, func(r *Request) { r.Months = 1201 }, func(r *Request) { r.PenaltyRate = "101" },
	} {
		r := baseRequest()
		modify(&r)
		if _, err := Calculate(r); err == nil {
			t.Fatal("accepted invalid request")
		}
	}
}

func TestFinancingKnownAndConservation(t *testing.T) {
	for _, system := range []string{"PRICE", "SAC", "GAUSS"} {
		for _, rate := range []string{"0", "1", "2.85"} {
			rows, err := Finance(FinancingRequest{"1000", rate, 3, system})
			if err != nil {
				t.Fatal(err)
			}
			amortization := decimal.Zero
			previous := decimal.NewFromInt(1000)
			for _, row := range rows {
				p := decimal.RequireFromString(row.Payment)
				i := decimal.RequireFromString(row.Interest)
				a := decimal.RequireFromString(row.Amortization)
				b := decimal.RequireFromString(row.Balance)
				if !p.Equal(i.Add(a)) || !previous.Sub(a).Equal(b) || b.IsNegative() {
					t.Fatalf("unbalanced row: %+v", row)
				}
				amortization = amortization.Add(a)
				previous = b
			}
			if !amortization.Equal(decimal.NewFromInt(1000)) || !previous.IsZero() {
				t.Fatal(rows)
			}
			if rate == "0" && (rows[0].Payment != "333.33" || rows[2].Payment != "333.34") {
				t.Fatal(rows)
			}
			if system == "SAC" && rate == "1" && rows[0].Payment != "343.33" {
				t.Fatal(rows)
			}
		}
	}
}

func TestFinancingValidation(t *testing.T) {
	for _, r := range []FinancingRequest{{"1000", "1", 0, "PRICE"}, {"1000", "-1", 3, "PRICE"}, {"1000", "1", 3, "UNKNOWN"}, {"0", "1", 3, "SAC"}, {"1000.001", "1", 3, "SAC"}} {
		if _, err := Finance(r); err == nil {
			t.Fatal(r)
		}
	}
}
