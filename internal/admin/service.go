// Package admin provides global operational administration over organizations, plans, quotas, users, keys, indices, and audit logs.
package admin

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"sync/atomic"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rdrgzma/robo-juridico-go-saas/internal/auth"
	"github.com/rdrgzma/robo-juridico-go-saas/internal/calculator"
	"github.com/rdrgzma/robo-juridico-go-saas/internal/juridico"
	"github.com/rdrgzma/robo-juridico-go-saas/internal/tenancy"
	"github.com/shopspring/decimal"
)

var (
	slugPattern     = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{1,62}$`)
	planCodePattern = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{1,40}$`)
	uuidPattern     = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)
	AllowedFeatures = []string{"cnj", "judicial", "liquidacao", "financiamento", "prazos", "indices", "api"}
)

type Service struct {
	pool            *pgxpool.Pool
	identity        *auth.Service
	workspace       *juridico.Service
	requests        atomic.Int64
	clientErrors    atomic.Int64
	serverErrors    atomic.Int64
	totalDurationNs atomic.Int64
	maxDurationNs   atomic.Int64
}

func New(pool *pgxpool.Pool, identity *auth.Service, workspace *juridico.Service) *Service {
	return &Service{pool: pool, identity: identity, workspace: workspace}
}

func (s *Service) RecordRequest(status int, duration time.Duration) {
	if s == nil {
		return
	}
	s.requests.Add(1)
	if status >= 400 && status < 500 {
		s.clientErrors.Add(1)
	} else if status >= 500 {
		s.serverErrors.Add(1)
	}
	ns := duration.Nanoseconds()
	if ns < 0 {
		ns = 0
	}
	s.totalDurationNs.Add(ns)
	for {
		cur := s.maxDurationNs.Load()
		if ns <= cur || s.maxDurationNs.CompareAndSwap(cur, ns) {
			break
		}
	}
}

func requireAdmin(actor tenancy.Principal) error {
	if !actor.PlatformAdmin || actor.RequiresMFA() {
		return auth.ErrDenied
	}
	return nil
}

type IntegrationStatus struct {
	Name   string
	Status string
	Detail string
}

type DashboardData struct {
	ActiveOrganizations int
	TotalOrganizations  int
	ActiveUsers         int
	TotalUsers          int
	ActiveMemberships   int
	MonthQueries        int
	MonthCalculations   int
	TotalOperations     int
	MonthQuotaUsed      int
	MonthQuotaLimit     int
	MonthQuotaBlocked   int
	MonthAPICompleted   int
	MonthAPIPending     int
	MonthAPIRefunded    int
	EstimatedMRR        string
	HomologatedReports  int
	CapturedLeads       int
	SyncedIndexPoints   int
	HTTPRequests        int64
	HTTPClientErrors    int64
	HTTPServerErrors    int64
	AvgLatencyMs        int64
	MaxLatencyMs        int64
	Integrations        []IntegrationStatus
}

func (s *Service) Dashboard(ctx context.Context, actor tenancy.Principal) (DashboardData, error) {
	var d DashboardData
	if err := requireAdmin(actor); err != nil {
		return d, err
	}
	var mrr decimal.Decimal
	err := s.pool.QueryRow(ctx, `SELECT
		(SELECT count(*) FROM organizations WHERE active),
		(SELECT count(*) FROM organizations),
		(SELECT count(*) FROM users WHERE active),
		(SELECT count(*) FROM users),
		(SELECT count(*) FROM memberships m JOIN organizations o ON o.id=m.organization_id JOIN users u ON u.id=m.user_id WHERE m.active AND o.active AND u.active),
		(SELECT count(*) FROM legal_operations WHERE kind IN ('cnj','indices') AND created_at >= date_trunc('month', now() AT TIME ZONE 'America/Sao_Paulo')),
		(SELECT count(*) FROM legal_operations WHERE kind NOT IN ('cnj','indices') AND created_at >= date_trunc('month', now() AT TIME ZONE 'America/Sao_Paulo')),
		(SELECT count(*) FROM legal_operations),
		COALESCE((SELECT sum(used) FROM quota_periods WHERE period = date_trunc('month', now() AT TIME ZONE 'America/Sao_Paulo')::date), 0),
		COALESCE((SELECT sum(l.monthly_limit) FROM organization_limits l JOIN organizations o ON o.id=l.organization_id WHERE o.active), 0),
		COALESCE((SELECT sum(blocked) FROM quota_periods WHERE period = date_trunc('month', now() AT TIME ZONE 'America/Sao_Paulo')::date), 0),
		(SELECT count(*) FROM usage_events WHERE state='completed' AND period = date_trunc('month', now() AT TIME ZONE 'America/Sao_Paulo')::date),
		(SELECT count(*) FROM usage_events WHERE state='pending' AND period = date_trunc('month', now() AT TIME ZONE 'America/Sao_Paulo')::date),
		(SELECT count(*) FROM usage_events WHERE state='refunded' AND period = date_trunc('month', now() AT TIME ZONE 'America/Sao_Paulo')::date),
		COALESCE((SELECT sum(l.contracted_price) FROM organization_limits l JOIN organizations o ON o.id=l.organization_id JOIN plans p ON p.code=l.plan_code
			WHERE o.active AND p.active AND (now() AT TIME ZONE 'America/Sao_Paulo')::date >= l.effective_from
			AND (l.effective_until IS NULL OR (now() AT TIME ZONE 'America/Sao_Paulo')::date <= l.effective_until)), 0.00),
		(SELECT count(*) FROM index_points)`).Scan(
		&d.ActiveOrganizations, &d.TotalOrganizations,
		&d.ActiveUsers, &d.TotalUsers, &d.ActiveMemberships,
		&d.MonthQueries, &d.MonthCalculations, &d.TotalOperations,
		&d.MonthQuotaUsed, &d.MonthQuotaLimit, &d.MonthQuotaBlocked,
		&d.MonthAPICompleted, &d.MonthAPIPending, &d.MonthAPIRefunded,
		&mrr, &d.SyncedIndexPoints,
	)
	if err != nil {
		return d, err
	}
	d.EstimatedMRR = mrr.StringFixed(2)
	d.HTTPRequests = s.requests.Load()
	d.HTTPClientErrors = s.clientErrors.Load()
	d.HTTPServerErrors = s.serverErrors.Load()
	if d.HTTPRequests > 0 {
		d.AvgLatencyMs = (s.totalDurationNs.Load() / d.HTTPRequests) / int64(time.Millisecond)
	}
	d.MaxLatencyMs = s.maxDurationNs.Load() / int64(time.Millisecond)

	var datajudConfigured, djenEnabled bool
	if s.workspace != nil {
		datajudConfigured, djenEnabled = s.workspace.Sources().Status()
	}
	datajudState := "Não configurada (validação CNJ local ativa)"
	if datajudConfigured {
		datajudState = "Chave configurada (homologação ao vivo pendente)"
	}
	djenState := "Desabilitado (DJEN_ENABLED=false)"
	if djenEnabled {
		djenState = "Habilitado (homologação ao vivo pendente)"
	}
	d.Integrations = []IntegrationStatus{
		{Name: "PostgreSQL", Status: "Operacional", Detail: "Pool transacional ativo, migrations 000001..000006 aplicadas"},
		{Name: "DataJud / CNJ", Status: datajudState, Detail: "Timeout 15s, limite 2 MiB, sem redirecionamentos"},
		{Name: "DJEN / Comunica", Status: djenState, Detail: "Timeout 15s, limite 2 MiB, sem redirecionamentos"},
		{Name: "BCB/SGS 433 e IBGE/SIDRA 1737", Status: "Disponível sob comando", Detail: fmt.Sprintf("%d pontos sincronizados em index_points", d.SyncedIndexPoints)},
	}
	return d, nil
}

type Plan struct {
	Code              string
	Name              string
	MonthlyPrice      decimal.Decimal
	MonthlyLimit      int
	RequestsPerMinute int
	Features          []string
	Active            bool
	Organizations     int
	CreatedAt         time.Time
}

func (p Plan) HasFeature(code string) bool {
	for _, f := range p.Features {
		if f == code {
			return true
		}
	}
	return false
}

type PlanInput struct {
	Code              string
	Name              string
	MonthlyPrice      string
	MonthlyLimit      int
	RequestsPerMinute int
	Features          []string
	Active            bool
}

func normalizeFeatures(features []string) ([]string, error) {
	allowed := map[string]bool{}
	for _, f := range AllowedFeatures {
		allowed[f] = true
	}
	seen := map[string]bool{}
	out := []string{}
	for _, f := range features {
		f = strings.TrimSpace(strings.ToLower(f))
		if f == "" {
			continue
		}
		if !allowed[f] {
			return nil, fmt.Errorf("%w: recurso de plano inválido (%s)", auth.ErrConflict, f)
		}
		if !seen[f] {
			seen[f] = true
			out = append(out, f)
		}
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("%w: selecione ao menos um recurso para o plano", auth.ErrConflict)
	}
	return out, nil
}

func (s *Service) ListPlans(ctx context.Context, actor tenancy.Principal) ([]Plan, error) {
	if err := requireAdmin(actor); err != nil {
		return nil, err
	}
	rows, err := s.pool.Query(ctx, `SELECT p.code,p.name,p.monthly_price,p.monthly_limit,p.requests_per_minute,p.features,p.active,p.created_at,
		(SELECT count(*) FROM organization_limits l WHERE l.plan_code=p.code)
		FROM plans p ORDER BY p.monthly_price ASC, p.code ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Plan{}
	for rows.Next() {
		var p Plan
		if err := rows.Scan(&p.Code, &p.Name, &p.MonthlyPrice, &p.MonthlyLimit, &p.RequestsPerMinute, &p.Features, &p.Active, &p.CreatedAt, &p.Organizations); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

func (s *Service) SavePlan(ctx context.Context, actor tenancy.Principal, in PlanInput) error {
	if err := requireAdmin(actor); err != nil {
		return err
	}
	code := strings.ToLower(strings.TrimSpace(in.Code))
	name := strings.TrimSpace(in.Name)
	if !planCodePattern.MatchString(code) || len(name) < 1 || len(name) > 120 || in.MonthlyLimit < 0 || in.RequestsPerMinute < 1 || in.RequestsPerMinute > 6000 {
		return auth.ErrConflict
	}
	price, err := calculator.ParseDecimal(strings.TrimSpace(in.MonthlyPrice))
	if err != nil || price.IsNegative() || price.Exponent() < -2 {
		return auth.ErrConflict
	}
	features, err := normalizeFeatures(in.Features)
	if err != nil {
		return err
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(context.Background())
	if _, err = tx.Exec(ctx, `INSERT INTO plans(code,name,monthly_price,monthly_limit,requests_per_minute,features,active)
		VALUES($1,$2,$3,$4,$5,$6,$7)
		ON CONFLICT(code) DO UPDATE SET name=EXCLUDED.name,monthly_price=EXCLUDED.monthly_price,monthly_limit=EXCLUDED.monthly_limit,
		requests_per_minute=EXCLUDED.requests_per_minute,features=EXCLUDED.features,active=EXCLUDED.active`,
		code, name, price.StringFixed(2), in.MonthlyLimit, in.RequestsPerMinute, features, in.Active); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, "INSERT INTO audit_events(organization_id,actor_id,action) VALUES($1,$2,$3)", actor.OrganizationID, actor.UserID, "admin.plan_saved:"+code); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

type OrganizationSummary struct {
	ID                string
	Slug              string
	Name              string
	LegalName         string
	Document          string
	ContactName       string
	ContactEmail      string
	Notes             string
	Active            bool
	CreatedAt         time.Time
	PlanCode          string
	PlanName          string
	ContractedPrice   decimal.Decimal
	MonthlyLimit      int
	RequestsPerMinute int
	EffectiveFrom     string
	EffectiveUntil    string
	ContractNotes     string
	MonthUsed         int
	MonthBlocked      int
	MemberCount       int
}

type ContractHistoryEntry struct {
	ID                int64
	PlanCode          string
	MonthlyLimit      int
	RequestsPerMinute int
	ContractedPrice   decimal.Decimal
	EffectiveFrom     string
	EffectiveUntil    string
	Notes             string
	ActorEmail        string
	CreatedAt         time.Time
}

type OrganizationDetail struct {
	Organization    OrganizationSummary
	Members         []UserMembershipView
	Keys            []AdminKeyView
	ContractHistory []ContractHistoryEntry
}

func (s *Service) ListOrganizations(ctx context.Context, actor tenancy.Principal, query, statusFilter, planFilter string) ([]OrganizationSummary, error) {
	if err := requireAdmin(actor); err != nil {
		return nil, err
	}
	q := "%" + strings.ToLower(strings.TrimSpace(query)) + "%"
	rows, err := s.pool.Query(ctx, `SELECT o.id::text,o.slug,o.name,o.legal_name,o.document,o.contact_name,o.contact_email,o.notes,o.active,o.created_at,
		l.plan_code,p.name,l.contracted_price,l.monthly_limit,l.requests_per_minute,l.effective_from::text,COALESCE(l.effective_until::text,''),l.contract_notes,
		COALESCE((SELECT used FROM quota_periods qp WHERE qp.organization_id=o.id AND qp.period=date_trunc('month',now() AT TIME ZONE 'America/Sao_Paulo')::date),0),
		COALESCE((SELECT blocked FROM quota_periods qp WHERE qp.organization_id=o.id AND qp.period=date_trunc('month',now() AT TIME ZONE 'America/Sao_Paulo')::date),0),
		(SELECT count(*) FROM memberships m WHERE m.organization_id=o.id AND m.active)
		FROM organizations o JOIN organization_limits l ON l.organization_id=o.id JOIN plans p ON p.code=l.plan_code
		WHERE ($1='%%' OR lower(o.name) LIKE $1 OR lower(o.slug) LIKE $1 OR lower(o.legal_name) LIKE $1 OR lower(o.document) LIKE $1 OR lower(o.contact_email) LIKE $1)
		AND ($2='' OR ($2='active' AND o.active) OR ($2='suspended' AND NOT o.active))
		AND ($3='' OR l.plan_code=$3)
		ORDER BY o.created_at DESC, o.slug ASC LIMIT 200`, q, strings.TrimSpace(statusFilter), strings.TrimSpace(planFilter))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []OrganizationSummary{}
	for rows.Next() {
		var o OrganizationSummary
		if err := rows.Scan(&o.ID, &o.Slug, &o.Name, &o.LegalName, &o.Document, &o.ContactName, &o.ContactEmail, &o.Notes, &o.Active, &o.CreatedAt,
			&o.PlanCode, &o.PlanName, &o.ContractedPrice, &o.MonthlyLimit, &o.RequestsPerMinute, &o.EffectiveFrom, &o.EffectiveUntil, &o.ContractNotes,
			&o.MonthUsed, &o.MonthBlocked, &o.MemberCount); err != nil {
			return nil, err
		}
		out = append(out, o)
	}
	return out, rows.Err()
}

func (s *Service) GetOrganization(ctx context.Context, actor tenancy.Principal, id string) (OrganizationDetail, error) {
	var detail OrganizationDetail
	if err := requireAdmin(actor); err != nil {
		return detail, err
	}
	if !uuidPattern.MatchString(id) {
		return detail, auth.ErrNotFound
	}
	o := &detail.Organization
	err := s.pool.QueryRow(ctx, `SELECT o.id::text,o.slug,o.name,o.legal_name,o.document,o.contact_name,o.contact_email,o.notes,o.active,o.created_at,
		l.plan_code,p.name,l.contracted_price,l.monthly_limit,l.requests_per_minute,l.effective_from::text,COALESCE(l.effective_until::text,''),l.contract_notes,
		COALESCE((SELECT used FROM quota_periods qp WHERE qp.organization_id=o.id AND qp.period=date_trunc('month',now() AT TIME ZONE 'America/Sao_Paulo')::date),0),
		COALESCE((SELECT blocked FROM quota_periods qp WHERE qp.organization_id=o.id AND qp.period=date_trunc('month',now() AT TIME ZONE 'America/Sao_Paulo')::date),0),
		(SELECT count(*) FROM memberships m WHERE m.organization_id=o.id AND m.active)
		FROM organizations o JOIN organization_limits l ON l.organization_id=o.id JOIN plans p ON p.code=l.plan_code
		WHERE o.id=$1`, id).Scan(
		&o.ID, &o.Slug, &o.Name, &o.LegalName, &o.Document, &o.ContactName, &o.ContactEmail, &o.Notes, &o.Active, &o.CreatedAt,
		&o.PlanCode, &o.PlanName, &o.ContractedPrice, &o.MonthlyLimit, &o.RequestsPerMinute, &o.EffectiveFrom, &o.EffectiveUntil, &o.ContractNotes,
		&o.MonthUsed, &o.MonthBlocked, &o.MemberCount,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return detail, auth.ErrNotFound
	}
	if err != nil {
		return detail, err
	}
	mRows, err := s.pool.Query(ctx, `SELECT u.id::text,u.email,m.organization_id::text,o.slug,o.name,m.role_code,m.active,u.active,u.platform_admin
		FROM memberships m JOIN users u ON u.id=m.user_id JOIN organizations o ON o.id=m.organization_id
		WHERE m.organization_id=$1 ORDER BY lower(u.email)`, id)
	if err != nil {
		return detail, err
	}
	defer mRows.Close()
	for mRows.Next() {
		var m UserMembershipView
		if err := mRows.Scan(&m.UserID, &m.Email, &m.OrganizationID, &m.OrganizationSlug, &m.OrganizationName, &m.Role, &m.Active, &m.UserActive, &m.PlatformAdmin); err != nil {
			return detail, err
		}
		detail.Members = append(detail.Members, m)
	}
	if err := mRows.Err(); err != nil {
		return detail, err
	}
	kRows, err := s.pool.Query(ctx, `SELECT k.id::text,k.organization_id::text,o.slug,k.name,k.prefix,k.scopes,k.created_at,k.last_used_at,k.revoked_at
		FROM api_keys k JOIN organizations o ON o.id=k.organization_id WHERE k.organization_id=$1 ORDER BY k.created_at DESC LIMIT 100`, id)
	if err != nil {
		return detail, err
	}
	defer kRows.Close()
	for kRows.Next() {
		var k AdminKeyView
		if err := kRows.Scan(&k.ID, &k.OrganizationID, &k.OrganizationSlug, &k.Name, &k.Prefix, &k.Scopes, &k.CreatedAt, &k.LastUsed, &k.RevokedAt); err != nil {
			return detail, err
		}
		detail.Keys = append(detail.Keys, k)
	}
	if err := kRows.Err(); err != nil {
		return detail, err
	}
	hRows, err := s.pool.Query(ctx, `SELECT h.id,h.plan_code,h.monthly_limit,h.requests_per_minute,h.contracted_price,h.effective_from::text,COALESCE(h.effective_until::text,''),h.notes,COALESCE(u.email,'sistema'),h.created_at
		FROM organization_contract_history h LEFT JOIN users u ON u.id=h.actor_id
		WHERE h.organization_id=$1 ORDER BY h.created_at DESC, h.id DESC LIMIT 100`, id)
	if err != nil {
		return detail, err
	}
	defer hRows.Close()
	for hRows.Next() {
		var h ContractHistoryEntry
		if err := hRows.Scan(&h.ID, &h.PlanCode, &h.MonthlyLimit, &h.RequestsPerMinute, &h.ContractedPrice, &h.EffectiveFrom, &h.EffectiveUntil, &h.Notes, &h.ActorEmail, &h.CreatedAt); err != nil {
			return detail, err
		}
		detail.ContractHistory = append(detail.ContractHistory, h)
	}
	return detail, hRows.Err()
}

type CreateOrganizationInput struct {
	Slug              string
	Name              string
	LegalName         string
	Document          string
	ContactName       string
	ContactEmail      string
	Notes             string
	OwnerEmail        string
	OwnerPassword     string
	PlanCode          string
	ContractedPrice   string
	MonthlyLimit      int
	RequestsPerMinute int
	EffectiveFrom     string
	EffectiveUntil    string
	ContractNotes     string
}

func parseContractDates(fromStr, untilStr string) (string, *string, error) {
	fromStr = strings.TrimSpace(fromStr)
	if fromStr == "" {
		fromStr = time.Now().Format("2006-01-02")
	}
	fromDate, err := time.Parse("2006-01-02", fromStr)
	if err != nil {
		return "", nil, auth.ErrConflict
	}
	untilStr = strings.TrimSpace(untilStr)
	if untilStr == "" {
		return fromStr, nil, nil
	}
	untilDate, err := time.Parse("2006-01-02", untilStr)
	if err != nil || untilDate.Before(fromDate) {
		return "", nil, auth.ErrConflict
	}
	return fromStr, &untilStr, nil
}

func (s *Service) CreateClientOrganization(ctx context.Context, actor tenancy.Principal, in CreateOrganizationInput) (string, error) {
	if err := requireAdmin(actor); err != nil {
		return "", err
	}
	slug := strings.ToLower(strings.TrimSpace(in.Slug))
	name := strings.TrimSpace(in.Name)
	ownerEmail := strings.ToLower(strings.TrimSpace(in.OwnerEmail))
	if !slugPattern.MatchString(slug) || len(name) < 1 || len(name) > 200 || len(in.LegalName) > 200 || len(in.Document) > 40 || len(in.ContactName) > 160 || len(in.ContactEmail) > 254 || len(in.Notes) > 4000 || len(in.ContractNotes) > 2000 || !strings.Contains(ownerEmail, "@") {
		return "", auth.ErrConflict
	}
	planCode := strings.ToLower(strings.TrimSpace(in.PlanCode))
	if planCode == "" {
		planCode = "padrao"
	}
	fromStr, untilPtr, err := parseContractDates(in.EffectiveFrom, in.EffectiveUntil)
	if err != nil {
		return "", err
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return "", err
	}
	defer tx.Rollback(context.Background())

	var defaultPrice decimal.Decimal
	var defaultLimit, defaultRPM int
	if err = tx.QueryRow(ctx, "SELECT monthly_price,monthly_limit,requests_per_minute FROM plans WHERE code=$1 AND active", planCode).Scan(&defaultPrice, &defaultLimit, &defaultRPM); errors.Is(err, pgx.ErrNoRows) {
		return "", auth.ErrConflict
	} else if err != nil {
		return "", err
	}
	price := defaultPrice
	if strings.TrimSpace(in.ContractedPrice) != "" {
		price, err = calculator.ParseDecimal(strings.TrimSpace(in.ContractedPrice))
		if err != nil || price.IsNegative() || price.Exponent() < -2 {
			return "", auth.ErrConflict
		}
	}
	limit := in.MonthlyLimit
	if limit < 0 {
		return "", auth.ErrConflict
	}
	if limit == 0 && strings.TrimSpace(in.ContractedPrice) == "" {
		limit = defaultLimit
	}
	rpm := in.RequestsPerMinute
	if rpm == 0 {
		rpm = defaultRPM
	}
	if rpm < 1 || rpm > 6000 {
		return "", auth.ErrConflict
	}

	var orgID string
	if err = tx.QueryRow(ctx, `INSERT INTO organizations(slug,name,legal_name,document,contact_name,contact_email,notes)
		VALUES($1,$2,$3,$4,$5,$6,$7) RETURNING id::text`,
		slug, name, strings.TrimSpace(in.LegalName), strings.TrimSpace(in.Document), strings.TrimSpace(in.ContactName), strings.TrimSpace(in.ContactEmail), strings.TrimSpace(in.Notes)).Scan(&orgID); err != nil {
		return "", err
	}

	var userID string
	err = tx.QueryRow(ctx, "SELECT id::text FROM users WHERE lower(email)=$1", ownerEmail).Scan(&userID)
	if errors.Is(err, pgx.ErrNoRows) {
		hash, hashErr := auth.HashPassword(in.OwnerPassword)
		if hashErr != nil {
			return "", auth.ErrConflict
		}
		if err = tx.QueryRow(ctx, "INSERT INTO users(email,password_hash) VALUES($1,$2) RETURNING id::text", ownerEmail, hash).Scan(&userID); err != nil {
			return "", err
		}
	} else if err != nil {
		return "", err
	}

	if _, err = tx.Exec(ctx, "INSERT INTO memberships(organization_id,user_id,role_code,active) VALUES($1,$2,'owner',true)", orgID, userID); err != nil {
		return "", err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO organization_limits(organization_id,plan_code,monthly_limit,requests_per_minute,contracted_price,effective_from,effective_until,contract_notes)
		VALUES($1,$2,$3,$4,$5,$6::date,$7::date,$8)`,
		orgID, planCode, limit, rpm, price.StringFixed(2), fromStr, untilPtr, strings.TrimSpace(in.ContractNotes)); err != nil {
		return "", err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO organization_contract_history(organization_id,actor_id,plan_code,monthly_limit,requests_per_minute,contracted_price,effective_from,effective_until,notes)
		VALUES($1,$2,$3,$4,$5,$6,$7::date,$8::date,$9)`,
		orgID, actor.UserID, planCode, limit, rpm, price.StringFixed(2), fromStr, untilPtr, strings.TrimSpace(in.ContractNotes)); err != nil {
		return "", err
	}
	if _, err = tx.Exec(ctx, "INSERT INTO audit_events(organization_id,actor_id,action,target_id) VALUES($1,$2,'admin.organization_created',$1)", orgID, actor.UserID); err != nil {
		return "", err
	}
	return orgID, tx.Commit(ctx)
}

type UpdateOrganizationInput struct {
	Name         string
	LegalName    string
	Document     string
	ContactName  string
	ContactEmail string
	Notes        string
}

func (s *Service) UpdateOrganization(ctx context.Context, actor tenancy.Principal, id string, in UpdateOrganizationInput) error {
	if err := requireAdmin(actor); err != nil {
		return err
	}
	if !uuidPattern.MatchString(id) {
		return auth.ErrNotFound
	}
	name := strings.TrimSpace(in.Name)
	if len(name) < 1 || len(name) > 200 || len(in.LegalName) > 200 || len(in.Document) > 40 || len(in.ContactName) > 160 || len(in.ContactEmail) > 254 || len(in.Notes) > 4000 {
		return auth.ErrConflict
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(context.Background())
	tag, err := tx.Exec(ctx, `UPDATE organizations SET name=$2,legal_name=$3,document=$4,contact_name=$5,contact_email=$6,notes=$7 WHERE id=$1`,
		id, name, strings.TrimSpace(in.LegalName), strings.TrimSpace(in.Document), strings.TrimSpace(in.ContactName), strings.TrimSpace(in.ContactEmail), strings.TrimSpace(in.Notes))
	if err != nil {
		return err
	}
	if tag.RowsAffected() != 1 {
		return auth.ErrNotFound
	}
	if _, err = tx.Exec(ctx, "INSERT INTO audit_events(organization_id,actor_id,action,target_id) VALUES($1,$2,'admin.organization_updated',$1)", id, actor.UserID); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (s *Service) SetOrganizationStatus(ctx context.Context, actor tenancy.Principal, id string, active bool) error {
	if err := requireAdmin(actor); err != nil {
		return err
	}
	if !uuidPattern.MatchString(id) {
		return auth.ErrNotFound
	}
	if !active && id == actor.OrganizationID {
		return auth.ErrConflict
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(context.Background())
	tag, err := tx.Exec(ctx, "UPDATE organizations SET active=$2 WHERE id=$1", id, active)
	if err != nil {
		return err
	}
	if tag.RowsAffected() != 1 {
		return auth.ErrNotFound
	}
	action := "admin.organization_activated"
	if !active {
		action = "admin.organization_suspended"
		if _, err = tx.Exec(ctx, "DELETE FROM sessions WHERE organization_id=$1", id); err != nil {
			return err
		}
	}
	if _, err = tx.Exec(ctx, "INSERT INTO audit_events(organization_id,actor_id,action,target_id) VALUES($1,$2,$3,$1)", id, actor.UserID, action); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

type ContractInput struct {
	PlanCode          string
	ContractedPrice   string
	MonthlyLimit      int
	RequestsPerMinute int
	EffectiveFrom     string
	EffectiveUntil    string
	Notes             string
}

func (s *Service) UpdateContract(ctx context.Context, actor tenancy.Principal, orgID string, in ContractInput) error {
	if err := requireAdmin(actor); err != nil {
		return err
	}
	if !uuidPattern.MatchString(orgID) {
		return auth.ErrNotFound
	}
	planCode := strings.ToLower(strings.TrimSpace(in.PlanCode))
	if !planCodePattern.MatchString(planCode) || in.MonthlyLimit < 0 || in.RequestsPerMinute < 1 || in.RequestsPerMinute > 6000 || len(in.Notes) > 2000 {
		return auth.ErrConflict
	}
	price, err := calculator.ParseDecimal(strings.TrimSpace(in.ContractedPrice))
	if err != nil || price.IsNegative() || price.Exponent() < -2 {
		return auth.ErrConflict
	}
	fromStr, untilPtr, err := parseContractDates(in.EffectiveFrom, in.EffectiveUntil)
	if err != nil {
		return err
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(context.Background())
	var planExists bool
	if err = tx.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM plans WHERE code=$1)", planCode).Scan(&planExists); err != nil || !planExists {
		return auth.ErrConflict
	}
	tag, err := tx.Exec(ctx, `UPDATE organization_limits
		SET plan_code=$2,monthly_limit=$3,requests_per_minute=$4,contracted_price=$5,effective_from=$6::date,effective_until=$7::date,contract_notes=$8
		WHERE organization_id=$1`,
		orgID, planCode, in.MonthlyLimit, in.RequestsPerMinute, price.StringFixed(2), fromStr, untilPtr, strings.TrimSpace(in.Notes))
	if err != nil {
		return err
	}
	if tag.RowsAffected() != 1 {
		return auth.ErrNotFound
	}
	if _, err = tx.Exec(ctx, `INSERT INTO organization_contract_history(organization_id,actor_id,plan_code,monthly_limit,requests_per_minute,contracted_price,effective_from,effective_until,notes)
		VALUES($1,$2,$3,$4,$5,$6,$7::date,$8::date,$9)`,
		orgID, actor.UserID, planCode, in.MonthlyLimit, in.RequestsPerMinute, price.StringFixed(2), fromStr, untilPtr, strings.TrimSpace(in.Notes)); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, "INSERT INTO audit_events(organization_id,actor_id,action,target_id) VALUES($1,$2,'admin.contract_updated',$1)", orgID, actor.UserID); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

type UserMembershipView struct {
	UserID           string
	Email            string
	OrganizationID   string
	OrganizationSlug string
	OrganizationName string
	Role             string
	Active           bool
	UserActive       bool
	PlatformAdmin    bool
}

type UserAdminView struct {
	ID             string
	Email          string
	Active         bool
	PlatformAdmin  bool
	MFAEnabled     bool
	ActiveSessions int
	CreatedAt      time.Time
	Memberships    []UserMembershipView
}

func (s *Service) ListUsers(ctx context.Context, actor tenancy.Principal, query string) ([]UserAdminView, error) {
	if err := requireAdmin(actor); err != nil {
		return nil, err
	}
	q := "%" + strings.ToLower(strings.TrimSpace(query)) + "%"
	rows, err := s.pool.Query(ctx, `SELECT u.id::text,u.email,u.active,u.platform_admin,(u.mfa_secret IS NOT NULL),
		(SELECT count(*) FROM sessions se WHERE se.user_id=u.id AND se.expires_at>now()),u.created_at
		FROM users u WHERE ($1='%%' OR lower(u.email) LIKE $1)
		ORDER BY u.platform_admin DESC, lower(u.email) ASC LIMIT 200`, q)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	users := []UserAdminView{}
	byID := map[string]int{}
	for rows.Next() {
		var u UserAdminView
		if err := rows.Scan(&u.ID, &u.Email, &u.Active, &u.PlatformAdmin, &u.MFAEnabled, &u.ActiveSessions, &u.CreatedAt); err != nil {
			return nil, err
		}
		byID[u.ID] = len(users)
		users = append(users, u)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	mRows, err := s.pool.Query(ctx, `SELECT m.user_id::text,u.email,m.organization_id::text,o.slug,o.name,m.role_code,m.active,u.active,u.platform_admin
		FROM memberships m JOIN organizations o ON o.id=m.organization_id JOIN users u ON u.id=m.user_id
		ORDER BY o.slug ASC`)
	if err != nil {
		return nil, err
	}
	defer mRows.Close()
	for mRows.Next() {
		var m UserMembershipView
		if err := mRows.Scan(&m.UserID, &m.Email, &m.OrganizationID, &m.OrganizationSlug, &m.OrganizationName, &m.Role, &m.Active, &m.UserActive, &m.PlatformAdmin); err != nil {
			return nil, err
		}
		if idx, ok := byID[m.UserID]; ok {
			users[idx].Memberships = append(users[idx].Memberships, m)
		}
	}
	return users, mRows.Err()
}

func validOrgRole(role string) bool {
	return role == "owner" || role == "admin" || role == "lawyer" || role == "viewer"
}

func (s *Service) ProvisionUserMembership(ctx context.Context, actor tenancy.Principal, orgID, email, password, role string, platformAdmin bool) (string, error) {
	if err := requireAdmin(actor); err != nil {
		return "", err
	}
	email = strings.ToLower(strings.TrimSpace(email))
	if !uuidPattern.MatchString(orgID) || !validOrgRole(role) || !strings.Contains(email, "@") || len(email) > 254 {
		return "", auth.ErrConflict
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return "", err
	}
	defer tx.Rollback(context.Background())
	var orgExists bool
	if err = tx.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM organizations WHERE id=$1 FOR UPDATE)", orgID).Scan(&orgExists); err != nil || !orgExists {
		return "", auth.ErrNotFound
	}
	var userID string
	err = tx.QueryRow(ctx, "SELECT id::text FROM users WHERE lower(email)=$1 FOR UPDATE", email).Scan(&userID)
	if errors.Is(err, pgx.ErrNoRows) {
		hash, hashErr := auth.HashPassword(password)
		if hashErr != nil {
			return "", auth.ErrConflict
		}
		if err = tx.QueryRow(ctx, "INSERT INTO users(email,password_hash,platform_admin) VALUES($1,$2,$3) RETURNING id::text", email, hash, platformAdmin).Scan(&userID); err != nil {
			return "", err
		}
	} else if err != nil {
		return "", err
	} else if platformAdmin {
		if _, err = tx.Exec(ctx, "UPDATE users SET platform_admin=true WHERE id=$1", userID); err != nil {
			return "", err
		}
	}
	if _, err = tx.Exec(ctx, `INSERT INTO memberships(organization_id,user_id,role_code,active) VALUES($1,$2,$3,true)
		ON CONFLICT(organization_id,user_id) DO UPDATE SET role_code=EXCLUDED.role_code,active=true`, orgID, userID, role); err != nil {
		return "", err
	}
	if _, err = tx.Exec(ctx, "INSERT INTO audit_events(organization_id,actor_id,action,target_id) VALUES($1,$2,'admin.membership_provisioned',$3)", orgID, actor.UserID, userID); err != nil {
		return "", err
	}
	return userID, tx.Commit(ctx)
}

func (s *Service) UpdateUserMembership(ctx context.Context, actor tenancy.Principal, orgID, userID, role string, active bool) error {
	if err := requireAdmin(actor); err != nil {
		return err
	}
	if !uuidPattern.MatchString(orgID) || !uuidPattern.MatchString(userID) {
		return auth.ErrNotFound
	}
	if !validOrgRole(role) {
		return auth.ErrConflict
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(context.Background())
	if _, err = tx.Exec(ctx, "SELECT id FROM organizations WHERE id=$1 FOR UPDATE", orgID); err != nil {
		return err
	}
	var oldRole string
	var oldActive bool
	if err = tx.QueryRow(ctx, "SELECT role_code,active FROM memberships WHERE organization_id=$1 AND user_id=$2", orgID, userID).Scan(&oldRole, &oldActive); errors.Is(err, pgx.ErrNoRows) {
		return auth.ErrNotFound
	} else if err != nil {
		return err
	}
	if oldRole == "owner" && oldActive && (role != "owner" || !active) {
		var owners int
		if err = tx.QueryRow(ctx, "SELECT count(*) FROM memberships m JOIN users u ON u.id=m.user_id WHERE m.organization_id=$1 AND m.role_code='owner' AND m.active AND u.active", orgID).Scan(&owners); err != nil {
			return err
		}
		if owners <= 1 {
			return auth.ErrConflict
		}
	}
	if _, err = tx.Exec(ctx, "UPDATE memberships SET role_code=$3,active=$4 WHERE organization_id=$1 AND user_id=$2", orgID, userID, role, active); err != nil {
		return err
	}
	if !active {
		if _, err = tx.Exec(ctx, "DELETE FROM sessions WHERE organization_id=$1 AND user_id=$2", orgID, userID); err != nil {
			return err
		}
	}
	if _, err = tx.Exec(ctx, "INSERT INTO audit_events(organization_id,actor_id,action,target_id) VALUES($1,$2,'admin.membership_updated',$3)", orgID, actor.UserID, userID); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (s *Service) SetUserStatus(ctx context.Context, actor tenancy.Principal, userID string, active bool) error {
	if err := requireAdmin(actor); err != nil {
		return err
	}
	if !uuidPattern.MatchString(userID) {
		return auth.ErrNotFound
	}
	if !active && userID == actor.UserID {
		return auth.ErrConflict
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(context.Background())
	var isPlatformAdmin bool
	var orgID string
	if err = tx.QueryRow(ctx, "SELECT platform_admin, COALESCE((SELECT organization_id::text FROM memberships WHERE user_id=$1 LIMIT 1), $2) FROM users WHERE id=$1 FOR UPDATE", userID, actor.OrganizationID).Scan(&isPlatformAdmin, &orgID); errors.Is(err, pgx.ErrNoRows) {
		return auth.ErrNotFound
	} else if err != nil {
		return err
	}
	if !active && isPlatformAdmin {
		var admins int
		if err = tx.QueryRow(ctx, "SELECT count(*) FROM users WHERE platform_admin AND active").Scan(&admins); err != nil {
			return err
		}
		if admins <= 1 {
			return auth.ErrConflict
		}
	}
	if _, err = tx.Exec(ctx, "UPDATE users SET active=$2 WHERE id=$1", userID, active); err != nil {
		return err
	}
	if !active {
		if _, err = tx.Exec(ctx, "DELETE FROM sessions WHERE user_id=$1", userID); err != nil {
			return err
		}
	}
	if _, err = tx.Exec(ctx, "INSERT INTO audit_events(organization_id,actor_id,action,target_id) VALUES($1,$2,'admin.user_status_changed',$3)", orgID, actor.UserID, userID); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (s *Service) SetPlatformAdmin(ctx context.Context, actor tenancy.Principal, userID string, platformAdmin bool) error {
	if err := requireAdmin(actor); err != nil {
		return err
	}
	if !uuidPattern.MatchString(userID) {
		return auth.ErrNotFound
	}
	if !platformAdmin && userID == actor.UserID {
		return auth.ErrConflict
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(context.Background())
	var orgID string
	if err = tx.QueryRow(ctx, "SELECT COALESCE((SELECT organization_id::text FROM memberships WHERE user_id=$1 LIMIT 1), $2) FROM users WHERE id=$1 FOR UPDATE", userID, actor.OrganizationID).Scan(&orgID); errors.Is(err, pgx.ErrNoRows) {
		return auth.ErrNotFound
	} else if err != nil {
		return err
	}
	if !platformAdmin {
		var admins int
		if err = tx.QueryRow(ctx, "SELECT count(*) FROM users WHERE platform_admin AND active").Scan(&admins); err != nil {
			return err
		}
		if admins <= 1 {
			return auth.ErrConflict
		}
	}
	if _, err = tx.Exec(ctx, "UPDATE users SET platform_admin=$2 WHERE id=$1", userID, platformAdmin); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, "INSERT INTO audit_events(organization_id,actor_id,action,target_id) VALUES($1,$2,'admin.user_platform_admin_changed',$3)", orgID, actor.UserID, userID); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (s *Service) RevokeUserSessions(ctx context.Context, actor tenancy.Principal, userID string) error {
	if err := requireAdmin(actor); err != nil {
		return err
	}
	if !uuidPattern.MatchString(userID) {
		return auth.ErrNotFound
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(context.Background())
	var orgID string
	if err = tx.QueryRow(ctx, "SELECT COALESCE((SELECT organization_id::text FROM memberships WHERE user_id=$1 LIMIT 1), $2) FROM users WHERE id=$1", userID, actor.OrganizationID).Scan(&orgID); errors.Is(err, pgx.ErrNoRows) {
		return auth.ErrNotFound
	} else if err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, "DELETE FROM sessions WHERE user_id=$1", userID); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, "INSERT INTO audit_events(organization_id,actor_id,action,target_id) VALUES($1,$2,'admin.user_sessions_revoked',$3)", orgID, actor.UserID, userID); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (s *Service) TriggerUserRecovery(ctx context.Context, actor tenancy.Principal, userID string) error {
	return s.identity.AdminTriggerRecovery(ctx, actor, userID)
}

type AdminKeyView struct {
	ID               string
	OrganizationID   string
	OrganizationSlug string
	Name             string
	Prefix           string
	Scopes           []string
	CreatedAt        time.Time
	LastUsed         *time.Time
	RevokedAt        *time.Time
}

type QuotaSummaryView struct {
	OrganizationID    string
	OrganizationSlug  string
	OrganizationName  string
	PlanCode          string
	MonthlyLimit      int
	RequestsPerMinute int
	Used              int
	Blocked           int
	CompletedEvents   int
	PendingEvents     int
	RefundedEvents    int
}

func (s *Service) ListKeysAndQuotas(ctx context.Context, actor tenancy.Principal, orgFilter string) ([]QuotaSummaryView, []AdminKeyView, error) {
	if err := requireAdmin(actor); err != nil {
		return nil, nil, err
	}
	orgFilter = strings.TrimSpace(orgFilter)
	qRows, err := s.pool.Query(ctx, `SELECT o.id::text,o.slug,o.name,l.plan_code,l.monthly_limit,l.requests_per_minute,
		COALESCE((SELECT used FROM quota_periods qp WHERE qp.organization_id=o.id AND qp.period=date_trunc('month',now() AT TIME ZONE 'America/Sao_Paulo')::date),0),
		COALESCE((SELECT blocked FROM quota_periods qp WHERE qp.organization_id=o.id AND qp.period=date_trunc('month',now() AT TIME ZONE 'America/Sao_Paulo')::date),0),
		(SELECT count(*) FROM usage_events ue WHERE ue.organization_id=o.id AND ue.state='completed' AND ue.period=date_trunc('month',now() AT TIME ZONE 'America/Sao_Paulo')::date),
		(SELECT count(*) FROM usage_events ue WHERE ue.organization_id=o.id AND ue.state='pending' AND ue.period=date_trunc('month',now() AT TIME ZONE 'America/Sao_Paulo')::date),
		(SELECT count(*) FROM usage_events ue WHERE ue.organization_id=o.id AND ue.state='refunded' AND ue.period=date_trunc('month',now() AT TIME ZONE 'America/Sao_Paulo')::date)
		FROM organizations o JOIN organization_limits l ON l.organization_id=o.id
		WHERE ($1='' OR o.id::text=$1 OR o.slug=$1)
		ORDER BY o.slug ASC LIMIT 200`, orgFilter)
	if err != nil {
		return nil, nil, err
	}
	defer qRows.Close()
	quotas := []QuotaSummaryView{}
	for qRows.Next() {
		var q QuotaSummaryView
		if err := qRows.Scan(&q.OrganizationID, &q.OrganizationSlug, &q.OrganizationName, &q.PlanCode, &q.MonthlyLimit, &q.RequestsPerMinute,
			&q.Used, &q.Blocked, &q.CompletedEvents, &q.PendingEvents, &q.RefundedEvents); err != nil {
			return nil, nil, err
		}
		quotas = append(quotas, q)
	}
	if err := qRows.Err(); err != nil {
		return nil, nil, err
	}
	kRows, err := s.pool.Query(ctx, `SELECT k.id::text,k.organization_id::text,o.slug,k.name,k.prefix,k.scopes,k.created_at,k.last_used_at,k.revoked_at
		FROM api_keys k JOIN organizations o ON o.id=k.organization_id
		WHERE ($1='' OR o.id::text=$1 OR o.slug=$1)
		ORDER BY k.created_at DESC LIMIT 200`, orgFilter)
	if err != nil {
		return nil, nil, err
	}
	defer kRows.Close()
	keys := []AdminKeyView{}
	for kRows.Next() {
		var k AdminKeyView
		if err := kRows.Scan(&k.ID, &k.OrganizationID, &k.OrganizationSlug, &k.Name, &k.Prefix, &k.Scopes, &k.CreatedAt, &k.LastUsed, &k.RevokedAt); err != nil {
			return nil, nil, err
		}
		keys = append(keys, k)
	}
	return quotas, keys, kRows.Err()
}

func (s *Service) IssueKey(ctx context.Context, actor tenancy.Principal, orgID, name string, scopes []string) (auth.Key, string, error) {
	return s.identity.AdminIssueKey(ctx, actor, orgID, name, scopes)
}

func (s *Service) RotateKey(ctx context.Context, actor tenancy.Principal, id string) (auth.Key, string, error) {
	return s.identity.AdminRotateKey(ctx, actor, id)
}

func (s *Service) RevokeKey(ctx context.Context, actor tenancy.Principal, id string) error {
	return s.identity.AdminRevokeKey(ctx, actor, id)
}

type AdminOperationView struct {
	ID               string
	OrganizationID   string
	OrganizationSlug string
	UserEmail        string
	Kind             string
	Title            string
	EngineVersion    string
	CreatedAt        time.Time
}

func (s *Service) ListOperations(ctx context.Context, actor tenancy.Principal, orgFilter, kindFilter string) ([]AdminOperationView, error) {
	if err := requireAdmin(actor); err != nil {
		return nil, err
	}
	rows, err := s.pool.Query(ctx, `SELECT op.id::text,op.organization_id::text,o.slug,u.email,op.kind,op.title,op.engine_version,op.created_at
		FROM legal_operations op JOIN organizations o ON o.id=op.organization_id JOIN users u ON u.id=op.user_id
		WHERE ($1='' OR o.id::text=$1 OR o.slug=$1) AND ($2='' OR op.kind=$2)
		ORDER BY op.created_at DESC, op.id DESC LIMIT 100`, strings.TrimSpace(orgFilter), strings.TrimSpace(kindFilter))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []AdminOperationView{}
	for rows.Next() {
		var op AdminOperationView
		if err := rows.Scan(&op.ID, &op.OrganizationID, &op.OrganizationSlug, &op.UserEmail, &op.Kind, &op.Title, &op.EngineVersion, &op.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, op)
	}
	return out, rows.Err()
}

type AuditEventView struct {
	ID               int64
	OrganizationID   string
	OrganizationSlug string
	ActorEmail       string
	Action           string
	TargetID         string
	CreatedAt        time.Time
}

func (s *Service) ListAuditEvents(ctx context.Context, actor tenancy.Principal, orgFilter, actionFilter, query string, beforeID int64) ([]AuditEventView, int64, error) {
	if err := requireAdmin(actor); err != nil {
		return nil, 0, err
	}
	if beforeID <= 0 {
		beforeID = 1<<62 - 1
	}
	q := "%" + strings.ToLower(strings.TrimSpace(query)) + "%"
	rows, err := s.pool.Query(ctx, `SELECT a.id,a.organization_id::text,o.slug,COALESCE(u.email,'sistema'),a.action,COALESCE(a.target_id::text,''),a.created_at
		FROM audit_events a JOIN organizations o ON o.id=a.organization_id LEFT JOIN users u ON u.id=a.actor_id
		WHERE a.id < $1
		AND ($2='' OR o.id::text=$2 OR o.slug=$2)
		AND ($3='' OR lower(a.action) LIKE '%' || lower($3) || '%')
		AND ($4='%%' OR lower(a.action) LIKE $4 OR lower(o.slug) LIKE $4 OR lower(COALESCE(u.email,'')) LIKE $4 OR lower(COALESCE(a.target_id::text,'')) LIKE $4)
		ORDER BY a.id DESC LIMIT 51`, beforeID, strings.TrimSpace(orgFilter), strings.TrimSpace(actionFilter), q)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	out := []AuditEventView{}
	for rows.Next() {
		var e AuditEventView
		if err := rows.Scan(&e.ID, &e.OrganizationID, &e.OrganizationSlug, &e.ActorEmail, &e.Action, &e.TargetID, &e.CreatedAt); err != nil {
			return nil, 0, err
		}
		out = append(out, e)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}
	var next int64
	if len(out) > 50 {
		next = out[49].ID
		out = out[:50]
	}
	return out, next, nil
}

type EconomicIndexView struct {
	Code       string
	Name       string
	Source     string
	Verified   bool
	PointCount int
	FirstMonth string
	LastMonth  string
}

type IndexSyncRun struct {
	ID           string
	ActorEmail   string
	IndexCode    string
	Source       string
	StartMonth   string
	EndMonth     string
	PointsSynced int
	Status       string
	ErrorMessage string
	DurationMs   int64
	CreatedAt    time.Time
}

func (s *Service) ListIndicesAndSyncs(ctx context.Context, actor tenancy.Principal) ([]EconomicIndexView, []IndexSyncRun, error) {
	if err := requireAdmin(actor); err != nil {
		return nil, nil, err
	}
	iRows, err := s.pool.Query(ctx, `SELECT ei.code,ei.name,ei.source,ei.verified,
		(SELECT count(*) FROM index_points ip WHERE ip.index_code=ei.code),
		COALESCE((SELECT min(to_char(reference_month,'YYYY-MM')) FROM index_points ip WHERE ip.index_code=ei.code),''),
		COALESCE((SELECT max(to_char(reference_month,'YYYY-MM')) FROM index_points ip WHERE ip.index_code=ei.code),'')
		FROM economic_indices ei ORDER BY ei.code ASC`)
	if err != nil {
		return nil, nil, err
	}
	defer iRows.Close()
	indices := []EconomicIndexView{}
	for iRows.Next() {
		var v EconomicIndexView
		if err := iRows.Scan(&v.Code, &v.Name, &v.Source, &v.Verified, &v.PointCount, &v.FirstMonth, &v.LastMonth); err != nil {
			return nil, nil, err
		}
		indices = append(indices, v)
	}
	if err := iRows.Err(); err != nil {
		return nil, nil, err
	}
	sRows, err := s.pool.Query(ctx, `SELECT r.id::text,COALESCE(u.email,'sistema'),r.index_code,r.source,r.start_month,r.end_month,r.points_synced,r.status,r.error_message,r.duration_ms,r.created_at
		FROM index_sync_runs r LEFT JOIN users u ON u.id=r.actor_id ORDER BY r.created_at DESC LIMIT 50`)
	if err != nil {
		return nil, nil, err
	}
	defer sRows.Close()
	runs := []IndexSyncRun{}
	for sRows.Next() {
		var r IndexSyncRun
		if err := sRows.Scan(&r.ID, &r.ActorEmail, &r.IndexCode, &r.Source, &r.StartMonth, &r.EndMonth, &r.PointsSynced, &r.Status, &r.ErrorMessage, &r.DurationMs, &r.CreatedAt); err != nil {
			return nil, nil, err
		}
		runs = append(runs, r)
	}
	return indices, runs, sRows.Err()
}

func (s *Service) SyncIndices(ctx context.Context, actor tenancy.Principal, source, startMonth, endMonth string) (IndexSyncRun, error) {
	var run IndexSyncRun
	if err := requireAdmin(actor); err != nil {
		return run, err
	}
	source = strings.ToUpper(strings.TrimSpace(source))
	startMonth = strings.TrimSpace(startMonth)
	endMonth = strings.TrimSpace(endMonth)
	if (source != "BCB" && source != "IBGE") || s.workspace == nil {
		return run, auth.ErrConflict
	}
	indexCode := "IPCA_" + source
	started := time.Now()
	res, syncErr := s.workspace.Sources().Index(ctx, juridico.IndexQuery{Source: source, Start: startMonth, End: endMonth})
	durationMs := time.Since(started).Milliseconds()
	if durationMs < 0 {
		durationMs = 0
	}
	status := "completed"
	errMsg := ""
	if syncErr != nil {
		status = "failed"
		errMsg = syncErr.Error()
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return run, err
	}
	defer tx.Rollback(context.Background())
	if syncErr == nil {
		if _, err = tx.Exec(ctx, `INSERT INTO economic_indices(code,name,source,verified) VALUES($1,$2,$3,false)
			ON CONFLICT(code) DO UPDATE SET name=EXCLUDED.name,source=EXCLUDED.source`, indexCode, res.Name, source); err != nil {
			return run, err
		}
		for _, pt := range res.Points {
			if _, err = tx.Exec(ctx, `INSERT INTO index_points(index_code,reference_month,percentage,source_url,collected_at)
				VALUES($1,($2 || '-01')::date,$3::numeric,$4,$5)
				ON CONFLICT(index_code,reference_month) DO UPDATE SET percentage=EXCLUDED.percentage,source_url=EXCLUDED.source_url,collected_at=EXCLUDED.collected_at`,
				indexCode, pt.Month, pt.Percent, res.URL, res.CollectedAt); err != nil {
				return run, err
			}
		}
	}
	err = tx.QueryRow(ctx, `INSERT INTO index_sync_runs(actor_id,index_code,source,start_month,end_month,points_synced,status,error_message,duration_ms)
		VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9) RETURNING id::text,created_at`,
		actor.UserID, indexCode, source, startMonth, endMonth, len(res.Points), status, errMsg, durationMs).Scan(&run.ID, &run.CreatedAt)
	if err != nil {
		return run, err
	}
	if _, err = tx.Exec(ctx, "INSERT INTO audit_events(organization_id,actor_id,action) VALUES($1,$2,$3)", actor.OrganizationID, actor.UserID, "admin.indices_sync:"+indexCode+":"+status); err != nil {
		return run, err
	}
	if err = tx.Commit(ctx); err != nil {
		return run, err
	}
	run.IndexCode = indexCode
	run.Source = source
	run.StartMonth = startMonth
	run.EndMonth = endMonth
	run.PointsSynced = len(res.Points)
	run.Status = status
	run.ErrorMessage = errMsg
	run.DurationMs = durationMs
	if syncErr != nil {
		return run, fmt.Errorf("%w: %s", juridico.ErrInput, syncErr)
	}
	return run, nil
}

type IntegrationConfigView struct {
	DataJudAPIKey     string
	DataJudConfigured bool
	DataJudSource     string
	DJENEnabled       bool
	DJENSource        string
	UpdatedAt         string
	UpdatedByEmail    string
}

func (s *Service) GetIntegrationConfig(ctx context.Context, actor tenancy.Principal) (IntegrationConfigView, error) {
	var cfg IntegrationConfigView
	if err := requireAdmin(actor); err != nil {
		return cfg, err
	}
	liveKey := ""
	liveDJEN := false
	if s.workspace != nil && s.workspace.Sources() != nil {
		liveKey, liveDJEN = s.workspace.Sources().Config()
	}

	var dbKey, dbDJEN, updatedByEmail string
	var updatedAt *time.Time
	rows, err := s.pool.Query(ctx, `SELECT s.key, s.value, s.updated_at, COALESCE(u.email, 'sistema')
		FROM system_settings s
		LEFT JOIN users u ON u.id = s.updated_by
		WHERE s.key IN ('datajud_api_key', 'djen_enabled')`)
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var k, v, email string
			var ts time.Time
			if err := rows.Scan(&k, &v, &ts, &email); err == nil {
				if k == "datajud_api_key" {
					dbKey = v
					if updatedAt == nil || ts.After(*updatedAt) {
						updatedAt = &ts
						updatedByEmail = email
					}
				} else if k == "djen_enabled" {
					dbDJEN = v
					if updatedAt == nil || ts.After(*updatedAt) {
						updatedAt = &ts
						updatedByEmail = email
					}
				}
			}
		}
	}

	if dbKey != "" {
		cfg.DataJudAPIKey = dbKey
		cfg.DataJudConfigured = true
		cfg.DataJudSource = "Banco de dados (persistida)"
	} else if liveKey != "" {
		cfg.DataJudAPIKey = liveKey
		cfg.DataJudConfigured = true
		cfg.DataJudSource = "Variável de ambiente (.env)"
	} else {
		cfg.DataJudAPIKey = ""
		cfg.DataJudConfigured = false
		cfg.DataJudSource = "Não configurada"
	}

	if dbDJEN != "" {
		cfg.DJENEnabled = dbDJEN == "true"
		cfg.DJENSource = "Banco de dados (persistida)"
	} else {
		cfg.DJENEnabled = liveDJEN
		if liveDJEN {
			cfg.DJENSource = "Variável de ambiente (.env)"
		} else {
			cfg.DJENSource = "Desabilitado por padrão"
		}
	}

	if updatedAt != nil {
		cfg.UpdatedAt = updatedAt.Format("02/01/2006 15:04:05")
		cfg.UpdatedByEmail = updatedByEmail
	}

	return cfg, nil
}

func (s *Service) SaveIntegrationConfig(ctx context.Context, actor tenancy.Principal, datajudKey string, djenEnabled bool) error {
	if err := requireAdmin(actor); err != nil {
		return err
	}
	cleanKey := strings.TrimSpace(datajudKey)
	cleanKey = strings.TrimPrefix(cleanKey, "APIKey ")
	djenVal := "false"
	if djenEnabled {
		djenVal = "true"
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(context.Background())

	if _, err := tx.Exec(ctx, `INSERT INTO system_settings(key, value, updated_at, updated_by)
		VALUES ('datajud_api_key', $1, now(), $2)
		ON CONFLICT (key) DO UPDATE SET value = EXCLUDED.value, updated_at = EXCLUDED.updated_at, updated_by = EXCLUDED.updated_by`,
		cleanKey, actor.UserID); err != nil {
		return err
	}

	if _, err := tx.Exec(ctx, `INSERT INTO system_settings(key, value, updated_at, updated_by)
		VALUES ('djen_enabled', $1, now(), $2)
		ON CONFLICT (key) DO UPDATE SET value = EXCLUDED.value, updated_at = EXCLUDED.updated_at, updated_by = EXCLUDED.updated_by`,
		djenVal, actor.UserID); err != nil {
		return err
	}

	action := fmt.Sprintf("admin.integrations_updated:djen=%v", djenEnabled)
	if _, err := tx.Exec(ctx, `INSERT INTO audit_events(organization_id, actor_id, action)
		VALUES ($1, $2, $3)`, actor.OrganizationID, actor.UserID, action); err != nil {
		return err
	}

	if err := tx.Commit(ctx); err != nil {
		return err
	}

	if s.workspace != nil && s.workspace.Sources() != nil {
		s.workspace.Sources().UpdateConfig(cleanKey, djenEnabled)
	}

	return nil
}
