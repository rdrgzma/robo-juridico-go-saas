CREATE TABLE legal_operations (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
 organization_id uuid NOT NULL REFERENCES organizations(id),
 user_id uuid NOT NULL,
 kind text NOT NULL CHECK(kind IN ('cnj','judicial','liquidacao','financiamento','prazos','indices')),
 title text NOT NULL CHECK(length(title) BETWEEN 1 AND 160),
 input jsonb NOT NULL, result jsonb NOT NULL,
 engine_version text NOT NULL,
 created_at timestamptz NOT NULL DEFAULT now(),
 idempotency_key text NOT NULL CHECK(length(idempotency_key)=43),
 UNIQUE(organization_id,user_id,idempotency_key),
 FOREIGN KEY(organization_id,user_id) REFERENCES memberships(organization_id,user_id)
);
CREATE INDEX legal_operations_org_time ON legal_operations(organization_id,created_at DESC,id DESC);
