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
			if tsTrack == 0x00 {
				// Unused (0x00) entry
				continue
			}

			deleted := false
			nameBytes := entry[3:33]
			if tsTrack == 0xFF || entry[32] < 35 {
				// Deleted entry: DOS copies the original first T/S list track
				// into the last byte of the filename (entry[32]) and sets
				// entry[0] to 0xFF.
				deleted = true
				tsTrack = int(entry[32])
				nameBytes = entry[3:32]
			}

			rawType := entry[2]
			locked := (rawType & 0x80) != 0
			fileType := rawType & 0x7F
			typeStr, ok := dosFileTypes[fileType]
			if !ok {
				typeStr = "?"
			}

			name := DecodeAppleBytes(nameBytes)
			sectors := int(binary.LittleEndian.Uint16(entry[33:35]))

			content, readErr := d.readFileContent(tsTrack, tsSector, fileType, sectors)
			corrupt := false
			errMsg := ""
			if readErr != nil && !deleted {
				corrupt = true
				errMsg = readErr.Error()
			}

			entries = append(entries, DiskEntry{
				Name:     name,
				Native:   dosNameGlyphs(nameBytes),
				Size:     uint32(len(content)),
				ModTime:  nil,
				Corrupt:  corrupt,
				Deleted:  deleted,
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

	type deletedCandidate struct {
		tsTrack  int
		tsSector int
		fileType byte
		sectors  int
	}
	var fallback *deletedCandidate

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
			if tsTrack == 0x00 {
				continue
			}

			fileType := entry[2] & 0x7F
			sectors := int(binary.LittleEndian.Uint16(entry[33:35]))

			if tsTrack == 0xFF || entry[32] < 35 {
				name := DecodeAppleBytes(entry[3:32])
				if fallback == nil && strings.ToLower(name) == targetLower {
					fallback = &deletedCandidate{
						tsTrack:  int(entry[32]),
						tsSector: tsSector,
						fileType: fileType,
						sectors:  sectors,
					}
				}
				continue
			}

			name := DecodeAppleBytes(entry[3:33])
			if strings.ToLower(name) == targetLower {
				return d.readFileContent(tsTrack, tsSector, fileType, sectors)
			}
		}
	}

	if fallback != nil {
		return d.readFileContent(fallback.tsTrack, fallback.tsSector, fallback.fileType, fallback.sectors)
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
			delStr := ""
			if e.Deleted {
				delStr = " [deleted]"
			}
			fmt.Fprintf(w, "   %s%s %03d %-30s (%d bytes)%s\n",
				lockStr, e.FileType, e.Blocks, e.Name, e.Size, delStr)
		}
	}

	return nil
}

// DumpBoot writes all Apple DOS boot sectors (Track 0, Sector 0 and any
// Stage 2 / multi-sector ROM boot sectors) and their metadata to w.
func (d *DOSDisk) DumpBoot(w io.Writer, disassemble bool) error {
	sec, err := d.readSector(0, 0)
	if err != nil {
		return fmt.Errorf("failed to read boot sector (track 0, sector 0): %w", err)
	}
	fmt.Fprintf(w, "Boot Sector (Track 0, Sector 0, %d bytes):\n", len(sec))
	if d.sectorsPerTrack == 13 {
		// On 13-sector disks, the P5A controller ROM decodes the 5-and-3 GCR
		// nibbles of Track 0, Sector 0 directly into $0300..$03FF without the
		// RWTS post-decode interleave used by standard data sectors. Undo the
		// 5-way interleave so the sector is shown as loaded at $0300.
		var boot0 [256]byte
		for j := 0; j < 51; j++ {
			i := 50 - j
			boot0[i] = sec[5*j+0]
			boot0[51+i] = sec[5*j+1]
			boot0[102+i] = sec[5*j+2]
			boot0[153+i] = sec[5*j+3]
			boot0[204+i] = sec[5*j+4]
		}
		boot0[255] = sec[255]
		isBoot0 := boot0[0] == 0x99 && boot0[1] == 0xB9 && boot0[2] == 0x00 && boot0[3] == 0x08
		stage2Page := uint16(boot0[0xCC])
		stage2Count := 9
		fmt.Fprintf(w, "  Load address:  $0300 (entry point $0301, 13-sector P5A boot PROM)\n")
		if isBoot0 {
			fmt.Fprintf(w, "  Boot loader:   Apple DOS 3.1/3.2 Boot0\n")
			fmt.Fprintf(w, "  Stage 2 load:  $%02X00 (%d sectors from Track 0)\n", stage2Page, stage2Count)
		}
		fmt.Fprintf(w, "  Special bytes:\n")
		fmt.Fprintf(w, "    $0300: $%02X - P5A boot PROM first byte (entry point $0301)\n", boot0[0])
		if isBoot0 {
			fmt.Fprintf(w, "    $03CC: $%02X - Stage 2 base load page ($%02X00)\n", boot0[0xCC], boot0[0xCC])
			fmt.Fprintf(w, "    $03FF: $%02X - Stage 2 end-sector marker\n", boot0[0xFF])
		}
		fmt.Fprintln(w)
		util.HexDump(w, boot0[:], 0x0300)
		if disassemble {
			fmt.Fprintln(w)
			fmt.Fprintln(w, "Disassembly ($0300):")
			fmt.Fprintf(w, "  0300- %02X        .BYTE $%02X        ; P5A boot PROM first byte (entry $0301)\n", boot0[0], boot0[0])
			util.Disassemble6502(w, boot0[:], 0x0300, 1, 256)
		}
		if isBoot0 {
			for s := 1; s <= stage2Count && s < d.sectorsPerTrack; s++ {
				s2, err := d.readSector(0, s)
				if err != nil {
					return fmt.Errorf("failed to read boot sector (track 0, sector %d): %w", s, err)
				}
				base := (stage2Page + uint16(s)) << 8
				fmt.Fprintf(w, "\nBoot Sector (Track 0, Sector %d, %d bytes, load address $%04X):\n\n", s, len(s2), base)
				util.HexDump(w, s2, uint32(base))
				if disassemble {
					fmt.Fprintln(w)
					fmt.Fprintf(w, "Disassembly ($%04X):\n", base)
					util.Disassemble6502(w, s2, base, 0, len(s2))
				}
			}
		}
		return nil
	}

	isBoot1 := sec[0] == 0x01 && sec[1] == 0xA5 && sec[2] == 0x27
	fmt.Fprintf(w, "  Load address:  $0800 (entry point $0801)\n")
	fmt.Fprintf(w, "  Sector count:  %d (ROM loads %d sector(s) to $0800)\n", sec[0], sec[0])
	if isBoot1 {
		fmt.Fprintf(w, "  Boot loader:   Apple DOS 3.3 Boot1\n")
		fmt.Fprintf(w, "  Stage 2 load:  $%02X00 (%d additional sectors from Track 0)\n", sec[0xFE], sec[0xFF])
	}
	fmt.Fprintf(w, "  Special bytes:\n")
	fmt.Fprintf(w, "    $0800: $%02X - ROM boot sector count (loads %d sector(s) to $0800; entry $0801)\n", sec[0], sec[0])
	if isBoot1 {
		fmt.Fprintf(w, "    $084D..$085C: sector skew table (logical -> physical)\n")
		fmt.Fprintf(w, "    $08FD..$08FE: $%02X $%02X - Stage 2 base load address ($%02X00) / jump vector\n", sec[0xFD], sec[0xFE], sec[0xFE])
		fmt.Fprintf(w, "    $08FF: $%02X - Stage 2 sector count (%d additional sectors from Track 0)\n", sec[0xFF], sec[0xFF])
	}
	fmt.Fprintln(w)
	util.HexDump(w, sec, 0x0800)
	if disassemble {
		fmt.Fprintln(w)
		fmt.Fprintln(w, "Disassembly ($0800):")
		fmt.Fprintf(w, "  0800- %02X        .BYTE $%02X        ; ROM boot sector count (loads %d sector(s) to $0800; entry $0801)\n", sec[0], sec[0], sec[0])
		if isBoot1 {
			util.Disassemble6502(w, sec, 0x0800, 1, 0x4D)
			fmt.Fprintf(w, "  084D- %02X %02X %02X %02X %02X %02X %02X %02X  ; Sector skew table (logical -> physical)\n",
				sec[0x4D], sec[0x4E], sec[0x4F], sec[0x50], sec[0x51], sec[0x52], sec[0x53], sec[0x54])
			fmt.Fprintf(w, "  0855- %02X %02X %02X %02X %02X %02X %02X %02X  ; Sector skew table (continued)\n",
				sec[0x55], sec[0x56], sec[0x57], sec[0x58], sec[0x59], sec[0x5A], sec[0x5B], sec[0x5C])
			util.Disassemble6502(w, sec, 0x0800, 0x5D, 0xFD)
			fmt.Fprintf(w, "  08FD- %02X        .BYTE $%02X        ; Stage 2 jump vector low byte\n", sec[0xFD], sec[0xFD])
			fmt.Fprintf(w, "  08FE- %02X        .BYTE $%02X        ; Stage 2 base load page ($%02X00)\n", sec[0xFE], sec[0xFE], sec[0xFE])
			fmt.Fprintf(w, "  08FF- %02X        .BYTE $%02X        ; Stage 2 sector count (%d additional sectors)\n", sec[0xFF], sec[0xFF], sec[0xFF])
		} else {
			util.Disassemble6502(w, sec, 0x0800, 1, 256)
		}
	}

	if isBoot1 {
		stage2Page := uint16(sec[0xFE])
		stage2Count := int(sec[0xFF])
		for s := 1; s <= stage2Count && s < d.sectorsPerTrack; s++ {
			s2, err := d.readSector(0, s)
			if err != nil {
				return fmt.Errorf("failed to read boot sector (track 0, sector %d): %w", s, err)
			}
			base := (stage2Page + uint16(s)) << 8
			fmt.Fprintf(w, "\nBoot Sector (Track 0, Sector %d, %d bytes, load address $%04X):\n\n", s, len(s2), base)
			util.HexDump(w, s2, uint32(base))
			if disassemble {
				fmt.Fprintln(w)
				fmt.Fprintf(w, "Disassembly ($%04X):\n", base)
				util.Disassemble6502(w, s2, base, 0, len(s2))
			}
		}
	} else if sec[0] > 1 {
		count := int(sec[0])
		if count > d.sectorsPerTrack {
			count = d.sectorsPerTrack
		}
		for s := 1; s < count; s++ {
			sn, err := d.readSector(0, s)
			if err != nil {
				return fmt.Errorf("failed to read boot sector (track 0, sector %d): %w", s, err)
			}
			base := uint16(0x0800) + uint16(s)*256
			fmt.Fprintf(w, "\nBoot Sector (Track 0, Sector %d, %d bytes, load address $%04X):\n\n", s, len(sn), base)
			util.HexDump(w, sn, uint32(base))
			if disassemble {
				fmt.Fprintln(w)
				fmt.Fprintf(w, "Disassembly ($%04X):\n", base)
				util.Disassemble6502(w, sn, base, 0, len(sn))
			}
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
