package auth

import (
	"crypto/hmac"
	"errors"
	"fmt"
	"github.com/go-chi/chi/v5"
	"github.com/rdrgzma/robo-juridico-go-saas/internal/tenancy"
	"github.com/rdrgzma/robo-juridico-go-saas/web"
	"mime"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const sessionCookie = "rj_session"
const preauthCookie = "rj_preauth"

type view struct {
	View, Title, CSRF, Notice, Token, Secret, AuthenticatorURL, NewKey string
	Principal                                                          tenancy.Principal
	CanManage, CanKeys, MFADisabled                                    bool
	Members                                                            []Member
	Keys                                                               []Key
	Codes                                                              []string
	DevelopmentLogins                                                  []DevelopmentShortcut
}

func render(w http.ResponseWriter, status int, v view) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	web.Auth(w, v)
}
func (s *Service) cookie(w http.ResponseWriter, name, value string, duration time.Duration) {
	http.SetCookie(w, &http.Cookie{Name: name, Value: value, Path: "/", HttpOnly: true, Secure: s.options.CookieSecure, SameSite: http.SameSiteStrictMode, MaxAge: int(duration.Seconds()), Expires: s.now().Add(duration)})
}
func (s *Service) clearCookie(w http.ResponseWriter, name string) {
	http.SetCookie(w, &http.Cookie{Name: name, Path: "/", HttpOnly: true, Secure: s.options.CookieSecure, SameSite: http.SameSiteStrictMode, MaxAge: -1, Expires: time.Unix(1, 0)})
}
func readCookie(r *http.Request, name string) string {
	c, err := r.Cookie(name)
	if err != nil {
		return ""
	}
	return c.Value
}
func (s *Service) preauth(w http.ResponseWriter, r *http.Request) string {
	token := readCookie(r, preauthCookie)
	if s.validPreauth(token) {
		return token
	}
	value := strconv.FormatInt(s.now().Unix(), 10) + "." + RandomToken()
	token = value + "." + s.mac("preauth", value)
	s.cookie(w, preauthCookie, token, 20*time.Minute)
	return token
}
func (s *Service) validPreauth(token string) bool {
	p := strings.Split(token, ".")
	if len(p) != 3 || len(p[1]) != 43 {
		return false
	}
	unix, err := strconv.ParseInt(p[0], 10, 64)
	if err != nil || s.now().Sub(time.Unix(unix, 0)) > 20*time.Minute || time.Unix(unix, 0).After(s.now().Add(time.Minute)) {
		return false
	}
	return hmac.Equal([]byte(p[2]), []byte(s.mac("preauth", p[0]+"."+p[1])))
}
func (s *Service) form(w http.ResponseWriter, r *http.Request, token string, preauth bool) bool {
	origin, _ := url.Parse(s.options.Origin)
	// The embedded local browser uses an opaque Origin, but supplies Fetch Metadata.
	// Only an explicitly enabled loopback development setup accepts this case;
	// the signed preauth/session CSRF validation below remains mandatory.
	local := origin.Hostname() == "localhost" || (net.ParseIP(origin.Hostname()) != nil && net.ParseIP(origin.Hostname()).IsLoopback())
	embeddedDevelopment := len(s.options.DevelopmentLogins) > 0 && local && r.Header.Get("Origin") == "null" && r.Header.Get("Sec-Fetch-Site") == "same-origin"
	if (r.Header.Get("Origin") != s.options.Origin && !embeddedDevelopment) || r.Host != origin.Host {
		http.Error(w, "Origem inválida; use o endereço oficial do ambiente", 403)
		return false
	}
	content, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	multipartUpload := content == "multipart/form-data" && r.URL.Path == "/app/judicial/import"
	if err != nil || (content != "application/x-www-form-urlencoded" && !multipartUpload) {
		http.Error(w, "Formato de formulário inválido", 415)
		return false
	}
	limit := int64(16 << 10)
	if multipartUpload {
		limit = (2 << 20) + (32 << 10)
	}
	r.Body = http.MaxBytesReader(w, r.Body, limit)
	var parseErr error
	if multipartUpload {
		parseErr = r.ParseMultipartForm(3 << 20)
	} else {
		parseErr = r.ParseForm()
	}
	if parseErr != nil {
		http.Error(w, "Formulário inválido", 400)
		return false
	}
	if token == "" || (preauth && !s.validPreauth(token)) || !hmac.Equal([]byte(r.PostForm.Get("csrf")), []byte(s.CSRF(token))) {
		http.Error(w, "Token CSRF inválido", 403)
		return false
	}
	return true
}
func clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}
func (s *Service) protected(requireMFA bool, next func(http.ResponseWriter, *http.Request, tenancy.Principal, string)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		token := readCookie(r, sessionCookie)
		p, err := s.Session(r.Context(), token)
		if err != nil {
			if !errors.Is(err, ErrDenied) {
				http.Error(w, "Autenticação indisponível", 503)
				return
			}
			s.clearCookie(w, sessionCookie)
			http.Redirect(w, r, "/login", http.StatusSeeOther)
			return
		}
		if requireMFA && p.RequiresMFA() {
			http.Redirect(w, r, "/account/mfa", http.StatusSeeOther)
			return
		}
		next(w, r.WithContext(tenancy.WithPrincipal(r.Context(), p)), p, token)
	}
}
func statusFor(err error) int {
	switch {
	case errors.Is(err, ErrDenied):
		return 403
	case errors.Is(err, ErrNotFound):
		return 404
	case errors.Is(err, ErrRate):
		return 429
	case errors.Is(err, ErrConflict):
		return 409
	default:
		return 503
	}
}
func (s *Service) Mount(r chi.Router) {
	r.Get("/login", func(w http.ResponseWriter, r *http.Request) {
		csrf := s.CSRF(s.preauth(w, r))
		render(w, 200, view{View: "login", Title: "Entrar", CSRF: csrf, MFADisabled: s.options.DisableMFA, DevelopmentLogins: s.developmentShortcuts()})
	})
	login := func(w http.ResponseWriter, r *http.Request) {
		pre := readCookie(r, preauthCookie)
		if !s.form(w, r, pre, true) {
			return
		}
		organization, email, password := r.PostForm.Get("organization"), r.PostForm.Get("email"), r.PostForm.Get("password")
		if r.URL.Path == "/login/development" {
			credential, ok := s.options.DevelopmentLogins[r.PostForm.Get("profile")]
			if !ok {
				http.NotFound(w, r)
				return
			}
			organization, email, password = credential.Organization, credential.Email, credential.Password
		}
		token, err := s.Login(r.Context(), organization, email, password, clientIP(r))
		if err != nil {
			status := 401
			if errors.Is(err, ErrRate) {
				status = 429
				w.Header().Set("Retry-After", "900")
			} else if !errors.Is(err, ErrCredentials) {
				status = 503
			}
			render(w, status, view{View: "login", Title: "Entrar", CSRF: s.CSRF(pre), MFADisabled: s.options.DisableMFA, DevelopmentLogins: s.developmentShortcuts(), Notice: "Não foi possível entrar. Confira as credenciais ou aguarde antes de tentar novamente."})
			return
		}
		if old := readCookie(r, sessionCookie); old != "" {
			_ = s.Logout(r.Context(), old)
		}
		s.cookie(w, sessionCookie, token, 12*time.Hour)
		s.clearCookie(w, preauthCookie)
		http.Redirect(w, r, "/account", http.StatusSeeOther)
	}
	r.Post("/login", login)
	if len(s.options.DevelopmentLogins) > 0 {
		r.Post("/login/development", login)
	}
	r.Get("/forgot", func(w http.ResponseWriter, r *http.Request) {
		render(w, 200, view{View: "forgot", Title: "Recuperar acesso", CSRF: s.CSRF(s.preauth(w, r))})
	})
	r.Post("/forgot", func(w http.ResponseWriter, r *http.Request) {
		pre := readCookie(r, preauthCookie)
		if !s.form(w, r, pre, true) {
			return
		}
		err := s.ForgotPassword(r.Context(), stringsLower(r.PostForm.Get("organization")), r.PostForm.Get("email"), clientIP(r))
		if err != nil && !errors.Is(err, ErrRate) {
			http.Error(w, "Recuperação indisponível", 503)
			return
		}
		render(w, 202, view{View: "forgot", Title: "Recuperar acesso", CSRF: s.CSRF(pre), Notice: "Se os dados corresponderem a uma conta ativa, uma instrução de recuperação será preparada. Neste ambiente local, consulte a caixa de saída pelo CLI."})
	})
	r.Get("/reset", func(w http.ResponseWriter, r *http.Request) {
		render(w, 200, view{View: "reset", Title: "Nova senha", CSRF: s.CSRF(s.preauth(w, r)), Token: r.URL.Query().Get("token")})
	})
	r.Post("/reset", func(w http.ResponseWriter, r *http.Request) {
		pre := readCookie(r, preauthCookie)
		if !s.form(w, r, pre, true) {
			return
		}
		if err := s.ResetPassword(r.Context(), r.PostForm.Get("token"), r.PostForm.Get("password")); err != nil {
			render(w, 400, view{View: "reset", Title: "Nova senha", CSRF: s.CSRF(pre), Token: r.PostForm.Get("token"), Notice: "Token inválido/expirado ou senha fora dos limites (12 a 128 bytes)."})
			return
		}
		s.clearCookie(w, sessionCookie)
		s.clearCookie(w, preauthCookie)
		http.Redirect(w, r, "/login", 303)
	})
	r.Post("/logout", s.protected(false, func(w http.ResponseWriter, r *http.Request, p tenancy.Principal, token string) {
		if !s.form(w, r, token, false) {
			return
		}
		if err := s.Logout(r.Context(), token); err != nil {
			http.Error(w, "Falha ao encerrar sessão", 503)
			return
		}
		s.clearCookie(w, sessionCookie)
		http.Redirect(w, r, "/login", 303)
	}))
	r.Post("/account/sessions/revoke-all", s.protected(true, func(w http.ResponseWriter, r *http.Request, p tenancy.Principal, token string) {
		if !s.form(w, r, token, false) {
			return
		}
		if err := s.LogoutAll(r.Context(), p); err != nil {
			http.Error(w, "Falha ao revogar sessões", 503)
			return
		}
		s.clearCookie(w, sessionCookie)
		http.Redirect(w, r, "/login", 303)
	}))
	r.Get("/account", s.protected(true, func(w http.ResponseWriter, r *http.Request, p tenancy.Principal, token string) {
		render(w, 200, view{View: "account", Title: "Minha identidade", Principal: p, CSRF: s.CSRF(token)})
	}))
	r.Get("/portal", s.protected(true, func(w http.ResponseWriter, r *http.Request, p tenancy.Principal, token string) {
		render(w, 200, view{View: "account", Title: "Portal autenticado", Principal: p, CSRF: s.CSRF(token), Notice: "A identidade e o acesso estão ativos. As funções operacionais deste painel serão entregues nas sprints seguintes."})
	}))
	r.Get("/account/members", s.protected(true, func(w http.ResponseWriter, r *http.Request, p tenancy.Principal, token string) {
		members, err := s.Members(r.Context(), p)
		if err != nil {
			http.Error(w, "Acesso aos membros indisponível", statusFor(err))
			return
		}
		render(w, 200, view{View: "members", Title: "Membros da organização", Principal: p, CSRF: s.CSRF(token), Members: members, CanManage: p.Can("members:manage")})
	}))
	r.Post("/account/members/{id}/role", s.protected(true, func(w http.ResponseWriter, r *http.Request, p tenancy.Principal, token string) {
		if !s.form(w, r, token, false) {
			return
		}
		if err := s.SetRole(r.Context(), p, chi.URLParam(r, "id"), r.PostForm.Get("role")); err != nil {
			http.Error(w, "Não foi possível alterar o papel", statusFor(err))
			return
		}
		http.Redirect(w, r, "/account/members", 303)
	}))
	r.Get("/account/keys", s.protected(true, func(w http.ResponseWriter, r *http.Request, p tenancy.Principal, token string) {
		keys, err := s.Keys(r.Context(), p)
		if err != nil {
			http.Error(w, "Acesso às chaves indisponível", statusFor(err))
			return
		}
		render(w, 200, view{View: "keys", Title: "Chaves da organização", Principal: p, CSRF: s.CSRF(token), Keys: keys})
	}))
	r.Post("/account/keys", s.protected(true, func(w http.ResponseWriter, r *http.Request, p tenancy.Principal, token string) {
		if !s.form(w, r, token, false) {
			return
		}
		_, raw, err := s.IssueKey(r.Context(), p, r.PostForm.Get("name"), r.PostForm["scope"])
		if err != nil {
			http.Error(w, "Não foi possível emitir a chave", statusFor(err))
			return
		}
		keys, err := s.Keys(r.Context(), p)
		if err != nil {
			http.Error(w, "Chave emitida; falha ao carregar lista", 503)
			return
		}
		render(w, 201, view{View: "keys", Title: "Chave emitida", Principal: p, CSRF: s.CSRF(token), Keys: keys, NewKey: raw, Notice: "Guarde a chave agora. Ela não será exibida novamente."})
	}))
	r.Post("/account/keys/{id}/rotate", s.protected(true, func(w http.ResponseWriter, r *http.Request, p tenancy.Principal, token string) {
		if !s.form(w, r, token, false) {
			return
		}
		_, raw, err := s.RotateKey(r.Context(), p, chi.URLParam(r, "id"))
		if err != nil {
			http.Error(w, "Não foi possível rotacionar a chave", statusFor(err))
			return
		}
		keys, err := s.Keys(r.Context(), p)
		if err != nil {
			http.Error(w, "Chave rotacionada; falha ao carregar lista", 503)
			return
		}
		render(w, 201, view{View: "keys", Title: "Chave rotacionada", Principal: p, CSRF: s.CSRF(token), Keys: keys, NewKey: raw, Notice: "A chave anterior foi revogada. Guarde a nova chave agora; ela não será exibida novamente."})
	}))
	r.Post("/account/keys/{id}/revoke", s.protected(true, func(w http.ResponseWriter, r *http.Request, p tenancy.Principal, token string) {
		if !s.form(w, r, token, false) {
			return
		}
		if err := s.RevokeKey(r.Context(), p, chi.URLParam(r, "id")); err != nil {
			http.Error(w, "Não foi possível revogar a chave", statusFor(err))
			return
		}
		http.Redirect(w, r, "/account/keys", 303)
	}))
	s.mountMFA(r)
}
func (s *Service) mountMFA(r chi.Router) {
	r.Get("/account/mfa", s.protected(false, func(w http.ResponseWriter, r *http.Request, p tenancy.Principal, token string) {
		render(w, 200, view{View: "mfa", Title: "Autenticação multifator", Principal: p, CSRF: s.CSRF(token)})
	}))
	r.Post("/account/mfa/setup", s.protected(false, func(w http.ResponseWriter, r *http.Request, p tenancy.Principal, token string) {
		if !s.form(w, r, token, false) {
			return
		}
		secret, err := s.MFASetup(r.Context(), p, token)
		if err != nil {
			http.Error(w, "Reentre com sua senha antes de configurar MFA", statusFor(err))
			return
		}
		uri := "otpauth://totp/" + url.PathEscape("RoboJuridico:"+p.Email) + "?secret=" + secret + "&issuer=RoboJuridico&algorithm=SHA1&digits=6&period=30"
		render(w, 200, view{View: "mfa", Title: "Configurar autenticador", Principal: p, CSRF: s.CSRF(token), Secret: secret, AuthenticatorURL: uri})
	}))
	r.Post("/account/mfa/verify", s.protected(false, func(w http.ResponseWriter, r *http.Request, p tenancy.Principal, token string) {
		if !s.form(w, r, token, false) {
			return
		}
		newToken, codes, err := s.VerifyMFA(r.Context(), p, token, strings.TrimSpace(r.PostForm.Get("code")))
		if err != nil {
			render(w, statusFor(err), view{View: "mfa", Title: "Verificar MFA", Principal: p, CSRF: s.CSRF(token), Notice: "Código inválido, já utilizado ou limite de tentativas atingido."})
			return
		}
		s.cookie(w, sessionCookie, newToken, 12*time.Hour)
		if len(codes) > 0 {
			p, err = s.Session(r.Context(), newToken)
			if err != nil {
				http.Error(w, "Falha ao carregar sessão", 503)
				return
			}
			render(w, 200, view{View: "recovery", Title: "Guarde os códigos de recuperação", Principal: p, CSRF: s.CSRF(newToken), Codes: codes})
			return
		}
		http.Redirect(w, r, "/account", 303)
	}))
}

func (s *Service) APIError(err error) int {
	switch {
	case errors.Is(err, ErrCredentials):
		return 401
	case errors.Is(err, ErrDenied):
		return 403
	case errors.Is(err, ErrRate):
		return 429
	case errors.Is(err, ErrQuota):
		return 402
	default:
		return 503
	}
}
func (s *Service) String() string { return fmt.Sprintf("identity service (%s)", s.options.Origin) }

// Web authenticates the session and enforces the current organization permission.
func (s *Service) Web(permission string, next http.HandlerFunc) http.HandlerFunc {
	return s.protected(true, func(w http.ResponseWriter, r *http.Request, p tenancy.Principal, token string) {
		if !p.Can(permission) {
			http.Error(w, "Sem permissão para esta operação", 403)
			return
		}
		next(w, r)
	})
}

// WebAdmin authenticates the session and enforces global platform_admin access and active MFA policy.
func (s *Service) WebAdmin(next http.HandlerFunc) http.HandlerFunc {
	return s.protected(true, func(w http.ResponseWriter, r *http.Request, p tenancy.Principal, token string) {
		if !p.PlatformAdmin || p.RequiresMFA() {
			http.Error(w, "Acesso administrativo requer permissão global e MFA", 403)
			return
		}
		next(w, r)
	})
}

// ValidateWebForm must be called for every workspace or admin mutation, after Web/WebAdmin.
func (s *Service) ValidateWebForm(w http.ResponseWriter, r *http.Request) bool {
	return s.form(w, r, readCookie(r, sessionCookie), false)
}
func (s *Service) WebCSRF(r *http.Request) string { return s.CSRF(readCookie(r, sessionCookie)) }
