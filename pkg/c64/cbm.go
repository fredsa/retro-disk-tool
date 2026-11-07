package c64

import (
	"encoding/binary"
	"fmt"
	"io"
	"math/bits"
	"strings"
	"time"

	"github.com/fredsa/retro-disk-tool/cmd/rdt/pkg/util"
)

// cbmLayout describes a CBM DOS disk geometry and where the DOS keeps its
// header, BAM and directory.
type cbmLayout struct {
	drive        string // "1541", "1581", "DFI"
	tracks       int
	headerTrack  int // header sector, if not sector 0 of the directory track
	headerSector int
	firstDirSec  int // the drive always starts the directory here (on the directory track); 0 = follow the header's link
	spt          func(track int) int
	dirTrack     int
	nameOff      int // disk name, ID and DOS type offsets in the header sector
	idOff        int
	dosOff       int
	heads        int    // physical heads (for display)
	sectorsDesc  string // sectors per track (for display)
	physicalSPT  int    // 256 byte sectors per physical track (for display), 0 = variable
	unformatted  int64
	unformDesc   string
	formattedKiB string
}

func d64SPT(track int) int {
	switch {
	case track <= 17:
		return 21
	case track <= 24:
		return 19
	case track <= 30:
		return 18
	}
	return 17
}

func d64Layout(tracks int) *cbmLayout {
	l := &cbmLayout{
		drive: "1541", tracks: tracks, spt: d64SPT, dirTrack: 18, firstDirSec: 1,
		nameOff: 0x90, idOff: 0xA2, dosOff: 0xA5, heads: 1, sectorsDesc: "17-21",
	}
	if tracks == 35 {
		l.unformatted, l.unformDesc = 250000, "250 KB"
	}
	return l
}

func d81Layout() *cbmLayout {
	return &cbmLayout{
		drive: "1581", tracks: 80, spt: func(int) int { return 40 }, dirTrack: 40, firstDirSec: 3,
		nameOff: 0x04, idOff: 0x16, dosOff: 0x19, heads: 2, sectorsDesc: "20", physicalSPT: 20,
		unformatted: 1000000, unformDesc: "1 MB",
	}
}

// CBMDisk is a CBM DOS disk (1541 D64, 1581 D81, or a decoded G64).
type CBMDisk struct {
	data      []byte // sector data in track/sector order
	errs      []byte // per sector error codes (nil if none), 0x01 = OK
	layout    *cbmLayout
	offsets   []int // offsets[t] = byte offset of track t (1-based)
	container string
	extBAM    int    // index into extBAMLayouts of the 40 track BAM extension, or -1
	errSource string // how sector errors were found
	g64       *g64Info
	dfiBAM    int // byte offset of the DreamLoad allocation bitmap
	dfiVer    uint32
}

// D64Disk is a 1541 disk image.
type D64Disk = CBMDisk

// DiskHeaderInfo is a disk's name, ID and DOS type.
type DiskHeaderInfo struct {
	Name    string
	ID      string
	DOSType string
}

// DiskEntry is a directory entry.
type DiskEntry struct {
	Name        string
	Type        string
	Sectors     int
	Bytes       int
	Locked      bool
	Closed      bool
	IsDir       bool // 1581 partition formatted as a sub-directory
	Corrupt     bool
	Error       string
	Native      []util.Glyph
	startTrack  int
	startSector int
}

// ListEntry is a file listed by a C64 image or archive.
type ListEntry = diskEntryCompat

// diskEntryCompat satisfies disk.Entry
type diskEntryCompat struct {
	Name    string
	IsDir   bool
	Size    uint32
	ModTime *time.Time
	Corrupt bool
	Error   string
	Native  []util.Glyph
}

// DiskUsageInfo describes sector usage.
type DiskUsageInfo struct {
	FormattedCapacity   int64
	FormattedDesc       string
	UnformattedCapacity int64
	UnformattedDesc     string
	SectorSize          int
	TotalSectors        int
	InUseSectors        int
	FreeSectors         int
	BAMSectors          int
	ExtraSectors        int // sectors on tracks the BAM does not cover
	TotalBytes          int64
	InUseBytes          int64
	FreeBytes           int64
	BAMBytes            int64
	ExtraBytes          int64
	HasUsage            bool
}

// OpenD64 parses a D64 image: 35, 40 or 42 tracks, optionally followed by
// one error byte per sector.
func OpenD64(r io.ReaderAt, size int64) (*D64Disk, error) {
	for _, tracks := range []int{35, 40, 42} {
		l := d64Layout(tracks)
		n := l.totalSectors()
		if size != int64(n*256) && size != int64(n*257) {
			continue
		}
		data := make([]byte, size)
		if _, err := r.ReadAt(data, 0); err != nil {
			return nil, fmt.Errorf("failed to read D64 file: %w", err)
		}
		var errs []byte
		if size == int64(n*257) {
			data, errs = data[:n*256], data[n*256:]
		}
		desc := fmt.Sprintf("D64 (1541 disk image, %d tracks", tracks)
		if errs != nil {
			desc += ", with error table"
		}
		return newCBMDisk(data, errs, l, desc+")"), nil
	}
	return nil, fmt.Errorf("invalid D64 disk size: %d bytes", size)
}

// OpenDFI parses a DreamLoad file archive: tracks of 256 sectors, a header
// in track 1 sector 0, an allocation bitmap and 1541 style directories.
func OpenDFI(r io.ReaderAt, size int64) (*CBMDisk, error) {
	if size < 0x10000 || size%0x10000 != 0 {
		return nil, fmt.Errorf("invalid DFI size %d: not a multiple of 65536 byte tracks", size)
	}
	data := make([]byte, size)
	if _, err := r.ReadAt(data, 0); err != nil {
		return nil, fmt.Errorf("failed to read DFI file: %w", err)
	}
	if string(data[:24]) != "\x00DREAMLOAD FILE ARCHIVE\x00" {
		return nil, fmt.Errorf("invalid DFI signature")
	}
	ver := binary.LittleEndian.Uint32(data[0x18:])
	tracks := int(binary.LittleEndian.Uint32(data[0x1C:]))
	if ver>>16 != 1 {
		return nil, fmt.Errorf("unsupported DFI version %d.%d", ver>>16, ver&0xFFFF)
	}
	if int64(tracks)*0x10000 != size || tracks > 255 {
		return nil, fmt.Errorf("DFI size mismatch: header says %d tracks, file has %d", tracks, size/0x10000)
	}
	ht, hs, bt, bs := int(data[0x20]), int(data[0x21]), int(data[0x22]), int(data[0x23])
	bam := (bt-1)*0x10000 + bs*256
	if ht < 1 || ht > tracks || bt < 1 || bam+8192 > len(data) {
		return nil, fmt.Errorf("invalid DFI header: root directory %d/%d, BAM %d/%d", ht, hs, bt, bs)
	}
	l := &cbmLayout{
		drive: "DFI", tracks: tracks, spt: func(int) int { return 256 }, headerTrack: ht, headerSector: hs,
		nameOff: 0x90, idOff: 0xA2, dosOff: 0xA5, heads: 1, sectorsDesc: "256",
	}
	d := newCBMDisk(data, nil, l, fmt.Sprintf("DFI (DreamLoad file archive, version %d.%d, %d tracks of 256 sectors)", ver>>16, ver&0xFFFF, tracks))
	d.dfiBAM, d.dfiVer = bam, ver
	if d.headerSector()[2] != 'M' {
		return nil, fmt.Errorf("invalid DFI root directory header at %d/%d", ht, hs)
	}
	return d, nil
}

// OpenD81 parses a D81 (1581) image, optionally followed by one error byte
// per sector.
func OpenD81(r io.ReaderAt, size int64) (*CBMDisk, error) {
	l := d81Layout()
	n := l.totalSectors()
	if size != int64(n*256) && size != int64(n*257) {
		return nil, fmt.Errorf("invalid D81 disk size: %d bytes (want %d or %d)", size, n*256, n*257)
	}
	data := make([]byte, size)
	if _, err := r.ReadAt(data, 0); err != nil {
		return nil, fmt.Errorf("failed to read D81 file: %w", err)
	}
	var errs []byte
	desc := "D81 (1581 disk image, 80 tracks"
	if size == int64(n*257) {
		data, errs = data[:n*256], data[n*256:]
		desc += ", with error table"
	}
	return newCBMDisk(data, errs, l, desc+")"), nil
}

func (l *cbmLayout) totalSectors() int {
	n := 0
	for t := 1; t <= l.tracks; t++ {
		n += l.spt(t)
	}
	return n
}

func newCBMDisk(data, errs []byte, l *cbmLayout, container string) *CBMDisk {
	d := &CBMDisk{data: data, errs: errs, layout: l, container: container, offsets: make([]int, l.tracks+2),
		errSource: "have read errors in the error table"}
	for t := 1; t <= l.tracks+1; t++ {
		if t > 1 {
			d.offsets[t] = d.offsets[t-1] + l.spt(t-1)*256
		}
	}
	d.extBAM = -1
	if l.drive == "1541" && l.tracks > 35 {
		d.extBAM = d.detectExtBAM()
	}
	return d
}

func (d *CBMDisk) sectorIndex(track, sector int) (int, error) {
	if track < 1 || track > d.layout.tracks {
		return 0, fmt.Errorf("invalid track: %d", track)
	}
	if sector < 0 || sector >= d.layout.spt(track) {
		return 0, fmt.Errorf("invalid sector %d for track %d", sector, track)
	}
	return d.offsets[track]/256 + sector, nil
}

func (d *CBMDisk) sector(track, sector int) ([]byte, error) {
	i, err := d.sectorIndex(track, sector)
	if err != nil {
		return nil, err
	}
	return d.data[i*256 : (i+1)*256], nil
}

func (d *CBMDisk) sectorErrorCode(track, sector int) byte {
	i, err := d.sectorIndex(track, sector)
	if err != nil || d.errs == nil || d.errs[i] == 0x00 {
		return 0x01
	}
	return d.errs[i]
}

// headerSector returns the sector holding the disk name and the link to
// the first directory sector.
func (d *CBMDisk) headerSector() []byte {
	if d.layout.headerTrack > 0 {
		s, _ := d.sector(d.layout.headerTrack, d.layout.headerSector)
		return s
	}
	s, _ := d.sector(d.layout.dirTrack, 0)
	return s
}

// rootDir returns the first root directory sector. Like the drive, a 1541
// or 1581 image always starts at its fixed sector (18/1, 40/3), whatever the
// header's link says (copy protections sometimes change it).
func (d *CBMDisk) rootDir() (int, int) {
	if d.layout.firstDirSec > 0 {
		return d.layout.dirTrack, d.layout.firstDirSec
	}
	h := d.headerSector()
	return int(h[0]), int(h[1])
}

// headerLinkMismatch describes a header directory link that differs from
// where the drive starts reading the directory, or "".
func (d *CBMDisk) headerLinkMismatch() string {
	h := d.headerSector()
	t, s := d.rootDir()
	if int(h[0]) == t && int(h[1]) == s {
		return ""
	}
	return fmt.Sprintf("the header's directory link (%d/%d) is not used by the drive; the directory starts at %d/%d", h[0], h[1], t, s)
}

func trimPadding(b []byte) []byte {
	n := len(b)
	for n > 0 && b[n-1] == 0xA0 {
		n--
	}
	return b[:n]
}

// Header returns the disk name, ID and DOS type.
func (d *CBMDisk) Header() *DiskHeaderInfo {
	h := d.headerSector()
	l := d.layout
	return &DiskHeaderInfo{
		Name:    DecodeBytes(trimPadding(h[l.nameOff : l.nameOff+16])),
		ID:      DecodeBytes(h[l.idOff : l.idOff+2]),
		DOSType: DecodeBytes(h[l.dosOff : l.dosOff+2]),
	}
}

// trackRange is the range of tracks a directory's files must stay within
// (the whole disk, or a 1581 sub-directory's partition).
type trackRange struct{ lo, hi int }

// readDir reads the directory chain starting at track/sector.
func (d *CBMDisk) readDir(track, sector int) []DiskEntry {
	return d.readDirIn(track, sector, trackRange{1, d.layout.tracks})
}

func (d *CBMDisk) readDirIn(track, sector int, within trackRange) []DiskEntry {
	var entries []DiskEntry
	visited := map[[2]int]bool{}
	for track != 0 {
		cur := [2]int{track, sector}
		if visited[cur] {
			break
		}
		visited[cur] = true
		sec, err := d.sector(track, sector)
		if err != nil {
			break
		}
		secErr := d.sectorErrorCode(track, sector)
		track, sector = int(sec[0]), int(sec[1])
		for i := 0; i < 8; i++ {
			if e, ok := d.parseEntry(sec[i*32:(i+1)*32], cur, secErr, within); ok {
				entries = append(entries, e)
			}
		}
	}
	return entries
}

func (d *CBMDisk) parseEntry(b []byte, at [2]int, secErr byte, within trackRange) (DiskEntry, bool) {
	rawType := b[2]
	ftype := rawType & 0x07
	if ftype == 0 {
		return DiskEntry{}, false // deleted or empty
	}
	rawName := trimPadding(b[5:21])
	e := DiskEntry{
		Name:        DecodeBytes(rawName),
		Sectors:     int(binary.LittleEndian.Uint16(b[30:32])),
		Locked:      rawType&0x40 != 0,
		Closed:      rawType&0x80 != 0,
		Native:      petsciiQuotedGlyphs(rawName),
		startTrack:  int(b[3]),
		startSector: int(b[4]),
	}
	if ftype == 6 && d.layout.drive == "DFI" {
		// DreamLoad sub-directory: points to a directory header.
		e.Type = "DIR"
		h, err := d.sector(e.startTrack, e.startSector)
		e.IsDir = err == nil && h[2] == 'M'
		if !e.IsDir {
			e.Corrupt, e.Error = true, fmt.Sprintf("invalid sub-directory header at %d/%d", e.startTrack, e.startSector)
		}
		return e, true
	}
	var reasons []string
	typeStr, ok := fileTypes[ftype]
	if !ok {
		typeStr = "???"
		reasons = append(reasons, fmt.Sprintf("invalid file type 0x%02X", rawType))
	}
	e.Type = typeStr
	if secErr != 0x01 {
		desc, has := d64ErrorCodes[secErr]
		if !has {
			desc = fmt.Sprintf("error 0x%02X", secErr)
		}
		reasons = append(reasons, fmt.Sprintf("track %d, sector %d: %s", at[0], at[1], desc))
	}
	if ftype == 5 {
		// 1581 partition: a contiguous area, possibly a sub-directory.
		e.IsDir = d.isSubdir(e)
		e.Bytes = e.Sectors * 256
		if _, err := d.sectorIndex(e.startTrack, e.startSector); err != nil {
			reasons = append(reasons, "invalid partition start: "+err.Error())
		}
	} else if e.startTrack != 0 || e.Sectors > 0 {
		content, err := d.readChain(e.startTrack, e.startSector)
		e.Bytes = len(content)
		blocks := (len(content) + 253) / 254
		switch {
		case err != nil:
			reasons = append(reasons, err.Error())
		case ftype != 4 && e.Sectors > 0 && blocks < e.Sectors:
			// REL files also count side sectors; 0 block entries are
			// typically directory art pointing into another file.
			reasons = append(reasons, fmt.Sprintf("block chain ends after %d of %d blocks", blocks, e.Sectors))
		}
		if t := d.chainOutside(e.startTrack, e.startSector, within); t != 0 {
			reasons = append(reasons, fmt.Sprintf("block chain leaves its partition (tracks %d-%d) at track %d", within.lo, within.hi, t))
		}
	}
	if len(reasons) > 0 {
		e.Corrupt, e.Error = true, strings.Join(reasons, "; ")
	}
	return e, true
}

// isSubdir reports whether a 1581 partition is formatted as a sub-directory:
// it starts at sector 0 of a track, spans whole tracks, and its first
// sector is a directory header linking to its third sector.
func (d *CBMDisk) isSubdir(e DiskEntry) bool {
	if d.layout.drive != "1581" || e.startSector != 0 || e.Sectors < 120 || e.Sectors%40 != 0 {
		return false
	}
	h, err := d.sector(e.startTrack, 0)
	return err == nil && int(h[0]) == e.startTrack && h[1] == 3 && h[2] == 'D'
}

// chainOutside returns the first track of a file's chain outside the given
// range, or 0.
func (d *CBMDisk) chainOutside(track, sector int, within trackRange) int {
	visited := map[[2]int]bool{}
	for track != 0 && !visited[[2]int{track, sector}] {
		visited[[2]int{track, sector}] = true
		if track < within.lo || track > within.hi {
			return track
		}
		blk, err := d.sector(track, sector)
		if err != nil {
			return 0
		}
		track, sector = int(blk[0]), int(blk[1])
	}
	return 0
}

// subdirRange returns the tracks of a sub-directory partition.
func subdirRange(e DiskEntry) trackRange {
	return trackRange{e.startTrack, e.startTrack + e.Sectors/40 - 1}
}

// readChain follows a file's track/sector chain.
func (d *CBMDisk) readChain(track, sector int) ([]byte, error) {
	var out []byte
	visited := map[[2]int]bool{}
	for track != 0 {
		if visited[[2]int{track, sector}] {
			return out, fmt.Errorf("cyclic block chain detected at %d/%d", track, sector)
		}
		visited[[2]int{track, sector}] = true
		blk, err := d.sector(track, sector)
		if err != nil {
			return out, err
		}
		if code := d.sectorErrorCode(track, sector); code != 0x01 {
			desc := d64ErrorCodes[code]
			return out, fmt.Errorf("track %d, sector %d: %s", track, sector, desc)
		}
		next, nextS := int(blk[0]), int(blk[1])
		if next == 0 {
			// Last block: the second byte is the index of the last byte used.
			if nextS >= 2 {
				out = append(out, blk[2:nextS+1]...)
			} else {
				out = append(out, blk[2:]...)
			}
			return out, nil
		}
		out = append(out, blk[2:]...)
		track, sector = next, nextS
	}
	return out, nil
}

// dirStart returns the first directory sector of the root or of the
// sub-directory at path.
func (d *CBMDisk) dirStart(path string) (int, int, trackRange, error) {
	track, sector := d.rootDir()
	within := trackRange{1, d.layout.tracks}
	for _, part := range strings.FieldsFunc(path, func(r rune) bool { return r == '/' }) {
		var found *DiskEntry
		for _, e := range d.readDirIn(track, sector, within) {
			if strings.EqualFold(e.Name, part) {
				found = &e
				break
			}
		}
		if found == nil {
			return 0, 0, within, fmt.Errorf("%q not found", path)
		}
		if !found.IsDir {
			return 0, 0, within, fmt.Errorf("%q is not a directory", path)
		}
		hdr, _ := d.sector(found.startTrack, found.startSector)
		track, sector = int(hdr[0]), int(hdr[1])
		if found.Type == "CBM" {
			within = subdirRange(*found)
		}
	}
	return track, sector, within, nil
}

// ReadDirectory returns the root directory entries.
func (d *CBMDisk) ReadDirectory() ([]DiskEntry, error) {
	return d.readDir(d.rootDir()), nil
}

func toCompat(entries []DiskEntry) []diskEntryCompat {
	res := make([]diskEntryCompat, len(entries))
	for i, e := range entries {
		res[i] = diskEntryCompat{Name: e.Name, IsDir: e.IsDir, Size: uint32(e.Bytes), Corrupt: e.Corrupt, Error: e.Error, Native: e.Native}
	}
	return res
}

// List returns the root directory entries.
func (d *CBMDisk) List() ([]diskEntryCompat, error) { return d.ListDir("") }

// ListDir returns the entries of the directory at path ("" for the root).
func (d *CBMDisk) ListDir(path string) ([]diskEntryCompat, error) {
	t, s, within, err := d.dirStart(path)
	if err != nil {
		return nil, err
	}
	return toCompat(d.readDirIn(t, s, within)), nil
}

// ReadFile returns the contents of a file ("NAME" or "DIR/NAME"). A
// partition that is not a sub-directory reads as its raw sectors.
func (d *CBMDisk) ReadFile(path string) ([]byte, error) {
	dir, name := "", strings.TrimPrefix(path, "/")
	if i := strings.LastIndex(name, "/"); i >= 0 {
		dir, name = name[:i], name[i+1:]
	}
	t, s, within, err := d.dirStart(dir)
	if err != nil {
		return nil, err
	}
	for _, e := range d.readDirIn(t, s, within) {
		if !strings.EqualFold(e.Name, name) {
			continue
		}
		if e.IsDir {
			return nil, fmt.Errorf("%q is a directory", path)
		}
		if e.Type == "CBM" {
			i, err := d.sectorIndex(e.startTrack, e.startSector)
			if err != nil || (i+e.Sectors)*256 > len(d.data) {
				return nil, fmt.Errorf("partition %q extends beyond the disk", path)
			}
			return d.data[i*256 : (i+e.Sectors)*256], nil
		}
		return d.readChain(e.startTrack, e.startSector)
	}
	return nil, fmt.Errorf("file %q not found", path)
}

// bamEntry returns the free sector count and allocation bitmap for a track,
// or ok=false if the BAM does not cover the track.
func (d *CBMDisk) bamEntry(track int) (free int, bitmap []byte, ok bool) {
	switch d.layout.drive {
	case "DFI":
		bm := d.data[d.dfiBAM+(track-1)*32 : d.dfiBAM+track*32]
		used := 0
		for _, b := range bm {
			used += bits.OnesCount8(b)
		}
		return 256 - used, bm, true
	case "1581":
		bam, _ := d.sector(40, 1+(track-1)/40)
		e := bam[0x10+6*((track-1)%40):]
		return int(e[0]), e[1:6], true
	default:
		bam := d.headerSector()
		switch {
		case track <= 35:
			e := bam[4*track:]
			return int(e[0]), e[1:4], true
		case track <= 40 && d.extBAM >= 0:
			e := bam[extBAMLayouts[d.extBAM].off+4*(track-36):]
			return int(e[0]), e[1:4], true
		}
	}
	return 0, nil, false
}

// extBAMLayouts are the known places for tracks 36-40 BAM entries.
var extBAMLayouts = []struct {
	name string
	off  int
}{{"SpeedDOS", 0xC0}, {"DolphinDOS", 0xAC}, {"$B0", 0xB0}}

// detectExtBAM identifies a 40 track BAM extension. Several DOS extensions
// put tracks 36-40 entries in different places, and more than one place
// may hold plausible data, so each candidate is checked against the
// sectors the files actually use: it must be self-consistent (free counts
// match bitmaps), must not mark a used sector free, must mark some sector
// free, and should match the files' usage on as many tracks as possible.
func (d *CBMDisk) detectExtBAM() int {
	used := d.usedSectors(36, 40)
	bam := d.headerSector()
	best, bestScore := -1, 0
	for i, c := range extBAMLayouts {
		score, anyFree := 0, false
		for t := 36; t <= 40; t++ {
			e := bam[c.off+4*(t-36):]
			free := 0
			for s := 0; s < 17; s++ {
				if e[1+s/8]>>(s%8)&1 == 1 {
					free++
					if used[t][s] {
						score = -1 // a used sector marked free
					}
				}
			}
			if score < 0 || e[3]&0xFE != 0 || int(e[0]) != free {
				score = -1
				break
			}
			anyFree = anyFree || free > 0
			if free == 17-len(used[t]) {
				score++
			}
		}
		// An all zero area (no free sectors) is taken as no extension.
		if score > bestScore && anyFree {
			best, bestScore = i, score
		}
	}
	return best
}

// usedSectors returns the sectors on tracks lo-hi used by files in the root
// directory (following their chains as far as they can be read).
func (d *CBMDisk) usedSectors(lo, hi int) map[int]map[int]bool {
	used := map[int]map[int]bool{}
	for t := lo; t <= hi; t++ {
		used[t] = map[int]bool{}
	}
	visitedDir := map[[2]int]bool{}
	t0, s0 := d.rootDir()
	for t, s := t0, s0; t != 0 && !visitedDir[[2]int{t, s}]; {
		visitedDir[[2]int{t, s}] = true
		sec, err := d.sector(t, s)
		if err != nil {
			break
		}
		for i := 0; i < 8; i++ {
			e := sec[i*32 : (i+1)*32]
			if e[2]&0x07 == 0 {
				continue
			}
			seen := map[[2]int]bool{}
			for ct, cs := int(e[3]), int(e[4]); ct != 0 && !seen[[2]int{ct, cs}]; {
				seen[[2]int{ct, cs}] = true
				if ct >= lo && ct <= hi {
					used[ct][cs] = true
				}
				blk, err := d.sector(ct, cs)
				if err != nil {
					break
				}
				ct, cs = int(blk[0]), int(blk[1])
			}
		}
		t, s = int(sec[0]), int(sec[1])
	}
	return used
}

// Usage computes sector usage from the BAM. Sectors on tracks the BAM does
// not cover (e.g. tracks 36-40 without a BAM extension) are reported as
// extra, so the total always matches the image.
func (d *CBMDisk) Usage() (*DiskUsageInfo, error) {
	l := d.layout
	total := l.totalSectors()
	free, extra := 0, 0
	for t := 1; t <= l.tracks; t++ {
		n, _, ok := d.bamEntry(t)
		switch {
		case !ok:
			extra += l.spt(t)
		case t != l.dirTrack:
			free += n
		}
	}
	system := 0
	if l.dirTrack > 0 {
		system = l.spt(l.dirTrack)
	}
	inUse := total - system - free - extra
	totalBytes := int64(total) * 256
	return &DiskUsageInfo{
		FormattedCapacity:   totalBytes,
		FormattedDesc:       fmt.Sprintf("%d KiB", totalBytes/1024),
		UnformattedCapacity: l.unformatted,
		UnformattedDesc:     l.unformDesc,
		SectorSize:          256,
		TotalSectors:        total,
		InUseSectors:        inUse,
		FreeSectors:         free,
		BAMSectors:          system,
		ExtraSectors:        extra,
		TotalBytes:          totalBytes,
		InUseBytes:          int64(inUse) * 256,
		FreeBytes:           int64(free) * 256,
		BAMBytes:            int64(system) * 256,
		ExtraBytes:          int64(extra) * 256,
		HasUsage:            true,
	}, nil
}

// Warnings describes a header that is not a valid BAM, an ignored
// directory link, and sectors with read errors.
func (d *CBMDisk) Warnings() []string {
	var w []string
	h := d.headerSector()
	switch d.layout.drive {
	case "1541":
		if h[2] != 'A' && h[2] != 'B' {
			w = append(w, fmt.Sprintf("track 18 sector 0 does not look like a 1541 BAM (DOS version byte $%02X); this may be a data disk without a CBM directory", h[2]))
		}
	case "1581":
		if h[2] != 'D' {
			w = append(w, fmt.Sprintf("track 40 sector 0 does not look like a 1581 header (DOS version byte $%02X); this may be a data disk without a CBM directory", h[2]))
		}
	}
	if msg := d.headerLinkMismatch(); msg != "" && len(w) == 0 {
		w = append(w, msg)
	}
	return append(w, d.errorWarnings()...)
}

// errorWarnings describes sectors marked bad in the error table.
func (d *CBMDisk) errorWarnings() []string {
	var bad []string
	for t := 1; t <= d.layout.tracks; t++ {
		for s := 0; s < d.layout.spt(t); s++ {
			if code := d.sectorErrorCode(t, s); code != 0x01 {
				bad = append(bad, fmt.Sprintf("%d/%d", t, s))
			}
		}
	}
	if len(bad) == 0 {
		return nil
	}
	if len(bad) > 8 {
		bad = append(bad[:8], "...")
	}
	return []string{fmt.Sprintf("%d sectors %s (%s)", d.countErrors(), d.errSource, strings.Join(bad, ", "))}
}

func (d *CBMDisk) countErrors() int {
	n := 0
	for i := range d.layout.totalSectors() {
		if d.errs != nil && d.errs[i] != 0x00 && d.errs[i] != 0x01 {
			n++
		}
	}
	return n
}

func (d *CBMDisk) filesystem() string {
	if d.layout.drive == "DFI" {
		return "DreamLoad"
	}
	fs := "CBM DOS"
	switch {
	case d.extBAM < 0:
	case extBAMLayouts[d.extBAM].name[0] == '$':
		fs += fmt.Sprintf(" (40 track BAM at %s)", extBAMLayouts[d.extBAM].name)
	default:
		fs += fmt.Sprintf(" (%s 40 track BAM)", extBAMLayouts[d.extBAM].name)
	}
	return fs
}

// Format describes the image format and geometry.
func (d *CBMDisk) Format() *util.FormatInfo {
	return &util.FormatInfo{
		Platform:   "Commodore 64",
		Image:      d.container,
		Filesystem: d.filesystem(),
		Tracks:     d.layout.tracks,
		Heads:      d.layout.heads,
		Sectors:    d.layout.sectorsDesc,
		SectorSize: 256,
	}
}

func (d *CBMDisk) dumpDir(w io.Writer, track, sector int, within trackRange, indent string, depth int) {
	for _, e := range d.readDirIn(track, sector, within) {
		lock := " "
		if e.Locked {
			lock = "<"
		}
		corrupt := ""
		if e.Corrupt {
			corrupt = fmt.Sprintf(" [CORRUPT: %s]", e.Error)
		}
		kind := ""
		if e.IsDir {
			kind = " (sub-directory)"
		}
		fmt.Fprintf(w, "%s%-18q %-3s%s %4d sectors (%d bytes) at %d/%d%s%s\n",
			indent, e.Name, e.Type, lock, e.Sectors, e.Bytes, e.startTrack, e.startSector, kind, corrupt)
		if e.IsDir && depth < 8 {
			hdr, _ := d.sector(e.startTrack, e.startSector)
			sub := within
			if e.Type == "CBM" {
				sub = subdirRange(e)
			}
			d.dumpDir(w, int(hdr[0]), int(hdr[1]), sub, indent+"  ", depth+1)
		}
	}
}

// Dump writes a detailed description of the disk.
func (d *CBMDisk) Dump(w io.Writer) error {
	h := d.Header()
	fmt.Fprintf(w, "Commodore %s Disk Image Dump\n", d.layout.drive)
	fmt.Fprintf(w, "  Image: %s\n", d.container)
	fmt.Fprintf(w, "  Filesystem: %s\n", d.filesystem())
	fmt.Fprintf(w, "  Disk Name: %q, ID: %q, DOS Type: %q\n", h.Name, h.ID, h.DOSType)
	fmt.Fprintf(w, "  Tracks: %d, Total Sectors: %d (%d bytes), Directory track: %d\n",
		d.layout.tracks, d.layout.totalSectors(), len(d.data), d.layout.dirTrack)
	if d.g64 != nil {
		d.dumpG64(w)
	}
	if d.errs != nil {
		fmt.Fprintf(w, "  Sector errors:\n")
		for t := 1; t <= d.layout.tracks; t++ {
			for s := 0; s < d.layout.spt(t); s++ {
				if code := d.sectorErrorCode(t, s); code != 0x01 {
					fmt.Fprintf(w, "    Track %2d, Sector %2d: Error code 0x%02X (%s)\n", t, s, code, d64ErrorCodes[code])
				}
			}
		}
	}
	if u, err := d.Usage(); err == nil {
		fmt.Fprintf(w, "  In-use: %d sectors, Free: %d sectors, Directory track: %d sectors", u.InUseSectors, u.FreeSectors, u.BAMSectors)
		if u.ExtraSectors > 0 {
			fmt.Fprintf(w, ", Not in BAM: %d sectors", u.ExtraSectors)
		}
		fmt.Fprintln(w)
	}
	rt, rs := d.rootDir()
	if msg := d.headerLinkMismatch(); msg != "" {
		fmt.Fprintf(w, "  Note: %s\n", msg)
	}
	entries := d.readDir(rt, rs)
	fmt.Fprintf(w, "  Directory Entries (%d):\n", len(entries))
	d.dumpDir(w, rt, rs, trackRange{1, d.layout.tracks}, "    ", 0)
	return nil
}
