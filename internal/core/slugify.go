// SPDX-License-Identifier: MIT
package core

import "strings"

// SlugMaxLen is the maximum length of a slug produced by Slugify.
// Memories use this to derive keys from free-form bodies; 50 chars keeps
// keys readable while leaving room for a numeric collision suffix.
const SlugMaxLen = 50

// Slugify converts free-form text into a kebab-case ASCII slug suitable
// for use as a memory key. Returns the empty string when the input has
// no slug-eligible characters.
//
// Rules:
//   - Lowercase ASCII letters and digits are preserved.
//   - Whitespace and punctuation collapse to a single hyphen.
//   - Non-ASCII characters are dropped (no Unicode normalization).
//   - Runs of hyphens collapse to one.
//   - Leading and trailing hyphens are trimmed.
//   - The result is truncated to SlugMaxLen characters, then trimmed of
//     any trailing hyphen the truncation may have exposed.
func Slugify(text string) string {
	var b strings.Builder
	b.Grow(len(text))
	prevHyphen := true // suppresses leading hyphen
	for _, r := range text {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
			prevHyphen = false
		case r >= 'A' && r <= 'Z':
			b.WriteRune(r + ('a' - 'A'))
			prevHyphen = false
		case r < 0x80:
			if !prevHyphen {
				b.WriteByte('-')
				prevHyphen = true
			}
		default:
			// non-ASCII: drop. Do NOT collapse to hyphen — that would let
			// any non-ASCII glyph create a delimiter, which is surprising.
		}
	}
	out := strings.TrimRight(b.String(), "-")
	if len(out) > SlugMaxLen {
		out = strings.TrimRight(out[:SlugMaxLen], "-")
	}
	return out
}
