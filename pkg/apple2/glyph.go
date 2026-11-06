package apple2

import "github.com/fredsa/retro-disk-tool/cmd/rdt/pkg/util"

// appleScreenGlyph maps an Apple II screen/file-name byte to a Unicode glyph
// plus style:
//
//	0x00..0x3F inverse  (0x00..0x1F -> '@'..'_', 0x20..0x3F -> ' '..'?')
//	0x40..0x7F flashing (same character mapping as inverse)
//	0x80..0xFF normal   (high-ASCII, b & 0x7F)
func appleScreenGlyph(b byte) util.Glyph {
	switch {
	case b >= 0x80:
		return util.Glyph{R: asciiGlyphRune(b & 0x7F), Style: util.StyleNormal}
	case b >= 0x40:
		return util.Glyph{R: inverseRune(b & 0x3F), Style: util.StyleFlashing}
	default:
		return util.Glyph{R: inverseRune(b & 0x3F), Style: util.StyleInverse}
	}
}

func inverseRune(c byte) rune {
	if c < 0x20 {
		return rune(c) + 0x40
	}
	return rune(c)
}

// asciiGlyphRune returns printable ASCII as-is and control characters as
// Unicode control pictures (U+2400..).
func asciiGlyphRune(c byte) rune {
	switch {
	case c < 0x20:
		return 0x2400 + rune(c)
	case c == 0x7F:
		return 0x2421
	}
	return rune(c)
}

// dosNameGlyphs converts a 30-byte Apple DOS 3.x file name to glyphs,
// trimming trailing normal spaces.
func dosNameGlyphs(b []byte) []util.Glyph {
	g := make([]util.Glyph, len(b))
	for i, c := range b {
		g[i] = appleScreenGlyph(c)
	}
	for len(g) > 0 && g[len(g)-1].Style == util.StyleNormal && g[len(g)-1].R == ' ' {
		g = g[:len(g)-1]
	}
	return g
}

// proDOSNameGlyphs converts a ProDOS file name (7-bit ASCII) to glyphs.
func proDOSNameGlyphs(b []byte) []util.Glyph {
	g := make([]util.Glyph, len(b))
	for i, c := range b {
		g[i] = util.Glyph{R: asciiGlyphRune(c & 0x7F), Style: util.StyleNormal}
	}
	return g
}

