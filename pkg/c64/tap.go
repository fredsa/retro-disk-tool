package c64

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"strings"

	"github.com/fredsa/retro-disk-tool/cmd/rdt/pkg/util"
)

// TAPFile is a raw tape image (pulse lengths). Files saved with the
// standard CBM ROM loader are decoded; other pulses (leaders, gaps and
// turbo loader data) are counted.
type TAPFile struct {
	version  byte
	machine  byte
	video    byte
	pulses   int
	romBytes int // pulses that belong to ROM loader blocks
	blocks   []tapBlock
	files    []tapFile
	warnings []string
}

type tapBlock struct {
	payload []byte
	repeat  bool // second copy (countdown $09..$01)
	ok      bool // checksum matched
	pulse   int  // index of the first pulse
}

type tapFile struct {
	name     string
	rawName  []byte
	kind     byte // header type: 1 relocatable program, 3 program, 4 data file
	start    uint16
	end      uint16
	data     []byte // nil if the data block was not found
	dataOK   bool
	position int // pulse index of the header
}

var tapMachines = []string{"C64", "VIC-20", "C16/Plus4", "PET", "C5x0", "C6x0/C7x0"}
var tapVideo = []string{"PAL", "NTSC", "old NTSC", "PAL-N"}

// OpenTAP parses a TAP image. The header's data size must account for the
// rest of the file exactly.
func OpenTAP(r io.ReaderAt, size int64) (*TAPFile, error) {
	if size < 20 {
		return nil, fmt.Errorf("TAP file too small: %d bytes", size)
	}
	raw := make([]byte, size)
	if _, err := r.ReadAt(raw, 0); err != nil {
		return nil, fmt.Errorf("failed to read TAP file: %w", err)
	}
	if !bytes.HasPrefix(raw, []byte("C64-TAPE-RAW")) && !bytes.HasPrefix(raw, []byte("C16-TAPE-RAW")) {
		return nil, fmt.Errorf("invalid TAP signature: %q", raw[:12])
	}
	t := &TAPFile{version: raw[12], machine: raw[13], video: raw[14]}
	if t.version > 2 {
		return nil, fmt.Errorf("unsupported TAP version %d", t.version)
	}
	n := int(binary.LittleEndian.Uint32(raw[16:20]))
	if 20+n != len(raw) {
		return nil, fmt.Errorf("TAP size mismatch: header says %d bytes of pulses, file has %d", n, len(raw)-20)
	}

	// Pulse lengths in units of 8 CPU cycles.
	var pulses []int
	for p := 20; p < len(raw); {
		v := int(raw[p])
		p++
		if v == 0 {
			if t.version == 0 {
				v = 256
			} else {
				if p+3 > len(raw) {
					return nil, fmt.Errorf("truncated long pulse at offset %d", p-1)
				}
				v = int(raw[p]) | int(raw[p+1])<<8 | int(raw[p+2])<<16
				v = (v + 7) / 8
				p += 3
			}
		}
		pulses = append(pulses, v)
	}
	t.pulses = len(pulses)
	t.decode(pulses)
	return t, nil
}

// pulse classes of the ROM loader (nominal $30, $42, $56 units).
const (
	pulseOther = iota
	pulseShort
	pulseMedium
	pulseLong
)

func classify(v int) int {
	switch {
	case v >= 0x24 && v <= 0x36:
		return pulseShort
	case v >= 0x37 && v <= 0x49:
		return pulseMedium
	case v >= 0x4A && v <= 0x64:
		return pulseLong
	}
	return pulseOther
}

// decode finds ROM loader bytes: a long+medium marker, 8 data bits (LSB
// first; short+medium = 0, medium+short = 1) and an odd parity bit. Runs of
// bytes form blocks, which start with a countdown ($89..$81 for the first
// copy, $09..$01 for the repeat) and end with an XOR checksum.
func (t *TAPFile) decode(pulses []int) {
	c := make([]int, len(pulses))
	for i, v := range pulses {
		c[i] = classify(v)
	}
	var cur []byte
	curStart := 0
	flush := func() {
		if len(cur) >= 11 {
			t.addBlock(cur, curStart)
		}
		cur = nil
	}
	for i := 0; i+1 < len(c); {
		if c[i] != pulseLong || c[i+1] != pulseMedium || i+20 > len(c) {
			if c[i] == pulseLong && c[i+1] == pulseShort {
				flush() // end of data marker
				i += 2
				continue
			}
			if len(cur) > 0 && c[i] != pulseShort {
				flush()
			}
			i++
			continue
		}
		var b byte
		ones, ok := 0, true
		for k := 0; k < 9; k++ {
			p0, p1 := c[i+2+2*k], c[i+3+2*k]
			var v int
			switch {
			case p0 == pulseShort && p1 == pulseMedium:
				v = 0
			case p0 == pulseMedium && p1 == pulseShort:
				v = 1
			default:
				ok = false
			}
			if k < 8 {
				b |= byte(v) << k
			}
			ones += v
		}
		if !ok || ones%2 == 0 {
			flush()
			i++
			continue
		}
		if len(cur) == 0 {
			curStart = i
		}
		cur = append(cur, b)
		i += 20
	}
	flush()
	t.assemble()
}

func (t *TAPFile) addBlock(b []byte, pulse int) {
	repeat := false
	switch {
	case bytes.Equal(b[:9], []byte{0x89, 0x88, 0x87, 0x86, 0x85, 0x84, 0x83, 0x82, 0x81}):
	case bytes.Equal(b[:9], []byte{0x09, 0x08, 0x07, 0x06, 0x05, 0x04, 0x03, 0x02, 0x01}):
		repeat = true
	default:
		return
	}
	t.romBytes += 20*len(b) + 2 // bytes and the end of data marker
	payload := b[9 : len(b)-1]
	var sum byte
	for _, x := range payload {
		sum ^= x
	}
	t.blocks = append(t.blocks, tapBlock{payload: payload, repeat: repeat, ok: sum == b[len(b)-1], pulse: pulse})
}

// assemble pairs header blocks with the data blocks that follow them,
// using whichever copy (first or repeat) has a good checksum.
func (t *TAPFile) assemble() {
	var cur *tapFile
	for i := 0; i < len(t.blocks); i++ {
		blk := t.blocks[i]
		// Prefer the repeat copy if the first copy is bad.
		if !blk.repeat && !blk.ok && i+1 < len(t.blocks) && t.blocks[i+1].repeat && t.blocks[i+1].ok {
			blk = t.blocks[i+1]
		}
		if blk.repeat && i > 0 && !t.blocks[i-1].repeat {
			continue // repeat of a block already handled
		}
		p := blk.payload
		if len(p) == 192 && p[0] >= 1 && p[0] <= 5 && p[0] != 2 {
			if p[0] == 5 {
				cur = nil // end of tape marker
				continue
			}
			raw := bytes.TrimRight(trimPadding(p[5:21]), " ")
			t.files = append(t.files, tapFile{
				name:     DecodeBytes(raw),
				rawName:  raw,
				kind:     p[0],
				start:    binary.LittleEndian.Uint16(p[1:3]),
				end:      binary.LittleEndian.Uint16(p[3:5]),
				position: blk.pulse,
			})
			cur = &t.files[len(t.files)-1]
			if !blk.ok {
				t.warnings = append(t.warnings, fmt.Sprintf("header of %q has a checksum error", cur.name))
			}
			continue
		}
		if cur != nil && cur.data == nil && cur.kind != 4 {
			cur.data, cur.dataOK = p, blk.ok
			if want := int(cur.end) - int(cur.start); want != len(p) {
				t.warnings = append(t.warnings, fmt.Sprintf("%q: data block has %d bytes, header says %d", cur.name, len(p), want))
			}
			if !blk.ok {
				t.warnings = append(t.warnings, fmt.Sprintf("data of %q has a checksum error", cur.name))
			}
		}
	}
	for _, f := range t.files {
		if f.data == nil && f.kind != 4 {
			t.warnings = append(t.warnings, fmt.Sprintf("%q: data block not found", f.name))
		}
	}
	if other := t.pulses - t.romBytes; other > t.pulses/10 {
		t.warnings = append(t.warnings, fmt.Sprintf("%d of %d pulses (%d%%) are not CBM ROM loader blocks (leaders, gaps, or data in another format such as a turbo loader); files stored that way are not listed",
			other, t.pulses, other*100/t.pulses))
	}
}

func (f tapFile) content() []byte {
	return append([]byte{byte(f.start), byte(f.start >> 8)}, f.data...)
}

// Header returns nil: tapes have no name.
func (t *TAPFile) Header() *DiskHeaderInfo { return nil }

// List returns the ROM loader files; program sizes include the 2 byte load
// address.
func (t *TAPFile) List() ([]diskEntryCompat, error) {
	var res []diskEntryCompat
	for _, f := range t.files {
		e := diskEntryCompat{Name: f.name, Native: petsciiQuotedGlyphs(f.rawName)}
		switch {
		case f.kind == 4:
		case f.data == nil:
			e.Corrupt, e.Error = true, "data block not found"
		default:
			e.Size = uint32(len(f.data) + 2)
			if !f.dataOK {
				e.Corrupt, e.Error = true, "data block checksum error"
			}
		}
		res = append(res, e)
	}
	return res, nil
}

// Usage reports no disk usage.
func (t *TAPFile) Usage() (*DiskUsageInfo, error) { return &DiskUsageInfo{HasUsage: false}, nil }

// ReadFile returns a program with its 2 byte load address, like a PRG file.
func (t *TAPFile) ReadFile(name string) ([]byte, error) {
	for _, f := range t.files {
		if strings.EqualFold(f.name, strings.TrimPrefix(name, "/")) {
			if f.data == nil {
				return nil, fmt.Errorf("data of %q not found on tape", name)
			}
			return f.content(), nil
		}
	}
	return nil, fmt.Errorf("file %q not found", name)
}

// Warnings describes decoding problems and undecoded pulses.
func (t *TAPFile) Warnings() []string { return t.warnings }

func (t *TAPFile) describe() string {
	m, v := fmt.Sprintf("machine %d", t.machine), fmt.Sprintf("video %d", t.video)
	if int(t.machine) < len(tapMachines) {
		m = tapMachines[t.machine]
	}
	if int(t.video) < len(tapVideo) {
		v = tapVideo[t.video]
	}
	return fmt.Sprintf("TAP (raw tape image, version %d, %s, %s, %d pulses)", t.version, m, v, t.pulses)
}

// Format describes the tape image.
func (t *TAPFile) Format() *util.FormatInfo {
	return &util.FormatInfo{Platform: "Commodore 64", Image: t.describe()}
}

// Dump writes the decoded blocks and files.
func (t *TAPFile) Dump(w io.Writer) error {
	fmt.Fprintf(w, "Commodore Tape Image (TAP) Dump\n")
	fmt.Fprintf(w, "  Image: %s\n", t.describe())
	fmt.Fprintf(w, "  ROM loader block pulses: %d of %d\n", t.romBytes, t.pulses)
	fmt.Fprintf(w, "  ROM loader blocks (%d):\n", len(t.blocks))
	for _, b := range t.blocks {
		copyName, sum := "first ", "ok"
		if b.repeat {
			copyName = "repeat"
		}
		if !b.ok {
			sum = "CHECKSUM ERROR"
		}
		fmt.Fprintf(w, "    pulse %7d: %s copy, %5d bytes, checksum %s\n", b.pulse, copyName, len(b.payload), sum)
	}
	kinds := map[byte]string{1: "PRG (relocatable)", 3: "PRG", 4: "SEQ"}
	fmt.Fprintf(w, "  Files (%d):\n", len(t.files))
	for _, f := range t.files {
		fmt.Fprintf(w, "    %-18q %-17s $%04X-$%04X  %d bytes\n", f.name, kinds[f.kind], f.start, f.end, len(f.data))
	}
	for _, msg := range t.warnings {
		fmt.Fprintf(w, "  Warning: %s\n", msg)
	}
	return nil
}
