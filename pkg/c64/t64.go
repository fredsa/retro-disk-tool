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

// T64File is a T64 tape archive: a 64 byte header, a table of 32 byte
// entries, then the file data.
type T64File struct {
	data     []byte
	version  uint16
	maxEntry int
	tapeName string
	files    []t64Entry
	warnings []string
}

type t64Entry struct {
	name     string
	rawName  []byte
	kind     byte // 1 = normal tape file, 3 = memory snapshot
	fileType byte // C64 file type ($82 = PRG, ...)
	start    uint16
	offset   int
	length   int
}

// OpenT64 parses a T64 archive. The entries' data must account for every
// byte after the entry table.
func OpenT64(r io.ReaderAt, size int64) (*T64File, error) {
	if size < 64 {
		return nil, fmt.Errorf("T64 file too small: %d bytes", size)
	}
	data := make([]byte, size)
	if _, err := r.ReadAt(data, 0); err != nil {
		return nil, fmt.Errorf("failed to read T64 file: %w", err)
	}
	if !bytes.HasPrefix(data, []byte("C64")) {
		return nil, fmt.Errorf("invalid T64 signature: %q", bytes.TrimRight(data[:32], "\x00"))
	}
	t := &T64File{
		data:     data,
		version:  binary.LittleEndian.Uint16(data[32:34]),
		maxEntry: int(binary.LittleEndian.Uint16(data[34:36])),
		tapeName: strings.TrimRight(DecodeBytes(trimPadding(data[40:64])), " "),
	}
	if t.version != 0x0100 && t.version != 0x0101 {
		return nil, fmt.Errorf("unsupported T64 version $%04X", t.version)
	}
	dirEnd := 64 + t.maxEntry*32
	if t.maxEntry == 0 || dirEnd > len(data) {
		return nil, fmt.Errorf("invalid T64 directory size: %d entries", t.maxEntry)
	}

	type ends struct {
		entry int
		end   uint16
	}
	var declared []ends
	for i := 0; i < t.maxEntry; i++ {
		e := data[64+i*32 : 96+i*32]
		if e[0] == 0 {
			continue // free entry
		}
		raw := bytes.TrimRight(trimPadding(e[16:32]), " ")
		t.files = append(t.files, t64Entry{
			name:     DecodeBytes(raw),
			rawName:  raw,
			kind:     e[0],
			fileType: e[1],
			start:    binary.LittleEndian.Uint16(e[2:4]),
			offset:   int(binary.LittleEndian.Uint32(e[8:12])),
		})
		declared = append(declared, ends{len(t.files) - 1, binary.LittleEndian.Uint16(e[4:6])})
	}
	if len(t.files) == 0 {
		return nil, fmt.Errorf("T64 archive has no files")
	}

	// Each file runs to the next file's data (or the end of the archive).
	order := make([]int, len(t.files))
	for i := range order {
		order[i] = i
	}
	sort.Slice(order, func(a, b int) bool { return t.files[order[a]].offset < t.files[order[b]].offset })
	if first := t.files[order[0]].offset; first != dirEnd {
		return nil, fmt.Errorf("T64 file data starts at offset %d, not right after the directory (%d)", first, dirEnd)
	}
	for k, i := range order {
		end := len(data)
		if k+1 < len(order) {
			end = t.files[order[k+1]].offset
		}
		if end <= t.files[i].offset {
			if k+1 == len(order) {
				return nil, fmt.Errorf("T64 entry %q has no data (offset %d, file size %d)", t.files[i].name, t.files[i].offset, len(data))
			}
			return nil, fmt.Errorf("T64 entries %q and %q overlap", t.files[i].name, t.files[order[k+1]].name)
		}
		t.files[i].length = end - t.files[i].offset
	}
	for _, d := range declared {
		f := t.files[d.entry]
		if want := int(f.start) + f.length; int(d.end) != want && !(d.end == 0 && want == 0x10000) {
			t.warnings = append(t.warnings, fmt.Sprintf("%q: end address $%04X in the directory does not match its %d bytes of data (want $%04X)",
				f.name, d.end, f.length, want&0xFFFF))
		}
	}
	return t, nil
}

func (f t64Entry) typeName() string {
	if f.kind == 3 {
		return "FRZ"
	}
	if name, ok := fileTypes[f.fileType&0x07]; ok && f.fileType != 0x01 && f.fileType != 0x00 {
		return name
	}
	return "PRG" // old archives store $00 or $01 for programs
}

// content returns the file as stored on disk: load address, then data.
func (f t64Entry) content(data []byte) []byte {
	out := []byte{byte(f.start), byte(f.start >> 8)}
	return append(out, data[f.offset:f.offset+f.length]...)
}

// Header returns the tape name.
func (t *T64File) Header() *DiskHeaderInfo {
	if t.tapeName == "" {
		return nil
	}
	return &DiskHeaderInfo{Name: t.tapeName}
}

// List returns the archived files; sizes include the 2 byte load address.
func (t *T64File) List() ([]diskEntryCompat, error) {
	res := make([]diskEntryCompat, len(t.files))
	for i, f := range t.files {
		res[i] = diskEntryCompat{Name: f.name, Size: uint32(f.length + 2), Native: petsciiQuotedGlyphs(f.rawName)}
	}
	return res, nil
}

// Usage reports no disk usage: a T64 is an archive, not a disk.
func (t *T64File) Usage() (*DiskUsageInfo, error) { return &DiskUsageInfo{HasUsage: false}, nil }

// ReadFile returns a file with its 2 byte load address, like a PRG file.
func (t *T64File) ReadFile(name string) ([]byte, error) {
	for _, f := range t.files {
		if strings.EqualFold(f.name, strings.TrimPrefix(name, "/")) {
			return f.content(t.data), nil
		}
	}
	return nil, fmt.Errorf("file %q not found", name)
}

// Warnings describes inconsistent directory entries.
func (t *T64File) Warnings() []string { return t.warnings }

// Format describes the archive.
func (t *T64File) Format() *util.FormatInfo {
	return &util.FormatInfo{
		Platform: "Commodore 64",
		Image:    fmt.Sprintf("T64 (tape archive, version %d.%d, %d of %d entries used)", t.version>>8, t.version&0xFF, len(t.files), t.maxEntry),
	}
}

// Dump writes a description of the archive.
func (t *T64File) Dump(w io.Writer) error {
	fmt.Fprintf(w, "Commodore 64 Tape Archive (T64) Dump\n")
	fmt.Fprintf(w, "  Signature: %q\n", bytes.TrimRight(t.data[:32], "\x00"))
	fmt.Fprintf(w, "  Version: %d.%d, Entries: %d of %d used, Tape name: %q\n", t.version>>8, t.version&0xFF, len(t.files), t.maxEntry, t.tapeName)
	for _, f := range t.files {
		fmt.Fprintf(w, "    %-18q %s  $%04X-$%04X  %6d bytes at offset %d\n",
			f.name, f.typeName(), f.start, int(f.start)+f.length, f.length, f.offset)
	}
	for _, msg := range t.warnings {
		fmt.Fprintf(w, "  Warning: %s\n", msg)
	}
	return nil
}
