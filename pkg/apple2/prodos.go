package apple2

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"math/bits"
	"strings"
	"time"

	"github.com/fredsa/retro-disk-tool/cmd/rdt/pkg/util"
)

type ProDOSDisk struct {
	data     []byte
	dosOrder bool
}

func newProDOSDisk(data []byte, dosOrder bool) (*ProDOSDisk, error) {
	return &ProDOSDisk{
		data:     data,
		dosOrder: dosOrder,
	}, nil
}

func (p *ProDOSDisk) readBlock(blockNum int) ([]byte, error) {
	return readProDOSBlock(p.data, blockNum, p.dosOrder)
}

var prodosFileTypes = map[byte]string{
	0x00: "UNK",
	0x01: "BAD",
	0x04: "TXT",
	0x06: "BIN",
	0x0F: "DIR",
	0x19: "ADB",
	0x1A: "AWP",
	0x1B: "ASP",
	0xEF: "PAS",
	0xF0: "CMD",
	0xFA: "INT",
	0xFB: "IVR",
	0xFC: "BAS",
	0xFD: "VAR",
	0xFE: "REL",
	0xFF: "SYS",
}

func decodeProDOSDateTime(b []byte) *time.Time {
	if len(b) < 4 {
		return nil
	}
	dateWord := binary.LittleEndian.Uint16(b[0:2])
	timeWord := binary.LittleEndian.Uint16(b[2:4])
	if dateWord == 0 && timeWord == 0 {
		return nil
	}
	day := int(dateWord & 0x1F)
	month := int((dateWord >> 5) & 0x0F)
	year := int((dateWord >> 9) & 0x7F)
	if year < 40 {
		year += 2000
	} else {
		year += 1900
	}
	minute := int(b[2] & 0x3F)
	hour := int(b[3] & 0x1F)

	if month < 1 || month > 12 || day < 1 || day > 31 {
		return nil
	}
	t := time.Date(year, time.Month(month), day, hour, minute, 0, 0, time.UTC)
	return &t
}

func (p *ProDOSDisk) Header() *DiskHeaderInfo {
	blk2, err := p.readBlock(2)
	if err != nil {
		return nil
	}
	nameLen := int(blk2[4] & 0x0F)
	if nameLen < 1 || nameLen > 15 {
		return nil
	}
	volName := DecodeAppleBytes(blk2[5 : 5+nameLen])
	return &DiskHeaderInfo{
		Name: "/" + volName,
	}
}

func (p *ProDOSDisk) List() ([]DiskEntry, error) {
	return p.readDirBlocks(2)
}

// ListDir returns the entries of the directory at path ("" for the volume
// directory). ProDOS names are case-insensitive.
func (p *ProDOSDisk) ListDir(path string) ([]DiskEntry, error) {
	block := 2
	parts := strings.FieldsFunc(path, func(r rune) bool { return r == '/' })
	for i, part := range parts {
		entries, err := p.readDirBlocks(block)
		if err != nil {
			return nil, err
		}
		var found *DiskEntry
		for j := range entries {
			if strings.EqualFold(entries[j].Name, part) {
				found = &entries[j]
				break
			}
		}
		if found == nil {
			return nil, fmt.Errorf("%q not found", strings.Join(parts[:i+1], "/"))
		}
		if !found.IsDir {
			return nil, fmt.Errorf("%q is not a directory", strings.Join(parts[:i+1], "/"))
		}
		block = found.key
	}
	return p.readDirBlocks(block)
}

// dumpDir writes a directory's entries, then its subdirectories' entries,
// returning how many entries it wrote. Each directory is listed once, so a
// subdirectory pointing back at an ancestor cannot loop.
func (p *ProDOSDisk) dumpDir(w io.Writer, block int, prefix string, depth int, visited map[int]bool) int {
	if visited[block] {
		fmt.Fprintf(w, "    %s[ERROR: directory at block %d already listed]\n", prefix, block)
		return 0
	}
	visited[block] = true
	entries, err := p.readDirBlocks(block)
	if err != nil {
		fmt.Fprintf(w, "    %s[ERROR: %v]\n", prefix, err)
		return 0
	}
	n := len(entries)
	for _, e := range entries {
		lockStr := " "
		if e.Locked {
			lockStr = "*"
		}
		name := prefix + e.Name
		if e.IsDir {
			name += "/"
		}
		fmt.Fprintf(w, "   %s%-16s %-3s %4d blocks (%d bytes)\n", lockStr, name, e.FileType, e.Blocks, e.Size)
		if e.IsDir && depth < 16 {
			n += p.dumpDir(w, e.key, name, depth+1, visited)
		}
	}
	return n
}

func (p *ProDOSDisk) readDirBlocks(startBlock int) ([]DiskEntry, error) {
	var entries []DiskEntry
	curBlock := startBlock
	visited := make(map[int]bool)

	isFirstBlock := true
	entryLen := 0x27
	entriesPerBlk := 0x0D

	for curBlock != 0 {
		if visited[curBlock] {
			break
		}
		visited[curBlock] = true

		blk, err := p.readBlock(curBlock)
		if err != nil {
			return nil, err
		}
		nextBlock := int(binary.LittleEndian.Uint16(blk[2:4]))

		offset := 4
		for i := 0; i < entriesPerBlk; i++ {
			if offset+entryLen > 512 {
				break
			}
			entry := blk[offset : offset+entryLen]
			offset += entryLen

			if isFirstBlock && i == 0 {
				// Volume or Subdirectory header
				if entry[0x1F] != 0 {
					entryLen = int(entry[0x1F])
				}
				if entry[0x20] != 0 {
					entriesPerBlk = int(entry[0x20])
				}
				continue
			}

			storageType := entry[0] >> 4
			nameLen := int(entry[0] & 0x0F)
			if storageType == 0 || nameLen == 0 {
				// Deleted or unused entry
				continue
			}

			name := DecodeAppleBytes(entry[1 : 1+nameLen])
			fileType := entry[0x10]
			typeStr, ok := prodosFileTypes[fileType]
			if !ok {
				typeStr = fmt.Sprintf("$%02X", fileType)
			}
			blocksUsed := int(binary.LittleEndian.Uint16(entry[0x13:0x15]))
			eof := uint32(entry[0x15]) | (uint32(entry[0x16]) << 8) | (uint32(entry[0x17]) << 16)
			creationTime := decodeProDOSDateTime(entry[0x18:0x1C])
			access := entry[0x1E]
			modTime := decodeProDOSDateTime(entry[0x21:0x25])
			if modTime == nil {
				modTime = creationTime
			}

			// Only storage type $D is a subdirectory: its key block is a
			// directory block (file type $0F alone is not enough).
			isDir := storageType == 0x0D
			size := eof
			if isDir {
				size = 0
			}

			entries = append(entries, DiskEntry{
				Name:     name,
				Native:   proDOSNameGlyphs(entry[1 : 1+nameLen]),
				IsDir:    isDir,
				Size:     size,
				ModTime:  modTime,
				FileType: typeStr,
				Blocks:   blocksUsed,
				Locked:   (access & 0x02) == 0, // Write-disabled
				key:      int(binary.LittleEndian.Uint16(entry[0x11:0x13])),
			})
		}

		isFirstBlock = false
		curBlock = nextBlock
	}

	return entries, nil
}

func (p *ProDOSDisk) Usage() (*DiskUsageInfo, error) {
	blk2, err := p.readBlock(2)
	if err != nil {
		return nil, err
	}
	bitmapPtr := int(binary.LittleEndian.Uint16(blk2[0x27:0x29]))
	totalBlocks := int(binary.LittleEndian.Uint16(blk2[0x29:0x2B]))
	if totalBlocks == 0 {
		totalBlocks = 280
	}

	// Count free blocks in Volume Bitmap
	// Each block of bitmap holds 512 * 8 = 4096 bits
	numBitmapBlocks := (totalBlocks + 4095) / 4096
	freeBlocks := 0
	blocksChecked := 0

	for b := 0; b < numBitmapBlocks; b++ {
		bmBlk, err := p.readBlock(bitmapPtr + b)
		if err != nil {
			return nil, err
		}
		for _, byteVal := range bmBlk {
			if blocksChecked+8 <= totalBlocks {
				freeBlocks += bits.OnesCount8(byteVal)
				blocksChecked += 8
			} else {
				remaining := totalBlocks - blocksChecked
				for bit := 7; bit >= 8-remaining; bit-- {
					if (byteVal & (1 << bit)) != 0 {
						freeBlocks++
					}
				}
				blocksChecked = totalBlocks
				break
			}
		}
	}

	inUseBlocks := totalBlocks - freeBlocks
	// Report in 256-byte sectors (560 sectors on a 140K disk) or 512-byte blocks?
	// Let's check: TotalSectors in Usage is printed as "sectors".
	// On a 5.25" Apple II disk, 280 blocks = 560 sectors of 256 bytes (143,360 bytes).
	totalSectors := totalBlocks * 2
	freeSectors := freeBlocks * 2
	inUseSectors := inUseBlocks * 2

	totalBytes := int64(totalBlocks) * 512
	inUseBytes := int64(inUseBlocks) * 512
	freeBytes := int64(freeBlocks) * 512

	return &DiskUsageInfo{
		FormattedCapacity:   totalBytes,
		FormattedDesc:       "140 KiB",
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

func (p *ProDOSDisk) ReadFile(targetPath string) ([]byte, error) {
	parts := strings.Split(strings.Trim(targetPath, "/"), "/")
	curDirBlock := 2

	for idx, part := range parts {
		isLast := idx == len(parts)-1
		partLower := strings.ToLower(part)

		found := false
		var matchedEntry []byte

		blkNum := curDirBlock
		visited := make(map[int]bool)
		isFirst := true
		entryLen := 0x27
		entriesPerBlk := 0x0D

		for blkNum != 0 && !found {
			if visited[blkNum] {
				break
			}
			visited[blkNum] = true

			blk, err := p.readBlock(blkNum)
			if err != nil {
				return nil, err
			}
			nextBlk := int(binary.LittleEndian.Uint16(blk[2:4]))

			offset := 4
			for i := 0; i < entriesPerBlk; i++ {
				if offset+entryLen > 512 {
					break
				}
				entry := blk[offset : offset+entryLen]
				offset += entryLen

				if isFirst && i == 0 {
					if entry[0x1F] != 0 {
						entryLen = int(entry[0x1F])
					}
					if entry[0x20] != 0 {
						entriesPerBlk = int(entry[0x20])
					}
					continue
				}

				storageType := entry[0] >> 4
				nameLen := int(entry[0] & 0x0F)
				if storageType == 0 || nameLen == 0 {
					continue
				}
				name := DecodeAppleBytes(entry[1 : 1+nameLen])
				if strings.ToLower(name) == partLower {
					matchedEntry = make([]byte, entryLen)
					copy(matchedEntry, entry)
					found = true
					break
				}
			}
			isFirst = false
			blkNum = nextBlk
		}

		if !found {
			return nil, fmt.Errorf("file %q not found", targetPath)
		}

		storageType := matchedEntry[0] >> 4
		keyPtr := int(binary.LittleEndian.Uint16(matchedEntry[0x11:0x13]))
		eof := int(matchedEntry[0x15]) | (int(matchedEntry[0x16]) << 8) | (int(matchedEntry[0x17]) << 16)

		if !isLast {
			if storageType != 0x0D {
				return nil, fmt.Errorf("%q is not a directory", part)
			}
			curDirBlock = keyPtr
			continue
		}

		if storageType == 0x0D {
			return nil, fmt.Errorf("%q is a directory", targetPath)
		}
		return p.readFileData(storageType, keyPtr, eof)
	}

	return nil, fmt.Errorf("file %q not found", targetPath)
}

func (p *ProDOSDisk) readFileData(storageType byte, keyPtr int, eof int) ([]byte, error) {
	if eof == 0 {
		return []byte{}, nil
	}
	numBlocks := (eof + 511) / 512
	out := make([]byte, 0, numBlocks*512)

	readDataBlock := func(blkNum int) ([]byte, error) {
		if blkNum == 0 {
			return make([]byte, 512), nil // Sparse block
		}
		return p.readBlock(blkNum)
	}

	switch storageType {
	case 0x01: // Seedling (1 data block)
		b, err := readDataBlock(keyPtr)
		if err != nil {
			return nil, err
		}
		out = append(out, b...)

	case 0x02: // Sapling (1 index block -> up to 256 data blocks)
		idxBlk, err := p.readBlock(keyPtr)
		if err != nil {
			return nil, err
		}
		for i := 0; i < numBlocks && i < 256; i++ {
			dataBlkNum := int(idxBlk[i]) | (int(idxBlk[256+i]) << 8)
			b, err := readDataBlock(dataBlkNum)
			if err != nil {
				return nil, err
			}
			out = append(out, b...)
		}

	case 0x03: // Tree (master index block -> index blocks -> data blocks)
		masterBlk, err := p.readBlock(keyPtr)
		if err != nil {
			return nil, err
		}
		blocksRead := 0
		for m := 0; m < 256 && blocksRead < numBlocks; m++ {
			subIdxNum := int(masterBlk[m]) | (int(masterBlk[256+m]) << 8)
			if subIdxNum == 0 {
				for i := 0; i < 256 && blocksRead < numBlocks; i++ {
					out = append(out, make([]byte, 512)...)
					blocksRead++
				}
				continue
			}
			subIdxBlk, err := p.readBlock(subIdxNum)
			if err != nil {
				return nil, err
			}
			for i := 0; i < 256 && blocksRead < numBlocks; i++ {
				dataBlkNum := int(subIdxBlk[i]) | (int(subIdxBlk[256+i]) << 8)
				b, err := readDataBlock(dataBlkNum)
				if err != nil {
					return nil, err
				}
				out = append(out, b...)
				blocksRead++
			}
		}

	default:
		return nil, fmt.Errorf("unsupported ProDOS storage type: 0x%X", storageType)
	}

	if len(out) > eof {
		out = out[:eof]
	}
	return out, nil
}

func (p *ProDOSDisk) Dump(w io.Writer) error {
	hdr := p.Header()
	volName := ""
	if hdr != nil {
		volName = hdr.Name
	}
	orderStr := "ProDOS order (.po)"
	if p.dosOrder {
		orderStr = "DOS 3.3 sector order (.do/.dsk)"
	}

	fmt.Fprintf(w, "Apple II ProDOS Disk Image Dump\n")
	fmt.Fprintf(w, "  Volume Name: %q\n", volName)
	fmt.Fprintf(w, "  Sector Order: %s\n", orderStr)

	usage, err := p.Usage()
	if err == nil {
		fmt.Fprintf(w, "  Formatted Capacity: %d bytes (%s)\n", usage.FormattedCapacity, usage.FormattedDesc)
		fmt.Fprintf(w, "  Unformatted Capacity: %d bytes (%s)\n", usage.UnformattedCapacity, usage.UnformattedDesc)
		fmt.Fprintf(w, "  In-use: %d sectors, Free: %d sectors, Total: %d sectors\n",
			usage.InUseSectors, usage.FreeSectors, usage.TotalSectors)
	}

	var tree bytes.Buffer
	n := p.dumpDir(&tree, 2, "", 0, map[int]bool{})
	fmt.Fprintf(w, "  Directory Entries (%d):\n", n)
	_, err = w.Write(tree.Bytes())
	return err
}

// Format describes the ProDOS filesystem and geometry.
func (p *ProDOSDisk) Format() *util.FormatInfo {
	return &util.FormatInfo{
		Platform:   "Apple II",
		Filesystem: "ProDOS",
		Tracks:     35,
		Heads:      1,
		Sectors:    "16",
		SectorSize: 256,
	}
}
