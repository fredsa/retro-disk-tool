package imd

import (
	"bytes"
	"os"
	"strings"
	"testing"

	"github.com/fredsa/retro-disk-tool/cmd/rdt/pkg/util"
)

func parseFile(t *testing.T, path string) (*Image, []byte) {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("failed to read %s: %v", path, err)
	}
	img, err := Parse(bytes.NewReader(raw), int64(len(raw)))
	if err != nil {
		t.Fatalf("Parse(%s) failed: %v", path, err)
	}
	return img, raw
}

func TestModel4Images(t *testing.T) {
	for _, tc := range []struct {
		file, comment  string
		deleted, compr int
	}{
		{"T80M4UPG.IMD", "700-2298 MODEL 4/4P\nTRSDOS UPGRADE\nVER. 06.02.00 to 06.02.01", 18, 12},
		{"T80M4DOS.IMD", "Model 4 System Diskette\nMODEL 4 TRSDOS and BASIC interpreter\nCAT NO. 26-0316\nVersion 6", 18, 1},
	} {
		img, _ := parseFile(t, "../../test_data/"+tc.file)
		if !strings.HasPrefix(img.Header, "IMD 1.10: 15/01/2006") || img.Comment != tc.comment {
			t.Errorf("%s: header %q, comment %q", tc.file, img.Header, img.Comment)
		}
		data, g, err := img.Linear()
		if err != nil {
			t.Fatalf("%s: Linear failed: %v", tc.file, err)
		}
		want := Geometry{Cylinders: 40, Heads: 1, Sectors: 18, SectorSize: 256, FirstSector: 0, Mode: "250 kbps MFM"}
		if *g != want || len(data) != 40*18*256 {
			t.Errorf("%s: geometry %+v, %d bytes", tc.file, *g, len(data))
		}
		// LDOS boot sector: $00 $FE <directory cylinder 20>.
		if !bytes.HasPrefix(data, []byte{0x00, 0xFE, 0x14}) {
			t.Errorf("%s: boot sector starts % x", tc.file, data[:3])
		}
		st := img.Stats()
		if st.Sectors != 720 || st.Deleted != tc.deleted || st.Compressed != tc.compr || st.DataErrors != 0 || st.Unavailable != 0 ||
			len(st.DeletedTracks) != 1 || st.DeletedTracks[0] != 20 {
			t.Errorf("%s: stats %+v", tc.file, st)
		}
		if w := img.Warnings(); len(w) != 0 {
			t.Errorf("%s: warnings %q", tc.file, w)
		}
	}
}

func TestParseRejects(t *testing.T) {
	_, raw := parseFile(t, "../../test_data/T80M4UPG.IMD")
	for name, data := range map[string][]byte{
		"trailing byte":  append(append([]byte{}, raw...), 0),
		"truncated":      raw[:len(raw)-1],
		"no signature":   append([]byte("XMD"), raw[3:]...),
		"no comment end": []byte("IMD 1.10: no terminator"),
		"no tracks":      []byte("IMD 1.10: x\x1a"),
	} {
		if _, err := Parse(bytes.NewReader(data), int64(len(data))); err == nil {
			t.Errorf("Parse(%s) succeeded, want error", name)
		}
	}
}

// track builds an IMD track record with 1-based sector IDs and 128 byte
// sectors, each sector given as a record type and payload.
func track(cyl, head byte, ids []byte, recs ...[]byte) []byte {
	b := []byte{2, cyl, head, byte(len(ids)), 0}
	b = append(b, ids...)
	for _, r := range recs {
		b = append(b, r...)
	}
	return b
}

func TestSectorRecords(t *testing.T) {
	full := append([]byte{1}, bytes.Repeat([]byte{0xAA}, 128)...)
	file := []byte("IMD 1.18: test\r\ncomment\x1a")
	file = append(file, track(0, 0, []byte{2, 1}, []byte{2, 0x55}, full)...)                     // compressed, normal
	file = append(file, track(1, 0, []byte{1, 2}, []byte{0}, append([]byte{5}, full[1:]...))...) // unavailable, data error
	img, err := Parse(bytes.NewReader(file), int64(len(file)))
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}
	data, g, err := img.Linear()
	if err != nil {
		t.Fatalf("Linear failed: %v", err)
	}
	if g.FirstSector != 1 || g.Cylinders != 2 || g.Sectors != 2 || g.Mode != "250 kbps FM" || len(data) != 4*128 {
		t.Fatalf("geometry %+v, %d bytes", *g, len(data))
	}
	// Sectors are placed by ID: cylinder 0 sector 1 is the normal one.
	if data[0] != 0xAA || data[128] != 0x55 || !util.IsBadSector(data[256:384]) || data[384] != 0xAA {
		t.Errorf("linear data wrong: % x % x % x % x", data[0], data[128], data[256:272], data[384])
	}
	if w := img.Warnings(); len(w) != 2 || !strings.HasPrefix(w[0], "1 IMD sectors are unavailable") || !strings.HasPrefix(w[1], "1 IMD sectors were read with data (CRC) errors") {
		t.Errorf("warnings %q", w)
	}

	bad := []byte("IMD 1.18: test\x1a")
	bad = append(bad, track(0, 0, []byte{1, 1}, full, full)...) // duplicate sector ID
	img, err = Parse(bytes.NewReader(bad), int64(len(bad)))
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := img.Linear(); err == nil || !strings.Contains(err.Error(), "non-contiguous") {
		t.Errorf("Linear(duplicate IDs) error = %v", err)
	}

	mixed := []byte("IMD 1.18: test\x1a")
	mixed = append(mixed, track(0, 0, []byte{1, 2}, full, full)...)
	mixed = append(mixed, track(1, 0, []byte{1}, full)...)
	img, err = Parse(bytes.NewReader(mixed), int64(len(mixed)))
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := img.Linear(); err == nil || !strings.Contains(err.Error(), "non-uniform") {
		t.Errorf("Linear(mixed layout) error = %v", err)
	}
}
