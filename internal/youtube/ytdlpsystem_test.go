package youtube

import "testing"

// TestColonFieldParsers checks the real output shapes of every supported package
// manager: a wrong parser here silently disables the comparison it feeds.
func TestColonFieldParsers(t *testing.T) {
	tests := []struct {
		name  string
		out   string
		parse func(string) []string
		want  string // newest version the parser should surface
	}{
		{
			name: "pacman -Si",
			out: "Repository      : extra\nName            : yt-dlp\nVersion         : 2026.08.19-1\n" +
				"Description     : A youtube-dl fork\n",
			parse: colonField("Version"),
			want:  "2026.08.19-1",
		},
		{
			name:  "apt-cache policy",
			out:   "yt-dlp:\n  Installed: 2023.03.04-1\n  Candidate: 2026.08.19-1\n  Version table:\n",
			parse: colonField("Candidate"),
			want:  "2026.08.19-1",
		},
		{
			name: "dnf info picks the newest of installed and available",
			out: "Installed Packages\nName         : yt-dlp\nVersion      : 2026.5.1\nRelease      : 1.fc42\n\n" +
				"Available Packages\nName         : yt-dlp\nVersion      : 2026.8.19\nRelease      : 1.fc42\n",
			parse: colonField("Version"),
			want:  "2026.8.19",
		},
		{
			name:  "zypper info",
			out:   "Information for package yt-dlp:\nRepository     : Main\nName           : yt-dlp\nVersion        : 2026.8.19-1.1\n",
			parse: colonField("Version"),
			want:  "2026.8.19-1.1",
		},
		{
			name:  "brew info",
			out:   "==> yt-dlp: stable 2026.08.19 (bottled), HEAD\nDownload YouTube videos\n",
			parse: brewStable,
			want:  "2026.08.19",
		},
	}
	for _, tt := range tests {
		got, ok := newestVersion(tt.parse(tt.out))
		if !ok {
			t.Errorf("%s: no version parsed from:\n%s", tt.name, tt.out)
			continue
		}
		if got.Raw != tt.want {
			t.Errorf("%s: got %q, want %q", tt.name, got.Raw, tt.want)
		}
	}
}

// TestParsersIgnoreUnrelatedOutput: a package manager that does not know yt-dlp
// prints an error or nothing, which must yield no version rather than a guess.
func TestParsersIgnoreUnrelatedOutput(t *testing.T) {
	for _, out := range []string{"", "error: package 'yt-dlp' was not found\n", "N: Unable to locate package yt-dlp\n"} {
		if _, ok := newestVersion(colonField("Version")(out)); ok {
			t.Errorf("parsed a version out of %q", out)
		}
		if _, ok := newestVersion(brewStable(out)); ok {
			t.Errorf("brew parser found a version in %q", out)
		}
	}
}

// TestNewestVersionSkipsUnparseable: managers can list versions we cannot read
// (a git snapshot, say) alongside ones we can; the readable maximum still wins.
func TestNewestVersionSkipsUnparseable(t *testing.T) {
	got, ok := newestVersion([]string{"r1234.abcdef", "2026.05.01", "2026.08.19", "nightly"})
	if !ok || got.Raw != "2026.08.19" {
		t.Errorf("newestVersion = %q (ok=%v), want 2026.08.19", got.Raw, ok)
	}
}
