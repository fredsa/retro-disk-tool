package atari_test

import (
	"bytes"
	"encoding/binary"
	"strings"
	"testing"

	"github.com/fredsa/retro-disk-tool/cmd/rdt/pkg/atari"
)

// bootDisk builds a 720 sector disk without a DOS: a boot header (count,
// load address, init address) in sector 1 and data in a few sectors.
func bootDisk(count byte, load, init uint16) []byte {
	sectors := make([]byte, 720*128)
	sectors[1] = count
	binary.LittleEndian.PutUint16(sectors[2:], load)
	binary.LittleEndian.PutUint16(sectors[4:], init)
	copy(sectors[6:], []byte{0xA9, 0xFF, 0x8D, 0x01, 0xD3, 0x4C, 0x00, 0x60}) // LDA #$FF; STA $D301; JMP $6000
	for _, s := range []int{3, 37, 108} {
		sectors[(s-1)*128] = 0x42
	}
	return sectors
}

func atr(sectors []byte) []byte {
	p := len(sectors) / 16
	h := []byte{0x96, 0x02, byte(p), byte(p >> 8), 128, 0, byte(p >> 16), 0, 0, 0, 0, 0, 0, 0, 0, 0}
	return append(h, sectors...)
}

func TestBootDisk(t *testing.T) {
	raw := atr(bootDisk(2, 0x5000, 0x50B1))
	d := openATR(t, raw)
	if f := d.Format(); f.Filesystem != "none (bootable disk)" || f.ContentsNote != "Atari boot: 2 sectors loaded at $5000, init $50B1" || f.Comment != "" || f.Tracks != 40 {
		t.Errorf("Format = %+v", f)
	}
	if entries, err := d.List(); err != nil || len(entries) != 0 {
		t.Errorf("List = %+v, %v; want no files", entries, err)
	}
	if _, err := d.ReadFile("AUTORUN.SYS"); err == nil || !strings.Contains(err.Error(), "no DOS filesystem") {
		t.Errorf("ReadFile error = %v", err)
	}
	if u, err := d.Usage(); err != nil || u.TotalSectors != 720 || u.InUseSectors != 720 || u.FreeSectors != 0 {
		t.Errorf("Usage = %+v, %v", u, err)
	}
	var dump bytes.Buffer
	if err := d.Dump(&dump); err != nil || !strings.Contains(dump.String(), "2 boot sectors, load address $5000, init address $50B1") ||
		!strings.Contains(dump.String(), "Sectors with data: 4 of 720") {
		t.Errorf("Dump:\n%s", dump.String())
	}

	// Raw images: .xfd accepts a boot-only disk; a guessed format (e.g.
	// .img) requires a DOS.
	sectors := bootDisk(2, 0x5000, 0x50B1)
	if _, err := atari.OpenXFD(bytes.NewReader(sectors), int64(len(sectors))); err != nil {
		t.Errorf("OpenXFD(boot disk) failed: %v", err)
	}
	if _, err := atari.OpenXFDDOS(bytes.NewReader(sectors), int64(len(sectors))); err == nil {
		t.Error("OpenXFDDOS(boot disk) succeeded, want error")
	}

	// Implausible boot headers are not bootable disks.
	for name, img := range map[string][]byte{
		"blank":                  atr(make([]byte, 720*128)),
		"loads below page 7":     atr(bootDisk(2, 0x0100, 0x0100)),
		"loads into ROM":         atr(bootDisk(200, 0xB000, 0xB000)),
		"more sectors than disk": atr(bootDisk(255, 0x0700, 0x0700)[:128*200]),
	} {
		if _, err := atari.Open(bytes.NewReader(img), int64(len(img))); err == nil {
			t.Errorf("Open(%s) succeeded, want error", name)
		}
	}
}
