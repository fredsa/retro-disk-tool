package trs80

import (
	"bytes"
	"os"
	"strings"
	"testing"
)

func TestProfileIII(t *testing.T) {
	const path = "../../images/trs80profileiii.img"
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("failed to read %s: %v", path, err)
	}
	d, err := Open(bytes.NewReader(raw), int64(len(raw)))
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	if d.Label() != "TRSDOS" || d.Date() != "07/16/82" || d.dirTrack != 17 {
		t.Errorf("Label %q, Date %q, dirTrack %d", d.Label(), d.Date(), d.dirTrack)
	}

	entries, err := d.List()
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}
	want := []struct {
		name string
		size uint32
	}{
		{"BASIC/CMD", 5120}, {"RM/CMD", 3384}, {"EFC7/CMD", 3683}, {"EFC8/CMD", 6651}, {"EFC9/CMD", 14978},
		{"EFCA/CMD", 6692}, {"EFCB/CMD", 4381}, {"EFCC/CMD", 9146}, {"EFCD/CMD", 9195},
	}
	if len(entries) != len(want) {
		t.Fatalf("len(entries) = %d, want %d: %+v", len(entries), len(want), entries)
	}
	for i, w := range want {
		if entries[i].Name != w.name || entries[i].Size != w.size || entries[i].Corrupt {
			t.Errorf("entries[%d] = %+v, want %s (%d bytes)", i, entries[i], w.name, w.size)
		}
	}
	if e := entries[0]; e.Attr&attrSystem == 0 || e.Attr&attrInvisible == 0 || e.Protection != 6 || e.Month != 5 || e.Year != 81 {
		t.Errorf("BASIC/CMD attributes = %+v", e)
	}
	if x := entries[8].Extents; len(x) != 2 || x[1] != (Extent{Track: 21, StartGran: 4, Grans: 11}) {
		t.Errorf("EFCD/CMD extents = %+v", x)
	}

	// RM/CMD is a CMD file: a header record, load records, then a transfer
	// record (type 2) to $7000.
	content, err := d.ReadFile("rm.cmd")
	if err != nil || len(content) != 3384 {
		t.Fatalf("ReadFile(rm.cmd) = %d bytes, err %v", len(content), err)
	}
	if content[0] != 0x05 || !bytes.Contains(content, []byte{0x02, 0x02, 0x00, 0x70}) {
		t.Errorf("RM/CMD is not a CMD file: % x", content[:16])
	}
	if _, err := d.ReadFile("NOPE/CMD"); err == nil {
		t.Error("ReadFile(NOPE/CMD) succeeded, want error")
	}

	u, err := d.Usage()
	if err != nil {
		t.Fatal(err)
	}
	if u.TotalSectors != 720 || u.InUseSectors != 423 || u.FreeSectors != 297 || u.FormattedDesc != "180 KiB" || u.UnformattedCapacity != 250000 {
		t.Errorf("Usage = %+v", u)
	}

	var buf bytes.Buffer
	if err := d.Dump(&buf); err != nil {
		t.Fatal(err)
	}
	for _, w := range []string{`Disk name: "TRSDOS", Date: 07/16/82`, "si6 BASIC/CMD", "t14 g5+1, t21 g4+11"} {
		if !strings.Contains(buf.String(), w) {
			t.Errorf("Dump missing %q:\n%s", w, buf.String())
		}
	}
}

func TestOpenRejects(t *testing.T) {
	for name, data := range map[string][]byte{
		"odd size": make([]byte, 1000),
		"blank":    make([]byte, 40*sectorsPerTrack*sectorSize),
	} {
		if _, err := Open(bytes.NewReader(data), int64(len(data))); err == nil {
			t.Errorf("Open(%s) succeeded, want error", name)
		}
	}
}
