// Package zx81 reads Sinclair ZX81 program files: P (memory image saved by
// SAVE, starting at the system variables) and P81 (P data preceded by the
// program name, possibly several programs in one file).
package zx81

import (
	"encoding/binary"
	"fmt"
	"io"
	"path/filepath"
	"strings"

	"github.com/fredsa/retro-disk-tool/cmd/rdt/pkg/util"
)

const (
	saveStart    = 0x4009 // VERSN, the first byte saved
	programStart = 0x407D // first BASIC line

	addrEPPC   = 0x400A
	addrDFile  = 0x400C
	addrVars   = 0x4010
	addrELine  = 0x4014
	addrNxtLin = 0x4029

	maxNameLen = 127
)

// Program is one saved ZX81 program.
type Program struct {
	Name   string
	Native []util.Glyph // name in the ZX81 character set (nil for P files)
	Data   []byte       // memory image from $4009 up to E_LINE
}

// Entry is a program listed as a file.
type Entry struct {
	Name   string
	Size   uint32
	Native []util.Glyph
}

// File is an opened P or P81 file.
type File struct {
	filename string
	p81      bool
	programs []Program
}

// Open parses a P or P81 file. The format is selected by the extension.
func Open(filename string, r io.ReaderAt, size int64) (*File, error) {
	raw := make([]byte, size)
	if _, err := r.ReadAt(raw, 0); err != nil {
		return nil, fmt.Errorf("failed to read ZX81 file: %w", err)
	}
	f := &File{filename: filename, p81: strings.EqualFold(filepath.Ext(filename), ".p81")}
	if !f.p81 {
		data, err := programData(raw)
		if err != nil {
			return nil, err
		}
		name := strings.TrimSuffix(filepath.Base(filename), filepath.Ext(filename))
		if len(data) != len(raw) {
			return nil, fmt.Errorf("P file size %d does not match program size %d (E_LINE)", len(raw), len(data))
		}
		f.programs = []Program{{Name: name, Data: data}}
		return f, nil
	}

	for len(raw) > 0 {
		n := 0
		for n < len(raw) && n < maxNameLen && raw[n]&0x80 == 0 {
			n++
		}
		if n >= len(raw) || raw[n]&0x80 == 0 {
			return nil, fmt.Errorf("invalid P81 file: program %d name not terminated", len(f.programs)+1)
		}
		native, name := decodeName(raw[:n+1])
		data, err := programData(raw[n+1:])
		if err != nil {
			return nil, fmt.Errorf("program %d %q: %w", len(f.programs)+1, name, err)
		}
		f.programs = append(f.programs, Program{Name: name, Native: native, Data: data})
		raw = raw[n+1+len(data):]
	}
	return f, nil
}

// programData validates a memory image starting at $4009 and returns it,
// trimmed to its length (E_LINE - $4009).
func programData(b []byte) ([]byte, error) {
	if len(b) < programStart-saveStart {
		return nil, fmt.Errorf("ZX81 program too small: %d bytes", len(b))
	}
	if b[0] != 0 {
		return nil, fmt.Errorf("not a ZX81 program (VERSN is %d, want 0)", b[0])
	}
	p := Program{Data: b}
	dFile, vars, eLine := p.word(addrDFile), p.word(addrVars), p.word(addrELine)
	if dFile < programStart || dFile > vars || vars >= eLine {
		return nil, fmt.Errorf("invalid ZX81 system variables (D_FILE $%04X, VARS $%04X, E_LINE $%04X)", dFile, vars, eLine)
	}
	n := int(eLine) - saveStart
	if n > len(b) {
		return nil, fmt.Errorf("truncated ZX81 program: %d bytes, E_LINE says %d", len(b), n)
	}
	return b[:n], nil
}

// word returns the 16 bit system variable at addr.
func (p *Program) word(addr int) uint16 {
	return binary.LittleEndian.Uint16(p.Data[addr-saveStart:])
}

// mem returns the program memory from addr to end.
func (p *Program) mem(addr, end int) []byte {
	if addr < saveStart || end > saveStart+len(p.Data) || addr > end {
		return nil
	}
	return p.Data[addr-saveStart : end-saveStart]
}

// Line is a BASIC program line.
type Line struct {
	Number int
	Addr   int
	Body   []byte
}

// Lines returns the BASIC program lines.
func (p *Program) Lines() []Line {
	var lines []Line
	end := int(p.word(addrDFile))
	for a := programStart; a+4 <= end; {
		hdr := p.mem(a, a+4)
		num := int(binary.BigEndian.Uint16(hdr[0:2]))
		n := int(binary.LittleEndian.Uint16(hdr[2:4]))
		if num >= 0x4000 || a+4+n > end {
			break
		}
		lines = append(lines, Line{Number: num, Addr: a, Body: p.mem(a+4, a+4+n)})
		a += 4 + n
	}
	return lines
}

// AutoRun returns the line the program runs from after loading, if any.
func (p *Program) AutoRun() (int, bool) {
	next := int(p.word(addrNxtLin))
	for _, l := range p.Lines() {
		if l.Addr == next {
			return l.Number, true
		}
	}
	return 0, false
}

// List returns the programs as file entries.
func (f *File) List() ([]Entry, error) {
	res := make([]Entry, len(f.programs))
	for i, p := range f.programs {
		res[i] = Entry{Name: p.Name, Size: uint32(len(p.Data)), Native: p.Native}
	}
	return res, nil
}

// ReadFile returns the P file data (memory image from $4009) of the named
// program.
func (f *File) ReadFile(name string) ([]byte, error) {
	name = strings.TrimPrefix(name, "/")
	for _, p := range f.programs {
		if strings.EqualFold(p.Name, name) {
			return p.Data, nil
		}
	}
	if len(f.programs) == 1 && name == "" {
		return f.programs[0].Data, nil
	}
	return nil, fmt.Errorf("program %q not found", name)
}

// Format describes the file format.
func (f *File) Format() *util.FormatInfo {
	image := "P (program file, memory image from $4009)"
	if f.p81 {
		s := ""
		if len(f.programs) != 1 {
			s = "s"
		}
		image = fmt.Sprintf("P81 (named program file, %d program%s)", len(f.programs), s)
	}
	return &util.FormatInfo{
		Platform: "Sinclair ZX81",
		Image:    image,
	}
}

// Dump writes the system variables and BASIC listing of each program.
func (f *File) Dump(w io.Writer) error {
	fmt.Fprintf(w, "Sinclair ZX81 Program File Dump\n")
	fmt.Fprintf(w, "  File: %s\n", filepath.Base(f.filename))
	for _, p := range f.programs {
		dFile, vars, eLine := p.word(addrDFile), p.word(addrVars), p.word(addrELine)
		fmt.Fprintf(w, "  Program: %q\n", p.Name)
		fmt.Fprintf(w, "    Size: %d bytes ($%04X - $%04X)\n", len(p.Data), saveStart, int(eLine)-1)
		fmt.Fprintf(w, "    BASIC: %d bytes, Display file: %d bytes at $%04X (%s), Variables: %d bytes at $%04X\n",
			int(dFile)-programStart, vars-dFile, dFile, displayKind(int(vars-dFile)), eLine-vars, vars)
		fmt.Fprintf(w, "    RAM required: %s\n", ramRequired(int(eLine)))
		fmt.Fprintf(w, "    Current line (E_PPC): %d\n", p.word(addrEPPC))
		if n, ok := p.AutoRun(); ok {
			fmt.Fprintf(w, "    Auto-run: line %d\n", n)
		} else {
			fmt.Fprintf(w, "    Auto-run: no\n")
		}
		lines := p.Lines()
		fmt.Fprintf(w, "    Listing (%d lines):\n", len(lines))
		for _, l := range lines {
			fmt.Fprintf(w, "    %4d %s\n", l.Number, listLine(l.Body))
		}
	}
	return nil
}

// displayKind describes the display file: fully expanded (24 lines of 32
// characters) or collapsed (as on a 1K ZX81).
func displayKind(n int) string {
	switch {
	case n == 1+24*33:
		return "expanded"
	case n == 25:
		return "collapsed"
	}
	return "partially collapsed"
}

func ramRequired(end int) string {
	switch {
	case end <= 0x4400:
		return "1K"
	case end <= 0x4800:
		return "2K"
	case end <= 0x8000:
		return "16K"
	}
	return "32K or more"
}
