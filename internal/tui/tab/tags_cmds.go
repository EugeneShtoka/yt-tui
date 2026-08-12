package tab

import (
	tea "charm.land/bubbletea/v2"

	"github.com/EugeneShtoka/yt-tui/internal/domain/feed"
	tuipkg "github.com/EugeneShtoka/yt-tui/internal/tui"
)

func (t Tags) tagsDataLoadCmd() tea.Cmd {
	return func() tea.Msg {
		ctx := t.ctx
		// Load the full channel universe so the mode picker can slice it
		// (subscribed / recommended / mixed) client-side without refetching.
		chans, err := t.backend.AllChannels(ctx)
		if err != nil {
			return tuipkg.StatusMsg{Text: "tags: " + err.Error(), IsErr: true}
		}
		ids := make([]string, 0, len(chans))
		for i := range chans {
			if chans[i].IsSubscribed() {
				ids = append(ids, chans[i].ID)
			}
		}
		subVideos, _ := t.backend.GetAllChannelVideos(ctx, ids)
		feed.SortVideos(subVideos, feed.SortDate)
		// The recommended-feed cache backs the recommended/mixed modes (tags on
		// channels we don't subscribe to). Best-effort.
		recVideos, _ := t.backend.GetFeedCache(ctx, "recommended")
		return tagsDataMsg{TabTarget: tuipkg.TabTarget{Tab: tuipkg.TabTags}, chans: chans, subVideos: subVideos, recVideos: recVideos}
	}
}
