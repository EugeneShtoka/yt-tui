// Package protoconv holds the canonical domain↔proto mapping for the backend v1
// API. Both the remote client (internal/api) and the transport server
// (internal/backend/transport) import it, so a schema change is edited in one
// place instead of four.
package protoconv

import (
	v1 "github.com/EugeneShtoka/yt-tui/internal/api/backend/v1"
	"github.com/EugeneShtoka/yt-tui/internal/domain"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// ── domain → proto ────────────────────────────────────────────────────────────

func VideoToProto(v domain.Video) *v1.Video {
	return &v1.Video{
		Id:         v.ID,
		Title:      v.Title,
		Channel:    v.Channel,
		ChannelId:  v.ChannelID,
		Duration:   int32(v.Duration), //nolint:gosec // G115: durations are bounded
		ViewCount:  v.ViewCount,
		UploadDate: v.UploadDate,
		Url:        v.URL,
	}
}

func ChannelToProto(ch domain.Channel) *v1.Channel {
	return &v1.Channel{
		Id:                ch.ID,
		Name:              ch.Name,
		Alias:             ch.Alias,
		Tags:              ch.Tags,
		Url:               ch.URL,
		Subscribers:       ch.Subscribers,
		IsLocal:           ch.IsLocal,
		VideosRefreshedAt: ch.VideosRefreshedAt,
		SubscriptionState: string(ch.SubState()),
		Blocked:           ch.Blocked,
		LastActivityAt:    ch.LastActivityAt,
	}
}

func LocalVideoToProto(v domain.LocalVideo) *v1.LocalVideo {
	pb := &v1.LocalVideo{
		Id:             v.ID,
		Title:          v.Title,
		Channel:        v.Channel,
		Duration:       int32(v.Duration), //nolint:gosec // G115: durations are bounded
		ViewCount:      v.ViewCount,
		UploadDate:     v.UploadDate,
		FilePath:       v.FilePath,
		FileSize:       v.FileSize,
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

func YTPlaylistToProto(p domain.YTPlaylist) *v1.YTPlaylist {
	return &v1.YTPlaylist{Id: p.ID, Title: p.Title}
}

func PlaylistToProto(p domain.Playlist) *v1.Playlist {
	return &v1.Playlist{
		Id:        p.ID,
		Name:      p.Name,
		CreatedAt: timestamppb.New(p.CreatedAt),
	}
}

func HistoryEntryToProto(e domain.HistoryEntry) *v1.HistoryEntry {
	return &v1.HistoryEntry{
		Id:         e.ID,
		VideoId:    e.VideoID,
		Title:      e.Title,
		Channel:    e.Channel,
		ChannelId:  e.ChannelID,
		Duration:   int32(e.Duration), //nolint:gosec // G115: durations are bounded
		ViewCount:  e.ViewCount,
		UploadDate: e.UploadDate,
		EventType:  e.EventType,
		Details:    e.Details,
		Timestamp:  timestamppb.New(e.Timestamp),
	}
}

func ActivityEntryToProto(e domain.ActivityEntry) *v1.ActivityEntry {
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

func LinkToProto(l domain.Link) *v1.Link {
	return &v1.Link{Label: l.Label, Url: l.URL}
}

func ChapterToProto(c domain.Chapter) *v1.Chapter {
	return &v1.Chapter{
		Title:         c.Title,
		OriginalStart: c.OriginalStart,
		OriginalEnd:   c.OriginalEnd,
		AdjustedStart: c.AdjustedStart,
		AdjustedEnd:   c.AdjustedEnd,
	}
}

func SBSegmentToProto(s domain.SBSegment) *v1.SBSegment {
	return &v1.SBSegment{Start: s.Start, End: s.End}
}

func RawChapterToProto(rc domain.RawChapter) *v1.RawChapter {
	return &v1.RawChapter{
		Title:     rc.Title,
		StartTime: rc.StartTime,
		EndTime:   rc.EndTime,
	}
}

func VideoDetailsToProto(vd domain.VideoDetails) *v1.VideoDetails {
	pb := &v1.VideoDetails{
		Video:        VideoToProto(vd.Video),
		Description:  vd.Description,
		ThumbnailUrl: vd.ThumbnailURL,
		Subscribers:  vd.Subscribers,
		Language:     vd.Language,
	}
	for _, rc := range vd.Chapters {
		pb.Chapters = append(pb.Chapters, RawChapterToProto(rc))
	}
	for _, s := range vd.SBSegments {
		pb.SbSegments = append(pb.SbSegments, SBSegmentToProto(s))
	}
	return pb
}

func CachedDetailsToProto(cd domain.CachedDetails) *v1.CachedDetails {
	pb := &v1.CachedDetails{
		Description:  cd.Description,
		ThumbnailUrl: cd.ThumbnailURL,
		Subscribers:  cd.Subscribers,
	}
	if cd.Links != nil {
		pb.LinksParsed = true
		for _, l := range *cd.Links {
			pb.Links = append(pb.Links, LinkToProto(l))
		}
	}
	if cd.Chapters != nil {
		pb.ChaptersParsed = true
		for _, c := range *cd.Chapters {
			pb.Chapters = append(pb.Chapters, ChapterToProto(c))
		}
	}
	if cd.SBSegments != nil {
		pb.SbSegmentsParsed = true
		for _, s := range *cd.SBSegments {
			pb.SbSegments = append(pb.SbSegments, SBSegmentToProto(s))
		}
	}
	return pb
}

// ── slice helpers (domain → proto) ────────────────────────────────────────────

func VideosToProto(vs []domain.Video) []*v1.Video {
	out := make([]*v1.Video, len(vs))
	for i := range vs {
		out[i] = VideoToProto(vs[i])
	}
	return out
}

func ChannelsToProto(cs []domain.Channel) []*v1.Channel {
	out := make([]*v1.Channel, len(cs))
	for i := range cs {
		out[i] = ChannelToProto(cs[i])
	}
	return out
}

// ── proto → domain ────────────────────────────────────────────────────────────

func ProtoToVideo(pb *v1.Video) domain.Video {
	if pb == nil {
		return domain.Video{}
	}
	return domain.Video{
		ID:         pb.GetId(),
		Title:      pb.GetTitle(),
		Channel:    pb.GetChannel(),
		ChannelID:  pb.GetChannelId(),
		Duration:   int(pb.GetDuration()),
		ViewCount:  pb.GetViewCount(),
		UploadDate: pb.GetUploadDate(),
		URL:        pb.GetUrl(),
	}
}

func ProtoToChannel(pb *v1.Channel) domain.Channel {
	if pb == nil {
		return domain.Channel{}
	}
	return domain.Channel{
		ID:                pb.GetId(),
		Name:              pb.GetName(),
		Alias:             pb.GetAlias(),
		Tags:              pb.GetTags(),
		URL:               pb.GetUrl(),
		Subscribers:       pb.GetSubscribers(),
		IsLocal:           pb.GetIsLocal(),
		VideosRefreshedAt: pb.GetVideosRefreshedAt(),
		State:             domain.SubscriptionState(pb.GetSubscriptionState()),
		Blocked:           pb.GetBlocked(),
		LastActivityAt:    pb.GetLastActivityAt(),
	}
}

func ProtoToLocalVideo(pb *v1.LocalVideo) domain.LocalVideo {
	if pb == nil {
		return domain.LocalVideo{}
	}
	v := domain.LocalVideo{
		ID:             pb.GetId(),
		Title:          pb.GetTitle(),
		Channel:        pb.GetChannel(),
		Duration:       int(pb.GetDuration()),
		ViewCount:      pb.GetViewCount(),
		UploadDate:     pb.GetUploadDate(),
		FilePath:       pb.GetFilePath(),
		FileSize:       pb.GetFileSize(),
		DownloadType:   pb.GetDownloadType(),
		Status:         domain.VideoStatus(pb.GetStatus()),
		LastPositionMs: pb.GetLastPositionMs(),
	}
	if pb.GetDownloadedAt() != nil {
		v.DownloadedAt = pb.GetDownloadedAt().AsTime()
	}
	if pb.GetLastPlayed() != nil {
		v.LastPlayed = pb.GetLastPlayed().AsTime()
	}
	return v
}

func ProtoToPlaylist(pb *v1.Playlist) domain.Playlist {
	if pb == nil {
		return domain.Playlist{}
	}
	p := domain.Playlist{ID: pb.GetId(), Name: pb.GetName()}
	if pb.GetCreatedAt() != nil {
		p.CreatedAt = pb.GetCreatedAt().AsTime()
	}
	return p
}

func ProtoToYTPlaylists(pbs []*v1.YTPlaylist) []domain.YTPlaylist {
	out := make([]domain.YTPlaylist, len(pbs))
	for i, pb := range pbs {
		if pb != nil {
			out[i] = domain.YTPlaylist{ID: pb.GetId(), Title: pb.GetTitle()}
		}
	}
	return out
}

func ProtoToHistoryEntry(pb *v1.HistoryEntry) domain.HistoryEntry {
	if pb == nil {
		return domain.HistoryEntry{}
	}
	e := domain.HistoryEntry{
		ID: pb.GetId(), VideoID: pb.GetVideoId(), Title: pb.GetTitle(), Channel: pb.GetChannel(), ChannelID: pb.GetChannelId(),
		Duration: int(pb.GetDuration()), ViewCount: pb.GetViewCount(), UploadDate: pb.GetUploadDate(),
		EventType: pb.GetEventType(), Details: pb.GetDetails(),
	}
	if pb.GetTimestamp() != nil {
		e.Timestamp = pb.GetTimestamp().AsTime()
	}
	return e
}

func ProtoToActivityEntry(pb *v1.ActivityEntry) domain.ActivityEntry {
	if pb == nil {
		return domain.ActivityEntry{}
	}
	e := domain.ActivityEntry{
		ID: pb.GetId(), Type: pb.GetType(), IsLocal: pb.GetIsLocal(), ChannelID: pb.GetChannelId(), ChannelName: pb.GetChannelName(),
		PlaylistID: pb.GetPlaylistId(), PlaylistLocalID: pb.GetPlaylistLocalId(), PlaylistName: pb.GetPlaylistName(),
		VideoID: pb.GetVideoId(), VideoTitle: pb.GetVideoTitle(),
	}
	if pb.GetTimestamp() != nil {
		e.Timestamp = pb.GetTimestamp().AsTime()
	}
	return e
}

func ProtoToVideoDetails(pb *v1.VideoDetails) domain.VideoDetails {
	if pb == nil {
		return domain.VideoDetails{}
	}
	vd := domain.VideoDetails{
		Video:        ProtoToVideo(pb.GetVideo()),
		Description:  pb.GetDescription(),
		ThumbnailURL: pb.GetThumbnailUrl(),
		Subscribers:  pb.GetSubscribers(),
		Language:     pb.GetLanguage(),
	}
	for _, rc := range pb.GetChapters() {
		if rc != nil {
			vd.Chapters = append(vd.Chapters, domain.RawChapter{Title: rc.GetTitle(), StartTime: rc.GetStartTime(), EndTime: rc.GetEndTime()})
		}
	}
	for _, s := range pb.GetSbSegments() {
		if s != nil {
			vd.SBSegments = append(vd.SBSegments, domain.SBSegment{Start: s.GetStart(), End: s.GetEnd()})
		}
	}
	return vd
}

func ProtoToCachedDetails(pb *v1.CachedDetails) domain.CachedDetails {
	if pb == nil {
		return domain.CachedDetails{}
	}
	cd := domain.CachedDetails{Description: pb.GetDescription(), ThumbnailURL: pb.GetThumbnailUrl(), Subscribers: pb.GetSubscribers()}
	if pb.GetLinksParsed() {
		links := make([]domain.Link, len(pb.GetLinks()))
		for i, l := range pb.GetLinks() {
			if l != nil {
				links[i] = domain.Link{Label: l.GetLabel(), URL: l.GetUrl()}
			}
		}
		cd.Links = &links
	}
	if pb.GetChaptersParsed() {
		chapters := make([]domain.Chapter, len(pb.GetChapters()))
		for i, c := range pb.GetChapters() {
			if c != nil {
				chapters[i] = domain.Chapter{Title: c.GetTitle(), OriginalStart: c.GetOriginalStart(), OriginalEnd: c.GetOriginalEnd(), AdjustedStart: c.GetAdjustedStart(), AdjustedEnd: c.GetAdjustedEnd()}
			}
		}
		cd.Chapters = &chapters
	}
	if pb.GetSbSegmentsParsed() {
		segs := make([]domain.SBSegment, len(pb.GetSbSegments()))
		for i, s := range pb.GetSbSegments() {
			if s != nil {
				segs[i] = domain.SBSegment{Start: s.GetStart(), End: s.GetEnd()}
			}
		}
		cd.SBSegments = &segs
	}
	return cd
}

// ── slice helpers (proto → domain) ────────────────────────────────────────────

func ProtoToVideos(pbs []*v1.Video) []domain.Video {
	out := make([]domain.Video, len(pbs))
	for i, pb := range pbs {
		out[i] = ProtoToVideo(pb)
	}
	return out
}

func ProtoToChannels(pbs []*v1.Channel) []domain.Channel {
	out := make([]domain.Channel, len(pbs))
	for i, pb := range pbs {
		out[i] = ProtoToChannel(pb)
	}
	return out
}

func ProtoToHistoryEntries(pbs []*v1.HistoryEntry) []domain.HistoryEntry {
	out := make([]domain.HistoryEntry, len(pbs))
	for i, pb := range pbs {
		out[i] = ProtoToHistoryEntry(pb)
	}
	return out
}
