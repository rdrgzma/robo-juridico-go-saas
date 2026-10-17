package juridico

import (
	"bytes"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/rdrgzma/robo-juridico-go-saas/internal/calculator"
	"github.com/rdrgzma/robo-juridico-go-saas/internal/tenancy"
	"github.com/rdrgzma/robo-juridico-go-saas/web"
)

func safeCell(s string) string {
	trimmed := strings.TrimLeft(s, " \t\r\n")
	if trimmed != "" && strings.ContainsRune("=+-@", rune(trimmed[0])) {
		return "'" + s
	}
	return s
}
func (s *Service) ServeDownload(w http.ResponseWriter, r *http.Request) {
	s.download(w, r)
}
func (s *Service) download(w http.ResponseWriter, r *http.Request) {
	p, _ := tenancy.FromContext(r.Context())
	o, err := s.Get(r.Context(), p, chi.URLParam(r, "id"))
	if err != nil {
		s.failure(w, err)
		return
	}
	format := chi.URLParam(r, "format")
	var body []byte
	media := ""
	switch format {
	case "json":
		body, err = json.MarshalIndent(struct {
			Operation Operation `json:"operation"`
			Signature string    `json:"hmac_sha256"`
			Notice    string    `json:"notice"`
		}{o, s.Integrity(o), "Memória privada; integridade do registro completo, sem certificação de aplicabilidade jurídica."}, "", "  ")
		media = "application/json; charset=utf-8"
	case "csv":
		if o.Kind == "judicial" {
			var result calculator.CalculoJudicialResponse
			if err = json.Unmarshal(o.Result, &result); err == nil {
				body, err = calculator.ExportarMemoriaCSV(&result)
			}
		} else {
			v := resultPage(o, o.Title, "")
			var b bytes.Buffer
			b.WriteString("\xef\xbb\xbf")
			writer := csv.NewWriter(&b)
			writer.Comma = ';'
			for _, m := range v.Metrics {
				writer.Write([]string{safeCell(m.Name), safeCell(m.Value)})
			}
			if len(v.Headers) > 0 {
				writer.Write(v.Headers)
			}
			for _, row := range v.Rows {
				cells := make([]string, len(row))
				for i, c := range row {
					cells[i] = safeCell(c)
				}
				writer.Write(cells)
			}
			writer.Flush()
			err = writer.Error()
			body = b.Bytes()
		}
		media = "text/csv; charset=utf-8"
	case "xlsx":
		if o.Kind != "judicial" {
			http.Error(w, "XLSX disponível para liquidação judicial; use CSV nos demais módulos", 422)
			return
		}
		var result calculator.CalculoJudicialResponse
		if err = json.Unmarshal(o.Result, &result); err == nil {
			body, err = calculator.ExportarMemoriaXLSX(&result)
		}
		media = "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"
	case "html":
		w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="memoria-%s.html"`, o.ID))
		v := resultPage(o, o.Title, s.Integrity(o))
		v.View = "report"
		v.ReportStyle = web.ReportStyle()
		w.Header().Set("Content-Security-Policy", web.ReportCSP())
		s.show(w, r, 200, v)
		return
	default:
		http.NotFound(w, r)
		return
	}
	if err != nil {
		s.failure(w, err)
		return
	}
	w.Header().Set("Content-Type", media)
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="memoria-%s.%s"`, o.ID, format))
	w.Write(body)
}
