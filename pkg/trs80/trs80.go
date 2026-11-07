// Package trs80 reads TRS-80 disk images holding TRSDOS 1.3 (Model III) or
// LDOS family (LDOS 5, TRSDOS 6 and LS-DOS 6 for the Model 4) volumes.
//
// Raw sector images (IMG) are 18 sectors of 256 bytes per track, single
// sided; other geometries come from containers such as IMD.
package trs80

import (
	"encoding/binary"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/fredsa/retro-disk-tool/cmd/rdt/pkg/util"
)

const (
	sectorSize      = 256
	sectorsPerTrack = 18 // raw IMG images
	firstDirSector  = 2  // sectors 0 (GAT) and 1 (HIT) precede the entries
	defaultDirTrack = 17

	attrFXDE      = 0x80 // LDOS extended directory entry
	attrSystem    = 0x40
	attrInUse     = 0x10
	attrInvisible = 0x08
)

type dosKind int

const (
	trsdos13 dosKind = iota // 48 byte entries, 3 sector granules
	ldos                    // 32 byte entries, geometry from the GAT
)

// Entry is a directory entry.
type Entry struct {
	Name       string // NAME/EXT
	Size       uint32
	Attr       byte
	Month      int
	Day        int // 0 if not recorded (TRSDOS 1.3)
	Year       int // 2 digit year
	ModTime    *time.Time
	Protection int // protection level (0-7)
	Extents    []Extent
	Corrupt    bool
	Error      string
}

// Extent is a run of contiguous granules, possibly continuing onto the
// following cylinders.
type Extent struct {
	Track     int // cylinder
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

// Disk is an opened TRS-80 disk image.
type Disk struct {
	data        []byte
	tracks      int // cylinders
	heads       int
	spt         int // sectors per track
	dirTrack    int
	dos         dosKind
	container   string
	secPerGran  int
	gransPerCyl int
}

// Open parses a raw TRS-80 disk image (18 sectors of 256 bytes per track).
func Open(r io.ReaderAt, size int64) (*Disk, error) {
	trackSize := int64(sectorsPerTrack * sectorSize)
	if size == 0 || size%trackSize != 0 {
		return nil, fmt.Errorf("not a TRS-80 image: size %d is not a multiple of %d byte tracks", size, trackSize)
	}
	if size/trackSize > 96 {
		return nil, fmt.Errorf("not a TRS-80 image: %d tracks is too many for a single sided disk", size/trackSize)
	}
	data := make([]byte, size)
	if _, err := r.ReadAt(data, 0); err != nil {
		return nil, fmt.Errorf("failed to read image: %w", err)
	}
	return OpenImage(data, int(size/trackSize), 1, sectorsPerTrack, "IMG (raw sector image)")
}

// OpenImage parses sector data laid out by cylinder, head and sector, with
// 256 byte sectors. container describes the image format.
func OpenImage(data []byte, cylinders, heads, spt int, container string) (*Disk, error) {
	if len(data) != cylinders*heads*spt*sectorSize || len(data) == 0 {
		return nil, fmt.Errorf("not a TRS-80 image: %d bytes for %d cylinders x %d heads x %d sectors", len(data), cylinders, heads, spt)
	}
	d := &Disk{data: data, tracks: cylinders, heads: heads, spt: spt, dirTrack: defaultDirTrack, container: container}
	switch {
	case data[0] == 0x00 && data[1] == 0xFE: // LDOS: $00 $FE <directory cylinder>
		d.dos, d.dirTrack = ldos, int(data[2])
	case data[0] == 0xFE && data[1] > 0: // TRSDOS 1.3: $FE <directory track>
		d.dirTrack = int(data[1])
	}
	if d.dirTrack >= d.tracks {
		return nil, fmt.Errorf("not a TRSDOS disk: directory track %d beyond %d tracks", d.dirTrack, d.tracks)
	}
	name := "TRSDOS 1.3"
	if d.dos == ldos {
		name = "LDOS / TRSDOS 6"
		cfg := d.gat()[0xCD]
		d.gransPerCyl = int(cfg&0x07) + 1
		if cfg&0x20 != 0 {
			d.gransPerCyl *= 2 // double sided
		}
		if (cfg&0x20 != 0) != (heads == 2) {
			return nil, fmt.Errorf("not an %s disk: GAT sidedness does not match %d heads", name, heads)
		}
		if spt*heads%d.gransPerCyl != 0 {
			return nil, fmt.Errorf("not an %s disk: %d granules per cylinder does not divide %d sectors", name, d.gransPerCyl, spt*heads)
		}
		d.secPerGran = spt * heads / d.gransPerCyl
	} else {
		if heads != 1 || spt != sectorsPerTrack {
			return nil, fmt.Errorf("not a TRSDOS 1.3 disk: %d heads x %d sectors per track", heads, spt)
		}
		d.secPerGran, d.gransPerCyl = 3, 6
	}
	if err := d.check(); err != nil {
		return nil, fmt.Errorf("not a %s disk: %w", name, err)
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

func (d *Disk) secPerCyl() int { return d.spt * d.heads }

// sector returns sector idx (counting both sides) of a cylinder.
func (d *Disk) sector(cyl, idx int) []byte {
	off := (cyl*d.secPerCyl() + idx) * sectorSize
	if cyl < 0 || cyl >= d.tracks || idx < 0 || idx >= d.secPerCyl() {
		return nil
	}
	return d.data[off : off+sectorSize]
}

// entryLayout returns the directory entry size, entries per sector and
// number of directory sectors.
func (d *Disk) entryLayout() (size, perSector, sectors int) {
	if d.dos == ldos {
		// The HIT addresses up to 32 entry sectors.
		return 32, 8, min(d.secPerCyl(), firstDirSector+32)
	}
	return 48, 5, sectorsPerTrack
}

// entryAt returns the directory entry for an LDOS directory entry code
// (bits 7-5: entry within sector, bits 4-0: sector - 2).
func (d *Disk) entryAt(dec byte) []byte {
	size, _, sectors := d.entryLayout()
	s := int(dec&0x1F) + firstDirSector
	if s >= sectors {
		return nil
	}
	i := int(dec >> 5)
	return d.sector(d.dirTrack, s)[i*size : (i+1)*size]
}

// rawEntries returns the in use primary directory entries.
func (d *Disk) rawEntries() [][]byte {
	var out [][]byte
	size, perSector, sectors := d.entryLayout()
	for s := firstDirSector; s < sectors; s++ {
		sec := d.sector(d.dirTrack, s)
		for i := 0; i < perSector; i++ {
			e := sec[i*size : (i+1)*size]
			if e[0]&attrInUse != 0 && (d.dos != ldos || e[0]&attrFXDE == 0) {
				out = append(out, e)
			}
		}
	}
	return out
}

func (d *Disk) parseEntry(e []byte) (Entry, error) {
	name := strings.TrimRight(string(e[5:13]), " ")
	if ext := strings.TrimRight(string(e[13:16]), " "); ext != "" {
		name += "/" + ext
	}
	ern := int(binary.LittleEndian.Uint16(e[20:22]))
	eof := int(e[3])
	ent := Entry{Name: name, Attr: e[0], Protection: int(e[0] & 0x07)}

	if d.dos == trsdos13 {
		ent.Size = uint32(ern*sectorSize + eof)
		ent.Month, ent.Year = int(e[1]), int(e[2])
		for k := 22; k+1 < len(e) && e[k] != 0xFF; k += 2 {
			ent.Extents = append(ent.Extents, Extent{Track: int(e[k]), StartGran: int(e[k+1] >> 5), Grans: int(e[k+1] & 0x1F)})
		}
		return ent, nil
	}

	// LDOS: ERN counts sectors including a partial last sector of EOF bytes.
	switch {
	case ern == 0:
	case eof == 0:
		ent.Size = uint32(ern * sectorSize)
	default:
		ent.Size = uint32((ern-1)*sectorSize + eof)
	}
	ent.Month, ent.Day, ent.Year = int(e[1]&0x0F), int(e[2]>>3), 80+int(e[2]&0x07)
	if ent.Month >= 1 && ent.Month <= 12 && ent.Day >= 1 {
		t := time.Date(1900+ent.Year, time.Month(ent.Month), ent.Day, 0, 0, 0, 0, time.UTC)
		ent.ModTime = &t
	}
	// Extents: (cylinder, start granule << 5 | granules - 1); $FE links to
	// an extended entry (FXDE) by directory entry code; $FF ends the list.
	visited := map[byte]bool{}
	for k := 22; k+1 < len(e); k += 2 {
		switch e[k] {
		case 0xFF:
			return ent, nil
		case 0xFE:
			dec := e[k+1]
			x := d.entryAt(dec)
			if x == nil || visited[dec] || x[0]&(attrFXDE|attrInUse) != attrFXDE|attrInUse {
				return ent, fmt.Errorf("invalid extended directory entry link $%02X", dec)
			}
			visited[dec] = true
			e, k = x, 20 // continue with the FXDE's extents at offset 22
		default:
			ent.Extents = append(ent.Extents, Extent{Track: int(e[k]), StartGran: int(e[k+1] >> 5), Grans: int(e[k+1]&0x1F) + 1})
		}
	}
	return ent, nil
}

// readExtents returns the data of all sectors allocated to e, trimmed to its
// size.
func (d *Disk) readExtents(e Entry) ([]byte, error) {
	var out []byte
	for _, x := range e.Extents {
		for s := x.StartGran * d.secPerGran; s < (x.StartGran+x.Grans)*d.secPerGran; s++ {
			sec := d.sector(x.Track+s/d.secPerCyl(), s%d.secPerCyl())
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

// entries parses all directory entries, flagging unreadable ones as corrupt.
func (d *Disk) entries() []Entry {
	var res []Entry
	for _, raw := range d.rawEntries() {
		e, err := d.parseEntry(raw)
		if err == nil {
			_, err = d.readExtents(e)
		}
		if err != nil {
			e.Corrupt, e.Error = true, err.Error()
		}
		res = append(res, e)
	}
	return res
}

// List returns the directory entries.
func (d *Disk) List() ([]Entry, error) { return d.entries(), nil }

// ReadFile returns the contents of a named file ("BASIC/CMD" or "BASIC.CMD").
func (d *Disk) ReadFile(name string) ([]byte, error) {
	want := strings.ReplaceAll(strings.TrimPrefix(name, "/"), ".", "/")
	for _, raw := range d.rawEntries() {
		e, err := d.parseEntry(raw)
		if !strings.EqualFold(e.Name, want) {
			continue
		}
		if err != nil {
			return nil, err
		}
		return d.readExtents(e)
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
		for g := 0; g < d.gransPerCyl; g++ {
			if gat[t]&(1<<g) != 0 {
				used++
			}
		}
	}
	total := d.tracks * d.secPerCyl()
	inUse := used * d.secPerGran
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
	if d.spt == 18 && (d.tracks == 40 || d.tracks == 80) {
		u.UnformattedCapacity = int64(d.tracks*d.heads) * 6250
		u.UnformattedDesc = fmt.Sprintf("%d KB", d.tracks*d.heads*6250/1000)
	}
	return u, nil
}

// system returns the platform and DOS name.
func (d *Disk) system() (platform, dos string) {
	if d.dos == trsdos13 {
		return "TRS-80 Model III", "TRSDOS 1.3"
	}
	v := d.gat()[0xCB]
	switch {
	case v == 0x63:
		return "TRS-80 Model 4", "LS-DOS 6.3"
	case v>>4 == 6:
		return "TRS-80 Model 4", fmt.Sprintf("TRSDOS %d.%d", v>>4, v&0x0F)
	case v>>4 == 5:
		return "TRS-80 Model I/III", fmt.Sprintf("LDOS %d.%d", v>>4, v&0x0F)
	}
	return "TRS-80", fmt.Sprintf("LDOS compatible (version $%02X)", v)
}

// Format describes the image format and geometry.
func (d *Disk) Format() *util.FormatInfo {
	platform, dos := d.system()
	return &util.FormatInfo{
		Platform:   platform,
		Image:      d.container,
		Filesystem: dos,
		Tracks:     d.tracks,
		Heads:      d.heads,
		Sectors:    fmt.Sprintf("%d", d.spt),
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
	platform, dos := d.system()
	fmt.Fprintf(w, "%s %s Disk Image Dump\n", platform, dos)
	fmt.Fprintf(w, "  Disk name: %q, Date: %s\n", d.Label(), d.Date())
	fmt.Fprintf(w, "  Tracks: %d, Heads: %d, Sectors/track: %d, Sector size: %d, Directory track: %d\n",
		d.tracks, d.heads, d.spt, sectorSize, d.dirTrack)
	fmt.Fprintf(w, "  Granules: %d per cylinder, %d sectors each\n", d.gransPerCyl, d.secPerGran)
	if u, err := d.Usage(); err == nil {
		fmt.Fprintf(w, "  In-use: %d sectors, Free: %d sectors, Total: %d sectors\n", u.InUseSectors, u.FreeSectors, u.TotalSectors)
	}
	entries := d.entries()
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
		date := fmt.Sprintf("%02d/%02d", e.Month, e.Year)
		if d.dos == ldos {
			date = fmt.Sprintf("%02d/%02d/%02d", e.Month, e.Day, e.Year)
		}
		fmt.Fprintf(w, "    %s %-12s %6d bytes  %s  extents: %s%s\n",
			attrString(e.Attr), e.Name, e.Size, date, strings.Join(ext, ", "), corrupt)
	}
	return nil
}
