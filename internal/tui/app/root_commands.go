package app

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	tuipkg "github.com/EugeneShtoka/yt-tui/internal/tui"
	"github.com/EugeneShtoka/yt-tui/internal/tui/command"
	ovpkg "github.com/EugeneShtoka/yt-tui/internal/tui/overlay"
)

// dispatchCommandPalette routes the command-palette messages (the `:` bar, its
// resolved actions, and the confirm gate). It reports handled=false for any
// other message so updateDispatch can fall through.
func (r Root) dispatchCommandPalette(msg tea.Msg) (bool, tea.Model, tea.Cmd) {
	switch m := msg.(type) {
	case tuipkg.RunCommandMsg:
		model, cmd := r.handleRunCommand(m)
		return true, model, cmd
	case tuipkg.OpenCommandHelpMsg:
		model, cmd := r.handleOpenCommandHelp()
		return true, model, cmd
	case tuipkg.OpenConfirmMsg:
		model, cmd := r.handleOpenConfirm(m)
		return true, model, cmd
	case tuipkg.ClearDownloadsMsg:
		model, cmd := r.handleClearDownloads()
		return true, model, cmd
	case tuipkg.DeleteAllLocalFilesMsg:
		model, cmd := r.handleDeleteAllLocal()
		return true, model, cmd
	}
	return false, r, nil
}

// localCommands returns the active view's view-local commands, or nil when the
// active tab exposes none. These shadow global commands in both completion and
// resolution.
func (r Root) localCommands() []command.Command {
	if p, ok := r.activeTab().(command.Provider); ok {
		return p.Commands()
	}
	return nil
}

// handleOpenCommandBar opens the bottom-line command palette, unless it is
// already the top overlay (pressing `:` again is a no-op rather than stacking).
func (r Root) handleOpenCommandBar() (Root, tea.Cmd) {
	if n := len(r.overlays); n > 0 {
		if _, ok := r.overlays[n-1].(ovpkg.CommandBar); ok {
			return r, nil
		}
	}
	bar := ovpkg.NewCommandBar(r.keys, r.cmds, r.localCommands())
	r.overlays = append(r.overlays, bar)
	return r, bar.Init()
}

// handleRunCommand closes the command bar, then resolves and dispatches the
// typed line. Resolution lives here (not in the bar) so the single registry is
// the one source of truth and popping happens before the action runs — a
// command that opens another overlay (e.g. delete-all-local's confirm) can't
// race the bar's own close.
func (r Root) handleRunCommand(m tuipkg.RunCommandMsg) (Root, tea.Cmd) {
	if n := len(r.overlays); n > 0 {
		if _, ok := r.overlays[n-1].(ovpkg.CommandBar); ok {
			r.overlays = r.overlays[:n-1]
		}
	}
	fields := strings.Fields(m.Input)
	if len(fields) == 0 {
		return r, nil
	}
	name, args := fields[0], fields[1:]
	cmd, ok := r.cmds.Resolve(name, r.localCommands())
	if !ok {
		unknown := name
		return r, func() tea.Msg {
			return tuipkg.StatusMsg{Text: "unknown command: " + unknown, IsErr: true}
		}
	}
	return r, cmd.Run(args)
}

// handleOpenConfirm stacks a yes/no confirmation overlay for an irreversible
// action; confirming dispatches m.OnConfirm, canceling does nothing.
func (r Root) handleOpenConfirm(m tuipkg.OpenConfirmMsg) (Root, tea.Cmd) {
	r.overlays = append(r.overlays, ovpkg.NewConfirm(r.keys, m.Prompt, m.OnConfirm))
	return r, nil
}

// handleOpenCommandHelp opens the `:help` command listing, built from the
// registry plus any view-local commands, unless it is already on top.
func (r Root) handleOpenCommandHelp() (Root, tea.Cmd) {
	if n := len(r.overlays); n > 0 {
		if _, ok := r.overlays[n-1].(ovpkg.CommandHelp); ok {
			return r, nil
		}
	}
	r.overlays = append(r.overlays, ovpkg.NewCommandHelp(r.keys, r.cmds.All(r.localCommands())))
	return r, nil
}

// handleClearDownloads dismisses the whole download queue and refreshes the
// Downloading tab. Queue-only: it never touches files, the DB, or history.
func (r Root) handleClearDownloads() (Root, tea.Cmd) {
	backend := r.backend
	return r, tea.Batch(
		func() tea.Msg {
			if err := backend.ClearDownloads(r.baseCtx()); err != nil {
				return tuipkg.StatusMsg{Text: "clear downloads: " + err.Error(), IsErr: true}
			}
			return tuipkg.StatusMsg{Text: "Download queue cleared"}
		},
		func() tea.Msg { return tuipkg.DownloadItemsChangedMsg{} },
	)
}

// handleDeleteAllLocal runs the irreversible bulk file delete (already gated by
// the confirm overlay), reports the count, and reloads the Local tab.
func (r Root) handleDeleteAllLocal() (Root, tea.Cmd) {
	backend := r.backend
	return r, tea.Batch(
		func() tea.Msg {
			n, err := backend.DeleteAllLocalFiles(r.baseCtx())
			if err != nil {
				return tuipkg.StatusMsg{Text: "delete all: " + err.Error(), IsErr: true}
			}
			return tuipkg.StatusMsg{Text: fmt.Sprintf("Deleted %d local file(s)", n)}
		},
		func() tea.Msg { return tuipkg.LocalVideosChangedMsg{} },
	)
}
