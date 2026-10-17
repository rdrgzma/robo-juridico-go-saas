package portal

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/rdrgzma/robo-juridico-go-saas/internal/auth"
	"github.com/rdrgzma/robo-juridico-go-saas/internal/juridico"
	"github.com/rdrgzma/robo-juridico-go-saas/internal/tenancy"
	"github.com/rdrgzma/robo-juridico-go-saas/web"
)

type page struct {
	View, Title, CSRF, Notice, Error, NewKey, SourceFilter, StateFilter, KindFilter, NextCursor string
	Principal                                                                                   tenancy.Principal
	CanManageMembers, CanReadMembers, CanManageKeys, CanRunCalculations                         bool
	Dashboard                                                                                   DashboardView
	Usage                                                                                       []UsageEntry
	KeysData                                                                                    PortalKeysData
	Operations                                                                                  []juridico.Operation
	Operation                                                                                   juridico.Operation
	Signature, InputJSON, ResultJSON                                                            string
	Members                                                                                     []auth.Member
	Sessions                                                                                    []auth.SessionView
}

func prettyJSON(raw []byte) string {
	var v any
	d := json.NewDecoder(bytes.NewReader(raw))
	d.UseNumber()
	if d.Decode(&v) != nil {
		return string(raw)
	}
	b, _ := json.MarshalIndent(v, "", "  ")
	return string(b)
}

func (s *Service) show(w http.ResponseWriter, r *http.Request, status int, v page) {
	v.Principal, _ = tenancy.FromContext(r.Context())
	v.CSRF = s.identity.WebCSRF(r)
	v.CanManageMembers = v.Principal.Can("members:manage")
	v.CanReadMembers = v.Principal.Can("members:read")
	v.CanManageKeys = v.Principal.Can("keys:manage")
	v.CanRunCalculations = v.Principal.Can("calculations:run")
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	web.Portal(w, v)
}

func (s *Service) failure(w http.ResponseWriter, err error) {
	status := 503
	msg := "Operação do portal indisponível"
	switch {
	case errors.Is(err, auth.ErrDenied):
		status = 403
		msg = "Sem permissão para esta operação no papel ou plano atual"
	case errors.Is(err, auth.ErrNotFound):
		status = 404
		msg = "Registro não encontrado nesta organização"
	case errors.Is(err, auth.ErrConflict), errors.Is(err, juridico.ErrInput):
		status = 409
		msg = "Operação não permitida ou dados inválidos para as regras atuais"
	case errors.Is(err, auth.ErrCredentials):
		status = 401
		msg = "Credenciais atuais inválidas"
	case errors.Is(err, auth.ErrQuota):
		status = 402
		msg = "Franquia mensal esgotada"
	case errors.Is(err, auth.ErrRate):
		status = 429
		msg = "Muitas tentativas; aguarde"
	}
	http.Error(w, msg, status)
}

func (s *Service) Mount(r chi.Router) {
	readGuard := func(h http.HandlerFunc) http.HandlerFunc { return s.identity.Web("account:read", h) }
	keyGuard := func(h http.HandlerFunc) http.HandlerFunc { return s.identity.Web("keys:manage", h) }
	memberGuard := func(h http.HandlerFunc) http.HandlerFunc { return s.identity.Web("members:manage", h) }
	calcGuard := func(h http.HandlerFunc) http.HandlerFunc { return s.identity.Web("calculations:run", h) }

	r.Get("/portal", readGuard(func(w http.ResponseWriter, r *http.Request) {
		p, _ := tenancy.FromContext(r.Context())
		d, err := s.Dashboard(r.Context(), p)
		if err != nil {
			s.failure(w, err)
			return
		}
		ops, _, err := s.ListOperations(r.Context(), p, "", time.Time{}, "")
		if err != nil {
			s.failure(w, err)
			return
		}
		if len(ops) > 5 {
			ops = ops[:5]
		}
		s.show(w, r, 200, page{View: "dashboard", Title: "Portal do cliente", Dashboard: d, Operations: ops})
	}))

	r.Get("/portal/usage", readGuard(func(w http.ResponseWriter, r *http.Request) {
		p, _ := tenancy.FromContext(r.Context())
		sourceFilter := r.URL.Query().Get("source")
		stateFilter := r.URL.Query().Get("state")
		var before time.Time
		if b := r.URL.Query().Get("before"); b != "" {
			var err error
			before, err = time.Parse(time.RFC3339Nano, b)
			if err != nil {
				http.Error(w, "Cursor de utilização inválido", 400)
				return
			}
		}
		entries, next, err := s.UsageHistory(r.Context(), p, sourceFilter, stateFilter, before)
		if err != nil {
			s.failure(w, err)
			return
		}
		d, err := s.Dashboard(r.Context(), p)
		if err != nil {
			s.failure(w, err)
			return
		}
		s.show(w, r, 200, page{
			View: "usage", Title: "Histórico de utilização e consumo",
			Dashboard: d, Usage: entries, NextCursor: next,
			SourceFilter: sourceFilter, StateFilter: stateFilter,
		})
	}))

	r.Get("/portal/keys", readGuard(func(w http.ResponseWriter, r *http.Request) {
		p, _ := tenancy.FromContext(r.Context())
		kd, err := s.KeysData(r.Context(), p)
		if err != nil {
			s.failure(w, err)
			return
		}
		s.show(w, r, 200, page{View: "keys", Title: "Chaves de API e documentação", KeysData: kd})
	}))

	r.Post("/portal/keys", keyGuard(func(w http.ResponseWriter, r *http.Request) {
		if !s.identity.ValidateWebForm(w, r) {
			return
		}
		p, _ := tenancy.FromContext(r.Context())
		_, raw, err := s.IssueKey(r.Context(), p, r.PostForm.Get("name"), r.PostForm["scope"])
		if err != nil {
			s.failure(w, err)
			return
		}
		kd, err := s.KeysData(r.Context(), p)
		if err != nil {
			s.failure(w, err)
			return
		}
		s.show(w, r, 201, page{
			View: "keys", Title: "Chave de API emitida",
			KeysData: kd, NewKey: raw,
			Notice: "Guarde a chave agora. Por segurança, o segredo bruto não é armazenado e não será exibido novamente.",
		})
	}))

	r.Post("/portal/keys/{id}/rotate", keyGuard(func(w http.ResponseWriter, r *http.Request) {
		if !s.identity.ValidateWebForm(w, r) {
			return
		}
		p, _ := tenancy.FromContext(r.Context())
		_, raw, err := s.RotateKey(r.Context(), p, chi.URLParam(r, "id"))
		if err != nil {
			s.failure(w, err)
			return
		}
		kd, err := s.KeysData(r.Context(), p)
		if err != nil {
			s.failure(w, err)
			return
		}
		s.show(w, r, 201, page{
			View: "keys", Title: "Chave de API rotacionada",
			KeysData: kd, NewKey: raw,
			Notice: "A chave anterior foi revogada imediatamente. Guarde a nova chave agora; ela não será exibida novamente.",
		})
	}))

	r.Post("/portal/keys/{id}/revoke", keyGuard(func(w http.ResponseWriter, r *http.Request) {
		if !s.identity.ValidateWebForm(w, r) {
			return
		}
		p, _ := tenancy.FromContext(r.Context())
		if err := s.RevokeKey(r.Context(), p, chi.URLParam(r, "id")); err != nil {
			s.failure(w, err)
			return
		}
		http.Redirect(w, r, "/portal/keys", 303)
	}))

	r.Get("/portal/history", readGuard(func(w http.ResponseWriter, r *http.Request) {
		p, _ := tenancy.FromContext(r.Context())
		kindFilter := r.URL.Query().Get("kind")
		var before time.Time
		cursorID := ""
		if b := r.URL.Query().Get("before"); b != "" {
			parts := strings.Split(b, "_")
			if len(parts) != 2 || !idPattern.MatchString(parts[1]) {
				http.Error(w, "Cursor de histórico inválido", 400)
				return
			}
			var err error
			before, err = time.Parse(time.RFC3339Nano, parts[0])
			if err != nil {
				http.Error(w, "Cursor de histórico inválido", 400)
				return
			}
			cursorID = parts[1]
		}
		ops, next, err := s.ListOperations(r.Context(), p, kindFilter, before, cursorID)
		if err != nil {
			s.failure(w, err)
			return
		}
		s.show(w, r, 200, page{
			View: "history", Title: "Histórico jurídico e documentos",
			Operations: ops, KindFilter: kindFilter, NextCursor: next,
		})
	}))

	r.Get("/portal/history/{id}", readGuard(func(w http.ResponseWriter, r *http.Request) {
		p, _ := tenancy.FromContext(r.Context())
		op, sig, err := s.GetOperation(r.Context(), p, chi.URLParam(r, "id"))
		if err != nil {
			s.failure(w, err)
			return
		}
		s.show(w, r, 200, page{
			View: "operation_detail", Title: "Memória · " + op.Title,
			Operation: op, Signature: sig,
			InputJSON: prettyJSON(op.Input), ResultJSON: prettyJSON(op.Result),
		})
	}))

	r.Get("/portal/history/{id}/recalculate", calcGuard(func(w http.ResponseWriter, r *http.Request) {
		p, _ := tenancy.FromContext(r.Context())
		op, _, err := s.GetOperation(r.Context(), p, chi.URLParam(r, "id"))
		if err != nil {
			s.failure(w, err)
			return
		}
		http.Redirect(w, r, "/app/history/"+op.ID+"/recalculate", 303)
	}))

	r.Get("/portal/history/{id}/download/{format}", readGuard(func(w http.ResponseWriter, r *http.Request) {
		if s.workspace == nil {
			http.NotFound(w, r)
			return
		}
		s.workspace.ServeDownload(w, r)
	}))

	r.Get("/portal/team", readGuard(func(w http.ResponseWriter, r *http.Request) {
		p, _ := tenancy.FromContext(r.Context())
		d, err := s.Dashboard(r.Context(), p)
		if err != nil {
			s.failure(w, err)
			return
		}
		var members []auth.Member
		if p.Can("members:read") {
			members, err = s.identity.Members(r.Context(), p)
			if err != nil {
				s.failure(w, err)
				return
			}
		}
		s.show(w, r, 200, page{
			View: "team", Title: "Organização e equipe",
			Dashboard: d, Members: members, Notice: r.URL.Query().Get("notice"),
		})
	}))

	r.Post("/portal/organization", memberGuard(func(w http.ResponseWriter, r *http.Request) {
		if !s.identity.ValidateWebForm(w, r) {
			return
		}
		p, _ := tenancy.FromContext(r.Context())
		if err := s.UpdateOrganizationSettings(r.Context(), p, OrganizationSettingsInput{
			Name:         r.PostForm.Get("name"),
			LegalName:    r.PostForm.Get("legal_name"),
			Document:     r.PostForm.Get("document"),
			ContactName:  r.PostForm.Get("contact_name"),
			ContactEmail: r.PostForm.Get("contact_email"),
		}); err != nil {
			s.failure(w, err)
			return
		}
		http.Redirect(w, r, "/portal/team?notice=Dados+da+organiza%C3%A7%C3%A3o+atualizados", 303)
	}))

	r.Post("/portal/members", memberGuard(func(w http.ResponseWriter, r *http.Request) {
		if !s.identity.ValidateWebForm(w, r) {
			return
		}
		p, _ := tenancy.FromContext(r.Context())
		if _, err := s.identity.ProvisionMember(r.Context(), p, r.PostForm.Get("email"), r.PostForm.Get("password"), r.PostForm.Get("role")); err != nil {
			s.failure(w, err)
			return
		}
		http.Redirect(w, r, "/portal/team?notice=Membro+adicionado+ou+atualizado", 303)
	}))

	r.Post("/portal/members/{id}/role", memberGuard(func(w http.ResponseWriter, r *http.Request) {
		if !s.identity.ValidateWebForm(w, r) {
			return
		}
		p, _ := tenancy.FromContext(r.Context())
		if err := s.identity.SetRole(r.Context(), p, chi.URLParam(r, "id"), r.PostForm.Get("role")); err != nil {
			s.failure(w, err)
			return
		}
		http.Redirect(w, r, "/portal/team?notice=Papel+atualizado", 303)
	}))

	r.Post("/portal/members/{id}/status", memberGuard(func(w http.ResponseWriter, r *http.Request) {
		if !s.identity.ValidateWebForm(w, r) {
			return
		}
		p, _ := tenancy.FromContext(r.Context())
		active := r.PostForm.Get("active") == "true"
		if err := s.identity.SetMemberStatus(r.Context(), p, chi.URLParam(r, "id"), active); err != nil {
			s.failure(w, err)
			return
		}
		http.Redirect(w, r, "/portal/team?notice=Estado+do+membro+atualizado", 303)
	}))

	r.Get("/portal/security", readGuard(func(w http.ResponseWriter, r *http.Request) {
		p, _ := tenancy.FromContext(r.Context())
		sessions, err := s.identity.UserSessions(r.Context(), p, s.identity.WebSessionToken(r))
		if err != nil {
			s.failure(w, err)
			return
		}
		s.show(w, r, 200, page{
			View: "security", Title: "Conta, sessões e segurança",
			Sessions: sessions, Notice: r.URL.Query().Get("notice"),
		})
	}))

	r.Post("/portal/security/password", readGuard(func(w http.ResponseWriter, r *http.Request) {
		if !s.identity.ValidateWebForm(w, r) {
			return
		}
		p, _ := tenancy.FromContext(r.Context())
		newToken, err := s.identity.ChangePassword(r.Context(), p, r.PostForm.Get("current_password"), r.PostForm.Get("new_password"))
		if err != nil {
			sessions, _ := s.identity.UserSessions(r.Context(), p, s.identity.WebSessionToken(r))
			status := 400
			errMsg := "Não foi possível alterar a senha. Verifique a senha atual e informe nova senha entre 12 e 128 bytes."
			if errors.Is(err, auth.ErrCredentials) {
				status = 401
			}
			s.show(w, r, status, page{
				View: "security", Title: "Conta, sessões e segurança",
				Sessions: sessions, Error: errMsg,
			})
			return
		}
		s.identity.SetSessionCookie(w, newToken)
		http.Redirect(w, r, "/portal/security?notice=Senha+alterada+com+sucesso+e+demais+sess%C3%B5es+revogadas", 303)
	}))

	r.Post("/portal/security/sessions/{id}/revoke", readGuard(func(w http.ResponseWriter, r *http.Request) {
		if !s.identity.ValidateWebForm(w, r) {
			return
		}
		p, _ := tenancy.FromContext(r.Context())
		if err := s.identity.RevokeSession(r.Context(), p, chi.URLParam(r, "id")); err != nil {
			s.failure(w, err)
			return
		}
		http.Redirect(w, r, "/portal/security?notice=Sess%C3%A3o+revogada", 303)
	}))

	r.Post("/portal/security/sessions/revoke-all", readGuard(func(w http.ResponseWriter, r *http.Request) {
		if !s.identity.ValidateWebForm(w, r) {
			return
		}
		p, _ := tenancy.FromContext(r.Context())
		if err := s.identity.LogoutAll(r.Context(), p); err != nil {
			s.failure(w, err)
			return
		}
		http.Redirect(w, r, "/login", 303)
	}))
}
