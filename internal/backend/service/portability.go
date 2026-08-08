package service

import (
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/EugeneShtoka/yt-tui/internal/domain"
	"github.com/EugeneShtoka/yt-tui/internal/domain/portability"
)

// PortabilityRepo is the read-only persistence port required to assemble an
// export bundle. Every method maps 1:1 to an existing db.DB reader, so the DB
// layer needs no new query dedicated to export.
type PortabilityRepo interface {
	AllChannels(ctx context.Context) ([]domain.Channel, error)
	Blocklist(ctx context.Context) (ids, names []string, err error)
	Playlists(ctx context.Context) ([]domain.Playlist, error)
	PlaylistVideos(ctx context.Context, playlistID int64) ([]domain.Video, error)
	GetYTPlaylists(ctx context.Context) ([]domain.YTPlaylist, error)
	History(ctx context.Context, limit int) ([]domain.HistoryEntry, error)
	AllVideoPositions(ctx context.Context) (map[string]int64, error)
}

// PortabilityWriter is the write port used by import to apply a bundle. Every
// method maps 1:1 to an existing db.DB writer, so the service stays db-free and
// no new persistence is invented purely for import. Reuse is the point: playlist
// merge and YT-playlist writes go through the same upserts the app uses
// everywhere else; only channel-full-row and timestamped-history needed new
// (import-specific) writers on the DB.
type PortabilityWriter interface {
	UpsertChannel(ctx context.Context, ch domain.Channel) error
	AddBlockedName(ctx context.Context, name string) error
	CreatePlaylist(ctx context.Context, name string) (int64, error)
	AddToPlaylist(ctx context.Context, playlistID int64, videoID string) error
	UpsertVideo(ctx context.Context, id, title, channel, channelID string, duration int, viewCount int64, uploadDate, url string) error
	SaveYTPlaylists(ctx context.Context, playlists []domain.YTPlaylist) error
	AddHistoryEvent(ctx context.Context, videoID, eventType, details string, ts time.Time) error
	SaveVideoPosition(ctx context.Context, videoID string, ms int64) error
}

// PortabilityService assembles the export bundle from the DB and applies an
// imported bundle back into it. It owns the merge policy (idempotent upserts,
// tag union, incoming-wins alias, YT→local conversion, max position, history
// dedup); the DB ports only read and write rows.
type PortabilityService struct {
	repo PortabilityRepo
	w    PortabilityWriter
}

// NewPortabilityService wires the read and write ports. Export only needs the
// reader, so w may be nil for export-only construction; import methods require
// both. In the app both are the same *db.DB.
func NewPortabilityService(repo PortabilityRepo, w PortabilityWriter) *PortabilityService {
	return &PortabilityService{repo: repo, w: w}
}

// unlimited is passed to History to fetch every row: SQLite treats LIMIT -1 as
// "no limit", so the export captures the complete log rather than a page.
const unlimited = -1

// Export reads all app-owned data and returns a versioned bundle. Personal
// watch data (history + positions) is included only when opts.IncludeWatchData
// is set. ctx is accepted for interface symmetry; the repo readers manage their
// own context.
func (s *PortabilityService) Export(ctx context.Context, opts portability.ExportOptions) (portability.Bundle, error) {
	b := portability.Bundle{SchemaVersion: portability.SchemaVersion}

	if err := s.exportChannels(ctx, &b); err != nil {
		return portability.Bundle{}, err
	}
	if err := s.exportPlaylists(ctx, &b); err != nil {
		return portability.Bundle{}, err
	}
	if err := s.exportYTPlaylists(ctx, &b); err != nil {
		return portability.Bundle{}, err
	}
	if opts.IncludeWatchData {
		if err := s.exportWatchData(ctx, &b); err != nil {
			return portability.Bundle{}, err
		}
	}
	return b, nil
}

// exportChannels fills Channels (every known row) and BlockedNames (the
// name-only side table); ID-keyed blocks already ride along in Channels.
func (s *PortabilityService) exportChannels(ctx context.Context, b *portability.Bundle) error {
	channels, err := s.repo.AllChannels(ctx)
	if err != nil {
		return fmt.Errorf("Export channels: %w", err)
	}
	for i := range channels {
		c := &channels[i]
		b.Channels = append(b.Channels, portability.ChannelExport{
			ChannelID:         c.ID,
			Name:              c.Name,
			URL:               c.URL,
			Alias:             c.Alias,
			Tags:              c.Tags,
			SubscriptionState: string(c.SubState()),
			Blocked:           c.Blocked,
		})
	}
	_, names, err := s.repo.Blocklist(ctx)
	if err != nil {
		return fmt.Errorf("Export blocklist: %w", err)
	}
	b.BlockedNames = names
	return nil
}

// exportPlaylists fills Playlists (name + ordered video ids) and the shared,
// deduplicated Videos section they reference.
func (s *PortabilityService) exportPlaylists(ctx context.Context, b *portability.Bundle) error {
	playlists, err := s.repo.Playlists(ctx)
	if err != nil {
		return fmt.Errorf("Export playlists: %w", err)
	}
	seen := make(map[string]bool)
	for i := range playlists {
		vids, err := s.repo.PlaylistVideos(ctx, playlists[i].ID)
		if err != nil {
			return fmt.Errorf("Export playlist %q videos: %w", playlists[i].Name, err)
		}
		pe := portability.PlaylistExport{Name: playlists[i].Name}
		for j := range vids {
			v := &vids[j]
			pe.VideoIDs = append(pe.VideoIDs, v.ID)
			if !seen[v.ID] {
				seen[v.ID] = true
				b.Videos = append(b.Videos, portability.VideoExport{
					ID:         v.ID,
					Title:      v.Title,
					Channel:    v.Channel,
					ChannelID:  v.ChannelID,
					Duration:   v.Duration,
					ViewCount:  v.ViewCount,
					UploadDate: v.UploadDate,
					URL:        v.URL,
				})
			}
		}
		b.Playlists = append(b.Playlists, pe)
	}
	return nil
}

func (s *PortabilityService) exportYTPlaylists(ctx context.Context, b *portability.Bundle) error {
	pls, err := s.repo.GetYTPlaylists(ctx)
	if err != nil {
		return fmt.Errorf("Export yt playlists: %w", err)
	}
	for i := range pls {
		b.YTPlaylists = append(b.YTPlaylists, portability.YTPlaylistRef{ID: pls[i].ID, Title: pls[i].Title})
	}
	return nil
}

// exportWatchData fills the opt-in History and Positions sections.
func (s *PortabilityService) exportWatchData(ctx context.Context, b *portability.Bundle) error {
	history, err := s.repo.History(ctx, unlimited)
	if err != nil {
		return fmt.Errorf("Export history: %w", err)
	}
	for i := range history {
		h := &history[i]
		b.History = append(b.History, portability.HistoryExport{
			VideoID:    h.VideoID,
			Title:      h.Title,
			Channel:    h.Channel,
			ChannelID:  h.ChannelID,
			Duration:   h.Duration,
			ViewCount:  h.ViewCount,
			UploadDate: h.UploadDate,
			EventType:  h.EventType,
			Details:    h.Details,
			Timestamp:  h.Timestamp.Unix(),
		})
	}
	positions, err := s.repo.AllVideoPositions(ctx)
	if err != nil {
		return fmt.Errorf("Export positions: %w", err)
	}
	for id, ms := range positions {
		b.Positions = append(b.Positions, portability.PositionExport{VideoID: id, PositionMs: ms})
	}
	// Map iteration is unordered; sort so the bundle is reproducible/diffable.
	sort.Slice(b.Positions, func(i, j int) bool { return b.Positions[i].VideoID < b.Positions[j].VideoID })
	return nil
}
