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
	// subscribed/recommended videos, then repeat every ChannelRefreshMinutes. Tied
	// to the shutdown signal so SIGTERM stops it.
	backend.StartBackgroundEnrichment(sigCtx)
	// On signal shutdown, join the background pass before the deferred
	// database.Close() so no in-flight enrichment write races the close (mirrors
	// the single-binary ordering). Guarded on sigCtx: the pass now loops until the
	// signal fires, so on a serve-error return (signal not fired) we must NOT block
	// waiting for it. Registered after sigCtx/Start, it runs before the earlier
	// Close defer (LIFO).
	defer func() {
		if sigCtx.Err() != nil {
			backend.WaitEnrichment()
		}
	}()

	srv := &http.Server{
		Handler:           httpauth.Bearer(token, mux),
		ReadHeaderTimeout: 30 * time.Second,
		// BaseContext ties every request context (including long-lived Events
		// streams) to the shutdown signal, so SIGTERM unblocks streaming
		// handlers immediately instead of Shutdown burning its full timeout
		// waiting for a stream that only exits on ctx-done.
		BaseContext: func(net.Listener) context.Context { return sigCtx },
	}

	return serve(srv, ln, sigCtx, dl, cert, key)
}

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
// gracefully (stopping the downloader first) within a bounded timeout.
func serve(srv *http.Server, ln net.Listener, sigCtx context.Context, dl *downloader.Downloader, cert, key string) error {
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
		dl.Stop()
		shutCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := srv.Shutdown(shutCtx); err != nil {
			return fmt.Errorf("shutdown: %w", err)
		}
	}
	return nil
}
