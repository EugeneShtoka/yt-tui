// Package text holds small, dependency-free string utilities shared across
// layers (persistence, scraping, UI) so none of them has to depend on another
// just for a helper.
package text

import "strings"

// emojiRanges are the inclusive Unicode ranges StripEmojis removes: emoji &
// pictographs (1F000–1FFFF), misc symbols + dingbats (2600–27BF), misc
// symbols-and-arrows (2B00–2BFF), and variation selectors (FE00–FE0F).
var emojiRanges = [...][2]rune{
	{0x1F000, 0x1FFFF},
	{0x2600, 0x27BF},
	{0x2B00, 0x2BFF},
	{0xFE00, 0xFE0F},
}

// isEmojiRune reports whether r is an emoji/pictograph, a variation selector, a
// zero-width joiner (200D), or a combining enclosing keycap (20E3) — the runes
// StripEmojis drops.
func isEmojiRune(r rune) bool {
	if r == 0x200D || r == 0x20E3 {
		return true
	}
	for _, rng := range emojiRanges {
		if r >= rng[0] && r <= rng[1] {
			return true
		}
	}
	return false
}

// StripEmojis removes emoji and related pictographic runes from s and collapses
// runs of whitespace to single spaces. Used to normalize scraped titles/names
// before display or storage.
func StripEmojis(s string) string {
	result := strings.Map(func(r rune) rune {
		if isEmojiRune(r) {
			return -1
		}
		return r
	}, s)
	return strings.Join(strings.Fields(result), " ")
}
