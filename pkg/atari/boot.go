package atari

import (
	"encoding/binary"
	"fmt"
	"io"
)

// A bootable disk without a DOS filesystem (typical of commercial games):
// the boot sectors hold a loader that reads the rest of the disk by sector
// number, so there are no files to list.

// bootHeader is the header of sector 1, used by the OS to boot any disk.
type bootHeader struct {
	flags   byte
	sectors int    // number of boot sectors to load
	load    uint16 // load address
	init    uint16 // initialization address
}

func (d *Disk) bootHeader() bootHeader {
	s, _ := d.readSector(1)
	return bootHeader{flags: s[0], sectors: int(s[1]), load: binary.LittleEndian.Uint16(s[2:]), init: binary.LittleEndian.Uint16(s[4:])}
}

// checkBoot reports whether sector 1 holds a plausible boot header: at least
// one boot sector, all on the disk, loaded into RAM above page 6 and below
// the OS ROM.
func (d *Disk) checkBoot() error {
	h := d.bootHeader()
	end := int(h.load) + h.sectors*128
	switch {
	case h.sectors == 0:
		return fmt.Errorf("no boot sectors")
	case h.sectors > d.numSectors:
		return fmt.Errorf("%d boot sectors on a %d sector disk", h.sectors, d.numSectors)
	case h.load < 0x0700 || end > 0xC000:
		return fmt.Errorf("boot sectors load at $%04X-$%04X, outside RAM", h.load, end-1)
	}
	return nil
}

func (h bootHeader) String() string {
	return fmt.Sprintf("boot: %d sectors loaded at $%04X, init $%04X", h.sectors, h.load, h.init)
}

// usedSectors counts sectors that are not all zero.
func (d *Disk) usedSectors() int {
	n := 0
	for s := 1; s <= d.numSectors; s++ {
		sec, err := d.readSector(s)
		if err != nil {
			break
		}
		for _, b := range sec {
			if b != 0 {
				n++
				break
			}
		}
	}
	return n
}

func (d *Disk) bootDump(w io.Writer) {
	h := d.bootHeader()
	fmt.Fprintf(w, "  Boot header: flags $%02X, %d boot sectors, load address $%04X, init address $%04X\n", h.flags, h.sectors, h.load, h.init)
	fmt.Fprintf(w, "  No DOS filesystem: the boot loader reads the disk by sector number (%s)\n", d.notDOS)
	fmt.Fprintf(w, "  Sectors with data: %d of %d\n", d.usedSectors(), d.numSectors)
}
