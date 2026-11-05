package c64

import (
	"encoding/binary"
	"fmt"
	"io"
	"path/filepath"
	"strings"
)

type CRTChip struct {
	Bank       uint16
	LoadAddr   uint16
	RomSize    uint16
	ChipType   uint16
	DataOffset int
}

type CRTFile struct {
	filename      string
	headerSize    uint32
	version       string
	cartridgeType uint16
	exrom         byte
	game          byte
	cartridgeName string
	chips         []CRTChip
	totalSize     int64
	data          []byte
}

func OpenCRT(filename string, r io.ReaderAt, size int64) (*CRTFile, error) {
	if size < 0x40 {
		return nil, fmt.Errorf("CRT file too small: %d bytes (minimum 64 byte header)", size)
	}

	data := make([]byte, size)
	if _, err := r.ReadAt(data, 0); err != nil {
		return nil, fmt.Errorf("failed to read CRT file: %w", err)
	}

	sig := string(data[0:16])
	if !strings.HasPrefix(sig, "C64 CARTRIDGE") {
		return nil, fmt.Errorf("invalid CRT signature: %q", sig)
	}

	headerSize := binary.BigEndian.Uint32(data[0x10:0x14])
	verMajor := data[0x14]
	verMinor := data[0x15]
	cartType := binary.BigEndian.Uint16(data[0x16:0x18])
	exrom := data[0x18]
	game := data[0x19]
	rawName := data[0x20:0x40]
	nameEnd := len(rawName)
	for nameEnd > 0 && (rawName[nameEnd-1] == 0x00 || rawName[nameEnd-1] == 0x20 || rawName[nameEnd-1] == 0xA0) {
		nameEnd--
	}
	cartName := DecodeBytes(rawName[:nameEnd])

	// Read CHIP packets
	var chips []CRTChip
	offset := int(headerSize)
	for offset+16 <= len(data) {
		chipSig := string(data[offset : offset+4])
		if chipSig != "CHIP" {
			break
		}
		totalPacketLen := binary.BigEndian.Uint32(data[offset+4 : offset+8])
		chipType := binary.BigEndian.Uint16(data[offset+8 : offset+10])
		bank := binary.BigEndian.Uint16(data[offset+10 : offset+12])
		loadAddr := binary.BigEndian.Uint16(data[offset+12 : offset+14])
		romSize := binary.BigEndian.Uint16(data[offset+14 : offset+16])

		chips = append(chips, CRTChip{
			Bank:       bank,
			LoadAddr:   loadAddr,
			RomSize:    romSize,
			ChipType:   chipType,
			DataOffset: offset + 16,
		})

		if totalPacketLen == 0 {
			break
		}
		offset += int(totalPacketLen)
	}

	return &CRTFile{
		filename:      filename,
		headerSize:    headerSize,
		version:       fmt.Sprintf("%d.%d", verMajor, verMinor),
		cartridgeType: cartType,
		exrom:         exrom,
		game:          game,
		cartridgeName: cartName,
		chips:         chips,
		totalSize:     size,
		data:          data,
	}, nil
}

func (c *CRTFile) List() ([]diskEntryCompat, error) {
	name := c.cartridgeName
	if name == "" {
		name = filepath.Base(c.filename)
	}
	var totalRomSize uint32
	for _, chip := range c.chips {
		totalRomSize += uint32(chip.RomSize)
	}
	return []diskEntryCompat{
		{
			Name:    name,
			IsDir:   false,
			Size:    totalRomSize,
			ModTime: nil,
		},
	}, nil
}

func (c *CRTFile) Usage() (*DiskUsageInfo, error) {
	return &DiskUsageInfo{
		HasUsage: false,
	}, nil
}

func (c *CRTFile) ReadFile(targetName string) ([]byte, error) {
	name := c.cartridgeName
	if name == "" {
		name = filepath.Base(c.filename)
	}
	if strings.EqualFold(targetName, name) || strings.EqualFold(targetName, filepath.Base(c.filename)) || targetName == "" || targetName == "/" {
		var allROM []byte
		for _, chip := range c.chips {
			if chip.DataOffset+int(chip.RomSize) <= len(c.data) {
				allROM = append(allROM, c.data[chip.DataOffset:chip.DataOffset+int(chip.RomSize)]...)
			}
		}
		return allROM, nil
	}
	return nil, fmt.Errorf("file %q not found in CRT cartridge", targetName)
}

func (c *CRTFile) Dump(w io.Writer) error {
	baseName := filepath.Base(c.filename)
	fmt.Fprintf(w, "Commodore 64 Cartridge (CRT) Dump\n")
	fmt.Fprintf(w, "  File: %s\n", baseName)
	fmt.Fprintf(w, "  Cartridge Name: %q\n", c.cartridgeName)
	fmt.Fprintf(w, "  Header Version: %s, Header Size: %d bytes\n", c.version, c.headerSize)
	fmt.Fprintf(w, "  Cartridge Hardware Type ID: %d (0x%04X)\n", c.cartridgeType, c.cartridgeType)
	fmt.Fprintf(w, "  EXROM line: %d, GAME line: %d\n", c.exrom, c.game)
	fmt.Fprintf(w, "  CHIP Packets (%d):\n", len(c.chips))
	for i, ch := range c.chips {
		fmt.Fprintf(w, "    #%d: Bank %d, Load Address: 0x%04X, Size: %d bytes (Type: %d)\n",
			i+1, ch.Bank, ch.LoadAddr, ch.RomSize, ch.ChipType)
	}
	return nil
}
