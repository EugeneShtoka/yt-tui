package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/EugeneShtoka/yt-tui/internal/api"
	"github.com/EugeneShtoka/yt-tui/internal/config"
	"github.com/EugeneShtoka/yt-tui/internal/db"
	"github.com/EugeneShtoka/yt-tui/internal/debug"
	"github.com/EugeneShtoka/yt-tui/internal/downloader"
	"github.com/EugeneShtoka/yt-tui/internal/theme"
	"github.com/EugeneShtoka/yt-tui/internal/tui/app"
	"github.com/EugeneShtoka/yt-tui/internal/tui/styles"
	"github.com/EugeneShtoka/yt-tui/internal/youtube"
	tea "github.com/charmbracelet/bubbletea"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	debugFlag := flag.Bool("debug", false, "write debug log to ~/.config/yt-tui/debug.log")
	flag.Parse()

	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("config: %w", err)
	}

	if *debugFlag {
		logPath := filepath.Join(cfg.DataDir, "debug.log")
		if initErr := debug.Init(logPath); initErr != nil {
			fmt.Fprintf(os.Stderr, "debug log: %v\n", initErr)
		} else {
			fmt.Fprintf(os.Stderr, "debug log: %s\n", logPath)
			defer debug.Close()
		}
	}

	// Write a sample theme.toml to the config dir if it doesn't exist yet.
	_ = theme.WriteDefault(filepath.Join(cfg.DataDir, "theme.toml"))

	// Load the user's theme if configured; fall back to built-in defaults.
	if cfg.Theme != "" {
		themeFile := cfg.Theme
		if !filepath.IsAbs(themeFile) {
			themeFile = filepath.Join(cfg.DataDir, themeFile)
		}
		if t, loadErr := theme.Load(themeFile); loadErr == nil {
			styles.Init(t)
		} else {
			fmt.Fprintf(os.Stderr, "theme warning: %v\n", loadErr)
		}
	}

	database, err := db.New(cfg.DataDir, cfg.StripEmojis, cfg.RecommendedMaxAgeDays)
	if err != nil {
		return fmt.Errorf("db: %w", err)
	}
	defer func() { _ = database.Close() }()

	dl := downloader.New(cfg, database)

	ytClient := youtube.NewClient(cfg)
	backend := api.NewInProc(database, ytClient, dl, cfg)

	m := app.New(backend, cfg)

	p := tea.NewProgram(m, tea.WithAltScreen(), tea.WithMouseCellMotion())
	_, err = p.Run()
	return err
}
