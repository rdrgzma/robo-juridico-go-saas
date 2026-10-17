ALTER TABLE organizations ADD COLUMN slug text;
UPDATE organizations SET slug='org-' || replace(id::text,'-','') WHERE slug IS NULL;
ALTER TABLE organizations ALTER COLUMN slug SET NOT NULL;
ALTER TABLE organizations ADD CONSTRAINT organizations_slug_unique UNIQUE(slug);
ALTER TABLE organizations ADD CONSTRAINT organizations_slug_format CHECK (slug ~ '^[a-z0-9][a-z0-9-]{1,62}$');

CREATE TABLE users (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(), email text NOT NULL,
 password_hash text NOT NULL, password_version integer NOT NULL DEFAULT 1,
 active boolean NOT NULL DEFAULT true, platform_admin boolean NOT NULL DEFAULT false,
 mfa_secret bytea, mfa_last_step bigint NOT NULL DEFAULT -1,
 created_at timestamptz NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX users_email_unique ON users(lower(email));
CREATE TABLE roles (code text PRIMARY KEY);
INSERT INTO roles(code) VALUES ('owner'),('admin'),('lawyer'),('viewer');
CREATE TABLE permissions (code text PRIMARY KEY);
INSERT INTO permissions(code) VALUES ('members:read'),('members:manage'),('keys:manage'),('calculations:run'),('account:read');
CREATE TABLE role_permissions (
 role_code text NOT NULL REFERENCES roles(code), permission_code text NOT NULL REFERENCES permissions(code),
 PRIMARY KEY(role_code,permission_code)
);
INSERT INTO role_permissions SELECT r.code,p.code FROM roles r CROSS JOIN permissions p WHERE r.code IN ('owner','admin');
INSERT INTO role_permissions VALUES ('lawyer','members:read'),('lawyer','calculations:run'),('lawyer','account:read'),('viewer','account:read');
CREATE TABLE memberships (
 organization_id uuid NOT NULL REFERENCES organizations(id), user_id uuid NOT NULL REFERENCES users(id),
 role_code text NOT NULL REFERENCES roles(code), active boolean NOT NULL DEFAULT true,
 PRIMARY KEY(organization_id,user_id)
);
CREATE TABLE sessions (
 token_hash bytea PRIMARY KEY CHECK(octet_length(token_hash)=32),
 organization_id uuid NOT NULL, user_id uuid NOT NULL,
 password_version integer NOT NULL, mfa_verified boolean NOT NULL DEFAULT false,
 pending_mfa bytea, created_at timestamptz NOT NULL,
 last_seen timestamptz NOT NULL, expires_at timestamptz NOT NULL,
 FOREIGN KEY(organization_id,user_id) REFERENCES memberships(organization_id,user_id)
);
CREATE INDEX sessions_user ON sessions(user_id);
CREATE TABLE recovery_codes (
 user_id uuid NOT NULL REFERENCES users(id), code_hash bytea NOT NULL,
 PRIMARY KEY(user_id,code_hash)
);
CREATE TABLE password_resets (
 token_hash bytea PRIMARY KEY, user_id uuid NOT NULL REFERENCES users(id),
 expires_at timestamptz NOT NULL, created_at timestamptz NOT NULL
);
CREATE INDEX password_resets_user ON password_resets(user_id);
CREATE TABLE mail_outbox (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(), user_id uuid NOT NULL REFERENCES users(id),
 kind text NOT NULL CHECK(kind='password_reset'), encrypted_payload bytea NOT NULL,
 created_at timestamptz NOT NULL, delivered_at timestamptz
);
CREATE TABLE auth_buckets (
 purpose text NOT NULL, bucket_hash bytea NOT NULL, window_start timestamptz NOT NULL,
 attempts integer NOT NULL CHECK(attempts>0), PRIMARY KEY(purpose,bucket_hash,window_start)
);
CREATE TABLE audit_events (
 id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
 organization_id uuid NOT NULL REFERENCES organizations(id), actor_id uuid REFERENCES users(id),
 action text NOT NULL, target_id uuid, created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX audit_org_time ON audit_events(organization_id,created_at DESC);
CREATE TABLE api_keys (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(), organization_id uuid NOT NULL REFERENCES organizations(id),
 created_by uuid NOT NULL, name text NOT NULL CHECK(length(name) BETWEEN 1 AND 100),
 prefix text NOT NULL, token_hash bytea NOT NULL UNIQUE CHECK(octet_length(token_hash)=32),
 scopes text[] NOT NULL, revoked_at timestamptz, last_used_at timestamptz,
 created_at timestamptz NOT NULL DEFAULT now(), UNIQUE(organization_id,id),
 FOREIGN KEY(organization_id,created_by) REFERENCES memberships(organization_id,user_id)
);
CREATE TABLE organization_limits (
 organization_id uuid PRIMARY KEY REFERENCES organizations(id), monthly_limit integer NOT NULL DEFAULT 1000 CHECK(monthly_limit>=0),
 requests_per_minute integer NOT NULL DEFAULT 60 CHECK(requests_per_minute BETWEEN 1 AND 6000)
);
INSERT INTO organization_limits(organization_id) SELECT id FROM organizations;
CREATE TABLE quota_periods (
 organization_id uuid NOT NULL REFERENCES organizations(id), period date NOT NULL,
 used integer NOT NULL DEFAULT 0 CHECK(used>=0), PRIMARY KEY(organization_id,period)
);
CREATE TABLE usage_events (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(), organization_id uuid NOT NULL, key_id uuid NOT NULL,
 period date NOT NULL, billable boolean NOT NULL, state text NOT NULL CHECK(state IN ('pending','completed','refunded')),
 http_status integer, created_at timestamptz NOT NULL,
 FOREIGN KEY(organization_id,key_id) REFERENCES api_keys(organization_id,id)
);
CREATE INDEX usage_pending ON usage_events(created_at) WHERE state='pending';
