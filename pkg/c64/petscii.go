package c64

import "github.com/fredsa/retro-disk-tool/cmd/rdt/pkg/util"

// Unicode equivalents of the C64 uppercase/graphics character set.
//
// petsciiShiftedGraphics covers PETSCII $C0..$DF (also $60..$7F and, for
// reverse video control codes, $80..$9F).
var petsciiShiftedGraphics = [32]rune{
	0x2500, 0x2660, 0x1FB72, 0x1FB78, 0x1FB77, 0x1FB76, 0x1FB7A, 0x1FB71, // C0-C7
	0x1FB74, 0x256E, 0x2570, 0x256F, 0x1FB7C, 0x2572, 0x2571, 0x1FB7D, // C8-CF
	0x1FB7E, 0x25CF, 0x1FB7B, 0x2665, 0x1FB70, 0x256D, 0x2573, 0x25CB, // D0-D7
	0x2663, 0x1FB75, 0x2666, 0x253C, 0x1FB8C, 0x2502, 0x03C0, 0x25E5, // D8-DF
}

// petsciiCommodoreGraphics covers PETSCII $A0..$BF (also $E0..$FE).
var petsciiCommodoreGraphics = [32]rune{
	' ', 0x258C, 0x2584, 0x2594, 0x2581, 0x258F, 0x2592, 0x2595, // A0-A7
	0x1FB8F, 0x25E4, 0x1FB87, 0x251C, 0x2597, 0x2514, 0x2510, 0x2582, // A8-AF
	0x250C, 0x2534, 0x252C, 0x2524, 0x258E, 0x258D, 0x1FB88, 0x1FB82, // B0-B7
	0x1FB83, 0x2583, 0x1FB7F, 0x2596, 0x259D, 0x2518, 0x2598, 0x259A, // B8-BF ($BA: right and lower corner)
}

// petsciiGlyph maps a PETSCII byte as it appears in a C64 quoted string
// (quote mode) to a Unicode glyph. In quote mode, control codes do not
// execute but are shown as reverse video characters:
//
//	$00..$1F -> reverse video of screen codes $00..$1F ('@', 'A'..'Z', ...)
//	$80..$9F -> reverse video of the shifted graphics $C0..$DF
func petsciiGlyph(b byte) util.Glyph {
	switch {
	case b < 0x20:
		return util.Glyph{R: petsciiPrintable(b + 0x40), Style: util.StyleInverse}
	case b >= 0x80 && b < 0xA0:
		return util.Glyph{R: petsciiShiftedGraphics[b-0x80], Style: util.StyleInverse}
	default:
		return util.Glyph{R: petsciiPrintable(b), Style: util.StyleNormal}
	}
}

// petsciiPrintable maps a printable PETSCII code to Unicode (uppercase/graphics set).
func petsciiPrintable(b byte) rune {
	switch {
	case b >= 0x20 && b <= 0x5F:
		switch b {
		case 0x5C:
			return '£'
		case 0x5E:
			return '↑'
		case 0x5F:
			return '←'
		}
		return rune(b)
	case b >= 0x60 && b <= 0x7F:
		return petsciiShiftedGraphics[b-0x60]
	case b >= 0xA0 && b <= 0xBF:
		return petsciiCommodoreGraphics[b-0xA0]
	case b >= 0xC0 && b <= 0xDF:
		return petsciiShiftedGraphics[b-0xC0]
	case b >= 0xE0 && b <= 0xFE:
		return petsciiCommodoreGraphics[b-0xE0]
	case b == 0xFF:
		return 0x03C0
	}
	return '?'
}

// petsciiQuotedGlyphs renders a C64 file name the way the C64 shows it in a
// directory listing: enclosed in quotes (quote mode), with control codes
// displayed as reverse video symbols.
func petsciiQuotedGlyphs(raw []byte) []util.Glyph {
	g := make([]util.Glyph, 0, len(raw)+2)
	g = append(g, util.Glyph{R: '"'})
	for _, b := range raw {
		g = append(g, petsciiGlyph(b))
	}
	return append(g, util.Glyph{R: '"'})
}
