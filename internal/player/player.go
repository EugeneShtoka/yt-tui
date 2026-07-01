package player

import (
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/EugeneShtoka/yt-tui/internal/config"
)

var fallbacks = []string{"mpv", "vlc", "cvlc", "ffplay"}

// startArgs returns player-specific CLI args with an optional resume offset.
func startArgs(player, filePath string, startAt time.Duration) []string {
	secs := startAt.Seconds()
	name := baseName(player)
	switch name {
	case "mpv":
		if secs > 0 {
			return []string{fmt.Sprintf("--start=%.0f", secs), filePath}
		}
	case "vlc", "cvlc":
		if secs > 0 {
			return []string{fmt.Sprintf("--start-time=%.0f", secs), filePath}
		}
	}
	return []string{filePath}
}

func baseName(player string) string {
	parts := strings.Split(player, "/")
	return parts[len(parts)-1]
}

// resolvePlayer returns the path to the first available player binary.
func resolvePlayer(cfg *config.Config) (string, error) {
	candidates := append([]string{cfg.Player}, fallbacks...)
	seen := make(map[string]bool)
	for _, name := range candidates {
		if name == "" || seen[name] {
			continue
		}
		seen[name] = true
		if path, err := exec.LookPath(name); err == nil {
			return path, nil
		}
	}
	return "", fmt.Errorf("no video player found (tried mpv, vlc, ffplay) — install one or set 'player' in config.toml")
}

// New returns the configured Backend, falling back to SimpleBackend if D-Bus is unavailable.
func New(cfg *config.Config) (Backend, error) {
	path, err := resolvePlayer(cfg)
	if err != nil {
		return nil, err
	}
	if cfg.PlayerBackend == "simple" {
		return newSimpleBackend(path), nil
	}
	// Default: MPRIS; fall back to simple if D-Bus is unavailable.
	b, dbusErr := newMPRISBackend(path)
	if dbusErr != nil {
		return newSimpleBackend(path), nil
	}
	return b, nil
}

// Play launches a player without position tracking (compatibility shim).
func Play(filePath string, cfg *config.Config) error {
	b, err := New(cfg)
	if err != nil {
		return err
	}
	return b.Launch(filePath, 0)
}
