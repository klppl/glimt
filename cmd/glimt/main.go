// Command glimt is a self-hosted, privacy-respecting web analytics server.
package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/klppl/glimt/internal/auth"
	"github.com/klppl/glimt/internal/config"
	"github.com/klppl/glimt/internal/dashboard"
	"github.com/klppl/glimt/internal/geo"
	"github.com/klppl/glimt/internal/ingest"
	"github.com/klppl/glimt/internal/query"
	"github.com/klppl/glimt/internal/rollup"
	"github.com/klppl/glimt/internal/sites"
	"github.com/klppl/glimt/internal/store"
	"github.com/klppl/glimt/internal/tracker"
	"github.com/klppl/glimt/internal/web"
)

// version is set at build time via -ldflags "-X main.version=...". CI stamps it
// with the incrementing build number; local builds report "dev".
var version = "dev"

func main() {
	if err := run(); err != nil {
		log.Fatalf("glimt: %v", err)
	}
}

func run() error {
	log.Printf("glimt %s starting", version)

	cfg, err := config.Load()
	if err != nil {
		return err
	}

	db, err := store.Open(cfg.DBPath)
	if err != nil {
		return err
	}
	defer db.Close()

	g, err := geo.Open(cfg.GeoDBPath)
	if err != nil {
		return err
	}
	defer g.Close()
	if g.Enabled() {
		log.Printf("geo: loaded %s", cfg.GeoDBPath)
	} else {
		log.Printf("geo: no database configured (country reports disabled)")
	}

	salt, err := ingest.NewSaltManager(db.W)
	if err != nil {
		return err
	}

	in := ingest.New(db, salt, g)
	in.Start()

	reg, err := sites.New(db)
	if err != nil {
		return err
	}

	a := auth.New(db, cfg.SessionTTL, strings.HasPrefix(cfg.BaseURL, "https://"))
	if err := a.EnsureAdmin(cfg.AdminUser, cfg.AdminPass); err != nil {
		return err
	}

	geoEnabled := g.Enabled() || cfg.CFCountry

	q := query.New(db)
	dash, err := dashboard.New(reg, q, a, dashboard.Config{
		JSGlobal:   cfg.JSGlobal,
		BaseURL:    strings.TrimRight(cfg.BaseURL, "/"),
		GeoEnabled: geoEnabled,
	})
	if err != nil {
		return err
	}

	if cfg.CFCountry {
		log.Printf("geo: using Cloudflare CF-IPCountry header")
	}
	if len(cfg.TrustedProxyNets) > 0 {
		log.Printf("ingest: trusting real-IP header from %d proxy network(s) via %q",
			len(cfg.TrustedProxyNets), cfg.RealIPHeader)
	} else {
		log.Printf("ingest: WARNING no GLIMT_TRUSTED_PROXIES set — the %q header is trusted unconditionally",
			cfg.RealIPHeader)
	}

	col := ingest.NewCollector(reg, in, cfg.RealIPHeader, cfg.TrustedProxyNets, cfg.CFCountry)
	trk := tracker.New(reg, cfg.JSGlobal, cfg.BaseURL)

	ctx, cancel := context.WithCancel(context.Background())
	rw := rollup.New(db)
	go rw.Run(ctx)

	srv := &http.Server{
		Addr:              cfg.Addr,
		Handler:           web.Router(col, trk, dash, a),
		ReadHeaderTimeout: 10 * time.Second,
	}

	errCh := make(chan error, 1)
	go func() {
		log.Printf("glimt listening on %s", cfg.Addr)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)

	select {
	case err := <-errCh:
		cancel()
		in.Stop()
		return err
	case <-stop:
		log.Printf("glimt: shutting down")
	}

	// Graceful shutdown: stop accepting, drain ingest, stop rollups.
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()
	_ = srv.Shutdown(shutdownCtx)
	in.Stop()
	cancel()
	return nil
}
