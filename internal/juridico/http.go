package juridico

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/rdrgzma/robo-juridico-go-saas/internal/auth"
	"github.com/rdrgzma/robo-juridico-go-saas/internal/calculator"
	"github.com/rdrgzma/robo-juridico-go-saas/internal/prazos"
	"github.com/rdrgzma/robo-juridico-go-saas/internal/tenancy"
	"github.com/rdrgzma/robo-juridico-go-saas/web"
	"github.com/shopspring/decimal"
)

type Metric struct{ Name, Value string }
type page struct {
	ReportStyle                                                                 template.CSS
	View, Title, CSRF, RequestID, Error, Next, Signature, Details, InputDetails string
	Principal                                                                   tenancy.Principal
	Values                                                                      map[string]string
	Operations                                                                  []Operation
	Operation                                                                   Operation
	Stats                                                                       Stats
	Metrics                                                                     []Metric
	Headers                                                                     []string
	Rows                                                                        [][]string
	Sources                                                                     []SourceResult
}

var labels = map[string]string{"cnj": "Consulta processual", "judicial": "Liquidação judicial", "liquidacao": "Simulação por fator", "financiamento": "Revisão bancária", "prazos": "Prazos processuais", "indices": "Índices econômicos"}

func (s *Service) show(w http.ResponseWriter, r *http.Request, status int, v page) {
	v.Principal, _ = tenancy.FromContext(r.Context())
	v.CSRF = s.identity.WebCSRF(r)
	if v.RequestID == "" {
		v.RequestID = auth.RandomToken()
	}
	if v.Values == nil {
		v.Values = map[string]string{}
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	web.Workspace(w, v)
}
func (s *Service) failure(w http.ResponseWriter, err error) {
	status := 503
	message := "Não foi possível concluir a operação. Tente novamente."
	switch {
	case errors.Is(err, auth.ErrNotFound):
		status = 404
		message = "Registro não encontrado"
	case errors.Is(err, auth.ErrDenied):
		status = 403
		message = "Operação não permitida"
	case errors.Is(err, auth.ErrConflict):
		status = 409
		message = "Este envio já possui outro conteúdo. Abra novamente o formulário para criar uma operação."
	case errors.Is(err, auth.ErrQuota):
		status = 402
		message = "Franquia mensal esgotada"
	case errors.Is(err, auth.ErrRate):
		status = 429
		message = "Limite de operações atingido; aguarde um minuto"
		w.Header().Set("Retry-After", "60")
	}
	http.Error(w, message, status)
}
func (s *Service) Mount(r chi.Router) {
	guard := func(h http.HandlerFunc) http.HandlerFunc { return s.identity.Web("calculations:run", h) }
	r.Get("/app", guard(func(w http.ResponseWriter, r *http.Request) {
		p, _ := tenancy.FromContext(r.Context())
		stats, err := s.Stats(r.Context(), p)
		if err != nil {
			s.failure(w, err)
			return
		}
		ops, err := s.List(r.Context(), p, time.Time{})
		if err != nil {
			s.failure(w, err)
			return
		}
		if len(ops) > 5 {
			ops = ops[:5]
		}
		s.show(w, r, 200, page{View: "dashboard", Title: "Seu espaço jurídico", Stats: stats, Operations: ops})
	}))
	for kind, label := range labels {
		kind, label := kind, label
		r.Get("/app/"+kind, guard(func(w http.ResponseWriter, r *http.Request) {
			s.show(w, r, 200, page{View: kind, Title: label, Values: defaults(kind)})
		}))
		r.Post("/app/"+kind, guard(func(w http.ResponseWriter, r *http.Request) { s.submit(w, r, kind) }))
	}
	r.Post("/app/judicial/import", guard(func(w http.ResponseWriter, r *http.Request) { s.submit(w, r, "judicial") }))
	r.Get("/app/history", guard(func(w http.ResponseWriter, r *http.Request) {
		p, _ := tenancy.FromContext(r.Context())
		before := time.Time{}
		cursorID := "ffffffff-ffff-ffff-ffff-ffffffffffff"
		if value := r.URL.Query().Get("before"); value != "" {
			parts := strings.Split(value, "_")
			if len(parts) != 2 || !idPattern.MatchString(parts[1]) {
				http.Error(w, "Cursor inválido", 400)
				return
			}
			value = parts[0]
			cursorID = parts[1]
			var err error
			before, err = time.Parse(time.RFC3339Nano, value)
			if err != nil {
				http.Error(w, "Cursor inválido", 400)
				return
			}
		}
		ops, err := s.List(r.Context(), p, before, cursorID)
		if err != nil {
			s.failure(w, err)
			return
		}
		next := ""
		if len(ops) > 50 {
			next = ops[49].CreatedAt.Format(time.RFC3339Nano) + "_" + ops[49].ID
			ops = ops[:50]
		}
		s.show(w, r, 200, page{View: "history", Title: "Histórico da organização", Operations: ops, Next: next})
	}))
	r.Get("/app/history/{id}", guard(s.detail))
	r.Get("/app/history/{id}/recalculate", guard(func(w http.ResponseWriter, r *http.Request) {
		p, _ := tenancy.FromContext(r.Context())
		o, err := s.Get(r.Context(), p, chi.URLParam(r, "id"))
		if err != nil {
			s.failure(w, err)
			return
		}
		s.show(w, r, 200, page{View: o.Kind, Title: "Recalcular · " + labels[o.Kind], Values: valuesFrom(o)})
	}))
	r.Get("/app/history/{id}/download/{format}", guard(s.download))
	r.Post("/app/history/{id}/verify", guard(func(w http.ResponseWriter, r *http.Request) {
		if !s.identity.ValidateWebForm(w, r) {
			return
		}
		p, _ := tenancy.FromContext(r.Context())
		o, err := s.Get(r.Context(), p, chi.URLParam(r, "id"))
		if err != nil {
			s.failure(w, err)
			return
		}
		if !s.Verify(o, r.PostForm.Get("signature")) {
			http.Error(w, "Assinatura não corresponde ao registro completo", 422)
			return
		}
		s.show(w, r, 200, resultPage(o, "Integridade do registro confirmada pelo servidor", s.Integrity(o)))
	}))
}
func defaults(kind string) map[string]string {
	return map[string]string{"title": labels[kind], "principal": "1000.00", "fator_correcao": "1", "taxa_mensal": "1", "meses": "1", "pagamento": "0", "multa_percentual": "0", "honorarios_percentual": "0", "parcelas": "12", "taxa_mercado": "0.8", "sistema": "PRICE", "dias": "15", "modo": "UTEIS", "intimacao": "DIRETA", "data_evento": time.Now().Format("2006-01-02"), "data_atualizacao": time.Now().Format("2006-01-02"), "vencimento": time.Now().AddDate(0, -1, 0).Format("2006-01-02"), "inicio": time.Now().AddDate(0, -3, 0).Format("2006-01"), "fim": time.Now().AddDate(0, -1, 0).Format("2006-01"), "fonte": "BCB", "juros_tipo": "CONVENCIONAL_MENSAL", "capitalizacao": "SIMPLES"}
}
func (s *Service) submit(w http.ResponseWriter, r *http.Request, kind string) {
	if !s.identity.ValidateWebForm(w, r) {
		return
	}
	if r.MultipartForm != nil {
		defer r.MultipartForm.RemoveAll()
	}
	values := map[string]string{}
	for k, v := range r.PostForm {
		if len(v) > 0 {
			values[k] = v[0]
		}
	}
	raw, err := inputFrom(r, kind)
	if err == nil && kind == "judicial" && r.PostForm.Get("source_id") != "" {
		p, _ := tenancy.FromContext(r.Context())
		original, getErr := s.Get(r.Context(), p, r.PostForm.Get("source_id"))
		if getErr != nil {
			s.failure(w, getErr)
			return
		}
		if original.Kind != "judicial" {
			http.Error(w, "Origem inválida para recálculo", 422)
			return
		}
		var previous, current calculator.CalculoJudicialRequest
		if json.Unmarshal(original.Input, &previous) != nil || json.Unmarshal(raw, &current) != nil {
			s.failure(w, fmt.Errorf("registro inválido"))
			return
		}
		current.Parcelas = previous.Parcelas
		raw, err = json.Marshal(current)
	}
	if err != nil {
		s.show(w, r, 422, page{View: kind, Title: labels[kind], Error: err.Error(), Values: values, RequestID: r.PostForm.Get("request_id")})
		return
	}
	p, _ := tenancy.FromContext(r.Context())
	o, err := s.Run(r.Context(), p, kind, r.PostForm.Get("title"), r.PostForm.Get("request_id"), raw)
	if err != nil {
		if errors.Is(err, ErrInput) {
			s.show(w, r, 422, page{View: kind, Title: labels[kind], Error: err.Error(), Values: values, RequestID: r.PostForm.Get("request_id")})
		} else {
			s.failure(w, err)
		}
		return
	}
	http.Redirect(w, r, "/app/history/"+o.ID, 303)
}
func inputFrom(r *http.Request, kind string) ([]byte, error) {
	f := r.PostForm
	integer := func(k string) (int, error) {
		v, err := strconv.Atoi(f.Get(k))
		if err != nil {
			return 0, fmt.Errorf("%s deve ser inteiro", k)
		}
		return v, nil
	}
	dec := func(k string) (decimal.Decimal, error) { return calculator.ParseDecimal(f.Get(k)) }
	flag := func(k string) bool { return f.Get(k) == "on" }
	var value any
	switch kind {
	case "liquidacao":
		n, err := integer("meses")
		if err != nil {
			return nil, err
		}
		value = calculator.Request{Principal: f.Get("principal"), CorrectionFactor: f.Get("fator_correcao"), MonthlyRate: f.Get("taxa_mensal"), Months: n, Compound: flag("compostos"), Payment: f.Get("pagamento"), InterestFirst: flag("juros_primeiro"), PenaltyRate: f.Get("multa_percentual"), FeeRate: f.Get("honorarios_percentual")}
	case "financiamento":
		n, err := integer("parcelas")
		if err != nil {
			return nil, err
		}
		p, err := dec("principal")
		if err != nil {
			return nil, err
		}
		contract, err := dec("taxa_mensal")
		if err != nil {
			return nil, err
		}
		market, err := dec("taxa_mercado")
		if err != nil {
			return nil, err
		}
		value = calculator.CalculoFinanciamentoRequest{ValorFinanciado: p, NumeroParcelas: n, TaxaContratadaMensal: contract, TaxaMercadoMensal: market, SistemaAmortizacao: calculator.SistemaAmortizacao(f.Get("sistema"))}
	case "prazos":
		n, err := integer("dias")
		if err != nil {
			return nil, err
		}
		value = prazos.Request{Start: f.Get("data_evento"), Days: n, Mode: f.Get("modo"), Notice: f.Get("intimacao"), Double: flag("dobro"), Recess: flag("recesso"), Holidays: strings.Fields(strings.ReplaceAll(f.Get("feriados"), ",", " "))}
	case "cnj":
		value = Query{f.Get("numero"), flag("externas")}
	case "indices":
		value = IndexQuery{f.Get("fonte"), f.Get("inicio"), f.Get("fim")}
	case "judicial":
		rate, err := dec("taxa_mensal")
		if err != nil {
			return nil, err
		}
		v := calculator.CalculoJudicialRequest{NumeroProcesso: f.Get("numero"), DataAtualizacao: f.Get("data_atualizacao"), IndexadorCodigo: "SEM_CORRECAO", ImputarArt354CC: flag("juros_primeiro"), JurosMora: calculator.JurosMoraConfig{Tipo: calculator.TipoJuros(f.Get("juros_tipo")), PercentualMensal: rate, Modo: calculator.ModoCapitalizacao(f.Get("capitalizacao")), AplicarProRataDie: flag("prorata")}, FaseExecucao: calculator.FaseExecucaoConfig{AplicarArt523Multa: flag("art523_multa"), AplicarArt523Honorarios: flag("art523_honorarios")}}
		if r.URL.Path == "/app/judicial/import" {
			file, header, err := r.FormFile("file")
			if err != nil {
				return nil, fmt.Errorf("selecione arquivo CSV ou XLSX")
			}
			defer file.Close()
			if !strings.HasSuffix(strings.ToLower(header.Filename), ".csv") && !strings.HasSuffix(strings.ToLower(header.Filename), ".xlsx") {
				return nil, fmt.Errorf("somente CSV e XLSX")
			}
			b, err := io.ReadAll(io.LimitReader(file, (2<<20)+1))
			if err != nil {
				return nil, fmt.Errorf("falha ao ler arquivo")
			}
			v.Parcelas, err = calculator.ImportarPlanilha(header.Filename, b)
			if err != nil {
				return nil, err
			}
		} else if f.Get("source_id") != "" {
			// Resolve immutable source parcels through the tenant-scoped store in submit.
		} else if f.Get("itens_json") != "" {
			if err := decode([]byte(f.Get("itens_json")), &v.Parcelas); err != nil {
				return nil, err
			}
		} else {
			p, err := dec("principal")
			if err != nil {
				return nil, err
			}
			v.Parcelas = []calculator.ParcelaItem{{NumeroItem: 1, Descricao: f.Get("descricao"), Tipo: calculator.ParcelaDebito, DataVencimento: f.Get("vencimento"), ValorOriginal: p}}
		}
		value = v
	default:
		return nil, fmt.Errorf("operação inválida")
	}
	return json.Marshal(value)
}
func pretty(raw []byte) string {
	var v any
	d := json.NewDecoder(bytes.NewReader(raw))
	d.UseNumber()
	if d.Decode(&v) != nil {
		return ""
	}
	b, _ := json.MarshalIndent(v, "", "  ")
	return string(b)
}
func (s *Service) detail(w http.ResponseWriter, r *http.Request) {
	p, _ := tenancy.FromContext(r.Context())
	o, err := s.Get(r.Context(), p, chi.URLParam(r, "id"))
	if err != nil {
		s.failure(w, err)
		return
	}
	s.show(w, r, 200, resultPage(o, o.Title, s.Integrity(o)))
}
func resultPage(o Operation, title, signature string) page {
	v := page{View: "result", Title: title, Operation: o, Signature: signature, Details: pretty(o.Result), InputDetails: pretty(o.Input)}
	switch o.Kind {
	case "liquidacao":
		var result calculator.Result
		json.Unmarshal(o.Result, &result)
		v.Metrics = []Metric{{"Principal corrigido", result.Corrected}, {"Juros", result.Interest}, {"Total", result.Total}}
	case "judicial":
		var result calculator.CalculoJudicialResponse
		json.Unmarshal(o.Result, &result)
		v.Metrics = []Metric{{"Principal corrigido", result.Resumo.TotalPrincipalCorrigido.StringFixed(2)}, {"Juros", result.Resumo.TotalJuros.StringFixed(2)}, {"Total", result.Resumo.TotalGeralDevido.StringFixed(2)}}
		v.Headers = []string{"Item", "Descrição", "Vencimento", "Corrigido", "Juros", "Subtotal"}
		for _, it := range result.ItensLiquidados {
			v.Rows = append(v.Rows, []string{strconv.Itoa(it.NumeroItem), it.Descricao, it.DataVencimento, it.ValorCorrigido.StringFixed(2), it.ValorJurosMora.Add(it.ValorJurosCompensatorios).StringFixed(2), it.TotalItem.StringFixed(2)})
		}
	case "financiamento":
		var result calculator.CalculoFinanciamentoResponse
		json.Unmarshal(o.Result, &result)
		v.Metrics = []Metric{{"Total contratado", result.TotalPagoContratado.StringFixed(2)}, {"Total revisado", result.TotalDevidoRevisado.StringFixed(2)}, {"Diferença simples", result.DiferencaTotalSimples.StringFixed(2)}}
		v.Headers = []string{"Parcela", "Contratada", "Revisada", "Juros", "Amortização", "Saldo"}
		for _, it := range result.TabelaEvolucao {
			v.Rows = append(v.Rows, []string{strconv.Itoa(it.NumeroParcela), it.PrestacaoContratada.StringFixed(2), it.PrestacaoRevisada.StringFixed(2), it.JurosRevisado.StringFixed(2), it.AmortizacaoRevisada.StringFixed(2), it.SaldoDevedor.StringFixed(2)})
		}
	case "prazos":
		var result prazos.Result
		json.Unmarshal(o.Result, &result)
		v.Metrics = []Metric{{"Publicação / evento", result.Publication}, {"Vencimento", result.Due}}
		v.Headers = []string{"Data", "Contagem", "Motivo"}
		for _, d := range result.Calendar {
			v.Rows = append(v.Rows, []string{d.Date, strconv.Itoa(d.Count), d.Reason})
		}
	case "cnj":
		var result QueryResult
		json.Unmarshal(o.Result, &result)
		v.Metrics = []Metric{{"Número CNJ", result.Number.Formatted}, {"Ano", strconv.Itoa(result.Number.Year)}}
		v.Sources = result.Sources
	case "indices":
		var result IndexResult
		json.Unmarshal(o.Result, &result)
		v.Metrics = []Metric{{"Índice", result.Name}, {"Fonte", result.Source}}
		v.Headers = []string{"Mês", "Variação (%)"}
		for _, point := range result.Points {
			v.Rows = append(v.Rows, []string{point.Month, point.Percent})
		}
	}
	return v
}
func valuesFrom(o Operation) map[string]string {
	values := defaults(o.Kind)
	values["title"] = o.Title
	var fields map[string]json.RawMessage
	json.Unmarshal(o.Input, &fields)
	for k, b := range fields {
		var text string
		if json.Unmarshal(b, &text) == nil {
			values[k] = text
		} else {
			values[k] = string(b)
		}
	}
	switch o.Kind {
	case "cnj":
		values["externas"] = values["fontes_externas"]
	case "liquidacao":
		values["juros_primeiro"] = values["imputar_juros_primeiro"]
	case "financiamento":
		values["principal"] = values["valor_financiado"]
		values["taxa_mensal"] = values["taxa_contratada_mensal"]
		values["taxa_mercado"] = values["taxa_mercado_mensal"]
		values["parcelas"] = values["numero_parcelas"]
		values["sistema"] = values["sistema_amortizacao"]
	case "prazos":
		var r prazos.Request
		json.Unmarshal(o.Input, &r)
		values["feriados"] = strings.Join(r.Holidays, "\n")
		values["recesso"] = values["suspender_recesso"]
	case "judicial":
		var r calculator.CalculoJudicialRequest
		json.Unmarshal(o.Input, &r)
		values["numero"] = r.NumeroProcesso
		values["taxa_mensal"] = r.JurosMora.PercentualMensal.String()
		values["juros_tipo"] = string(r.JurosMora.Tipo)
		values["capitalizacao"] = string(r.JurosMora.Modo)
		values["prorata"] = strconv.FormatBool(r.JurosMora.AplicarProRataDie)
		values["juros_primeiro"] = strconv.FormatBool(r.ImputarArt354CC)
		values["art523_multa"] = strconv.FormatBool(r.FaseExecucao.AplicarArt523Multa)
		values["art523_honorarios"] = strconv.FormatBool(r.FaseExecucao.AplicarArt523Honorarios)

		values["source_id"] = o.ID
		values["source_count"] = strconv.Itoa(len(r.Parcelas))
	}
	return values
}
