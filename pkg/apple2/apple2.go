package apple2

import (
	"encoding/binary"
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"time"

	"github.com/fredsa/retro-disk-tool/cmd/rdt/pkg/util"
)

// High-ASCII / Apple II byte decoding:
// Apple II DOS 3.x stores characters with bit 7 set (0x80..0xFF).
// Standard ASCII printable is 0x20..0x7E.
func DecodeAppleBytes(b []byte) string {
	var sb strings.Builder
	for _, c := range b {
		ch := c & 0x7F
		if ch >= 0x20 && ch <= 0x7E {
			sb.WriteByte(ch)
		} else {
			sb.WriteString(fmt.Sprintf("\\x%02X", c))
		}
	}
	return strings.TrimRight(sb.String(), " ")
}

type DiskHeaderInfo struct {
	Name    string
	ID      string
	DOSType string
}

type DiskEntry struct {
	Name    string
	IsDir   bool
	Size    uint32
	ModTime *time.Time
	Corrupt bool
	Error   string
	// Native is the platform specific rendering of the name (inverse /
	// flashing styles for Apple DOS 3.x).
	Native []util.Glyph
	// Format-specific info for Dump:
	FileType string
	Blocks   int
	Locked   bool
	key      int // ProDOS key block (a subdirectory's first directory block)
}

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
	TotalBytes          int64
	InUseBytes          int64
	FreeBytes           int64
	BAMBytes            int64
	HasUsage            bool
}

type Image interface {
	Header() *DiskHeaderInfo
	List() ([]DiskEntry, error)
	// ListDir returns the entries of the directory at path ("" for the root).
	ListDir(path string) ([]DiskEntry, error)
	Usage() (*DiskUsageInfo, error)
	ReadFile(path string) ([]byte, error)
	Dump(w io.Writer) error
	Format() *util.FormatInfo
}

// containerImage overrides the image (container) description of a wrapped Image.
type containerImage struct {
	Image
	container string
}

func (c containerImage) Format() *util.FormatInfo {
	f := *c.Image.Format()
	f.Image = c.container
	return &f
}

// Sector skew table between DOS 3.3 logical sector order and ProDOS sector order
var dosToProdosSector = [16]int{
	0, 14, 13, 12, 11, 10, 9, 8, 7, 6, 5, 4, 3, 2, 1, 15,
}

func Open(filename string, r io.ReaderAt, size int64) (Image, error) {
	if size <= 0 {
		return nil, fmt.Errorf("invalid Apple II disk size: %d bytes", size)
	}
	data := make([]byte, size)
	if _, err := r.ReadAt(data, 0); err != nil {
		return nil, fmt.Errorf("failed to read Apple II disk image: %w", err)
	}

	var (
		img       Image
		err       error
		container string
	)
	ext := strings.ToLower(filepath.Ext(filename))
	switch {
	case ext == ".woz":
		img, err = openWOZ(data)
		container = wozContainer(data)
	case ext == ".nib" || size == nibDiskSize:
		img, err = openNIB(data)
		container = "NIB (nibblized image, 35 tracks x 6656 bytes)"
	default:
		img, err = openRaw(data, size)
		if err == nil {
			container = rawContainer(ext, img)
		}
	}
	if err != nil {
		return nil, err
	}
	return containerImage{Image: img, container: container}, nil
}

// wozContainer describes a WOZ file from its header and INFO chunk.
func wozContainer(data []byte) string {
	if len(data) < 4 {
		return "WOZ"
	}
	desc := fmt.Sprintf("WOZ%c (bitstream image", data[3])
	// INFO chunk is always first: header(12) + id/size(8) + data; creator at INFO+5..37
	if len(data) >= 12+8+37 && string(data[12:16]) == "INFO" {
		creator := strings.TrimRight(string(data[12+8+5:12+8+37]), " \x00")
		if creator != "" {
			desc += fmt.Sprintf(", created by %q", creator)
		}
	}
	return desc + ")"
}

// rawContainer describes a headerless sector image by extension and sector order.
func rawContainer(ext string, img Image) string {
	label := strings.ToUpper(strings.TrimPrefix(ext, "."))
	if label == "" {
		label = "Raw"
	}
	order := "13-sector"
	switch d := img.(type) {
	case *DOSDisk:
		if d.sectorsPerTrack == 16 {
			order = "DOS 3.3 sector order"
			if d.prodosOrder {
				order = "ProDOS sector order"
			}
		}
	case *ProDOSDisk:
		order = "ProDOS sector order"
		if d.dosOrder {
			order = "DOS 3.3 sector order"
		}
	}
	return fmt.Sprintf("%s (raw sector image, %s)", label, order)
}

func openRaw(data []byte, size int64) (Image, error) {
	if size != 116480 && size != 143360 {
		return nil, fmt.Errorf("invalid Apple II disk size: %d bytes", size)
	}

	if size == 116480 {
		// 13-sector Apple DOS 3.1 / 3.2 (35 tracks * 13 sectors * 256 bytes)
		return newDOSDisk(data, 13, false)
	}

	return open140K(data)
}

func open140K(data []byte) (Image, error) {
	// 143,360 bytes (35 tracks * 16 sectors * 256 bytes):
	// Could be ProDOS (in ProDOS order or DOS order) or DOS 3.3 (in DOS order or ProDOS order).
	if isProDOSBlock2(data, false) {
		return newProDOSDisk(data, false)
	}
	if isProDOSBlock2(data, true) {
		return newProDOSDisk(data, true)
	}
	if isDOS33VTOC(data, false) {
		return newDOSDisk(data, 16, false)
	}
	if isDOS33VTOC(data, true) {
		return newDOSDisk(data, 16, true)
	}

	return nil, fmt.Errorf("unrecognized Apple II 140K disk filesystem")
}

// readProDOSBlock reads a 512-byte ProDOS logical block (0..279).
// If dosOrder is false (.po), block N is at offset N*512.
// If dosOrder is true (.do/.dsk), block N on track T=N/8, blockInTrack B=N%8
// consists of two 256-byte halves stored at DOS sectors dosToProdosSector[2*B] and dosToProdosSector[2*B+1].
func readProDOSBlock(data []byte, blockNum int, dosOrder bool) ([]byte, error) {
	if blockNum < 0 || (blockNum+1)*512 > len(data) {
		return nil, fmt.Errorf("block %d out of range", blockNum)
	}
	if !dosOrder {
		out := make([]byte, 512)
		copy(out, data[blockNum*512:(blockNum+1)*512])
		return out, nil
	}
	track := blockNum / 8
	blockInTrack := blockNum % 8
	sec1 := dosToProdosSector[blockInTrack*2]
	sec2 := dosToProdosSector[blockInTrack*2+1]
	off1 := (track*16 + sec1) * 256
	off2 := (track*16 + sec2) * 256
	out := make([]byte, 512)
	copy(out[0:256], data[off1:off1+256])
	copy(out[256:512], data[off2:off2+256])
	return out, nil
}

func isProDOSBlock2(data []byte, dosOrder bool) bool {
	blk, err := readProDOSBlock(data, 2, dosOrder)
	if err != nil {
		return false
	}
	prevBlk := binary.LittleEndian.Uint16(blk[0:2])
	storageType := blk[4] >> 4
	nameLen := int(blk[4] & 0x0F)
	entryLen := blk[0x23]
	entriesPerBlk := blk[0x24]
	totalBlocks := binary.LittleEndian.Uint16(blk[0x29:0x2B])
	return prevBlk == 0 && storageType == 0x0F && nameLen >= 1 && nameLen <= 15 &&
		entryLen == 0x27 && entriesPerBlk == 0x0D && totalBlocks == 280
}

// readDOSSector reads a 256-byte Apple DOS sector.
func readDOSSector(data []byte, track, sector, sectorsPerTrack int, prodosOrder bool) ([]byte, error) {
	if track < 0 || track >= 35 || sector < 0 || sector >= sectorsPerTrack {
		return nil, fmt.Errorf("invalid track %d, sector %d", track, sector)
	}
	actualSector := sector
	if prodosOrder && sectorsPerTrack == 16 {
		actualSector = dosToProdosSector[sector]
	}
	off := (track*sectorsPerTrack + actualSector) * 256
	if off+256 > len(data) {
		return nil, fmt.Errorf("sector offset out of bounds")
	}
	return data[off : off+256], nil
}

func isDOS33VTOC(data []byte, prodosOrder bool) bool {
	vtoc, err := readDOSSector(data, 17, 0, 16, prodosOrder)
	if err != nil {
		return false
	}
	catTrack := vtoc[1]
	catSector := vtoc[2]
	tracksPerDisk := vtoc[0x34]
	sectorsPerTrack := vtoc[0x35]
	bytesPerSector := binary.LittleEndian.Uint16(vtoc[0x36:0x38])
	return catTrack == 17 && catSector >= 1 && catSector < 16 &&
		tracksPerDisk == 35 && sectorsPerTrack == 16 && bytesPerSector == 256
}
