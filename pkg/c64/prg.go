package c64

import (
	"encoding/binary"
	"fmt"
	"github.com/fredsa/retro-disk-tool/cmd/rdt/pkg/util"
	"io"
	"path/filepath"
	"strings"
)

type PRGFile struct {
	filename  string
	loadAddr  uint16
	data      []byte
	totalSize int64
}

func OpenPRG(filename string, r io.ReaderAt, size int64) (*PRGFile, error) {
	if size < 2 {
		return nil, fmt.Errorf("PRG file too small: %d bytes (minimum 2 bytes load address)", size)
	}

	data := make([]byte, size)
	if _, err := r.ReadAt(data, 0); err != nil {
		return nil, fmt.Errorf("failed to read PRG file: %w", err)
	}

	loadAddr := binary.LittleEndian.Uint16(data[0:2])
	return &PRGFile{
		filename:  filename,
		loadAddr:  loadAddr,
		data:      data[2:],
		totalSize: size,
	}, nil
}

func (p *PRGFile) List() ([]diskEntryCompat, error) {
	baseName := filepath.Base(p.filename)
	return []diskEntryCompat{
		{
			Name:    baseName,
			IsDir:   false,
			Size:    uint32(len(p.data)),
			ModTime: nil,
		},
	}, nil
}

func (p *PRGFile) Usage() (*DiskUsageInfo, error) {
	// A PRG is an executable / memory image file, not a disk container
	return &DiskUsageInfo{
		HasUsage: false,
	}, nil
}

func (p *PRGFile) ReadFile(targetName string) ([]byte, error) {
	baseName := filepath.Base(p.filename)
	if strings.EqualFold(targetName, baseName) || targetName == "" || targetName == "/" {
		return p.data, nil
	}
	return nil, fmt.Errorf("file %q not found in PRG", targetName)
}

func (p *PRGFile) Dump(w io.Writer) error {
	baseName := filepath.Base(p.filename)
	fmt.Fprintf(w, "Commodore 64 PRG File Dump\n")
	fmt.Fprintf(w, "  File: %s\n", baseName)
	fmt.Fprintf(w, "  Load Address: 0x%04X (%d)\n", p.loadAddr, p.loadAddr)
	fmt.Fprintf(w, "  Program Size: %d bytes (total file size: %d bytes)\n", len(p.data), p.totalSize)
	endAddr := int(p.loadAddr) + len(p.data)
	fmt.Fprintf(w, "  Memory Range: 0x%04X - 0x%04X\n", p.loadAddr, endAddr)
	return nil
}

// Format describes the PRG file format.
func (p *PRGFile) Format() *util.FormatInfo {
	return &util.FormatInfo{
		Platform: "Commodore 64",
		Image:    fmt.Sprintf("PRG (program file, load address $%04X)", p.loadAddr),
	}
}
