//nolint:gosec // G115: int→int32 proto field conversions are bounded in practice (durations, counts).
package transport

import (
	"time"

	"github.com/EugeneShtoka/yt-tui/internal/api"
	v1 "github.com/EugeneShtoka/yt-tui/internal/api/backend/v1"
	"github.com/EugeneShtoka/yt-tui/internal/domain"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// ── domain → proto ────────────────────────────────────────────────────────────

func videoToProto(v domain.Video) *v1.Video {
	return &v1.Video{
		Id:         v.ID,
		Title:      v.Title,
		Channel:    v.Channel,
		ChannelId:  v.ChannelID,
		Duration:   int32(v.Duration),
		ViewCount:  v.ViewCount,
		UploadDate: v.UploadDate,
		Url:        v.URL,
	}
}

func channelToProto(ch domain.Channel) *v1.Channel {
	return &v1.Channel{
		Id:          ch.ID,
		Name:        ch.Name,
		Alias:       ch.Alias,
		Tags:        ch.Tags,
		Url:         ch.URL,
		Subscribers: ch.Subscribers,
		IsLocal:     ch.IsLocal,
	}
}

func localVideoToProto(v domain.LocalVideo) *v1.LocalVideo {
	pb := &v1.LocalVideo{
		Id:             v.ID,
		Title:          v.Title,
		Channel:        v.Channel,
		Duration:       int32(v.Duration),
		ViewCount:      v.ViewCount,
		UploadDate:     v.UploadDate,
		FilePath:       v.FilePath,
		DownloadType:   v.DownloadType,
		Status:         string(v.Status),
		LastPositionMs: v.LastPositionMs,
	}
	if !v.DownloadedAt.IsZero() {
		pb.DownloadedAt = timestamppb.New(v.DownloadedAt)
	}
	if !v.LastPlayed.IsZero() {
		pb.LastPlayed = timestamppb.New(v.LastPlayed)
	}
	return pb
}

func ytPlaylistToProto(p domain.YTPlaylist) *v1.YTPlaylist {
	return &v1.YTPlaylist{Id: p.ID, Title: p.Title}
}

func playlistToProto(p domain.Playlist) *v1.Playlist {
	return &v1.Playlist{
		Id:        p.ID,
		Name:      p.Name,
		CreatedAt: timestamppb.New(p.CreatedAt),
	}
}

func watchLaterEntryToProto(e domain.WatchLaterEntry) *v1.WatchLaterEntry {
	return &v1.WatchLaterEntry{
		VideoId: e.VideoID,
		Title:   e.Title,
		Channel: e.Channel,
		Url:     e.URL,
		AddedAt: timestamppb.New(e.AddedAt),
	}
}

func historyEntryToProto(e domain.HistoryEntry) *v1.HistoryEntry {
	return &v1.HistoryEntry{
		Id:         e.ID,
		VideoId:    e.VideoID,
		Title:      e.Title,
		Channel:    e.Channel,
		ChannelId:  e.ChannelID,
		Duration:   int32(e.Duration),
		ViewCount:  e.ViewCount,
		UploadDate: e.UploadDate,
		EventType:  e.EventType,
		Details:    e.Details,
		Timestamp:  timestamppb.New(e.Timestamp),
	}
}

func activityEntryToProto(e domain.ActivityEntry) *v1.ActivityEntry {
	return &v1.ActivityEntry{
		Id:              e.ID,
		Type:            e.Type,
		IsLocal:         e.IsLocal,
		ChannelId:       e.ChannelID,
		ChannelName:     e.ChannelName,
		PlaylistId:      e.PlaylistID,
		PlaylistLocalId: e.PlaylistLocalID,
		PlaylistName:    e.PlaylistName,
		VideoId:         e.VideoID,
		VideoTitle:      e.VideoTitle,
		Timestamp:       timestamppb.New(e.Timestamp),
	}
}

func linkToProto(l domain.Link) *v1.Link {
	return &v1.Link{Label: l.Label, Url: l.URL}
}

func chapterToProto(c domain.Chapter) *v1.Chapter {
	return &v1.Chapter{
		Title:         c.Title,
		OriginalStart: c.OriginalStart,
		OriginalEnd:   c.OriginalEnd,
		AdjustedStart: c.AdjustedStart,
		AdjustedEnd:   c.AdjustedEnd,
	}
}

func sbSegmentToProto(s domain.SBSegment) *v1.SBSegment {
	return &v1.SBSegment{Start: s.Start, End: s.End}
}

func rawChapterToProto(rc domain.RawChapter) *v1.RawChapter {
	return &v1.RawChapter{
		Title:     rc.Title,
		StartTime: rc.StartTime,
		EndTime:   rc.EndTime,
	}
}

func videoDetailsToProto(vd domain.VideoDetails) *v1.VideoDetails {
	pb := &v1.VideoDetails{
		Video:        videoToProto(vd.Video),
		Description:  vd.Description,
		ThumbnailUrl: vd.ThumbnailURL,
		Subscribers:  vd.Subscribers,
	}
	for _, rc := range vd.Chapters {
		pb.Chapters = append(pb.Chapters, rawChapterToProto(rc))
	}
	return pb
}

func cachedDetailsToProto(cd domain.CachedDetails) *v1.CachedDetails {
	pb := &v1.CachedDetails{
		Description:  cd.Description,
		ThumbnailUrl: cd.ThumbnailURL,
		Subscribers:  cd.Subscribers,
	}
	if cd.Links != nil {
		pb.LinksParsed = true
		for _, l := range *cd.Links {
			pb.Links = append(pb.Links, linkToProto(l))
		}
	}
	if cd.Chapters != nil {
		pb.ChaptersParsed = true
		for _, c := range *cd.Chapters {
			pb.Chapters = append(pb.Chapters, chapterToProto(c))
		}
	}
	if cd.SBSegments != nil {
		pb.SbSegmentsParsed = true
		for _, s := range *cd.SBSegments {
			pb.SbSegments = append(pb.SbSegments, sbSegmentToProto(s))
		}
	}
	return pb
}

func downloadItemToProto(it api.DownloadItem) *v1.DownloadItem {
	errStr := ""
	if it.Err != nil {
		errStr = it.Err.Error()
	}
	return &v1.DownloadItem{
		VideoId:   it.VideoID,
		Title:     it.Title,
		Channel:   it.Channel,
		Duration:  it.Duration,
		Url:       it.URL,
		AudioOnly: it.AudioOnly,
		Status:    string(it.Status),
		Progress:  it.Progress,
		Speed:     it.Speed,
		Eta:       it.ETA,
		FilePath:  it.FilePath,
		Error:     errStr,
	}
}

// ── slice helpers ─────────────────────────────────────────────────────────────

func videosToProto(vs []domain.Video) []*v1.Video {
	out := make([]*v1.Video, len(vs))
	for i := range vs {
		out[i] = videoToProto(vs[i])
	}
	return out
}

func channelsToProto(cs []domain.Channel) []*v1.Channel {
	out := make([]*v1.Channel, len(cs))
	for i := range cs {
		out[i] = channelToProto(cs[i])
	}
	return out
}

func protoToVideos(pbs []*v1.Video) []domain.Video {
	out := make([]domain.Video, len(pbs))
	for i, pb := range pbs {
		out[i] = protoToVideo(pb)
	}
	return out
}

func protoToChannels(pbs []*v1.Channel) []domain.Channel {
	out := make([]domain.Channel, len(pbs))
	for i, pb := range pbs {
		out[i] = protoToChannel(pb)
	}
	return out
}

// ── proto → domain ────────────────────────────────────────────────────────────

func protoToVideo(pb *v1.Video) domain.Video {
	if pb == nil {
		return domain.Video{}
	}
	return domain.Video{
		ID:         pb.Id,
		Title:      pb.Title,
		Channel:    pb.Channel,
		ChannelID:  pb.ChannelId,
		Duration:   int(pb.Duration),
		ViewCount:  pb.ViewCount,
		UploadDate: pb.UploadDate,
		URL:        pb.Url,
	}
}

func protoToChannel(pb *v1.Channel) domain.Channel {
	if pb == nil {
		return domain.Channel{}
	}
	return domain.Channel{
		ID:          pb.Id,
		Name:        pb.Name,
		Alias:       pb.Alias,
		Tags:        pb.Tags,
		URL:         pb.Url,
		Subscribers: pb.Subscribers,
		IsLocal:     pb.IsLocal,
	}
}

func protoToLocalVideo(pb *v1.LocalVideo) domain.LocalVideo {
	if pb == nil {
		return domain.LocalVideo{}
	}
	v := domain.LocalVideo{
		ID:             pb.Id,
		Title:          pb.Title,
		Channel:        pb.Channel,
		Duration:       int(pb.Duration),
		ViewCount:      pb.ViewCount,
		UploadDate:     pb.UploadDate,
		FilePath:       pb.FilePath,
		DownloadType:   pb.DownloadType,
		Status:         domain.VideoStatus(pb.Status),
		LastPositionMs: pb.LastPositionMs,
	}
	if pb.DownloadedAt != nil {
		v.DownloadedAt = pb.DownloadedAt.AsTime()
	}
	if pb.LastPlayed != nil {
		v.LastPlayed = pb.LastPlayed.AsTime()
	}
	return v
}

func protoToActivityEntry(pb *v1.ActivityEntry) domain.ActivityEntry {
	if pb == nil {
		return domain.ActivityEntry{}
	}
	e := domain.ActivityEntry{
		ID:              pb.Id,
		Type:            pb.Type,
		IsLocal:         pb.IsLocal,
		ChannelID:       pb.ChannelId,
		ChannelName:     pb.ChannelName,
		PlaylistID:      pb.PlaylistId,
		PlaylistLocalID: pb.PlaylistLocalId,
		PlaylistName:    pb.PlaylistName,
		VideoID:         pb.VideoId,
		VideoTitle:      pb.VideoTitle,
	}
	if pb.Timestamp != nil {
		e.Timestamp = pb.Timestamp.AsTime()
	} else {
		e.Timestamp = time.Time{}
	}
	return e
}
