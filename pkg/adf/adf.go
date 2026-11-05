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
	TotalSectors = 1760
	DiskSize     = SectorSize * TotalSectors // 901120 bytes
	RootSector   = 880

	TypeHeader = 2

	SecTypeRoot = 1
	SecTypeDir  = 2
	SecTypeFile = -3
)

type Entry struct {
	Name    string
	IsDir   bool
	Size    uint32
	ModTime time.Time
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
	data []byte
}

func Open(r io.ReaderAt, size int64) (*FileSystem, error) {
	if size != DiskSize {
		return nil, fmt.Errorf("invalid ADF disk size: expected %d bytes, got %d", DiskSize, size)
	}

	data := make([]byte, DiskSize)
	if _, err := r.ReadAt(data, 0); err != nil {
		return nil, fmt.Errorf("failed reading disk image: %w", err)
	}

	dosType := string(data[0:3])
	if dosType != "DOS" {
		return nil, fmt.Errorf("unrecognized file system DOS type: %q", string(data[0:4]))
	}

	return &FileSystem{data: data}, nil
}

func (fs *FileSystem) ListRoot() ([]Entry, error) {
	rootOffset := RootSector * SectorSize
	rootBlock := fs.data[rootOffset : rootOffset+SectorSize]

	pType := binary.BigEndian.Uint32(rootBlock[0:4])
	secType := int32(binary.BigEndian.Uint32(rootBlock[508:512]))

	if pType != TypeHeader || secType != SecTypeRoot {
		return nil, fmt.Errorf("invalid root block: type=%d, secType=%d", pType, secType)
	}

	htSize := binary.BigEndian.Uint32(rootBlock[12:16])
	if htSize == 0 || htSize > 128 {
		htSize = 72
	}

	var entries []Entry
	visited := make(map[uint32]bool)

	for i := uint32(0); i < htSize; i++ {
		sector := binary.BigEndian.Uint32(rootBlock[24+i*4 : 24+(i+1)*4])
		for sector != 0 {
			if visited[sector] {
				break
			}
			visited[sector] = true

			if int(sector)*SectorSize+SectorSize > len(fs.data) {
				break
			}

			blk := fs.data[sector*SectorSize : (sector+1)*SectorSize]
			entryPType := binary.BigEndian.Uint32(blk[0:4])
			entrySecType := int32(binary.BigEndian.Uint32(blk[508:512]))

			if entryPType == TypeHeader {
				nameLen := int(blk[432])
				if nameLen > 0 && nameLen <= 30 && 433+nameLen <= 512 {
					name := util.EscapeNonPrintable(blk[433 : 433+nameLen])
					days := binary.BigEndian.Uint32(blk[420:424])
					mins := binary.BigEndian.Uint32(blk[424:428])
					ticks := binary.BigEndian.Uint32(blk[428:432])
					modTime := parseAmigaDate(days, mins, ticks)

					var isDir bool
					var size uint32

					if entrySecType == SecTypeDir {
						isDir = true
					} else if entrySecType == SecTypeFile {
						isDir = false
						size = binary.BigEndian.Uint32(blk[324:328])
					}

					entries = append(entries, Entry{
						Name:    name,
						IsDir:   isDir,
						Size:    size,
						ModTime: modTime,
					})
				}
			}

			// Next in hash chain
			sector = binary.BigEndian.Uint32(blk[496:500])
		}
	}

	// Sort entries case-insensitively for clean and deterministic output
	sort.Slice(entries, func(i, j int) bool {
		return strings.ToLower(entries[i].Name) < strings.ToLower(entries[j].Name)
	})

	return entries, nil
}

func (fs *FileSystem) DiskName() string {
	rootOffset := RootSector * SectorSize
	rootBlock := fs.data[rootOffset : rootOffset+SectorSize]
	nameLen := int(rootBlock[432])
	if nameLen > 0 && nameLen <= 30 {
		return util.EscapeNonPrintable(rootBlock[433 : 433+nameLen])
	}
	return ""
}

func (fs *FileSystem) DiskUsage() (*DiskUsage, error) {
	rootOffset := RootSector * SectorSize
	rootBlock := fs.data[rootOffset : rootOffset+SectorSize]

	bmFlag := int32(binary.BigEndian.Uint32(rootBlock[312:316]))
	if bmFlag != -1 {
		return nil, fmt.Errorf("bitmap flag is invalid (%d), disk bitmap may be corrupted", bmFlag)
	}

	freeSectors := 0
	totalManagedSectors := TotalSectors - 2 // Sectors 2 through 1759 (1758 sectors)
	sectorsChecked := 0

	for i := 0; i < 25 && sectorsChecked < totalManagedSectors; i++ {
		bmSector := binary.BigEndian.Uint32(rootBlock[316+i*4 : 320+i*4])
		if bmSector == 0 {
			continue
		}
		if int(bmSector)*SectorSize+SectorSize > len(fs.data) {
			return nil, fmt.Errorf("bitmap sector %d out of range", bmSector)
		}
		bmBlock := fs.data[bmSector*SectorSize : (bmSector+1)*SectorSize]
		// Long 0 is checksum. Longs 1 through 127 are bitmap data words.
		for wordOffset := 4; wordOffset < SectorSize && sectorsChecked < totalManagedSectors; wordOffset += 4 {
			val := binary.BigEndian.Uint32(bmBlock[wordOffset : wordOffset+4])
			for bit := 0; bit < 32 && sectorsChecked < totalManagedSectors; bit++ {
				if (val>>bit)&1 == 1 {
					freeSectors++
				}
				sectorsChecked++
			}
		}
	}

	// In standard Amiga disk usage:
	// Boot sectors 0 and 1 are reserved, so in-use sectors includes reserved sectors plus allocated sectors.
	// In-use sectors = TotalSectors - freeSectors
	inUseSectors := TotalSectors - freeSectors
	totalBytes := int64(TotalSectors) * int64(SectorSize)
	inUseBytes := int64(inUseSectors) * int64(SectorSize)
	freeBytes := int64(freeSectors) * int64(SectorSize)
	percentFull := (float64(inUseSectors) / float64(TotalSectors)) * 100.0

	// For standard 3.5" DD floppy:
	// Formatted capacity is 880 KiB (80 * 2 * 11 * 512 = 901,120 bytes)
	// Unformatted capacity is 1 MB (1,000,000 bytes)
	formattedCapacity := totalBytes
	unformattedCapacity := int64(1000000)

	return &DiskUsage{
		SectorSize:          SectorSize,
		TotalSectors:        TotalSectors,
		InUseSectors:        inUseSectors,
		FreeSectors:         freeSectors,
		TotalBytes:          totalBytes,
		InUseBytes:          inUseBytes,
		FreeBytes:           freeBytes,
		PercentFull:         percentFull,
		FormattedCapacity:   formattedCapacity,
		UnformattedCapacity: unformattedCapacity,
	}, nil
}

func (fs *FileSystem) Dump(w io.Writer) error {
	dosType := string(fs.data[0:4])
	fmt.Fprintf(w, "Amiga ADF Disk Image Dump\n")
	fmt.Fprintf(w, "  DOS Type: %q (0x%08x)\n", dosType, binary.BigEndian.Uint32(fs.data[0:4]))
	fmt.Fprintf(w, "  Total size: %d bytes (%d sectors of %d bytes)\n", len(fs.data), TotalSectors, SectorSize)

	rootOffset := RootSector * SectorSize
	rootBlock := fs.data[rootOffset : rootOffset+SectorSize]
	nameLen := int(rootBlock[432])
	diskName := ""
	if nameLen > 0 && nameLen <= 30 {
		diskName = util.EscapeNonPrintable(rootBlock[433 : 433+nameLen])
	}
	fmt.Fprintf(w, "  Disk Name: %q\n", diskName)
	fmt.Fprintf(w, "  Root Block: sector %d\n", RootSector)

	bmFlag := int32(binary.BigEndian.Uint32(rootBlock[312:316]))
	fmt.Fprintf(w, "  Bitmap Flag: %d (valid=%v)\n", bmFlag, bmFlag == -1)

	usage, err := fs.DiskUsage()
	if err == nil {
		fmt.Fprintf(w, "  Formatted Capacity: %d bytes (880 KiB)\n", usage.FormattedCapacity)
		fmt.Fprintf(w, "  Unformatted Capacity: %d bytes (1 MB)\n", usage.UnformattedCapacity)
		fmt.Fprintf(w, "  In-use sectors: %d, Free sectors: %d, Total sectors: %d\n",
			usage.InUseSectors, usage.FreeSectors, usage.TotalSectors)
	}

	entries, err := fs.ListRoot()
	if err == nil {
		fmt.Fprintf(w, "  Root Entries (%d):\n", len(entries))
		for _, e := range entries {
			typeStr := "FILE"
			if e.IsDir {
				typeStr = "DIR "
			}
			fmt.Fprintf(w, "    [%s] %-25s %8d bytes  %s\n",
				typeStr, e.Name, e.Size, e.ModTime.Format("2006-01-02 15:04:05"))
		}
	}

	return nil
}
