# glimt

Self-hosted, privacy-first web analytics in a single static Go binary + one
SQLite file. Cookieless, no raw IP stored, and served first-party so ad-block
lists can't match it.

- **Cookieless IDs** — daily-rotating `hash(salt + site + ip + ua)`; IP used only
  in-memory at ingest, never stored. Unlinkable across days.
- **Unblockable by design** — tracking script and collector are same-origin at
  randomized paths (`/s/<token>.js`, `/e/<token>`); ~640-byte snippet, no
  recognizable names. Not fingerprinting.
- **Light** — pure-Go SQLite (no cgo), WAL, batched writes, rollup tables.
- Multi-site, single-admin dashboard (HTMX + inline-SVG, gruvbox light/dark),
  public share links, custom events.

## Run with Docker

```yaml
# docker-compose.yml
services:
  glimt:
    image: ghcr.io/klppl/glimt:latest
    restart: unless-stopped
    ports: ["8080:8080"]
    environment:
      GLIMT_ADMIN_USER: admin
      GLIMT_ADMIN_PASS: change-me            # creates the admin on first boot
      GLIMT_BASE_URL: https://stats.example.com
    volumes:
      - glimt-data:/data                     # holds /data/glimt.db
volumes:
  glimt-data:
```

```bash
docker compose up -d
```

Open the dashboard, sign in, add a site, and paste its snippet into your `<head>`:

```html
<script defer src="https://stats.example.com/s/<token>.js"></script>
```

Custom events (global name configurable via `GLIMT_JS_GLOBAL`):

```js
glimt('signup', { plan: 'pro' });
```

> The image is `linux/amd64`. If the GHCR package is private, either make it
> public (repo → Packages → settings) or `docker login ghcr.io` on the host.

## Configuration

Env vars (prefixed `GLIMT_`) override an optional `GLIMT_CONFIG` JSON file.

| Var | Default | Description |
|---|---|---|
| `GLIMT_ADDR` | `:8080` | Listen address. |
| `GLIMT_DB` | `glimt.db` | SQLite path (use `/data/glimt.db` in Docker). |
| `GLIMT_ADMIN_USER` / `_PASS` | — | Admin, created on first boot. |
| `GLIMT_BASE_URL` | — | Public URL; enables `Secure` cookies + absolute links. |
| `GLIMT_GEO_DB` | — | DB-IP or GeoLite2 `.mmdb` for country reports. |
| `GLIMT_REAL_IP_HEADER` | `X-Forwarded-For` | Header with the real client IP. |
| `GLIMT_TRUSTED_PROXIES` | — | CIDRs / `cloudflare` / `private` allowed to set that header. |
| `GLIMT_CF_COUNTRY` | `false` | Use Cloudflare `CF-IPCountry` for geo (no DB needed). |
| `GLIMT_JS_GLOBAL` | `glimt` | Custom-event global name. |

## Behind a reverse proxy

glimt must be reached on **your own domain** (first-party). Forward the client
IP and only trust that header from your proxy.

**Cloudflare:**

```bash
GLIMT_REAL_IP_HEADER=CF-Connecting-IP
GLIMT_TRUSTED_PROXIES=cloudflare,private   # drop ,private if CF hits glimt directly
GLIMT_CF_COUNTRY=true
```

`cloudflare` = CF edge ranges; add `private` when a local proxy (npmplus/nginx)
sits in between. Without `GLIMT_TRUSTED_PROXIES`, the IP header is trusted
unconditionally (fine for dev, not for production).

## GeoIP

Country/region resolves in-memory from a local `.mmdb` (DB-IP Lite needs no
account; GeoLite2 also works), or skip it entirely with `GLIMT_CF_COUNTRY=true`.

## Build from source

```bash
CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o glimt ./cmd/glimt
```

## Privacy

No cookies, no localStorage, no persistent IDs, no raw IP, no cross-day
linkability, no fingerprinting. GDPR-friendly by construction.
