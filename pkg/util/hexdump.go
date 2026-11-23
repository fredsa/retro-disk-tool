package util

import (
	"fmt"
	"io"
)

// HexDump writes a classic 16-byte-per-line hex and ASCII dump of data to w,
// with addresses starting at baseAddr.
func HexDump(w io.Writer, data []byte, baseAddr uint32) {
	for off := 0; off < len(data); off += 16 {
		fmt.Fprintf(w, "  %04X: ", baseAddr+uint32(off))
		var ascii [16]byte
		n := min(16, len(data)-off)
		for i := 0; i < 16; i++ {
			if i == 8 {
				fmt.Fprint(w, " ")
			}
			if i < n {
				b := data[off+i]
				fmt.Fprintf(w, "%02X ", b)
				if b >= 0x20 && b <= 0x7E {
					ascii[i] = b
				} else {
					ascii[i] = '.'
				}
			} else {
				fmt.Fprint(w, "   ")
				ascii[i] = ' '
			}
		}
		fmt.Fprintf(w, " |%s|\n", string(ascii[:n]))
	}
}
