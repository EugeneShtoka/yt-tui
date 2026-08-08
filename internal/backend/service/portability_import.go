package service

import (
	"context"
	"fmt"

	"github.com/EugeneShtoka/yt-tui/internal/domain/portability"
)

// ImportPreview computes a non-mutating diff of what ImportApply would write
// against the current DB. It reads existing rows and counts, but never writes,
// so a caller can show the plan before committing. An incompatible schema
// version returns early with Compatible=false and zero counts.
func (s *PortabilityService) ImportPreview(ctx context.Context, b portability.Bundle, opts portability.ImportOptions) (portability.ImportPlan, error) {
	plan := portability.ImportPlan{
		SchemaVersion: b.SchemaVersion,
		Compatible:    b.SchemaVersion == portability.SchemaVersion,
	}
	if !plan.Compatible {
		return plan, nil
	}
	if err := s.previewChannels(ctx, b, &plan); err != nil {
		return portability.ImportPlan{}, err
	}
	if err := s.previewPlaylists(ctx, b, &plan); err != nil {
		return portability.ImportPlan{}, err
	}
	plan.Videos = len(b.Videos)
	if err := s.previewRefs(ctx, b, &plan); err != nil {
		return portability.ImportPlan{}, err
	}
	if err := s.previewWatchData(ctx, b, opts, &plan); err != nil {
		return portability.ImportPlan{}, err
	}
	return plan, nil
}

func (s *PortabilityService) previewChannels(ctx context.Context, b portability.Bundle, plan *portability.ImportPlan) error {
	existing, err := s.channelsByID(ctx)
	if err != nil {
		return err
	}
	for i := range b.Channels {
		if _, ok := existing[b.Channels[i].ChannelID]; ok {
			plan.UpdatedChannels++
		} else {
			plan.NewChannels++
		}
		if b.Channels[i].Blocked {
			plan.BlockedChannels++
		}
	}
	_, names, err := s.repo.Blocklist(ctx)
	if err != nil {
		return fmt.Errorf("ImportPreview blocklist: %w", err)
	}
	nameSet := toSet(names)
	for _, n := range b.BlockedNames {
		if n != "" && !nameSet[n] {
			plan.NewBlockedNames++
		}
	}
	return nil
}

func (s *PortabilityService) previewPlaylists(ctx context.Context, b portability.Bundle, plan *portability.ImportPlan) error {
	sets, err := s.playlistVideoSets(ctx)
	if err != nil {
		return err
	}
	for i := range b.Playlists {
		pl := b.Playlists[i]
		existingSet, merged := sets[pl.Name]
		if merged {
			plan.MergedPlaylists++
		} else {
			plan.NewPlaylists++
		}
		seen := map[string]bool{}
		for _, vid := range pl.VideoIDs {
			if vid == "" || seen[vid] || existingSet[vid] {
				continue
			}
			seen[vid] = true
			plan.PlaylistAdds++
		}
	}
	return nil
}

func (s *PortabilityService) previewRefs(ctx context.Context, b portability.Bundle, plan *portability.ImportPlan) error {
	yt, err := s.repo.GetYTPlaylists(ctx)
	if err != nil {
		return fmt.Errorf("ImportPreview yt playlists: %w", err)
	}
	ytSet := make(map[string]bool, len(yt))
	for i := range yt {
		ytSet[yt[i].ID] = true
	}
	for _, y := range b.YTPlaylists {
		if y.ID != "" && !ytSet[y.ID] {
			plan.NewYTPlaylists++
		}
	}
	return nil
}

func (s *PortabilityService) previewWatchData(ctx context.Context, b portability.Bundle, opts portability.ImportOptions, plan *portability.ImportPlan) error {
	plan.HasWatchData = opts.IncludeWatchData && (len(b.History) > 0 || len(b.Positions) > 0)
	if !opts.IncludeWatchData {
		return nil
	}
	histSet, err := s.historyKeys(ctx)
	if err != nil {
		return err
	}
	for i := range b.History {
		h := b.History[i]
		if !histSet[historyKey(h.VideoID, h.EventType, h.Timestamp)] {
			plan.NewHistory++
		}
	}
	pos, err := s.repo.AllVideoPositions(ctx)
	if err != nil {
		return fmt.Errorf("ImportPreview positions: %w", err)
	}
	for _, p := range b.Positions {
		if p.VideoID != "" && p.PositionMs > pos[p.VideoID] {
			plan.NewPositions++
		}
	}
	return nil
}
