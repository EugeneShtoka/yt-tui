package overlay

import (
	"encoding/base64"
	"image"
	"image/color"
	"image/png"
	"strings"
	"testing"
)

// solidImage returns a w×h image filled with c.
func solidImage(w, h int, c color.Color) image.Image {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.Set(x, y, c)
		}
	}
	return img
}

func TestEncodeThumbB64_RoundTrips(t *testing.T) {
	t.Parallel()
	img := solidImage(4, 4, color.RGBA{R: 10, G: 20, B: 30, A: 255})

	b64 := encodeThumbB64(img)
	if b64 == "" {
		t.Fatal("encodeThumbB64 returned empty string for a valid image")
	}
	raw, err := base64.StdEncoding.DecodeString(b64)
	if err != nil {
		t.Fatalf("output is not valid base64: %v", err)
	}
	if _, err := png.Decode(strings.NewReader(string(raw))); err != nil {
		t.Errorf("decoded bytes are not a valid PNG: %v", err)
	}
}

func TestKittyImageSeq_ShortPayloadSingleChunk(t *testing.T) {
	t.Parallel()
	seq := kittyImageSeq("QUJD", 3, 10, 20, 8) // "ABC", well under the 4096 chunk size

	for _, want := range []string{
		"\033[3;10H", // cursor move to (row, col)
		"i=42",       // thumbImageID
		"c=20,r=8",   // columns × rows
		"\033_Gm=0;", // final (only) chunk marker
		"\033[u",     // cursor restore
	} {
		if !strings.Contains(seq, want) {
			t.Errorf("sequence missing %q\n got: %q", want, seq)
		}
	}
	if strings.Contains(seq, "\033_Gm=1;") {
		t.Error("short payload must not emit a continuation (m=1) chunk")
	}
}

func TestKittyImageSeq_LongPayloadChunks(t *testing.T) {
	t.Parallel()
	seq := kittyImageSeq(strings.Repeat("A", 5000), 1, 1, 10, 10) // > 4096 → chunked

	if !strings.Contains(seq, "\033_Gm=1;") {
		t.Error("payload over 4096 bytes must emit at least one continuation (m=1) chunk")
	}
	if !strings.Contains(seq, "\033_Gm=0;") {
		t.Error("chunked payload must still end with a final (m=0) chunk")
	}
}

func TestKittyDeleteSeq(t *testing.T) {
	t.Parallel()
	if got, want := kittyDeleteSeq(), "\033_Ga=d,d=i,i=42\033\\"; got != want {
		t.Errorf("kittyDeleteSeq() = %q, want %q", got, want)
	}
}

func TestRenderThumbnailHalfBlock_Dimensions(t *testing.T) {
	t.Parallel()
	img := solidImage(8, 8, color.RGBA{R: 255, G: 0, B: 0, A: 255})

	out := renderThumbnailHalfBlock(img, 4, 3) // targetW=4, targetH=3
	if out == "" {
		t.Fatal("expected non-empty output for a valid image")
	}
	// One newline between each of the targetH rows → targetH-1 newlines.
	if got := strings.Count(out, "\n"); got != 2 {
		t.Errorf("got %d newlines, want 2 (targetH-1) for targetH=3", got)
	}
	if !strings.Contains(out, "▄") {
		t.Error("half-block output must contain the ▄ glyph")
	}
	if !strings.Contains(out, "\x1b[0m") {
		t.Error("each row must reset ANSI attributes")
	}
}

func TestRenderThumbnailHalfBlock_EdgeCases(t *testing.T) {
	t.Parallel()
	img := solidImage(4, 4, color.White)
	tests := []struct {
		name    string
		img     image.Image
		targetW int
		targetH int
	}{
		{"nil image", nil, 4, 4},
		{"zero width target", img, 0, 4},
		{"zero height target", img, 4, 0},
		{"empty-bounds image", image.NewRGBA(image.Rect(0, 0, 0, 0)), 4, 4},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if out := renderThumbnailHalfBlock(tt.img, tt.targetW, tt.targetH); out != "" {
				t.Errorf("want empty string, got %q", out)
			}
		})
	}
}

func TestSampleRegion_SolidColorAveragesToItself(t *testing.T) {
	t.Parallel()
	want := color.RGBA{R: 100, G: 150, B: 200, A: 255}
	img := solidImage(10, 10, want)

	r, g, b := sampleRegion(img, img.Bounds(), 0, 0, 5, 5, 10, 10)
	if r != want.R || g != want.G || b != want.B {
		t.Errorf("sampleRegion of a solid image = (%d,%d,%d), want (%d,%d,%d)", r, g, b, want.R, want.G, want.B)
	}
}
