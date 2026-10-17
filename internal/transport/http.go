package transport

import (
	"crypto/sha256"
	"crypto/subtle"
	"encoding/json"
	"io"
	"log/slog"
	"mime"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/rdrgzma/robo-juridico-go-saas/internal/calculator"
	"github.com/rdrgzma/robo-juridico-go-saas/internal/cnj"
	"github.com/rdrgzma/robo-juridico-go-saas/internal/prazos"
	"github.com/rdrgzma/robo-juridico-go-saas/web"
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

func New(keyHash [32]byte, logger *slog.Logger) http.Handler {
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
	r.Get("/saude", func(w http.ResponseWriter, r *http.Request) {
		reply(w, 200, map[string]any{"status": "ok", "sprint": 1, "production_ready": false})
	})
	r.Get("/openapi.json", web.OpenAPI)
	for _, path := range []string{"/app", "/admin", "/portal"} {
		r.HandleFunc(path, func(w http.ResponseWriter, r *http.Request) {
			fail(w, r, 503, "modulo_pendente", "Painel aguarda autenticação e isolamento multiempresa nas próximas sprints")
		})
	}
	for _, prefix := range []string{"/v1", "/api/v1"} {
		r.Route(prefix, func(api chi.Router) {
			api.Use(func(next http.Handler) http.Handler {
				return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					if !limiter.allow() {
						w.Header().Set("Retry-After", "60")
						fail(w, r, 429, "limite_excedido", "Limite local de 60 requisições por minuto")
						return
					}
					key := r.Header.Get("X-API-Key")
					if key == "" {
						key = strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
					}
					h := sha256.Sum256([]byte(key))
					zero := [32]byte{}
					if key == "" || len(key) > 200 || keyHash == zero || subtle.ConstantTimeCompare(h[:], keyHash[:]) != 1 {
						fail(w, r, 401, "nao_autorizado", "Chave de API inválida")
						return
					}
					next.ServeHTTP(w, r)
				})
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
			api.Post("/simulacoes/liquidacao", handle(calculator.Calculate))
			api.Post("/simulacoes/financiamento", handle(calculator.Finance))
			api.Post("/simulacoes/prazos", handle(prazos.Calculate))
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
func fail(w http.ResponseWriter, r *http.Request, status int, code, message string) {
	reply(w, status, map[string]any{"erro": map[string]string{"codigo": code, "mensagem": message}, "request_id": middleware.GetReqID(r.Context())})
}
