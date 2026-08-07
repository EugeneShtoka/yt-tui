package service

import (
	"context"
	"fmt"
	"time"

	"github.com/EugeneShtoka/yt-tui/internal/domain"
	"github.com/EugeneShtoka/yt-tui/internal/domain/portability"
)

// ImportApply writes an imported bundle into the DB. It is idempotent at the DB
// level: re-importing the same bundle converges to the same state. The dedup'd
// sections (blocked names, playlist adds, watch-later, YT refs, history,
// positions) report zero on a re-import; channel and video rows are re-upserted
// with identical values (a harmless no-op write), so ChannelsUpserted and
// VideosUpserted count rows written rather than rows changed. Sections are
// applied in FK-safe order — videos first (they are the FK target of playlists,
// history, and positions), then everything else.
//
// Transaction granularity: each writer call is its own autocommit statement
// (matching the existing DB method style). Because every write is idempotent, a
// mid-run failure leaves a partially-applied but self-consistent DB that a retry
// converges — the practical equivalent of per-section atomicity without
// threading a *sql.Tx through the whole port surface.
func (s *PortabilityService) ImportApply(ctx context.Context, b portability.Bundle, opts portability.ImportOptions) (portability.ImportResult, error) {
	var res portability.ImportResult
	if b.SchemaVersion != portability.SchemaVersion {
		return res, fmt.Errorf("ImportApply: unsupported schema version %d (want %d)", b.SchemaVersion, portability.SchemaVersion)
	}
	// known tracks video ids with a guaranteed videos row (upserted this run),
	// so downstream sections can satisfy the FK without re-querying.
	known := make(map[string]bool, len(b.Videos))
	if err := s.applyVideos(ctx, b, known, &res); err != nil {
		return res, err
	}
	if err := s.applyChannels(ctx, b, opts, &res); err != nil {
		return res, err
	}
	if err := s.applyBlockedNames(ctx, b, &res); err != nil {
		return res, err
	}
	if err := s.applyPlaylists(ctx, b, known, &res); err != nil {
		return res, err
	}
	if err := s.applyRefs(ctx, b, &res); err != nil {
		return res, err
	}
	if opts.IncludeWatchData {
		if err := s.applyWatchData(ctx, b, known, &res); err != nil {
			return res, err
		}
	}
	return res, nil
}

func (s *PortabilityService) applyVideos(ctx context.Context, b portability.Bundle, known map[string]bool, res *portability.ImportResult) error {
	for i := range b.Videos {
		v := b.Videos[i]
		if v.ID == "" {
			continue
		}
		if err := s.w.UpsertVideo(ctx, v.ID, v.Title, v.Channel, v.ChannelID, v.Duration, v.ViewCount, v.UploadDate, v.URL); err != nil {
			return fmt.Errorf("ImportApply video %q: %w", v.ID, err)
		}
		known[v.ID] = true
		res.VideosUpserted++
	}
	return nil
}

func (s *PortabilityService) applyChannels(ctx context.Context, b portability.Bundle, opts portability.ImportOptions, res *portability.ImportResult) error {
	existing, err := s.channelsByID(ctx)
	if err != nil {
		return err
	}
	for i := range b.Channels {
		ch := resolveChannel(existing, b.Channels[i], opts)
		if ch.ID == "" {
			continue
		}
		if err := s.w.UpsertChannel(ctx, ch); err != nil {
			return fmt.Errorf("ImportApply channel %q: %w", ch.ID, err)
		}
		res.ChannelsUpserted++
	}
	return nil
}

func (s *PortabilityService) applyBlockedNames(ctx context.Context, b portability.Bundle, res *portability.ImportResult) error {
	_, names, err := s.repo.Blocklist(ctx)
	if err != nil {
		return fmt.Errorf("ImportApply blocklist: %w", err)
	}
	nameSet := toSet(names)
	for _, n := range b.BlockedNames {
		if n == "" || nameSet[n] {
			continue
		}
		if err := s.w.AddBlockedName(ctx, n); err != nil {
			return fmt.Errorf("ImportApply blocked name %q: %w", n, err)
		}
		nameSet[n] = true
		res.BlockedNames++
	}
	return nil
}

// applyPlaylists merges by name: CreatePlaylist upserts the name (returning the
// id), then each not-yet-present video ref is added. Only refs whose video was
// upserted this run are added, so the playlist_videos FK always holds — our own
// export always includes every playlist ref in the shared Videos section.
func (s *PortabilityService) applyPlaylists(ctx context.Context, b portability.Bundle, known map[string]bool, res *portability.ImportResult) error {
	sets, err := s.playlistVideoSets(ctx)
	if err != nil {
		return err
	}
	for i := range b.Playlists {
		pl := b.Playlists[i]
		if pl.Name == "" {
			continue
		}
		id, err := s.w.CreatePlaylist(ctx, pl.Name)
		if err != nil {
			return fmt.Errorf("ImportApply playlist %q: %w", pl.Name, err)
		}
		res.PlaylistsTouched++
		have := sets[pl.Name]
		if have == nil {
			have = map[string]bool{}
			sets[pl.Name] = have
		}
		for _, vid := range pl.VideoIDs {
			if vid == "" || have[vid] || !known[vid] {
				continue
			}
			if err := s.w.AddToPlaylist(ctx, id, vid); err != nil {
				return fmt.Errorf("ImportApply playlist %q add %q: %w", pl.Name, vid, err)
			}
			have[vid] = true
			res.PlaylistAdds++
		}
	}
	return nil
}

func (s *PortabilityService) applyRefs(ctx context.Context, b portability.Bundle, res *portability.ImportResult) error {
	wl, err := s.repo.WatchLater(ctx)
	if err != nil {
		return fmt.Errorf("ImportApply watch later: %w", err)
	}
	wlSet := make(map[string]bool, len(wl))
	for i := range wl {
		wlSet[wl[i].VideoID] = true
	}
	for _, w := range b.WatchLater {
		if w.VideoID == "" || wlSet[w.VideoID] {
			continue
		}
		if err := s.w.AddWatchLater(ctx, w.VideoID, w.Title, w.Channel, w.URL); err != nil {
			return fmt.Errorf("ImportApply watch later %q: %w", w.VideoID, err)
		}
		wlSet[w.VideoID] = true
		res.WatchLaterAdded++
	}
	return s.applyYTPlaylists(ctx, b, res)
}

// applyYTPlaylists merges the imported YT-playlist references with the existing
// cache. SaveYTPlaylists is replace-all, so the full merged set is written (not
// just the additions) to avoid wiping references already present.
func (s *PortabilityService) applyYTPlaylists(ctx context.Context, b portability.Bundle, res *portability.ImportResult) error {
	existing, err := s.repo.GetYTPlaylists(ctx)
	if err != nil {
		return fmt.Errorf("ImportApply yt playlists: %w", err)
	}
	ytSet := make(map[string]bool, len(existing))
	for i := range existing {
		ytSet[existing[i].ID] = true
	}
	added := 0
	merged := existing
	for _, y := range b.YTPlaylists {
		if y.ID == "" || ytSet[y.ID] {
			continue
		}
		merged = append(merged, domain.YTPlaylist{ID: y.ID, Title: y.Title})
		ytSet[y.ID] = true
		added++
	}
	if added == 0 {
		return nil
	}
	if err := s.w.SaveYTPlaylists(ctx, merged); err != nil {
		return fmt.Errorf("ImportApply save yt playlists: %w", err)
	}
	res.YTPlaylists = added
	return nil
}

func (s *PortabilityService) applyWatchData(ctx context.Context, b portability.Bundle, known map[string]bool, res *portability.ImportResult) error {
	histSet, err := s.historyKeys(ctx)
	if err != nil {
		return err
	}
	for i := range b.History {
		if e := s.applyHistoryEvent(ctx, b.History[i], known, histSet, res); e != nil {
			return e
		}
	}
	pos, err := s.repo.AllVideoPositions(ctx)
	if err != nil {
		return fmt.Errorf("ImportApply positions: %w", err)
	}
	for _, p := range b.Positions {
		// max policy; skip a position whose video has no row we can vouch for
		// (not upserted this run and no existing position ⇒ FK unsafe).
		if p.VideoID == "" || p.PositionMs <= pos[p.VideoID] {
			continue
		}
		if !known[p.VideoID] {
			if _, had := pos[p.VideoID]; !had {
				continue
			}
		}
		if err := s.w.SaveVideoPosition(ctx, p.VideoID, p.PositionMs); err != nil {
			return fmt.Errorf("ImportApply position %q: %w", p.VideoID, err)
		}
		pos[p.VideoID] = p.PositionMs
		res.PositionsSet++
	}
	return nil
}

// applyHistoryEvent inserts one event unless an identical one (video+type+second)
// already exists. A non-empty video id needs a videos row for the FK, upserted
// here from the event's inline metadata when not already known.
func (s *PortabilityService) applyHistoryEvent(ctx context.Context, h portability.HistoryExport, known, histSet map[string]bool, res *portability.ImportResult) error {
	key := historyKey(h.VideoID, h.EventType, h.Timestamp)
	if histSet[key] {
		return nil
	}
	if h.VideoID != "" && !known[h.VideoID] {
		// HistoryExport carries no URL; a history-only video has none.
		if err := s.w.UpsertVideo(ctx, h.VideoID, h.Title, h.Channel, h.ChannelID, h.Duration, h.ViewCount, h.UploadDate, ""); err != nil {
			return fmt.Errorf("ImportApply history video %q: %w", h.VideoID, err)
		}
		known[h.VideoID] = true
		res.VideosUpserted++
	}
	if err := s.w.AddHistoryEvent(ctx, h.VideoID, h.EventType, h.Details, time.Unix(h.Timestamp, 0).UTC()); err != nil {
		return fmt.Errorf("ImportApply history %q: %w", h.VideoID, err)
	}
	histSet[key] = true
	res.HistoryAdded++
	return nil
}
