package zx81

import (
	"bytes"
	"os"
	"strings"
	"testing"

	"github.com/fredsa/retro-disk-tool/cmd/rdt/pkg/util"
)

const kong = "../../test_data/ZX81_Kong (Paul Farrow 2010) [V1-01]"

func open(t *testing.T, path string) *File {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("failed to read %s: %v", path, err)
	}
	f, err := Open(path, bytes.NewReader(raw), int64(len(raw)))
	if err != nil {
		t.Fatalf("Open(%s) failed: %v", path, err)
	}
	return f
}

func TestKong(t *testing.T) {
	p := open(t, kong+".p")
	p81 := open(t, kong+".P81")

	entries, _ := p.List()
	if len(entries) != 1 || entries[0].Name != "ZX81_Kong (Paul Farrow 2010) [V1-01]" || entries[0].Size != 12513 || entries[0].Native != nil {
		t.Errorf("P List = %+v", entries)
	}
	entries, _ = p81.List()
	if len(entries) != 1 || entries[0].Name != "ZX81 KONG" || entries[0].Size != 12513 ||
		util.GlyphsString(entries[0].Native) != "ZX81 KONG" {
		t.Errorf("P81 List = %+v", entries)
	}
	for _, g := range entries[0].Native {
		if g.Style != util.StyleNormal {
			t.Errorf("P81 name glyph %q is not normal; the terminating bit 7 is not inverse video", g.R)
		}
	}

	a, err := p.ReadFile("")
	if err != nil {
		t.Fatal(err)
	}
	b, err := p81.ReadFile("zx81 kong")
	if err != nil || !bytes.Equal(a, b) {
		t.Errorf("P81 ReadFile(zx81 kong) differs from P data (err %v)", err)
	}
	if _, err := p81.ReadFile("NOPE"); err == nil {
		t.Error("ReadFile(NOPE) succeeded, want error")
	}

	prog := p81.programs[0]
	if n, ok := prog.AutoRun(); !ok || n != 30 {
		t.Errorf("AutoRun = %d, %v; want 30, true", n, ok)
	}
	var got []string
	for _, l := range prog.Lines() {
		got = append(got, listLine(l.Body))
	}
	want := []string{
		"REM       ZX81 KONG                 (ZX80 + 8K BASIC ROM)",
		`RAND USR VAL "16574"`,
		`SAVE "ZX81 KONG"`,
		"RUN",
	}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("listing =\n%s\nwant\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}

	if fm := p81.Format(); fm.Platform != "Sinclair ZX81" || !strings.HasPrefix(fm.Image, "P81 (named program file, 1 program)") {
		t.Errorf("Format = %+v", fm)
	}
	var buf bytes.Buffer
	if err := p81.Dump(&buf); err != nil {
		t.Fatal(err)
	}
	for _, w := range []string{"Auto-run: line 30", "RAM required: 16K", "(collapsed)", `  10 RAND USR VAL "16574"`} {
		if !strings.Contains(buf.String(), w) {
			t.Errorf("Dump missing %q:\n%s", w, buf.String())
		}
	}
}

func TestMultiProgramP81(t *testing.T) {
	raw, err := os.ReadFile(kong + ".P81")
	if err != nil {
		t.Fatal(err)
	}
	// Second program named "AB" (the last character has bit 7 set).
	two := append(append(append([]byte{}, raw...), 0x26, 0xA7), raw[9:]...)
	f, err := Open("two.p81", bytes.NewReader(two), int64(len(two)))
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	entries, _ := f.List()
	if len(entries) != 2 || entries[1].Name != "AB" || entries[1].Size != 12513 {
		t.Errorf("List = %+v", entries)
	}
}

func TestListLine(t *testing.T) {
	// 10 PRINT AT 1,2;"[inverse A]";PI**2: number markers hide the binary value.
	body := []byte{0xF5, 0xC1, 0x1D, 0x7E, 0x81, 0, 0, 0, 0, 0x1A, 0x1E, 0x7E, 0x82, 0, 0, 0, 0,
		0x19, 0x0B, 0xA6, 0x0B, 0x19, 0x42, 0xD8, 0x1E, 0x76}
	if got, want := listLine(body), `PRINT AT 1,2;"A";PI**2`; got != want {
		t.Errorf("listLine = %q, want %q", got, want)
	}
	for want, body := range map[string][]byte{
		"IF A>=1 THEN GOTO 10":   {0xFA, 0x26, 0xDC, 0x1D, 0xDE, 0xEC, 0x1D, 0x1C, 0x76},
		"LET A=INT (RND*10)":     {0xF1, 0x26, 0x14, 0xCF, 0x10, 0x40, 0x17, 0x1D, 0x1C, 0x11},
		"IF INKEY$ <>\"\" AND X": {0xFA, 0x41, 0x00, 0xDD, 0xC0, 0xDA, 0x3D},
	} {
		if got := listLine(body); got != want {
			t.Errorf("listLine = %q, want %q", got, want)
		}
	}
}

func TestOpenErrors(t *testing.T) {
	for name, data := range map[string][]byte{
		"short.p":    make([]byte, 10),
		"versn.p":    append([]byte{0xFF}, make([]byte, 200)...),
		"zeros.p":    make([]byte, 200),
		"noname.p81": {0x26, 0x27},
	} {
		if _, err := Open(name, bytes.NewReader(data), int64(len(data))); err == nil {
			t.Errorf("Open(%s) succeeded, want error", name)
		}
	}
}
