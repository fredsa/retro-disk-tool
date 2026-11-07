package ibm_test

import (
	"bytes"
	"os"
	"strings"
	"testing"

	"github.com/fredsa/retro-disk-tool/cmd/rdt/pkg/ibm"
)

func TestLPMUDImg(t *testing.T) {
	const path = "../../test_data/lpmud.img"
	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("failed to open %s: %v", path, err)
	}
	defer f.Close()
	fi, err := f.Stat()
	if err != nil {
		t.Fatal(err)
	}

	d, err := ibm.Open(f, fi.Size())
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}

	entries, err := d.List()
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}
	if len(entries) != 2 || entries[0].Name != "DOSP11" || !entries[0].IsDir ||
		entries[1].Name != ".fseventsd" || entries[1].ShortName != "FSEVEN~1" || !entries[1].IsDir {
		t.Fatalf("entries = %+v, want DOSP11/ and .fseventsd/", entries)
	}
	if got := entries[0].ModTime.Format("2006-01-02 15:04:05"); got != "1993-02-21 22:25:26" {
		t.Errorf("DOSP11 ModTime = %s, want 1993-02-21 22:25:26", got)
	}

	sub, err := d.ListDir("/dosp11/")
	if err != nil || len(sub) != 1 || sub[0].Name != "LPMUD.ARJ" || sub[0].Size != 264730 {
		t.Errorf("ListDir(/dosp11/) = %+v, %v; want LPMUD.ARJ", sub, err)
	}
	if _, err := d.ListDir("DOSP11/LPMUD.ARJ"); err == nil {
		t.Error("ListDir(DOSP11/LPMUD.ARJ) succeeded, want not a directory error")
	}

	content, err := d.ReadFile("dosp11/lpmud.arj")
	if err != nil || len(content) != 264730 || !bytes.HasPrefix(content, []byte{0x60, 0xEA}) {
		t.Errorf("ReadFile(dosp11/lpmud.arj) = %d bytes, err %v; want 264730 byte ARJ archive", len(content), err)
	}
	if content, err := d.ReadFile(`.fseventsd\fseventsd-uuid`); err != nil || len(content) != 36 {
		t.Errorf("ReadFile(.fseventsd/fseventsd-uuid) = %d bytes, err %v; want 36 bytes", len(content), err)
	}
	if _, err := d.ReadFile("DOSP11"); err == nil {
		t.Error("ReadFile(DOSP11) succeeded, want directory error")
	}
	if _, err := d.ReadFile("DOSP11/NOPE.TXT"); err == nil {
		t.Error("ReadFile(DOSP11/NOPE.TXT) succeeded, want error")
	}

	u, err := d.Usage()
	if err != nil {
		t.Fatalf("Usage failed: %v", err)
	}
	if u.TotalSectors != 1440 || u.FreeSectors != 898 || u.InUseSectors != 542 || u.SectorSize != 512 ||
		u.FormattedDesc != "720 KiB" || u.UnformattedDesc != "1 MB" {
		t.Errorf("Usage = %+v", u)
	}

	fm := d.Format()
	if fm.Platform != "IBM PC" || fm.Filesystem != "FAT12" || fm.Tracks != 80 || fm.Heads != 2 || fm.Sectors != "9" || fm.SectorSize != 512 {
		t.Errorf("Format = %+v", fm)
	}
	if l := d.Label(); l != "" {
		t.Errorf("Label = %q, want empty", l)
	}

	var buf bytes.Buffer
	if err := d.Dump(&buf); err != nil {
		t.Fatalf("Dump failed: %v", err)
	}
	if !strings.Contains(buf.String(), "DOSP11/LPMUD.ARJ") || !strings.Contains(buf.String(), `OEM name: "MSDOS5.0"`) {
		t.Errorf("Dump output missing expected content:\n%s", buf.String())
	}
}

// TestDOS1x builds a 160 KiB PC DOS 1.0 style floppy, which has no BPB.
func TestDOS1x(t *testing.T) {
	img := make([]byte, 160*1024)
	// FAT (sector 1): media byte 0xFE; cluster 2 is EOC.
	copy(img[512:], []byte{0xFE, 0xFF, 0xFF, 0xFF, 0x0F})
	// Root directory starts at sector 3 (1 reserved + 2 FATs of 1 sector).
	root := img[3*512:]
	copy(root[0:], "MYDISK     ")
	root[11] = 0x08
	copy(root[32:], "HELLO   TXT")
	root[32+11] = 0x20
	root[32+26] = 2 // first cluster
	root[32+28] = 5 // size
	// Data starts after 64 root entries (4 sectors) at sector 7.
	copy(img[7*512:], "hello")

	d, err := ibm.Open(bytes.NewReader(img), int64(len(img)))
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	if l := d.Label(); l != "MYDISK" {
		t.Errorf("Label = %q, want MYDISK", l)
	}
	entries, err := d.List()
	if err != nil || len(entries) != 1 || entries[0].Name != "HELLO.TXT" || entries[0].ModTime != nil {
		t.Fatalf("List = %+v, %v", entries, err)
	}
	if c, err := d.ReadFile("hello.txt"); err != nil || string(c) != "hello" {
		t.Errorf("ReadFile = %q, %v", c, err)
	}
	fm := d.Format()
	if fm.Tracks != 40 || fm.Heads != 1 || fm.Sectors != "8" || !strings.Contains(fm.Filesystem, "no BPB") {
		t.Errorf("Format = %+v", fm)
	}
	if u, _ := d.Usage(); u.UnformattedDesc != "250 KB" || u.FreeSectors != 312 {
		t.Errorf("Usage = %+v", u)
	}
}

func TestBadSectors(t *testing.T) {
	raw, err := os.ReadFile("../../test_data/maxell.img")
	if err != nil {
		t.Fatal(err)
	}
	d, err := ibm.Open(bytes.NewReader(raw), int64(len(raw)))
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	w := d.Warnings()
	if len(w) != 2 ||
		!strings.Contains(w[0], "720 extra sectors after the first 720 (720 of them unreadable)") ||
		!strings.Contains(w[1], "274 unreadable sectors") || !strings.Contains(w[1], "1 of them in the boot sector, FAT or root directory") {
		t.Errorf("Warnings = %q", w)
	}
	if fm := d.Format(); fm.Tracks != 40 || fm.Heads != 2 {
		t.Errorf("Format = %+v, want 40 tracks, 2 heads", fm)
	}

	// A file with an unreadable sector is reported as corrupt.
	img, err := os.ReadFile("../../test_data/lpmud.img")
	if err != nil {
		t.Fatal(err)
	}
	if d, _ := ibm.Open(bytes.NewReader(img), int64(len(img))); len(d.Warnings()) != 0 {
		t.Errorf("lpmud.img Warnings = %q, want none", d.Warnings())
	}
	// LPMUD.ARJ starts at cluster 3: data starts at sector 14, 2 sectors per cluster.
	copy(img[16*512:17*512], bytes.Repeat([]byte("-=[BAD SECTOR]=-"), 32))
	d, err = ibm.Open(bytes.NewReader(img), int64(len(img)))
	if err != nil {
		t.Fatal(err)
	}
	sub, _ := d.ListDir("DOSP11")
	if len(sub) != 1 || !sub[0].Corrupt || sub[0].Error != "1 unreadable sectors" {
		t.Errorf("ListDir(DOSP11) = %+v, want LPMUD.ARJ corrupt with 1 unreadable sector", sub)
	}
	if w := d.Warnings(); len(w) != 1 || !strings.HasPrefix(w[0], "1 unreadable sectors in the filesystem") || strings.Contains(w[0], "listing may be incomplete") {
		t.Errorf("Warnings = %q", w)
	}
}
