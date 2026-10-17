package auth

import (
	"context"
	"errors"
	"fmt"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rdrgzma/robo-juridico-go-saas/internal/tenancy"
	"net"
	"net/mail"
	"net/url"
	"regexp"
	"strings"
	"time"
	_ "time/tzdata"
)

var (
	ErrDenied      = errors.New("acesso negado")
	ErrCredentials = errors.New("credenciais inválidas")
	ErrRate        = errors.New("muitas tentativas; aguarde")
	ErrNotFound    = errors.New("registro não encontrado")
	ErrConflict    = errors.New("operação não permitida no estado atual")
	ErrQuota       = errors.New("franquia mensal esgotada")
	slugPattern    = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{1,62}$`)
	uuidPattern    = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)
)

type Options struct {
	Origin            string
	CookieSecure      bool
	DisableMFA        bool
	Clock             func() time.Time
	DevelopmentLogins map[string]DevelopmentLogin
}
type Service struct {
	pool     *pgxpool.Pool
	key      []byte
	options  Options
	dummy    string
	location *time.Location
}

func New(pool *pgxpool.Pool, key []byte, options Options) (*Service, error) {
	origin, err := url.Parse(options.Origin)
	if err != nil || origin.Host == "" || origin.User != nil || origin.RawQuery != "" || origin.Fragment != "" || origin.Path != "" || (origin.Scheme != "http" && origin.Scheme != "https") {
		return nil, fmt.Errorf("PUBLIC_ORIGIN inválida")
	}
	if options.DisableMFA {
		ip := net.ParseIP(origin.Hostname())
		if origin.Hostname() != "localhost" && (ip == nil || !ip.IsLoopback()) {
			return nil, fmt.Errorf("dispensa de MFA permitida apenas em origem local de desenvolvimento")
		}
	}
	if len(key) != 32 || pool == nil {
		return nil, fmt.Errorf("pool e chave de autenticação de 32 bytes obrigatórios")
	}
	if origin.Scheme == "https" && !options.CookieSecure {
		return nil, fmt.Errorf("HTTPS exige cookie Secure")
	}
	if options.Clock == nil {
		options.Clock = time.Now
	}
	location, err := time.LoadLocation("America/Sao_Paulo")
	if err != nil {
		return nil, err
	}
	dummy, err := HashPassword(RandomToken())
	if err != nil {
		return nil, err
	}
	return &Service{pool: pool, key: append([]byte(nil), key...), options: options, dummy: dummy, location: location}, nil
}
func normalizeEmail(input string) (string, error) {
	s := strings.ToLower(strings.TrimSpace(input))
	a, err := mail.ParseAddress(s)
	if err != nil || a.Address != s || len(s) > 254 {
		return "", ErrCredentials
	}
	return s, nil
}
func validRole(role string) bool {
	return role == "owner" || role == "admin" || role == "lawyer" || role == "viewer"
}
func stringsLower(value string) string { return strings.ToLower(strings.TrimSpace(value)) }
func (s *Service) now() time.Time      { return s.options.Clock().UTC() }
func (s *Service) attempt(ctx context.Context, purpose, key string, limit int, window time.Duration) error {
	now := s.now()
	start := now.Truncate(window)
	var count int
	err := s.pool.QueryRow(ctx, `INSERT INTO auth_buckets(purpose,bucket_hash,window_start,attempts) VALUES($1,$2,$3,1)
 ON CONFLICT(purpose,bucket_hash,window_start) DO UPDATE SET attempts=LEAST(auth_buckets.attempts+1,1000000) RETURNING attempts`, purpose, Digest(s.mac(purpose, key)), start).Scan(&count)
	if err != nil {
		return err
	}
	if count > limit {
		return ErrRate
	}
	return nil
}
func audit(ctx context.Context, tx pgx.Tx, p tenancy.Principal, action, target string) error {
	_, err := tx.Exec(ctx, "INSERT INTO audit_events(organization_id,actor_id,action,target_id) VALUES($1,$2,$3,NULLIF($4,'')::uuid)", p.OrganizationID, p.UserID, action, target)
	return err
}
func authorized(ctx context.Context, tx pgx.Tx, p tenancy.Principal, permission string) bool {
	var yes bool
	err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM memberships m JOIN organizations o ON o.id=m.organization_id JOIN users u ON u.id=m.user_id
 JOIN role_permissions rp ON rp.role_code=m.role_code WHERE m.organization_id=$1 AND m.user_id=$2 AND m.active AND u.active AND o.active AND rp.permission_code=$3)`, p.OrganizationID, p.UserID, permission).Scan(&yes)
	return err == nil && yes && !p.RequiresMFA()
}

// CreateOrganization is a provisioning operation, exposed only by the development CLI.
func (s *Service) CreateOrganization(ctx context.Context, slug, name, email, password string, platformAdmin bool) (tenancy.Principal, error) {
	var p tenancy.Principal
	email, err := normalizeEmail(email)
	if err != nil || !slugPattern.MatchString(slug) || len(name) < 1 || len(name) > 200 {
		return p, ErrConflict
	}
	hash, err := HashPassword(password)
	if err != nil {
		return p, err
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return p, err
	}
	defer tx.Rollback(context.Background())
	if err = tx.QueryRow(ctx, "INSERT INTO organizations(slug,name) VALUES($1,$2) RETURNING id::text", slug, name).Scan(&p.OrganizationID); err != nil {
		return p, err
	}
	if err = tx.QueryRow(ctx, "INSERT INTO users(email,password_hash,platform_admin) VALUES($1,$2,$3) RETURNING id::text", email, hash, platformAdmin).Scan(&p.UserID); err != nil {
		return p, err
	}
	if _, err = tx.Exec(ctx, "INSERT INTO memberships(organization_id,user_id,role_code) VALUES($1,$2,'owner')", p.OrganizationID, p.UserID); err != nil {
		return p, err
	}
	if _, err = tx.Exec(ctx, "INSERT INTO organization_limits(organization_id) VALUES($1)", p.OrganizationID); err != nil {
		return p, err
	}
	p.Organization = slug
	p.Email = email
	p.Role = "owner"
	p.PlatformAdmin = platformAdmin
	p.MFADisabled = s.options.DisableMFA
	if err = audit(ctx, tx, p, "organization.created", p.OrganizationID); err != nil {
		return p, err
	}
	return p, tx.Commit(ctx)
}

func (s *Service) Login(ctx context.Context, org, email, password, ip string) (string, error) {
	email = strings.ToLower(strings.TrimSpace(email))
	org = strings.ToLower(strings.TrimSpace(org))
	if err := s.attempt(ctx, "login-ip", ip, 60, 15*time.Minute); err != nil {
		return "", err
	}
	if err := s.attempt(ctx, "login-account", org+":"+email, 10, 15*time.Minute); err != nil {
		return "", err
	}
	var p tenancy.Principal
	var hash string
	var version int
	err := s.pool.QueryRow(ctx, `SELECT u.id::text,o.id::text,u.password_hash,u.password_version,u.platform_admin,(u.mfa_secret IS NOT NULL)
 FROM users u JOIN memberships m ON m.user_id=u.id JOIN organizations o ON o.id=m.organization_id
 WHERE lower(u.email)=$1 AND o.slug=$2 AND u.active AND m.active AND o.active`, email, org).Scan(&p.UserID, &p.OrganizationID, &hash, &version, &p.PlatformAdmin, &p.MFAEnabled)
	if err != nil {
		_ = VerifyPassword(s.dummy, password)
		if errors.Is(err, pgx.ErrNoRows) {
			return "", ErrCredentials
		}
		return "", err
	}
	if !VerifyPassword(hash, password) {
		return "", ErrCredentials
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return "", err
	}
	defer tx.Rollback(context.Background())
	token, err := s.issueSession(ctx, tx, p, version, false)
	if err != nil {
		return "", err
	}
	if err = audit(ctx, tx, p, "auth.login", p.UserID); err != nil {
		return "", err
	}
	return token, tx.Commit(ctx)
}
func (s *Service) issueSession(ctx context.Context, tx pgx.Tx, p tenancy.Principal, version int, verified bool) (string, error) {
	token := RandomToken()
	now := s.now()
	duration := 12 * time.Hour
	if !s.options.DisableMFA && (p.PlatformAdmin || p.MFAEnabled) && !verified {
		duration = 5 * time.Minute
	}
	tag, err := tx.Exec(ctx, `INSERT INTO sessions(token_hash,organization_id,user_id,password_version,mfa_verified,created_at,last_seen,expires_at)
 SELECT $1,$2,u.id,u.password_version,$4,$5,$5,$6 FROM users u JOIN memberships m ON m.user_id=u.id
 JOIN organizations o ON o.id=m.organization_id WHERE u.id=$3 AND m.organization_id=$2 AND u.password_version=$7 AND u.active AND m.active AND o.active`, Digest(token), p.OrganizationID, p.UserID, verified, now, now.Add(duration), version)
	if err != nil {
		return "", err
	}
	if tag.RowsAffected() != 1 {
		return "", ErrCredentials
	}
	return token, nil
}
func (s *Service) Session(ctx context.Context, token string) (tenancy.Principal, error) {
	var p tenancy.Principal
	if len(token) != 43 {
		return p, ErrDenied
	}
	now := s.now()
	err := s.pool.QueryRow(ctx, `SELECT u.id::text,o.id::text,o.slug,u.email,m.role_code,u.platform_admin,se.mfa_verified,(u.mfa_secret IS NOT NULL),
 ARRAY(SELECT permission_code FROM role_permissions WHERE role_code=m.role_code ORDER BY permission_code)
 FROM sessions se JOIN users u ON u.id=se.user_id JOIN organizations o ON o.id=se.organization_id
 JOIN memberships m ON m.organization_id=se.organization_id AND m.user_id=se.user_id
 WHERE se.token_hash=$1 AND se.expires_at>$2 AND se.last_seen>$3 AND se.password_version=u.password_version AND u.active AND o.active AND m.active`, Digest(token), now, now.Add(-2*time.Hour)).Scan(&p.UserID, &p.OrganizationID, &p.Organization, &p.Email, &p.Role, &p.PlatformAdmin, &p.MFAVerified, &p.MFAEnabled, &p.Permissions)
	if errors.Is(err, pgx.ErrNoRows) {
		return p, ErrDenied
	}
	if err != nil {
		return p, err
	}
	p.MFADisabled = s.options.DisableMFA
	_, err = s.pool.Exec(ctx, "UPDATE sessions SET last_seen=$2 WHERE token_hash=$1 AND last_seen<$3", Digest(token), now, now.Add(-time.Minute))
	return p, err
}
func (s *Service) Logout(ctx context.Context, token string) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(context.Background())
	var p tenancy.Principal
	err = tx.QueryRow(ctx, "DELETE FROM sessions WHERE token_hash=$1 RETURNING organization_id::text,user_id::text", Digest(token)).Scan(&p.OrganizationID, &p.UserID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	if err = audit(ctx, tx, p, "auth.logout", p.UserID); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
func (s *Service) LogoutAll(ctx context.Context, p tenancy.Principal) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(context.Background())
	if _, err = tx.Exec(ctx, "DELETE FROM sessions WHERE user_id=$1", p.UserID); err != nil {
		return err
	}
	if err = audit(ctx, tx, p, "auth.sessions_revoked", p.UserID); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

type Member struct {
	UserID string `json:"user_id"`
	Email  string `json:"email"`
	Role   string `json:"role"`
	Active bool   `json:"active"`
}

func (s *Service) Members(ctx context.Context, p tenancy.Principal) ([]Member, error) {
	if !p.Can("members:read") || p.RequiresMFA() {
		return nil, ErrDenied
	}
	rows, err := s.pool.Query(ctx, "SELECT u.id::text,u.email,m.role_code,(m.active AND u.active) FROM memberships m JOIN users u ON u.id=m.user_id WHERE m.organization_id=$1 ORDER BY lower(u.email)", p.OrganizationID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Member{}
	for rows.Next() {
		var m Member
		if err := rows.Scan(&m.UserID, &m.Email, &m.Role, &m.Active); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}
func (s *Service) SetRole(ctx context.Context, p tenancy.Principal, userID, role string) error {
	if !uuidPattern.MatchString(userID) {
		return ErrNotFound
	}
	if !validRole(role) {
		return ErrConflict
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(context.Background())
	if _, err = tx.Exec(ctx, "SELECT id FROM organizations WHERE id=$1 FOR UPDATE", p.OrganizationID); err != nil {
		return err
	}
	if !authorized(ctx, tx, p, "members:manage") {
		return ErrDenied
	}
	var old, actorRole string
	if err = tx.QueryRow(ctx, "SELECT role_code FROM memberships WHERE organization_id=$1 AND user_id=$2 AND active", p.OrganizationID, userID).Scan(&old); errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	} else if err != nil {
		return err
	}
	if err = tx.QueryRow(ctx, "SELECT role_code FROM memberships WHERE organization_id=$1 AND user_id=$2", p.OrganizationID, p.UserID).Scan(&actorRole); err != nil {
		return err
	}
	if (old == "owner" || role == "owner") && actorRole != "owner" {
		return ErrDenied
	}
	if old == "owner" && role != "owner" {
		var count int
		if err = tx.QueryRow(ctx, "SELECT count(*) FROM memberships m JOIN users u ON u.id=m.user_id WHERE m.organization_id=$1 AND m.role_code='owner' AND m.active AND u.active", p.OrganizationID).Scan(&count); err != nil {
			return err
		}
		if count <= 1 {
			return ErrConflict
		}
	}
	if _, err = tx.Exec(ctx, "UPDATE memberships SET role_code=$3 WHERE organization_id=$1 AND user_id=$2", p.OrganizationID, userID, role); err != nil {
		return err
	}
	if err = audit(ctx, tx, p, "membership.role_changed", userID); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
