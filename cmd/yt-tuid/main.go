package main

import (
	"flag"
	"fmt"
	"net"
	"net/http"
	"os"

	"github.com/EugeneShtoka/yt-tui/internal/api"
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
	flag.Parse()

	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("config: %w", err)
	}

	database, err := db.New(cfg.DataDir, cfg.StripEmojis, cfg.RecommendedMaxAgeDays)
	if err != nil {
		return fmt.Errorf("db: %w", err)
	}
	defer func() { _ = database.Close() }()

	dl := downloader.New(cfg, database)
	ytClient := youtube.NewClient(cfg)
	backend := api.NewInProc(database, ytClient, dl, cfg)

	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	transport.Mount(mux, backend)

	ln, err := net.Listen("tcp", *listenAddr)
	if err != nil {
		return fmt.Errorf("listen %s: %w", *listenAddr, err)
	}
	fmt.Fprintf(os.Stderr, "yt-tuid listening on %s\n", ln.Addr())

	srv := &http.Server{Handler: mux}
	if err := srv.Serve(ln); err != nil {
		return fmt.Errorf("serve: %w", err)
	}
	return nil
}
