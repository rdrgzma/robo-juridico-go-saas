CREATE TABLE organizations (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(), name text NOT NULL CHECK (length(name) BETWEEN 1 AND 200),
 active boolean NOT NULL DEFAULT true, created_at timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE economic_indices (
 code text PRIMARY KEY, name text NOT NULL, source text NOT NULL CHECK (length(source)>0), verified boolean NOT NULL DEFAULT false
);
CREATE TABLE index_points (
 index_code text NOT NULL REFERENCES economic_indices(code), reference_month date NOT NULL CHECK (extract(day FROM reference_month)=1),
 percentage numeric(24,12) NOT NULL CHECK (percentage>-100), index_number numeric(32,12),
 source_url text NOT NULL, collected_at timestamptz NOT NULL DEFAULT now(), PRIMARY KEY(index_code,reference_month)
);
