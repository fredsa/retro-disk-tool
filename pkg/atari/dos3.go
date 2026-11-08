package atari

import (
	"encoding/binary"
	"fmt"
	"io"
	"strings"
)

// Atari DOS 3 (for the 1050 drive) stores files in 1 KiB blocks of eight
// 128 byte sectors, chained through a one sector file allocation table.
//
//	sectors 1-15   boot
//	sectors 16-23  directory: 16 byte entries (entry 0 is a header)
//	sector 24      FAT: one byte per block (next block, or a marker below)
//	sector 25+     data blocks (87 on a 720 sector disk, 127 on 1040)
const (
	dos3DirFirst   = 16
	dos3DirSectors = 8
	dos3FATSector  = 24
	dos3DataFirst  = 25
	dos3BlockSecs  = 8
	dos3BlockSize  = dos3BlockSecs * 128

	dos3EndOfFile = 0xFD // last block of a file
	dos3Free      = 0xFE
	dos3Bad       = 0xFF

	dos3InUse = 0x80 // directory entry status
)

type dos3Entry struct {
	name   string
	status byte
	blocks int
	start  int
	length int
}

func (d *Disk) dos3Blocks() int { return (d.numSectors - dos3FATSector) / dos3BlockSecs }

func (d *Disk) dos3FAT() []byte {
	fat, _ := d.readSector(dos3FATSector)
	return fat
}

// checkDOS3 reports whether the disk holds a DOS 3 filesystem: a single or
// enhanced density layout, a FAT whose entries are all block numbers or
// markers (and zero past the last block), and valid directory entries.
func (d *Disk) checkDOS3() error {
	if d.secSize != 128 || (d.numSectors != 720 && d.numSectors != 1040) {
		return fmt.Errorf("DOS 3 needs a 720 or 1040 sector disk with 128 byte sectors")
	}
	n := d.dos3Blocks()
	for b, v := range d.dos3FAT() {
		switch {
		case b >= n && v != 0:
			return fmt.Errorf("FAT entry %d past the last block is $%02X", b, v)
		case b < n && int(v) >= n && v != dos3EndOfFile && v != dos3Free && v != dos3Bad:
			return fmt.Errorf("invalid FAT entry %d: $%02X", b, v)
		}
	}
	// The FAT's allocated blocks must be exactly the directory's files'
	// blocks (this also rejects blank disks, whose zero FAT entries look
	// like chains).
	allocated, listed := 0, 0
	for _, v := range d.dos3FAT()[:n] {
		if v != dos3Free && v != dos3Bad {
			allocated++
		}
	}
	for _, e := range d.dos3Entries() {
		if e.name == "" || strings.Trim(e.name, ".ABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789") != "" {
			return fmt.Errorf("invalid file name %q", e.name)
		}
		if e.start >= n {
			return fmt.Errorf("file %s starts at block %d of %d", e.name, e.start, n)
		}
		listed += e.blocks
	}
	if allocated != listed {
		return fmt.Errorf("FAT allocates %d blocks but the directory lists %d", allocated, listed)
	}
	return nil
}

// dos3Entries returns the in use directory entries.
func (d *Disk) dos3Entries() []dos3Entry {
	var out []dos3Entry
	for s := 0; s < dos3DirSectors; s++ {
		sec, err := d.readSector(dos3DirFirst + s)
		if err != nil {
			break
		}
		for i := 0; i < 8; i++ {
			if s == 0 && i == 0 {
				continue // header entry
			}
			e := sec[i*16 : (i+1)*16]
			if e[0]&dos3InUse == 0 {
				continue
			}
			name := strings.TrimRight(string(e[1:9]), " ")
			if ext := strings.TrimRight(string(e[9:12]), " "); ext != "" {
				name += "." + ext
			}
			out = append(out, dos3Entry{
				name:   name,
				status: e[0],
				blocks: int(e[12]),
				start:  int(e[13]),
				length: int(binary.LittleEndian.Uint16(e[14:16])),
			})
		}
	}
	return out
}

// dos3Read returns a file's data, following its block chain through the
// FAT and trimming it to the directory length.
func (d *Disk) dos3Read(e dos3Entry) ([]byte, error) {
	fat, n := d.dos3FAT(), d.dos3Blocks()
	var data []byte
	visited := map[int]bool{}
	for b, count := e.start, 0; ; count++ {
		if b >= n || visited[b] {
			return data, fmt.Errorf("invalid block chain at block %d", b)
		}
		visited[b] = true
		for s := 0; s < dos3BlockSecs; s++ {
			sec, err := d.readSector(dos3DataFirst + b*dos3BlockSecs + s)
			if err != nil {
				return data, err
			}
			data = append(data, sec...)
		}
		next := fat[b]
		if next == dos3EndOfFile {
			if count+1 != e.blocks {
				return data, fmt.Errorf("block chain has %d blocks, directory says %d", count+1, e.blocks)
			}
			break
		}
		if next == dos3Free || next == dos3Bad {
			return data, fmt.Errorf("block chain reaches a %s block after block %d", map[byte]string{dos3Free: "free", dos3Bad: "bad"}[next], b)
		}
		b = int(next)
	}
	// The directory keeps the length in 16 bits: an empty file may still own
	// a block, and a file over 64 KiB (possible on a 1040 sector disk) keeps
	// only the low 16 bits, so its length is the one ending in the last block.
	if e.length == 0 {
		return data[:0], nil
	}
	for l := e.length; l <= len(data); l += 0x10000 {
		if l > len(data)-dos3BlockSize {
			return data[:l], nil
		}
	}
	return data, fmt.Errorf("file length %d does not fit its %d blocks", e.length, e.blocks)
}

func (d *Disk) dos3List() []Entry {
	var res []Entry
	for _, e := range d.dos3Entries() {
		data, err := d.dos3Read(e)
		ent := Entry{Name: e.name, Size: uint32(len(data)), Sectors: e.blocks * dos3BlockSecs}
		if err != nil {
			ent.Corrupt, ent.Error = true, err.Error()
		}
		res = append(res, ent)
	}
	return res
}

func (d *Disk) dos3ReadFile(name string) ([]byte, error) {
	want := strings.ToUpper(strings.TrimPrefix(name, "/"))
	for _, e := range d.dos3Entries() {
		if e.name == want {
			return d.dos3Read(e)
		}
	}
	return nil, fmt.Errorf("file %q not found", name)
}

func (d *Disk) dos3FreeSectors() int {
	free := 0
	for _, v := range d.dos3FAT()[:d.dos3Blocks()] {
		if v == dos3Free {
			free += dos3BlockSecs
		}
	}
	return free
}

func (d *Disk) dos3Dump(w io.Writer) {
	fat := d.dos3FAT()[:d.dos3Blocks()]
	counts := map[string]int{}
	for _, v := range fat {
		switch v {
		case dos3Free:
			counts["free"]++
		case dos3Bad:
			counts["bad"]++
		default:
			counts["used"]++
		}
	}
	fmt.Fprintf(w, "  Blocks: %d blocks of %d sectors (%d used, %d free, %d bad), data from sector %d\n",
		len(fat), dos3BlockSecs, counts["used"], counts["free"], counts["bad"], dos3DataFirst)
	entries := d.dos3Entries()
	fmt.Fprintf(w, "  Directory Entries (%d):\n", len(entries))
	for _, e := range entries {
		data, err := d.dos3Read(e)
		corrupt := ""
		if err != nil {
			corrupt = fmt.Sprintf(" [CORRUPT: %s]", err)
		}
		fmt.Fprintf(w, "    %-12s %3d blocks from block %2d (%d bytes) status $%02X%s\n",
			e.name, e.blocks, e.start, len(data), e.status, corrupt)
	}
}
