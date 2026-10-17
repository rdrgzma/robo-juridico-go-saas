package juridico

import (
	"context"
	"io"
	"net/http"
	"os"
	"strings"
	"sync/atomic"
	"testing"
)

type roundTrip func(*http.Request) (*http.Response, error)

func (f roundTrip) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func response(status int, body string) *http.Response {
	return &http.Response{StatusCode: status, Body: io.NopCloser(strings.NewReader(body))}
}

func TestSourcesIsolationStatusAndBounds(t *testing.T) {
	const number = "00008323520184013202"
	s := NewSources("synthetic-test-key", true)
	var calls atomic.Int32
	s.client.Transport = roundTrip(func(r *http.Request) (*http.Response, error) {
		calls.Add(1)
		if r.URL.Host == "api-publica.datajud.cnj.jus.br" {
			if r.Method != "POST" || r.URL.Path != "/api_publica_trf1/_search" || r.Header.Get("Authorization") != "APIKey synthetic-test-key" {
				t.Error("wrong DataJud contract")
			}
			return response(200, `{"hits":{"hits":[{"_source":{"numeroProcesso":"`+number+`","movimentos":[]}}]}}`), nil
		}
		if r.URL.Host != "comunicaapi.pje.jus.br" || r.URL.Query().Get("numeroProcesso") != number {
			t.Error("wrong DJEN contract")
		}
		return response(200, `{"count":1,"items":[{"numero_processo":"`+number+`","texto":"<script>synthetic</script>"}]}`), nil
	})
	for i, org := range []string{"org-a", "org-a", "org-b"} {
		out, err := s.Query(context.Background(), org, Query{number, true})
		if err != nil || len(out.Sources) != 2 {
			t.Fatal(err)
		}
		if out.Cached != (i == 1) {
			t.Fatal("tenant cache scope")
		}
		for _, v := range out.Sources {
			if v.Status != "ok" || v.Total != 1 {
				t.Fatal(v)
			}
		}
	}
	if calls.Load() != 4 {
		t.Fatal(calls.Load())
	}
	s.client.Transport = roundTrip(func(*http.Request) (*http.Response, error) { return response(404, `{}`), nil })
	out, err := s.Query(context.Background(), "org-failure", Query{number, true})
	if err != nil {
		t.Fatal(err)
	}
	for _, v := range out.Sources {
		if v.Status != "indisponivel" {
			t.Fatal("404 converted into no results")
		}
	}
	s.client.Transport = roundTrip(func(*http.Request) (*http.Response, error) { return response(200, strings.Repeat("x", (2<<20)+1)), nil })
	if err = s.fetch(context.Background(), "GET", "https://api.bcb.gov.br/", nil, "", new(any)); err == nil {
		t.Fatal("oversized response accepted")
	}
	s.client.Transport = roundTrip(func(*http.Request) (*http.Response, error) {
		return response(200, `{"hits":{"hits":[{"_source":{"numeroProcesso":"00000000000000000000"}}]}}`), nil
	})
	n, _ := s.Query(context.Background(), "mismatch", Query{number, true})
	if n.Sources[0].Status != "indisponivel" {
		t.Fatal("cross-process response accepted")
	}
}

func TestIndexSourcesRejectGapsAndMissingValues(t *testing.T) {
	s := NewSources("", false)
	cases := []struct{ source, body string }{
		{"BCB", `[{"data":"01/01/2024","valor":"0.42"},{"data":"01/02/2024","valor":"0.83"}]`},
		{"IBGE", `[{"id":"63","resultados":[{"series":[{"localidade":{"id":"1"},"serie":{"202401":"0.42","202402":"0.83"}}]}]}]`},
	}
	for _, tc := range cases {
		s.client.Transport = roundTrip(func(*http.Request) (*http.Response, error) { return response(200, tc.body), nil })
		out, err := s.Index(context.Background(), IndexQuery{tc.source, "2024-01", "2024-02"})
		if err != nil || len(out.Points) != 2 || out.Points[0].Percent != "0.42" {
			t.Fatal(out, err)
		}
	}
	for _, body := range []string{`[]`, `[{"data":"01/01/2024","valor":"-"}]`, `[{"data":"01/01/2024","valor":"0.4"},{"data":"01/01/2024","valor":"0.4"}]`} {
		s.client.Transport = roundTrip(func(*http.Request) (*http.Response, error) { return response(200, body), nil })
		if _, err := s.Index(context.Background(), IndexQuery{"BCB", "2024-01", "2024-02"}); err == nil {
			t.Fatal("incomplete/invalid series accepted")
		}
	}
	if _, err := s.Index(context.Background(), IndexQuery{"BCB", "2023-01", "2024-01"}); err == nil {
		t.Fatal("13-month query accepted")
	}
}

func TestOfficialIndexSmoke(t *testing.T) {
	if os.Getenv("RJ_EXTERNAL_SMOKE") != "1" {
		t.Skip("opt-in external source check")
	}
	s := NewSources("", false)
	for _, source := range []string{"BCB", "IBGE"} {
		out, err := s.Index(context.Background(), IndexQuery{source, "2024-01", "2024-02"})
		if err != nil || len(out.Points) != 2 {
			t.Fatal(source, err)
		}
		if out.Points[0].Percent != "0.42" || out.Points[1].Percent != "0.83" {
			t.Fatal("official fixture changed; investigate source revision", source, out.Points)
		}
	}
}

func TestDataJudLiveSmoke(t *testing.T) {
	key := os.Getenv("DATAJUD_API_KEY")
	if key == "" || os.Getenv("RJ_EXTERNAL_SMOKE") != "1" {
		t.Skip("opt-in live DataJud check")
	}
	s := NewSources(key, false)
	out, err := s.Query(context.Background(), "smoke-org", Query{Number: "5053283-30.2024.8.13.0079", External: true})
	if err != nil {
		t.Fatalf("query failed: %v", err)
	}
	for _, src := range out.Sources {
		if src.Name == "CNJ / DataJud" {
			t.Logf("DataJud live response: status=%s total=%d notice=%s", src.Status, src.Total, src.Notice)
			if src.Status != "ok" {
				t.Fatalf("DataJud query returned non-ok status: %s (notice: %s)", src.Status, src.Notice)
			}
		}
	}
}

