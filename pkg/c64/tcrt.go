package c64

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/fredsa/retro-disk-tool/cmd/rdt/pkg/util"
)

const (
	tcrtHeaderLen = 216
	tcrtMaxFlash  = 0x200000
)

// TCRTFile is a tapecart image: a 216 byte header (fastload parameters and
// loader) followed by up to 2 MiB of flash memory. If the flash holds a
// TapcrtFileSys directory (as written by the tapecart browser), its files
// are listed; otherwise the fastload program is.
type TCRTFile struct {
	version  uint16
	dataAddr int
	dataLen  int
	callAddr uint16
	name     string
	flags    byte
	flash    []byte
	hasFS    bool
	files    []tcrtEntry
}

type tcrtEntry struct {
	name    string
	rawName []byte
	kind    byte
	start   int
	size    int
	load    uint16
}

// decodeLowerPETSCII decodes text shown in the C64's lowercase character
// set, where $41-$5A are lowercase and $61-$7A / $C1-$DA uppercase letters.
func decodeLowerPETSCII(b []byte) string {
	var sb strings.Builder
	for _, c := range b {
		switch {
		case c >= 0x41 && c <= 0x5A:
			sb.WriteByte(c + 0x20)
		case c >= 0x61 && c <= 0x7A:
			sb.WriteByte(c - 0x20)
		case c >= 0xC1 && c <= 0xDA:
			sb.WriteByte(c - 0x80)
		case c >= 0x20 && c < 0x7F:
			sb.WriteByte(c)
		default:
			fmt.Fprintf(&sb, "\\x%02X", c)
		}
	}
	return sb.String()
}

// OpenTCRT parses a tapecart image. The header's flash length must account
// for the rest of the file exactly.
func OpenTCRT(r io.ReaderAt, size int64) (*TCRTFile, error) {
	if size < tcrtHeaderLen {
		return nil, fmt.Errorf("TCRT file too small: %d bytes", size)
	}
	raw := make([]byte, size)
	if _, err := r.ReadAt(raw, 0); err != nil {
		return nil, fmt.Errorf("failed to read TCRT file: %w", err)
	}
	if string(raw[:16]) != "tapecartImage\r\n\x1a" {
		return nil, fmt.Errorf("invalid TCRT signature")
	}
	t := &TCRTFile{
		version:  binary.LittleEndian.Uint16(raw[16:]),
		dataAddr: int(binary.LittleEndian.Uint16(raw[18:])),
		dataLen:  int(binary.LittleEndian.Uint16(raw[20:])),
		callAddr: binary.LittleEndian.Uint16(raw[22:]),
		name:     strings.TrimRight(DecodeBytes(raw[24:40]), " "),
		flags:    raw[40],
	}
	if t.version != 1 {
		return nil, fmt.Errorf("unsupported TCRT version %d", t.version)
	}
	n := int(binary.LittleEndian.Uint32(raw[212:]))
	if n > tcrtMaxFlash || tcrtHeaderLen+n != len(raw) {
		return nil, fmt.Errorf("TCRT size mismatch: header says %d bytes of flash, file has %d", n, len(raw)-tcrtHeaderLen)
	}
	t.flash = raw[tcrtHeaderLen:]
	if t.dataAddr+t.dataLen > len(t.flash) {
		return nil, fmt.Errorf("TCRT fastload block $%04X+%d extends beyond the flash", t.dataAddr, t.dataLen)
	}
	if bytes.HasPrefix(t.flash[min(2, len(t.flash)):], []byte("TapcrtFileSys")) {
		if err := t.readFS(); err != nil {
			return nil, err
		}
	}
	return t, nil
}

// readFS parses the TapcrtFileSys directory in the first 4 KiB of flash.
func (t *TCRTFile) readFS() error {
	t.hasFS = true
	for i := 0; i < 127; i++ {
		e := t.flash[15+32*i : 47+32*i]
		if e[16] == 0xFF {
			break
		}
		if e[16] == 0xF0 {
			continue // separator
		}
		raw := bytes.TrimRight(e[:16], "\x00")
		f := tcrtEntry{
			name:    decodeLowerPETSCII(raw),
			rawName: raw,
			kind:    e[16],
			start:   int(binary.LittleEndian.Uint16(e[17:])) << 8,
			size:    int(e[19]) | int(e[20])<<8 | int(e[21])<<16,
			load:    binary.LittleEndian.Uint16(e[22:]),
		}
		if f.start+f.size > len(t.flash) || f.start < 0x1000 {
			return fmt.Errorf("TapcrtFileSys entry %q ($%06X+%d) lies outside the flash", f.name, f.start, f.size)
		}
		t.files = append(t.files, f)
	}
	sorted := append([]tcrtEntry{}, t.files...)
	sort.Slice(sorted, func(a, b int) bool { return sorted[a].start < sorted[b].start })
	for i := 1; i < len(sorted); i++ {
		if prev := sorted[i-1]; prev.start+prev.size > sorted[i].start {
			return fmt.Errorf("TapcrtFileSys entries %q and %q overlap", prev.name, sorted[i].name)
		}
	}
	return nil
}

// isProgram reports whether a file is a program whose load address is kept
// in the directory entry rather than in its data.
func (f tcrtEntry) isProgram() bool { return f.kind < 0x80 && f.load != 0xFFFF }

func (f tcrtEntry) typeName() string {
	switch {
	case f.kind == 0xFE:
		return "system"
	case f.kind < 0x40:
		return "program"
	case f.kind < 0x80:
		return "bundle"
	}
	return "data"
}

// content returns a file; programs get their 2 byte load address, like a
// PRG file.
func (t *TCRTFile) content(f tcrtEntry) []byte {
	data := t.flash[f.start : f.start+f.size]
	if !f.isProgram() {
		return data
	}
	return append([]byte{byte(f.load), byte(f.load >> 8)}, data...)
}

// Header returns the name shown when loading from tape.
func (t *TCRTFile) Header() *DiskHeaderInfo { return &DiskHeaderInfo{Name: t.name} }

// List returns the flash filesystem's files, or the fastload program.
func (t *TCRTFile) List() ([]diskEntryCompat, error) {
	if !t.hasFS {
		return []diskEntryCompat{{Name: t.name, Size: uint32(t.dataLen)}}, nil
	}
	res := make([]diskEntryCompat, len(t.files))
	for i, f := range t.files {
		size := f.size
		if f.isProgram() {
			size += 2
		}
		res[i] = diskEntryCompat{Name: f.name, Size: uint32(size)}
	}
	return res, nil
}

// Usage reports no disk usage.
func (t *TCRTFile) Usage() (*DiskUsageInfo, error) { return &DiskUsageInfo{HasUsage: false}, nil }

// ReadFile returns a file from the flash filesystem, or the fastload block
// (which starts with its load address).
func (t *TCRTFile) ReadFile(name string) ([]byte, error) {
	name = strings.TrimPrefix(name, "/")
	if !t.hasFS {
		if strings.EqualFold(name, t.name) {
			return t.flash[t.dataAddr : t.dataAddr+t.dataLen], nil
		}
		return nil, fmt.Errorf("file %q not found", name)
	}
	for _, f := range t.files {
		if strings.EqualFold(f.name, name) {
			return t.content(f), nil
		}
	}
	return nil, fmt.Errorf("file %q not found", name)
}

func (t *TCRTFile) describe() string {
	return fmt.Sprintf("TCRT (tapecart image, version %d, %d KiB flash)", t.version, (len(t.flash)+1023)/1024)
}

// Format describes the image.
func (t *TCRTFile) Format() *util.FormatInfo {
	f := &util.FormatInfo{Platform: "Commodore 64", Image: t.describe()}
	if t.hasFS {
		f.Filesystem = "TapcrtFileSys"
	}
	return f
}

// Dump writes the header and flash contents.
func (t *TCRTFile) Dump(w io.Writer) error {
	fmt.Fprintf(w, "Commodore 64 Tapecart Image (TCRT) Dump\n")
	fmt.Fprintf(w, "  Image: %s\n", t.describe())
	fmt.Fprintf(w, "  Name: %q, Flags: $%02X (custom loader: %v)\n", t.name, t.flags, t.flags&1 != 0)
	fmt.Fprintf(w, "  Fastload block: flash $%06X, %d bytes, call address $%04X\n", t.dataAddr, t.dataLen, t.callAddr)
	if !t.hasFS {
		fmt.Fprintf(w, "  Flash: %d bytes, no TapcrtFileSys directory (data is read by the program's own loader)\n", len(t.flash))
		return nil
	}
	fmt.Fprintf(w, "  TapcrtFileSys directory (%d files):\n", len(t.files))
	for _, f := range t.files {
		fmt.Fprintf(w, "    %-18q %-7s type $%02X  flash $%06X  %7d bytes  load $%04X\n", f.name, f.typeName(), f.kind, f.start, f.size, f.load)
	}
	return nil
}
