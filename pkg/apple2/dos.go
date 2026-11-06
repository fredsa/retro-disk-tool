package apple2

import (
	"encoding/binary"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/fredsa/retro-disk-tool/cmd/rdt/pkg/util"
)

type DOSDisk struct {
	data            []byte
	sectorsPerTrack int
	prodosOrder     bool
}

func newDOSDisk(data []byte, sectorsPerTrack int, prodosOrder bool) (*DOSDisk, error) {
	return &DOSDisk{
		data:            data,
		sectorsPerTrack: sectorsPerTrack,
		prodosOrder:     prodosOrder,
	}, nil
}

func (d *DOSDisk) readSector(track, sector int) ([]byte, error) {
	return readDOSSector(d.data, track, sector, d.sectorsPerTrack, d.prodosOrder)
}

var dosFileTypes = map[byte]string{
	0x00: "T",
	0x01: "I",
	0x02: "A",
	0x04: "B",
	0x08: "S",
	0x10: "R",
	0x20: "A",
	0x40: "B",
}

func (d *DOSDisk) Header() *DiskHeaderInfo {
	vtoc, err := d.readSector(17, 0)
	if err != nil {
		return nil
	}
	volNum := vtoc[0x06]
	return &DiskHeaderInfo{
		Name: fmt.Sprintf("DISK VOLUME %d", volNum),
	}
}

func (d *DOSDisk) List() ([]DiskEntry, error) {
	vtoc, err := d.readSector(17, 0)
	if err != nil {
		return nil, err
	}
	catTrack := int(vtoc[1])
	catSector := int(vtoc[2])

	var entries []DiskEntry
	visited := make(map[string]bool)

	for catTrack != 0 {
		key := fmt.Sprintf("%d/%d", catTrack, catSector)
		if visited[key] {
			break
		}
		visited[key] = true

		sec, err := d.readSector(catTrack, catSector)
		if err != nil {
			break
		}
		catTrack = int(sec[1])
		catSector = int(sec[2])

		// 7 file entries per catalog sector starting at offset 0x0B (35 bytes each)
		for i := 0; i < 7; i++ {
			off := 0x0B + i*35
			entry := sec[off : off+35]
			tsTrack := int(entry[0])
			tsSector := int(entry[1])
			if tsTrack == 0x00 || tsTrack == 0xFF {
				// Unused (0x00) or deleted (0xFF) entry
				continue
			}

			rawType := entry[2]
			locked := (rawType & 0x80) != 0
			fileType := rawType & 0x7F
			typeStr, ok := dosFileTypes[fileType]
			if !ok {
				typeStr = "?"
			}

			name := DecodeAppleBytes(entry[3:33])
			sectors := int(binary.LittleEndian.Uint16(entry[33:35]))

			content, readErr := d.readFileContent(tsTrack, tsSector, fileType, sectors)
			corrupt := false
			errMsg := ""
			if readErr != nil {
				corrupt = true
				errMsg = readErr.Error()
			}

			entries = append(entries, DiskEntry{
				Name:     name,
				Native:   dosNameGlyphs(entry[3:33]),
				Size:     uint32(len(content)),
				ModTime:  nil,
				Corrupt:  corrupt,
				Error:    errMsg,
				FileType: typeStr,
				Blocks:   sectors,
				Locked:   locked,
			})
		}
	}

	return entries, nil
}

func (d *DOSDisk) readFileContent(tsTrack, tsSector int, fileType byte, maxSectors int) ([]byte, error) {
	var rawData []byte
	curTrack := tsTrack
	curSector := tsSector
	visited := make(map[string]bool)

	for curTrack != 0 {
		key := fmt.Sprintf("%d/%d", curTrack, curSector)
		if visited[key] {
			return rawData, fmt.Errorf("cyclic T/S list at track %d, sector %d", curTrack, curSector)
		}
		visited[key] = true

		tsSec, err := d.readSector(curTrack, curSector)
		if err != nil {
			return rawData, err
		}
		nextTrack := int(tsSec[1])
		nextSector := int(tsSec[2])

		// Up to 122 Track/Sector pairs starting at offset 0x0C
		for i := 0; i < 122; i++ {
			off := 0x0C + i*2
			dataTrack := int(tsSec[off])
			dataSector := int(tsSec[off+1])
			if dataTrack == 0 {
				// End of data or sparse hole: check if any non-zero pairs follow in this T/S list
				hasMore := false
				for j := i + 1; j < 122; j++ {
					if tsSec[0x0C+j*2] != 0 {
						hasMore = true
						break
					}
				}
				if !hasMore && nextTrack == 0 {
					break
				}
				rawData = append(rawData, make([]byte, 256)...)
				continue
			}

			dataBlk, err := d.readSector(dataTrack, dataSector)
			if err != nil {
				return rawData, err
			}
			rawData = append(rawData, dataBlk...)
		}

		curTrack = nextTrack
		curSector = nextSector
	}

	if len(rawData) == 0 {
		return rawData, nil
	}

	// Determine logical payload size from Apple DOS file type headers:
	switch fileType {
	case 0x00: // Text ('T'): sequential text up to first 0x00 byte (or trim trailing 0x00s)
		end := len(rawData)
		for end > 0 && rawData[end-1] == 0x00 {
			end--
		}
		return rawData[:end], nil

	case 0x01, 0x02, 0x20: // Integer BASIC ('I') or Applesoft BASIC ('A'): 2-byte length header
		if len(rawData) >= 2 {
			progLen := int(binary.LittleEndian.Uint16(rawData[0:2]))
			if 2+progLen <= len(rawData) {
				return rawData[:2+progLen], nil
			}
		}
		return rawData, nil

	case 0x04, 0x40: // Binary ('B'): 2-byte load address + 2-byte length header
		if len(rawData) >= 4 {
			binLen := int(binary.LittleEndian.Uint16(rawData[2:4]))
			if 4+binLen <= len(rawData) {
				return rawData[:4+binLen], nil
			}
		}
		return rawData, nil

	default:
		return rawData, nil
	}
}

func (d *DOSDisk) Usage() (*DiskUsageInfo, error) {
	vtoc, err := d.readSector(17, 0)
	if err != nil {
		return nil, err
	}

	totalSectors := 35 * d.sectorsPerTrack
	freeSectors := 0

	// VTOC free sector bitmaps start at 0x38 (4 bytes per track, tracks 0..34)
	for t := 0; t < 35; t++ {
		off := 0x38 + t*4
		// On both 13-sector (DOS 3.1/3.2) and 16-sector (DOS 3.3), the 16-bit big-endian
		// bitmap word in bytes 0..1 has 1 bits for free sectors:
		// 16-sector: bits 15..0 represent sectors 15..0 (0xFFFF when all 16 free)
		// 13-sector: bits 15..3 represent sectors 12..0 (0xFFF8 when all 13 free)
		bitmap16 := (uint16(vtoc[off]) << 8) | uint16(vtoc[off+1])
		if d.sectorsPerTrack == 13 {
			bitmap16 &= 0xFFF8
		}
		for b := 0; b < 16; b++ {
			if (bitmap16 & (1 << b)) != 0 {
				freeSectors++
			}
		}
	}

	inUseSectors := totalSectors - freeSectors
	totalBytes := int64(totalSectors) * 256
	inUseBytes := int64(inUseSectors) * 256
	freeBytes := int64(freeSectors) * 256

	formattedDesc := "140 KiB"
	if d.sectorsPerTrack == 13 {
		formattedDesc = "113 KiB"
	}

	return &DiskUsageInfo{
		FormattedCapacity:   totalBytes,
		FormattedDesc:       formattedDesc,
		UnformattedCapacity: 250000,
		UnformattedDesc:     "250 KB",
		SectorSize:          256,
		TotalSectors:        totalSectors,
		InUseSectors:        inUseSectors,
		FreeSectors:         freeSectors,
		TotalBytes:          totalBytes,
		InUseBytes:          inUseBytes,
		FreeBytes:           freeBytes,
		HasUsage:            true,
	}, nil
}

func (d *DOSDisk) ReadFile(targetName string) ([]byte, error) {
	vtoc, err := d.readSector(17, 0)
	if err != nil {
		return nil, err
	}
	catTrack := int(vtoc[1])
	catSector := int(vtoc[2])
	targetLower := strings.ToLower(targetName)
	visited := make(map[string]bool)

	for catTrack != 0 {
		key := fmt.Sprintf("%d/%d", catTrack, catSector)
		if visited[key] {
			break
		}
		visited[key] = true

		sec, err := d.readSector(catTrack, catSector)
		if err != nil {
			break
		}
		catTrack = int(sec[1])
		catSector = int(sec[2])

		for i := 0; i < 7; i++ {
			off := 0x0B + i*35
			entry := sec[off : off+35]
			tsTrack := int(entry[0])
			tsSector := int(entry[1])
			if tsTrack == 0x00 || tsTrack == 0xFF {
				continue
			}

			fileType := entry[2] & 0x7F
			name := DecodeAppleBytes(entry[3:33])
			sectors := int(binary.LittleEndian.Uint16(entry[33:35]))

			if strings.ToLower(name) == targetLower {
				return d.readFileContent(tsTrack, tsSector, fileType, sectors)
			}
		}
	}

	return nil, fmt.Errorf("file %q not found", targetName)
}

func (d *DOSDisk) Dump(w io.Writer) error {
	vtoc, err := d.readSector(17, 0)
	if err != nil {
		return err
	}
	dosVer := vtoc[3]
	volNum := vtoc[6]

	fmt.Fprintf(w, "Apple II DOS 3.%d Disk Image (%d-sector) Dump\n", dosVer, d.sectorsPerTrack)
	fmt.Fprintf(w, "  Disk Volume: %d\n", volNum)
	fmt.Fprintf(w, "  Total Tracks: 35, Sectors/Track: %d, Total Sectors: %d (%d bytes)\n",
		d.sectorsPerTrack, 35*d.sectorsPerTrack, 35*d.sectorsPerTrack*256)

	usage, err := d.Usage()
	if err == nil {
		fmt.Fprintf(w, "  Formatted Capacity: %d bytes (%s)\n", usage.FormattedCapacity, usage.FormattedDesc)
		fmt.Fprintf(w, "  Unformatted Capacity: %d bytes (%s)\n", usage.UnformattedCapacity, usage.UnformattedDesc)
		fmt.Fprintf(w, "  In-use: %d sectors, Free: %d sectors\n", usage.InUseSectors, usage.FreeSectors)
	}

	entries, err := d.List()
	if err == nil {
		fmt.Fprintf(w, "  Catalog Entries (%d):\n", len(entries))
		for _, e := range entries {
			lockStr := " "
			if e.Locked {
				lockStr = "*"
			}
			fmt.Fprintf(w, "   %s%s %03d %-30s (%d bytes)\n",
				lockStr, e.FileType, e.Blocks, e.Name, e.Size)
		}
	}

	return nil
}

// Format describes the Apple DOS 3.x filesystem and geometry.
func (d *DOSDisk) Format() *util.FormatInfo {
	fsName := "Apple DOS"
	if vtoc, err := d.readSector(17, 0); err == nil {
		fsName = fmt.Sprintf("Apple DOS 3.%d", vtoc[3])
	}
	return &util.FormatInfo{
		Platform:   "Apple II",
		Filesystem: fsName,
		Tracks:     35,
		Heads:      1,
		Sectors:    strconv.Itoa(d.sectorsPerTrack),
		SectorSize: 256,
	}
}
