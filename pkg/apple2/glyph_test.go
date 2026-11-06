package apple2

import (
	"testing"

	"github.com/fredsa/retro-disk-tool/cmd/rdt/pkg/util"
)

func TestDOSNameGlyphs(t *testing.T) {
	// Inverse "HELLO", flashing "AB", normal "C d" + trailing padding.
	raw := []byte{0x08, 0x05, 0x0C, 0x0C, 0x0F, 0x41 + 0x40 - 0x40, 0x42, 0xC3, 0xA0, 0xE4, 0xA0, 0xA0}
	raw[5] = 0x41 // 0x41 & 0x3F = 0x01 -> 'A' flashing
	raw[6] = 0x42 // 'B' flashing
	got := dosNameGlyphs(raw)
	want := []util.Glyph{
		{R: 'H', Style: util.StyleInverse}, {R: 'E', Style: util.StyleInverse},
		{R: 'L', Style: util.StyleInverse}, {R: 'L', Style: util.StyleInverse},
		{R: 'O', Style: util.StyleInverse},
		{R: 'A', Style: util.StyleFlashing}, {R: 'B', Style: util.StyleFlashing},
		{R: 'C', Style: util.StyleNormal}, {R: ' ', Style: util.StyleNormal},
		{R: 'd', Style: util.StyleNormal},
	}
	if len(got) != len(want) {
		t.Fatalf("len = %d, want %d: %+v", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("glyph[%d] = %+v, want %+v", i, got[i], want[i])
		}
	}
}

