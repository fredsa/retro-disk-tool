package imd

import (
	"bytes"
	"strings"
	"testing"

	"github.com/fredsa/retro-disk-tool/cmd/rdt/pkg/util"
)

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
