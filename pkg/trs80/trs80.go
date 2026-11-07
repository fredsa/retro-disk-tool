// Package trs80 reads TRS-80 Model III raw sector disk images (IMG) holding
// TRSDOS 1.3 volumes: 18 sectors of 256 bytes per track, single sided.
package trs80

import (
	"encoding/binary"
	"fmt"
	"io"
	"strings"

	"github.com/fredsa/retro-disk-tool/cmd/rdt/pkg/util"
)

const (
	sectorSize      = 256
	sectorsPerTrack = 18
	sectorsPerGran  = 3
	gransPerTrack   = sectorsPerTrack / sectorsPerGran

	entrySize       = 48
	entriesPerSec   = 5
	firstDirSector  = 2 // sectors 0 (GAT) and 1 (HIT) precede the entries
	defaultDirTrack = 17

	attrInvisible = 0x08
	attrInUse     = 0x10
	attrSystem    = 0x40
)

// Entry is a directory entry.
type Entry struct {
	Name       string // NAME/EXT
	Size       uint32
	Attr       byte
	Month      int
	Year       int // 2 digit year
	Protection int // protection level (0-7)
	Extents    []Extent
	Corrupt    bool
	Error      string
}

// Extent is a run of contiguous granules, possibly continuing onto the
// following tracks.
type Extent struct {
	Track     int
	StartGran int
	Grans     int
}

// DiskUsage describes sector usage.
type DiskUsage struct {
	FormattedCapacity   int64
	FormattedDesc       string
	UnformattedCapacity int64
	UnformattedDesc     string
	SectorSize          int
	TotalSectors        int
	InUseSectors        int
	FreeSectors         int
	TotalBytes          int64
	InUseBytes          int64
	FreeBytes           int64
}

// Disk is an opened TRSDOS 1.3 disk image.
type Disk struct {
	data     []byte
	tracks   int
	dirTrack int
}

// Open parses a raw TRS-80 Model III disk image.
func Open(r io.ReaderAt, size int64) (*Disk, error) {
	trackSize := int64(sectorsPerTrack * sectorSize)
	if size == 0 || size%trackSize != 0 || size/trackSize > 96 {
		return nil, fmt.Errorf("not a TRS-80 Model III image: size %d is not a multiple of %d byte tracks", size, trackSize)
	}
	data := make([]byte, size)
	if _, err := r.ReadAt(data, 0); err != nil {
		return nil, fmt.Errorf("failed to read image: %w", err)
	}
	d := &Disk{data: data, tracks: int(size / trackSize), dirTrack: defaultDirTrack}
	// The boot sector starts with $FE followed by the directory track.
	if data[0] == 0xFE && data[1] > 0 && int(data[1]) < d.tracks {
		d.dirTrack = int(data[1])
	}
	if d.dirTrack >= d.tracks {
		return nil, fmt.Errorf("not a TRSDOS 1.3 disk: no directory track")
	}
	if err := d.check(); err != nil {
		return nil, fmt.Errorf("not a TRSDOS 1.3 disk: %w", err)
	}
	return d, nil
}

// check sanity checks the directory entries.
func (d *Disk) check() error {
	if d.gat()[d.dirTrack] == 0 {
		return fmt.Errorf("directory track %d is not allocated in the GAT", d.dirTrack)
	}
	for i, e := range d.rawEntries() {
		for _, c := range e[5:16] {
			if c != ' ' && (c < '0' || c > '9') && (c < 'A' || c > 'Z') {
				return fmt.Errorf("invalid file name in directory entry %d", i)
			}
		}
		if e[5] == ' ' {
			return fmt.Errorf("empty file name in directory entry %d", i)
		}
	}
	return nil
}

func (d *Disk) sector(track, sec int) []byte {
	off := (track*sectorsPerTrack + sec) * sectorSize
	if track < 0 || track >= d.tracks || off+sectorSize > len(d.data) {
		return nil
	}
	return d.data[off : off+sectorSize]
}

// rawEntries returns the in use 48 byte directory entries.
func (d *Disk) rawEntries() [][]byte {
	var out [][]byte
	for s := firstDirSector; s < sectorsPerTrack; s++ {
		sec := d.sector(d.dirTrack, s)
		for i := 0; i < entriesPerSec; i++ {
			e := sec[i*entrySize : (i+1)*entrySize]
			if e[0]&attrInUse != 0 {
				out = append(out, e)
			}
		}
	}
	return out
}

func parseEntry(e []byte) Entry {
	name := strings.TrimRight(string(e[5:13]), " ")
	if ext := strings.TrimRight(string(e[13:16]), " "); ext != "" {
		name += "/" + ext
	}
	ern := int(binary.LittleEndian.Uint16(e[20:22]))
	ent := Entry{
		Name:       name,
		Size:       uint32(ern*sectorSize + int(e[3])),
		Attr:       e[0],
		Month:      int(e[1]),
		Year:       int(e[2]),
		Protection: int(e[0] & 0x07),
	}
	for k := 22; k+1 < entrySize; k += 2 {
		if e[k] == 0xFF {
			break
		}
		ent.Extents = append(ent.Extents, Extent{Track: int(e[k]), StartGran: int(e[k+1] >> 5), Grans: int(e[k+1] & 0x1F)})
	}
	return ent
}

// readExtents returns the data of all sectors allocated to e.
func (d *Disk) readExtents(e Entry) ([]byte, error) {
	var out []byte
	for _, x := range e.Extents {
		for s := x.StartGran * sectorsPerGran; s < (x.StartGran+x.Grans)*sectorsPerGran; s++ {
			sec := d.sector(x.Track+s/sectorsPerTrack, s%sectorsPerTrack)
			if sec == nil {
				return out, fmt.Errorf("extent at track %d extends beyond end of disk", x.Track)
			}
			out = append(out, sec...)
		}
	}
	if len(out) < int(e.Size) {
		return out, fmt.Errorf("file size %d exceeds allocated %d bytes", e.Size, len(out))
	}
	return out[:e.Size], nil
}

// List returns the directory entries.
func (d *Disk) List() ([]Entry, error) {
	var res []Entry
	for _, raw := range d.rawEntries() {
		e := parseEntry(raw)
		if _, err := d.readExtents(e); err != nil {
			e.Corrupt, e.Error = true, err.Error()
		}
		res = append(res, e)
	}
	return res, nil
}

// ReadFile returns the contents of a named file ("BASIC/CMD" or "BASIC.CMD").
func (d *Disk) ReadFile(name string) ([]byte, error) {
	want := strings.ReplaceAll(strings.TrimPrefix(name, "/"), ".", "/")
	for _, raw := range d.rawEntries() {
		if e := parseEntry(raw); strings.EqualFold(e.Name, want) {
			return d.readExtents(e)
		}
	}
	return nil, fmt.Errorf("file %q not found", name)
}

// gat returns the granule allocation table sector.
func (d *Disk) gat() []byte { return d.sector(d.dirTrack, 0) }

// Label returns the disk name from the GAT sector.
func (d *Disk) Label() string { return strings.TrimRight(string(d.gat()[0xD0:0xD8]), " ") }

// Date returns the disk creation date (MM/DD/YY) from the GAT sector.
func (d *Disk) Date() string { return strings.TrimSpace(string(d.gat()[0xD8:0xE0])) }

// Usage computes sector usage from the granule allocation table.
func (d *Disk) Usage() (*DiskUsage, error) {
	gat := d.gat()
	used := 0
	for t := 0; t < d.tracks; t++ {
		for g := 0; g < gransPerTrack; g++ {
			if gat[t]&(1<<g) != 0 {
				used++
			}
		}
	}
	total := d.tracks * sectorsPerTrack
	inUse := used * sectorsPerGran
	totalBytes := int64(len(d.data))
	u := &DiskUsage{
		FormattedCapacity: totalBytes,
		FormattedDesc:     fmt.Sprintf("%d KiB", totalBytes/1024),
		SectorSize:        sectorSize,
		TotalSectors:      total,
		InUseSectors:      inUse,
		FreeSectors:       total - inUse,
		TotalBytes:        totalBytes,
		InUseBytes:        int64(inUse) * sectorSize,
		FreeBytes:         int64(total-inUse) * sectorSize,
	}
	// 5.25" double density: 250 kbit/s at 300 rpm, 6,250 bytes per track.
	if d.tracks == 40 || d.tracks == 80 {
		u.UnformattedCapacity = int64(d.tracks) * 6250
		u.UnformattedDesc = fmt.Sprintf("%d KB", d.tracks*6250/1000)
	}
	return u, nil
}

// Format describes the image format and geometry.
func (d *Disk) Format() *util.FormatInfo {
	return &util.FormatInfo{
		Platform:   "TRS-80 Model III",
		Image:      "IMG (raw sector image)",
		Filesystem: "TRSDOS 1.3",
		Tracks:     d.tracks,
		Heads:      1,
		Sectors:    fmt.Sprintf("%d", sectorsPerTrack),
		SectorSize: sectorSize,
	}
}

func attrString(a byte) string {
	s := []byte("---")
	if a&attrSystem != 0 {
		s[0] = 's'
	}
	if a&attrInvisible != 0 {
		s[1] = 'i'
	}
	s[2] = '0' + a&0x07
	return string(s)
}

// Dump writes a detailed description of the disk.
func (d *Disk) Dump(w io.Writer) error {
	fmt.Fprintf(w, "TRS-80 Model III TRSDOS 1.3 Disk Image Dump\n")
	fmt.Fprintf(w, "  Disk name: %q, Date: %s\n", d.Label(), d.Date())
	fmt.Fprintf(w, "  Tracks: %d, Sectors/track: %d, Sector size: %d, Directory track: %d\n",
		d.tracks, sectorsPerTrack, sectorSize, d.dirTrack)
	if u, err := d.Usage(); err == nil {
		fmt.Fprintf(w, "  In-use: %d sectors, Free: %d sectors, Total: %d sectors\n", u.InUseSectors, u.FreeSectors, u.TotalSectors)
	}
	entries, _ := d.List()
	fmt.Fprintf(w, "  Directory Entries (%d), attributes: s=system i=invisible 0-7=protection level:\n", len(entries))
	for _, e := range entries {
		var ext []string
		for _, x := range e.Extents {
			ext = append(ext, fmt.Sprintf("t%d g%d+%d", x.Track, x.StartGran, x.Grans))
		}
		corrupt := ""
		if e.Corrupt {
			corrupt = fmt.Sprintf(" [CORRUPT: %s]", e.Error)
		}
		fmt.Fprintf(w, "    %s %-12s %6d bytes  %02d/%02d  extents: %s%s\n",
			attrString(e.Attr), e.Name, e.Size, e.Month, e.Year, strings.Join(ext, ", "), corrupt)
	}
	return nil
}
