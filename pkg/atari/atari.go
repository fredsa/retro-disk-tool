// Package atari reads Atari 8-bit ATR disk images containing Atari DOS 2.x
// (2.0S single density, 2.5 enhanced density, 2.0D double density) volumes.
package atari

import (
	"encoding/binary"
	"fmt"
	"io"
	"strings"

	"github.com/fredsa/retro-disk-tool/cmd/rdt/pkg/util"
)

const (
	atrMagic     = 0x0296
	atrHeaderLen = 16

	vtocSector     = 360
	vtoc2Sector    = 1024
	dirFirstSector = 361
	dirSectors     = 8
	entriesPerSec  = 8
	entrySize      = 16
)

// Entry is a directory entry.
type Entry struct {
	Name    string
	Size    uint32
	Sectors int
	Locked  bool
	Corrupt bool
	Error   string
}

// DiskUsage describes sector usage.
type DiskUsage struct {
	FormattedCapacity int64
	SectorSize        int
	TotalSectors      int
	InUseSectors      int
	FreeSectors       int
	TotalBytes        int64
	InUseBytes        int64
	FreeBytes         int64
}

// Disk is an opened ATR image.
type Disk struct {
	data       []byte // sector data (ATR header removed)
	secSize    int
	numSectors int
}

type rawEntry struct {
	flag    byte
	count   int
	start   int
	name    string
	index   int
	display string
}

// Open parses an ATR image.
func Open(r io.ReaderAt, size int64) (*Disk, error) {
	if size < atrHeaderLen {
		return nil, fmt.Errorf("ATR file too small: %d bytes", size)
	}
	raw := make([]byte, size)
	if _, err := r.ReadAt(raw, 0); err != nil {
		return nil, fmt.Errorf("failed to read ATR file: %w", err)
	}
	if binary.LittleEndian.Uint16(raw[0:2]) != atrMagic {
		return nil, fmt.Errorf("invalid ATR signature: 0x%04X", binary.LittleEndian.Uint16(raw[0:2]))
	}
	paragraphs := int(binary.LittleEndian.Uint16(raw[2:4])) | int(raw[6])<<16
	secSize := int(binary.LittleEndian.Uint16(raw[4:6]))
	if secSize != 128 && secSize != 256 {
		return nil, fmt.Errorf("unsupported ATR sector size: %d", secSize)
	}
	imageSize := paragraphs * 16
	data := raw[atrHeaderLen:]
	if imageSize > len(data) {
		return nil, fmt.Errorf("truncated ATR image: header says %d bytes, file has %d", imageSize, len(data))
	}
	data = data[:imageSize]

	n := len(data) / 128
	if secSize == 256 {
		if len(data) < 3*128 {
			return nil, fmt.Errorf("ATR image too small")
		}
		n = 3 + (len(data)-3*128)/256
	}
	d := &Disk{data: data, secSize: secSize, numSectors: n}

	vtoc, err := d.readSector(vtocSector)
	if err != nil {
		return nil, fmt.Errorf("not an Atari DOS 2.x disk: %w", err)
	}
	if vtoc[0] != 2 {
		return nil, fmt.Errorf("unsupported Atari DOS type (VTOC code %d); only Atari DOS 2.x is supported", vtoc[0])
	}
	return d, nil
}

func sectorLen(n, secSize int) int {
	if n <= 3 {
		return 128
	}
	return secSize
}

func (d *Disk) readSector(n int) ([]byte, error) {
	if n < 1 || n > d.numSectors {
		return nil, fmt.Errorf("sector %d out of range (1..%d)", n, d.numSectors)
	}
	off := (n - 1) * 128
	if n > 3 {
		off = 3*128 + (n-4)*d.secSize
	}
	l := sectorLen(n, d.secSize)
	if off+l > len(d.data) {
		return nil, fmt.Errorf("sector %d beyond end of image", n)
	}
	return d.data[off : off+l], nil
}

func (d *Disk) isEnhanced() bool { return d.secSize == 128 && d.numSectors == 1040 }

func (d *Disk) dirEntries() []rawEntry {
	var out []rawEntry
	for s := 0; s < dirSectors; s++ {
		sec, err := d.readSector(dirFirstSector + s)
		if err != nil {
			return out
		}
		for i := 0; i < entriesPerSec; i++ {
			e := sec[i*entrySize : (i+1)*entrySize]
			if e[0] == 0 {
				return out // end of directory
			}
			if e[0]&0x80 != 0 || e[0]&0x40 == 0 {
				continue // deleted or not in use
			}
			base := strings.TrimRight(string(e[5:13]), " ")
			ext := strings.TrimRight(string(e[13:16]), " ")
			name := base
			if ext != "" {
				name += "." + ext
			}
			out = append(out, rawEntry{
				flag:  e[0],
				count: int(binary.LittleEndian.Uint16(e[1:3])),
				start: int(binary.LittleEndian.Uint16(e[3:5])),
				name:  name,
				index: s*entriesPerSec + i,
			})
		}
	}
	return out
}

// readChain follows a file's sector chain.
func (d *Disk) readChain(start int) ([]byte, error) {
	var out []byte
	visited := map[int]bool{}
	s := start
	for s != 0 {
		if visited[s] {
			return out, fmt.Errorf("cyclic sector chain at sector %d", s)
		}
		visited[s] = true
		sec, err := d.readSector(s)
		if err != nil {
			return out, err
		}
		l := len(sec)
		next := int(sec[l-3]&0x03)<<8 | int(sec[l-2])
		cnt := int(sec[l-1])
		if cnt > l-3 {
			return out, fmt.Errorf("invalid byte count %d in sector %d", cnt, s)
		}
		out = append(out, sec[:cnt]...)
		s = next
	}
	return out, nil
}

// List returns the directory entries.
func (d *Disk) List() ([]Entry, error) {
	var res []Entry
	for _, re := range d.dirEntries() {
		content, err := d.readChain(re.start)
		e := Entry{
			Name:    re.name,
			Size:    uint32(len(content)),
			Sectors: re.count,
			Locked:  re.flag&0x20 != 0,
		}
		if err != nil {
			e.Corrupt = true
			e.Error = err.Error()
		}
		res = append(res, e)
	}
	return res, nil
}

// ReadFile returns the contents of a named file.
func (d *Disk) ReadFile(name string) ([]byte, error) {
	want := strings.ToUpper(strings.TrimPrefix(name, "/"))
	for _, re := range d.dirEntries() {
		if strings.ToUpper(re.name) == want {
			return d.readChain(re.start)
		}
	}
	return nil, fmt.Errorf("file %q not found", name)
}

func (d *Disk) freeSectors() (int, error) {
	vtoc, err := d.readSector(vtocSector)
	if err != nil {
		return 0, err
	}
	free := 0
	isFree := func(bm []byte, base, idx int) bool {
		return bm[base+idx/8]&(0x80>>(idx%8)) != 0
	}
	// VTOC bitmap: byte 10, bit 7 is sector 0.
	for s := 1; s < 720 && s <= d.numSectors; s++ {
		if isFree(vtoc, 10, s) {
			free++
		}
	}
	if d.isEnhanced() {
		v2, err := d.readSector(vtoc2Sector)
		if err != nil {
			return 0, err
		}
		for s := 720; s < 1024; s++ {
			if isFree(v2, 84, s-720) {
				free++
			}
		}
	}
	return free, nil
}

// Usage computes sector usage from the VTOC bitmap(s).
func (d *Disk) Usage() (*DiskUsage, error) {
	free, err := d.freeSectors()
	if err != nil {
		return nil, err
	}
	total := d.numSectors
	totalBytes := int64(len(d.data))
	freeBytes := int64(free) * int64(d.secSize)
	return &DiskUsage{
		FormattedCapacity: totalBytes,
		SectorSize:        d.secSize,
		TotalSectors:      total,
		InUseSectors:      total - free,
		FreeSectors:       free,
		TotalBytes:        totalBytes,
		InUseBytes:        totalBytes - freeBytes,
		FreeBytes:         freeBytes,
	}, nil
}

// FormattedDesc returns a short capacity description such as "90 KiB".
func (d *Disk) FormattedDesc() string {
	return fmt.Sprintf("%d KiB", (len(d.data)+512)/1024)
}

// Format describes the ATR format and geometry.
func (d *Disk) Format() *util.FormatInfo {
	fsName := "Atari DOS 2.0S"
	sectors := "18"
	switch {
	case d.isEnhanced():
		fsName, sectors = "Atari DOS 2.5", "26"
	case d.secSize == 256:
		fsName = "Atari DOS 2.0D"
	}
	return &util.FormatInfo{
		Platform:   "Atari 8-bit",
		Image:      fmt.Sprintf("ATR (disk image with 16 byte header, %d byte sectors)", d.secSize),
		Filesystem: fsName,
		Tracks:     40,
		Heads:      1,
		Sectors:    sectors,
		SectorSize: d.secSize,
	}
}

// Dump writes a detailed description of the disk.
func (d *Disk) Dump(w io.Writer) error {
	f := d.Format()
	fmt.Fprintf(w, "Atari ATR Disk Image Dump\n")
	fmt.Fprintf(w, "  Filesystem: %s\n", f.Filesystem)
	fmt.Fprintf(w, "  Sector size: %d bytes, Sectors: %d (%d bytes)\n", d.secSize, d.numSectors, len(d.data))
	if vtoc, err := d.readSector(vtocSector); err == nil {
		fmt.Fprintf(w, "  VTOC: DOS code %d, total data sectors %d, free sectors %d\n",
			vtoc[0], binary.LittleEndian.Uint16(vtoc[1:3]), binary.LittleEndian.Uint16(vtoc[3:5]))
	}
	if u, err := d.Usage(); err == nil {
		fmt.Fprintf(w, "  In-use: %d sectors, Free: %d sectors, Total: %d sectors\n", u.InUseSectors, u.FreeSectors, u.TotalSectors)
	}
	entries, _ := d.List()
	fmt.Fprintf(w, "  Directory Entries (%d):\n", len(entries))
	for _, e := range entries {
		lock := " "
		if e.Locked {
			lock = "*"
		}
		corrupt := ""
		if e.Corrupt {
			corrupt = fmt.Sprintf(" [CORRUPT: %s]", e.Error)
		}
		fmt.Fprintf(w, "   %s%-12s %4d sectors (%d bytes)%s\n", lock, e.Name, e.Sectors, e.Size, corrupt)
	}
	return nil
}

