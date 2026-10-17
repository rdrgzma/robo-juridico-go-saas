package auth

import (
	"context"
	"crypto/rand"
	"encoding/base32"
	"errors"
	"github.com/jackc/pgx/v5"
	"github.com/rdrgzma/robo-juridico-go-saas/internal/tenancy"
	"time"
)

func (s *Service) MFASetup(ctx context.Context, p tenancy.Principal, token string) (string, error) {
	if p.MFAEnabled {
		return "", ErrConflict
	}
	var pending []byte
	err := s.pool.QueryRow(ctx, "SELECT pending_mfa FROM sessions WHERE token_hash=$1 AND user_id=$2 AND created_at>$3", Digest(token), p.UserID, s.now().Add(-5*time.Minute)).Scan(&pending)
	if err != nil {
		return "", ErrDenied
	}
	if len(pending) > 0 {
		return s.open(pending, "mfa:"+p.UserID)
	}
	b := make([]byte, 20)
	if _, err = rand.Read(b); err != nil {
		return "", err
	}
	secret := base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(b)
	encrypted, err := s.seal(secret, "mfa:"+p.UserID)
	if err != nil {
		return "", err
	}
	_, err = s.pool.Exec(ctx, "UPDATE sessions SET pending_mfa=$2 WHERE token_hash=$1", Digest(token), encrypted)
	return secret, err
}

// VerifyMFA consumes a TOTP step or recovery code and rotates the session atomically.
func (s *Service) VerifyMFA(ctx context.Context, p tenancy.Principal, token, code string) (string, []string, error) {
	if err := s.attempt(ctx, "mfa", p.UserID, 8, 5*time.Minute); err != nil {
		return "", nil, err
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return "", nil, err
	}
	defer tx.Rollback(context.Background())
	var encrypted, pending []byte
	var last int64
	var version int
	err = tx.QueryRow(ctx, "SELECT mfa_secret,mfa_last_step,password_version FROM users WHERE id=$1 AND active FOR UPDATE", p.UserID).Scan(&encrypted, &last, &version)
	if err != nil {
		return "", nil, ErrDenied
	}
	if err = tx.QueryRow(ctx, "SELECT pending_mfa FROM sessions WHERE token_hash=$1 AND user_id=$2 AND expires_at>$3 AND password_version=$4", Digest(token), p.UserID, s.now(), version).Scan(&pending); err != nil {
		return "", nil, ErrDenied
	}
	enrolling := len(encrypted) == 0
	codes := []string{}
	if enrolling {
		encrypted = pending
		if len(encrypted) == 0 {
			return "", nil, ErrDenied
		}
	}
	secret, err := s.open(encrypted, "mfa:"+p.UserID)
	if err != nil {
		return "", nil, ErrDenied
	}
	step, valid := totpStep(secret, code, s.now())
	if valid && step > last {
		if _, err = tx.Exec(ctx, "UPDATE users SET mfa_secret=$2,mfa_last_step=$3 WHERE id=$1", p.UserID, encrypted, step); err != nil {
			return "", nil, err
		}
	} else {
		if enrolling || len(code) != 20 {
			return "", nil, ErrDenied
		}
		tag, err := tx.Exec(ctx, "DELETE FROM recovery_codes WHERE user_id=$1 AND code_hash=$2", p.UserID, Digest(code))
		if err != nil {
			return "", nil, err
		}
		if tag.RowsAffected() != 1 {
			return "", nil, ErrDenied
		}
	}
	if enrolling {
		for i := 0; i < 10; i++ {
			c := RandomToken()[:20]
			if _, err = tx.Exec(ctx, "INSERT INTO recovery_codes(user_id,code_hash) VALUES($1,$2)", p.UserID, Digest(c)); err != nil {
				return "", nil, err
			}
			codes = append(codes, c)
		}
	}
	p.MFAEnabled = true
	newToken, err := s.issueSession(ctx, tx, p, version, true)
	if err != nil {
		return "", nil, err
	}
	if _, err = tx.Exec(ctx, "DELETE FROM sessions WHERE token_hash=$1", Digest(token)); err != nil {
		return "", nil, err
	}
	if err = audit(ctx, tx, p, "auth.mfa_verified", p.UserID); err != nil {
		return "", nil, err
	}
	if err = tx.Commit(ctx); err != nil {
		return "", nil, err
	}
	return newToken, codes, nil
}

func (s *Service) ForgotPassword(ctx context.Context, org, email, ip string) error {
	if err := s.attempt(ctx, "reset-ip", ip, 10, 15*time.Minute); err != nil {
		return err
	}
	email = stringsLower(email)
	if err := s.attempt(ctx, "reset-account", org+":"+email, 3, 15*time.Minute); err != nil {
		return err
	}
	var userID string
	err := s.pool.QueryRow(ctx, `SELECT u.id::text FROM users u JOIN memberships m ON m.user_id=u.id JOIN organizations o ON o.id=m.organization_id
 WHERE lower(u.email)=$1 AND o.slug=$2 AND u.active AND m.active AND o.active`, email, org).Scan(&userID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
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
	if _, err = tx.Exec(ctx, "SELECT id FROM users WHERE id=$1 FOR UPDATE", userID); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, "DELETE FROM password_resets WHERE user_id=$1", userID); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, "INSERT INTO password_resets(token_hash,user_id,expires_at,created_at) VALUES($1,$2,$3,$4)", Digest(token), userID, s.now().Add(15*time.Minute), s.now()); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, "INSERT INTO mail_outbox(user_id,kind,encrypted_payload,created_at) VALUES($1,'password_reset',$2,$3)", userID, payload, s.now()); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
func (s *Service) ResetPassword(ctx context.Context, token, password string) error {
	if len(token) != 43 {
		return ErrDenied
	}
	var userID string
	if err := s.pool.QueryRow(ctx, "SELECT user_id::text FROM password_resets WHERE token_hash=$1 AND expires_at>$2", Digest(token), s.now()).Scan(&userID); err != nil {
		return ErrDenied
	}
	hash, err := HashPassword(password)
	if err != nil {
		return err
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(context.Background())
	if _, err = tx.Exec(ctx, "SELECT id FROM users WHERE id=$1 FOR UPDATE", userID); err != nil {
		return err
	}
	tag, err := tx.Exec(ctx, "DELETE FROM password_resets WHERE token_hash=$1 AND user_id=$2 AND expires_at>$3", Digest(token), userID, s.now())
	if err != nil {
		return err
	}
	if tag.RowsAffected() != 1 {
		return ErrDenied
	}
	if tag, err = tx.Exec(ctx, "UPDATE users SET password_hash=$2,password_version=password_version+1 WHERE id=$1 AND active", userID, hash); err != nil {
		return err
	}
	if tag.RowsAffected() != 1 {
		return ErrDenied
	}
	if _, err = tx.Exec(ctx, `INSERT INTO audit_events(organization_id,actor_id,action,target_id) SELECT organization_id,$1,'auth.password_reset',$1 FROM memberships WHERE user_id=$1 AND active`, userID); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, "DELETE FROM sessions WHERE user_id=$1", userID); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, "DELETE FROM password_resets WHERE user_id=$1", userID); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

type OutboxMessage struct {
	ID  string `json:"id"`
	URL string `json:"url"`
}

func (s *Service) DevelopmentOutbox(ctx context.Context) ([]OutboxMessage, error) {
	rows, err := s.pool.Query(ctx, "SELECT id::text,user_id::text,encrypted_payload FROM mail_outbox WHERE created_at>$1 ORDER BY created_at DESC LIMIT 20", s.now().Add(-15*time.Minute))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []OutboxMessage{}
	for rows.Next() {
		var id, userID string
		var encrypted []byte
		if err := rows.Scan(&id, &userID, &encrypted); err != nil {
			return nil, err
		}
		text, err := s.open(encrypted, "outbox:"+userID)
		if err != nil {
			return nil, err
		}
		out = append(out, OutboxMessage{id, text})
	}
	return out, rows.Err()
}
