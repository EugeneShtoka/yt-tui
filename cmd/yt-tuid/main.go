package main

import (
	"flag"
	"fmt"
	"net"
	"net/http"
	"os"

	"github.com/EugeneShtoka/yt-tui/internal/api"
	"github.com/EugeneShtoka/yt-tui/internal/backend/media"
	"github.com/EugeneShtoka/yt-tui/internal/backend/transport"
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

func run() error {
	listenAddr := flag.String("listen", "localhost:7373", "address to listen on")
	tokenFlag := flag.String("token", "", "bearer token required for all requests (overrides config)")
	tlsCert := flag.String("tls-cert", "", "path to TLS certificate file")
	tlsKey := flag.String("tls-key", "", "path to TLS private key file")
	flag.Parse()

	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("config: %w", err)
	}

	// Flag takes precedence over config.
	token := cfg.Token
	if *tokenFlag != "" {
		token = *tokenFlag
	}
	cert := cfg.TLSCert
	if *tlsCert != "" {
		cert = *tlsCert
	}
	key := cfg.TLSKey
	if *tlsKey != "" {
		key = *tlsKey
	}

	database, err := db.New(cfg.DataDir, cfg.StripEmojis, cfg.RecommendedMaxAgeDays)
	if err != nil {
		return fmt.Errorf("db: %w", err)
	}
	defer func() { _ = database.Close() }()

	dl := downloader.New(cfg, database)
	ytClient := youtube.NewClient(cfg)
	backend := api.NewInProc(database, ytClient, dl, cfg)

	scheme := "http"
	if cert != "" && key != "" {
		scheme = "https"
	}
	mediaBaseURL := scheme + "://" + *listenAddr

	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	mux.Handle("/media/", media.Handler(backend, token))
	transport.Mount(mux, backend, mediaBaseURL)

	ln, err := net.Listen("tcp", *listenAddr)
	if err != nil {
		return fmt.Errorf("listen %s: %w", *listenAddr, err)
	}
	fmt.Fprintf(os.Stderr, "yt-tuid listening on %s://%s\n", scheme, ln.Addr())

	srv := &http.Server{Handler: bearerAuth(token, mux)}
	if cert != "" && key != "" {
		if err := srv.ServeTLS(ln, cert, key); err != nil {
			return fmt.Errorf("serve tls: %w", err)
		}
	} else {
		if err := srv.Serve(ln); err != nil {
			return fmt.Errorf("serve: %w", err)
		}
	}
	return nil
}

// bearerAuth wraps h to require "Authorization: Bearer <token>" on every request.
// /healthz is exempt so load-balancers and monitoring can reach it without auth.
// When token is empty, all requests are allowed through.
func bearerAuth(token string, h http.Handler) http.Handler {
	if token == "" {
		return h
	}
	want := "Bearer " + token
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/healthz" || r.Header.Get("Authorization") == want {
			h.ServeHTTP(w, r)
			return
		}
		http.Error(w, "unauthorized", http.StatusUnauthorized)
	})
}
