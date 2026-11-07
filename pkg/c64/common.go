package c64

import (
	"fmt"
	"strings"
)

// DecodeBytes converts C64 byte sequences to string.
// Standard printable ASCII characters (0x20..0x7E) remain as-is.
// Shifted PETSCII uppercase (0xC1..0xDA) map to lowercase ASCII ('a'..'z').
// Shifted space (0xA0) maps to ' '.
// All other bytes (control codes, PETSCII graphics) are escaped as \xXX.
func DecodeBytes(b []byte) string {
	var sb strings.Builder
	for _, c := range b {
		switch {
		case c == 0xA0:
			sb.WriteByte(' ')
		case c >= 0x20 && c <= 0x7E:
			sb.WriteByte(c)
		case c >= 0xC1 && c <= 0xDA:
			sb.WriteByte(c - 0x80)
		default:
			sb.WriteString(fmt.Sprintf("\\x%02X", c))
		}
	}
	return strings.TrimSpace(sb.String())
}

// Common C64 file type strings
var fileTypes = map[byte]string{
	0: "DEL",
	1: "SEQ",
	2: "PRG",
	3: "USR",
	4: "REL",
	5: "CBM",
}

// D64 error byte descriptions (from 683-byte / 768-byte error info block)
var d64ErrorCodes = map[byte]string{
	0x01: "00, OK",
	0x02: "20, READ ERROR (header block not found)",
	0x03: "21, READ ERROR (no sync character)",
	0x04: "22, READ ERROR (data block not present)",
	0x05: "23, READ ERROR (checksum error in data block)",
	0x06: "24, READ ERROR (byte decoding error)",
	0x07: "25, WRITE ERROR (write-verify error)",
	0x08: "26, WRITE PROTECT ON",
	0x09: "27, READ ERROR (checksum error in header)",
	0x0A: "28, WRITE ERROR (long data block)",
	0x0B: "29, DISK ID MISMATCH",
	0x0F: "74, DRIVE NOT READY",
}
