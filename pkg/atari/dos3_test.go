package atari_test

import (
	"bytes"
	"encoding/binary"
	"os"
	"strings"
	"testing"

	"github.com/fredsa/retro-disk-tool/cmd/rdt/pkg/atari"
)

const dos3Image = "../../images/atari/atari800-720x128-dos3.atr"

func openATR(t *testing.T, raw []byte) *atari.Disk {
	t.Helper()
	d, err := atari.Open(bytes.NewReader(raw), int64(len(raw)))
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	return d
}

func TestDOS3(t *testing.T) {
	raw, err := os.ReadFile(dos3Image)
	if err != nil {
		t.Fatal(err)
	}
	d := openATR(t, raw)
	if f := d.Format(); f.Filesystem != "Atari DOS 3" || f.Tracks != 40 || f.Sectors != "18" || f.SectorSize != 128 {
		t.Errorf("Format = %+v", f)
	}
	entries, err := d.List()
	if err != nil || len(entries) != 2 ||
		entries[0].Name != "HELLO" || entries[0].Size != 44 || entries[0].Sectors != 8 || entries[0].Corrupt ||
		entries[1].Name != "HELLO.XEX" || entries[1].Size != 51 || entries[1].Corrupt {
		t.Fatalf("List = %+v, %v", entries, err)
	}

	// HELLO.XEX is a binary load file: segments of (start, end, data) after
	// $FFFF, ending exactly at the end of the file, with a run address.
	xex, err := d.ReadFile("hello.xex")
	if err != nil || !bytes.HasPrefix(xex, []byte{0xFF, 0xFF}) {
		t.Fatalf("ReadFile(hello.xex) = % x, %v", xex, err)
	}
	var segs []uint16
	for p := 2; p < len(xex); {
		start, end := binary.LittleEndian.Uint16(xex[p:]), binary.LittleEndian.Uint16(xex[p+2:])
		segs = append(segs, start)
		p += 4 + int(end-start) + 1
		if p > len(xex) {
			t.Fatalf("segment $%04X-$%04X runs past the end of the file", start, end)
		}
	}
	if len(segs) != 2 || segs[0] != 0x4000 || segs[1] != 0x02E0 {
		t.Errorf("segments start at %04X, want $4000 and $02E0 (run address)", segs)
	}

	// HELLO is a tokenized Atari BASIC program: a 14 byte header of
	// pointers, whose last (STARP) minus the first table (VNTP) is the size
	// of the rest.
	bas, err := d.ReadFile("HELLO")
	if err != nil || len(bas) != 14+int(binary.LittleEndian.Uint16(bas[12:])-binary.LittleEndian.Uint16(bas[2:])) ||
		!bytes.Contains(bas, []byte("HELLO")) {
		t.Errorf("ReadFile(HELLO) = % x, %v", bas, err)
	}

	u, err := d.Usage()
	if err != nil || u.TotalSectors != 720 || u.FreeSectors != 680 || u.InUseSectors != 40 {
		t.Errorf("Usage = %+v, %v", u, err)
	}
	var dump bytes.Buffer
	if err := d.Dump(&dump); err != nil || !strings.Contains(dump.String(), "Blocks: 87 of 8 sectors (2 used, 85 free, 0 bad)") ||
		!strings.Contains(dump.String(), "HELLO.XEX      1 blocks from block  1 (51 bytes)") {
		t.Errorf("Dump:\n%s", dump.String())
	}
}

// dos3Edit returns a copy of the image with changes to the HELLO.XEX
// directory entry and the FAT.
func dos3Edit(t *testing.T, edit func(entry, fat []byte, sector func(int) []byte)) []byte {
	t.Helper()
	raw, err := os.ReadFile(dos3Image)
	if err != nil {
		t.Fatal(err)
	}
	raw = append([]byte{}, raw...)
	sector := func(n int) []byte { return raw[16+(n-1)*128 : 16+n*128] }
	edit(sector(16)[32:48], sector(24), sector)
	return raw
}

func TestDOS3BlockChains(t *testing.T) {
	// Grow HELLO.XEX to two blocks (1, then 5) with 100 bytes in the second
	// (FAT and directory must agree on the number of allocated blocks).
	raw := dos3Edit(t, func(entry, fat []byte, sector func(int) []byte) {
		entry[12] = 2
		binary.LittleEndian.PutUint16(entry[14:], 1024+100)
		fat[1], fat[5] = 5, 0xFD
		copy(sector(25+5*8), bytes.Repeat([]byte{0xAB}, 100))
	})
	d := openATR(t, raw)
	c, err := d.ReadFile("HELLO.XEX")
	if err != nil || len(c) != 1124 || c[0] != 0xFF || c[1024] != 0xAB || c[1123] != 0xAB {
		t.Errorf("two block file: %d bytes, %v", len(c), err)
	}
	if u, _ := d.Usage(); u.FreeSectors != 672 {
		t.Errorf("free sectors = %d, want 672", u.FreeSectors)
	}

	for name, tc := range map[string]struct {
		edit func(entry, fat []byte, sector func(int) []byte)
		want string
	}{
		"chain into a free block": {func(e, fat []byte, _ func(int) []byte) { e[12] = 2; fat[1] = 7; fat[9] = 0xFD }, "reaches a free block"},
		"short chain":             {func(e, fat []byte, _ func(int) []byte) { e[12] = 3; fat[8], fat[9] = 0xFD, 0xFD }, "block chain has 1 blocks, directory says 3"},
		"cyclic chain":            {func(e, fat []byte, _ func(int) []byte) { e[12] = 2; fat[1] = 1; fat[9] = 0xFD }, "invalid block chain at block 1"},
		"length too long":         {func(e, fat []byte, _ func(int) []byte) { binary.LittleEndian.PutUint16(e[14:], 2000) }, "file length 2000 does not fit its 1 blocks"},
	} {
		d := openATR(t, dos3Edit(t, tc.edit))
		entries, _ := d.List()
		if e := entries[1]; !e.Corrupt || !strings.Contains(e.Error, tc.want) {
			t.Errorf("%s: HELLO.XEX = %+v, want error %q", name, e, tc.want)
		}
	}

	// Not DOS 3: a FAT entry past the last block, or a FAT that disagrees
	// with the directory. The disk is still bootable, so it opens as a
	// boot-only disk whose dump says why DOS 3 was not recognized.
	for name, tc := range map[string]struct {
		edit func(entry, fat []byte, sector func(int) []byte)
		why  string
	}{
		"entry past the last block": {func(_, fat []byte, _ func(int) []byte) { fat[100] = 1 }, "FAT entry 100 past the last block is $01"},
		"unlisted allocated block":  {func(_, fat []byte, _ func(int) []byte) { fat[30] = 0xFD }, "FAT allocates 3 blocks but the directory lists 2"},
	} {
		d := openATR(t, dos3Edit(t, tc.edit))
		var dump bytes.Buffer
		d.Dump(&dump)
		if f := d.Format().Filesystem; f != "none (bootable disk)" || !strings.Contains(dump.String(), "not DOS 3 ("+tc.why+")") {
			t.Errorf("%s: filesystem %q, dump:\n%s", name, f, dump.String())
		}
	}
}
