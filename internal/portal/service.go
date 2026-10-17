// Package portal implements the tenant-scoped self-service client portal (/portal).
package portal

import (
	"context"
	"errors"
	"net/mail"
	"regexp"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rdrgzma/robo-juridico-go-saas/internal/auth"
	"github.com/rdrgzma/robo-juridico-go-saas/internal/juridico"
	"github.com/rdrgzma/robo-juridico-go-saas/internal/tenancy"
	"github.com/shopspring/decimal"
)

var idPattern = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

type Service struct {
	pool      *pgxpool.Pool
	identity  *auth.Service
	workspace *juridico.Service
}

func New(pool *pgxpool.Pool, identity *auth.Service, workspace *juridico.Service) *Service {
	return &Service{pool: pool, identity: identity, workspace: workspace}
}

func requirePortalAccess(actor tenancy.Principal) error {
	if !actor.Can("account:read") || actor.RequiresMFA() || actor.OrganizationID == "" {
		return auth.ErrDenied
	}
	return nil
}

type DashboardView struct {
	OrgName            string
	OrgSlug            string
	LegalName          string
	Document           string
	ContactName        string
	ContactEmail       string
	PlanCode           string
	PlanName           string
	Features           []string
	PlanActive         bool
	ContractedPrice    decimal.Decimal
	EffectiveFrom      string
	EffectiveUntil     string
	ContractActive     bool
	APIAllowed         bool
	MonthlyLimit       int
	MonthUsed          int
	MonthAvailable     int
	MonthBlocked       int
	RequestsPerMinute  int
	MonthQueries       int
	MonthCalculations  int
	TotalOperations    int
	MemoryReportsCount int
	HomologatedReports int
	MonthAPICompleted  int
	MonthAPIPending    int
	MonthAPIRefunded   int
	MonthAPIFree       int
}

func (s *Service) Dashboard(ctx context.Context, actor tenancy.Principal) (DashboardView, error) {
	var d DashboardView
	if err := requirePortalAccess(actor); err != nil {
		return d, err
	}
	err := s.pool.QueryRow(ctx, `SELECT
		o.name, o.slug, o.legal_name, o.document, o.contact_name, o.contact_email,
		l.plan_code, p.name, p.features, p.active, l.contracted_price,
		l.effective_from::text, COALESCE(l.effective_until::text, ''),
		(o.active AND p.active AND (now() AT TIME ZONE 'America/Sao_Paulo')::date >= l.effective_from AND (l.effective_until IS NULL OR (now() AT TIME ZONE 'America/Sao_Paulo')::date <= l.effective_until)),
		('api' = ANY(p.features)),
		l.monthly_limit,
		COALESCE((SELECT used FROM quota_periods qp WHERE qp.organization_id=o.id AND qp.period=date_trunc('month', now() AT TIME ZONE 'America/Sao_Paulo')::date), 0),
		COALESCE((SELECT blocked FROM quota_periods qp WHERE qp.organization_id=o.id AND qp.period=date_trunc('month', now() AT TIME ZONE 'America/Sao_Paulo')::date), 0),
		l.requests_per_minute,
		(SELECT count(*) FROM legal_operations lo WHERE lo.organization_id=o.id AND lo.kind IN ('cnj','indices') AND lo.created_at >= date_trunc('month', now() AT TIME ZONE 'America/Sao_Paulo')),
		(SELECT count(*) FROM legal_operations lo WHERE lo.organization_id=o.id AND lo.kind NOT IN ('cnj','indices') AND lo.created_at >= date_trunc('month', now() AT TIME ZONE 'America/Sao_Paulo')),
		(SELECT count(*) FROM legal_operations lo WHERE lo.organization_id=o.id),
		(SELECT count(*) FROM usage_events ue WHERE ue.organization_id=o.id AND ue.state='completed' AND ue.period=date_trunc('month', now() AT TIME ZONE 'America/Sao_Paulo')::date),
		(SELECT count(*) FROM usage_events ue WHERE ue.organization_id=o.id AND ue.state='pending' AND ue.period=date_trunc('month', now() AT TIME ZONE 'America/Sao_Paulo')::date),
		(SELECT count(*) FROM usage_events ue WHERE ue.organization_id=o.id AND ue.state='refunded' AND ue.period=date_trunc('month', now() AT TIME ZONE 'America/Sao_Paulo')::date),
		(SELECT count(*) FROM usage_events ue WHERE ue.organization_id=o.id AND NOT ue.billable AND ue.period=date_trunc('month', now() AT TIME ZONE 'America/Sao_Paulo')::date)
		FROM organizations o
		JOIN organization_limits l ON l.organization_id=o.id
		JOIN plans p ON p.code=l.plan_code
		WHERE o.id=$1`, actor.OrganizationID).Scan(
		&d.OrgName, &d.OrgSlug, &d.LegalName, &d.Document, &d.ContactName, &d.ContactEmail,
		&d.PlanCode, &d.PlanName, &d.Features, &d.PlanActive, &d.ContractedPrice,
		&d.EffectiveFrom, &d.EffectiveUntil, &d.ContractActive, &d.APIAllowed,
		&d.MonthlyLimit, &d.MonthUsed, &d.MonthBlocked, &d.RequestsPerMinute,
		&d.MonthQueries, &d.MonthCalculations, &d.TotalOperations,
		&d.MonthAPICompleted, &d.MonthAPIPending, &d.MonthAPIRefunded, &d.MonthAPIFree,
	)
	if err != nil {
		return d, err
	}
	d.APIAllowed = d.APIAllowed && d.ContractActive
	d.MonthAvailable = d.MonthlyLimit - d.MonthUsed
	if d.MonthAvailable < 0 {
		d.MonthAvailable = 0
	}
	d.MemoryReportsCount = d.TotalOperations
	d.HomologatedReports = 0
	return d, nil
}

type UsageEntry struct {
	ID            string
	Source        string
	Module        string
	Description   string
	Actor         string
	State         string
	Billable      bool
	UnitsConsumed int
	HTTPStatus    int
	CreatedAt     time.Time
}

func (s *Service) UsageHistory(ctx context.Context, actor tenancy.Principal, sourceFilter, stateFilter string, before time.Time) ([]UsageEntry, string, error) {
	if err := requirePortalAccess(actor); err != nil {
		return nil, "", err
	}
	if before.IsZero() {
		before = time.Now().Add(time.Minute)
	}
	sourceFilter = strings.TrimSpace(strings.ToLower(sourceFilter))
	stateFilter = strings.TrimSpace(strings.ToLower(stateFilter))

	rows, err := s.pool.Query(ctx, `SELECT id, source, module, description, actor, state, billable, http_status, created_at FROM (
		SELECT ue.id::text AS id, 'api' AS source, ue.module AS module,
			('Chave ' || k.name || ' (' || k.prefix || ')') AS description,
			k.prefix AS actor, ue.state AS state, ue.billable AS billable,
			COALESCE(ue.http_status, 0) AS http_status, ue.created_at AS created_at
		FROM usage_events ue JOIN api_keys k ON k.id=ue.key_id AND k.organization_id=ue.organization_id
		WHERE ue.organization_id=$1
		UNION ALL
		SELECT lo.id::text AS id, 'panel' AS source, lo.kind AS module,
			lo.title AS description, u.email AS actor, 'completed' AS state,
			(lo.kind NOT IN ('cnj','indices')) AS billable, 200 AS http_status, lo.created_at AS created_at
		FROM legal_operations lo JOIN users u ON u.id=lo.user_id
		WHERE lo.organization_id=$1
	) combined
	WHERE created_at < $2
	AND ($3 = '' OR $3 = 'all' OR source = $3)
	AND ($4 = '' OR $4 = 'all' OR ($4 = 'free' AND NOT billable) OR state = $4)
	ORDER BY created_at DESC, id DESC LIMIT 51`, actor.OrganizationID, before, sourceFilter, stateFilter)
	if err != nil {
		return nil, "", err
	}
	defer rows.Close()
	out := []UsageEntry{}
	for rows.Next() {
		var e UsageEntry
		if err := rows.Scan(&e.ID, &e.Source, &e.Module, &e.Description, &e.Actor, &e.State, &e.Billable, &e.HTTPStatus, &e.CreatedAt); err != nil {
			return nil, "", err
		}
		if e.Billable && e.State != "refunded" {
			e.UnitsConsumed = 1
		}
		out = append(out, e)
	}
	if err := rows.Err(); err != nil {
		return nil, "", err
	}
	next := ""
	if len(out) > 50 {
		next = out[49].CreatedAt.UTC().Format(time.RFC3339Nano)
		out = out[:50]
	}
	return out, next, nil
}

type PortalKeysData struct {
	CanManageKeys bool
	APIAllowed    bool
	PlanCode      string
	PlanName      string
	Keys          []auth.Key
}

func (s *Service) KeysData(ctx context.Context, actor tenancy.Principal) (PortalKeysData, error) {
	var data PortalKeysData
	if err := requirePortalAccess(actor); err != nil {
		return data, err
	}
	dash, err := s.Dashboard(ctx, actor)
	if err != nil {
		return data, err
	}
	data.CanManageKeys = actor.Can("keys:manage")
	data.APIAllowed = dash.APIAllowed
	data.PlanCode = dash.PlanCode
	data.PlanName = dash.PlanName
	if data.CanManageKeys {
		keys, err := s.identity.Keys(ctx, actor)
		if err != nil {
			return data, err
		}
		data.Keys = keys
	}
	return data, nil
}

func (s *Service) IssueKey(ctx context.Context, actor tenancy.Principal, name string, scopes []string) (auth.Key, string, error) {
	if err := requirePortalAccess(actor); err != nil {
		return auth.Key{}, "", err
	}
	return s.identity.IssueKey(ctx, actor, name, scopes)
}

func (s *Service) RotateKey(ctx context.Context, actor tenancy.Principal, id string) (auth.Key, string, error) {
	if err := requirePortalAccess(actor); err != nil {
		return auth.Key{}, "", err
	}
	return s.identity.RotateKey(ctx, actor, id)
}

func (s *Service) RevokeKey(ctx context.Context, actor tenancy.Principal, id string) error {
	if err := requirePortalAccess(actor); err != nil {
		return err
	}
	return s.identity.RevokeKey(ctx, actor, id)
}

func (s *Service) ListOperations(ctx context.Context, actor tenancy.Principal, kindFilter string, before time.Time, cursorID string) ([]juridico.Operation, string, error) {
	if err := requirePortalAccess(actor); err != nil {
		return nil, "", err
	}
	if before.IsZero() {
		before = time.Now().Add(time.Second)
	}
	if !idPattern.MatchString(cursorID) {
		cursorID = "ffffffff-ffff-ffff-ffff-ffffffffffff"
	}
	kindFilter = strings.TrimSpace(strings.ToLower(kindFilter))
	rows, err := s.pool.Query(ctx, `SELECT id::text,organization_id::text,user_id::text,kind,title,engine_version,created_at
		FROM legal_operations
		WHERE organization_id=$1 AND ($2='' OR kind=$2) AND (created_at,id)<($3,$4::uuid)
		ORDER BY created_at DESC, id DESC LIMIT 51`, actor.OrganizationID, kindFilter, before, cursorID)
	if err != nil {
		return nil, "", err
	}
	defer rows.Close()
	out := []juridico.Operation{}
	for rows.Next() {
		var o juridico.Operation
		if err := rows.Scan(&o.ID, &o.OrganizationID, &o.UserID, &o.Kind, &o.Title, &o.EngineVersion, &o.CreatedAt); err != nil {
			return nil, "", err
		}
		out = append(out, o)
	}
	if err := rows.Err(); err != nil {
		return nil, "", err
	}
	next := ""
	if len(out) > 50 {
		next = out[49].CreatedAt.UTC().Format(time.RFC3339Nano) + "_" + out[49].ID
		out = out[:50]
	}
	return out, next, nil
}

func (s *Service) GetOperation(ctx context.Context, actor tenancy.Principal, id string) (juridico.Operation, string, error) {
	if err := requirePortalAccess(actor); err != nil {
		return juridico.Operation{}, "", err
	}
	if s.workspace == nil {
		return juridico.Operation{}, "", auth.ErrNotFound
	}
	o, err := s.workspace.Get(ctx, actor, id)
	if err != nil {
		return o, "", err
	}
	return o, s.workspace.Integrity(o), nil
}

type OrganizationSettingsInput struct {
	Name         string
	LegalName    string
	Document     string
	ContactName  string
	ContactEmail string
}

func (s *Service) UpdateOrganizationSettings(ctx context.Context, actor tenancy.Principal, in OrganizationSettingsInput) error {
	if err := requirePortalAccess(actor); err != nil {
		return err
	}
	if !actor.Can("members:manage") {
		return auth.ErrDenied
	}
	name := strings.TrimSpace(in.Name)
	legalName := strings.TrimSpace(in.LegalName)
	doc := strings.TrimSpace(in.Document)
	contactName := strings.TrimSpace(in.ContactName)
	contactEmail := strings.ToLower(strings.TrimSpace(in.ContactEmail))
	if len(name) < 2 || len(name) > 160 || len(legalName) > 200 || len(doc) > 40 || len(contactName) > 160 || len(contactEmail) > 254 {
		return auth.ErrConflict
	}
	if contactEmail != "" {
		if _, err := mail.ParseAddress(contactEmail); err != nil {
			return auth.ErrConflict
		}
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(context.Background())
	var canManage bool
	if err = tx.QueryRow(ctx, `SELECT EXISTS(
		SELECT 1 FROM memberships m JOIN organizations o ON o.id=m.organization_id JOIN users u ON u.id=m.user_id
		WHERE m.organization_id=$1 AND m.user_id=$2 AND m.active AND o.active AND u.active
		AND EXISTS(SELECT 1 FROM role_permissions rp WHERE rp.role_code=m.role_code AND rp.permission_code='members:manage')
	)`, actor.OrganizationID, actor.UserID).Scan(&canManage); err != nil {
		return err
	}
	if !canManage {
		return auth.ErrDenied
	}
	tag, err := tx.Exec(ctx, `UPDATE organizations SET name=$2,legal_name=$3,document=$4,contact_name=$5,contact_email=$6 WHERE id=$1`,
		actor.OrganizationID, name, legalName, doc, contactName, contactEmail)
	if err != nil {
		return err
	}
	if tag.RowsAffected() != 1 {
		return errors.New("organização não encontrada")
	}
	if _, err = tx.Exec(ctx, "INSERT INTO audit_events(organization_id,actor_id,action,target_id) VALUES($1,$2,'portal.organization_updated',$1)", actor.OrganizationID, actor.UserID); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
