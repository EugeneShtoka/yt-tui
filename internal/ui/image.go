package ui

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"image"
	_ "image/jpeg"
	"image/png"
	"net/http"
	"os"
	"strings"
	"sync"

	tea "github.com/charmbracelet/bubbletea"
)

type thumbnailLoadedMsg struct {
	img image.Image
}

func loadThumbnailCmd(url string) tea.Cmd {
	return func() tea.Msg {
		resp, err := http.Get(url) //nolint:gosec
		if err != nil {
			return thumbnailLoadedMsg{}
		}
		defer resp.Body.Close()
		img, _, err := image.Decode(resp.Body)
		if err != nil {
			return thumbnailLoadedMsg{}
		}
		return thumbnailLoadedMsg{img: img}
	}
}

// kittyCapable is true when the terminal supports the Kitty Graphics Protocol.
var kittyCapable = sync.OnceValue(func() bool {
	switch strings.ToLower(os.Getenv("TERM_PROGRAM")) {
	case "kitty", "wezterm", "ghostty":
		return true
	}
	return os.Getenv("KITTY_WINDOW_ID") != ""
})

const thumbImageID = 42

// encodeThumbB64 PNG-encodes img and returns the base64 string for use in
// kittyImageOverlay. Called once when the thumbnail loads, not on every frame.
func encodeThumbB64(img image.Image) string {
	var pngBuf bytes.Buffer
	if err := png.Encode(&pngBuf, img); err != nil {
		return ""
	}
	return base64.StdEncoding.EncodeToString(pngBuf.Bytes())
}

// kittyImageOverlay returns a terminal sequence that:
//  1. Saves the cursor (DECSC)
//  2. Jumps to the absolute 1-indexed (row, col) position
//  3. Deletes any previous placement of our image ID
//  4. Transmits and displays the pre-encoded b64 via the Kitty Graphics Protocol
//  5. Restores the cursor (DECRC)
//
// Appending this to the View() output causes BubbleTea to write it after the
// full frame, placing the image in WezTerm's pixel layer without disturbing
// the character-grid layout.
func kittyImageOverlay(b64 string, row, col, thumbW, thumbH int) string {

	var sb strings.Builder
	// Save cursor, jump to image top-left.
	fmt.Fprintf(&sb, "\033[s\033[%d;%dH", row, col)
	// Delete previous placement of our image.
	fmt.Fprintf(&sb, "\033_Ga=d,d=i,i=%d\033\\", thumbImageID)
	// Transmit + display: PNG format, inline data, cell size c×r.
	fmt.Fprintf(&sb, "\033_Ga=T,f=100,t=d,i=%d,c=%d,r=%d,m=1;\033\\", thumbImageID, thumbW, thumbH)
	const chunkSize = 4096
	for len(b64) > chunkSize {
		fmt.Fprintf(&sb, "\033_Gm=1;%s\033\\", b64[:chunkSize])
		b64 = b64[chunkSize:]
	}
	fmt.Fprintf(&sb, "\033_Gm=0;%s\033\\", b64)
	// Restore cursor to where BubbleTea's renderer left off.
	sb.WriteString("\033[u")
	return sb.String()
}

// kittyDeleteOverlay emits the sequence to remove the thumbnail image.
// Called when the video detail panel is closed.
func kittyDeleteOverlay() string {
	return fmt.Sprintf("\033_Ga=d,d=i,i=%d\033\\", thumbImageID)
}

// renderThumbnail renders img using Unicode half-block characters (▄) with
// true-color ANSI escape sequences. Returns "" if img is nil.
// Used as fallback when the terminal does not support Kitty.
func renderThumbnail(img image.Image, targetW, targetH int) string {
	if targetW <= 0 || targetH <= 0 || img == nil {
		return ""
	}
	return renderThumbnailHalfBlock(img, targetW, targetH)
}

// renderThumbnailHalfBlock renders img using Unicode half-block characters (▄)
// with true-color ANSI escape sequences.
func renderThumbnailHalfBlock(img image.Image, targetW, targetH int) string {
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

// sampleRegion returns the average r,g,b of the source image region that maps
// to pixel cell (dstCol, dstRow) in a (dstW × dstH) grid.
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
	return uint8(rSum / n), uint8(gSum / n), uint8(bSum / n)
}
