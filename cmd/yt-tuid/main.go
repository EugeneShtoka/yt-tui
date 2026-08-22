// Command yt-tuid is the headless yt-tui daemon. It hosts the shared backend
// over Connect RPC (optionally TLS) behind a bearer token, so a thin yt-tui
// client can drive it remotely.
package main

import (
	"context"
	"flag"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/EugeneShtoka/yt-tui/internal/api"
	"github.com/EugeneShtoka/yt-tui/internal/backend/httpauth"
	"github.com/EugeneShtoka/yt-tui/internal/backend/media"
	"github.com/EugeneShtoka/yt-tui/internal/backend/transport"
	"github.com/EugeneShtoka/yt-tui/internal/buildinfo"
	"github.com/EugeneShtoka/yt-tui/internal/config"
	"github.com/EugeneShtoka/yt-tui/internal/db"
	"github.com/EugeneShtoka/yt-tui/internal/downloader"
	"github.com/EugeneShtoka/yt-tui/internal/youtube"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

// daemonFlags holds the parsed command-line flags for the daemon.
type daemonFlags struct {
	listen, token, tlsCert, tlsKey, configPath string
	version                                    bool
}

// parseFlags defines and parses the daemon's command-line flags.
func parseFlags() daemonFlags {
	var f daemonFlags
	flag.StringVar(&f.listen, "listen", "localhost:7373", "address to listen on")
	flag.StringVar(&f.token, "token", "", "bearer token required for all requests (overrides config)")
	flag.StringVar(&f.tlsCert, "tls-cert", "", "path to TLS certificate file")
	flag.StringVar(&f.tlsKey, "tls-key", "", "path to TLS private key file")
	flag.StringVar(&f.configPath, "config", "", "path to config file (overrides $YT_TUI_CONFIG and the default ~/.config/yt-tui/config.toml)")
	flag.BoolVar(&f.version, "version", false, "print version information and exit")
	flag.Parse()
	return f
}

func run() error {
	f := parseFlags()
	if f.version {
		fmt.Println(buildinfo.String("yt-tuid"))
		return nil
	}

	cfg, err := config.LoadFrom(f.configPath)
	if err != nil {
		return fmt.Errorf("config: %w", err)
	}
	defer cfg.Close()

	token, cert, key := resolveCreds(cfg, f.token, f.tlsCert, f.tlsKey)

	database, dbErr := db.New(cfg.DataDir, cfg.StripEmojis, cfg.RecommendedMaxAgeDays)
	if dbErr != nil {
		return fmt.Errorf("db: %w", dbErr)
	}
	defer func() { _ = database.Close() }()

	dl := downloader.New(cfg, database)
	ytClient := youtube.NewClient(cfg)
	backend := api.NewInProc(database, ytClient, dl, cfg)

	scheme := "http"
	if cert != "" && key != "" {
		scheme = "https"
	}

	mux := buildMux(backend, token)

	ln, lnErr := (&net.ListenConfig{}).Listen(context.Background(), "tcp", f.listen)
	if lnErr != nil {
		return fmt.Errorf("listen %s: %w", f.listen, lnErr)
	}
	fmt.Fprintf(os.Stderr, "yt-tuid listening on %s://%s\n", scheme, ln.Addr())

	sigCtx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM, os.Interrupt)
	defer stop()

	// Background enrichment: correct approximate dates and cache thumbnails for
	// subscribed/recommended videos, then repeat every RefreshMinutes. Tied
	// to the shutdown signal so SIGTERM stops it.
	backend.StartBackgroundEnrichment(sigCtx)

	// Refresh the cached newest-yt-dlp release in the background so the
	// availability probe this daemon serves can report how far behind its yt-dlp
	// is, without the probe itself making a request.
	go youtube.RefreshLatestVersion(sigCtx, cfg)

	srv := &http.Server{
		Handler:           httpauth.Bearer(token, mux),
		ReadHeaderTimeout: 30 * time.Second,
		// BaseContext ties every request context (including long-lived Events
		// streams) to the shutdown signal, so SIGTERM unblocks streaming
		// handlers immediately instead of Shutdown burning its full timeout
		// waiting for a stream that only exits on ctx-done.
		BaseContext: func(net.Listener) context.Context { return sigCtx },
	}

	serveErr := serve(srv, ln, sigCtx, cert, key)
	// Teardown runs unconditionally on ANY serve return — signal shutdown OR an
	// abnormal serve error. Previously the background pass was only joined on the
	// signal path, so a serve-error return fell straight into the deferred
	// database.Close() while a download or enrichment write was still in flight
	// (use-after-close, M-1). teardown cancels the enrichment context, then joins
	// both background writers, before the deferred Close runs (LIFO).
	teardown(stop, backend, dl)
	return serveErr
}

// teardown stops the background writers before the daemon's DB connection is
// closed. It cancels the shutdown context first so the enrichment loop exits
// its ticker, then joins the download workers and the enrichment pass. Ordering
// matters only in that stop() must precede WaitEnrichment (the loop runs until
// its context is done); the two joins are independent. Idempotent: stop() and
// Downloader.Stop() are safe to call once here even after a graceful shutdown.
func teardown(stop func(), backend enrichmentWaiter, dl downloadStopper) {
	stop()
	dl.Stop()
	backend.WaitEnrichment()
}

// enrichmentWaiter / downloadStopper are the narrow teardown seams (declared at
// the point of use), so teardown is unit-testable with fakes instead of a live
// backend + downloader.
type enrichmentWaiter interface{ WaitEnrichment() }

type downloadStopper interface{ Stop() }

// resolveCreds applies the flag-over-config precedence for the bearer token and
// TLS certificate/key pair.
func resolveCreds(cfg *config.Config, tokenFlag, tlsCert, tlsKey string) (token, cert, key string) {
	// Flag takes precedence over config.
	token = cfg.Token
	if tokenFlag != "" {
		token = tokenFlag
	}
	cert = cfg.TLSCert
	if tlsCert != "" {
		cert = tlsCert
	}
	key = cfg.TLSKey
	if tlsKey != "" {
		key = tlsKey
	}
	return token, cert, key
}

// buildMux wires the health check, media handler, and transport routes.
func buildMux(backend api.Backend, token string) *http.ServeMux {
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	mux.Handle("/media/", media.Handler(backend, token))
	transport.Mount(mux, backend, token)
	return mux
}

// serve runs srv on ln until it errors or sigCtx fires, then shuts it down
// gracefully within a bounded timeout. Background-writer teardown (downloader +
// enrichment) is the caller's responsibility (see teardown), so it runs on the
// serve-error path too, not only on graceful signal shutdown.
func serve(srv *http.Server, ln net.Listener, sigCtx context.Context, cert, key string) error {
	serveErr := make(chan error, 1)
	go func() {
		if cert != "" && key != "" {
			serveErr <- srv.ServeTLS(ln, cert, key)
		} else {
			serveErr <- srv.Serve(ln)
		}
	}()

	select {
	case err := <-serveErr:
		if err != http.ErrServerClosed {
			return fmt.Errorf("serve: %w", err)
		}
	case <-sigCtx.Done():
		shutCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := srv.Shutdown(shutCtx); err != nil {
			return fmt.Errorf("shutdown: %w", err)
		}
	}
	return nil
}
