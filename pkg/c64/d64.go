package c64

import (
	"encoding/binary"
	"fmt"
	"io"
	"strings"
	"time"
)

type DiskHeaderInfo struct {
	Name    string
	ID      string
	DOSType string
}

func (d *D64Disk) Header() *DiskHeaderInfo {
	bamOffset, err := getSectorOffset(18, 0)
	if err != nil {
		return nil
	}
	bam := d.data[bamOffset : bamOffset+256]

	diskNameRaw := bam[0x90 : 0x90+16]
	nameEnd := 16
	for nameEnd > 0 && diskNameRaw[nameEnd-1] == 0xa0 {
		nameEnd--
	}
	diskName := DecodeBytes(diskNameRaw[:nameEnd])
	diskID := DecodeBytes(bam[0xa2:0xa4])
	dosType := DecodeBytes(bam[0xa5:0xa7])

	return &DiskHeaderInfo{
		Name:    diskName,
		ID:      diskID,
		DOSType: dosType,
	}
}

type DiskEntry struct {
	Name    string
	Type    string
	Sectors int
	Bytes   int
	Locked  bool
	Closed  bool
}

type D64Disk struct {
	data []byte
}

func OpenD64(r io.ReaderAt, size int64) (*D64Disk, error) {
	// Standard D64 sizes:
	// 35 tracks without errors = 174,848 bytes (683 sectors * 256 bytes)
	// 35 tracks with errors = 175,531 bytes
	// 40 tracks without errors = 196,608 bytes
	// 40 tracks with errors = 197,376 bytes
	if size != 174848 && size != 175531 && size != 196608 && size != 197376 {
		return nil, fmt.Errorf("invalid D64 disk size: %d bytes", size)
	}

	data := make([]byte, size)
	if _, err := r.ReadAt(data, 0); err != nil {
		return nil, fmt.Errorf("failed to read D64 file: %w", err)
	}

	return &D64Disk{data: data}, nil
}

func (d *D64Disk) List() ([]diskEntryCompat, error) {
	entries, err := d.ReadDirectory()
	if err != nil {
		return nil, err
	}
	res := make([]diskEntryCompat, len(entries))
	for i, e := range entries {
		res[i] = diskEntryCompat{
			Name:    e.Name,
			IsDir:   false,
			Size:    uint32(e.Bytes),
			ModTime: nil,
		}
	}
	return res, nil
}

// diskEntryCompat satisfies disk.Entry
type diskEntryCompat struct {
	Name    string
	IsDir   bool
	Size    uint32
	ModTime *time.Time
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

func (d *D64Disk) Usage() (*DiskUsageInfo, error) {
	// BAM is at track 18, sector 0
	bamOffset, err := getSectorOffset(18, 0)
	if err != nil {
		return nil, err
	}
	bam := d.data[bamOffset : bamOffset+256]

	freeSectors := 0
	totalSectors := 683 // Tracks 1-35
	// Count free sectors from BAM entries for tracks 1-35 excluding Track 18.
	for t := 1; t <= 35; t++ {
		if t == 18 {
			continue
		}
		offset := 4 * t
		freeInTrack := int(bam[offset])
		freeSectors += freeInTrack
	}

	// Track 18 has 19 reserved sectors for BAM & directory
	bamSectors := 19
	// In-use user sectors:
	inUseSectors := (totalSectors - bamSectors) - freeSectors
	totalBytes := int64(totalSectors) * 256
	inUseBytes := int64(inUseSectors) * 256
	freeBytes := int64(freeSectors) * 256
	bamBytes := int64(bamSectors) * 256

	// 1541 single-sided 5.25" floppy:
	// Formatted capacity: 170 KiB (174,848 bytes / ~170.75 KiB)
	// Unformatted capacity: 250 KB (250,000 bytes)
	return &DiskUsageInfo{
		FormattedCapacity:   totalBytes,
		FormattedDesc:       "170 KiB",
		UnformattedCapacity: 250000,
		UnformattedDesc:     "250 KB",
		SectorSize:          256,
		TotalSectors:        totalSectors,
		InUseSectors:        inUseSectors,
		FreeSectors:         freeSectors,
		BAMSectors:          bamSectors,
		TotalBytes:          totalBytes,
		InUseBytes:          inUseBytes,
		FreeBytes:           freeBytes,
		BAMBytes:            bamBytes,
		HasUsage:            true,
	}, nil
}

func (d *D64Disk) ReadDirectory() ([]DiskEntry, error) {
	bamOffset, err := getSectorOffset(18, 0)
	if err != nil {
		return nil, err
	}
	bam := d.data[bamOffset : bamOffset+256]

	nextTrack := int(bam[0])
	nextSector := int(bam[1])

	var entries []DiskEntry
	visited := make(map[string]bool)

	for nextTrack != 0 {
		key := fmt.Sprintf("%d/%d", nextTrack, nextSector)
		if visited[key] {
			break
		}
		visited[key] = true

		secOffset, err := getSectorOffset(nextTrack, nextSector)
		if err != nil {
			break
		}
		sec := d.data[secOffset : secOffset+256]
		nextTrack = int(sec[0])
		nextSector = int(sec[1])

		// Up to 8 directory entries per sector (32 bytes each)
		for i := 0; i < 8; i++ {
			entryBytes := sec[i*32 : (i+1)*32]
			rawType := entryBytes[2]
			actualType := rawType & 0x07
			if actualType == 0 {
				continue // Deleted / empty
			}

			typeStr, ok := fileTypes[actualType]
			if !ok {
				typeStr = "???"
			}

			// Raw PETSCII file name (16 bytes at offset 5..20)
			rawName := entryBytes[5:21]
			// Trim padding (0xA0)
			nameEnd := 16
			for nameEnd > 0 && rawName[nameEnd-1] == 0xa0 {
				nameEnd--
			}
			name := DecodeBytes(rawName[:nameEnd])

			// Sectors count is a 16-bit little endian at offset 30..31
			sectors := int(binary.LittleEndian.Uint16(entryBytes[30:32]))
			// In C64 1541, file size in bytes is roughly sectors * 254 bytes
			// (each data block has 2-byte header: next track and next sector).
			// If sectors > 0, last sector has nextTrack=0 and nextSector=number of bytes used + 1
			fileContent, _ := d.readFileContent(int(entryBytes[3]), int(entryBytes[4]), sectors)
			bytesCount := len(fileContent)

			entries = append(entries, DiskEntry{
				Name:    name,
				Type:    typeStr,
				Sectors: sectors,
				Bytes:   bytesCount,
				Locked:  (rawType & 0x40) != 0,
				Closed:  (rawType & 0x80) != 0,
			})
		}
	}

	return entries, nil
}

func (d *D64Disk) ReadFile(targetName string) ([]byte, error) {
	bamOffset, err := getSectorOffset(18, 0)
	if err != nil {
		return nil, err
	}
	bam := d.data[bamOffset : bamOffset+256]

	nextTrack := int(bam[0])
	nextSector := int(bam[1])

	targetLower := strings.ToLower(targetName)
	visited := make(map[string]bool)

	for nextTrack != 0 {
		key := fmt.Sprintf("%d/%d", nextTrack, nextSector)
		if visited[key] {
			break
		}
		visited[key] = true

		secOffset, err := getSectorOffset(nextTrack, nextSector)
		if err != nil {
			break
		}
		sec := d.data[secOffset : secOffset+256]
		nextTrack = int(sec[0])
		nextSector = int(sec[1])

		for i := 0; i < 8; i++ {
			entryBytes := sec[i*32 : (i+1)*32]
			rawType := entryBytes[2]
			actualType := rawType & 0x07
			if actualType == 0 {
				continue
			}

			rawName := entryBytes[5:21]
			nameEnd := 16
			for nameEnd > 0 && rawName[nameEnd-1] == 0xa0 {
				nameEnd--
			}
			name := DecodeBytes(rawName[:nameEnd])

			if strings.ToLower(name) == targetLower {
				firstTrack := int(entryBytes[3])
				firstSector := int(entryBytes[4])
				sectors := int(binary.LittleEndian.Uint16(entryBytes[30:32]))
				return d.readFileContent(firstTrack, firstSector, sectors)
			}
		}
	}

	return nil, fmt.Errorf("file %q not found", targetName)
}

func (d *D64Disk) readFileContent(firstTrack, firstSector int, sectors int) ([]byte, error) {
	if firstTrack == 0 {
		return nil, nil
	}

	var data []byte
	t := firstTrack
	s := firstSector
	visited := make(map[string]bool)
	blocks := 0

	for t != 0 && blocks <= sectors+20 {
		key := fmt.Sprintf("%d/%d", t, s)
		if visited[key] {
			return nil, fmt.Errorf("cyclic block chain detected at %d/%d", t, s)
		}
		visited[key] = true
		blocks++

		off, err := getSectorOffset(t, s)
		if err != nil {
			return nil, err
		}
		blk := d.data[off : off+256]
		nextT := int(blk[0])
		nextS := int(blk[1])

		if nextT == 0 {
			// Last block: nextS is index of last byte stored in sector
			// bytes used in this block is nextS (offset 2 up to and including offset nextS)
			if nextS >= 2 && nextS <= 255 {
				data = append(data, blk[2:nextS+1]...)
			} else {
				data = append(data, blk[2:256]...)
			}
			break
		} else {
			data = append(data, blk[2:256]...)
		}
		t = nextT
		s = nextS
	}

	return data, nil
}

func (d *D64Disk) Dump(w io.Writer) error {
	bamOffset, err := getSectorOffset(18, 0)
	if err != nil {
		return err
	}
	bam := d.data[bamOffset : bamOffset+256]

	diskNameRaw := bam[0x90 : 0x90+16]
	nameEnd := 16
	for nameEnd > 0 && diskNameRaw[nameEnd-1] == 0xa0 {
		nameEnd--
	}
	diskName := DecodeBytes(diskNameRaw[:nameEnd])
	diskID := DecodeBytes(bam[0xa2:0xa4])
	dosType := DecodeBytes(bam[0xa5:0xa7])

	fmt.Fprintf(w, "Commodore 64 1541 Disk Image (D64) Dump\n")
	fmt.Fprintf(w, "  Disk Name: %q, ID: %q, DOS Type: %q\n", diskName, diskID, dosType)
	fmt.Fprintf(w, "  Total Tracks: 35, Total Sectors: 683 (174,848 bytes)\n")

	usage, err := d.Usage()
	if err == nil {
		fmt.Fprintf(w, "  Formatted Capacity: %d bytes (170 KiB)\n", usage.FormattedCapacity)
		fmt.Fprintf(w, "  Unformatted Capacity: %d bytes (250 KB)\n", usage.UnformattedCapacity)
		fmt.Fprintf(w, "  In-use: %d sectors, Free: %d sectors\n", usage.InUseSectors, usage.FreeSectors)
	}

	entries, err := d.ReadDirectory()
	if err == nil {
		fmt.Fprintf(w, "  Directory Entries (%d):\n", len(entries))
		for _, e := range entries {
			lockStr := " "
			if e.Locked {
				lockStr = "<"
			}
			fmt.Fprintf(w, "    %-18q %-3s%s %4d sectors (%d bytes)\n",
				e.Name, e.Type, lockStr, e.Sectors, e.Bytes)
		}
	}

	return nil
}
