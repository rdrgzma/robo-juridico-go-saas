package transport

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"mime"
	"net/http"
	"reflect"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/rdrgzma/robo-juridico-go-saas/internal/auth"
	"github.com/rdrgzma/robo-juridico-go-saas/internal/calculator"
	"github.com/rdrgzma/robo-juridico-go-saas/internal/cnj"
	"github.com/rdrgzma/robo-juridico-go-saas/internal/juridico"
	"github.com/rdrgzma/robo-juridico-go-saas/internal/prazos"
	"github.com/rdrgzma/robo-juridico-go-saas/internal/tenancy"
	"github.com/rdrgzma/robo-juridico-go-saas/web"
	"github.com/shopspring/decimal"
)

type gate struct {
	mu    sync.Mutex
	start time.Time
	used  int
}

func (g *gate) allow() bool {
	g.mu.Lock()
	defer g.mu.Unlock()
	if time.Since(g.start) >= time.Minute {
		g.start = time.Now()
		g.used = 0
	}
	if g.used >= 60 {
		return false
	}
	g.used++
	return true
}

func New(keyHash [32]byte, logger *slog.Logger, readiness ...func(context.Context) error) http.Handler {
	return newRouter(keyHash, logger, nil, nil, readiness...)
}
func NewWithIdentity(keyHash [32]byte, logger *slog.Logger, identity *auth.Service, readiness ...func(context.Context) error) http.Handler {
	return newRouter(keyHash, logger, identity, nil, readiness...)
}
func NewWithWorkspace(keyHash [32]byte, logger *slog.Logger, identity *auth.Service, workspace *juridico.Service, readiness ...func(context.Context) error) http.Handler {
	return newRouter(keyHash, logger, identity, workspace, readiness...)
}
func newRouter(keyHash [32]byte, logger *slog.Logger, identity *auth.Service, workspace *juridico.Service, readiness ...func(context.Context) error) http.Handler {
	started := time.Now().UTC().Format(time.RFC3339Nano)
	r := chi.NewRouter()
	limiter := &gate{}
	r.Use(middleware.RequestID, middleware.Recoverer)
	r.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			start := time.Now()
			w.Header().Set("X-Request-ID", middleware.GetReqID(req.Context()))
			w.Header().Set("X-Content-Type-Options", "nosniff")
			w.Header().Set("X-Frame-Options", "DENY")
			w.Header().Set("Referrer-Policy", "no-referrer")
			w.Header().Set("Cache-Control", "no-store")
			w.Header().Set("Content-Security-Policy", "default-src 'self'; style-src 'self'; frame-ancestors 'none'; base-uri 'none'; form-action 'self'")
			ww := middleware.NewWrapResponseWriter(w, req.ProtoMajor)
			next.ServeHTTP(ww, req)
			// No URL, query string, body, IP or credentials in logs.
			logger.Info("http", "request_id", middleware.GetReqID(req.Context()), "method", req.Method, "status", ww.Status(), "duration_ms", time.Since(start).Milliseconds())
		})
	})
	r.NotFound(func(w http.ResponseWriter, r *http.Request) {
		fail(w, r, 404, "rota_inexistente", "Rota não encontrada")
	})
	r.MethodNotAllowed(func(w http.ResponseWriter, r *http.Request) {
		fail(w, r, 405, "metodo_nao_permitido", "Método não permitido")
	})
	r.Get("/", web.Index)
	r.Get("/static/style.css", web.Style)
	r.Get("/static/app.css", web.AppStyle)
	r.Get("/static/app.js", web.AppScript)
	r.Get("/saude", func(w http.ResponseWriter, r *http.Request) {
		reply(w, 200, map[string]any{"status": "ok", "sprint": 3, "production_ready": false})
	})
	r.Get("/openapi.json", web.OpenAPI)
	r.Get("/docs", web.Docs)
	r.Get("/health/live", func(w http.ResponseWriter, r *http.Request) {
		reply(w, 200, map[string]string{"status": "alive", "started_at": started})
	})
	r.Get("/health/ready", func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
		defer cancel()
		if len(readiness) == 0 || readiness[0](ctx) != nil {
			fail(w, r, 503, "nao_pronto", "Banco indisponível ou migrations pendentes")
			return
		}
		reply(w, 200, map[string]string{"status": "ready"})
	})
	engine := calculator.NovoEngineCalculo(nil)
	if identity != nil {
		identity.Mount(r)
		if workspace != nil {
			workspace.Mount(r)
		} else {
			r.Get("/app", identity.Web("calculations:run", func(w http.ResponseWriter, r *http.Request) {
				web.Auth(w, map[string]any{"View": "account", "Title": "Área jurídica autenticada"})
			}))
		}
	} else {
		for _, path := range []string{"/app", "/admin", "/portal"} {
			r.HandleFunc(path, func(w http.ResponseWriter, r *http.Request) {
				fail(w, r, 503, "modulo_pendente", "Painel aguarda autenticação e isolamento multiempresa nas próximas sprints")
			})
		}
	}
	for _, prefix := range []string{"/v1", "/api/v1"} {
		r.Route(prefix, func(api chi.Router) {
			api.Use(apiAuthentication(keyHash, logger, identity, limiter))
			api.Get("/me", func(w http.ResponseWriter, r *http.Request) {
				p, ok := tenancy.FromContext(r.Context())
				if !ok {
					fail(w, r, 403, "organizacao_obrigatoria", "Use uma chave vinculada à organização")
					return
				}
				reply(w, 200, p)
			})
			api.Get("/cnj/{numero}", func(w http.ResponseWriter, r *http.Request) {
				n, err := cnj.Parse(chi.URLParam(r, "numero"))
				if err != nil {
					fail(w, r, 422, "cnj_invalido", err.Error())
					return
				}
				reply(w, 200, n)
			})
			// Different paths intentionally avoid claiming compatibility with the richer legacy payloads.
			api.Get("/", func(w http.ResponseWriter, r *http.Request) {
				reply(w, 200, map[string]string{"version": "v1", "docs": "/docs"})
			})
			api.Post("/simulacoes/liquidacao", handle(calculator.Calculate))
			api.Post("/simulacoes/financiamento", handle(calculator.Finance))
			api.Post("/simulacoes/prazos", handle(prazos.Calculate))
			api.Post("/calculos/financiamento", handle(calculator.CalcularRevisaoFinanciamento))
			api.Post("/calculos/judicial", handle(func(input calculator.CalculoJudicialRequest) (*calculator.CalculoJudicialResponse, error) {
				if input.JurosMora.Tipo == calculator.JurosTaxaLegal || input.JurosMora.Tipo == calculator.JurosLei14905 || input.JurosMora.Tipo == calculator.JurosSelic {
					return nil, fmt.Errorf("regime legal depende de séries oficiais e homologação; não há fallback automático")
				}
				return engine.Executar(input)
			}))
			api.Get("/calculos/indices", func(w http.ResponseWriter, r *http.Request) {
				reply(w, 200, map[string]any{"indices": engine.Repositorio().ListarIndices()})
			})
		})
	}
	return r
}

func handle[I any, O any](calculate func(I) (O, error)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ct, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
		if err != nil || ct != "application/json" {
			fail(w, r, 415, "tipo_invalido", "Use application/json")
			return
		}
		r.Body = http.MaxBytesReader(w, r.Body, 64<<10)
		decoder := json.NewDecoder(r.Body)
		decoder.DisallowUnknownFields()
		var input I
		if err := decoder.Decode(&input); err != nil {
			fail(w, r, 400, "json_invalido", "JSON inválido, campos desconhecidos ou corpo acima de 64 KiB")
			return
		}
		if err := decoder.Decode(new(any)); err != io.EOF {
			fail(w, r, 400, "json_invalido", "Envie um único objeto JSON")
			return
		}
		if err := boundedDecimals(reflect.ValueOf(input)); err != nil {
			fail(w, r, 422, "decimal_invalido", err.Error())
			return
		}
		out, err := calculate(input)
		if err != nil {
			fail(w, r, 422, "entrada_invalida", err.Error())
			return
		}
		reply(w, 200, out)
	}
}

func reply(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func boundedDecimals(value reflect.Value) error {
	if !value.IsValid() {
		return nil
	}
	if value.Type() == reflect.TypeOf(decimal.Decimal{}) {
		return calculator.ValidateDecimal(value.Interface().(decimal.Decimal))
	}
	switch value.Kind() {
	case reflect.Struct:
		for i := 0; i < value.NumField(); i++ {
			if err := boundedDecimals(value.Field(i)); err != nil {
				return err
			}
		}
	case reflect.Slice:
		for i := 0; i < value.Len(); i++ {
			if err := boundedDecimals(value.Index(i)); err != nil {
				return err
			}
		}
	}
	return nil
}
func fail(w http.ResponseWriter, r *http.Request, status int, code, message string) {
	reply(w, status, map[string]any{"erro": map[string]string{"codigo": code, "mensagem": message}, "request_id": middleware.GetReqID(r.Context())})
}
