package text

import "testing"

func TestStripEmojis(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"ascii passthrough", "Hello World", "Hello World"},
		{"collapse whitespace", "a   b\tc", "a b c"},
		{"trim edges", "  padded  ", "padded"},
		{"strip pictographic (1F600 range)", "hi 😀 there", "hi there"},
		{"strip misc symbols (2600-27BF)", "sun ☀ shine", "sun shine"},
		{"strip dingbats/symbols (2B00-2BFF)", "star ⭐ here", "star here"},
		{"strip variation selector (FE0F)", "warn ⚠️ now", "warn now"},
		{"strip ZWJ sequence", "family \U0001F468\u200D\U0001F469\u200D\U0001F467 ok", "family ok"},
		{"strip keycap combiner (20E3)", "num 1⃣ done", "num 1 done"},
		{"keep non-emoji unicode", "Тбилиси café", "Тбилиси café"},
		{"emoji-only becomes empty", "😀😀", ""},
		{"empty input", "", ""},
	}
	for _, c := range cases {
		if got := StripEmojis(c.in); got != c.want {
			t.Errorf("%s: StripEmojis(%q) = %q, want %q", c.name, c.in, got, c.want)
		}
	}
}

// isEmojiRune boundary check: every range edge is inclusive, and ordinary text
// runes just outside the ranges are kept.
func TestIsEmojiRune(t *testing.T) {
	emoji := []rune{
		0x1F000, 0x1F600, 0x1FFFF, // pictographs (low/mid/high edge)
		0x2600, 0x27BF, // misc symbols + dingbats edges
		0x2B00, 0x2BFF, // symbols-and-arrows edges
		0xFE00, 0xFE0F, // variation selector edges
		0x200D, 0x20E3, // ZWJ, keycap combiner
	}
	for _, r := range emoji {
		if !isEmojiRune(r) {
			t.Errorf("isEmojiRune(%#U) = false, want true", r)
		}
	}
	keep := []rune{'a', 'Я', 'é', 0x0FFF, 0x25FF, 0x2800, 0x2A00, 0x2C00, 0xFDFF, 0x1EFFF}
	for _, r := range keep {
		if isEmojiRune(r) {
			t.Errorf("isEmojiRune(%#U) = true, want false", r)
		}
	}
}
