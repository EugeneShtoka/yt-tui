package thumbs

import (
	"bytes"
	"fmt"
	"image"
	"image/jpeg"
	"os"
	"path/filepath"
	"strings"
)

// recropMarker is written into the cache dir once the one-time Recrop migration
// has swept it, so subsequent starts skip the (decode-heavy) rescan. It is not a
// .jpg, so Retain's sweep leaves it in place.
const recropMarker = ".recropped"

// CropLetterbox trims solid black horizontal bars from the top and bottom of a
// thumbnail. YouTube's hqdefault image (the predictable, always-present URL) is
// a 4:3 frame with a 16:9 thumbnail letterboxed inside it, so it arrives with
// black bands above and below the actual image. Detecting and cropping the bars
// (rather than assuming fixed proportions) also leaves genuinely 16:9 sources
// untouched. Trimming is capped at 20% per side so a legitimately dark frame is
// never gutted. Returns img unchanged when it has no SubImage support.
func CropLetterbox(img image.Image) image.Image {
	type subImager interface {
		SubImage(r image.Rectangle) image.Image
	}
	si, ok := img.(subImager)
	if !ok {
		return img
	}
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	if w <= 0 || h <= 0 {
		return img
	}
	maxTrim := h / 5 // never remove more than 20% from either edge
	top := b.Min.Y
	for top-b.Min.Y < maxTrim && isBlackRow(img, b, top) {
		top++
	}
	bottom := b.Max.Y
	for b.Max.Y-bottom < maxTrim && isBlackRow(img, b, bottom-1) {
		bottom--
	}
	if top == b.Min.Y && bottom == b.Max.Y {
		return img // no bars found
	}
	if bottom-top < h/2 {
		return img // suspiciously aggressive crop — leave the image alone
	}
	return si.SubImage(image.Rect(b.Min.X, top, b.Max.X, bottom))
}

// isBlackRow reports whether row y is (near) solid black across its full width.
// Pixels are sampled in steps for speed; a few stray non-black pixels are
// tolerated so JPEG ringing at the bar edge doesn't defeat detection.
func isBlackRow(img image.Image, b image.Rectangle, y int) bool {
	const threshold = 24 // 0-255 luma-ish: below this a channel reads as black
	step := b.Dx() / 32
	if step < 1 {
		step = 1
	}
	nonBlack := 0
	for x := b.Min.X; x < b.Max.X; x += step {
		r, g, bl, _ := img.At(x, y).RGBA()
		if r>>8 > threshold || g>>8 > threshold || bl>>8 > threshold {
			nonBlack++
			if nonBlack > 1 {
				return false
			}
		}
	}
	return true
}

// CropLetterboxJPEG decodes JPEG bytes, crops any letterbox bars, and re-encodes.
// It returns (cropped, true) only when bars were actually trimmed; when the image
// needs no cropping (or can't be decoded) it returns (data, false) so callers can
// skip a needless rewrite and the extra recompression that comes with it.
func CropLetterboxJPEG(data []byte) ([]byte, bool) {
	img, err := jpeg.Decode(bytes.NewReader(data))
	if err != nil {
		return data, false
	}
	cropped := CropLetterbox(img)
	if cropped == img { // identical interface value → nothing was trimmed
		return data, false
	}
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, cropped, &jpeg.Options{Quality: 90}); err != nil {
		return data, false
	}
	return buf.Bytes(), true
}

// Recrop rewrites every cached thumbnail that still carries letterbox bars,
// cropping them in place. It exists to migrate images cached before cropping
// moved to store time; it is idempotent (a clean image is left untouched) and
// guarded by a marker file so the directory is scanned only once per install.
// Returns the number of images rewritten.
func (s *Store) Recrop() (int, error) {
	marker := filepath.Join(s.dir, recropMarker)
	if _, err := os.Stat(marker); err == nil {
		return 0, nil // already swept
	}
	entries, err := os.ReadDir(s.dir)
	if err != nil {
		return 0, fmt.Errorf("thumbs: recrop readdir: %w", err)
	}
	rewrote := 0
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".jpg") {
			continue
		}
		id := strings.TrimSuffix(name, ".jpg")
		data, ok, err := s.Get(id)
		if err != nil || !ok {
			continue
		}
		cropped, changed := CropLetterboxJPEG(data)
		if !changed {
			continue
		}
		if err := s.Put(id, cropped); err != nil {
			return rewrote, err
		}
		rewrote++
	}
	if err := os.WriteFile(marker, []byte("1\n"), 0o600); err != nil {
		return rewrote, fmt.Errorf("thumbs: recrop marker: %w", err)
	}
	return rewrote, nil
}
