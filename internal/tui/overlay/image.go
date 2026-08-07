package overlay

import (
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"image"
	_ "image/jpeg"
	"image/png"
	"os"
	"strings"
	"sync"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/EugeneShtoka/yt-tui/internal/api"
	tuipkg "github.com/EugeneShtoka/yt-tui/internal/tui"
	_ "golang.org/x/image/webp"
)

// kittyRefreshMsg asks the VideoDetail overlay to re-place its Kitty image on
// the next frame (see kittyAfterFrameCmd). It is addressed to the requesting
// panel so the tick reaches it even when another overlay is stacked on top —
// otherwise the placement is dropped on the top overlay and the thumbnail only
// appears once that overlay closes.
type kittyRefreshMsg struct{ tuipkg.OverlayTarget }

// ThumbnailLoadedMsg carries the decoded thumbnail image from a background fetch.
// OverlayTarget routes it back to the VideoDetail instance that requested it, so
// a thumbnail that finishes loading while another overlay is stacked on top still
// reaches its panel instead of the top overlay.
type ThumbnailLoadedMsg struct {
	tuipkg.OverlayTarget
	Img image.Image
}

// loadThumbnailCmd loads a video's thumbnail through the media seam off the
// event loop. The seam decides where the bytes come from (client cache, the
// daemon, or a direct CDN fetch when the daemon's cache is off) and returns
// already-cropped bytes, so the overlay just decodes and renders — it owns no
// network dependency and no CDN URL contract. A miss/error renders no image.
// overlayID addresses the result back to the requesting VideoDetail; it is
// stamped on every return path so even a miss reaches the right panel.
func loadThumbnailCmd(ctx context.Context, media api.MediaProvider, overlayID int64, videoID, url string) tea.Cmd {
	target := tuipkg.OverlayTarget{ID: overlayID}
	return func() tea.Msg {
		if media == nil || videoID == "" {
			return ThumbnailLoadedMsg{OverlayTarget: target}
		}
		data, ok, err := media.GetThumbnail(ctx, videoID, url)
		if err != nil || !ok || len(data) == 0 {
			return ThumbnailLoadedMsg{OverlayTarget: target}
		}
		img, _, derr := image.Decode(bytes.NewReader(data))
		if derr != nil {
			return ThumbnailLoadedMsg{OverlayTarget: target}
		}
		return ThumbnailLoadedMsg{OverlayTarget: target, Img: img}
	}
}

// kittyCapable is true when the running terminal supports the Kitty Graphics Protocol.
var kittyCapable = sync.OnceValue(func() bool {
	switch strings.ToLower(os.Getenv("TERM_PROGRAM")) {
	case "kitty", "wezterm", "ghostty":
		return true
	}
	return os.Getenv("KITTY_WINDOW_ID") != ""
})

const thumbImageID = 42

// encodeThumbB64 PNG-encodes img and returns the base64 string used by
// kittyImageSeq. Called once per thumbnail, not on every frame.
func encodeThumbB64(img image.Image) string {
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return ""
	}
	return base64.StdEncoding.EncodeToString(buf.Bytes())
}

// kittyImageSeq returns the terminal escape sequence to display the pre-encoded
// thumbnail at the given 1-indexed (row, col) cell position using the Kitty
// Graphics Protocol. Append to View() output; BubbleTea writes it after the frame.
func kittyImageSeq(b64 string, row, col, thumbW, thumbH int) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "\033[s\033[%d;%dH", row, col)
	fmt.Fprintf(&sb, "\033_Ga=d,d=i,i=%d\033\\", thumbImageID)
	fmt.Fprintf(&sb, "\033_Ga=T,f=100,t=d,i=%d,c=%d,r=%d,m=1;\033\\", thumbImageID, thumbW, thumbH)
	const chunkSize = 4096
	for len(b64) > chunkSize {
		fmt.Fprintf(&sb, "\033_Gm=1;%s\033\\", b64[:chunkSize])
		b64 = b64[chunkSize:]
	}
	fmt.Fprintf(&sb, "\033_Gm=0;%s\033\\", b64)
	sb.WriteString("\033[u")
	return sb.String()
}

// kittyDeleteSeq emits the escape sequence to remove our thumbnail image placement.
// Returned by Render when the video-detail panel is closed.
func kittyDeleteSeq() string {
	return fmt.Sprintf("\033_Ga=d,d=i,i=%d\033\\", thumbImageID)
}

// kittyCmd returns tea.Raw(kittyImageSeq) when a Kitty image should be placed,
// or nil when conditions are not met.
func (vd VideoDetail) kittyCmd() tea.Cmd {
	// Gate on initialView, not subState: the panel is drawn underneath any modal
	// (links/chapters/transcript are centered boxes composed on top), and the
	// image sits at a fixed top-right position, so it belongs on screen whenever
	// this overlay is the side panel — regardless of which modal is open. Keying
	// on subState made a thumbnail that finished loading while the transcript was
	// up stay blank until the modal closed.
	if !kittyCapable() || vd.thumbB64 == "" || vd.contentW == 0 || vd.initialView != InitialViewPanel {
		return nil
	}
	thumbW, thumbH := vd.thumbDimensions()
	// The panel sits panelGap columns right of the tab content, so its interior
	// begins at contentW + panelGap + 2 (gap + border + padding).
	seq := kittyImageSeq(vd.thumbB64, vd.kittyRow, vd.contentW+panelGap+2, thumbW, thumbH)
	return tea.Raw(seq)
}

// kittyAfterFrameCmd schedules a kittyRefreshMsg after one renderer frame cycle
// (~25 ms) so the Kitty image is placed after the changed frame has been flushed.
// The renderer goroutine flushes p.outputBuf before the frame diff, so placing
// the image immediately would have it overwritten by the re-render; the delay
// ensures viewEquals=true on the tick when the image is placed.
func (vd VideoDetail) kittyAfterFrameCmd() tea.Cmd {
	if !kittyCapable() || vd.thumbB64 == "" || vd.contentW == 0 || vd.initialView != InitialViewPanel {
		return nil
	}
	target := tuipkg.OverlayTarget{ID: vd.ID()}
	return tea.Tick(25*time.Millisecond, func(_ time.Time) tea.Msg {
		return kittyRefreshMsg{OverlayTarget: target}
	})
}

// renderThumbnailHalfBlock renders img using Unicode half-block characters (▄)
// with true-color ANSI sequences. Used as a fallback on non-Kitty terminals.
func renderThumbnailHalfBlock(img image.Image, targetW, targetH int) string {
	if img == nil || targetW <= 0 || targetH <= 0 {
		return ""
	}
	bounds := img.Bounds()
	srcW := bounds.Max.X - bounds.Min.X
	srcH := bounds.Max.Y - bounds.Min.Y
	if srcW == 0 || srcH == 0 {
		return ""
	}
	var sb strings.Builder
	for row := 0; row < targetH; row++ {
		for col := 0; col < targetW; col++ {
			tr, tg, tb := sampleRegion(img, bounds, col, 2*row, targetW, 2*targetH, srcW, srcH)
			br, bg, bb := sampleRegion(img, bounds, col, 2*row+1, targetW, 2*targetH, srcW, srcH)
			fmt.Fprintf(&sb, "\x1b[48;2;%d;%d;%dm\x1b[38;2;%d;%d;%dm▄",
				tr, tg, tb, br, bg, bb)
		}
		sb.WriteString("\x1b[0m")
		if row < targetH-1 {
			sb.WriteByte('\n')
		}
	}
	return sb.String()
}

func sampleRegion(img image.Image, bounds image.Rectangle, dstCol, dstRow, dstW, dstH, srcW, srcH int) (r, g, b uint8) {
	x0 := dstCol * srcW / dstW
	x1 := (dstCol+1)*srcW/dstW + 1
	y0 := dstRow * srcH / dstH
	y1 := (dstRow+1)*srcH/dstH + 1
	if x1 > srcW {
		x1 = srcW
	}
	if y1 > srcH {
		y1 = srcH
	}
	var rSum, gSum, bSum, n uint64
	for y := y0; y < y1; y++ {
		for x := x0; x < x1; x++ {
			rv, gv, bv, _ := img.At(bounds.Min.X+x, bounds.Min.Y+y).RGBA()
			rSum += uint64(rv >> 8)
			gSum += uint64(gv >> 8)
			bSum += uint64(bv >> 8)
			n++
		}
	}
	if n == 0 {
		return 0, 0, 0
	}
	return uint8(rSum / n), uint8(gSum / n), uint8(bSum / n) //nolint:gosec // RGBA()>>8 ≤ 255; sum/n ≤ 255 fits uint8
}
