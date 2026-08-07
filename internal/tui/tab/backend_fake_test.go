//nolint:wrapcheck // test stub — delegates to the apitest fake; errors are irrelevant
package tab

import (
	"context"

	"github.com/EugeneShtoka/yt-tui/internal/api/apitest"
	"github.com/EugeneShtoka/yt-tui/internal/domain"
)

// fakeBackend is the shared api.Backend double for the full-interface tabs
// (recommended, subscriptions, local, …). It embeds apitest.NopBackend and
// overrides only the mutation methods whose failure paths the tests exercise.
type fakeBackend struct {
	apitest.NopBackend
	hideRecVideo        func(context.Context, string) error
	deleteLocalVideo    func(context.Context, string) error
	localVideos         func(context.Context) ([]domain.LocalVideo, error)
	allChannels         func(context.Context) ([]domain.Channel, error)
	getFeedCache        func(context.Context, string) ([]domain.Video, error)
	getAllChannelVideos func(context.Context, []string) ([]domain.Video, error)
	addSubscribedChan   func(context.Context, domain.Channel) error
	setChannelTags      func(context.Context, string, []string) error
	search              func(context.Context, string) ([]domain.Channel, []domain.Video, error)
	channelVideos       func(context.Context, string, string) ([]domain.Video, error)
	deletePlaylist      func(context.Context, int64) error
	deleteYTPlaylist    func(context.Context, string) error
	localPlaylistVideos func(context.Context, int64) ([]domain.Video, error)
	removeFromPlaylist  func(context.Context, int64, string) error
	activityLog         func(context.Context, int) ([]domain.ActivityEntry, error)
}

func (f *fakeBackend) ActivityLog(ctx context.Context, limit int) ([]domain.ActivityEntry, error) {
	if f.activityLog != nil {
		return f.activityLog(ctx, limit)
	}
	return f.NopBackend.ActivityLog(ctx, limit)
}

func (f *fakeBackend) Search(ctx context.Context, q string) ([]domain.Channel, []domain.Video, error) {
	if f.search != nil {
		return f.search(ctx, q)
	}
	return f.NopBackend.Search(ctx, q)
}

func (f *fakeBackend) ChannelVideos(ctx context.Context, url, id string) ([]domain.Video, error) {
	if f.channelVideos != nil {
		return f.channelVideos(ctx, url, id)
	}
	return f.NopBackend.ChannelVideos(ctx, url, id)
}

func (f *fakeBackend) DeletePlaylist(ctx context.Context, id int64) error {
	if f.deletePlaylist != nil {
		return f.deletePlaylist(ctx, id)
	}
	return f.NopBackend.DeletePlaylist(ctx, id)
}

func (f *fakeBackend) DeleteYTPlaylist(ctx context.Context, id string) error {
	if f.deleteYTPlaylist != nil {
		return f.deleteYTPlaylist(ctx, id)
	}
	return f.NopBackend.DeleteYTPlaylist(ctx, id)
}

func (f *fakeBackend) LocalPlaylistVideos(ctx context.Context, id int64) ([]domain.Video, error) {
	if f.localPlaylistVideos != nil {
		return f.localPlaylistVideos(ctx, id)
	}
	return f.NopBackend.LocalPlaylistVideos(ctx, id)
}

func (f *fakeBackend) RemoveFromPlaylist(ctx context.Context, id int64, vid string) error {
	if f.removeFromPlaylist != nil {
		return f.removeFromPlaylist(ctx, id, vid)
	}
	return f.NopBackend.RemoveFromPlaylist(ctx, id, vid)
}

func (f *fakeBackend) GetAllChannelVideos(ctx context.Context, ids []string) ([]domain.Video, error) {
	if f.getAllChannelVideos != nil {
		return f.getAllChannelVideos(ctx, ids)
	}
	return f.NopBackend.GetAllChannelVideos(ctx, ids)
}

func (f *fakeBackend) AllChannels(ctx context.Context) ([]domain.Channel, error) {
	if f.allChannels != nil {
		return f.allChannels(ctx)
	}
	return f.NopBackend.AllChannels(ctx)
}

func (f *fakeBackend) HideRecVideo(ctx context.Context, id string) error {
	if f.hideRecVideo != nil {
		return f.hideRecVideo(ctx, id)
	}
	return f.NopBackend.HideRecVideo(ctx, id)
}

func (f *fakeBackend) DeleteLocalVideo(ctx context.Context, id string) error {
	if f.deleteLocalVideo != nil {
		return f.deleteLocalVideo(ctx, id)
	}
	return f.NopBackend.DeleteLocalVideo(ctx, id)
}

func (f *fakeBackend) LocalVideos(ctx context.Context) ([]domain.LocalVideo, error) {
	if f.localVideos != nil {
		return f.localVideos(ctx)
	}
	return f.NopBackend.LocalVideos(ctx)
}

func (f *fakeBackend) GetFeedCache(ctx context.Context, feed string) ([]domain.Video, error) {
	if f.getFeedCache != nil {
		return f.getFeedCache(ctx, feed)
	}
	return f.NopBackend.GetFeedCache(ctx, feed)
}

func (f *fakeBackend) AddSubscribedChannel(ctx context.Context, ch domain.Channel) error {
	if f.addSubscribedChan != nil {
		return f.addSubscribedChan(ctx, ch)
	}
	return f.NopBackend.AddSubscribedChannel(ctx, ch)
}

func (f *fakeBackend) SetChannelTags(ctx context.Context, id string, tags []string) error {
	if f.setChannelTags != nil {
		return f.setChannelTags(ctx, id, tags)
	}
	return f.NopBackend.SetChannelTags(ctx, id, tags)
}
