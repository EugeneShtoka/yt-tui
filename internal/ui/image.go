package ui

import (
	"fmt"
	"image"
	_ "image/jpeg"
	"net/http"
	"strings"

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

// renderThumbnail renders img using Unicode half-block characters (▄) with
// true-color ANSI escape sequences. Returns "" if img is nil.
func renderThumbnail(img image.Image, targetW, targetH int) string {
	if targetW <= 0 || targetH <= 0 || img == nil {
		return ""
	}
	return renderThumbnailHalfBlock(img, targetW, targetH)
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
