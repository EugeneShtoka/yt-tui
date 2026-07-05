package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/EugeneShtoka/yt-tui/internal/config"
	"github.com/EugeneShtoka/yt-tui/internal/db"
	"github.com/EugeneShtoka/yt-tui/internal/debug"
	"github.com/EugeneShtoka/yt-tui/internal/downloader"
	"github.com/EugeneShtoka/yt-tui/internal/theme"
	"github.com/EugeneShtoka/yt-tui/internal/ui"
)

func main() {
	debugFlag := flag.Bool("debug", false, "write debug log to ~/.config/yt-tui/debug.log")
	flag.Parse()

	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "config error: %v\n", err)
		os.Exit(1)
	}

	if *debugFlag {
		logPath := filepath.Join(cfg.DataDir, "debug.log")
		if err := debug.Init(logPath); err != nil {
			fmt.Fprintf(os.Stderr, "debug log: %v\n", err)
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
		if t, err := theme.Load(themeFile); err == nil {
			ui.InitStyles(t)
		} else {
			fmt.Fprintf(os.Stderr, "theme warning: %v\n", err)
		}
	}

	database, err := db.New(cfg.DataDir, cfg.StripEmojis)
	if err != nil {
		fmt.Fprintf(os.Stderr, "db error: %v\n", err)
		os.Exit(1)
	}
	defer database.Close()

	dl := downloader.New(cfg, database)

	m := ui.NewModel(cfg, database, dl)

	p := tea.NewProgram(m, tea.WithAltScreen(), tea.WithMouseCellMotion())
	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}
