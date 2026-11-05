package util

import (
	"fmt"
	"strings"
)

// EscapeNonPrintable converts non-printable characters or control bytes into \xXX escapes.
// Standard printable ASCII (0x20..0x7E) remains unchanged.
func EscapeNonPrintable(b []byte) string {
	var sb strings.Builder
	for _, c := range b {
		if c >= 0x20 && c <= 0x7E {
			sb.WriteByte(c)
		} else {
			sb.WriteString(fmt.Sprintf("\\x%02X", c))
		}
	}
	return sb.String()
}
