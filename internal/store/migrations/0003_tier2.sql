ALTER TABLE event ADD COLUMN city TEXT;
ALTER TABLE session ADD COLUMN city TEXT;
ALTER TABLE admin ADD COLUMN role TEXT NOT NULL DEFAULT 'admin';
ALTER TABLE admin_session ADD COLUMN user_id INTEGER;

CREATE TABLE IF NOT EXISTS funnel (
  id          INTEGER PRIMARY KEY,
  website_id  INTEGER NOT NULL,
  name        TEXT NOT NULL,
  steps_json  TEXT NOT NULL,
  created_at  INTEGER NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_funnel_site ON funnel(website_id);
