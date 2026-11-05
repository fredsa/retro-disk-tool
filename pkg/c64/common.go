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

// Track sector counts for 1541 disk (tracks 1-35)
var trackSectors = []int{
	// 1-based indexing convenience (index 0 dummy)
	0,
	21, 21, 21, 21, 21, 21, 21, 21, 21, 21, 21, 21, 21, 21, 21, 21, 21, // tracks 1-17: 21 sectors
	19, 19, 19, 19, 19, 19, 19,                                         // tracks 18-24: 19 sectors
	18, 18, 18, 18, 18, 18,                                             // tracks 25-30: 18 sectors
	17, 17, 17, 17, 17,                                                 // tracks 31-35: 17 sectors
}

func getSectorOffset(track, sector int) (int, error) {
	if track < 1 || track > 35 {
		return 0, fmt.Errorf("invalid track: %d", track)
	}
	if sector < 0 || sector >= trackSectors[track] {
		return 0, fmt.Errorf("invalid sector %d for track %d", sector, track)
	}
	offset := 0
	for t := 1; t < track; t++ {
		offset += trackSectors[t] * 256
	}
	offset += sector * 256
	return offset, nil
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
