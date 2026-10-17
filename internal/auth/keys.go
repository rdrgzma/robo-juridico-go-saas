package auth

import (
	"context"
	"errors"
	"github.com/jackc/pgx/v5"
	"github.com/rdrgzma/robo-juridico-go-saas/internal/tenancy"
	"strings"
	"time"
)

type Key struct {
	ID        string     `json:"id"`
	Name      string     `json:"name"`
	Prefix    string     `json:"prefix"`
	Scopes    []string   `json:"scopes"`
	RevokedAt *time.Time `json:"revoked_at"`
	LastUsed  *time.Time `json:"last_used_at"`
}

func (s *Service) IssueKey(ctx context.Context, p tenancy.Principal, name string, scopes []string) (Key, string, error) {
	var key Key
	if len(name) < 1 || len(name) > 100 || len(scopes) == 0 || len(scopes) > 2 {
		return key, "", ErrConflict
	}
	seen := map[string]bool{}
	for _, scope := range scopes {
		if (scope != "account:read" && scope != "calculations:run") || seen[scope] {
			return key, "", ErrDenied
		}
		seen[scope] = true
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return key, "", err
	}
	defer tx.Rollback(context.Background())
	if _, err = tx.Exec(ctx, "SELECT id FROM organizations WHERE id=$1 FOR UPDATE", p.OrganizationID); err != nil {
		return key, "", err
	}
	if !authorized(ctx, tx, p, "keys:manage") {
		return key, "", ErrDenied
	}
	var count int
	if err = tx.QueryRow(ctx, "SELECT count(*) FROM api_keys WHERE organization_id=$1 AND revoked_at IS NULL", p.OrganizationID).Scan(&count); err != nil {
		return key, "", err
	}
	if count >= 100 {
		return key, "", ErrConflict
	}
	token := "rj_" + RandomToken()
	key.Name = name
	key.Prefix = token[:11]
	key.Scopes = scopes
	if err = tx.QueryRow(ctx, "INSERT INTO api_keys(organization_id,created_by,name,prefix,token_hash,scopes) VALUES($1,$2,$3,$4,$5,$6) RETURNING id::text", p.OrganizationID, p.UserID, name, key.Prefix, Digest(token), scopes).Scan(&key.ID); err != nil {
		return key, "", err
	}
	if err = audit(ctx, tx, p, "api_key.created", key.ID); err != nil {
		return key, "", err
	}
	return key, token, tx.Commit(ctx)
}
func (s *Service) Keys(ctx context.Context, p tenancy.Principal) ([]Key, error) {
	if !p.Can("keys:manage") || p.RequiresMFA() {
		return nil, ErrDenied
	}
	rows, err := s.pool.Query(ctx, "SELECT id::text,name,prefix,scopes,revoked_at,last_used_at FROM api_keys WHERE organization_id=$1 ORDER BY created_at DESC LIMIT 200", p.OrganizationID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	keys := []Key{}
	for rows.Next() {
		var k Key
		if err := rows.Scan(&k.ID, &k.Name, &k.Prefix, &k.Scopes, &k.RevokedAt, &k.LastUsed); err != nil {
			return nil, err
		}
		keys = append(keys, k)
	}
	return keys, rows.Err()
}
func (s *Service) RevokeKey(ctx context.Context, p tenancy.Principal, id string) error {
	if !uuidPattern.MatchString(id) {
		return ErrNotFound
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(context.Background())
	if !authorized(ctx, tx, p, "keys:manage") {
		return ErrDenied
	}
	tag, err := tx.Exec(ctx, "UPDATE api_keys SET revoked_at=$3 WHERE organization_id=$1 AND id=$2 AND revoked_at IS NULL", p.OrganizationID, id, s.now())
	if err != nil {
		return err
	}
	if tag.RowsAffected() != 1 {
		return ErrNotFound
	}
	if err = audit(ctx, tx, p, "api_key.revoked", id); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

type APIAccess struct {
	Principal tenancy.Principal
	EventID   string
}

func (s *Service) BeginAPI(ctx context.Context, token, scope, ip string, billable bool) (APIAccess, error) {
	var access APIAccess
	if len(token) < 32 || len(token) > 200 {
		return access, ErrCredentials
	}
	if err := s.attempt(ctx, "api-ip", ip, 300, time.Minute); err != nil {
		return access, err
	}
	p := &access.Principal
	var keyID string
	var scopes []string
	err := s.pool.QueryRow(ctx, `SELECT k.id::text,u.id::text,o.id::text,o.slug,u.email,m.role_code,k.scopes,
 ARRAY(SELECT permission_code FROM role_permissions WHERE role_code=m.role_code ORDER BY permission_code)
 FROM api_keys k JOIN users u ON u.id=k.created_by JOIN organizations o ON o.id=k.organization_id
 JOIN memberships m ON m.organization_id=k.organization_id AND m.user_id=k.created_by
 WHERE k.token_hash=$1 AND k.revoked_at IS NULL AND u.active AND m.active AND o.active`, Digest(token)).Scan(&keyID, &p.UserID, &p.OrganizationID, &p.Organization, &p.Email, &p.Role, &scopes, &p.Permissions)
	if errors.Is(err, pgx.ErrNoRows) {
		return access, ErrCredentials
	}
	if err != nil {
		return access, err
	}
	allowed := false
	for _, v := range scopes {
		if v == scope {
			allowed = true
		}
	}
	if !allowed || !p.Can(scope) {
		return access, ErrDenied
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return access, err
	}
	defer tx.Rollback(context.Background())
	var monthly, rpm int
	if err = tx.QueryRow(ctx, "SELECT monthly_limit,requests_per_minute FROM organization_limits WHERE organization_id=$1 FOR UPDATE", p.OrganizationID).Scan(&monthly, &rpm); err != nil {
		return access, err
	}
	// A key revoked while this transaction waits must not acquire a new reservation.
	var valid bool
	if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM api_keys k JOIN memberships m ON m.organization_id=k.organization_id AND m.user_id=k.created_by
 JOIN organizations o ON o.id=k.organization_id JOIN users u ON u.id=k.created_by WHERE k.id=$1 AND k.revoked_at IS NULL AND o.active AND m.active AND u.active
 AND EXISTS(SELECT 1 FROM role_permissions WHERE role_code=m.role_code AND permission_code=$2))`, keyID, scope).Scan(&valid); err != nil {
		return access, err
	}
	if !valid {
		return access, ErrDenied
	}
	var attempts int
	err = tx.QueryRow(ctx, `INSERT INTO auth_buckets(purpose,bucket_hash,window_start,attempts) VALUES('api-org',$1,$2,1)
 ON CONFLICT(purpose,bucket_hash,window_start) DO UPDATE SET attempts=LEAST(auth_buckets.attempts+1,1000000) RETURNING attempts`, Digest(p.OrganizationID), s.now().Truncate(time.Minute)).Scan(&attempts)
	if err != nil {
		return access, err
	}
	if attempts > rpm {
		if err = tx.Commit(ctx); err != nil {
			return access, err
		}
		return access, ErrRate
	}
	now := s.now()
	period := now.In(s.location).Format("2006-01") + "-01"
	if _, err = tx.Exec(ctx, "INSERT INTO quota_periods(organization_id,period) VALUES($1,$2::date) ON CONFLICT DO NOTHING", p.OrganizationID, period); err != nil {
		return access, err
	}
	if billable {
		tag, err := tx.Exec(ctx, "UPDATE quota_periods SET used=used+1 WHERE organization_id=$1 AND period=$2::date AND used<$3", p.OrganizationID, period, monthly)
		if err != nil {
			return access, err
		}
		if tag.RowsAffected() != 1 {
			if err = tx.Commit(ctx); err != nil {
				return access, err
			}
			return access, ErrQuota
		}
	}
	if err = tx.QueryRow(ctx, "INSERT INTO usage_events(organization_id,key_id,period,billable,state,created_at) VALUES($1,$2,$3::date,$4,'pending',$5) RETURNING id::text", p.OrganizationID, keyID, period, billable, now).Scan(&access.EventID); err != nil {
		return access, err
	}
	if _, err = tx.Exec(ctx, "UPDATE api_keys SET last_used_at=$2 WHERE id=$1", keyID, now); err != nil {
		return access, err
	}
	return access, tx.Commit(ctx)
}
func (s *Service) FinishAPI(ctx context.Context, eventID string, status int) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(context.Background())
	var org, period, state string
	var billable bool
	err = tx.QueryRow(ctx, "SELECT organization_id::text,period::text,state,billable FROM usage_events WHERE id=$1 FOR UPDATE", eventID).Scan(&org, &period, &state, &billable)
	if err != nil {
		return err
	}
	if state != "pending" {
		return nil
	}
	state = "completed"
	if status < 200 || status >= 300 {
		state = "refunded"
		if billable {
			if _, err = tx.Exec(ctx, "UPDATE quota_periods SET used=used-1 WHERE organization_id=$1 AND period=$2::date AND used>0", org, period); err != nil {
				return err
			}
		}
	}
	if _, err = tx.Exec(ctx, "UPDATE usage_events SET state=$2,http_status=$3 WHERE id=$1", eventID, state, status); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
func (s *Service) Reconcile(ctx context.Context) error {
	rows, err := s.pool.Query(ctx, "SELECT id::text FROM usage_events WHERE state='pending' AND created_at<$1 ORDER BY created_at LIMIT 100", s.now().Add(-10*time.Minute))
	if err != nil {
		return err
	}
	ids := []string{}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return err
		}
		ids = append(ids, id)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}
	for _, id := range ids {
		if err := s.FinishAPI(ctx, id, 503); err != nil {
			return err
		}
	}
	_, err = s.pool.Exec(ctx, "DELETE FROM auth_buckets WHERE window_start<$1", s.now().Add(-48*time.Hour))
	return err
}
func APIRequirement(path, method string) (string, bool) {
	if strings.Contains(path, "/cnj/") {
		return "calculations:run", false
	}
	if method == "POST" && (strings.Contains(path, "/calculos/") || strings.Contains(path, "/simulacoes/")) {
		return "calculations:run", true
	}
	return "account:read", false
}
