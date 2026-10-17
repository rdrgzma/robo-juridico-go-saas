package juridico

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/rdrgzma/robo-juridico-go-saas/internal/calculator"
	"github.com/rdrgzma/robo-juridico-go-saas/internal/cnj"
	"github.com/shopspring/decimal"
)

type Query struct {
	Number   string `json:"numero"`
	External bool   `json:"fontes_externas"`
}
type SourceResult struct {
	Name        string            `json:"fonte"`
	URL         string            `json:"url"`
	Status      string            `json:"estado"`
	Notice      string            `json:"aviso,omitempty"`
	CollectedAt time.Time         `json:"consultado_em"`
	Total       int               `json:"total"`
	Records     []json.RawMessage `json:"registros,omitempty"`
}
type QueryResult struct {
	Number  cnj.Number     `json:"cnj"`
	Sources []SourceResult `json:"fontes"`
	Cached  bool           `json:"cache"`
}
type cachedQuery struct {
	value   QueryResult
	expires time.Time
}
type Sources struct {
	client     *http.Client
	datajudKey string
	djen       bool
	mu         sync.Mutex
	cache      map[string]cachedQuery
}

func NewSources(datajudKey string, djen bool) *Sources {
	return NewSourcesWithClient(&http.Client{Timeout: 15 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}, datajudKey, djen)
}
func NewSourcesWithClient(client *http.Client, datajudKey string, djen bool) *Sources {
	if client == nil {
		client = &http.Client{Timeout: 15 * time.Second}
	}
	return &Sources{client: client, datajudKey: datajudKey, djen: djen, cache: map[string]cachedQuery{}}
}
func (s *Sources) Status() (datajudConfigured, djenEnabled bool) {
	if s == nil {
		return false, false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return strings.TrimSpace(s.datajudKey) != "", s.djen
}

func (s *Sources) Config() (datajudKey string, djenEnabled bool) {
	if s == nil {
		return "", false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.datajudKey, s.djen
}

func (s *Sources) UpdateConfig(datajudKey string, djen bool) {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.datajudKey = strings.TrimSpace(datajudKey)
	s.djen = djen
	s.cache = map[string]cachedQuery{}
}
func (s *Sources) fetch(ctx context.Context, method, address string, body []byte, key string, target any) error {
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, method, address, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("requisição inválida")
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "RoboJuridico-Go-SaaS/0.3")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if key != "" {
		trimmedKey := strings.TrimSpace(key)
		trimmedKey = strings.TrimPrefix(trimmedKey, "APIKey ")
		req.Header.Set("Authorization", "APIKey "+trimmedKey)
	}
	res, err := s.client.Do(req)
	if err != nil {
		return fmt.Errorf("fonte indisponível ou tempo limite atingido")
	}
	defer res.Body.Close()
	if res.StatusCode != 200 {
		return fmt.Errorf("fonte retornou HTTP %d", res.StatusCode)
	}
	b, err := io.ReadAll(io.LimitReader(res.Body, (2<<20)+1))
	if err != nil || len(b) > 2<<20 {
		return fmt.Errorf("resposta excede limite ou está incompleta")
	}
	if err = json.Unmarshal(b, target); err != nil {
		return fmt.Errorf("contrato da fonte inválido")
	}
	return nil
}

// Unsupported branches are reported rather than guessed.
func court(n cnj.Number) string {
	if n.Segment == 4 && n.CourtCode >= 1 && n.CourtCode <= 6 {
		return fmt.Sprintf("trf%d", n.CourtCode)
	}
	if n.Segment == 5 && n.CourtCode >= 1 && n.CourtCode <= 24 {
		return fmt.Sprintf("trt%d", n.CourtCode)
	}
	states := []string{"", "tjac", "tjal", "tjap", "tjam", "tjba", "tjce", "tjdft", "tjes", "tjgo", "tjma", "tjmt", "tjms", "tjmg", "tjpa", "tjpb", "tjpr", "tjpe", "tjpi", "tjrj", "tjrn", "tjrs", "tjro", "tjrr", "tjsc", "tjse", "tjsp", "tjto"}
	if n.Segment == 8 && n.CourtCode > 0 && n.CourtCode < len(states) {
		return states[n.CourtCode]
	}
	return ""
}
func (s *Sources) Query(ctx context.Context, organization string, q Query) (QueryResult, error) {
	n, err := cnj.Parse(q.Number)
	if err != nil {
		return QueryResult{}, err
	}
	out := QueryResult{Number: n, Sources: []SourceResult{}}
	if !q.External {
		return out, nil
	}
	cacheKey := organization + ":" + n.Digits
	s.mu.Lock()
	cached, ok := s.cache[cacheKey]
	s.mu.Unlock()
	if ok && cached.expires.After(time.Now()) {
		out = cached.value
		out.Cached = true
		return out, nil
	}
	var wg sync.WaitGroup
	results := make([]SourceResult, 2)
	wg.Add(2)
	go func() { defer wg.Done(); results[0] = s.datajud(ctx, n) }()
	go func() { defer wg.Done(); results[1] = s.communications(ctx, n) }()
	wg.Wait()
	out.Sources = results
	cacheable := true
	for _, v := range results {
		if v.Status == "indisponivel" {
			cacheable = false
		}
	}
	if cacheable {
		s.mu.Lock()
		if len(s.cache) >= 200 {
			s.cache = map[string]cachedQuery{}
		}
		s.cache[cacheKey] = cachedQuery{out, time.Now().Add(5 * time.Minute)}
		s.mu.Unlock()
	}
	return out, nil
}
func (s *Sources) datajud(ctx context.Context, n cnj.Number) SourceResult {
	v := SourceResult{Name: "CNJ / DataJud", URL: "https://www.cnj.jus.br/sistemas/datajud/api-publica/", CollectedAt: time.Now().UTC(), Status: "nao_configurado"}
	s.mu.Lock()
	key := s.datajudKey
	s.mu.Unlock()
	if key == "" {
		v.Notice = "Configure DATAJUD_API_KEY no painel ou ambiente para consultar esta fonte."
		return v
	}
	alias := court(n)
	if alias == "" {
		v.Status = "nao_suportado"
		v.Notice = "Este ramo/tribunal ainda não possui alias homologado."
		return v
	}
	v.URL = "https://api-publica.datajud.cnj.jus.br/api_publica_" + alias + "/_search"
	b, _ := json.Marshal(map[string]any{"size": 10, "query": map[string]any{"match": map[string]string{"numeroProcesso": n.Digits}}})
	var data struct {
		Hits *struct {
			Hits []struct {
				Source json.RawMessage `json:"_source"`
			} `json:"hits"`
		} `json:"hits"`
	}
	if err := s.fetch(ctx, "POST", v.URL, b, key, &data); err != nil {
		v.Status = "indisponivel"
		v.Notice = err.Error()
		return v
	}
	if data.Hits == nil || len(data.Hits.Hits) > 10 {
		v.Status = "indisponivel"
		v.Notice = "Contrato hits inválido."
		return v
	}
	v.Status = "ok"
	v.Total = len(data.Hits.Hits)
	for _, hit := range data.Hits.Hits {
		var record struct {
			Number string `json:"numeroProcesso"`
		}
		if json.Unmarshal(hit.Source, &record) != nil || record.Number != n.Digits {
			v.Status = "indisponivel"
			v.Notice = "Fonte retornou processo divergente."
			v.Records = nil
			return v
		}
		v.Records = append(v.Records, hit.Source)
	}
	if len(data.Hits.Hits) >= 10 {
		v.Notice = "Exibindo até dez registros/graus. Consulte a fonte para cobertura integral."
	}
	return v
}
func (s *Sources) communications(ctx context.Context, n cnj.Number) SourceResult {
	v := SourceResult{Name: "CNJ / DJEN-Comunica", URL: "https://comunica.pje.jus.br/", CollectedAt: time.Now().UTC(), Status: "nao_configurado"}
	s.mu.Lock()
	djenEnabled := s.djen
	s.mu.Unlock()
	if !djenEnabled {
		v.Notice = "Configure DJEN_ENABLED=true no painel ou ambiente para habilitar esta fonte pública."
		return v
	}
	values := url.Values{"numeroProcesso": {n.Digits}, "tamanhoPagina": {"20"}, "pagina": {"1"}}
	address := "https://comunicaapi.pje.jus.br/api/v1/comunicacao?" + values.Encode()
	var data struct {
		Count *int              `json:"count"`
		Items []json.RawMessage `json:"items"`
	}
	if err := s.fetch(ctx, "GET", address, nil, "", &data); err != nil {
		v.Status = "indisponivel"
		v.Notice = err.Error()
		return v
	}
	if data.Count == nil || *data.Count < 0 || len(data.Items) > 20 {
		v.Status = "indisponivel"
		v.Notice = "Contrato da fonte inválido."
		return v
	}
	for _, item := range data.Items {
		var record struct {
			Number      string `json:"numero_processo"`
			NumberCamel string `json:"numeroProcesso"`
		}
		if json.Unmarshal(item, &record) != nil {
			v.Status = "indisponivel"
			return v
		}
		number := record.Number
		if number == "" {
			number = record.NumberCamel
		}
		number = strings.NewReplacer("-", "", ".", "").Replace(number)
		if number != n.Digits {
			v.Status = "indisponivel"
			v.Notice = "Publicação sem correspondência ao processo."
			return v
		}
	}
	v.Status = "ok"
	v.Total = *data.Count
	v.Records = data.Items
	if v.Total > len(v.Records) {
		v.Notice = "Primeira página, até vinte publicações. Consulte a fonte para cobertura integral."
	}
	return v
}

type IndexQuery struct {
	Source string `json:"fonte"`
	Start  string `json:"inicio"`
	End    string `json:"fim"`
}
type IndexPoint struct {
	Month   string `json:"mes"`
	Percent string `json:"variacao_percentual"`
}
type IndexResult struct {
	Name        string       `json:"nome"`
	Source      string       `json:"fonte"`
	URL         string       `json:"url"`
	CollectedAt time.Time    `json:"coletado_em"`
	Points      []IndexPoint `json:"pontos"`
	Notice      string       `json:"aviso"`
}

func (s *Sources) Index(ctx context.Context, q IndexQuery) (IndexResult, error) {
	out := IndexResult{Name: "IPCA — variação mensal", Source: q.Source, CollectedAt: time.Now().UTC(), Notice: "Série informativa da fonte oficial; não aplicada automaticamente aos cálculos. Lacunas não são substituídas por zero."}
	start, e := time.Parse("2006-01", q.Start)
	end, e2 := time.Parse("2006-01", q.End)
	months := (end.Year()-start.Year())*12 + int(end.Month()-start.Month()) + 1
	if e != nil || e2 != nil || start.After(end) || start.Year() < 1980 || end.After(time.Now()) || months > 12 {
		return out, fmt.Errorf("informe intervalo mensal de até doze meses, sem datas futuras")
	}
	values := map[string]string{}
	switch q.Source {
	case "BCB":
		out.URL = "https://api.bcb.gov.br/dados/serie/bcdata.sgs.433/dados?" + url.Values{"formato": {"json"}, "dataInicial": {start.Format("02/01/2006")}, "dataFinal": {end.AddDate(0, 1, -1).Format("02/01/2006")}}.Encode()
		var data []struct {
			Date  string `json:"data"`
			Value string `json:"valor"`
		}
		if err := s.fetch(ctx, "GET", out.URL, nil, "", &data); err != nil {
			return out, err
		}
		for _, v := range data {
			d, err := time.Parse("02/01/2006", v.Date)
			if err != nil {
				return out, fmt.Errorf("data inválida na fonte")
			}
			month := d.Format("2006-01")
			if _, ok := values[month]; ok {
				return out, fmt.Errorf("mês duplicado na fonte")
			}
			values[month] = v.Value
		}
	case "IBGE":
		periods := []string{}
		for d := start; !d.After(end); d = d.AddDate(0, 1, 0) {
			periods = append(periods, d.Format("200601"))
		}
		out.URL = "https://servicodados.ibge.gov.br/api/v3/agregados/1737/periodos/" + strings.Join(periods, "|") + "/variaveis/63?localidades=N1%5Ball%5D"
		var data []struct {
			ID      string `json:"id"`
			Results []struct {
				Series []struct {
					Local struct {
						ID string `json:"id"`
					} `json:"localidade"`
					Values map[string]string `json:"serie"`
				} `json:"series"`
			} `json:"resultados"`
		}
		if err := s.fetch(ctx, "GET", out.URL, nil, "", &data); err != nil {
			return out, err
		}
		for _, v := range data {
			if v.ID != "63" {
				continue
			}
			for _, r := range v.Results {
				for _, series := range r.Series {
					if series.Local.ID != "1" {
						continue
					}
					for period, value := range series.Values {
						d, err := time.Parse("200601", period)
						if err != nil {
							return out, fmt.Errorf("período inválido na fonte")
						}
						month := d.Format("2006-01")
						if _, ok := values[month]; ok {
							return out, fmt.Errorf("mês duplicado na fonte")
						}
						values[month] = value
					}
				}
			}
		}
	default:
		return out, fmt.Errorf("fonte deve ser BCB ou IBGE")
	}
	for month, value := range values {
		d, _ := time.Parse("2006-01", month)
		if d.Before(start) || d.After(end) {
			return out, fmt.Errorf("fonte retornou período fora do intervalo")
		}
		v, err := calculator.ParseDecimal(strings.ReplaceAll(value, ",", "."))
		if err != nil || v.LessThanOrEqual(decimal.NewFromInt(-100)) || v.GreaterThan(decimal.NewFromInt(1000)) {
			return out, fmt.Errorf("valor ausente ou inválido na fonte")
		}
		out.Points = append(out.Points, IndexPoint{month, v.String()})
	}
	sort.Slice(out.Points, func(i, j int) bool { return out.Points[i].Month < out.Points[j].Month })
	if len(out.Points) != months {
		return out, fmt.Errorf("fonte retornou série incompleta: %d de %d meses", len(out.Points), months)
	}
	return out, nil
}
