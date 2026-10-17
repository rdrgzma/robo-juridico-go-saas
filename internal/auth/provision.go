package auth

import (
	"context"
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
