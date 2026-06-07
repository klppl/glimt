# glimt

A self-hosted, privacy-respecting web analytics server in a single static Go
binary. Cookieless, no persistent identifiers, no raw IP storage — and
architecturally invisible to ad-block filter lists because everything is served
first-party from your own domain with no recognizable names.

> Umami, but lighter, prettier, and unblockable by design.

## Highlights

- **Single static binary** (pure-Go SQLite via `modernc.org/sqlite`, no cgo). One
  `.db` file, low RAM, fast cold start.
- **Cookieless visitor IDs.** A daily visitor hash is derived from a
  `salt + website + IP + user-agent`. The salt rotates every 24h and the old one
  is discarded, so hashes are unlinkable across days and irreversible to an IP.
  The raw IP is used only in-memory at ingest (hash + geo) and never stored.
- **Block-resistant by architecture, not by fingerprinting.** The tracking
  script and the collection endpoint are same-origin and served at
  operator-randomized paths (`/s/<random>.js`, `/e/<random>`). No third-party
  domain, no cookies, no `analytics.js`-style names — nothing for EasyList /
  EasyPrivacy to match. No canvas fingerprinting, no supercookies, no
  consent-defeating tricks.
- **Tiny snippet** (~640 bytes) using `navigator.sendBeacon` with a
  `fetch(keepalive)` fallback. SPA-aware (`pushState`/`popstate`).
- **Scandinavian-minimal dashboard**, server-rendered with HTMX. Inline-SVG
  charts, no SPA, no Chart.js bloat.
- **Multi-tenant** with per-site randomized paths, per-site public share links,
  single-admin auth.

## Quick start

```bash
# Build
CGO_ENABLED=0 go build -o glimt ./cmd/glimt

# Run (first admin is created from these env vars)
GLIMT_ADMIN_USER=admin GLIMT_ADMIN_PASS='a-strong-password' ./glimt
```

Open <http://localhost:8080>, sign in, add a website, and copy its install
snippet into your site's `<head>`:

```html
<script defer src="https://stats.example.com/s/6652f5fef4c882c65f.js"></script>
```

Track custom events from your site (the global name is configurable via
`GLIMT_JS_GLOBAL`, default `glimt`):

```js
glimt('signup', { plan: 'pro' });
```

## Configuration

Config comes from environment variables (recommended) and/or a JSON file pointed
to by `GLIMT_CONFIG`. Env vars always override the file. See
[`config.example.json`](config.example.json).

| Env var | Default | Description |
|---|---|---|
| `GLIMT_ADDR` | `:8080` | Listen address. |
| `GLIMT_DB` | `glimt.db` | SQLite file path. |
| `GLIMT_GEO_DB` | _(none)_ | Path to a DB-IP or GeoLite2 `.mmdb`. Omitted ⇒ country reports off. |
| `GLIMT_ADMIN_USER` | _(none)_ | Bootstrap admin username (first boot). |
| `GLIMT_ADMIN_PASS` | _(none)_ | Bootstrap admin password. Setting both on later boots resets the password. |
| `GLIMT_BASE_URL` | _(none)_ | Public URL. Enables `Secure` cookies (https) and absolute snippet/share links. |
| `GLIMT_REAL_IP_HEADER` | `X-Forwarded-For` | Header your proxy sets with the real client IP. |
| `GLIMT_JS_GLOBAL` | `glimt` | Global function name the snippet exposes for custom events. |
| `GLIMT_SESSION_TTL_HOURS` | `168` | Admin login session lifetime. |

## Deploy behind your reverse proxy (first-party is the whole point)

glimt must be reached on **your own domain** for block-resistance to hold. Run it
behind your proxy and forward the client IP. Example for nginx / npmplus:

```nginx
location ~ ^/(s|e)/ {                 # tracking script + collection endpoint
    proxy_pass http://glimt:8080;
    proxy_set_header X-Forwarded-For $remote_addr;
    proxy_set_header Host $host;
}
location / {                          # dashboard (restrict as you like)
    proxy_pass http://glimt:8080;
    proxy_set_header X-Forwarded-For $remote_addr;
    proxy_set_header Host $host;
}
```

- Behind **Cloudflare**, set `GLIMT_REAL_IP_HEADER=CF-Connecting-IP`.
- With **CrowdSec**, keep the analytics paths out of aggressive bot rules — they
  are legitimate same-origin POSTs.
- Set `GLIMT_BASE_URL=https://stats.example.com` so cookies are `Secure` and the
  install snippet shows absolute URLs.

### Docker / Dockge

A multi-stage build produces a distroless image; see [`Dockerfile`](Dockerfile)
and [`docker-compose.yml`](docker-compose.yml). Mount a volume at `/data` for the
DB (and optionally a `.mmdb` GeoIP file).

## GeoIP

glimt reads either **DB-IP Lite** (no account needed) or **MaxMind GeoLite2**
`.mmdb` files — same format. Download one, mount it, and point `GLIMT_GEO_DB` at
it. The IP is resolved to country/region in-memory at ingest and immediately
discarded.

## Architecture

- **Ingest hot path** (`/e/<token>`): validate → enrich (hash, geo, UA, referrer)
  → enqueue → `204`. A single writer goroutine flushes batched transactions to
  SQLite (WAL). The IP never reaches the writer.
- **Sessions**: events are stitched into visits with a 30-minute inactivity
  window (entry/exit page, bounce, navigation).
- **Rollups**: a worker recomputes a trailing window each minute (recent hours
  for the timeseries, today+yesterday for top-N dimensions). Older buckets are
  final and never rewritten — no fragile incremental deltas.
- **Query path**: timeseries and top-N read from rollup tables; accurate
  range-unique visitor counts and session KPIs read from the session table.

```
internal/
  config/   ingest/    (hot path: collector, enrich, visitor hash, batched writer)
  store/    sites/     (tenant registry + token lookups)
  geo/ ua/ referrer/   (enrichment)
  rollup/   query/     (aggregation + read models)
  auth/     dashboard/ (single-admin auth + HTMX UI, inline-SVG charts)
  tracker/  web/       (snippet server + chi router)
```

## Backups

Stop the process (or use the SQLite backup API / `VACUUM INTO`) and copy
`glimt.db`. WAL mode means `glimt.db-wal` / `glimt.db-shm` may be present;
checkpoint or copy all three for a hot copy.

## Privacy stance

No cookies. No localStorage. No persistent identifiers. No raw IP stored. No
cross-day linkability. No fingerprinting. GDPR-friendly by construction.
