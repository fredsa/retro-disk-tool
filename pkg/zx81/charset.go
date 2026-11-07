package zx81

import (
	"strings"

	"github.com/fredsa/retro-disk-tool/cmd/rdt/pkg/util"
)

// charset maps ZX81 character codes $00..$3F to Unicode. Codes $80..$BF are
// the same characters in inverse video.
var charset = [64]rune{
	' ', 0x2598, 0x259D, 0x2580, 0x2596, 0x258C, 0x259E, 0x259B, // 00-07
	0x2592, 0x1FB8F, 0x1FB8E, '"', '£', '$', ':', '?', // 08-0F
	'(', ')', '>', '<', '=', '+', '-', '*', // 10-17
	'/', ';', ',', '.', '0', '1', '2', '3', // 18-1F
	'4', '5', '6', '7', '8', '9', 'A', 'B', // 20-27
	'C', 'D', 'E', 'F', 'G', 'H', 'I', 'J', // 28-2F
	'K', 'L', 'M', 'N', 'O', 'P', 'Q', 'R', // 30-37
	'S', 'T', 'U', 'V', 'W', 'X', 'Y', 'Z', // 38-3F
}

// tokens are the keywords for codes $C0..$FF.
var tokens = [64]string{
	`""`, "AT", "TAB", "?", "CODE", "VAL", "LEN", "SIN", // C0-C7
	"COS", "TAN", "ASN", "ACS", "ATN", "LN", "EXP", "INT", // C8-CF
	"SQR", "SGN", "ABS", "PEEK", "USR", "STR$", "CHR$", "NOT", // D0-D7
	"**", "OR", "AND", "<=", ">=", "<>", "THEN", "TO", // D8-DF
	"STEP", "LPRINT", "LLIST", "STOP", "SLOW", "FAST", "NEW", "SCROLL", // E0-E7
	"CONT", "DIM", "REM", "FOR", "GOTO", "GOSUB", "INPUT", "LOAD", // E8-EF
	"LIST", "LET", "PAUSE", "NEXT", "POKE", "PRINT", "PLOT", "RUN", // F0-F7
	"SAVE", "RAND", "IF", "CLS", "UNPLOT", "CLEAR", "RETURN", "COPY", // F8-FF
}

const (
	codeNewline = 0x76
	codeNumber  = 0x7E // followed by the 5 byte floating point value
)

// glyph maps a ZX81 character code ($00..$3F, $80..$BF) to a Unicode glyph.
// Other codes are shown as '?', as on the ZX81.
func glyph(b byte) util.Glyph {
	switch {
	case b < 0x40:
		return util.Glyph{R: charset[b]}
	case b >= 0x80 && b < 0xC0:
		return util.Glyph{R: charset[b-0x80], Style: util.StyleInverse}
	}
	return util.Glyph{R: '?'}
}

// decodeName decodes a program name, whose last character has bit 7 set.
func decodeName(raw []byte) ([]util.Glyph, string) {
	g := make([]util.Glyph, len(raw))
	for i, b := range raw {
		if i == len(raw)-1 {
			b &= 0x7F
		}
		g[i] = glyph(b)
	}
	return g, util.GlyphsString(g)
}

// keyword returns the text of a token or function code, or "" if b is a
// plain character.
func keyword(b byte) string {
	switch {
	case b >= 0xC0:
		return tokens[b-0xC0]
	case b == 0x40:
		return "RND"
	case b == 0x41:
		return "INKEY$"
	case b == 0x42:
		return "PI"
	}
	return ""
}

func isAlnum(c byte) bool { return c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' }

// listLine renders the body of a BASIC line as LIST shows it: the hidden
// binary form of numbers is skipped, the line ends at the first NEWLINE, and
// keywords are spaced following the ROM rules:
//   - codes $D8 and up starting with a letter get a leading space, unless a
//     keyword's trailing space was just printed (or at the start of a line)
//   - codes $C0 and up ending with a letter, digit or '$' get a trailing space
func listLine(body []byte) string {
	var sb strings.Builder
	suppress := true // a leading space is not needed
	for i := 0; i < len(body); i++ {
		b := body[i]
		if b == codeNewline {
			break
		}
		if b == codeNumber {
			i += 5
			continue
		}
		kw := keyword(b)
		if kw == "" {
			sb.WriteRune(glyph(b).R)
			suppress = false
			continue
		}
		if b >= 0xD8 && isAlnum(kw[0]) && !suppress {
			sb.WriteByte(' ')
		}
		sb.WriteString(kw)
		suppress = false
		if last := kw[len(kw)-1]; b >= 0xC0 && (isAlnum(last) || last == '$') {
			sb.WriteByte(' ')
			suppress = true
		}
	}
	return strings.TrimRight(sb.String(), " ")
}
