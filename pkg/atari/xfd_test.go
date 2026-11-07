package atari_test

import (
	"bytes"
	"os"
	"strings"
	"testing"

	"github.com/fredsa/retro-disk-tool/cmd/rdt/pkg/atari"
)

func TestInvertedXFD(t *testing.T) {
	raw, err := os.ReadFile("../../test_data/mfgamania.img")
	if err != nil {
		t.Fatal(err)
	}
	normal := make([]byte, len(raw))
	for i, b := range raw {
		normal[i] = ^b
	}

	for _, tc := range []struct {
		name     string
		data     []byte
		inverted bool
	}{{"inverted", raw, true}, {"normal", normal, false}} {
		d, err := atari.OpenXFD(bytes.NewReader(tc.data), int64(len(tc.data)))
		if err != nil {
			t.Fatalf("%s: OpenXFD failed: %v", tc.name, err)
		}
		f := d.Format()
		if f.Filesystem != "Atari DOS 2.0S" || strings.Contains(f.Image, "inverted") != tc.inverted || !strings.HasPrefix(f.Image, "XFD (raw sector image") {
			t.Errorf("%s: Format = %+v", tc.name, f)
		}
		entries, err := d.List()
		if err != nil {
			t.Fatal(err)
		}
		var names []string
		for _, e := range entries {
			if e.Corrupt {
				t.Errorf("%s: %s is corrupt: %s", tc.name, e.Name, e.Error)
			}
			names = append(names, e.Name)
		}
		if got, want := strings.Join(names, " "), "TAPPER.EXE MEGAMANI.EXE DOS.SYS DUP.SYS STARCAST.EXE BOULDER2.EXE AUTORUN.SYS"; got != want {
			t.Errorf("%s: names = %s, want %s", tc.name, got, want)
		}
		exe, err := d.ReadFile("TAPPER.EXE")
		if err != nil || len(exe) != 26916 || !bytes.HasPrefix(exe, []byte{0xFF, 0xFF}) {
			t.Errorf("%s: ReadFile(TAPPER.EXE) = %d bytes, err %v; want 26916 byte binary load file", tc.name, len(exe), err)
		}
		if u, _ := d.Usage(); u.FreeSectors != 65 || u.TotalSectors != 720 {
			t.Errorf("%s: Usage = %+v", tc.name, u)
		}
	}
}

func TestOpenXFDRejects(t *testing.T) {
	for name, data := range map[string][]byte{
		"odd size": make([]byte, 1000),
		"blank":    make([]byte, 720*128),
	} {
		if _, err := atari.OpenXFD(bytes.NewReader(data), int64(len(data))); err == nil {
			t.Errorf("OpenXFD(%s) succeeded, want error", name)
		}
	}
}
