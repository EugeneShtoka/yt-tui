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
// Detected once via env vars; no TTY query needed.
var kittyCapable = sync.OnceValue(func() bool {
	switch strings.ToLower(os.Getenv("TERM_PROGRAM")) {
	case "kitty", "wezterm", "ghostty":
		return true
	}
	return os.Getenv("KITTY_WINDOW_ID") != ""
})

// renderThumbnail picks the best rendering backend for the current terminal
// and returns a targetH-line string where each line is exactly targetW visible
// columns wide (or a CUF escape for Kitty to preserve image pixels).
func renderThumbnail(img image.Image, targetW, targetH int) string {
	if img == nil || targetW <= 0 || targetH <= 0 {
		return ""
	}
	if kittyCapable() {
		if s := renderThumbnailKitty(img, targetW, targetH); s != "" {
			return s
		}
	}
	return renderThumbnailHalfBlock(img, targetW, targetH)
}

// renderThumbnailKitty encodes img into Kitty Graphics Protocol APC sequences.
//
// Layout contract (required for side-by-side compositing):
//   - Line 0: APC sequences that render the image. The terminal advances the
//     cursor by targetW columns after processing, so text written after the
//     sequence appears in the correct column.
//   - Lines 1..targetH-1: "\x1b[{targetW}C" (CUF — cursor forward N).
//     CUF moves the cursor WITHOUT writing to the cells, preserving the Kitty
//     image pixels that the protocol already placed there.
func renderThumbnailKitty(img image.Image, targetW, targetH int) string {
	// PNG-encode the image.
	var pngBuf bytes.Buffer
	if err := png.Encode(&pngBuf, img); err != nil {
		return ""
	}

	// Base64-encode the entire PNG payload.
	b64 := base64.StdEncoding.EncodeToString(pngBuf.Bytes())

	var sb strings.Builder
	const chunkSize = 4096

	// Delete all existing placements before rendering the new image.
	// BubbleTea buffers the full render into a single write, so this is atomic.
	fmt.Fprintf(&sb, "\x1b_Ga=d,d=A\x1b\\")

	// First APC packet: all placement parameters, m=1 (more data follows), no payload.
	fmt.Fprintf(&sb, "\x1b_Ga=T,f=100,t=d,c=%d,r=%d,m=1;\x1b\\", targetW, targetH)

	// Data chunks with m=1.
	for len(b64) > chunkSize {
		fmt.Fprintf(&sb, "\x1b_Gm=1;%s\x1b\\", b64[:chunkSize])
		b64 = b64[chunkSize:]
	}

	// Final chunk with m=0 (last).
	fmt.Fprintf(&sb, "\x1b_Gm=0;%s\x1b\\", b64)

	// Lines 1..targetH-1: CUF to skip the image cells without overwriting them.
	cuf := fmt.Sprintf("\x1b[%dC", targetW)
	for i := 1; i < targetH; i++ {
		sb.WriteByte('\n')
		sb.WriteString(cuf)
	}

	return sb.String()
}

// renderThumbnailHalfBlock renders img using Unicode half-block characters (▄)
// with true-color ANSI escape sequences.
// Background color = upper pixel row, foreground = lower pixel row.
// Each line is exactly targetW visible columns.
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
			// Average a small region instead of single-pixel nearest-neighbor.
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
