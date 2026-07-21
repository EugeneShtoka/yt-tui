package app

import (
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/EugeneShtoka/yt-tui/internal/api"
	tuipkg "github.com/EugeneShtoka/yt-tui/internal/tui"
	"github.com/EugeneShtoka/yt-tui/internal/tui/command"
)

// tabIDByName maps the lowercase command-bar name to a TabID.
var tabIDByName = map[string]tuipkg.TabID{
	"recommended":   tuipkg.TabRecommended,
	"subscriptions": tuipkg.TabSubscriptions,
	"channels":      tuipkg.TabChannels,
	"playlists":     tuipkg.TabPlaylists,
	"search":        tuipkg.TabSearch,
	"downloading":   tuipkg.TabDownloading,
	"local":         tuipkg.TabLocal,
	"history":       tuipkg.TabHistory,
	"activity":      tuipkg.TabActivity,
}

// globalCommands returns the full set of commands available in every view.
// Run functions return a tea.Cmd whose resulting tea.Msg flows through
// Root.Update — they never mutate state directly.
func globalCommands(backend api.Backend) []command.Command {
	return []command.Command{
		{
			Name:    "q",
			Aliases: []string{"quit"},
			Help:    "quit the application",
			Scope:   command.ScopeGlobal,
			Run:     func([]string) tea.Cmd { return tea.Quit },
		},
		{
			Name:  "tab",
			Help:  "switch to a tab  :tab <name>",
			Scope: command.ScopeGlobal,
			Complete: func(prefix string) []string {
				var names []string
				for name := range tabIDByName {
					if strings.HasPrefix(name, prefix) {
						names = append(names, name)
					}
				}
				return names
			},
			Run: func(args []string) tea.Cmd {
				if len(args) == 0 {
					return nil
				}
				id, ok := tabIDByName[strings.ToLower(args[0])]
				if !ok {
					return func() tea.Msg {
						return tuipkg.StatusMsg{Text: "unknown tab: " + args[0], IsErr: true}
					}
				}
				return func() tea.Msg { return tuipkg.NavigateMsg{Tab: id} }
			},
		},
		{
			Name:  "download",
			Help:  "enqueue a video for download  :download <url>",
			Scope: command.ScopeGlobal,
			Run: func(args []string) tea.Cmd {
				if len(args) == 0 {
					return func() tea.Msg {
						return tuipkg.StatusMsg{Text: "usage: :download <url>", IsErr: true}
					}
				}
				url := args[0]
				_ = backend // TODO: resolve video info then call backend.Enqueue
				return func() tea.Msg {
					return tuipkg.StatusMsg{Text: "download: " + url + " (not yet implemented)"}
				}
			},
		},
	}
}
