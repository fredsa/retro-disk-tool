package c64

import (
	"testing"

	"github.com/fredsa/retro-disk-tool/cmd/rdt/pkg/util"
)

func TestPetsciiQuotedGlyphs(t *testing.T) {
	// "A" + CLR ($93) + CBM graphic ($F7) + RETURN ($0D) + shifted S ($D3) + £ ($5C)
	got := petsciiQuotedGlyphs([]byte{0x41, 0x93, 0xF7, 0x0D, 0xD3, 0x5C})
	want := []util.Glyph{
		{R: '"'},
		{R: 'A'},
		{R: 0x2665, Style: util.StyleInverse}, // CLR shown as reverse heart in quote mode
		{R: 0x1FB82},
		{R: 'M', Style: util.StyleInverse}, // RETURN shown as reverse M
		{R: 0x2665},
		{R: '£'},
		{R: '"'},
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

func TestPetsciiCornerGlyphs(t *testing.T) {
	// $BA (and its alias $FA) is the right-and-lower corner, the mirror of $6F/$CF
	// (left-and-upper) and $D0 (right-and-upper).
	for _, b := range []byte{0xBA, 0xFA} {
		if r := petsciiPrintable(b); r != 0x1FB7F {
			t.Errorf("petsciiPrintable(%#x) = %U, want U+1FB7F", b, r)
		}
	}
}

