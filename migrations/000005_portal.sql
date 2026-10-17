ALTER TABLE usage_events
 ADD COLUMN IF NOT EXISTS module text NOT NULL DEFAULT 'api';

CREATE INDEX IF NOT EXISTS usage_events_org_created_idx
 ON usage_events(organization_id, created_at DESC, id DESC);
