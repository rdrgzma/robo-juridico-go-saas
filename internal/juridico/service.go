// Package juridico coordinates the shared engines and organization-owned history.
package juridico

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"regexp"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rdrgzma/robo-juridico-go-saas/internal/auth"
	"github.com/rdrgzma/robo-juridico-go-saas/internal/calculator"
	"github.com/rdrgzma/robo-juridico-go-saas/internal/prazos"
	"github.com/rdrgzma/robo-juridico-go-saas/internal/tenancy"
)

const EngineVersion = "go-saas/0.3.0"

var idPattern = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)
var requestPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{43}$`)
var ErrInput = errors.New("entrada inválida")

type Service struct {
	pool      *pgxpool.Pool
	identity  *auth.Service
	sources   *Sources
	reportKey []byte
}

func New(pool *pgxpool.Pool, identity *auth.Service, key []byte, sources *Sources) *Service {
	h := hmac.New(sha256.New, key)
	h.Write([]byte("rj:private-calculation-report:v1"))
	if sources == nil {
		sources = NewSources("", false)
	}
	return &Service{pool: pool, identity: identity, reportKey: h.Sum(nil), sources: sources}
}

type Operation struct {
	ID             string          `json:"id"`
	OrganizationID string          `json:"organization_id"`
	UserID         string          `json:"user_id"`
	Kind           string          `json:"kind"`
	Title          string          `json:"title"`
	Input          json.RawMessage `json:"input"`
	Result         json.RawMessage `json:"result"`
	EngineVersion  string          `json:"engine_version"`
	CreatedAt      time.Time       `json:"created_at"`
}
type Stats struct {
	Total      int
	MonthUsed  int
	MonthLimit int
}

func (s *Service) Stats(ctx context.Context, p tenancy.Principal) (Stats, error) {
	var v Stats
	err := s.pool.QueryRow(ctx, `SELECT (SELECT count(*) FROM legal_operations WHERE organization_id=$1),
 COALESCE((SELECT used FROM quota_periods WHERE organization_id=$1 AND period=date_trunc('month',now() AT TIME ZONE 'America/Sao_Paulo')::date),0),
 monthly_limit FROM organization_limits WHERE organization_id=$1`, p.OrganizationID).Scan(&v.Total, &v.MonthUsed, &v.MonthLimit)
	return v, err
}
func (s *Service) List(ctx context.Context, p tenancy.Principal, before time.Time, beforeID ...string) ([]Operation, error) {
	if before.IsZero() {
		before = time.Now().Add(time.Second)
	}
	cursorID := "ffffffff-ffff-ffff-ffff-ffffffffffff"
	if len(beforeID) > 0 && idPattern.MatchString(beforeID[0]) {
		cursorID = beforeID[0]
	}
	rows, err := s.pool.Query(ctx, `SELECT id::text,kind,title,engine_version,created_at FROM legal_operations WHERE organization_id=$1 AND (created_at,id)<($2,$3::uuid) ORDER BY created_at DESC,id DESC LIMIT 51`, p.OrganizationID, before, cursorID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Operation{}
	for rows.Next() {
		var o Operation
		if err = rows.Scan(&o.ID, &o.Kind, &o.Title, &o.EngineVersion, &o.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, o)
	}
	return out, rows.Err()
}
func (s *Service) Get(ctx context.Context, p tenancy.Principal, id string) (Operation, error) {
	var o Operation
	if !idPattern.MatchString(id) {
		return o, auth.ErrNotFound
	}
	err := s.pool.QueryRow(ctx, `SELECT id::text,organization_id::text,user_id::text,kind,title,input,result,engine_version,created_at FROM legal_operations WHERE organization_id=$1 AND id=$2`, p.OrganizationID, id).Scan(&o.ID, &o.OrganizationID, &o.UserID, &o.Kind, &o.Title, &o.Input, &o.Result, &o.EngineVersion, &o.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		err = auth.ErrNotFound
	}
	return o, err
}
func decode(raw []byte, target any) error {
	if len(raw) > 256<<10 {
		return fmt.Errorf("entrada excede 256 KiB")
	}
	d := json.NewDecoder(bytes.NewReader(raw))
	d.DisallowUnknownFields()
	if err := d.Decode(target); err != nil {
		return fmt.Errorf("dados inválidos ou campos desconhecidos")
	}
	if d.Decode(new(any)) != io.EOF {
		return fmt.Errorf("envie um único objeto")
	}
	return nil
}

// Run never accepts a frontend result; every output is computed on this server.
func (s *Service) Run(ctx context.Context, p tenancy.Principal, kind, title, requestID string, raw []byte) (Operation, error) {
	var o Operation
	if !p.Can("calculations:run") || p.RequiresMFA() {
		return o, auth.ErrDenied
	}
	if !requestPattern.MatchString(requestID) || len(title) == 0 || len(title) > 160 {
		return o, fmt.Errorf("%w: título ou identificador inválido", ErrInput)
	}
	var priorID string
	err := s.pool.QueryRow(ctx, "SELECT id::text FROM legal_operations WHERE organization_id=$1 AND user_id=$2 AND idempotency_key=$3", p.OrganizationID, p.UserID, requestID).Scan(&priorID)
	if err == nil {
		return s.existing(ctx, p, priorID, kind, raw)
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return o, err
	}
	if err = s.throttle(ctx, p); err != nil {
		return o, err
	}
	var result any
	switch kind {
	case "liquidacao":
		var v calculator.Request
		err = decode(raw, &v)
		if err == nil {
			result, err = calculator.Calculate(v)
		}
	case "judicial":
		var v calculator.CalculoJudicialRequest
		err = decode(raw, &v)
		if err == nil {
			if v.JurosMora.Tipo == calculator.JurosTaxaLegal || v.JurosMora.Tipo == calculator.JurosLei14905 || v.JurosMora.Tipo == calculator.JurosSelic {
				err = fmt.Errorf("regime legal aguarda homologação de séries e política")
			} else {
				result, err = calculator.NovoEngineCalculo(nil).Executar(v)
			}
		}
	case "financiamento":
		var v calculator.CalculoFinanciamentoRequest
		err = decode(raw, &v)
		if err == nil {
			result, err = calculator.CalcularRevisaoFinanciamento(v)
		}
	case "prazos":
		var v prazos.Request
		err = decode(raw, &v)
		if err == nil {
			result, err = prazos.Calculate(v)
		}
	case "cnj":
		var v Query
		err = decode(raw, &v)
		if err == nil {
			result, err = s.sources.Query(ctx, p.OrganizationID, v)
		}
	case "indices":
		var v IndexQuery
		err = decode(raw, &v)
		if err == nil {
			result, err = s.sources.Index(ctx, v)
		}
	default:
		err = fmt.Errorf("tipo de operação desconhecido")
	}
	if err != nil {
		return o, fmt.Errorf("%w: %s", ErrInput, err)
	}
	encoded, err := json.Marshal(result)
	if err != nil {
		return o, err
	}
	// Normalize input before persistence. Results are capped independently from upstream limits.
	if len(encoded) > 3<<20 || !json.Valid(raw) {
		return o, fmt.Errorf("%w: dados excedem limite", ErrInput)
	}
	canonical := canonicalJSON(raw)
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return o, err
	}
	defer tx.Rollback(context.Background())
	var limit int
	// The same lock and quota period are shared with API calls.
	err = tx.QueryRow(ctx, `SELECT l.monthly_limit FROM organization_limits l JOIN organizations org ON org.id=l.organization_id
 JOIN memberships m ON m.organization_id=org.id JOIN users u ON u.id=m.user_id
 WHERE org.id=$1 AND u.id=$2 AND org.active AND u.active AND m.active AND EXISTS
 (SELECT 1 FROM role_permissions rp WHERE rp.role_code=m.role_code AND rp.permission_code='calculations:run') FOR UPDATE OF l`, p.OrganizationID, p.UserID).Scan(&limit)
	if errors.Is(err, pgx.ErrNoRows) {
		return o, auth.ErrDenied
	}
	if err != nil {
		return o, err
	}
	// A concurrent duplicate must not consume the quota twice.
	err = tx.QueryRow(ctx, "SELECT id::text FROM legal_operations WHERE organization_id=$1 AND user_id=$2 AND idempotency_key=$3", p.OrganizationID, p.UserID, requestID).Scan(&priorID)
	if err == nil {
		_ = tx.Rollback(ctx)
		return s.existing(ctx, p, priorID, kind, raw)
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return o, err
	}
	billable := kind != "cnj" && kind != "indices"
	if billable {
		if _, err = tx.Exec(ctx, `INSERT INTO quota_periods(organization_id,period) VALUES($1,date_trunc('month',now() AT TIME ZONE 'America/Sao_Paulo')::date) ON CONFLICT DO NOTHING`, p.OrganizationID); err != nil {
			return o, err
		}
		tag, e := tx.Exec(ctx, `UPDATE quota_periods SET used=used+1 WHERE organization_id=$1 AND period=date_trunc('month',now() AT TIME ZONE 'America/Sao_Paulo')::date AND used<$2`, p.OrganizationID, limit)
		if e != nil {
			return o, e
		}
		if tag.RowsAffected() != 1 {
			return o, auth.ErrQuota
		}
	}
	o = Operation{OrganizationID: p.OrganizationID, UserID: p.UserID, Kind: kind, Title: title, Input: canonical, Result: encoded, EngineVersion: EngineVersion}
	err = tx.QueryRow(ctx, `INSERT INTO legal_operations(organization_id,user_id,kind,title,input,result,engine_version,idempotency_key) VALUES($1,$2,$3,$4,$5,$6,$7,$8) RETURNING id::text,created_at`, p.OrganizationID, p.UserID, kind, title, canonical, encoded, EngineVersion, requestID).Scan(&o.ID, &o.CreatedAt)
	if err != nil {
		return Operation{}, err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO audit_events(organization_id,actor_id,action,target_id) VALUES($1,$2,'legal.operation_created',$3)`, p.OrganizationID, p.UserID, o.ID); err != nil {
		return Operation{}, err
	}
	return o, tx.Commit(ctx)
}
func (s *Service) throttle(ctx context.Context, p tenancy.Principal) error {
	var attempts, limit int
	err := s.pool.QueryRow(ctx, `SELECT requests_per_minute FROM organization_limits WHERE organization_id=$1`, p.OrganizationID).Scan(&limit)
	if err != nil {
		return err
	}
	err = s.pool.QueryRow(ctx, `INSERT INTO auth_buckets(purpose,bucket_hash,window_start,attempts) VALUES('web-operations',$1,date_trunc('minute',now()),1)
 ON CONFLICT(purpose,bucket_hash,window_start) DO UPDATE SET attempts=auth_buckets.attempts+1 RETURNING attempts`, auth.Digest(p.OrganizationID)).Scan(&attempts)
	if err == nil && attempts > limit {
		return auth.ErrRate
	}
	return err
}

// Integrity authenticates the full persisted record, not merely the total.
func (s *Service) Integrity(o Operation) string {
	canonical := func(raw json.RawMessage) json.RawMessage {
		d := json.NewDecoder(bytes.NewReader(raw))
		d.UseNumber()
		var v any
		if d.Decode(&v) != nil {
			return raw
		}
		b, _ := json.Marshal(v)
		return b
	}
	o.Input = canonical(o.Input)
	o.Result = canonical(o.Result)
	b, _ := json.Marshal(o)
	h := hmac.New(sha256.New, s.reportKey)
	h.Write(b)
	return hex.EncodeToString(h.Sum(nil))
}
func (s *Service) Verify(o Operation, signature string) bool {
	b, err := hex.DecodeString(signature)
	if err != nil {
		return false
	}
	expected, _ := hex.DecodeString(s.Integrity(o))
	return hmac.Equal(b, expected)
}

func canonicalJSON(raw []byte) []byte {
	d := json.NewDecoder(bytes.NewReader(raw))
	d.UseNumber()
	var v any
	if d.Decode(&v) != nil {
		return nil
	}
	b, _ := json.Marshal(v)
	return b
}
func (s *Service) existing(ctx context.Context, p tenancy.Principal, id, kind string, raw []byte) (Operation, error) {
	o, err := s.Get(ctx, p, id)
	if err != nil {
		return o, err
	}
	if o.Kind != kind || !bytes.Equal(canonicalJSON(o.Input), canonicalJSON(raw)) {
		return Operation{}, auth.ErrConflict
	}
	return o, nil
}
