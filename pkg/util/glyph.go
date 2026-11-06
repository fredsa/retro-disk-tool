package util

// Style is a text rendering style for a Glyph.
type Style int

const (
	StyleNormal Style = iota
	StyleInverse
	StyleFlashing
)

// Glyph is a single Unicode character along with the platform specific
// style it was displayed in (e.g. Apple II inverse / flashing text).
type Glyph struct {
	R     rune
	Style Style
}

// GlyphsString returns the plain text of g, ignoring styles.
func GlyphsString(g []Glyph) string {
	r := make([]rune, len(g))
	for i, x := range g {
		r[i] = x.R
	}
	return string(r)
}

