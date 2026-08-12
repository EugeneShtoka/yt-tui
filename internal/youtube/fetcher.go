package youtube

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"

	"github.com/EugeneShtoka/yt-tui/internal/config"
	"github.com/EugeneShtoka/yt-tui/internal/domain"
	"github.com/EugeneShtoka/yt-tui/internal/procexec"
	"github.com/EugeneShtoka/yt-tui/internal/text"
)

// Client wraps config for plain (non-tea) YouTube fetch operations. The exec +
// retry machinery lives in runner.go and the yt-dlp JSON parsers in parse.go.
type Client struct {
	cfg    *config.Config
	runner procexec.Runner
}

// NewClient creates a new Client that execs the real yt-dlp binary.
func NewClient(cfg *config.Config) *Client {
	return &Client{cfg: cfg, runner: procexec.OS{}}
}

// subtitleSRTArgs returns the yt-dlp flags that fetch captions/auto-subtitles and
// convert them to .srt. langs is a --sub-langs value; empty falls back to English.
// Shared by FetchTranscript and VideoDetailsWithTranscript so the flag set and the
// English fallback can't drift between the two transcript paths.
func subtitleSRTArgs(langs string) []string {
	if langs == "" {
		langs = "en.*"
	}
	return []string{
		"--write-sub", "--write-auto-sub",
		"--sub-langs", langs,
		"--convert-subs", "srt",
	}
}

// FetchTranscript downloads a video's captions/auto-subtitles as sidecar .srt
// files (no media download), using the given yt-dlp output template. langs is a
// yt-dlp --sub-langs value; empty falls back to English. Used to save transcripts
// during enrichment and when video info is actively requested.
func (c *Client) FetchTranscript(ctx context.Context, videoURL, langs, outTemplate string) error {
	args := []string{"--skip-download"}
	args = append(args, subtitleSRTArgs(langs)...)
	args = append(args,
		"--no-playlist", "--no-warnings", "--quiet",
		"-o", outTemplate,
	)
	args = append(args, cookieArgs(c.cfg)...)
	args = append(args, videoURL)
	if _, err := runYtdlpOutputWithRetry(ctx, c.runner, "transcript", args); err != nil {
		return fmt.Errorf("FetchTranscript: %w", err)
	}
	return nil
}

func applyStripEmojisVideos(vv []domain.Video) []domain.Video {
	for i := range vv {
		vv[i].Title = text.StripEmojis(vv[i].Title)
		vv[i].Channel = text.StripEmojis(vv[i].Channel)
	}
	return vv
}

func applyStripEmojisChannels(cc []domain.Channel) []domain.Channel {
	for i := range cc {
		cc[i].Name = text.StripEmojis(cc[i].Name)
	}
	return cc
}

// Recommended fetches the recommended feed videos (intentionally capped by config).
func (c *Client) Recommended(ctx context.Context) ([]domain.Video, error) {
	limit := c.cfg.RecommendedFetchCount
	if limit <= 0 {
		limit = 150
	}
	args := buildArgs(c.cfg, "https://www.youtube.com/feed/recommended", limit)
	videos, _, err := c.runAndParseVideos(ctx, args)
	if c.cfg.StripEmojis {
		videos = applyStripEmojisVideos(videos)
	}
	return videos, err
}

// SubscribedChannels fetches all subscribed channels, paginated.
func (c *Client) SubscribedChannels(ctx context.Context) ([]domain.Channel, error) {
	u := "https://www.youtube.com/feed/channels"
	var all []domain.Channel
	for start := 1; ; start += pageSize {
		if err := ctx.Err(); err != nil {
			return all, fmt.Errorf("SubscribedChannels: %w", err)
		}
		args := buildArgsPage(c.cfg, u, start)
		page, raw, err := c.runAndParseChannels(ctx, args)
		if err != nil {
			return all, err
		}
		if c.cfg.StripEmojis {
			page = applyStripEmojisChannels(page)
		}
		all = append(all, page...)
		if raw < pageSize {
			break
		}
	}
	return all, nil
}

// channelVideosURL builds the /videos listing URL for a channel from either an
// explicit channel URL or a bare channel ID, appending /videos when absent.
func channelVideosURL(channelURL, channelID string) string {
	u := channelURL
	if u == "" {
		u = "https://www.youtube.com/channel/" + channelID
	}
	if !strings.HasSuffix(u, "/videos") {
		u += "/videos"
	}
	return u
}

// ChannelVideos fetches all videos for a channel, paginated.
func (c *Client) ChannelVideos(ctx context.Context, channelURL, channelID string) ([]domain.Video, error) {
	return c.ChannelVideosStream(ctx, channelURL, channelID, nil)
}

// ChannelVideosPage fetches a single page (pageSize wide) of a channel's videos,
// already emoji-stripped per config, starting at the 1-based list offset `start`
// (start=1 is the newest video). It returns the offset to resume from for the
// next page and whether more pages remain (the raw page came back full), so a
// caller driving a round-robin crawl across many channels can advance one page
// per channel at a time — and persist nextStart-1 as a resume cursor — instead
// of draining each channel before moving to the next.
func (c *Client) ChannelVideosPage(ctx context.Context, channelURL, channelID string, start int) (videos []domain.Video, nextStart int, more bool, err error) {
	if cerr := ctx.Err(); cerr != nil {
		return nil, start, false, fmt.Errorf("ChannelVideosPage: %w", cerr)
	}
	args := buildArgsPage(c.cfg, channelVideosURL(channelURL, channelID), start)
	page, raw, err := c.runAndParseVideos(ctx, args)
	if err != nil {
		return nil, start, false, err
	}
	if c.cfg.StripEmojis {
		page = applyStripEmojisVideos(page)
	}
	return page, start + pageSize, raw >= pageSize, nil
}

// ChannelVideosStream fetches all videos for a channel, paginated, invoking
// onPage after each page is fetched and parsed (already emoji-stripped) and
// before the next page is requested. It lets callers persist a page at a time so
// a long back-catalog crawl becomes visible before the full pull finishes. An
// onPage error aborts the crawl; onPage may be nil. The full accumulated list is
// always returned, including on early exit.
func (c *Client) ChannelVideosStream(ctx context.Context, channelURL, channelID string, onPage func([]domain.Video) error) ([]domain.Video, error) {
	var all []domain.Video
	for start := 1; ; {
		page, nextStart, more, err := c.ChannelVideosPage(ctx, channelURL, channelID, start)
		if err != nil {
			return all, err
		}
		all = append(all, page...)
		if onPage != nil && len(page) > 0 {
			if err := onPage(page); err != nil {
				return all, fmt.Errorf("ChannelVideos onPage: %w", err)
			}
		}
		if !more {
			break
		}
		start = nextStart
	}
	return all, nil
}

// ChannelLatest fetches the cfg.ChannelLatestCount most recent videos for a channel.
func (c *Client) ChannelLatest(ctx context.Context, channelURL, channelID string) ([]domain.Video, error) {
	return c.ChannelLatestN(ctx, channelURL, channelID, c.cfg.ChannelLatestCount)
}

// ChannelLatestN fetches at most n recent videos for a channel (intentionally capped).
func (c *Client) ChannelLatestN(ctx context.Context, channelURL, channelID string, n int) ([]domain.Video, error) {
	args := buildArgs(c.cfg, channelVideosURL(channelURL, channelID), n)
	videos, _, err := c.runAndParseVideos(ctx, args)
	if c.cfg.StripEmojis {
		videos = applyStripEmojisVideos(videos)
	}
	return videos, err
}

// Search searches YouTube for the given query (intentionally capped results).
func (c *Client) Search(ctx context.Context, query string) (channels []domain.Channel, videos []domain.Video, err error) {
	type chResult struct {
		channels []domain.Channel
		err      error
	}
	chCh := make(chan chResult, 1)
	go func() {
		chURL := "https://www.youtube.com/results?search_query=" +
			url.QueryEscape(query) + "&sp=EgIQAg%3D%3D"
		args := buildArgs(c.cfg, chURL, 10)
		chs, _, chErr := c.runAndParseMixed(ctx, args)
		if c.cfg.StripEmojis {
			chs = applyStripEmojisChannels(chs)
		}
		chCh <- chResult{chs, chErr}
	}()

	vidArgs := buildArgs(c.cfg, "ytsearch25:"+query, 25)
	_, videos, err = c.runAndParseMixed(ctx, vidArgs)
	if c.cfg.StripEmojis {
		videos = applyStripEmojisVideos(videos)
	}

	cr := <-chCh
	if err == nil && cr.err != nil {
		err = cr.err
	}
	return cr.channels, videos, err
}

// YTPlaylists fetches all user playlists, paginated.
func (c *Client) YTPlaylists(ctx context.Context) ([]domain.YTPlaylist, error) {
	u := "https://www.youtube.com/feed/playlists"
	var all []domain.YTPlaylist
	for start := 1; ; start += pageSize {
		if err := ctx.Err(); err != nil {
			return all, fmt.Errorf("YTPlaylists: %w", err)
		}
		args := buildArgsPage(c.cfg, u, start)
		page, raw, err := c.runAndParsePlaylists(ctx, args)
		if err != nil {
			return all, err
		}
		all = append(all, page...)
		if raw < pageSize {
			break
		}
	}
	return all, nil
}

// PlaylistVideos fetches all videos for a YouTube playlist, paginated.
func (c *Client) PlaylistVideos(ctx context.Context, playlistID string) ([]domain.Video, error) {
	u := "https://www.youtube.com/playlist?list=" + playlistID
	var all []domain.Video
	for start := 1; ; start += pageSize {
		if err := ctx.Err(); err != nil {
			return all, fmt.Errorf("PlaylistVideos: %w", err)
		}
		args := buildArgsPage(c.cfg, u, start)
		page, raw, err := c.runAndParseVideos(ctx, args)
		if err != nil {
			return all, err
		}
		if c.cfg.StripEmojis {
			page = applyStripEmojisVideos(page)
		}
		all = append(all, page...)
		if raw < pageSize {
			break
		}
	}
	return all, nil
}

type ytdlpDetailChapter struct {
	Title     string  `json:"title"`
	StartTime float64 `json:"start_time"`
	EndTime   float64 `json:"end_time"`
}

// ytdlpSBChapter is one entry of yt-dlp's top-level sponsorblock_chapters array,
// present only when --sponsorblock-mark ran. The timecodes are on the original
// (un-cut) timeline, matching the caption cue timings.
type ytdlpSBChapter struct {
	StartTime float64 `json:"start_time"`
	EndTime   float64 `json:"end_time"`
}

type ytdlpDetailEntry struct {
	ID                   string               `json:"id"`
	Title                string               `json:"title"`
	Channel              string               `json:"channel"`
	ChannelID            string               `json:"channel_id"`
	Duration             float64              `json:"duration"`
	ViewCount            int64                `json:"view_count"`
	UploadDate           string               `json:"upload_date"`
	WebpageURL           string               `json:"webpage_url"`
	Description          string               `json:"description"`
	Thumbnail            string               `json:"thumbnail"`
	ChannelFollowerCount int64                `json:"channel_follower_count"`
	Chapters             []ytdlpDetailChapter `json:"chapters"`
	Language             string               `json:"language"`
	SponsorBlockChapters []ytdlpSBChapter     `json:"sponsorblock_chapters"`
}

// VideoDetails fetches detailed info for a single video URL.
func (c *Client) VideoDetails(ctx context.Context, videoURL string) (domain.VideoDetails, error) {
	args := []string{"--dump-json", "--no-warnings", "--quiet"}
	args = append(args, cookieArgs(c.cfg)...)
	args = append(args, videoURL)

	out, err := runYtdlpOutputWithRetry(ctx, c.runner, "detail", args)
	if err != nil {
		return domain.VideoDetails{}, err
	}
	return c.parseVideoDetails(out)
}

// VideoDetailsWithTranscript fetches a video's full metadata AND writes its
// captions/auto-subtitles as .srt sidecar files (under outTemplate) in a single
// yt-dlp invocation. Pairing --dump-json with --no-simulate turns the otherwise
// simulate-only JSON dump into a real run that still performs the subtitle side
// effects, so one network round-trip yields both the frontmatter data (title,
// channel, exact upload_date, chapters, thumbnail URL) and the transcript. langs
// is a --sub-langs value; empty falls back to English. sbCats, when non-empty, is
// a --sponsorblock-mark category list; it makes yt-dlp report the matching
// SponsorBlock segments in sponsorblock_chapters so callers can excise them.
func (c *Client) VideoDetailsWithTranscript(ctx context.Context, videoURL, langs, sbCats, outTemplate string) (domain.VideoDetails, error) {
	args := []string{"--dump-json", "--no-simulate", "--skip-download"}
	args = append(args, subtitleSRTArgs(langs)...)
	args = append(args,
		"--no-playlist", "--no-warnings", "--quiet",
		"-o", outTemplate,
	)
	if sbCats != "" {
		args = append(args, "--sponsorblock-mark", sbCats)
	}
	args = append(args, cookieArgs(c.cfg)...)
	args = append(args, videoURL)

	out, err := runYtdlpOutputWithRetry(ctx, c.runner, "detail", args)
	if err != nil {
		return domain.VideoDetails{}, fmt.Errorf("VideoDetailsWithTranscript: %w", err)
	}
	return c.parseVideoDetails(out)
}

// parseVideoDetails maps a single yt-dlp --dump-json object into VideoDetails,
// shared by the metadata-only and metadata+transcript fetch paths.
func (c *Client) parseVideoDetails(out []byte) (domain.VideoDetails, error) {
	var e ytdlpDetailEntry
	if err := json.Unmarshal(out, &e); err != nil {
		return domain.VideoDetails{}, fmt.Errorf("parse: %w", err)
	}
	u := e.WebpageURL
	if u == "" && e.ID != "" {
		u = "https://www.youtube.com/watch?v=" + e.ID
	}
	title := e.Title
	if c.cfg.StripEmojis {
		title = text.StripEmojis(title)
	}
	chapters := make([]domain.RawChapter, len(e.Chapters))
	for i, ch := range e.Chapters {
		chapters[i] = domain.RawChapter{Title: ch.Title, StartTime: ch.StartTime, EndTime: ch.EndTime}
	}
	sbSegs := make([]domain.SBSegment, len(e.SponsorBlockChapters))
	for i, sb := range e.SponsorBlockChapters {
		sbSegs[i] = domain.SBSegment{Start: sb.StartTime, End: sb.EndTime}
	}
	return domain.VideoDetails{
		Video: domain.Video{
			ID:         e.ID,
			Title:      title,
			Channel:    e.Channel,
			ChannelID:  e.ChannelID,
			Duration:   int(e.Duration),
			ViewCount:  e.ViewCount,
			UploadDate: e.UploadDate,
			URL:        u,
		},
		Description:  e.Description,
		ThumbnailURL: e.Thumbnail,
		Subscribers:  e.ChannelFollowerCount,
		Chapters:     chapters,
		Language:     e.Language,
		SBSegments:   sbSegs,
	}, nil
}
