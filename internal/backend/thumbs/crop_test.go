package thumbs

import (
	"bytes"
	"image"
	"image/color"
	"image/jpeg"
	"os"
	"path/filepath"
	"testing"
)

// letterboxed builds a 480×360 frame with a 16:9 (480×270) red image centered
// between 45px black bars — the shape YouTube's hqdefault arrives in.
func letterboxed() *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, 480, 360))
	for y := 0; y < 360; y++ {
		c := color.RGBA{0, 0, 0, 255}
		if y >= 45 && y < 315 {
			c = color.RGBA{200, 30, 30, 255}
		}
		for x := 0; x < 480; x++ {
			img.Set(x, y, c)
		}
	}
	return img
}

// TestCropLetterbox verifies black top/bottom bars are trimmed while a bar-free
// image is returned untouched.
func TestCropLetterbox(t *testing.T) {
	cropped := CropLetterbox(letterboxed())
	if b := cropped.Bounds(); b.Min.Y != 45 || b.Dy() != 270 {
		t.Errorf("cropped bounds = %v (Dy=%d), want Min.Y=45, Dy=270", b, b.Dy())
	}

	// A fully-red image has no bars and must come back unchanged.
	solid := image.NewRGBA(image.Rect(0, 0, 480, 270))
	for i := range solid.Pix {
		solid.Pix[i] = 255
	}
	if got := CropLetterbox(solid); got.Bounds() != solid.Bounds() {
		t.Errorf("solid image was cropped: %v", got.Bounds())
	}
}

func jpegBytes(t *testing.T, img image.Image) []byte {
	t.Helper()
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: 90}); err != nil {
		t.Fatalf("encode: %v", err)
	}
	return buf.Bytes()
}

// TestCropLetterboxJPEG confirms the bytes variant reports change only when it
// actually trims, so callers can skip needless rewrites.
func TestCropLetterboxJPEG(t *testing.T) {
	out, changed := CropLetterboxJPEG(jpegBytes(t, letterboxed()))
	if !changed {
		t.Fatal("letterboxed image reported no change")
	}
	img, err := jpeg.Decode(bytes.NewReader(out))
	if err != nil {
		t.Fatalf("decode result: %v", err)
	}
	if h := img.Bounds().Dy(); h < 260 || h > 280 {
		t.Errorf("recropped height %d, want ~270", h)
	}

	// A clean 16:9 image: no trim, and the original bytes returned verbatim.
	clean := image.NewRGBA(image.Rect(0, 0, 480, 270))
	for i := range clean.Pix {
		clean.Pix[i] = 255
	}
	src := jpegBytes(t, clean)
	got, changed := CropLetterboxJPEG(src)
	if changed {
		t.Error("clean image reported a change")
	}
	if !bytes.Equal(got, src) {
		t.Error("clean image bytes were rewritten")
	}
}

// TestPutCrops verifies the store's write path is the crop choke point: bytes
// handed to Put land on disk already trimmed.
func TestPutCrops(t *testing.T) {
	s, err := NewStore(t.TempDir())
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	if err := s.Put("aaaaaaaaaaa", jpegBytes(t, letterboxed())); err != nil {
		t.Fatalf("Put: %v", err)
	}
	data, _, _ := s.Get("aaaaaaaaaaa")
	img, _ := jpeg.Decode(bytes.NewReader(data))
	if h := img.Bounds().Dy(); h > 300 {
		t.Errorf("stored image still %dpx tall — Put did not crop", h)
	}
}

// TestRecrop migrates a legacy letterboxed file (written before Put cropped) in
// place, is idempotent on the second pass (marker short-circuits), and writes a
// marker so later starts skip the scan.
func TestRecrop(t *testing.T) {
	dir := t.TempDir()
	s, err := NewStore(dir)
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	// Write the letterboxed file directly, bypassing Put's crop, to stand in for a
	// thumbnail cached before crop-on-write existed.
	if werr := os.WriteFile(s.Path("aaaaaaaaaaa"), jpegBytes(t, letterboxed()), 0o600); werr != nil {
		t.Fatalf("seed file: %v", werr)
	}

	n, err := s.Recrop()
	if err != nil {
		t.Fatalf("Recrop: %v", err)
	}
	if n != 1 {
		t.Fatalf("Recrop rewrote %d, want 1", n)
	}
	data, _, _ := s.Get("aaaaaaaaaaa")
	img, _ := jpeg.Decode(bytes.NewReader(data))
	if h := img.Bounds().Dy(); h > 300 {
		t.Errorf("stored image still %dpx tall — bars not trimmed", h)
	}
	if _, serr := os.Stat(filepath.Join(dir, recropMarker)); serr != nil {
		t.Errorf("marker not written: %v", serr)
	}

	// Second pass is a no-op thanks to the marker.
	if n2, err2 := s.Recrop(); err2 != nil || n2 != 0 {
		t.Errorf("second Recrop = (%d, %v), want (0, nil)", n2, err2)
	}
}
