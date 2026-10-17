package auth

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/jackc/pgx/v5"
)

// DevelopmentLogin contains local credentials, never serialized into the HTML view.
type DevelopmentLogin struct {
	Organization string `json:"organization"`
	Email        string `json:"email"`
	Password     string `json:"password"`
}
type DevelopmentShortcut struct{ Profile, Label string }

var developmentProfiles = []DevelopmentShortcut{
	{"platform_admin", "Administrador global"},
	{"owner", "Proprietário da organização"},
	{"admin", "Administrador da organização"},
	{"lawyer", "Advogado"},
	{"viewer", "Visualizador"},
}

// LoadDevelopmentLogins fails closed outside the exclusive local development DB.
// A missing file disables the feature; deployment images do not include this file.
func LoadDevelopmentLogins(environment, databaseURL, path string) (map[string]DevelopmentLogin, error) {
	if environment != "development" {
		return nil, nil
	}
	cfg, err := pgx.ParseConfig(databaseURL)
	if err != nil {
		return nil, err
	}
	if cfg.Database != "rj_saas_dev" || cfg.User != "rj_app" {
		return nil, nil
	}
	b, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var credentials map[string]DevelopmentLogin
	if err = json.Unmarshal(b, &credentials); err != nil {
		return nil, fmt.Errorf("invalid development credentials file")
	}
	if len(credentials) != len(developmentProfiles) {
		return nil, fmt.Errorf("incomplete development profiles")
	}
	for _, profile := range developmentProfiles {
		c, ok := credentials[profile.Profile]
		if !ok || !slugPattern.MatchString(c.Organization) || len(c.Password) < 12 || len(c.Password) > 128 {
			return nil, fmt.Errorf("invalid development profile")
		}
		if _, err = normalizeEmail(c.Email); err != nil {
			return nil, fmt.Errorf("invalid development email")
		}
	}
	return credentials, nil
}

func (s *Service) developmentShortcuts() []DevelopmentShortcut {
	var shortcuts []DevelopmentShortcut
	for _, p := range developmentProfiles {
		if _, ok := s.options.DevelopmentLogins[p.Profile]; ok {
			shortcuts = append(shortcuts, p)
		}
	}
	return shortcuts
}
