package youtube

import "strings"

func StripEmojis(s string) string {
	result := strings.Map(func(r rune) rune {
		if (r >= 0x1F000 && r <= 0x1FFFF) ||
			(r >= 0x2600 && r <= 0x27BF) ||
			(r >= 0x2B00 && r <= 0x2BFF) ||
			(r >= 0xFE00 && r <= 0xFE0F) ||
			r == 0x200D || r == 0x20E3 {
			return -1
		}
		return r
	}, s)
	return strings.Join(strings.Fields(result), " ")
}
