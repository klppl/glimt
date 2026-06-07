CREATE TABLE IF NOT EXISTS website (
  id            INTEGER PRIMARY KEY,
  uuid          TEXT NOT NULL UNIQUE,
  name          TEXT NOT NULL,
  domain        TEXT NOT NULL DEFAULT '',
  api_key_hash  TEXT,
  script_token  TEXT NOT NULL UNIQUE,
  collect_token TEXT NOT NULL UNIQUE,
  public        INTEGER NOT NULL DEFAULT 0,
  share_token   TEXT,
  created_at    INTEGER NOT NULL
);

CREATE TABLE IF NOT EXISTS salt (
  day        TEXT PRIMARY KEY,
  value      BLOB NOT NULL,
  created_at INTEGER NOT NULL
);

CREATE TABLE IF NOT EXISTS session (
  id           INTEGER PRIMARY KEY,
  website_id   INTEGER NOT NULL,
  visitor_hash BLOB NOT NULL,
  day          TEXT NOT NULL,
  started_at   INTEGER NOT NULL,
  last_seen_at INTEGER NOT NULL,
  entry_path   TEXT,
  exit_path    TEXT,
  pageviews    INTEGER NOT NULL DEFAULT 0,
  events       INTEGER NOT NULL DEFAULT 0,
  is_bounce    INTEGER NOT NULL DEFAULT 1,
  country      TEXT,
  browser      TEXT,
  os           TEXT,
  device       TEXT,
  ref_class    INTEGER,
  ref_source   TEXT
);
CREATE INDEX IF NOT EXISTS idx_session_stitch  ON session(website_id, visitor_hash, last_seen_at);
CREATE INDEX IF NOT EXISTS idx_session_started ON session(website_id, started_at);
CREATE INDEX IF NOT EXISTS idx_session_day     ON session(website_id, day);

CREATE TABLE IF NOT EXISTS event (
  id           INTEGER PRIMARY KEY,
  website_id   INTEGER NOT NULL,
  session_id   INTEGER NOT NULL,
  visitor_hash BLOB NOT NULL,
  ts           INTEGER NOT NULL,
  type         INTEGER NOT NULL,
  name         TEXT,
  url_path     TEXT,
  referrer     TEXT,
  ref_class    INTEGER,
  ref_source   TEXT,
  utm_source   TEXT,
  utm_medium   TEXT,
  utm_campaign TEXT,
  utm_term     TEXT,
  utm_content  TEXT,
  browser      TEXT,
  browser_ver  TEXT,
  os           TEXT,
  device       TEXT,
  screen_bucket TEXT,
  language     TEXT,
  country      TEXT,
  region       TEXT
);
CREATE INDEX IF NOT EXISTS idx_event_site_ts      ON event(website_id, ts);
CREATE INDEX IF NOT EXISTS idx_event_site_type_ts ON event(website_id, type, ts);
CREATE INDEX IF NOT EXISTS idx_event_session      ON event(session_id);

CREATE TABLE IF NOT EXISTS event_prop (
  event_id INTEGER NOT NULL,
  key      TEXT NOT NULL,
  value    TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_prop_event ON event_prop(event_id);
CREATE INDEX IF NOT EXISTS idx_prop_kv    ON event_prop(key, value);

CREATE TABLE IF NOT EXISTS rollup_stats (
  website_id  INTEGER NOT NULL,
  bucket_hour INTEGER NOT NULL,
  pageviews   INTEGER NOT NULL DEFAULT 0,
  visitors    INTEGER NOT NULL DEFAULT 0,
  sessions    INTEGER NOT NULL DEFAULT 0,
  PRIMARY KEY (website_id, bucket_hour)
);

CREATE TABLE IF NOT EXISTS rollup_dim (
  website_id INTEGER NOT NULL,
  bucket_day INTEGER NOT NULL,
  dimension  TEXT NOT NULL,
  value      TEXT NOT NULL,
  pageviews  INTEGER NOT NULL DEFAULT 0,
  sessions   INTEGER NOT NULL DEFAULT 0,
  visitors   INTEGER NOT NULL DEFAULT 0,
  PRIMARY KEY (website_id, bucket_day, dimension, value)
);
CREATE INDEX IF NOT EXISTS idx_dim_lookup ON rollup_dim(website_id, dimension, bucket_day);

CREATE TABLE IF NOT EXISTS admin (
  id       INTEGER PRIMARY KEY,
  username TEXT UNIQUE NOT NULL,
  pw_hash  TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS admin_session (
  token      TEXT PRIMARY KEY,
  expires_at INTEGER NOT NULL
);
