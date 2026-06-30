package player

import (
	"fmt"
	"os/exec"
	"syscall"

	"github.com/EugeneShtoka/yt-tui/internal/config"
)

var fallbacks = []string{"mpv", "vlc", "cvlc", "ffplay"}

// Play launches a video player for the given file, detached from this process.
func Play(filePath string, cfg *config.Config) error {
	candidates := []string{cfg.Player}
	candidates = append(candidates, fallbacks...)

	seen := make(map[string]bool)
	for _, name := range candidates {
		if name == "" || seen[name] {
			continue
		}
		seen[name] = true
		path, err := exec.LookPath(name)
		if err != nil {
			continue
		}
		cmd := exec.Command(path, filePath)
		cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
		return cmd.Start()
	}
	return fmt.Errorf("no video player found (tried mpv, vlc, ffplay) — install one or set 'player' in config.yaml")
}
