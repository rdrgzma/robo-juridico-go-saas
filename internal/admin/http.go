package admin

import (
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/rdrgzma/robo-juridico-go-saas/internal/auth"
	"github.com/rdrgzma/robo-juridico-go-saas/internal/juridico"
	"github.com/rdrgzma/robo-juridico-go-saas/internal/tenancy"
	"github.com/rdrgzma/robo-juridico-go-saas/web"
)

type page struct {
	View, Title, CSRF, Notice, Error, NewKey, Query, StatusFilter, PlanFilter, OrgFilter, KindFilter, ActionFilter string
	NextAuditID                                                                                                    int64
	Principal                                                                                                      tenancy.Principal
	Dashboard                                                                                                      DashboardData
	Organizations                                                                                                  []OrganizationSummary
	Detail                                                                                                         OrganizationDetail
	Plans                                                                                                          []Plan
	AllowedFeatures                                                                                                []string
	Users                                                                                                          []UserAdminView
	Quotas                                                                                                         []QuotaSummaryView
	Keys                                                                                                           []AdminKeyView
	Operations                                                                                                     []AdminOperationView
	AuditEvents                                                                                                    []AuditEventView
	Indices                                                                                                        []EconomicIndexView
	SyncRuns                                                                                                       []IndexSyncRun
	DefaultSyncStart, DefaultSyncEnd, Today                                                                        string
}

func (s *Service) show(w http.ResponseWriter, r *http.Request, status int, v page) {
	v.Principal, _ = tenancy.FromContext(r.Context())
	v.CSRF = s.identity.WebCSRF(r)
	v.AllowedFeatures = AllowedFeatures
	v.Today = time.Now().Format("2006-01-02")
	v.DefaultSyncStart = time.Now().AddDate(0, -2, 0).Format("2006-01")
	v.DefaultSyncEnd = time.Now().AddDate(0, -1, 0).Format("2006-01")
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	web.Admin(w, v)
}

func (s *Service) failure(w http.ResponseWriter, err error) {
	status := 503
	msg := "Operação administrativa indisponível"
	switch {
	case errors.Is(err, auth.ErrDenied):
		status = 403
		msg = "Acesso administrativo negado"
	case errors.Is(err, auth.ErrNotFound):
		status = 404
		msg = "Registro não encontrado"
	case errors.Is(err, auth.ErrConflict), errors.Is(err, juridico.ErrInput):
		status = 409
		msg = "Operação não permitida ou dados inválidos para as regras atuais"
	case errors.Is(err, auth.ErrRate):
		status = 429
		msg = "Muitas tentativas; aguarde"
	}
	http.Error(w, msg, status)
}

func (s *Service) Mount(r chi.Router) {
	guard := s.identity.WebAdmin

	r.Get("/admin", guard(func(w http.ResponseWriter, r *http.Request) {
		p, _ := tenancy.FromContext(r.Context())
		d, err := s.Dashboard(r.Context(), p)
		if err != nil {
			s.failure(w, err)
			return
		}
		s.show(w, r, 200, page{View: "dashboard", Title: "Painel administrativo", Dashboard: d})
	}))

	r.Get("/admin/organizations", guard(func(w http.ResponseWriter, r *http.Request) {
		p, _ := tenancy.FromContext(r.Context())
		q := r.URL.Query().Get("q")
		statusFilter := r.URL.Query().Get("status")
		planFilter := r.URL.Query().Get("plan")
		orgs, err := s.ListOrganizations(r.Context(), p, q, statusFilter, planFilter)
		if err != nil {
			s.failure(w, err)
			return
		}
		plans, err := s.ListPlans(r.Context(), p)
		if err != nil {
			s.failure(w, err)
			return
		}
		s.show(w, r, 200, page{
			View: "organizations", Title: "Clientes e organizações",
			Organizations: orgs, Plans: plans,
			Query: q, StatusFilter: statusFilter, PlanFilter: planFilter,
		})
	}))

	r.Post("/admin/organizations", guard(func(w http.ResponseWriter, r *http.Request) {
		if !s.identity.ValidateWebForm(w, r) {
			return
		}
		p, _ := tenancy.FromContext(r.Context())
		limit, _ := strconv.Atoi(r.PostForm.Get("monthly_limit"))
		rpm, _ := strconv.Atoi(r.PostForm.Get("requests_per_minute"))
		id, err := s.CreateClientOrganization(r.Context(), p, CreateOrganizationInput{
			Slug:              r.PostForm.Get("slug"),
			Name:              r.PostForm.Get("name"),
			LegalName:         r.PostForm.Get("legal_name"),
			Document:          r.PostForm.Get("document"),
			ContactName:       r.PostForm.Get("contact_name"),
			ContactEmail:      r.PostForm.Get("contact_email"),
			Notes:             r.PostForm.Get("notes"),
			OwnerEmail:        r.PostForm.Get("owner_email"),
			OwnerPassword:     r.PostForm.Get("owner_password"),
			PlanCode:          r.PostForm.Get("plan_code"),
			ContractedPrice:   r.PostForm.Get("contracted_price"),
			MonthlyLimit:      limit,
			RequestsPerMinute: rpm,
			EffectiveFrom:     r.PostForm.Get("effective_from"),
			EffectiveUntil:    r.PostForm.Get("effective_until"),
			ContractNotes:     r.PostForm.Get("contract_notes"),
		})
		if err != nil {
			s.failure(w, err)
			return
		}
		http.Redirect(w, r, "/admin/organizations/"+id, 303)
	}))

	r.Get("/admin/organizations/{id}", guard(func(w http.ResponseWriter, r *http.Request) {
		p, _ := tenancy.FromContext(r.Context())
		detail, err := s.GetOrganization(r.Context(), p, chi.URLParam(r, "id"))
		if err != nil {
			s.failure(w, err)
			return
		}
		plans, err := s.ListPlans(r.Context(), p)
		if err != nil {
			s.failure(w, err)
			return
		}
		s.show(w, r, 200, page{View: "organization_detail", Title: "Organização · " + detail.Organization.Name, Detail: detail, Plans: plans})
	}))

	r.Post("/admin/organizations/{id}", guard(func(w http.ResponseWriter, r *http.Request) {
		if !s.identity.ValidateWebForm(w, r) {
			return
		}
		p, _ := tenancy.FromContext(r.Context())
		id := chi.URLParam(r, "id")
		if err := s.UpdateOrganization(r.Context(), p, id, UpdateOrganizationInput{
			Name:         r.PostForm.Get("name"),
			LegalName:    r.PostForm.Get("legal_name"),
			Document:     r.PostForm.Get("document"),
			ContactName:  r.PostForm.Get("contact_name"),
			ContactEmail: r.PostForm.Get("contact_email"),
			Notes:        r.PostForm.Get("notes"),
		}); err != nil {
			s.failure(w, err)
			return
		}
		http.Redirect(w, r, "/admin/organizations/"+id, 303)
	}))

	r.Post("/admin/organizations/{id}/status", guard(func(w http.ResponseWriter, r *http.Request) {
		if !s.identity.ValidateWebForm(w, r) {
			return
		}
		p, _ := tenancy.FromContext(r.Context())
		id := chi.URLParam(r, "id")
		active := r.PostForm.Get("active") == "true"
		if err := s.SetOrganizationStatus(r.Context(), p, id, active); err != nil {
			s.failure(w, err)
			return
		}
		http.Redirect(w, r, "/admin/organizations/"+id, 303)
	}))

	r.Post("/admin/organizations/{id}/contract", guard(func(w http.ResponseWriter, r *http.Request) {
		if !s.identity.ValidateWebForm(w, r) {
			return
		}
		p, _ := tenancy.FromContext(r.Context())
		id := chi.URLParam(r, "id")
		limit, err1 := strconv.Atoi(r.PostForm.Get("monthly_limit"))
		rpm, err2 := strconv.Atoi(r.PostForm.Get("requests_per_minute"))
		if err1 != nil || err2 != nil {
			s.failure(w, auth.ErrConflict)
			return
		}
		if err := s.UpdateContract(r.Context(), p, id, ContractInput{
			PlanCode:          r.PostForm.Get("plan_code"),
			ContractedPrice:   r.PostForm.Get("contracted_price"),
			MonthlyLimit:      limit,
			RequestsPerMinute: rpm,
			EffectiveFrom:     r.PostForm.Get("effective_from"),
			EffectiveUntil:    r.PostForm.Get("effective_until"),
			Notes:             r.PostForm.Get("notes"),
		}); err != nil {
			s.failure(w, err)
			return
		}
		http.Redirect(w, r, "/admin/organizations/"+id, 303)
	}))

	r.Get("/admin/plans", guard(func(w http.ResponseWriter, r *http.Request) {
		p, _ := tenancy.FromContext(r.Context())
		plans, err := s.ListPlans(r.Context(), p)
		if err != nil {
			s.failure(w, err)
			return
		}
		s.show(w, r, 200, page{View: "plans", Title: "Planos e autorização de recursos", Plans: plans})
	}))

	r.Post("/admin/plans", guard(func(w http.ResponseWriter, r *http.Request) {
		if !s.identity.ValidateWebForm(w, r) {
			return
		}
		p, _ := tenancy.FromContext(r.Context())
		limit, err1 := strconv.Atoi(r.PostForm.Get("monthly_limit"))
		rpm, err2 := strconv.Atoi(r.PostForm.Get("requests_per_minute"))
		if err1 != nil || err2 != nil {
			s.failure(w, auth.ErrConflict)
			return
		}
		active := r.PostForm.Get("active") == "true" || r.PostForm.Get("active") == "on"
		if err := s.SavePlan(r.Context(), p, PlanInput{
			Code:              r.PostForm.Get("code"),
			Name:              r.PostForm.Get("name"),
			MonthlyPrice:      r.PostForm.Get("monthly_price"),
			MonthlyLimit:      limit,
			RequestsPerMinute: rpm,
			Features:          r.PostForm["feature"],
			Active:            active,
		}); err != nil {
			s.failure(w, err)
			return
		}
		http.Redirect(w, r, "/admin/plans", 303)
	}))

	r.Get("/admin/users", guard(func(w http.ResponseWriter, r *http.Request) {
		p, _ := tenancy.FromContext(r.Context())
		q := r.URL.Query().Get("q")
		users, err := s.ListUsers(r.Context(), p, q)
		if err != nil {
			s.failure(w, err)
			return
		}
		orgs, err := s.ListOrganizations(r.Context(), p, "", "", "")
		if err != nil {
			s.failure(w, err)
			return
		}
		s.show(w, r, 200, page{View: "users", Title: "Usuários e controle de acesso", Users: users, Organizations: orgs, Query: q, Notice: r.URL.Query().Get("notice")})
	}))

	r.Post("/admin/users", guard(func(w http.ResponseWriter, r *http.Request) {
		if !s.identity.ValidateWebForm(w, r) {
			return
		}
		p, _ := tenancy.FromContext(r.Context())
		platformAdmin := r.PostForm.Get("platform_admin") == "on" || r.PostForm.Get("platform_admin") == "true"
		if _, err := s.ProvisionUserMembership(r.Context(), p, r.PostForm.Get("organization_id"), r.PostForm.Get("email"), r.PostForm.Get("password"), r.PostForm.Get("role"), platformAdmin); err != nil {
			s.failure(w, err)
			return
		}
		http.Redirect(w, r, "/admin/users", 303)
	}))

	r.Post("/admin/users/{id}/membership", guard(func(w http.ResponseWriter, r *http.Request) {
		if !s.identity.ValidateWebForm(w, r) {
			return
		}
		p, _ := tenancy.FromContext(r.Context())
		active := r.PostForm.Get("active") == "true" || r.PostForm.Get("active") == "on"
		if err := s.UpdateUserMembership(r.Context(), p, r.PostForm.Get("organization_id"), chi.URLParam(r, "id"), r.PostForm.Get("role"), active); err != nil {
			s.failure(w, err)
			return
		}
		http.Redirect(w, r, "/admin/users", 303)
	}))

	r.Post("/admin/users/{id}/status", guard(func(w http.ResponseWriter, r *http.Request) {
		if !s.identity.ValidateWebForm(w, r) {
			return
		}
		p, _ := tenancy.FromContext(r.Context())
		active := r.PostForm.Get("active") == "true"
		if err := s.SetUserStatus(r.Context(), p, chi.URLParam(r, "id"), active); err != nil {
			s.failure(w, err)
			return
		}
		http.Redirect(w, r, "/admin/users", 303)
	}))

	r.Post("/admin/users/{id}/platform-admin", guard(func(w http.ResponseWriter, r *http.Request) {
		if !s.identity.ValidateWebForm(w, r) {
			return
		}
		p, _ := tenancy.FromContext(r.Context())
		flag := r.PostForm.Get("platform_admin") == "true"
		if err := s.SetPlatformAdmin(r.Context(), p, chi.URLParam(r, "id"), flag); err != nil {
			s.failure(w, err)
			return
		}
		http.Redirect(w, r, "/admin/users", 303)
	}))

	r.Post("/admin/users/{id}/revoke-sessions", guard(func(w http.ResponseWriter, r *http.Request) {
		if !s.identity.ValidateWebForm(w, r) {
			return
		}
		p, _ := tenancy.FromContext(r.Context())
		if err := s.RevokeUserSessions(r.Context(), p, chi.URLParam(r, "id")); err != nil {
			s.failure(w, err)
			return
		}
		http.Redirect(w, r, "/admin/users", 303)
	}))

	r.Post("/admin/users/{id}/recover", guard(func(w http.ResponseWriter, r *http.Request) {
		if !s.identity.ValidateWebForm(w, r) {
			return
		}
		p, _ := tenancy.FromContext(r.Context())
		if err := s.TriggerUserRecovery(r.Context(), p, chi.URLParam(r, "id")); err != nil {
			s.failure(w, err)
			return
		}
		http.Redirect(w, r, "/admin/users?notice=Instru%C3%A7%C3%A3o+de+recupera%C3%A7%C3%A3o+preparada+no+outbox+criptografado", 303)
	}))

	r.Get("/admin/keys", guard(func(w http.ResponseWriter, r *http.Request) {
		p, _ := tenancy.FromContext(r.Context())
		orgFilter := r.URL.Query().Get("org")
		quotas, keys, err := s.ListKeysAndQuotas(r.Context(), p, orgFilter)
		if err != nil {
			s.failure(w, err)
			return
		}
		orgs, err := s.ListOrganizations(r.Context(), p, "", "", "")
		if err != nil {
			s.failure(w, err)
			return
		}
		s.show(w, r, 200, page{View: "keys", Title: "Chaves de API e cotas", Quotas: quotas, Keys: keys, Organizations: orgs, OrgFilter: orgFilter})
	}))

	r.Post("/admin/keys", guard(func(w http.ResponseWriter, r *http.Request) {
		if !s.identity.ValidateWebForm(w, r) {
			return
		}
		p, _ := tenancy.FromContext(r.Context())
		_, raw, err := s.IssueKey(r.Context(), p, r.PostForm.Get("organization_id"), r.PostForm.Get("name"), r.PostForm["scope"])
		if err != nil {
			s.failure(w, err)
			return
		}
		quotas, keys, err := s.ListKeysAndQuotas(r.Context(), p, "")
		if err != nil {
			s.failure(w, err)
			return
		}
		orgs, _ := s.ListOrganizations(r.Context(), p, "", "", "")
		s.show(w, r, 201, page{
			View: "keys", Title: "Chave emitida",
			Quotas: quotas, Keys: keys, Organizations: orgs,
			NewKey: raw, Notice: "Guarde a chave agora. Ela não será exibida novamente.",
		})
	}))

	r.Post("/admin/keys/{id}/rotate", guard(func(w http.ResponseWriter, r *http.Request) {
		if !s.identity.ValidateWebForm(w, r) {
			return
		}
		p, _ := tenancy.FromContext(r.Context())
		_, raw, err := s.RotateKey(r.Context(), p, chi.URLParam(r, "id"))
		if err != nil {
			s.failure(w, err)
			return
		}
		quotas, keys, err := s.ListKeysAndQuotas(r.Context(), p, "")
		if err != nil {
			s.failure(w, err)
			return
		}
		orgs, _ := s.ListOrganizations(r.Context(), p, "", "", "")
		s.show(w, r, 201, page{
			View: "keys", Title: "Chave rotacionada",
			Quotas: quotas, Keys: keys, Organizations: orgs,
			NewKey: raw, Notice: "A chave anterior foi revogada. Guarde a nova chave agora; ela não será exibida novamente.",
		})
	}))

	r.Post("/admin/keys/{id}/revoke", guard(func(w http.ResponseWriter, r *http.Request) {
		if !s.identity.ValidateWebForm(w, r) {
			return
		}
		p, _ := tenancy.FromContext(r.Context())
		if err := s.RevokeKey(r.Context(), p, chi.URLParam(r, "id")); err != nil {
			s.failure(w, err)
			return
		}
		http.Redirect(w, r, "/admin/keys", 303)
	}))

	r.Get("/admin/operations", guard(func(w http.ResponseWriter, r *http.Request) {
		p, _ := tenancy.FromContext(r.Context())
		orgFilter := r.URL.Query().Get("org")
		kindFilter := r.URL.Query().Get("kind")
		ops, err := s.ListOperations(r.Context(), p, orgFilter, kindFilter)
		if err != nil {
			s.failure(w, err)
			return
		}
		indices, runs, err := s.ListIndicesAndSyncs(r.Context(), p)
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
			View: "operations", Title: "Gestão jurídica e operacional",
			Operations: ops, Indices: indices, SyncRuns: runs, Dashboard: d,
			OrgFilter: orgFilter, KindFilter: kindFilter,
		})
	}))

	r.Post("/admin/indices/sync", guard(func(w http.ResponseWriter, r *http.Request) {
		if !s.identity.ValidateWebForm(w, r) {
			return
		}
		p, _ := tenancy.FromContext(r.Context())
		run, syncErr := s.SyncIndices(r.Context(), p, r.PostForm.Get("source"), r.PostForm.Get("start_month"), r.PostForm.Get("end_month"))
		ops, _ := s.ListOperations(r.Context(), p, "", "")
		indices, runs, _ := s.ListIndicesAndSyncs(r.Context(), p)
		d, _ := s.Dashboard(r.Context(), p)
		if syncErr != nil {
			s.show(w, r, 422, page{
				View: "operations", Title: "Gestão jurídica e operacional",
				Operations: ops, Indices: indices, SyncRuns: runs, Dashboard: d,
				Error: "Falha ao sincronizar índice: " + strings.TrimSpace(run.ErrorMessage),
			})
			return
		}
		s.show(w, r, 200, page{
			View: "operations", Title: "Gestão jurídica e operacional",
			Operations: ops, Indices: indices, SyncRuns: runs, Dashboard: d,
			Notice: "Sincronização concluída: " + strconv.Itoa(run.PointsSynced) + " pontos registrados em " + run.IndexCode + ".",
		})
	}))

	r.Get("/admin/audit", guard(func(w http.ResponseWriter, r *http.Request) {
		p, _ := tenancy.FromContext(r.Context())
		orgFilter := r.URL.Query().Get("org")
		actionFilter := r.URL.Query().Get("action")
		q := r.URL.Query().Get("q")
		var beforeID int64
		if b := r.URL.Query().Get("before"); b != "" {
			var err error
			beforeID, err = strconv.ParseInt(b, 10, 64)
			if err != nil || beforeID <= 0 {
				http.Error(w, "Cursor de auditoria inválido", 400)
				return
			}
		}
		events, next, err := s.ListAuditEvents(r.Context(), p, orgFilter, actionFilter, q, beforeID)
		if err != nil {
			s.failure(w, err)
			return
		}
		s.show(w, r, 200, page{
			View: "audit", Title: "Auditoria administrativa e operacional",
			AuditEvents: events, NextAuditID: next,
			OrgFilter: orgFilter, ActionFilter: actionFilter, Query: q,
		})
	}))
}
