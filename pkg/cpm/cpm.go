// Package cpm reads CP/M and CP/M-86 disk images (raw IMG/IMA/VFD/CPM and
// container-decoded images such as IMD), including standard IBM PC CP/M-86
// floppy formats (160K, 320K, 360K, 720K, 1.2M, 1.44M), 8-inch SSSD floppies,
// and MBR type 0xDB CP/M-86 hard disk partitions.
package cpm

import (
	"encoding/binary"
	"fmt"
	"io"
	"sort"
	"strings"
	"time"

	"github.com/fredsa/retro-disk-tool/cmd/rdt/pkg/util"
)

const (
	dirEntrySize = 32
	recordSize   = 128

	userDeleted = 0xE5
	userLabel   = 0x20
	userStamp   = 0x21
)

// SideOrder specifies how logical CP/M tracks map to physical (cylinder, head)
// positions in a CHS-ordered sector image.
type SideOrder int

const (
	SideSingle  SideOrder = iota // 1 head: track -> (track, 0)
	SideOutBack                  // 2 heads: 0..C-1 on head 0, C-1..0 on head 1 (CP/M-86 standard)
	SideAlt                      // 2 heads: track -> (track/2, track%2)
	SideOutOut                   // 2 heads: 0..C-1 on head 0, 0..C-1 on head 1
)

func (s SideOrder) String() string {
	switch s {
	case SideSingle:
		return "single-sided"
	case SideOutBack:
		return "out-and-back"
	case SideAlt:
		return "alternating"
	case SideOutOut:
		return "out-and-out"
	default:
		return fmt.Sprintf("unknown(%d)", int(s))
	}
}

// Spec describes the physical geometry and CP/M Disk Parameter Block (DPB) of
// a CP/M volume.
type Spec struct {
	Name            string
	Platform        string
	Cylinders       int
	Heads           int
	SectorsPerTrack int
	SectorSize      int
	BlockSize       int
	MaxDir          int
	BootTracks      int
	SideOrder       SideOrder
	Skew            int // 0 or 1 means no sector skew
	TotalBlocks     int // DSM + 1; computed from geometry if 0
	HasIDByte       bool
	IDByte          byte
	Partition       int // MBR partition number (1-4), 0 for floppy / unpartitioned
	PartLBA         int // starting LBA of MBR partition
	PartSectors     int // sector count of MBR partition
}

func (s Spec) totalTracks() int { return s.Cylinders * s.Heads }

func (s Spec) totalBlocks() int {
	if s.TotalBlocks > 0 {
		return s.TotalBlocks
	}
	dataTracks := s.totalTracks() - s.BootTracks
	if dataTracks <= 0 || s.BlockSize <= 0 {
		return 0
	}
	return (dataTracks * s.SectorsPerTrack * s.SectorSize) / s.BlockSize
}

func (s Spec) dirBlocks() int {
	if s.BlockSize <= 0 {
		return 0
	}
	return (s.MaxDir*dirEntrySize + s.BlockSize - 1) / s.BlockSize
}

func (s Spec) use16BitPtrs() bool { return s.totalBlocks()-1 >= 256 }

func (s Spec) ptrsPerExtent() int {
	if s.use16BitPtrs() {
		return 8
	}
	return 16
}

func (s Spec) extentMask() int {
	if s.use16BitPtrs() {
		if s.BlockSize < 2048 {
			return 0
		}
		return s.BlockSize/2048 - 1
	}
	return s.BlockSize/1024 - 1
}

// standardFormats lists the known CP/M-86 and generic CP/M floppy formats,
// ordered from most specific / standard to fallback variants.
var standardFormats = []Spec{
	// IBM PC CP/M-86 160 KiB (5.25" SS, 8 spt)
	{
		Name: "CP/M-86", Platform: "IBM PC",
		Cylinders: 40, Heads: 1, SectorsPerTrack: 8, SectorSize: 512,
		BlockSize: 1024, MaxDir: 64, BootTracks: 1, SideOrder: SideSingle,
		HasIDByte: true, IDByte: 0x00,
	},
	// IBM PC CP/M-86 320 KiB (5.25" DS, 8 spt, out-and-back)
	{
		Name: "CP/M-86", Platform: "IBM PC",
		Cylinders: 40, Heads: 2, SectorsPerTrack: 8, SectorSize: 512,
		BlockSize: 2048, MaxDir: 64, BootTracks: 1, SideOrder: SideOutBack,
		HasIDByte: true, IDByte: 0x01,
	},
	// OEM / Columbia CP/M-86 320 KiB (5.25" DS, 8 spt, out-and-out, 128 dir entries)
	{
		Name: "CP/M-86", Platform: "IBM PC",
		Cylinders: 40, Heads: 2, SectorsPerTrack: 8, SectorSize: 512,
		BlockSize: 2048, MaxDir: 128, BootTracks: 1, SideOrder: SideOutOut,
		HasIDByte: true, IDByte: 0x00,
	},
	// IBM PC CP/M-86 320 KiB (logical / untwisted linear image)
	{
		Name: "CP/M-86", Platform: "IBM PC",
		Cylinders: 40, Heads: 2, SectorsPerTrack: 8, SectorSize: 512,
		BlockSize: 2048, MaxDir: 64, BootTracks: 1, SideOrder: SideAlt,
		HasIDByte: true, IDByte: 0x01,
	},
	// Personal CP/M-86 360 KiB (5.25" DS, 9 spt, ID 0x10)
	{
		Name: "CP/M-86", Platform: "IBM PC",
		Cylinders: 40, Heads: 2, SectorsPerTrack: 9, SectorSize: 512,
		BlockSize: 2048, MaxDir: 64, BootTracks: 4, SideOrder: SideOutBack,
		HasIDByte: true, IDByte: 0x10,
	},
	{
		Name: "CP/M-86", Platform: "IBM PC",
		Cylinders: 40, Heads: 2, SectorsPerTrack: 9, SectorSize: 512,
		BlockSize: 2048, MaxDir: 64, BootTracks: 4, SideOrder: SideAlt,
		HasIDByte: true, IDByte: 0x10,
	},
	// Personal CP/M-86 360 KiB (ID 0x40)
	{
		Name: "CP/M-86", Platform: "IBM PC",
		Cylinders: 40, Heads: 2, SectorsPerTrack: 9, SectorSize: 512,
		BlockSize: 2048, MaxDir: 64, BootTracks: 4, SideOrder: SideOutBack,
		HasIDByte: true, IDByte: 0x40,
	},
	{
		Name: "CP/M-86", Platform: "IBM PC",
		Cylinders: 40, Heads: 2, SectorsPerTrack: 9, SectorSize: 512,
		BlockSize: 2048, MaxDir: 64, BootTracks: 2, SideOrder: SideOutBack,
	},
	// IBM PC CP/M-86 720 KiB (3.5" DS, 9 spt, 144FEAT, ID 0x48)
	{
		Name: "CP/M-86", Platform: "IBM PC",
		Cylinders: 80, Heads: 2, SectorsPerTrack: 9, SectorSize: 512,
		BlockSize: 2048, MaxDir: 256, BootTracks: 2, SideOrder: SideOutBack,
		HasIDByte: true, IDByte: 0x48,
	},
	// Personal CP/M-86 720 KiB (3.5" DS, 9 spt, ID 0x11)
	{
		Name: "CP/M-86", Platform: "IBM PC",
		Cylinders: 80, Heads: 2, SectorsPerTrack: 9, SectorSize: 512,
		BlockSize: 2048, MaxDir: 256, BootTracks: 4, SideOrder: SideOutBack,
		HasIDByte: true, IDByte: 0x11,
	},
	{
		Name: "CP/M-86", Platform: "IBM PC",
		Cylinders: 80, Heads: 2, SectorsPerTrack: 9, SectorSize: 512,
		BlockSize: 2048, MaxDir: 256, BootTracks: 4, SideOrder: SideAlt,
		HasIDByte: true, IDByte: 0x11,
	},
	{
		Name: "CP/M-86", Platform: "IBM PC",
		Cylinders: 80, Heads: 2, SectorsPerTrack: 9, SectorSize: 512,
		BlockSize: 2048, MaxDir: 256, BootTracks: 2, SideOrder: SideAlt,
	},
	// IBM PC CP/M-86 1.2 MiB (5.25" HD, 15 spt, 144FEAT, ID 0x0C)
	{
		Name: "CP/M-86", Platform: "IBM PC",
		Cylinders: 80, Heads: 2, SectorsPerTrack: 15, SectorSize: 512,
		BlockSize: 4096, MaxDir: 256, BootTracks: 2, SideOrder: SideOutBack,
		HasIDByte: true, IDByte: 0x0C,
	},
	{
		Name: "CP/M-86", Platform: "IBM PC",
		Cylinders: 80, Heads: 2, SectorsPerTrack: 15, SectorSize: 512,
		BlockSize: 4096, MaxDir: 256, BootTracks: 2, SideOrder: SideAlt,
		HasIDByte: true, IDByte: 0x0C,
	},
	// IBM PC CP/M-86 1.44 MiB (3.5" HD, 18 spt, 144FEAT, ID 0x90)
	{
		Name: "CP/M-86", Platform: "IBM PC",
		Cylinders: 80, Heads: 2, SectorsPerTrack: 18, SectorSize: 512,
		BlockSize: 4096, MaxDir: 256, BootTracks: 2, SideOrder: SideOutBack,
		HasIDByte: true, IDByte: 0x90,
	},
	{
		Name: "CP/M-86", Platform: "IBM PC",
		Cylinders: 80, Heads: 2, SectorsPerTrack: 18, SectorSize: 512,
		BlockSize: 4096, MaxDir: 256, BootTracks: 2, SideOrder: SideAlt,
		HasIDByte: true, IDByte: 0x90,
	},
	// CP/M-86 800 KiB (80 tracks, 2 heads, 5 x 1024-byte sectors, skew 2)
	{
		Name: "CP/M-86", Platform: "CP/M-86",
		Cylinders: 80, Heads: 2, SectorsPerTrack: 5, SectorSize: 1024,
		BlockSize: 2048, MaxDir: 256, BootTracks: 4, SideOrder: SideAlt,
		Skew: 2,
	},
	// Standard IBM 3740 8" SSSD CP/M (77 tracks, 26 x 128-byte sectors, skew 6)
	{
		Name: "CP/M", Platform: "CP/M",
		Cylinders: 77, Heads: 1, SectorsPerTrack: 26, SectorSize: 128,
		BlockSize: 1024, MaxDir: 64, BootTracks: 2, SideOrder: SideSingle,
		Skew: 6,
	},
	{
		Name: "CP/M", Platform: "CP/M",
		Cylinders: 77, Heads: 1, SectorsPerTrack: 26, SectorSize: 128,
		BlockSize: 1024, MaxDir: 64, BootTracks: 2, SideOrder: SideSingle,
		Skew: 1,
	},
}

// Entry is a file in a CP/M directory (assembled from all its extents).
type Entry struct {
	User     int
	Name     string // NAME.EXT (or U:NAME.EXT for user > 0)
	Size     uint32
	ModTime  *time.Time
	ReadOnly bool
	System   bool
	Archive  bool
	Blocks   []int
	Extents  int
	Corrupt  bool
	Error    string
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

type rawExtent struct {
	slot     int
	user     int
	baseName string
	ext      string
	extNum   int
	s1       byte
	rc       byte
	readOnly bool
	system   bool
	archive  bool
	blocks   []int
}

// Disk is an opened CP/M or CP/M-86 disk image.
type Disk struct {
	data      []byte
	spec      Spec
	skewTable []int
	container string
	label     string
	hasCPM3   bool
	entries   []Entry
	extents   map[string][]rawExtent
}

// Open parses a raw CP/M or CP/M-86 disk image.
func Open(r io.ReaderAt, size int64) (*Disk, error) {
	if size < 512 {
		return nil, fmt.Errorf("image too small: %d bytes", size)
	}
	raw := make([]byte, size)
	if _, err := r.ReadAt(raw, 0); err != nil {
		return nil, fmt.Errorf("failed to read image: %w", err)
	}
	return openRaw(raw, "IMG (raw sector image)")
}

// OpenImage parses sector data with known physical geometry (e.g. from IMD).
func OpenImage(data []byte, cylinders, heads, spt, sectorSize int, container string) (*Disk, error) {
	if len(data) == 0 || len(data) != cylinders*heads*spt*sectorSize {
		return nil, fmt.Errorf("not a CP/M image: %d bytes for %d cylinders x %d heads x %d x %d byte sectors",
			len(data), cylinders, heads, spt, sectorSize)
	}
	return probeSpecs(data, matchingSpecs(cylinders, heads, spt, sectorSize), container)
}

// OpenWithSpec opens a CP/M disk image using an explicit Spec.
func OpenWithSpec(data []byte, spec Spec, container string) (*Disk, error) {
	if spec.SectorSize <= 0 || spec.BlockSize <= 0 || spec.BlockSize%spec.SectorSize != 0 {
		return nil, fmt.Errorf("invalid CP/M spec: block size %d, sector size %d", spec.BlockSize, spec.SectorSize)
	}
	if spec.Partition == 0 {
		want := spec.Cylinders * spec.Heads * spec.SectorsPerTrack * spec.SectorSize
		if len(data) != want {
			return nil, fmt.Errorf("image size %d does not match CP/M geometry (%d bytes)", len(data), want)
		}
	}
	d := newDisk(data, spec, container)
	if _, err := d.parseDirectory(); err != nil {
		return nil, err
	}
	return d, nil
}

func openRaw(raw []byte, container string) (*Disk, error) {
	// Check for an MBR hard disk image with a Digital Research type 0xDB partition.
	if spec, ok := findDRPartition(raw); ok {
		d := newDisk(raw, spec, container)
		if _, err := d.parseDirectory(); err == nil {
			return d, nil
		}
	}
	var cands []Spec
	for _, s := range standardFormats {
		if s.Cylinders*s.Heads*s.SectorsPerTrack*s.SectorSize == len(raw) {
			cands = append(cands, s)
		}
	}
	if len(cands) == 0 {
		return nil, fmt.Errorf("not a recognized CP/M-86 image size (%d bytes)", len(raw))
	}
	return probeSpecs(raw, cands, container)
}

func matchingSpecs(cylinders, heads, spt, sectorSize int) []Spec {
	var cands []Spec
	for _, s := range standardFormats {
		if s.Cylinders == cylinders && s.Heads == heads && s.SectorsPerTrack == spt && s.SectorSize == sectorSize {
			cands = append(cands, s)
		}
	}
	return cands
}

// probeSpecs tries candidate CP/M specs and returns the best valid match.
// Candidates whose boot sector ID byte matches and whose directory contains
// live entries are preferred over empty (all-0xE5) directories.
func probeSpecs(data []byte, cands []Spec, container string) (*Disk, error) {
	if len(cands) == 0 {
		return nil, fmt.Errorf("no matching CP/M format for geometry")
	}
	var best *Disk
	bestScore := -1
	var lastErr error
	for _, s := range cands {
		idMatch := s.HasIDByte && len(data) >= 512 && data[511] == s.IDByte
		d := newDisk(data, s, container)
		liveEntries, err := d.parseDirectory()
		if err != nil {
			lastErr = err
			continue
		}
		// Reject empty all-0xE5 directories unless the boot sector matches a
		// known CP/M-86 ID byte and has non-zero boot code (to avoid matching
		// blank or non-CP/M images).
		if liveEntries == 0 {
			if !idMatch || isAllSame(data[:512]) {
				lastErr = fmt.Errorf("empty directory without CP/M-86 boot sector")
				continue
			}
		}
		score := 0
		if liveEntries > 0 {
			score += 100
		}
		if idMatch {
			score += 50
		}
		if s.SideOrder == SideOutBack || s.SideOrder == SideSingle {
			score += 10
		}
		if score > bestScore {
			best = d
			bestScore = score
		}
	}
	if best != nil {
		return best, nil
	}
	if lastErr != nil {
		return nil, fmt.Errorf("not a valid CP/M-86 filesystem: %w", lastErr)
	}
	return nil, fmt.Errorf("not a valid CP/M-86 filesystem")
}

func isAllSame(b []byte) bool {
	if len(b) == 0 {
		return true
	}
	for _, v := range b[1:] {
		if v != b[0] {
			return false
		}
	}
	return true
}

func isAllZero(b []byte) bool {
	for _, v := range b {
		if v != 0 {
			return false
		}
	}
	return true
}

// findDRPartition checks an MBR partition table for a Digital Research CP/M-86
// (type 0xDB) partition and parses its hard disk label (at partition LBA + 3).
func findDRPartition(raw []byte) (Spec, bool) {
	if len(raw) < 4*512 || raw[510] != 0x55 || raw[511] != 0xAA {
		return Spec{}, false
	}
	for i := 0; i < 4; i++ {
		e := raw[446+i*16 : 446+(i+1)*16]
		if e[4] != 0xDB {
			continue
		}
		partLBA := int(binary.LittleEndian.Uint32(e[8:12]))
		partSecs := int(binary.LittleEndian.Uint32(e[12:16]))
		labelOff := (partLBA + 3) * 512
		if partLBA == 0 || labelOff+512 > len(raw) {
			continue
		}
		lbl := raw[labelOff : labelOff+512]
		var sum uint16
		for o := 0; o < 512; o += 2 {
			sum += binary.LittleEndian.Uint16(lbl[o : o+2])
		}
		if sum != 0 {
			continue
		}
		// 15-byte DPB starts at offset 41 of the DR hard disk label.
		dpb := lbl[41 : 41+15]
		sptRecs := int(binary.LittleEndian.Uint16(dpb[0:2]))
		bsh := dpb[2]
		dsm := int(binary.LittleEndian.Uint16(dpb[5:7]))
		drm := int(binary.LittleEndian.Uint16(dpb[7:9]))
		off := int(binary.LittleEndian.Uint16(dpb[13:15]))
		if sptRecs == 0 || sptRecs%4 != 0 || bsh < 3 || bsh > 7 || dsm == 0 || drm == 0 {
			continue
		}
		spt := sptRecs / 4
		blockSize := 128 << bsh
		totalBlocks := dsm + 1
		dataSecs := (totalBlocks * blockSize) / 512
		if (off*spt+dataSecs)*512 > len(raw) {
			continue
		}
		return Spec{
			Name:            "CP/M-86",
			Platform:        "IBM PC",
			Cylinders:       off + (dataSecs+spt-1)/spt,
			Heads:           1,
			SectorsPerTrack: spt,
			SectorSize:      512,
			BlockSize:       blockSize,
			MaxDir:          drm + 1,
			BootTracks:      off,
			SideOrder:       SideSingle,
			TotalBlocks:     totalBlocks,
			Partition:       i + 1,
			PartLBA:         partLBA,
			PartSectors:     partSecs,
		}, true
	}
	return Spec{}, false
}

func buildSkewTable(spt, skew int) []int {
	tbl := make([]int, spt)
	if skew <= 1 || spt <= 1 {
		for i := range tbl {
			tbl[i] = i
		}
		return tbl
	}
	used := make([]bool, spt)
	cur := 0
	for i := 0; i < spt; i++ {
		for used[cur] {
			cur = (cur + 1) % spt
		}
		tbl[i] = cur
		used[cur] = true
		cur = (cur + skew) % spt
	}
	return tbl
}

func newDisk(data []byte, spec Spec, container string) *Disk {
	return &Disk{
		data:      data,
		spec:      spec,
		skewTable: buildSkewTable(spec.SectorsPerTrack, spec.Skew),
		container: container,
	}
}

// physSectorIndex maps a logical sector (relative to track 0 of the data area,
// i.e., after BootTracks) to its 0-based physical sector index in d.data.
func (d *Disk) physSectorIndex(dataSec int) int {
	spt := d.spec.SectorsPerTrack
	logTrk := d.spec.BootTracks + dataSec/spt
	secInTrk := d.skewTable[dataSec%spt]

	var cyl, head int
	switch d.spec.SideOrder {
	case SideSingle:
		cyl, head = logTrk, 0
	case SideOutBack:
		if logTrk < d.spec.Cylinders {
			cyl, head = logTrk, 0
		} else {
			cyl, head = 2*d.spec.Cylinders-1-logTrk, 1
		}
	case SideAlt:
		cyl, head = logTrk/d.spec.Heads, logTrk%d.spec.Heads
	case SideOutOut:
		if logTrk < d.spec.Cylinders {
			cyl, head = logTrk, 0
		} else {
			cyl, head = logTrk-d.spec.Cylinders, 1
		}
	}
	return (cyl*d.spec.Heads+head)*spt + secInTrk
}

func (d *Disk) readSector(physSec int) ([]byte, error) {
	ss := d.spec.SectorSize
	off := physSec * ss
	if off < 0 || off+ss > len(d.data) {
		return nil, fmt.Errorf("sector %d out of range", physSec)
	}
	return d.data[off : off+ss], nil
}

// readBlock reads allocation block b (0-based from start of directory).
// It also reports whether any sector in the block is marked unreadable.
func (d *Disk) readBlock(b int) ([]byte, bool, error) {
	if b < 0 || b >= d.spec.totalBlocks() {
		return nil, false, fmt.Errorf("block %d out of range (max %d)", b, d.spec.totalBlocks()-1)
	}
	secsPerBlock := d.spec.BlockSize / d.spec.SectorSize
	out := make([]byte, 0, d.spec.BlockSize)
	bad := false
	for s := 0; s < secsPerBlock; s++ {
		phys := d.physSectorIndex(b*secsPerBlock + s)
		sec, err := d.readSector(phys)
		if err != nil {
			return nil, bad, err
		}
		if util.IsBadSector(sec) {
			bad = true
		}
		out = append(out, sec...)
	}
	return out, bad, nil
}

func (d *Disk) readDirBytes() ([]byte, error) {
	dirBytes := d.spec.MaxDir * dirEntrySize
	nBlocks := d.spec.dirBlocks()
	out := make([]byte, 0, nBlocks*d.spec.BlockSize)
	for b := 0; b < nBlocks; b++ {
		blk, _, err := d.readBlock(b)
		if err != nil {
			return nil, err
		}
		out = append(out, blk...)
	}
	return out[:dirBytes], nil
}

func isValidCPMChar(c byte) bool {
	if c < 0x20 || c > 0x7E {
		return false
	}
	switch c {
	case '<', '>', '.', ',', ';', ':', '=', '?', '*', '[', ']':
		return false
	}
	return true
}

func cleanASCII(raw []byte) (string, bool) {
	buf := make([]byte, len(raw))
	for i, b := range raw {
		c := b & 0x7F
		if c < 0x20 || c > 0x7E {
			return "", false
		}
		buf[i] = c
	}
	return strings.TrimRight(string(buf), " "), true
}

func decodeBCD(b byte) (int, bool) {
	hi, lo := int(b>>4), int(b&0x0F)
	if hi > 9 || lo > 9 {
		return 0, false
	}
	return hi*10 + lo, true
}

// decodeCPM3Time decodes a 4-byte CP/M 3 timestamp (uint16 LE days since
// 1978-01-01 where 1 = 1978-01-01, BCD hour, BCD minute).
func decodeCPM3Time(b []byte) *time.Time {
	days := binary.LittleEndian.Uint16(b[0:2])
	if days == 0 {
		return nil
	}
	hour, okH := decodeBCD(b[2])
	minute, okM := decodeBCD(b[3])
	if !okH || !okM || hour > 23 || minute > 59 {
		return nil
	}
	t := time.Date(1978, 1, 1, hour, minute, 0, 0, time.UTC).AddDate(0, 0, int(days)-1)
	return &t
}

func formatFileKey(user int, base, ext string) string {
	name := base
	if ext != "" {
		name += "." + ext
	}
	if user > 0 {
		return fmt.Sprintf("%d:%s", user, name)
	}
	return name
}

// parseDirectory validates and parses all directory entries, returning the
// count of live file or label entries found.
func (d *Disk) parseDirectory() (int, error) {
	dirData, err := d.readDirBytes()
	if err != nil {
		return 0, err
	}

	totalBlocks := d.spec.totalBlocks()
	dirBlocks := d.spec.dirBlocks()
	use16 := d.spec.use16BitPtrs()
	exm := d.spec.extentMask()

	var label string
	hasLabel := false
	hasCPM3 := false
	liveCount := 0
	stamps := map[int]*time.Time{}
	order := []string{}
	byFile := map[string][]rawExtent{}
	seenEntryIdx := map[string]map[int]bool{}

	for slot := 0; slot < d.spec.MaxDir; slot++ {
		e := dirData[slot*dirEntrySize : (slot+1)*dirEntrySize]
		u := e[0]

		switch {
		case u == userDeleted:
			continue

		case u == 0xFF && e[15] == 0 && isAllZero(e[16:32]):
			// Some CP/M-86 formatters/utilities mark unused/special slots with 0xFF.
			continue

		case u == userLabel:
			if hasLabel {
				return 0, fmt.Errorf("duplicate CP/M 3 disk label at slot %d", slot)
			}
			lbl, ok := cleanASCII(e[1:12])
			if !ok {
				return 0, fmt.Errorf("invalid CP/M 3 disk label at slot %d", slot)
			}
			label = lbl
			hasLabel = true
			hasCPM3 = true
			liveCount++

		case u == userStamp:
			if slot%4 != 3 {
				return 0, fmt.Errorf("misplaced CP/M 3 timestamp entry at slot %d", slot)
			}
			hasCPM3 = true
			for k := 0; k < 3; k++ {
				sub := e[1+k*10 : 1+(k+1)*10]
				// Prefer update timestamp (bytes 4..7), fall back to create/access (bytes 0..3).
				ts := decodeCPM3Time(sub[4:8])
				if ts == nil {
					ts = decodeCPM3Time(sub[0:4])
				}
				if ts != nil {
					stamps[slot-3+k] = ts
				}
			}

		case u >= 0x10 && u <= 0x1F:
			// CP/M 3 password entry.
			if _, ok := cleanASCII(e[1:12]); !ok {
				return 0, fmt.Errorf("invalid password entry at slot %d", slot)
			}
			hasCPM3 = true

		case u <= 15:
			// Standard file extent.
			nameBuf := make([]byte, 8)
			for i := 0; i < 8; i++ {
				c := e[1+i] & 0x7F
				if !isValidCPMChar(c) {
					return 0, fmt.Errorf("invalid file name character 0x%02X at slot %d", c, slot)
				}
				nameBuf[i] = c
			}
			extBuf := make([]byte, 3)
			for i := 0; i < 3; i++ {
				c := e[9+i] & 0x7F
				if !isValidCPMChar(c) {
					return 0, fmt.Errorf("invalid extension character 0x%02X at slot %d", c, slot)
				}
				extBuf[i] = c
			}
			base := strings.TrimRight(string(nameBuf), " ")
			ext := strings.TrimRight(string(extBuf), " ")
			if base == "" && ext == "" {
				return 0, fmt.Errorf("empty file name at directory slot %d", slot)
			}
			ex := e[12]
			s1 := e[13]
			s2 := e[14]
			rc := e[15]
			if ex&0xE0 != 0 {
				return 0, fmt.Errorf("invalid extent byte 0x%02X at slot %d", ex, slot)
			}
			if s2&0x40 != 0 {
				return 0, fmt.Errorf("invalid S2 byte 0x%02X at slot %d", s2, slot)
			}
			if rc > 0x80 {
				return 0, fmt.Errorf("invalid record count %d at slot %d", rc, slot)
			}

			var blocks []int
			if use16 {
				for i := 0; i < 8; i++ {
					b := int(binary.LittleEndian.Uint16(e[16+i*2 : 18+i*2]))
					if b != 0 && (b < dirBlocks || b >= totalBlocks) {
						return 0, fmt.Errorf("block %d out of range at slot %d", b, slot)
					}
					blocks = append(blocks, b)
				}
			} else {
				for i := 0; i < 16; i++ {
					b := int(e[16+i])
					if b != 0 && (b < dirBlocks || b >= totalBlocks) {
						return 0, fmt.Errorf("block %d out of range at slot %d", b, slot)
					}
					blocks = append(blocks, b)
				}
			}

			key := formatFileKey(int(u), base, ext)
			extNum := (int(s2&0x3F) << 5) | int(ex&0x1F)
			entryIdx := extNum / (exm + 1)

			if seenEntryIdx[key] == nil {
				seenEntryIdx[key] = map[int]bool{}
				order = append(order, key)
			}
			if seenEntryIdx[key][entryIdx] {
				return 0, fmt.Errorf("duplicate extent %d for %s at slot %d", entryIdx, key, slot)
			}
			seenEntryIdx[key][entryIdx] = true

			byFile[key] = append(byFile[key], rawExtent{
				slot:     slot,
				user:     int(u),
				baseName: base,
				ext:      ext,
				extNum:   extNum,
				s1:       s1,
				rc:       rc,
				readOnly: e[9]&0x80 != 0,
				system:   e[10]&0x80 != 0,
				archive:  e[11]&0x80 != 0,
				blocks:   blocks,
			})
			liveCount++

		default:
			return 0, fmt.Errorf("invalid directory status byte 0x%02X at slot %d", u, slot)
		}
	}

	entries := make([]Entry, 0, len(order))
	for _, key := range order {
		exts := byFile[key]
		sort.Slice(exts, func(i, j int) bool { return exts[i].extNum < exts[j].extNum })
		byFile[key] = exts

		first := exts[0]
		last := exts[len(exts)-1]
		totalRecords := last.extNum*128 + int(last.rc)
		size := uint32(totalRecords * recordSize)
		if last.s1 > 0 && last.s1 < recordSize && totalRecords > 0 {
			size = uint32((totalRecords-1)*recordSize + int(last.s1))
		}

		var allBlocks []int
		corrupt := false
		var errMsg string
		for _, x := range exts {
			for _, b := range x.blocks {
				if b == 0 {
					continue
				}
				allBlocks = append(allBlocks, b)
				_, bad, err := d.readBlock(b)
				if err != nil && !corrupt {
					corrupt, errMsg = true, err.Error()
				} else if bad && !corrupt {
					corrupt, errMsg = true, "contains unreadable sector(s)"
				}
			}
		}

		var modTime *time.Time
		if ts, ok := stamps[first.slot]; ok {
			modTime = ts
		}

		entries = append(entries, Entry{
			User:     first.user,
			Name:     key,
			Size:     size,
			ModTime:  modTime,
			ReadOnly: first.readOnly,
			System:   first.system,
			Archive:  first.archive,
			Blocks:   allBlocks,
			Extents:  len(exts),
			Corrupt:  corrupt,
			Error:    errMsg,
		})
	}

	d.label = label
	d.hasCPM3 = hasCPM3
	d.entries = entries
	d.extents = byFile
	return liveCount, nil
}

// List returns the directory entries in order of appearance.
func (d *Disk) List() ([]Entry, error) {
	out := make([]Entry, len(d.entries))
	copy(out, d.entries)
	return out, nil
}

// Label returns the CP/M 3 disk label, or "" if none is present.
func (d *Disk) Label() string { return d.label }

// ReadFile returns the contents of the named file.
// Matching is case-insensitive; "0:NAME.EXT" matches user 0's "NAME.EXT".
func (d *Disk) ReadFile(path string) ([]byte, error) {
	want := strings.TrimPrefix(path, "/")
	want = strings.TrimPrefix(want, "0:")
	for _, e := range d.entries {
		if !strings.EqualFold(e.Name, want) {
			continue
		}
		return d.readFileEntry(e)
	}
	return nil, fmt.Errorf("file %q not found", path)
}

func (d *Disk) readFileEntry(e Entry) ([]byte, error) {
	exts := d.extents[e.Name]
	if len(exts) == 0 || e.Size == 0 {
		return []byte{}, nil
	}
	exm := d.spec.extentMask()
	ptrs := d.spec.ptrsPerExtent()
	byIdx := make(map[int]rawExtent, len(exts))
	maxIdx := 0
	for _, x := range exts {
		idx := x.extNum / (exm + 1)
		byIdx[idx] = x
		if idx > maxIdx {
			maxIdx = idx
		}
	}

	out := make([]byte, 0, int(e.Size)+d.spec.BlockSize)
	zeroBlock := make([]byte, d.spec.BlockSize)
	for idx := 0; idx <= maxIdx; idx++ {
		x, ok := byIdx[idx]
		if !ok {
			for i := 0; i < ptrs; i++ {
				out = append(out, zeroBlock...)
			}
			continue
		}
		for _, b := range x.blocks {
			if b == 0 {
				out = append(out, zeroBlock...)
				continue
			}
			blk, _, err := d.readBlock(b)
			if err != nil {
				return nil, err
			}
			out = append(out, blk...)
		}
	}
	if len(out) < int(e.Size) {
		return out, fmt.Errorf("file size %d exceeds allocated %d bytes", e.Size, len(out))
	}
	return out[:e.Size], nil
}

func (d *Disk) badSectors() []int {
	ss := d.spec.SectorSize
	n := len(d.data) / ss
	var bad []int
	for i := 0; i < n; i++ {
		if util.IsBadSector(d.data[i*ss : (i+1)*ss]) {
			bad = append(bad, i)
		}
	}
	return bad
}

// Warnings reports any unreadable sectors in the image.
func (d *Disk) Warnings() []string {
	bad := d.badSectors()
	if len(bad) == 0 {
		return nil
	}
	return []string{fmt.Sprintf("%d unreadable sector(s): %s", len(bad), util.FormatRanges(bad))}
}

func (d *Disk) unformatted() (int64, string) {
	if d.spec.Partition != 0 || d.spec.SectorSize != 512 {
		return 0, ""
	}
	t, h, spt := d.spec.Cylinders, d.spec.Heads, d.spec.SectorsPerTrack
	switch {
	case spt <= 9 && t == 40:
		return int64(250000 * h), fmt.Sprintf("%d KB", 250*h)
	case spt <= 9 && t == 80:
		return int64(500000 * h), map[int]string{1: "500 KB", 2: "1 MB"}[h]
	case spt == 15 && t == 80:
		return 1666666, "1.6 MB"
	case spt == 18 && t == 80:
		return 2000000, "2 MB"
	}
	return 0, ""
}

// Usage computes sector usage from the directory allocation table.
func (d *Disk) Usage() (*DiskUsage, error) {
	usedBlocks := make(map[int]bool)
	dirBlocks := d.spec.dirBlocks()
	for b := 0; b < dirBlocks; b++ {
		usedBlocks[b] = true
	}
	for _, e := range d.entries {
		for _, b := range e.Blocks {
			usedBlocks[b] = true
		}
	}

	totalBlocks := d.spec.totalBlocks()
	freeBlocks := totalBlocks - len(usedBlocks)
	if freeBlocks < 0 {
		freeBlocks = 0
	}

	secsPerBlock := d.spec.BlockSize / d.spec.SectorSize
	totalSectors := d.spec.Cylinders * d.spec.Heads * d.spec.SectorsPerTrack
	if d.spec.Partition != 0 {
		totalSectors = d.spec.PartSectors
	}
	freeSectors := freeBlocks * secsPerBlock
	if freeSectors > totalSectors {
		freeSectors = totalSectors
	}
	inUseSectors := totalSectors - freeSectors

	sz := int64(d.spec.SectorSize)
	totalBytes := int64(totalSectors) * sz
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
		SectorSize:          d.spec.SectorSize,
		TotalSectors:        totalSectors,
		InUseSectors:        inUseSectors,
		FreeSectors:         freeSectors,
		TotalBytes:          totalBytes,
		InUseBytes:          int64(inUseSectors) * sz,
		FreeBytes:           int64(freeSectors) * sz,
	}, nil
}

// Format describes the image format and geometry.
func (d *Disk) Format() *util.FormatInfo {
	image := d.container
	if d.spec.Partition != 0 {
		image = fmt.Sprintf("IMG (raw disk image, MBR partition %d)", d.spec.Partition)
	}
	fs := d.spec.Name
	if d.hasCPM3 && fs == "CP/M-86" {
		fs = "CP/M-86 (BDOS 3.x)"
	}
	return &util.FormatInfo{
		Platform:   d.spec.Platform,
		Image:      image,
		Filesystem: fs,
		Tracks:     d.spec.Cylinders,
		Heads:      d.spec.Heads,
		Sectors:    fmt.Sprintf("%d", d.spec.SectorsPerTrack),
		SectorSize: d.spec.SectorSize,
	}
}

func attrString(e Entry) string {
	s := []byte("---")
	if e.ReadOnly {
		s[0] = 'r'
	}
	if e.System {
		s[1] = 's'
	}
	if e.Archive {
		s[2] = 'a'
	}
	return string(s)
}

// Dump writes a detailed description of the disk.
func (d *Disk) Dump(w io.Writer) error {
	fmt.Fprintf(w, "%s Disk Image Dump\n", d.spec.Name)
	if d.spec.Partition != 0 {
		fmt.Fprintf(w, "  MBR partition: %d (LBA %d, %d sectors)\n", d.spec.Partition, d.spec.PartLBA, d.spec.PartSectors)
	}
	fmt.Fprintf(w, "  Geometry: %d tracks, %d heads, %d sectors/track, %d bytes/sector (%s)\n",
		d.spec.Cylinders, d.spec.Heads, d.spec.SectorsPerTrack, d.spec.SectorSize, d.spec.SideOrder)
	if d.spec.Skew > 1 {
		fmt.Fprintf(w, "  Sector skew: %d\n", d.spec.Skew)
	}
	fmt.Fprintf(w, "  DPB: Block size %d, Max dir %d, Reserved tracks %d, DSM %d, EXM %d\n",
		d.spec.BlockSize, d.spec.MaxDir, d.spec.BootTracks, d.spec.totalBlocks()-1, d.spec.extentMask())
	if d.spec.SectorSize == 512 && len(d.data) >= 512 && d.spec.Partition == 0 {
		fmt.Fprintf(w, "  Boot sector ID byte (0x1FF): 0x%02X\n", d.data[511])
	}
	if d.label != "" {
		fmt.Fprintf(w, "  Volume label: %q\n", d.label)
	}
	if u, err := d.Usage(); err == nil {
		fmt.Fprintf(w, "  In-use: %d sectors, Free: %d sectors, Total: %d sectors\n",
			u.InUseSectors, u.FreeSectors, u.TotalSectors)
	}
	if bad := d.badSectors(); len(bad) > 0 {
		fmt.Fprintf(w, "  Unreadable sectors (%d): %s\n", len(bad), util.FormatRanges(bad))
	}
	fmt.Fprintf(w, "  Directory Entries (%d), attributes: r=read-only s=system a=archive:\n", len(d.entries))
	for _, e := range d.entries {
		ts := ""
		if e.ModTime != nil {
			ts = "  " + e.ModTime.Format("2006-01-02 15:04")
		}
		corrupt := ""
		if e.Corrupt {
			corrupt = fmt.Sprintf(" [CORRUPT: %s]", e.Error)
		}
		fmt.Fprintf(w, "    %s %8d bytes  u%-2d %-14s  extents:%d blocks:%d%s%s\n",
			attrString(e), e.Size, e.User, e.Name, e.Extents, len(e.Blocks), ts, corrupt)
	}
	return nil
}
