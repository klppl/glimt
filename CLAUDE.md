# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

glimt is a self-hosted, cookieless, block-resistant web analytics server: a single
static Go binary backed by one SQLite file. See `README.md` for the product/deploy
story; this file is the engineering map.

## Commands

```bash
# Build / run (admin is created on first boot from these env vars)
go build ./...
GLIMT_ADMIN_USER=admin GLIMT_ADMIN_PASS=secret GLIMT_ADDR=:8080 go run ./cmd/glimt

# Tests
go test ./...
go test -race ./...
go test ./internal/ingest -run TestVisitorHash   # single test

go vet ./...
gofmt -l -w .

# Static binary exactly as the Docker build produces it (pure-Go, no cgo)
CGO_ENABLED=0 go build -trimpath -ldflags="-s -w -X main.version=dev" -o glimt ./cmd/glimt
```

Config is env vars (prefixed `GLIMT_`) overriding an optional `GLIMT_CONFIG` JSON
file — see `internal/config` and `config.example.json`. The DB and dashboard
assets/templates/snippet/migrations are all `go:embed`-ed; only the GeoIP `.mmdb`
is external.

## Hard invariants (do not break these)

These are the product's reason for existing — a change that violates one is a bug
even if it compiles and passes tests:

- **No raw IP is ever stored.** The IP exists only inside the ingest HTTP handler:
  `Collector.clientIP` → `Ingestor.build` uses it for the visitor hash and geo
  lookup, then it is dropped. It must never reach `model.Event`, the writer, or any
  table. No cookies, no localStorage, no persistent IDs, no fingerprinting.
- **Visitor identity is the daily hash only:** `sha256(salt + websiteID + ip + ua)`
  where the salt rotates daily (`internal/ingest/visitor.go`). Keep it unlinkable
  across days and irreversible.
- **Block-resistance comes from first-party + signature-free paths**, never from
  re-identifying opted-out users. The snippet (`internal/tracker/snippet.js`) stays
  tiny (<2KB) with no recognizable names; script/collect paths are random per-site
  tokens. Don't add recognizable filenames, third-party requests, or global names.

## Architecture

Two deliberately separated paths through one process (`cmd/glimt/main.go` wires
everything; `internal/web` mounts the chi router):

**Ingest hot path** — `POST /e/{token}` (`internal/ingest`). The handler looks up
the site by collect-token, enriches in-handler (UA via `internal/ua`, geo via
`internal/geo`, referrer class via `internal/referrer`, UTM/screen/lang), enqueues
on a bounded channel, returns `204`. A single writer goroutine flushes batched
transactions; `insertEvent` also stitches the event into a session (30-min
inactivity window: entry/exit page, bounce, pageview count).

**Query/dashboard path** — auth-gated (`internal/auth`, single admin, pbkdf2,
first-party session cookie). `internal/dashboard` renders server-side HTML with
HTMX; charts are hand-rolled inline SVG (`chart.go`), top-N panels are HTML bars.

**Two SQLite pools** (`internal/store`): `db.W` is the single writer
(`SetMaxOpenConns(1)`, owns all writes including the rollup worker and salt
manager); `db.R` is the read pool for the dashboard. Use the right one — writing
through `db.R` or reading large queries through `db.W` defeats the design. WAL mode
is set via DSN pragmas.

**Rollups** (`internal/rollup`) — the worker **recomputes a trailing window** every
60s (recent hours for the timeseries, today+yesterday for top-N dimensions) rather
than doing incremental deltas. Once a bucket leaves the window it is final and never
rewritten. This is why session-derived fields (bounce, exit page) stay correct
despite being mutated after insert.

**Query split for accuracy** (`internal/query`) — timeseries and top-N read from the
rollup tables; **range-unique visitor counts and session KPIs read from the
`session` table directly** (summing per-bucket uniques would overcount returning
visitors). Note the hourly upper bound is inclusive (`bucket_hour <= toHour`) so the
current partial hour counts — see the comment in `query.go`.

**Site registry** (`internal/sites`) — sites are looked up by random tokens via an
in-memory registry. All CRUD methods call `Reload()` after writing; if you add a
mutation, it must refresh the registry or the new state won't be visible to the hot
path.

## Conventions / gotchas

- **Migrations**: numbered `internal/store/migrations/NNNN_*.sql`, applied when their
  1-based ordinal exceeds `PRAGMA user_version`. Add a new numbered file (never edit
  an applied one) and use `IF NOT EXISTS` — re-running must be safe.
- **Dashboard templates** are parsed per-page (`base.html` + `<page>.html`) in
  `parseTemplates`; adding a page means adding it to that list, and template funcs
  must be registered in `funcMap`.
- **Keep dependencies minimal** (currently chi, modernc/sqlite, maxminddb,
  mileusna/useragent). Password hashing uses stdlib `crypto/pbkdf2` (Go 1.24+) on
  purpose — don't pull in `x/crypto`.
- **Real client IP** comes from `Collector.clientIP`: the configured
  `GLIMT_REAL_IP_HEADER` is honored only when the TCP peer is in
  `GLIMT_TRUSTED_PROXIES` (CIDRs / `cloudflare` / `private`); empty trust list ⇒
  honored unconditionally (dev). Country can also come from Cloudflare's
  `CF-IPCountry` (`GLIMT_CF_COUNTRY`), which overrides the mmdb lookup in `build`.
- **Timestamps** are unix milliseconds (`ts`); day/hour buckets are integer
  divisions of ms. `version` is stamped via `-ldflags -X main.version`.
- **CI** (`.github/workflows/docker.yml`) publishes `ghcr.io/klppl/glimt` on every
  run; `github.run_number` is the auto-incrementing version (`:v<N>` + `:latest`).
