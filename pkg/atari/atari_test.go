package atari_test

import (
	"os"
	"testing"

	"github.com/fredsa/retro-disk-tool/cmd/rdt/pkg/atari"
)

func TestDOSATR(t *testing.T) {
	const path = "../../test_data/dos.atr"
	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("failed to open %s: %v", path, err)
	}
	defer f.Close()
	fi, err := f.Stat()
	if err != nil {
		t.Fatal(err)
	}

	d, err := atari.Open(f, fi.Size())
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}

	entries, err := d.List()
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}
	want := []struct {
		name string
		size uint32
	}{{"DOS.SYS", 4875}, {"DUP.SYS", 5126}, {"AUTORUN.SYS", 88}}
	if len(entries) != len(want) {
		t.Fatalf("len(entries) = %d, want %d: %+v", len(entries), len(want), entries)
	}
	for i, w := range want {
		if entries[i].Name != w.name || entries[i].Size != w.size || entries[i].Corrupt {
			t.Errorf("entries[%d] = %+v, want %s (%d bytes)", i, entries[i], w.name, w.size)
		}
	}

	u, err := d.Usage()
	if err != nil {
		t.Fatalf("Usage failed: %v", err)
	}
	if u.TotalSectors != 720 || u.FreeSectors != 625 || u.InUseSectors != 95 || u.SectorSize != 128 {
		t.Errorf("Usage = %+v, want Total=720 Free=625 InUse=95 SectorSize=128", u)
	}

	content, err := d.ReadFile("autorun.sys")
	if err != nil || len(content) != 88 {
		t.Errorf("ReadFile(autorun.sys) = %d bytes, err %v; want 88 bytes", len(content), err)
	}
	if _, err := d.ReadFile("NOPE.SYS"); err == nil {
		t.Error("ReadFile(NOPE.SYS) succeeded, want error")
	}

	f2 := d.Format()
	if f2.Platform != "Atari 8-bit" || f2.Filesystem != "Atari DOS 2.0S" || f2.Tracks != 40 || f2.Sectors != "18" || f2.SectorSize != 128 {
		t.Errorf("Format = %+v", f2)
	}
}

