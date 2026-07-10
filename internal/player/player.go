package player

import (
	"fmt"
	"os/exec"
	"strings"

	"github.com/EugeneShtoka/yt-tui/internal/config"
)

var fallbacks = []string{"mpv", "vlc", "cvlc", "ffplay"}

func baseName(path string) string {
	parts := strings.Split(path, "/")
	name := parts[len(parts)-1]
	if idx := strings.IndexByte(name, '-'); idx > 0 {
		name = name[:idx]
	}
	return name
}

func newDriver(path string) Driver {
	switch baseName(path) {
	case "mpv":
		return &mpvDriver{path: path}
	case "vlc", "cvlc":
		return &vlcDriver{path: path}
	default:
		return &genericDriver{path: path}
	}
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
	driver := newDriver(path)
	if cfg.PlayerBackend == "simple" {
		return newSimpleBackend(driver), nil
	}
	// Default: MPRIS; fall back to simple if D-Bus is unavailable.
	b, dbusErr := newMPRISBackend(driver)
	if dbusErr != nil {
		return newSimpleBackend(driver), nil
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
