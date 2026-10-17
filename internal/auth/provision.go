package auth

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/rdrgzma/robo-juridico-go-saas/internal/tenancy"
)

// CreateDevelopmentMember is reachable only from the explicitly development-only CLI.
// Existing global users are never given a replacement password through organization management.
func (s *Service) CreateDevelopmentMember(ctx context.Context, organization, email, password, role string) (tenancy.Principal, error) {
	var p tenancy.Principal
	email, err := normalizeEmail(email)
	if err != nil || !validRole(role) {
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
	if err = tx.QueryRow(ctx, "SELECT id::text FROM organizations WHERE slug=$1 AND active FOR UPDATE", organization).Scan(&p.OrganizationID); err != nil {
		return p, err
	}
	if err = tx.QueryRow(ctx, "INSERT INTO users(email,password_hash) VALUES($1,$2) RETURNING id::text", email, hash).Scan(&p.UserID); err != nil {
		return p, err
	}
	if _, err = tx.Exec(ctx, "INSERT INTO memberships(organization_id,user_id,role_code) VALUES($1,$2,$3)", p.OrganizationID, p.UserID, role); err != nil {
		return p, err
	}
	p.Organization = organization
	p.Email = email
	p.Role = role
	if err = audit(ctx, tx, p, "development.member_created", p.UserID); err != nil {
		return p, err
	}
	return p, tx.Commit(ctx)
}

func (s *Service) AdminTriggerRecovery(ctx context.Context, actor tenancy.Principal, userID string) error {
	if !actor.PlatformAdmin || actor.RequiresMFA() {
		return ErrDenied
	}
	if !uuidPattern.MatchString(userID) {
		return ErrNotFound
	}
	token := RandomToken()
	payload, err := s.seal(s.options.Origin+"/reset?token="+token, "outbox:"+userID)
	if err != nil {
		return err
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(context.Background())
	var orgID string
	if err = tx.QueryRow(ctx, "SELECT COALESCE((SELECT organization_id::text FROM memberships WHERE user_id=$1 ORDER BY active DESC LIMIT 1), $2) FROM users WHERE id=$1 AND active FOR UPDATE", userID, actor.OrganizationID).Scan(&orgID); errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	} else if err != nil {
		return err
	}
	now := s.now()
	if _, err = tx.Exec(ctx, "DELETE FROM password_resets WHERE user_id=$1", userID); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, "INSERT INTO password_resets(token_hash,user_id,expires_at,created_at) VALUES($1,$2,$3,$4)", Digest(token), userID, now.Add(15*time.Minute), now); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, "INSERT INTO mail_outbox(user_id,kind,encrypted_payload,created_at) VALUES($1,'password_reset',$2,$3)", userID, payload, now); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, "INSERT INTO audit_events(organization_id,actor_id,action,target_id) VALUES($1,$2,'admin.user_recovery_requested',$3)", orgID, actor.UserID, userID); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (s *Service) AdminIssueKey(ctx context.Context, actor tenancy.Principal, orgID, name string, scopes []string) (Key, string, error) {
	var key Key
	if !actor.PlatformAdmin || actor.RequiresMFA() {
		return key, "", ErrDenied
	}
	if !uuidPattern.MatchString(orgID) {
		return key, "", ErrNotFound
	}
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
	var creatorID string
	err = tx.QueryRow(ctx, `SELECT m.user_id::text FROM organizations o
 JOIN memberships m ON m.organization_id=o.id JOIN users u ON u.id=m.user_id
 WHERE o.id=$1 AND o.active AND m.active AND u.active AND m.role_code IN ('owner','admin')
 ORDER BY (m.role_code='owner') DESC, u.created_at ASC LIMIT 1 FOR UPDATE OF o`, orgID).Scan(&creatorID)
	if errors.Is(err, pgx.ErrNoRows) {
		return key, "", ErrConflict
	}
	if err != nil {
		return key, "", err
	}
	var count int
	if err = tx.QueryRow(ctx, "SELECT count(*) FROM api_keys WHERE organization_id=$1 AND revoked_at IS NULL", orgID).Scan(&count); err != nil {
		return key, "", err
	}
	if count >= 100 {
		return key, "", ErrConflict
	}
	token := "rj_" + RandomToken()
	key.Name = name
	key.Prefix = token[:11]
	key.Scopes = scopes
	if err = tx.QueryRow(ctx, "INSERT INTO api_keys(organization_id,created_by,name,prefix,token_hash,scopes) VALUES($1,$2,$3,$4,$5,$6) RETURNING id::text,created_at", orgID, creatorID, name, key.Prefix, Digest(token), scopes).Scan(&key.ID, &key.CreatedAt); err != nil {
		return key, "", err
	}
	if _, err = tx.Exec(ctx, "INSERT INTO audit_events(organization_id,actor_id,action,target_id) VALUES($1,$2,'admin.api_key_created',$3)", orgID, actor.UserID, key.ID); err != nil {
		return key, "", err
	}
	return key, token, tx.Commit(ctx)
}

func (s *Service) AdminRotateKey(ctx context.Context, actor tenancy.Principal, id string) (Key, string, error) {
	var key Key
	if !actor.PlatformAdmin || actor.RequiresMFA() {
		return key, "", ErrDenied
	}
	if !uuidPattern.MatchString(id) {
		return key, "", ErrNotFound
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return key, "", err
	}
	defer tx.Rollback(context.Background())
	var orgID, createdBy string
	if err = tx.QueryRow(ctx, "SELECT organization_id::text,created_by::text,name,scopes FROM api_keys WHERE id=$1 AND revoked_at IS NULL FOR UPDATE", id).Scan(&orgID, &createdBy, &key.Name, &key.Scopes); errors.Is(err, pgx.ErrNoRows) {
		return key, "", ErrNotFound
	} else if err != nil {
		return key, "", err
	}
	if _, err = tx.Exec(ctx, "UPDATE api_keys SET revoked_at=$2 WHERE id=$1 AND revoked_at IS NULL", id, s.now()); err != nil {
		return key, "", err
	}
	token := "rj_" + RandomToken()
	key.Prefix = token[:11]
	if err = tx.QueryRow(ctx, "INSERT INTO api_keys(organization_id,created_by,name,prefix,token_hash,scopes) VALUES($1,$2,$3,$4,$5,$6) RETURNING id::text,created_at", orgID, createdBy, key.Name, key.Prefix, Digest(token), key.Scopes).Scan(&key.ID, &key.CreatedAt); err != nil {
		return key, "", err
	}
	if _, err = tx.Exec(ctx, "INSERT INTO audit_events(organization_id,actor_id,action,target_id) VALUES($1,$2,'admin.api_key_rotated',$3)", orgID, actor.UserID, key.ID); err != nil {
		return key, "", err
	}
	return key, token, tx.Commit(ctx)
}

func (s *Service) AdminRevokeKey(ctx context.Context, actor tenancy.Principal, id string) error {
	if !actor.PlatformAdmin || actor.RequiresMFA() {
		return ErrDenied
	}
	if !uuidPattern.MatchString(id) {
		return ErrNotFound
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(context.Background())
	var orgID string
	if err = tx.QueryRow(ctx, "UPDATE api_keys SET revoked_at=$2 WHERE id=$1 AND revoked_at IS NULL RETURNING organization_id::text", id, s.now()).Scan(&orgID); errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	} else if err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, "INSERT INTO audit_events(organization_id,actor_id,action,target_id) VALUES($1,$2,'admin.api_key_revoked',$3)", orgID, actor.UserID, id); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

type SessionView struct {
	ID          string
	OrgSlug     string
	CreatedAt   time.Time
	LastSeen    time.Time
	ExpiresAt   time.Time
	MFAVerified bool
	Current     bool
}

func (s *Service) UserSessions(ctx context.Context, p tenancy.Principal, currentToken string) ([]SessionView, error) {
	if !p.Can("account:read") || p.RequiresMFA() {
		return nil, ErrDenied
	}
	var currentID string
	if len(currentToken) == 43 {
		curHash := Digest(currentToken)
		curIDBytes := Digest(string(curHash))
		for _, b := range curIDBytes {
			const hexChars = "0123456789abcdef"
			currentID += string([]byte{hexChars[b>>4], hexChars[b&0x0f]})
		}
	}
	rows, err := s.pool.Query(ctx, `SELECT encode(sha256(se.token_hash),'hex'),o.slug,se.created_at,se.last_seen,se.expires_at,se.mfa_verified
 FROM sessions se JOIN organizations o ON o.id=se.organization_id
 WHERE se.user_id=$1 AND se.expires_at>$2 AND se.last_seen>$3
 ORDER BY se.last_seen DESC`, p.UserID, s.now(), s.now().Add(-2*time.Hour))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []SessionView{}
	for rows.Next() {
		var v SessionView
		if err := rows.Scan(&v.ID, &v.OrgSlug, &v.CreatedAt, &v.LastSeen, &v.ExpiresAt, &v.MFAVerified); err != nil {
			return nil, err
		}
		v.Current = v.ID == currentID
		out = append(out, v)
	}
	return out, rows.Err()
}

func (s *Service) RevokeSession(ctx context.Context, p tenancy.Principal, sessionID string) error {
	if !p.Can("account:read") || p.RequiresMFA() {
		return ErrDenied
	}
	if len(sessionID) != 64 {
		return ErrNotFound
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(context.Background())
	tag, err := tx.Exec(ctx, "DELETE FROM sessions WHERE user_id=$1 AND encode(sha256(token_hash),'hex')=$2", p.UserID, sessionID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() != 1 {
		return ErrNotFound
	}
	if err = audit(ctx, tx, p, "auth.session_revoked", p.UserID); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (s *Service) ChangePassword(ctx context.Context, p tenancy.Principal, currentPassword, newPassword string) (string, error) {
	if !p.Can("account:read") || p.RequiresMFA() {
		return "", ErrDenied
	}
	newHash, err := HashPassword(newPassword)
	if err != nil {
		return "", ErrConflict
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return "", err
	}
	defer tx.Rollback(context.Background())
	var currentHash string
	var version int
	if err = tx.QueryRow(ctx, "SELECT password_hash,password_version FROM users WHERE id=$1 AND active FOR UPDATE", p.UserID).Scan(&currentHash, &version); errors.Is(err, pgx.ErrNoRows) {
		return "", ErrDenied
	} else if err != nil {
		return "", err
	}
	if !VerifyPassword(currentHash, currentPassword) {
		return "", ErrCredentials
	}
	version++
	if _, err = tx.Exec(ctx, "UPDATE users SET password_hash=$2,password_version=$3 WHERE id=$1", p.UserID, newHash, version); err != nil {
		return "", err
	}
	if _, err = tx.Exec(ctx, "DELETE FROM sessions WHERE user_id=$1", p.UserID); err != nil {
		return "", err
	}
	if _, err = tx.Exec(ctx, "DELETE FROM password_resets WHERE user_id=$1", p.UserID); err != nil {
		return "", err
	}
	token, err := s.issueSession(ctx, tx, p, version, p.MFAVerified)
	if err != nil {
		return "", err
	}
	if err = audit(ctx, tx, p, "auth.password_changed", p.UserID); err != nil {
		return "", err
	}
	return token, tx.Commit(ctx)
}

func (s *Service) ProvisionMember(ctx context.Context, p tenancy.Principal, email, password, role string) (string, error) {
	email, err := normalizeEmail(email)
	if err != nil || !validRole(role) {
		return "", ErrConflict
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return "", err
	}
	defer tx.Rollback(context.Background())
	if _, err = tx.Exec(ctx, "SELECT id FROM organizations WHERE id=$1 FOR UPDATE", p.OrganizationID); err != nil {
		return "", err
	}
	if !authorized(ctx, tx, p, "members:manage") {
		return "", ErrDenied
	}
	var actorRole string
	if err = tx.QueryRow(ctx, "SELECT role_code FROM memberships WHERE organization_id=$1 AND user_id=$2 AND active", p.OrganizationID, p.UserID).Scan(&actorRole); err != nil {
		return "", ErrDenied
	}
	if role == "owner" && actorRole != "owner" {
		return "", ErrDenied
	}
	var userID string
	err = tx.QueryRow(ctx, "SELECT id::text FROM users WHERE lower(email)=$1 FOR UPDATE", email).Scan(&userID)
	if errors.Is(err, pgx.ErrNoRows) {
		hash, hashErr := HashPassword(password)
		if hashErr != nil {
			return "", ErrConflict
		}
		if err = tx.QueryRow(ctx, "INSERT INTO users(email,password_hash) VALUES($1,$2) RETURNING id::text", email, hash).Scan(&userID); err != nil {
			return "", err
		}
	} else if err != nil {
		return "", err
	} else {
		var existingRole string
		if e := tx.QueryRow(ctx, "SELECT role_code FROM memberships WHERE organization_id=$1 AND user_id=$2", p.OrganizationID, userID).Scan(&existingRole); e == nil && existingRole == "owner" && actorRole != "owner" {
			return "", ErrDenied
		}
	}
	if _, err = tx.Exec(ctx, `INSERT INTO memberships(organization_id,user_id,role_code,active) VALUES($1,$2,$3,true)
 ON CONFLICT(organization_id,user_id) DO UPDATE SET role_code=EXCLUDED.role_code,active=true`, p.OrganizationID, userID, role); err != nil {
		return "", err
	}
	if err = audit(ctx, tx, p, "membership.provisioned", userID); err != nil {
		return "", err
	}
	return userID, tx.Commit(ctx)
}

func (s *Service) SetMemberStatus(ctx context.Context, p tenancy.Principal, userID string, active bool) error {
	if !uuidPattern.MatchString(userID) {
		return ErrNotFound
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
	var targetRole, actorRole string
	if err = tx.QueryRow(ctx, "SELECT role_code FROM memberships WHERE organization_id=$1 AND user_id=$2", p.OrganizationID, userID).Scan(&targetRole); errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	} else if err != nil {
		return err
	}
	if err = tx.QueryRow(ctx, "SELECT role_code FROM memberships WHERE organization_id=$1 AND user_id=$2", p.OrganizationID, p.UserID).Scan(&actorRole); err != nil {
		return err
	}
	if targetRole == "owner" && actorRole != "owner" {
		return ErrDenied
	}
	if !active && targetRole == "owner" {
		var count int
		if err = tx.QueryRow(ctx, "SELECT count(*) FROM memberships m JOIN users u ON u.id=m.user_id WHERE m.organization_id=$1 AND m.role_code='owner' AND m.active AND u.active", p.OrganizationID).Scan(&count); err != nil {
			return err
		}
		if count <= 1 {
			return ErrConflict
		}
	}
	if _, err = tx.Exec(ctx, "UPDATE memberships SET active=$3 WHERE organization_id=$1 AND user_id=$2", p.OrganizationID, userID, active); err != nil {
		return err
	}
	if !active {
		if _, err = tx.Exec(ctx, "DELETE FROM sessions WHERE organization_id=$1 AND user_id=$2", p.OrganizationID, userID); err != nil {
			return err
		}
	}
	if err = audit(ctx, tx, p, "membership.status_changed", userID); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
