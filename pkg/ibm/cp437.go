package ibm

import "strings"

// cp437High maps IBM PC code page 437 bytes 0x80-0xFF to Unicode.
var cp437High = []rune("" +
	"ÇüéâäàåçêëèïîìÄÅ" +
	"ÉæÆôöòûùÿÖÜ¢£¥₧ƒ" +
	"áíóúñÑªº¿⌐¬½¼¡«»" +
	"░▒▓│┤╡╢╖╕╣║╗╝╜╛┐" +
	"└┴┬├─┼╞╟╚╔╩╦╠═╬╧" +
	"╨╤╥╙╘╒╓╫╪┘┌█▄▌▐▀" +
	"αßΓπΣσµτΦΘΩδ∞φε∩" +
	"≡±≥≤⌠⌡÷≈°∙·√ⁿ²■ ")

// decodeCP437 converts code page 437 bytes to a string. Control bytes are
// rendered as "?".
func decodeCP437(b []byte) string {
	var sb strings.Builder
	for _, c := range b {
		switch {
		case c >= 0x80:
			sb.WriteRune(cp437High[c-0x80])
		case c < 0x20 || c == 0x7F:
			sb.WriteByte('?')
		default:
			sb.WriteByte(c)
		}
	}
	return sb.String()
}
