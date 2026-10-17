package transport

import (
	"strings"
	"testing"
)

func TestLegacyCalculationContracts(t *testing.T) {
	h := handler()
	body := `{"data_atualizacao":"2026-03-01","indexador_codigo":"SEM_CORRECAO","juros_mora":{"tipo":"CONVENCIONAL_MENSAL","percentual_mensal":"1"},"parcelas":[{"data_vencimento":"2026-01-01","valor_original":"1000"}]}`
	for _, prefix := range []string{"/v1", "/api/v1"} {
		w := request(h, "POST", prefix+"/calculos/judicial", "test-key", body)
		if w.Code != 200 || !strings.Contains(w.Body.String(), `"total_geral":"1020"`) || !strings.Contains(w.Body.String(), `"ok":true`) {
			t.Fatal(w.Code, w.Body.String())
		}
	}
	extreme := strings.Replace(body, `"1000"`, `"1e100"`, 1)
	if w := request(h, "POST", "/v1/calculos/judicial", "test-key", extreme); w.Code != 422 {
		t.Fatal("extreme exponent accepted", w.Code)
	}
	legal := strings.Replace(body, "CONVENCIONAL_MENSAL", "TAXA_LEGAL", 1)
	if w := request(h, "POST", "/v1/calculos/judicial", "test-key", legal); w.Code != 422 {
		t.Fatal("unverified legal rate exposed", w.Code)
	}
}
