package app

import (
	"context"
	"errors"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/EugeneShtoka/yt-tui/internal/api/apitest"
	"github.com/EugeneShtoka/yt-tui/internal/domain"
	tuipkg "github.com/EugeneShtoka/yt-tui/internal/tui"
)

// runCmd executes a tea.Cmd and returns its message (nil-safe).
func runCmd(cmd tea.Cmd) tea.Msg {
	if cmd == nil {
		return nil
	}
	return cmd()
}

// flatten runs a message that may be a tea.BatchMsg, returning every leaf
// message produced (one level of batching is enough for these handlers).
func flatten(msg tea.Msg) []tea.Msg {
	batch, ok := msg.(tea.BatchMsg)
	if !ok {
		return []tea.Msg{msg}
	}
	var out []tea.Msg
	for _, c := range batch {
		out = append(out, runCmd(c))
	}
	return out
}

// blockCaptureBackend records which guarded transition the root invoked.
type blockCaptureBackend struct {
	apitest.NopBackend
	blocked    string
	unblocked  string
	blockErr   error
	unblockErr error
}

func (b *blockCaptureBackend) BlockChannel(_ context.Context, ch domain.Channel) error {
	b.blocked = ch.ID
	return b.blockErr
}

func (b *blockCaptureBackend) UnblockChannel(_ context.Context, id string) error {
	b.unblocked = id
	return b.unblockErr
}

func TestHandleBlockChannel_RoutesToBackend(t *testing.T) {
	be := &blockCaptureBackend{}
	r := Root{backend: be, tabs: []tuipkg.Tab{fakeTab{}}}

	_, cmd := r.Update(tuipkg.BlockChannelMsg{Channel: domain.Channel{ID: "ch1", Name: "C1"}, Block: true})
	res, ok := runCmd(cmd).(tuipkg.BlockChannelResultMsg)
	if !ok {
		t.Fatalf("want BlockChannelResultMsg, got %#v", runCmd(cmd))
	}
	if be.blocked != "ch1" {
		t.Errorf("BlockChannel not called with ch1, got %q", be.blocked)
	}
	if res.Err != nil || !res.Block {
		t.Errorf("result: got %#v, want Block=true no err", res)
	}
}

func TestHandleBlockChannel_UnblockRoutesToBackend(t *testing.T) {
	be := &blockCaptureBackend{}
	r := Root{backend: be, tabs: []tuipkg.Tab{fakeTab{}}}

	_, cmd := r.Update(tuipkg.BlockChannelMsg{Channel: domain.Channel{ID: "ch2"}, Block: false})
	runCmd(cmd)
	if be.unblocked != "ch2" {
		t.Errorf("UnblockChannel not called with ch2, got %q", be.unblocked)
	}
}

func TestHandleBlockChannelResult_ErrorSurfacesStatus(t *testing.T) {
	r := Root{backend: apitest.NopBackend{}, tabs: []tuipkg.Tab{fakeTab{}}}

	_, cmd := r.Update(tuipkg.BlockChannelResultMsg{
		Channel: domain.Channel{Name: "C1"},
		Block:   true,
		Err:     errors.New("boom"),
	})
	// The batch fans out to a broadcast (to tabs) and a status command; find the
	// status message.
	sm := findStatus(t, cmd)
	if !sm.IsErr {
		t.Errorf("want error status, got %#v", sm)
	}
}

func TestHandleBlockChannelResult_SuccessShowsBlocked(t *testing.T) {
	r := Root{backend: apitest.NopBackend{}, tabs: []tuipkg.Tab{fakeTab{}}}

	_, cmd := r.Update(tuipkg.BlockChannelResultMsg{
		Channel: domain.Channel{Name: "C1"},
		Block:   true,
	})
	sm := findStatus(t, cmd)
	if sm.IsErr || sm.Text == "" {
		t.Errorf("want success status, got %#v", sm)
	}
}

// findStatus drains a (possibly batched) command and returns the first StatusMsg.
func findStatus(t *testing.T, cmd tea.Cmd) tuipkg.StatusMsg {
	t.Helper()
	for _, m := range flatten(runCmd(cmd)) {
		if sm, ok := m.(tuipkg.StatusMsg); ok {
			return sm
		}
	}
	t.Fatalf("no StatusMsg produced")
	return tuipkg.StatusMsg{}
}
