// Package adf reads Amiga Disk Files (ADF): raw sector images of DD (880
// KiB) and HD (1.76 MiB) floppies holding an OFS or FFS filesystem.
package adf

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
	SectorSize   = 512
	TotalSectors = 1760                          // DD disk
	DiskSize     = SectorSize * TotalSectors     // 901120 bytes
	HDDiskSize   = SectorSize * TotalSectors * 2 // 1802240 bytes
	RootSector   = 880                           // DD disk

	TypeHeader = 2
	TypeData   = 8  // OFS data block
	TypeList   = 16 // file extension block

	SecTypeRoot = 1
	SecTypeDir  = 2
	SecTypeFile = -3

	tableEntries = SectorSize/4 - 56 // 72 block pointers in header and extension blocks
)

type Entry struct {
	Name    string
	IsDir   bool
	Size    uint32
	ModTime time.Time
	header  uint32 // header block
}

type DiskUsage struct {
	SectorSize          int
	TotalSectors        int
	InUseSectors        int
	FreeSectors         int
	TotalBytes          int64
	InUseBytes          int64
	FreeBytes           int64
	PercentFull         float64
	FormattedCapacity   int64
	UnformattedCapacity int64
	FormattedDesc       string
	UnformattedDesc     string
}

// amigaEpoch is Jan 1, 1978 00:00:00 UTC
var amigaEpoch = time.Date(1978, time.January, 1, 0, 0, 0, 0, time.UTC)

func parseAmigaDate(days, mins, ticks uint32) time.Time {
	// 1 day = 24h, 1 min = 60s, 1 tick = 1/50 sec
	t := amigaEpoch.AddDate(0, 0, int(days))
	t = t.Add(time.Duration(mins)*time.Minute + time.Duration(ticks)*(time.Second/50))
	return t
}

type FileSystem struct {
	data    []byte
	sectors int
}

// Open parses a DD or HD ADF image.
func Open(r io.ReaderAt, size int64) (*FileSystem, error) {
	if size != DiskSize && size != HDDiskSize {
		return nil, fmt.Errorf("invalid ADF disk size: expected %d (DD) or %d (HD) bytes, got %d", DiskSize, HDDiskSize, size)
	}
	data := make([]byte, size)
	if _, err := r.ReadAt(data, 0); err != nil {
		return nil, fmt.Errorf("failed reading disk image: %w", err)
	}
	if string(data[0:3]) != "DOS" {
		return nil, fmt.Errorf("unrecognized file system DOS type: %q", string(data[0:4]))
	}
	fs := &FileSystem{data: data, sectors: int(size / SectorSize)}
	root := fs.block(fs.rootBlock())
	if u32(root, 0) != TypeHeader || s32(root, 508) != SecTypeRoot {
		return nil, fmt.Errorf("invalid root block: type=%d, secType=%d", u32(root, 0), s32(root, 508))
	}
	return fs, nil
}

func u32(b []byte, off int) uint32 { return binary.BigEndian.Uint32(b[off:]) }
func s32(b []byte, off int) int32  { return int32(binary.BigEndian.Uint32(b[off:])) }

func (fs *FileSystem) rootBlock() uint32 { return uint32(fs.sectors / 2) }

func (fs *FileSystem) isHD() bool { return fs.sectors == 2*TotalSectors }

func (fs *FileSystem) isFFS() bool { return fs.data[3]&0x01 != 0 }

// block returns block n, or nil if it is out of range (blocks 0 and 1 are
// the boot block and never part of the filesystem).
func (fs *FileSystem) block(n uint32) []byte {
	if n < 2 || int(n) >= fs.sectors {
		return nil
	}
	return fs.data[int(n)*SectorSize : int(n+1)*SectorSize]
}

// readDir lists the entries of the root or a directory header block.
func (fs *FileSystem) readDir(dir uint32) ([]Entry, error) {
	blk := fs.block(dir)
	if blk == nil || u32(blk, 0) != TypeHeader || (s32(blk, 508) != SecTypeRoot && s32(blk, 508) != SecTypeDir) {
		return nil, fmt.Errorf("block %d is not a directory", dir)
	}
	htSize := uint32(tableEntries)
	if s32(blk, 508) == SecTypeRoot {
		if n := u32(blk, 12); n > 0 && n <= tableEntries {
			htSize = n
		}
	}

	var entries []Entry
	visited := make(map[uint32]bool)
	for i := uint32(0); i < htSize; i++ {
		for sector := u32(blk, int(24+i*4)); sector != 0 && !visited[sector]; {
			visited[sector] = true
			hdr := fs.block(sector)
			if hdr == nil {
				break
			}
			if nameLen := int(hdr[432]); u32(hdr, 0) == TypeHeader && nameLen > 0 && nameLen <= 30 {
				e := Entry{
					Name:    util.EscapeNonPrintable(hdr[433 : 433+nameLen]),
					ModTime: parseAmigaDate(u32(hdr, 420), u32(hdr, 424), u32(hdr, 428)),
					header:  sector,
				}
				switch s32(hdr, 508) {
				case SecTypeDir:
					e.IsDir = true
				case SecTypeFile:
					e.Size = u32(hdr, 324)
				}
				entries = append(entries, e)
			}
			sector = u32(hdr, 496) // next in hash chain
		}
	}

	// Sort entries case-insensitively for clean and deterministic output
	sort.Slice(entries, func(i, j int) bool {
		return strings.ToLower(entries[i].Name) < strings.ToLower(entries[j].Name)
	})
	return entries, nil
}

// find resolves a "/" separated path (Amiga names are case-insensitive).
func (fs *FileSystem) find(path string) (*Entry, error) {
	parts := strings.FieldsFunc(path, func(r rune) bool { return r == '/' })
	if len(parts) == 0 {
		return nil, fmt.Errorf("empty path")
	}
	dir := fs.rootBlock()
	for i, part := range parts {
		entries, err := fs.readDir(dir)
		if err != nil {
			return nil, err
		}
		var found *Entry
		for j := range entries {
			if strings.EqualFold(entries[j].Name, part) {
				found = &entries[j]
				break
			}
		}
		if found == nil {
			return nil, fmt.Errorf("%q not found", path)
		}
		if i == len(parts)-1 {
			return found, nil
		}
		if !found.IsDir {
			return nil, fmt.Errorf("%q is not a directory", strings.Join(parts[:i+1], "/"))
		}
		dir = found.header
	}
	return nil, fmt.Errorf("%q not found", path)
}

// ListRoot returns the root directory entries.
func (fs *FileSystem) ListRoot() ([]Entry, error) { return fs.readDir(fs.rootBlock()) }

// ListDir returns the entries of the directory at path ("" for the root).
func (fs *FileSystem) ListDir(path string) ([]Entry, error) {
	if strings.Trim(path, "/") == "" {
		return fs.ListRoot()
	}
	e, err := fs.find(path)
	if err != nil {
		return nil, err
	}
	if !e.IsDir {
		return nil, fmt.Errorf("%q is not a directory", path)
	}
	return fs.readDir(e.header)
}

// ReadFile returns the contents of the file at path (e.g. "s/Startup-Sequence").
func (fs *FileSystem) ReadFile(path string) ([]byte, error) {
	e, err := fs.find(path)
	if err != nil {
		return nil, fmt.Errorf("file %w", err)
	}
	hdr := fs.block(e.header)
	switch s32(hdr, 508) {
	case SecTypeDir:
		return nil, fmt.Errorf("%q is a directory, not a file", path)
	case SecTypeFile:
	default:
		return nil, fmt.Errorf("%q is not a regular file", path)
	}
	return fs.readFile(e.header)
}

// dataBlocks returns a file's data blocks, listed in its header and
// extension blocks (each holds up to 72 pointers, stored from the end).
func (fs *FileSystem) dataBlocks(header uint32) ([]uint32, error) {
	var blocks []uint32
	visited := map[uint32]bool{}
	for blk, n := fs.block(header), header; blk != nil; {
		if visited[n] {
			return blocks, fmt.Errorf("cyclic extension block chain at block %d", n)
		}
		visited[n] = true
		count := u32(blk, 8)
		if count > tableEntries {
			return blocks, fmt.Errorf("invalid block count %d in block %d", count, n)
		}
		for i := uint32(0); i < count; i++ {
			blocks = append(blocks, u32(blk, int(24+(tableEntries-1-i)*4)))
		}
		if n = u32(blk, 504); n == 0 {
			break
		}
		if blk = fs.block(n); blk == nil || u32(blk, 0) != TypeList {
			return blocks, fmt.Errorf("invalid extension block %d", n)
		}
	}
	return blocks, nil
}

// readFile reads a file's data: OFS data blocks have a 24 byte header
// (with the number of data bytes), FFS data blocks are all data.
func (fs *FileSystem) readFile(header uint32) ([]byte, error) {
	size := int(u32(fs.block(header), 324))
	blocks, err := fs.dataBlocks(header)
	if err != nil {
		return nil, err
	}
	buf := make([]byte, 0, size)
	for i, n := range blocks {
		blk := fs.block(n)
		if blk == nil {
			return nil, fmt.Errorf("data block %d out of range", n)
		}
		if fs.isFFS() {
			buf = append(buf, blk...)
			continue
		}
		if u32(blk, 0) != TypeData || u32(blk, 4) != header || u32(blk, 8) != uint32(i+1) {
			return nil, fmt.Errorf("invalid OFS data block %d (#%d of file at block %d)", n, i+1, header)
		}
		ds := int(u32(blk, 12))
		if ds > SectorSize-24 {
			return nil, fmt.Errorf("invalid data size %d in block %d", ds, n)
		}
		buf = append(buf, blk[24:24+ds]...)
	}
	if len(buf) < size {
		return nil, fmt.Errorf("file has %d bytes in its data blocks, header says %d", len(buf), size)
	}
	return buf[:size], nil
}

func (fs *FileSystem) DiskName() string {
	root := fs.block(fs.rootBlock())
	if nameLen := int(root[432]); nameLen > 0 && nameLen <= 30 {
		return util.EscapeNonPrintable(root[433 : 433+nameLen])
	}
	return ""
}

func (fs *FileSystem) DiskUsage() (*DiskUsage, error) {
	root := fs.block(fs.rootBlock())
	if bmFlag := s32(root, 312); bmFlag != -1 {
		return nil, fmt.Errorf("bitmap flag is invalid (%d), disk bitmap may be corrupted", bmFlag)
	}

	// The bitmap covers blocks 2 and up; a set bit means free.
	freeSectors := 0
	managed := fs.sectors - 2
	checked := 0
	for i := 0; i < 25 && checked < managed; i++ {
		bmPtr := u32(root, 316+i*4)
		if bmPtr == 0 {
			continue
		}
		bmBlock := fs.block(bmPtr)
		if bmBlock == nil {
			return nil, fmt.Errorf("bitmap block %d out of range", bmPtr)
		}
		// Long 0 is checksum. Longs 1 through 127 are bitmap data words.
		for off := 4; off < SectorSize && checked < managed; off += 4 {
			val := u32(bmBlock, off)
			for bit := 0; bit < 32 && checked < managed; bit++ {
				if (val>>bit)&1 == 1 {
					freeSectors++
				}
				checked++
			}
		}
	}

	// Boot blocks 0 and 1 are always in use.
	inUse := fs.sectors - freeSectors
	totalBytes := int64(fs.sectors) * SectorSize
	u := &DiskUsage{
		SectorSize:          SectorSize,
		TotalSectors:        fs.sectors,
		InUseSectors:        inUse,
		FreeSectors:         freeSectors,
		TotalBytes:          totalBytes,
		InUseBytes:          int64(inUse) * SectorSize,
		FreeBytes:           int64(freeSectors) * SectorSize,
		PercentFull:         float64(inUse) / float64(fs.sectors) * 100,
		FormattedCapacity:   totalBytes,
		UnformattedCapacity: 1000000,
		FormattedDesc:       "880 KiB",
		UnformattedDesc:     "1 MB",
	}
	if fs.isHD() {
		u.UnformattedCapacity, u.FormattedDesc, u.UnformattedDesc = 2000000, "1760 KiB", "2 MB"
	}
	return u, nil
}

func (fs *FileSystem) dumpDir(w io.Writer, dir uint32, prefix string, depth int) {
	entries, err := fs.readDir(dir)
	if err != nil {
		fmt.Fprintf(w, "    [ERROR: %v]\n", err)
		return
	}
	for _, e := range entries {
		typeStr, name := "FILE", prefix+e.Name
		if e.IsDir {
			typeStr, name = "DIR ", name+"/"
		}
		fmt.Fprintf(w, "    [%s] %-25s %8d bytes  %s  block %d\n",
			typeStr, name, e.Size, e.ModTime.Format("2006-01-02 15:04:05"), e.header)
		if e.IsDir && depth < 32 {
			fs.dumpDir(w, e.header, name, depth+1)
		}
	}
}

func (fs *FileSystem) Dump(w io.Writer) error {
	fmt.Fprintf(w, "Amiga ADF Disk Image Dump\n")
	fmt.Fprintf(w, "  DOS Type: %q (0x%08x), %s\n", string(fs.data[0:4]), u32(fs.data, 0), fs.Format().Filesystem)
	fmt.Fprintf(w, "  Total size: %d bytes (%d sectors of %d bytes)\n", len(fs.data), fs.sectors, SectorSize)
	fmt.Fprintf(w, "  Disk Name: %q\n", fs.DiskName())
	fmt.Fprintf(w, "  Root Block: sector %d\n", fs.rootBlock())

	bmFlag := s32(fs.block(fs.rootBlock()), 312)
	fmt.Fprintf(w, "  Bitmap Flag: %d (valid=%v)\n", bmFlag, bmFlag == -1)

	if usage, err := fs.DiskUsage(); err == nil {
		fmt.Fprintf(w, "  Formatted Capacity: %d bytes (%s)\n", usage.FormattedCapacity, usage.FormattedDesc)
		fmt.Fprintf(w, "  Unformatted Capacity: %d bytes (%s)\n", usage.UnformattedCapacity, usage.UnformattedDesc)
		fmt.Fprintf(w, "  In-use sectors: %d, Free sectors: %d, Total sectors: %d\n",
			usage.InUseSectors, usage.FreeSectors, usage.TotalSectors)
	}

	if entries, err := fs.ListRoot(); err == nil {
		fmt.Fprintf(w, "  Root Entries (%d):\n", len(entries))
		fs.dumpDir(w, fs.rootBlock(), "", 0)
	}
	return nil
}

// Format describes the ADF image format and geometry.
func (fs *FileSystem) Format() *util.FormatInfo {
	flags := fs.data[3]
	name := "Amiga OFS"
	if flags&0x01 != 0 {
		name = "Amiga FFS"
	}
	switch {
	case flags&0x04 != 0:
		name += " (directory cache)"
	case flags&0x02 != 0:
		name += " (international)"
	}
	image, sectors := "ADF (Amiga Disk File, raw sector image)", "11"
	if fs.isHD() {
		image, sectors = "ADF (Amiga Disk File, raw sector image, HD)", "22"
	}
	return &util.FormatInfo{
		Platform:   "Amiga",
		Image:      image,
		Filesystem: name,
		Tracks:     80,
		Heads:      2,
		Sectors:    sectors,
		SectorSize: SectorSize,
	}
}
