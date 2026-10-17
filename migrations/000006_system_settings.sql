CREATE TABLE IF NOT EXISTS system_settings (
    key text PRIMARY KEY CHECK (length(key) BETWEEN 1 AND 80),
    value text NOT NULL DEFAULT '',
    updated_at timestamptz NOT NULL DEFAULT now(),
    updated_by uuid REFERENCES users(id)
);
