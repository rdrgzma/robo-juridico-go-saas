package tenancy

import "context"

type Principal struct {
	UserID         string   `json:"user_id"`
	OrganizationID string   `json:"organization_id"`
	Organization   string   `json:"organization"`
	Email          string   `json:"email"`
	Role           string   `json:"role"`
	PlatformAdmin  bool     `json:"platform_admin"`
	MFAVerified    bool     `json:"mfa_verified"`
	MFAEnabled     bool     `json:"mfa_enabled"`
	Permissions    []string `json:"permissions"`
}

func (p Principal) Can(permission string) bool {
	for _, v := range p.Permissions {
		if v == permission {
			return true
		}
	}
	return false
}
func (p Principal) RequiresMFA() bool { return (p.PlatformAdmin || p.MFAEnabled) && !p.MFAVerified }

type contextKey struct{}

func WithPrincipal(ctx context.Context, p Principal) context.Context {
	return context.WithValue(ctx, contextKey{}, p)
}
func FromContext(ctx context.Context) (Principal, bool) {
	p, ok := ctx.Value(contextKey{}).(Principal)
	return p, ok
}
