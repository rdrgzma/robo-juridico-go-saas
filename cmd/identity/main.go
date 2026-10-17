package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"github.com/jackc/pgx/v5"
	"github.com/rdrgzma/robo-juridico-go-saas/internal/auth"
	"github.com/rdrgzma/robo-juridico-go-saas/internal/config"
	"github.com/rdrgzma/robo-juridico-go-saas/internal/storage"
	"os"
	"path/filepath"
	"time"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
func save(path string, data any) error {
	b, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return err
	}
	if err = os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = f.Write(append(b, '\n'))
	return err
}
func run() error {
	if len(os.Args) < 2 {
		return fmt.Errorf("uso: identity bootstrap | member -organization slug -email endereço -role papel | outbox")
	}
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	db, err := pgx.ParseConfig(cfg.DatabaseURL)
	if cfg.Environment != "development" || err != nil || db.Database != "rj_saas_dev" || db.User != "rj_app" {
		return fmt.Errorf("provisionamento permitido apenas no banco/role de desenvolvimento novos")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	pool, err := storage.Connect(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer pool.Close()
	service, err := auth.New(pool, cfg.AuthKey, auth.Options{Origin: cfg.PublicOrigin, CookieSecure: cfg.CookieSecure})
	if err != nil {
		return err
	}
	switch os.Args[1] {
	case "bootstrap":
		const path = ".local/bootstrap.json"
		if _, err := os.Stat(path); err == nil {
			fmt.Println("Credenciais existentes preservadas em", path)
			return nil
		}
		suffix := fmt.Sprintf("%x", auth.Digest(auth.RandomToken()))[:12]
		slug := "dev-" + suffix
		email := "owner-" + suffix + "@example.test"
		password := auth.RandomToken()
		p, err := service.CreateOrganization(ctx, slug, "Organização local de desenvolvimento", email, password, true)
		if err != nil {
			return fmt.Errorf("falha ao criar organização local: %w", err)
		}
		if err = save(path, map[string]any{"organization": slug, "email": email, "password": password, "user_id": p.UserID, "organization_id": p.OrganizationID, "url": cfg.PublicOrigin + "/login", "mfa_required": true}); err != nil {
			return err
		}
		fmt.Println("Conta local criada; credenciais aleatórias em", path, ". MFA obrigatório no primeiro acesso.")
	case "member":
		flags := flag.NewFlagSet("member", flag.ContinueOnError)
		org := flags.String("organization", "", "slug")
		email := flags.String("email", "", "email de desenvolvimento")
		role := flags.String("role", "viewer", "owner/admin/lawyer/viewer")
		if err := flags.Parse(os.Args[2:]); err != nil {
			return err
		}
		password := auth.RandomToken()
		p, err := service.CreateDevelopmentMember(ctx, *org, *email, password, *role)
		if err != nil {
			return err
		}
		path := ".local/member-" + p.UserID + ".json"
		if err = save(path, map[string]string{"organization": *org, "email": *email, "password": password}); err != nil {
			return err
		}
		fmt.Println("Membro local criado; credenciais em", path)
	case "outbox":
		messages, err := service.DevelopmentOutbox(ctx)
		if err != nil {
			return err
		}
		path := ".local/outbox-" + time.Now().UTC().Format("20060102T150405.000000000") + ".json"
		if err = save(path, messages); err != nil {
			return err
		}
		fmt.Println("Instruções locais de recuperação exportadas para", path)
	default:
		return fmt.Errorf("subcomando inválido")
	}
	return nil
}
