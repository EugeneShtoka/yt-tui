package app

import (
	"context"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/EugeneShtoka/yt-tui/internal/api"
	tuipkg "github.com/EugeneShtoka/yt-tui/internal/tui"
	"github.com/EugeneShtoka/yt-tui/internal/tui/command"
)

// statusErr returns a command that surfaces text as an error in the status bar.
func statusErr(text string) tea.Cmd {
	return func() tea.Msg { return tuipkg.StatusMsg{Text: text, IsErr: true} }
}

// emit returns a command that produces a single static message.
func emit(msg tea.Msg) tea.Cmd {
	return func() tea.Msg { return msg }
}

// globalCommands returns the full set of commands available in every view.
// Run functions return a tea.Cmd whose resulting tea.Msg flows through
// Root.Update — they never mutate state directly. names is the ordered list of
// configured panel names, used for `:tab` completion; Root resolves the chosen
// name to a panel index.
func globalCommands(ctx context.Context, backend api.Backend, names []string) []command.Command {
	return []command.Command{
		{
			Name:    "quit",
			Aliases: []string{"q"},
			Help:    "quit the application",
			Scope:   command.ScopeGlobal,
			Run:     func([]string) tea.Cmd { return tea.Quit },
		},
		{
			Name:     "tab",
			Help:     "switch to a panel  :tab <name>",
			Scope:    command.ScopeGlobal,
			Complete: prefixCompleter(names),
			Run: func(args []string) tea.Cmd {
				if len(args) == 0 {
					return statusErr("usage: :tab <name>")
				}
				return emit(tuipkg.NavigateToPanelMsg{Name: args[0]})
			},
		},
		{
			Name:  "download",
			Help:  "enqueue a video for download  :download <url>",
			Scope: command.ScopeGlobal,
			Run: func(args []string) tea.Cmd {
				if len(args) == 0 {
					return statusErr("usage: :download <url>")
				}
				return downloadCmd(ctx, backend, args[0])
			},
		},
		{
			Name:    "clear-downloads",
			Aliases: []string{"cd"},
			Help:    "dismiss the whole download queue (files untouched)",
			Scope:   command.ScopeGlobal,
			Run:     func([]string) tea.Cmd { return emit(tuipkg.ClearDownloadsMsg{}) },
		},
		{
			Name:    "delete-all-local",
			Aliases: []string{"reclaim-space"},
			Help:    "delete every downloaded file to reclaim disk space",
			Scope:   command.ScopeGlobal,
			Run: func([]string) tea.Cmd {
				return emit(tuipkg.OpenConfirmMsg{
					Prompt:    "Delete ALL downloaded files? This cannot be undone.",
					OnConfirm: tuipkg.DeleteAllLocalFilesMsg{},
				})
			},
		},
		{
			Name:    "help",
			Aliases: []string{"commands"},
			Help:    "list all commands",
			Scope:   command.ScopeGlobal,
			Run:     func([]string) tea.Cmd { return emit(tuipkg.OpenCommandHelpMsg{}) },
		},
	}
}

// prefixCompleter completes an argument prefix against a fixed candidate list
// (used by `:tab` for panel names).
func prefixCompleter(candidates []string) func(string) []string {
	return func(prefix string) []string {
		var out []string
		for _, c := range candidates {
			if strings.HasPrefix(c, prefix) {
				out = append(out, c)
			}
		}
		return out
	}
}

// downloadCmd resolves a URL's video metadata off the event loop, then hands a
// domain.Video to the same EnqueueMsg path the download key uses. Keeping the
// fetch here (not in Root) means the palette owns URL→video resolution while
// Root owns the queue mutation.
func downloadCmd(ctx context.Context, backend api.Backend, url string) tea.Cmd {
	return func() tea.Msg {
		d, err := backend.VideoDetails(ctx, url)
		if err != nil {
			return tuipkg.StatusMsg{Text: "download: " + err.Error(), IsErr: true}
		}
		return tuipkg.EnqueueMsg{Video: d.Video, AudioOnly: false}
	}
}
