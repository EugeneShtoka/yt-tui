package app

import (
	"context"

	tea "charm.land/bubbletea/v2"
	"github.com/EugeneShtoka/yt-tui/internal/domain"
	tuipkg "github.com/EugeneShtoka/yt-tui/internal/tui"
)

// actionBackend is the narrow slice of the backend the mutation command factories
// need — declared at the point of use (ISP); api.Backend satisfies it.
type actionBackend interface {
	Enqueue(ctx context.Context, v domain.Video, audioOnly bool) error
	HideRecVideo(ctx context.Context, videoID string) error
	Unsubscribe(ctx context.Context, ch domain.Channel) error
	BlockChannel(ctx context.Context, ch domain.Channel) error
	UnblockChannel(ctx context.Context, channelID string) error
	AddToWatchLater(ctx context.Context, v domain.Video) error
}

// backendActions builds the fire-and-forget tea.Cmds for backend mutations that
// need only a context and the backend — enqueue, hide, unsubscribe, block —
// extracted from Root (H-2) so those command factories aren't tangled into the
// top-level model. Each command performs the mutation off the event loop and
// reports the outcome as a message; Root's handlers own the follow-up
// orchestration (status line, optimistic-update broadcasts).
type backendActions struct{ backend actionBackend }

// enqueue queues a video for download, reporting success as EnqueueSucceededMsg
// (Root turns that into a status + a download-list refresh).
func (a backendActions) enqueue(ctx context.Context, v domain.Video, audioOnly bool) tea.Cmd {
	return func() tea.Msg {
		if err := a.backend.Enqueue(ctx, v, audioOnly); err != nil {
			return tuipkg.StatusMsg{Text: "enqueue: " + err.Error(), IsErr: true}
		}
		return tuipkg.EnqueueSucceededMsg{Title: v.Title, AudioOnly: audioOnly}
	}
}

// watchLater adds a video to Watch Later via the backend, which picks the store
// (YouTube's "WL" playlist when authed, else a local "Watch Later" playlist) —
// the TUI never talks to YouTube directly.
func (a backendActions) watchLater(ctx context.Context, v domain.Video) tea.Cmd {
	return func() tea.Msg {
		if err := a.backend.AddToWatchLater(ctx, v); err != nil {
			return tuipkg.StatusMsg{Text: "watch later: " + err.Error(), IsErr: true}
		}
		return tuipkg.StatusMsg{Text: "Added to Watch Later: " + v.Title}
	}
}

// hideChannel hides a recommended-feed video by its channel id.
func (a backendActions) hideChannel(ctx context.Context, ch domain.Channel) tea.Cmd {
	return func() tea.Msg {
		if err := a.backend.HideRecVideo(ctx, ch.ID); err != nil {
			return tuipkg.StatusMsg{Text: "hide: " + err.Error(), IsErr: true}
		}
		return tuipkg.StatusMsg{Text: "Hidden: " + ch.Name}
	}
}

// unsubscribe unsubscribes from a channel, reporting the result (including any
// error) as UnsubscribeResultMsg so tabs that removed it optimistically can
// restore it on failure.
func (a backendActions) unsubscribe(ctx context.Context, ch domain.Channel) tea.Cmd {
	return func() tea.Msg {
		err := a.backend.Unsubscribe(ctx, ch)
		return tuipkg.UnsubscribeResultMsg{Channel: ch, Err: err}
	}
}

// blockChannel runs the guarded block/unblock transition, reporting the result as
// BlockChannelResultMsg so the Channels tab can revert its optimistic change.
func (a backendActions) blockChannel(ctx context.Context, ch domain.Channel, block bool) tea.Cmd {
	return func() tea.Msg {
		var err error
		if block {
			err = a.backend.BlockChannel(ctx, ch)
		} else {
			err = a.backend.UnblockChannel(ctx, ch.ID)
		}
		return tuipkg.BlockChannelResultMsg{Channel: ch, Block: block, Err: err}
	}
}
