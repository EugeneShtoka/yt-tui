package app

import (
	"context"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/EugeneShtoka/yt-tui/internal/api/apitest"
	"github.com/EugeneShtoka/yt-tui/internal/domain"
	tuipkg "github.com/EugeneShtoka/yt-tui/internal/tui"
	"github.com/EugeneShtoka/yt-tui/internal/tui/command"
)

// cmdByName finds a registered command by name (test helper).
func cmdByName(cmds []command.Command, name string) (command.Command, bool) {
	for _, c := range cmds {
		if c.Name == name {
			return c, true
		}
	}
	return command.Command{}, false
}

func runCommandMsgs(c command.Command, args ...string) tea.Msg {
	cmd := c.Run(args)
	if cmd == nil {
		return nil
	}
	return cmd()
}

// videoDetailsBackend returns a canned VideoDetails for any URL so :download can
// build an EnqueueMsg without touching yt-dlp.
type videoDetailsBackend struct {
	apitest.NopBackend
	gotURL string
}

func (b *videoDetailsBackend) VideoDetails(_ context.Context, url string) (domain.VideoDetails, error) {
	b.gotURL = url
	return domain.VideoDetails{Video: domain.Video{ID: "vid1", Title: "Clip", URL: url}}, nil
}

func TestGlobalCommandsRegistersPhase21(t *testing.T) {
	cmds := globalCommands(context.Background(), apitest.NopBackend{}, []string{"Feed"})
	for _, name := range []string{"quit", "tab", "download", "clear-downloads", "delete-all-local", "help"} {
		if _, ok := cmdByName(cmds, name); !ok {
			t.Errorf("command %q not registered", name)
		}
	}
}

func TestDownloadCommandFetchesAndEnqueues(t *testing.T) {
	be := &videoDetailsBackend{}
	cmds := globalCommands(context.Background(), be, nil)
	dl, _ := cmdByName(cmds, "download")

	msg := runCommandMsgs(dl, "https://youtu.be/vid1")
	enq, ok := msg.(tuipkg.EnqueueMsg)
	if !ok {
		t.Fatalf("download must emit EnqueueMsg, got %T", msg)
	}
	if enq.Video.ID != "vid1" {
		t.Errorf("enqueued video ID = %q, want vid1", enq.Video.ID)
	}
	if be.gotURL != "https://youtu.be/vid1" {
		t.Errorf("VideoDetails called with %q", be.gotURL)
	}
}

func TestDownloadCommandNoArgIsError(t *testing.T) {
	dl, _ := cmdByName(globalCommands(context.Background(), apitest.NopBackend{}, nil), "download")
	msg := runCommandMsgs(dl)
	sm, ok := msg.(tuipkg.StatusMsg)
	if !ok || !sm.IsErr {
		t.Fatalf("download with no arg must be an error StatusMsg, got %#v", msg)
	}
}

func TestClearDownloadsCommand(t *testing.T) {
	cd, _ := cmdByName(globalCommands(context.Background(), apitest.NopBackend{}, nil), "clear-downloads")
	if _, ok := runCommandMsgs(cd).(tuipkg.ClearDownloadsMsg); !ok {
		t.Errorf("clear-downloads must emit ClearDownloadsMsg, got %T", runCommandMsgs(cd))
	}
}

func TestDeleteAllLocalCommandOpensConfirm(t *testing.T) {
	da, _ := cmdByName(globalCommands(context.Background(), apitest.NopBackend{}, nil), "delete-all-local")
	msg := runCommandMsgs(da)
	oc, ok := msg.(tuipkg.OpenConfirmMsg)
	if !ok {
		t.Fatalf("delete-all-local must open a confirm, got %T", msg)
	}
	if _, ok := oc.OnConfirm.(tuipkg.DeleteAllLocalFilesMsg); !ok {
		t.Errorf("confirm's OnConfirm must be DeleteAllLocalFilesMsg, got %T", oc.OnConfirm)
	}
}

func TestHelpCommandOpensListing(t *testing.T) {
	h, _ := cmdByName(globalCommands(context.Background(), apitest.NopBackend{}, nil), "help")
	if _, ok := runCommandMsgs(h).(tuipkg.OpenCommandHelpMsg); !ok {
		t.Errorf("help must emit OpenCommandHelpMsg, got %T", runCommandMsgs(h))
	}
}
