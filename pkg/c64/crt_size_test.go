package c64

import (
	"bytes"
	"encoding/binary"
	"testing"
)

func TestCRTSize(t *testing.T) {
	hdr := make([]byte, 0x40)
	copy(hdr, "C64 CARTRIDGE   ")
	binary.BigEndian.PutUint32(hdr[0x10:], 0x40)
	hdr[0x14] = 1
	chip := make([]byte, 16+8192)
	copy(chip, "CHIP")
	binary.BigEndian.PutUint32(chip[4:], uint32(len(chip)))
	binary.BigEndian.PutUint16(chip[12:], 0x8000)
	binary.BigEndian.PutUint16(chip[14:], 8192)
	good := append(append([]byte{}, hdr...), chip...)
	if c, err := OpenCRT("x.crt", bytes.NewReader(good), int64(len(good))); err != nil || len(c.chips) != 1 {
		t.Fatalf("OpenCRT failed: %v", err)
	}
	short := append([]byte{}, good...)
	binary.BigEndian.PutUint32(short[0x40+4:], 16+8191)
	for name, data := range map[string][]byte{
		"trailing byte":  append(append([]byte{}, good...), 0),
		"trailing junk":  append(append([]byte{}, good...), make([]byte, 32)...),
		"truncated chip": good[:len(good)-1],
		"short packet":   short,
	} {
		if _, err := OpenCRT("x.crt", bytes.NewReader(data), int64(len(data))); err == nil {
			t.Errorf("OpenCRT(%s) succeeded, want error", name)
		}
	}
}
