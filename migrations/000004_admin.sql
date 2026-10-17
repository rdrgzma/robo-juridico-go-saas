CREATE TABLE plans (
 code text PRIMARY KEY CHECK (code ~ '^[a-z0-9][a-z0-9_-]{1,40}$'),
 name text NOT NULL CHECK (length(name) BETWEEN 1 AND 120),
 monthly_price numeric(14,2) NOT NULL DEFAULT 0.00 CHECK (monthly_price >= 0),
 monthly_limit integer NOT NULL CHECK (monthly_limit >= 0),
 requests_per_minute integer NOT NULL CHECK (requests_per_minute BETWEEN 1 AND 6000),
 features text[] NOT NULL DEFAULT ARRAY['cnj','judicial','liquidacao','financiamento','prazos','indices','api']::text[],
 active boolean NOT NULL DEFAULT true,
 created_at timestamptz NOT NULL DEFAULT now()
);

INSERT INTO plans(code,name,monthly_price,monthly_limit,requests_per_minute,features,active) VALUES
 ('padrao','Plano Padrão',0.00,1000,60,ARRAY['cnj','judicial','liquidacao','financiamento','prazos','indices','api'],true),
 ('essencial','Essencial Jurídico',190.00,250,30,ARRAY['cnj','judicial','liquidacao','prazos','indices'],true),
 ('escritorio','Escritório Corporativo',590.00,2500,120,ARRAY['cnj','judicial','liquidacao','financiamento','prazos','indices','api'],true);

ALTER TABLE organizations
 ADD COLUMN legal_name text NOT NULL DEFAULT '' CHECK (length(legal_name) <= 200),
 ADD COLUMN document text NOT NULL DEFAULT '' CHECK (length(document) <= 40),
 ADD COLUMN contact_name text NOT NULL DEFAULT '' CHECK (length(contact_name) <= 160),
 ADD COLUMN contact_email text NOT NULL DEFAULT '' CHECK (length(contact_email) <= 254),
 ADD COLUMN notes text NOT NULL DEFAULT '' CHECK (length(notes) <= 4000);

ALTER TABLE organization_limits
 ADD COLUMN plan_code text NOT NULL DEFAULT 'padrao' REFERENCES plans(code),
 ADD COLUMN contracted_price numeric(14,2) NOT NULL DEFAULT 0.00 CHECK (contracted_price >= 0),
 ADD COLUMN effective_from date NOT NULL DEFAULT CURRENT_DATE,
 ADD COLUMN effective_until date,
 ADD COLUMN contract_notes text NOT NULL DEFAULT '' CHECK (length(contract_notes) <= 2000),
 ADD CONSTRAINT organization_limits_effective_dates CHECK (effective_until IS NULL OR effective_until >= effective_from);

CREATE TABLE organization_contract_history (
 id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
 organization_id uuid NOT NULL REFERENCES organizations(id),
 actor_id uuid REFERENCES users(id),
 plan_code text NOT NULL REFERENCES plans(code),
 monthly_limit integer NOT NULL CHECK (monthly_limit >= 0),
 requests_per_minute integer NOT NULL CHECK (requests_per_minute BETWEEN 1 AND 6000),
 contracted_price numeric(14,2) NOT NULL CHECK (contracted_price >= 0),
 effective_from date NOT NULL,
 effective_until date,
 notes text NOT NULL DEFAULT '' CHECK (length(notes) <= 2000),
 created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX contract_history_org_time ON organization_contract_history(organization_id,created_at DESC,id DESC);

ALTER TABLE quota_periods
 ADD COLUMN blocked integer NOT NULL DEFAULT 0 CHECK (blocked >= 0);

CREATE TABLE index_sync_runs (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
 actor_id uuid REFERENCES users(id),
 index_code text NOT NULL,
 source text NOT NULL,
 start_month text NOT NULL,
 end_month text NOT NULL,
 points_synced integer NOT NULL DEFAULT 0 CHECK (points_synced >= 0),
 status text NOT NULL CHECK (status IN ('completed','failed')),
 error_message text NOT NULL DEFAULT '',
 duration_ms bigint NOT NULL DEFAULT 0 CHECK (duration_ms >= 0),
 created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX index_sync_runs_time ON index_sync_runs(created_at DESC,id DESC);
CREATE INDEX audit_events_time ON audit_events(created_at DESC,id DESC);
