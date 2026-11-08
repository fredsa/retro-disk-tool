package c64

import (
	"bytes"
	"encoding/binary"
	"strings"
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

// Malformed archives must return errors, not panic.
func TestMalformedArchives(t *testing.T) {
	// A T64 whose only entry points at the end of the file (no data).
	t64 := make([]byte, 64+32)
	copy(t64, "C64 tape image file")
	binary.LittleEndian.PutUint16(t64[32:], 0x0100)
	binary.LittleEndian.PutUint16(t64[34:], 1)
	t64[64] = 1
	binary.LittleEndian.PutUint32(t64[64+8:], uint32(len(t64)))
	if _, err := OpenT64(bytes.NewReader(t64), int64(len(t64))); err == nil || !strings.Contains(err.Error(), "has no data") {
		t.Errorf("OpenT64(entry without data) error = %v", err)
	}

	// A TCRT with a TapcrtFileSys marker but a flash too small for its
	// directory.
	tcrt := make([]byte, 216+64)
	copy(tcrt, "tapecartImage\r\n\x1a")
	tcrt[16] = 1
	binary.LittleEndian.PutUint32(tcrt[212:], 64)
	copy(tcrt[216+2:], "TapcrtFileSys")
	tcrt[216+15+16] = 0xF0 // entry 0 is a separator; entry 1 runs past the flash
	if _, err := OpenTCRT(bytes.NewReader(tcrt), int64(len(tcrt))); err == nil || !strings.Contains(err.Error(), "extends beyond") {
		t.Errorf("OpenTCRT(truncated directory) error = %v", err)
	}
}
