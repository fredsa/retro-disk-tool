// Package ibm reads IBM PC compatible raw sector disk images (IMG / IMA)
// holding FAT12, FAT16 or FAT32 (MS-DOS / PC DOS / Windows) volumes.
package ibm

import (
	"encoding/binary"
	"fmt"
	"io"
	"strings"
	"time"
	"unicode/utf16"

	"github.com/fredsa/retro-disk-tool/cmd/rdt/pkg/util"
)

const (
	dirEntrySize = 32

	attrReadOnly = 0x01
	attrHidden   = 0x02
	attrSystem   = 0x04
	attrVolume   = 0x08
	attrDir      = 0x10
	attrArchive  = 0x20
	attrLFN      = 0x0F
)

// Entry is a directory entry.
type Entry struct {
	Name      string // long file name if present, else the 8.3 name
	ShortName string // 8.3 name
	IsDir     bool
	Size      uint32
	ModTime   *time.Time
	Attr      byte
	Cluster   uint32
	Corrupt   bool
	Error     string
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

// bpb holds the BIOS Parameter Block fields needed to read the volume.
type bpb struct {
	oem          string
	bytesPerSec  int
	secPerClus   int
	reserved     int
	numFATs      int
	rootEntries  int
	totalSectors int
	media        byte
	fatSize      int
	secPerTrack  int
	heads        int
	rootCluster  uint32 // FAT32 only
	serial       uint32
	label        string // boot sector volume label (extended BPB)
	fromBPB      bool   // false when geometry was inferred (DOS 1.x disks)
}

// Disk is an opened FAT volume.
type Disk struct {
	data      []byte // volume data (starting at the boot sector)
	partition int    // MBR partition number (1-4), 0 for an unpartitioned image
	partOff   int    // byte offset of the partition in the image
	b         bpb
	fatBits   int // 12, 16 or 32
	fatStart  int // byte offsets
	rootStart int
	dataStart int
	clusters  int // number of data clusters
}

// Standard floppy geometries, used when the boot sector has no BPB (DOS 1.x).
var floppyGeometries = []struct {
	size                                     int
	media                                    byte
	secPerClus, rootEntries, fatSize, spt, h int
}{
	{160 * 1024, 0xFE, 1, 64, 1, 8, 1},
	{180 * 1024, 0xFC, 1, 64, 2, 9, 1},
	{320 * 1024, 0xFF, 2, 112, 1, 8, 2},
	{360 * 1024, 0xFD, 2, 112, 2, 9, 2},
	{720 * 1024, 0xF9, 2, 112, 3, 9, 2},
	{1200 * 1024, 0xF9, 1, 224, 7, 15, 2},
	{1440 * 1024, 0xF0, 1, 224, 9, 18, 2},
	{2880 * 1024, 0xF0, 2, 240, 9, 36, 2},
}

// Open parses a raw IBM PC disk image.
func Open(r io.ReaderAt, size int64) (*Disk, error) {
	if size < 512 {
		return nil, fmt.Errorf("image too small: %d bytes", size)
	}
	raw := make([]byte, size)
	if _, err := r.ReadAt(raw, 0); err != nil {
		return nil, fmt.Errorf("failed to read image: %w", err)
	}

	d := &Disk{data: raw}
	b, ok := parseBPB(raw)
	if !ok {
		if off, n := findPartition(raw); off > 0 {
			if pb, pok := parseBPB(raw[off:]); pok {
				d.data, d.partition, d.partOff, b, ok = raw[off:], n, off, pb, true
			}
		}
	}
	if !ok {
		if b, ok = inferBPB(raw); !ok {
			return nil, fmt.Errorf("no FAT boot sector (BPB) found")
		}
	}
	d.b = b

	d.fatStart = b.reserved * b.bytesPerSec
	rootSectors := (b.rootEntries*dirEntrySize + b.bytesPerSec - 1) / b.bytesPerSec
	d.rootStart = d.fatStart + b.numFATs*b.fatSize*b.bytesPerSec
	d.dataStart = d.rootStart + rootSectors*b.bytesPerSec
	dataSectors := b.totalSectors - d.dataStart/b.bytesPerSec
	if dataSectors <= 0 {
		return nil, fmt.Errorf("invalid FAT layout: no data area")
	}
	d.clusters = dataSectors / b.secPerClus
	switch {
	case d.clusters < 4085:
		d.fatBits = 12
	case d.clusters < 65525:
		d.fatBits = 16
	default:
		d.fatBits = 32
	}
	if d.fatBits == 32 && b.rootCluster < 2 {
		return nil, fmt.Errorf("invalid FAT32 root cluster %d", b.rootCluster)
	}
	if d.fatStart+b.fatSize*b.bytesPerSec > len(d.data) || d.dataStart > len(d.data) {
		return nil, fmt.Errorf("truncated image: FAT area extends beyond end of file")
	}
	return d, nil
}

func parseBPB(s []byte) (bpb, bool) {
	if len(s) < 512 {
		return bpb{}, false
	}
	le16 := func(o int) int { return int(binary.LittleEndian.Uint16(s[o:])) }
	b := bpb{
		oem:         strings.TrimRight(string(s[3:11]), " \x00"),
		bytesPerSec: le16(11),
		secPerClus:  int(s[13]),
		reserved:    le16(14),
		numFATs:     int(s[16]),
		rootEntries: le16(17),
		media:       s[21],
		fatSize:     le16(22),
		secPerTrack: le16(24),
		heads:       le16(26),
		fromBPB:     true,
	}
	b.totalSectors = le16(19)
	if b.totalSectors == 0 {
		b.totalSectors = int(binary.LittleEndian.Uint32(s[32:]))
	}
	extOff := 36 // extended BPB (FAT12/16)
	if b.fatSize == 0 {
		b.fatSize = int(binary.LittleEndian.Uint32(s[36:]))
		b.rootCluster = binary.LittleEndian.Uint32(s[44:])
		extOff = 64
	}
	if s[extOff+2] == 0x29 {
		b.serial = binary.LittleEndian.Uint32(s[extOff+3:])
		b.label = strings.TrimRight(string(s[extOff+7:extOff+18]), " \x00")
	}

	pow2 := func(n int) bool { return n > 0 && n&(n-1) == 0 }
	if !pow2(b.bytesPerSec) || b.bytesPerSec < 128 || b.bytesPerSec > 4096 ||
		!pow2(b.secPerClus) || b.reserved < 1 || b.numFATs < 1 || b.numFATs > 2 ||
		b.fatSize == 0 || b.totalSectors == 0 || (b.media < 0xF0 && b.media != 0) {
		return bpb{}, false
	}
	return b, true
}

// inferBPB derives the layout of a DOS 1.x floppy (no BPB) from the image
// size and the media descriptor in the first FAT byte.
func inferBPB(raw []byte) (bpb, bool) {
	for _, g := range floppyGeometries {
		if len(raw) == g.size && len(raw) > 512 && raw[512] == g.media {
			return bpb{
				bytesPerSec:  512,
				secPerClus:   g.secPerClus,
				reserved:     1,
				numFATs:      2,
				rootEntries:  g.rootEntries,
				totalSectors: g.size / 512,
				media:        g.media,
				fatSize:      g.fatSize,
				secPerTrack:  g.spt,
				heads:        g.h,
			}, true
		}
	}
	return bpb{}, false
}

// findPartition returns the byte offset and number of the first FAT
// partition in an MBR partition table.
func findPartition(raw []byte) (int, int) {
	if raw[510] != 0x55 || raw[511] != 0xAA {
		return 0, 0
	}
	for i := 0; i < 4; i++ {
		e := raw[446+i*16 : 446+(i+1)*16]
		switch e[4] {
		case 0x01, 0x04, 0x06, 0x0B, 0x0C, 0x0E:
			off := int(binary.LittleEndian.Uint32(e[8:])) * 512
			if off > 0 && off+512 <= len(raw) {
				return off, i + 1
			}
		}
	}
	return 0, 0
}

// fatEntry returns the FAT value for cluster n.
func (d *Disk) fatEntry(n uint32) uint32 {
	fat := d.data[d.fatStart:]
	switch d.fatBits {
	case 12:
		o := int(n) * 3 / 2
		if o+1 >= len(fat) {
			return 0xFFF
		}
		v := uint32(binary.LittleEndian.Uint16(fat[o:]))
		if n&1 != 0 {
			return v >> 4
		}
		return v & 0xFFF
	case 16:
		o := int(n) * 2
		if o+1 >= len(fat) {
			return 0xFFFF
		}
		return uint32(binary.LittleEndian.Uint16(fat[o:]))
	default:
		o := int(n) * 4
		if o+3 >= len(fat) {
			return 0x0FFFFFFF
		}
		return binary.LittleEndian.Uint32(fat[o:]) & 0x0FFFFFFF
	}
}

func (d *Disk) isEOC(v uint32) bool {
	switch d.fatBits {
	case 12:
		return v >= 0xFF8
	case 16:
		return v >= 0xFFF8
	default:
		return v >= 0x0FFFFFF8
	}
}

func (d *Disk) clusterSize() int { return d.b.secPerClus * d.b.bytesPerSec }

// chain returns the cluster numbers in the chain starting at start.
func (d *Disk) chain(start uint32) ([]uint32, error) {
	var out []uint32
	visited := map[uint32]bool{}
	for c := start; ; {
		if c < 2 || int(c) >= d.clusters+2 {
			return out, fmt.Errorf("invalid cluster %d in chain", c)
		}
		if visited[c] {
			return out, fmt.Errorf("cyclic cluster chain at cluster %d", c)
		}
		visited[c] = true
		out = append(out, c)
		next := d.fatEntry(c)
		if d.isEOC(next) {
			return out, nil
		}
		c = next
	}
}

// readChain returns the data of the clusters in the chain starting at start.
func (d *Disk) readChain(start uint32) ([]byte, error) {
	clusters, err := d.chain(start)
	cs := d.clusterSize()
	var out []byte
	for _, c := range clusters {
		off := d.dataStart + int(c-2)*cs
		if off+cs > len(d.data) {
			return out, fmt.Errorf("cluster %d beyond end of image", c)
		}
		out = append(out, d.data[off:off+cs]...)
	}
	return out, err
}

// dirData returns the raw directory entries of the directory at cluster
// (0 for the root directory).
func (d *Disk) dirData(cluster uint32) ([]byte, error) {
	if cluster == 0 {
		if d.fatBits == 32 {
			return d.readChain(d.b.rootCluster)
		}
		end := d.rootStart + d.b.rootEntries*dirEntrySize
		if end > len(d.data) {
			return nil, fmt.Errorf("root directory beyond end of image")
		}
		return d.data[d.rootStart:end], nil
	}
	return d.readChain(cluster)
}

func lfnChecksum(short []byte) byte {
	var sum byte
	for _, c := range short[:11] {
		sum = (sum>>1 | sum<<7) + c
	}
	return sum
}

func shortName(e []byte) string {
	base := append([]byte{}, e[0:8]...)
	if base[0] == 0x05 {
		base[0] = 0xE5
	}
	b := strings.TrimRight(decodeCP437(base), " ")
	x := strings.TrimRight(decodeCP437(e[8:11]), " ")
	// Windows NT stores all lowercase 8.3 names as uppercase plus flags.
	if e[12]&0x08 != 0 {
		b = strings.ToLower(b)
	}
	if e[12]&0x10 != 0 {
		x = strings.ToLower(x)
	}
	if x != "" {
		return b + "." + x
	}
	return b
}

func dosTime(date, tm uint16) *time.Time {
	if date == 0 {
		return nil
	}
	t := time.Date(1980+int(date>>9), time.Month(date>>5&0x0F), int(date&0x1F),
		int(tm>>11), int(tm>>5&0x3F), int(tm&0x1F)*2, 0, time.UTC)
	return &t
}

// readDir parses the directory at cluster (0 for root), returning its entries
// and the volume label (if any).
func (d *Disk) readDir(cluster uint32) ([]Entry, string, error) {
	data, err := d.dirData(cluster)
	var entries []Entry
	label := ""
	var lfn []uint16
	lfnSum, lfnNext := byte(0), 0
	for o := 0; o+dirEntrySize <= len(data); o += dirEntrySize {
		e := data[o : o+dirEntrySize]
		if e[0] == 0x00 {
			break // end of directory
		}
		if e[0] == 0xE5 {
			lfn = nil
			continue // deleted
		}
		attr := e[11]
		if attr&0x3F == attrLFN {
			seq := int(e[0] & 0x1F)
			if e[0]&0x40 != 0 {
				lfn = make([]uint16, seq*13)
				lfnSum, lfnNext = e[13], seq
			}
			if lfn == nil || seq != lfnNext || e[13] != lfnSum || seq == 0 {
				lfn = nil
				continue
			}
			var part []uint16
			for _, r := range [][2]int{{1, 11}, {14, 26}, {28, 32}} {
				for i := r[0]; i < r[1]; i += 2 {
					part = append(part, binary.LittleEndian.Uint16(e[i:]))
				}
			}
			copy(lfn[(seq-1)*13:], part)
			lfnNext--
			continue
		}
		if attr&attrVolume != 0 {
			if cluster == 0 && label == "" {
				label = strings.TrimRight(decodeCP437(e[0:11]), " ")
			}
			lfn = nil
			continue
		}

		short := shortName(e)
		name := short
		if lfn != nil && lfnNext == 0 && lfnSum == lfnChecksum(e) {
			for i, c := range lfn {
				if c == 0x0000 || c == 0xFFFF {
					lfn = lfn[:i]
					break
				}
			}
			name = string(utf16.Decode(lfn))
		}
		lfn = nil
		if short == "." || short == ".." {
			continue
		}

		start := uint32(binary.LittleEndian.Uint16(e[26:]))
		if d.fatBits == 32 {
			start |= uint32(binary.LittleEndian.Uint16(e[20:])) << 16
		}
		ent := Entry{
			Name:      name,
			ShortName: short,
			IsDir:     attr&attrDir != 0,
			Attr:      attr,
			Cluster:   start,
			ModTime:   dosTime(binary.LittleEndian.Uint16(e[24:]), binary.LittleEndian.Uint16(e[22:])),
		}
		if !ent.IsDir {
			ent.Size = binary.LittleEndian.Uint32(e[28:])
			if ent.Size > 0 {
				clusters, cerr := d.chain(start)
				if cerr != nil {
					ent.Corrupt, ent.Error = true, cerr.Error()
				} else if need := (int(ent.Size) + d.clusterSize() - 1) / d.clusterSize(); len(clusters) < need {
					ent.Corrupt = true
					ent.Error = fmt.Sprintf("cluster chain too short: %d clusters, need %d", len(clusters), need)
				}
			}
		}
		entries = append(entries, ent)
	}
	return entries, label, err
}

// List returns the root directory entries.
func (d *Disk) List() ([]Entry, error) {
	entries, _, err := d.readDir(0)
	return entries, err
}

// find resolves a "/" or "\" separated path to a directory entry.
func (d *Disk) find(path string) (*Entry, error) {
	parts := strings.FieldsFunc(path, func(r rune) bool { return r == '/' || r == '\\' })
	if len(parts) == 0 {
		return nil, fmt.Errorf("empty path")
	}
	cluster := uint32(0)
	for i, p := range parts {
		entries, _, _ := d.readDir(cluster)
		var found *Entry
		for j := range entries {
			if strings.EqualFold(entries[j].Name, p) || strings.EqualFold(entries[j].ShortName, p) {
				found = &entries[j]
				break
			}
		}
		if found == nil {
			return nil, fmt.Errorf("file %q not found", path)
		}
		if i == len(parts)-1 {
			return found, nil
		}
		if !found.IsDir {
			return nil, fmt.Errorf("%q is not a directory", strings.Join(parts[:i+1], "/"))
		}
		cluster = found.Cluster
	}
	return nil, fmt.Errorf("file %q not found", path)
}

// ReadFile returns the contents of the file at path (e.g. "DOS/README.TXT").
func (d *Disk) ReadFile(path string) ([]byte, error) {
	e, err := d.find(path)
	if err != nil {
		return nil, err
	}
	if e.IsDir {
		return nil, fmt.Errorf("%q is a directory, not a file", path)
	}
	if e.Size == 0 {
		return []byte{}, nil
	}
	data, err := d.readChain(e.Cluster)
	if int(e.Size) <= len(data) {
		data = data[:e.Size]
	}
	return data, err
}

// Label returns the volume label from the root directory, falling back to
// the boot sector label.
func (d *Disk) Label() string {
	_, label, _ := d.readDir(0)
	if label == "" && d.b.label != "NO NAME" {
		label = d.b.label
	}
	return label
}

// tracks returns the number of tracks (cylinders), or 0 if unknown.
func (d *Disk) tracks() int {
	perCyl := d.b.secPerTrack * d.b.heads
	if perCyl == 0 || d.b.totalSectors%perCyl != 0 {
		return 0
	}
	return d.b.totalSectors / perCyl
}

// unformatted returns the raw (unformatted) capacity of standard floppy
// disks, based on sectors per track (data rate) and tracks.
func (d *Disk) unformatted() (int64, string) {
	t, h := d.tracks(), d.b.heads
	switch {
	case d.partition != 0 || t == 0:
	case d.b.secPerTrack <= 9 && t == 40: // 5.25" DD, 250 kbit/s at 300 rpm
		return int64(250000 * h), fmt.Sprintf("%d KB", 250*h)
	case d.b.secPerTrack <= 9 && t == 80: // 3.5" DD
		return int64(500000 * h), map[int]string{1: "500 KB", 2: "1 MB"}[h]
	case d.b.secPerTrack == 15 && t == 80: // 5.25" HD, 500 kbit/s at 360 rpm
		return 1666666, "1.6 MB"
	case d.b.secPerTrack == 18 && t == 80: // 3.5" HD
		return 2000000, "2 MB"
	case d.b.secPerTrack == 36 && t == 80: // 3.5" ED
		return 4000000, "4 MB"
	}
	return 0, ""
}

// Usage computes sector usage from the FAT.
func (d *Disk) Usage() (*DiskUsage, error) {
	freeClusters := 0
	for c := uint32(2); int(c) < d.clusters+2; c++ {
		if d.fatEntry(c) == 0 {
			freeClusters++
		}
	}
	total := d.b.totalSectors
	free := freeClusters * d.b.secPerClus
	sz := int64(d.b.bytesPerSec)
	totalBytes := int64(total) * sz
	desc := fmt.Sprintf("%d KiB", (totalBytes+512)/1024)
	if totalBytes >= 10*1024*1024 {
		desc = fmt.Sprintf("%d MiB", (totalBytes+512*1024)/(1024*1024))
	}
	ucap, udesc := d.unformatted()
	return &DiskUsage{
		FormattedCapacity:   totalBytes,
		FormattedDesc:       desc,
		UnformattedCapacity: ucap,
		UnformattedDesc:     udesc,
		SectorSize:          d.b.bytesPerSec,
		TotalSectors:        total,
		InUseSectors:        total - free,
		FreeSectors:         free,
		TotalBytes:          totalBytes,
		InUseBytes:          int64(total-free) * sz,
		FreeBytes:           int64(free) * sz,
	}, nil
}

// Format describes the image format and geometry.
func (d *Disk) Format() *util.FormatInfo {
	image := "IMG (raw sector image)"
	if d.partition != 0 {
		image = fmt.Sprintf("IMG (raw disk image, MBR partition %d)", d.partition)
	}
	fs := fmt.Sprintf("FAT%d", d.fatBits)
	if !d.b.fromBPB {
		fs += " (DOS 1.x, no BPB)"
	}
	return &util.FormatInfo{
		Platform:   "IBM PC",
		Image:      image,
		Filesystem: fs,
		Tracks:     d.tracks(),
		Heads:      d.b.heads,
		Sectors:    fmt.Sprintf("%d", d.b.secPerTrack),
		SectorSize: d.b.bytesPerSec,
	}
}

func attrString(a byte) string {
	s := []byte("------")
	for i, f := range []struct {
		bit byte
		c   byte
	}{{attrDir, 'd'}, {attrArchive, 'a'}, {attrReadOnly, 'r'}, {attrHidden, 'h'}, {attrSystem, 's'}, {attrVolume, 'v'}} {
		if a&f.bit != 0 {
			s[i] = f.c
		}
	}
	return string(s)
}

func (d *Disk) dumpDir(w io.Writer, cluster uint32, prefix string, depth int) {
	entries, _, err := d.readDir(cluster)
	if err != nil {
		fmt.Fprintf(w, "    %s[ERROR: %v]\n", prefix, err)
	}
	for _, e := range entries {
		ts := "                   "
		if e.ModTime != nil {
			ts = e.ModTime.Format("2006-01-02 15:04:05")
		}
		name := prefix + e.Name
		if e.IsDir {
			name += "/"
		}
		if e.Name != e.ShortName {
			name += fmt.Sprintf(" (%s)", e.ShortName)
		}
		corrupt := ""
		if e.Corrupt {
			corrupt = fmt.Sprintf(" [CORRUPT: %s]", e.Error)
		}
		fmt.Fprintf(w, "    %s %8d %s cluster %5d  %s%s\n", attrString(e.Attr), e.Size, ts, e.Cluster, name, corrupt)
		if e.IsDir && depth < 16 && e.Cluster >= 2 {
			d.dumpDir(w, e.Cluster, prefix+e.Name+"/", depth+1)
		}
	}
}

// Dump writes a detailed description of the disk.
func (d *Disk) Dump(w io.Writer) error {
	b := d.b
	fmt.Fprintf(w, "IBM PC FAT Disk Image Dump\n")
	if d.partition != 0 {
		fmt.Fprintf(w, "  MBR partition: %d (offset %d)\n", d.partition, d.partOff)
	}
	fmt.Fprintf(w, "  Filesystem: FAT%d\n", d.fatBits)
	if b.fromBPB {
		fmt.Fprintf(w, "  OEM name: %q\n", b.oem)
	}
	fmt.Fprintf(w, "  Media descriptor: 0x%02X\n", b.media)
	fmt.Fprintf(w, "  Bytes/sector: %d, Sectors/cluster: %d, Reserved sectors: %d\n", b.bytesPerSec, b.secPerClus, b.reserved)
	fmt.Fprintf(w, "  FATs: %d x %d sectors, Root entries: %d\n", b.numFATs, b.fatSize, b.rootEntries)
	fmt.Fprintf(w, "  Total sectors: %d, Data clusters: %d\n", b.totalSectors, d.clusters)
	fmt.Fprintf(w, "  Geometry: %d tracks, %d heads, %d sectors/track\n", d.tracks(), b.heads, b.secPerTrack)
	if b.serial != 0 {
		fmt.Fprintf(w, "  Volume serial: %04X-%04X\n", b.serial>>16, b.serial&0xFFFF)
	}
	fmt.Fprintf(w, "  Volume label: %q\n", d.Label())
	if u, err := d.Usage(); err == nil {
		fmt.Fprintf(w, "  In-use: %d sectors, Free: %d sectors, Total: %d sectors\n", u.InUseSectors, u.FreeSectors, u.TotalSectors)
	}
	fmt.Fprintf(w, "  Directory tree:\n")
	d.dumpDir(w, 0, "", 0)
	return nil
}
