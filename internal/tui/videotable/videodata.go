package videotable

import "github.com/EugeneShtoka/yt-tui/internal/domain"

// VideoData is a pre-enriched video row that merges a domain.Video with live playback
// state (position, watched, local status) and alias resolution.
// It replaces the VideoCell+RenderContext pattern: tabs enrich once on data load or
// state update, then pass the slice to generic column factories.
type VideoData struct {
	domain.Video
	LastPositionSecs int
	IsWatched        bool
	LocalStatus      domain.VideoStatus
	ChannelAlias     string // resolved display name; "" means use Video.Channel
}

// Enrich builds a VideoData from a Video plus live state.
func Enrich(v domain.Video, aux AuxData, aliases map[string]string) VideoData {
	vd := VideoData{Video: v}
	if posMs, ok := aux.Positions[v.ID]; ok {
		vd.LastPositionSecs = int(posMs / 1000)
	}
	vd.IsWatched = aux.Watched[v.ID]
	if st, ok := aux.LocalStatus[v.ID]; ok {
		vd.LocalStatus = st
	}
	if aliases != nil {
		vd.ChannelAlias = aliases[v.ChannelID]
	}
	return vd
}

// EnrichAll builds a []VideoData slice from a []domain.Video.
func EnrichAll(videos []domain.Video, aux AuxData, aliases map[string]string) []VideoData {
	out := make([]VideoData, len(videos))
	for i, v := range videos {
		out[i] = Enrich(v, aux, aliases)
	}
	return out
}

// — HasTitle —
func (vd VideoData) GetTitle() string { return vd.Title }

// — HasAudioTitle — videos are never audio content in this context
func (vd VideoData) GetBaseTitle() string { return vd.Title }
func (vd VideoData) IsAudio() bool        { return false }

// — HasChannelInfo —
func (vd VideoData) GetChannelID() string { return vd.ChannelID }
func (vd VideoData) GetChannelName() string {
	if vd.ChannelAlias != "" {
		return vd.ChannelAlias
	}
	return vd.Channel
}

// — HasCount —
func (vd VideoData) GetCount() int64 { return vd.ViewCount }

// — HasDate —
func (vd VideoData) GetRawDate() string { return vd.UploadDate }

// — HasDuration —
func (vd VideoData) GetDurationSecs() int     { return vd.Duration }
func (vd VideoData) GetLastPositionSecs() int { return vd.LastPositionSecs }

// — HasIndicator —
func (vd VideoData) GetIndicator() string {
	if vd.LastPositionSecs > 0 {
		return " ○ "
	}
	if vd.IsWatched {
		return " ○ "
	}
	switch vd.LocalStatus {
	case domain.StatusNew:
		return " ● "
	case domain.StatusStarted, domain.StatusWatched:
		return " ○ "
	}
	return "   "
}

// isFadedVD returns true when a VideoData row should be rendered with Dim style.
func isFadedVD(vd VideoData) bool {
	if vd.LocalStatus == domain.StatusStarted || vd.LocalStatus == domain.StatusWatched {
		return true
	}
	return vd.LastPositionSecs > 0 || vd.IsWatched
}
