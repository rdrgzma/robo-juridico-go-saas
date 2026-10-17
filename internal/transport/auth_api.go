package transport

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/rdrgzma/robo-juridico-go-saas/internal/auth"
	"github.com/rdrgzma/robo-juridico-go-saas/internal/tenancy"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"time"
)

func apiAuthentication(keyHash [32]byte, logger *slog.Logger, identity *auth.Service, limiter *gate) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			key := r.Header.Get("X-API-Key")
			if key == "" {
				key = strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
			}
			hash := sha256.Sum256([]byte(key))
			zero := [32]byte{}
			if key != "" && len(key) <= 200 && keyHash != zero && subtle.ConstantTimeCompare(hash[:], keyHash[:]) == 1 {
				if !limiter.allow() {
					w.Header().Set("Retry-After", "60")
					fail(w, r, 429, "limite_excedido", "Limite local de 60 requisições por minuto")
					return
				}
				// Development compatibility key has no tenant and cannot enter private routes.
				if !statelessPath(r.URL.Path) {
					fail(w, r, 403, "organizacao_obrigatoria", "Use uma chave vinculada à organização")
					return
				}
				next.ServeHTTP(w, r)
				return
			}
			if identity == nil {
				if !limiter.allow() {
					w.Header().Set("Retry-After", "60")
					fail(w, r, 429, "limite_excedido", "Aguarde antes de tentar novamente")
					return
				}
				fail(w, r, 401, "nao_autorizado", "Chave de API inválida")
				return
			}
			ip, _, err := net.SplitHostPort(r.RemoteAddr)
			if err != nil {
				ip = r.RemoteAddr
			}
			scope, billable := auth.APIRequirement(r.URL.Path, r.Method)
			access, err := identity.BeginAPI(r.Context(), key, scope, ip, billable, auth.APIModule(r.URL.Path))
			if err != nil {
				status := identity.APIError(err)
				if status == 429 {
					w.Header().Set("Retry-After", "60")
				}
				fail(w, r, status, "acesso_api_recusado", "Chave inválida, sem permissão ou limite de uso atingido")
				return
			}
			req := r.WithContext(tenancy.WithPrincipal(r.Context(), access.Principal))
			rec := middleware.NewWrapResponseWriter(w, r.ProtoMajor)
			status := 500
			defer func() {
				ctx, cancel := context.WithTimeout(context.WithoutCancel(r.Context()), 5*time.Second)
				defer cancel()
				if err := identity.FinishAPI(ctx, access.EventID, status); err != nil {
					logger.Error("usage finalization pending", "request_id", middleware.GetReqID(r.Context()))
				}
			}()
			next.ServeHTTP(rec, req)
			status = rec.Status()
			if status == 0 {
				status = 200
			}
		})
	}
}
func statelessPath(path string) bool {
	if path == "/v1" || path == "/v1/" || path == "/api/v1" || path == "/api/v1/" || strings.Contains(path, "/cnj/") {
		return true
	}
	for _, suffix := range []string{"/simulacoes/liquidacao", "/simulacoes/financiamento", "/simulacoes/prazos", "/calculos/financiamento", "/calculos/judicial", "/calculos/indices"} {
		if strings.HasSuffix(path, suffix) {
			return true
		}
	}
	return false
}
