package overlay

import "testing"

func TestStripNoteChrome(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{
			name: "flat note drops frontmatter, image and redundant header",
			in: "---\ntitle: \"V\"\nchannel: \"C\"\nurl: u\n---\n\n" +
				"![](../thumbnails/x.jpg)\n\n## Transcript\n\nHello world.\n",
			want: "Hello world.\n",
		},
		{
			name: "chaptered note keeps chapter headers",
			in: "---\ntitle: \"V\"\n---\n\n![](../thumbnails/x.jpg)\n\n" +
				"## 0:00 Intro\n\nHi there.\n\n## 2:30 Middle\n\nMore.\n",
			want: "## 0:00 Intro\n\nHi there.\n\n## 2:30 Middle\n\nMore.\n",
		},
		{
			name: "legacy raw transcript (no frontmatter) passes through",
			in:   "Just some raw text.\nSecond line.",
			want: "Just some raw text.\nSecond line.",
		},
		{
			name: "no image ref",
			in:   "---\ntitle: \"V\"\n---\n\n## Transcript\n\nBody.\n",
			want: "Body.\n",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := stripNoteChrome(tc.in); got != tc.want {
				t.Fatalf("stripNoteChrome:\n got %q\nwant %q", got, tc.want)
			}
		})
	}
}
