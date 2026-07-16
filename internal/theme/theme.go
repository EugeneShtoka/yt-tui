package theme

import (
	"fmt"
	"os"

	"github.com/BurntSushi/toml"
)

// Theme defines the full color palette used throughout the UI.
// All values are hex color strings (e.g. "#FF6B6B").
type Theme struct {
	Accent    string `toml:"accent"`    // primary highlight: tab labels, section titles, prompts
	Muted     string `toml:"muted"`     // very dim: help text, duration col, empty progress
	Subtle    string `toml:"subtle"`    // medium dim: row numbers, col headers, channel names
	Success   string `toml:"success"`   // green: downloaded indicator, complete download tag
	Warning   string `toml:"warning"`   // yellow: download speed, history event type
	Error     string `toml:"error"`     // red: error messages, failed download tag
	BgSelect  string `toml:"bg_select"` // selected-row background fill
	Border    string `toml:"border"`    // tab bar and box borders
	Highlight string `toml:"highlight"` // newly downloaded, not yet watched
}

func Default() Theme {
	return Theme{
		Accent:    "#FF6B6B",
		Muted:     "#666666",
		Subtle:    "#888888",
		Success:   "#4CAF50",
		Warning:   "#FFC107",
		Error:     "#FF4444",
		BgSelect:  "#2a2a4e",
		Border:    "#333355",
		Highlight: "#7EC8E3",
	}
}

// Load reads a theme TOML file, filling any missing fields from Default().
func Load(path string) (Theme, error) {
	t := Default()
	data, err := os.ReadFile(path)
	if err != nil {
		return t, fmt.Errorf("Load read: %w", err)
	}
	if err := toml.Unmarshal(data, &t); err != nil {
		return t, fmt.Errorf("Load unmarshal: %w", err)
	}
	return t, nil
}

// WriteDefault writes the default theme to path as a TOML file.
// Does not overwrite an existing file.
func WriteDefault(path string) error {
	if _, err := os.Stat(path); err == nil {
		return nil
	}
	f, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("WriteDefault: %w", err)
	}
	defer f.Close()
	if err := toml.NewEncoder(f).Encode(Default()); err != nil {
		return fmt.Errorf("WriteDefault encode: %w", err)
	}
	return nil
}
