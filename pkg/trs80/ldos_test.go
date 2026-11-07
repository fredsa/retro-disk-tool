package trs80

import (
	"bytes"
	"os"
	"strings"
	"testing"

	"github.com/fredsa/retro-disk-tool/cmd/rdt/pkg/imd"
)

func openIMD(t *testing.T, name string) (*Disk, []byte) {
	t.Helper()
	raw, err := os.ReadFile("../../images/" + name)
	if err != nil {
		t.Fatal(err)
	}
	img, err := imd.Parse(bytes.NewReader(raw), int64(len(raw)))
	if err != nil {
		t.Fatal(err)
	}
	data, g, err := img.Linear()
	if err != nil {
		t.Fatal(err)
	}
	d, err := OpenImage(data, g.Cylinders, g.Heads, g.Sectors, "IMD")
	if err != nil {
		t.Fatalf("OpenImage(%s) failed: %v", name, err)
	}
	return d, data
}

func TestTRSDOS6Upgrade(t *testing.T) {
	d, data := openIMD(t, "T80M4UPG.IMD")
	if d.dos != ldos || d.dirTrack != 20 || d.gransPerCyl != 3 || d.secPerGran != 6 {
		t.Errorf("dos %v, dirTrack %d, granules %d x %d sectors", d.dos, d.dirTrack, d.gransPerCyl, d.secPerGran)
	}
	if d.Label() != "UPGRADE6" || d.Date() != "06/02/85" {
		t.Errorf("Label %q, Date %q", d.Label(), d.Date())
	}
	if f := d.Format(); f.Platform != "TRS-80 Model 4" || f.Filesystem != "TRSDOS 6.2" || f.Tracks != 40 || f.Heads != 1 || f.Sectors != "18" {
		t.Errorf("Format = %+v", f)
	}

	entries, _ := d.List()
	want := []struct {
		name string
		size uint32
	}{{"BOOT/SYS", 1280}, {"DIR/SYS", 4608}, {"MAKE621/JCL", 503}, {"MAKE621A/JCL", 8821}}
	if len(entries) != len(want) {
		t.Fatalf("entries = %+v", entries)
	}
	for i, w := range want {
		if entries[i].Name != w.name || entries[i].Size != w.size || entries[i].Corrupt {
			t.Errorf("entries[%d] = %+v, want %s (%d bytes)", i, entries[i], w.name, w.size)
		}
	}
	e := entries[3]
	if e.ModTime == nil || e.ModTime.Format("2006-01-02") != "1985-06-02" || e.Attr&attrInvisible == 0 ||
		len(e.Extents) != 1 || e.Extents[0] != (Extent{Track: 1, StartGran: 1, Grans: 6}) {
		t.Errorf("MAKE621A/JCL = %+v", e)
	}

	// JCL files are text ending with a carriage return or EOF marker.
	jcl, err := d.ReadFile("make621.jcl")
	if err != nil || len(jcl) != 503 || jcl[len(jcl)-1] != 0x1A {
		t.Errorf("ReadFile(make621.jcl) = %d bytes, err %v", len(jcl), err)
	}
	long, err := d.ReadFile("MAKE621A/JCL")
	if err != nil || !strings.HasSuffix(string(long), "boot\r") {
		t.Fatalf("ReadFile(MAKE621A/JCL) = %d bytes, err %v", len(long), err)
	}

	if u, _ := d.Usage(); u.TotalSectors != 720 || u.InUseSectors != 66 || u.FreeSectors != 654 || u.UnformattedCapacity != 250000 {
		t.Errorf("Usage = %+v", u)
	}

	// Split MAKE621A/JCL's extent across an extended directory entry: the
	// FPDE (sector 11, entry 1) keeps 2 granules and links to an FXDE in
	// sector 12, entry 0, holding the remaining 3 + 1 granules.
	split := append([]byte{}, data...)
	sec := func(s int) []byte { return split[(20*18+s)*256 : (20*18+s+1)*256] }
	fpde := sec(11)[32:64]
	copy(fpde[22:], []byte{0x01, 0x21, 0xFE, 0x0A, 0xFF, 0xFF})
	fxde := sec(12)[0:32]
	fxde[0], fxde[1] = 0x90, 0x29
	copy(fxde[22:], []byte{0x02, 0x02, 0x03, 0x00, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF})
	ds, err := OpenImage(split, 40, 1, 18, "test")
	if err != nil {
		t.Fatal(err)
	}
	got, err := ds.ReadFile("MAKE621A/JCL")
	if err != nil || !bytes.Equal(got, long) {
		t.Errorf("ReadFile via FXDE = %d bytes, err %v; want %d identical bytes", len(got), err, len(long))
	}
	if es, _ := ds.List(); len(es) != 4 || len(es[3].Extents) != 3 || es[3].Corrupt {
		t.Errorf("List with FXDE = %+v", es)
	}

	// A link to an unused entry is reported as corrupt.
	fxde[0] = 0
	ds, _ = OpenImage(split, 40, 1, 18, "test")
	if es, _ := ds.List(); len(es) != 4 || !es[3].Corrupt || !strings.Contains(es[3].Error, "invalid extended directory entry link $0A") {
		t.Errorf("List with broken FXDE link = %+v", es)
	}
}

func TestTRSDOS6System(t *testing.T) {
	d, _ := openIMD(t, "T80M4DOS.IMD")
	if d.Label() != "TRSDOS62" || d.Date() != "03/15/84" {
		t.Errorf("Label %q, Date %q", d.Label(), d.Date())
	}
	entries, _ := d.List()
	if len(entries) != 35 {
		t.Errorf("len(entries) = %d, want 35", len(entries))
	}
	for _, e := range entries {
		if e.Corrupt {
			t.Errorf("%s is corrupt: %s", e.Name, e.Error)
		}
	}
	// Load modules end with a transfer record (type 2) exactly at the end
	// of the file.
	for _, name := range []string{"FORMAT/CMD", "BASIC/CMD", "BACKUP/CMD"} {
		c, err := d.ReadFile(name)
		if err != nil {
			t.Fatal(err)
		}
		p := 0
		for p+2 <= len(c) && c[p] != 2 {
			l := int(c[p+1])
			if c[p] == 1 && l <= 2 {
				l += 256
			}
			p += 2 + l
		}
		if p+2 > len(c) || p+2+int(c[p+1]) != len(c) {
			t.Errorf("%s (%d bytes): transfer record not at end of file", name, len(c))
		}
	}
	if u, _ := d.Usage(); u.InUseSectors != 654 || u.FreeSectors != 66 {
		t.Errorf("Usage = %+v", u)
	}
}
