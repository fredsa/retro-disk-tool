package zx81

import (
	"bytes"
	"testing"
)

func TestListLine(t *testing.T) {
	// 10 PRINT AT 1,2;"[inverse A]";PI**2: number markers hide the binary value.
	body := []byte{0xF5, 0xC1, 0x1D, 0x7E, 0x81, 0, 0, 0, 0, 0x1A, 0x1E, 0x7E, 0x82, 0, 0, 0, 0,
		0x19, 0x0B, 0xA6, 0x0B, 0x19, 0x42, 0xD8, 0x1E, 0x76}
	if got, want := listLine(body), `PRINT AT 1,2;"A";PI**2`; got != want {
		t.Errorf("listLine = %q, want %q", got, want)
	}
	for want, body := range map[string][]byte{
		"IF A>=1 THEN GOTO 10":   {0xFA, 0x26, 0xDC, 0x1D, 0xDE, 0xEC, 0x1D, 0x1C, 0x76},
		"LET A=INT (RND*10)":     {0xF1, 0x26, 0x14, 0xCF, 0x10, 0x40, 0x17, 0x1D, 0x1C, 0x11},
		"IF INKEY$ <>\"\" AND X": {0xFA, 0x41, 0x00, 0xDD, 0xC0, 0xDA, 0x3D},
	} {
		if got := listLine(body); got != want {
			t.Errorf("listLine = %q, want %q", got, want)
		}
	}
}

func TestOpenErrors(t *testing.T) {
	for name, data := range map[string][]byte{
		"short.p":    make([]byte, 10),
		"versn.p":    append([]byte{0xFF}, make([]byte, 200)...),
		"zeros.p":    make([]byte, 200),
		"noname.p81": {0x26, 0x27},
	} {
		if _, err := Open(name, bytes.NewReader(data), int64(len(data))); err == nil {
			t.Errorf("Open(%s) succeeded, want error", name)
		}
	}
}
