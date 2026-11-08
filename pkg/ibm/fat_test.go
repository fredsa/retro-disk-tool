package ibm_test

import (
	"bytes"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/fredsa/retro-disk-tool/cmd/rdt/pkg/ibm"
)

func readImage(t *testing.T, name string) []byte {
	t.Helper()
	raw, err := os.ReadFile("../../images/ibm/" + name)
	if err != nil {
		t.Fatalf("failed to read %s: %v", name, err)
	}
	return raw
}

func openImage(t *testing.T, raw []byte) *ibm.Disk {
	t.Helper()
	d, err := ibm.Open(bytes.NewReader(raw), int64(len(raw)))
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	return d
}

func names(entries []ibm.Entry) string {
	var n []string
	for _, e := range entries {
		if e.IsDir {
			n = append(n, e.Name+"/")
		} else {
			n = append(n, fmt.Sprintf("%s(%d)", e.Name, e.Size))
		}
	}
	return strings.Join(n, " ")
}

// Both images were formatted by macOS newfs_msdos, whose BPB has a hard disk
// style geometry (16 heads, 32 sectors/track); the floppy geometry comes
// from the image size.
func TestFloppyImages(t *testing.T) {
	for _, tc := range []struct {
		name, label, root string
		spt, free, inUse  int
		unformatted       string
	}{
		{"144mb.img", "FLOPPY", "README.TXT(6) HELLO.TXT(12) DIR1/ DIR2/", 18, 2824, 56, "2 MB"},
		{"720kb.img", "FLOPPY720", ".fseventsd/ HELLO.TXT(12) DIR1/ DIR2/", 9, 1391, 49, "1 MB"},
	} {
		raw := readImage(t, tc.name)
		d := openImage(t, raw)
		if l := d.Label(); l != tc.label {
			t.Errorf("%s: Label = %q, want %q", tc.name, l, tc.label)
		}
		fm := d.Format()
		if fm.Platform != "IBM PC" || fm.Filesystem != "FAT12" || fm.Tracks != 80 || fm.Heads != 2 || fm.Sectors != fmt.Sprint(tc.spt) || fm.SectorSize != 512 {
			t.Errorf("%s: Format = %+v", tc.name, fm)
		}
		entries, err := d.List()
		if err != nil || names(entries) != tc.root {
			t.Errorf("%s: List = %q, %v; want %q", tc.name, names(entries), err, tc.root)
		}
		if got := entries[len(entries)-3].ModTime; got == nil || got.Year() != 2026 {
			t.Errorf("%s: HELLO.TXT ModTime = %v", tc.name, got)
		}
		for path, want := range map[string]string{"hello.txt": "hello world\n", "DIR1/FOO.TXT": "foo was here\n", `dir1\foo.txt`: "foo was here\n"} {
			if c, err := d.ReadFile(path); err != nil || string(c) != want {
				t.Errorf("%s: ReadFile(%s) = %q, %v; want %q", tc.name, path, c, err, want)
			}
		}
		if sub, err := d.ListDir("/dir1/"); err != nil || names(sub) != "FOO.TXT(13)" {
			t.Errorf("%s: ListDir(/dir1/) = %q, %v", tc.name, names(sub), err)
		}
		if sub, err := d.ListDir("DIR2"); err != nil || len(sub) != 0 {
			t.Errorf("%s: ListDir(DIR2) = %q, %v; want empty", tc.name, names(sub), err)
		}
		for _, bad := range []string{"DIR1/FOO.TXT", "NOPE"} {
			if _, err := d.ListDir(bad); err == nil {
				t.Errorf("%s: ListDir(%s) succeeded, want error", tc.name, bad)
			}
		}
		if _, err := d.ReadFile("DIR1"); err == nil {
			t.Errorf("%s: ReadFile(DIR1) succeeded, want directory error", tc.name)
		}
		u, err := d.Usage()
		if err != nil {
			t.Fatal(err)
		}
		if u.TotalSectors != len(raw)/512 || u.FreeSectors != tc.free || u.InUseSectors != tc.inUse || u.UnformattedDesc != tc.unformatted || u.TotalBytes != int64(len(raw)) {
			t.Errorf("%s: Usage = %+v", tc.name, u)
		}
		if w := d.Warnings(); len(w) != 0 {
			t.Errorf("%s: Warnings = %q, want none", tc.name, w)
		}
		var buf bytes.Buffer
		if err := d.Dump(&buf); err != nil {
			t.Fatal(err)
		}
		for _, w := range []string{`OEM name: "BSD  4.4"`, "BPB geometry: 16 heads, 32 sectors/track",
			fmt.Sprintf("Image geometry: 80 tracks, 2 heads, %d sectors/track (%d sectors, from the image size)", tc.spt, len(raw)/512), "DIR1/FOO.TXT"} {
			if !strings.Contains(buf.String(), w) {
				t.Errorf("%s: Dump missing %q:\n%s", tc.name, w, buf.String())
			}
		}
	}

	// Long file names: .fseventsd is stored as FSEVEN~1 (hidden).
	d := openImage(t, readImage(t, "720kb.img"))
	entries, _ := d.List()
	if e := entries[0]; e.Name != ".fseventsd" || e.ShortName != "FSEVEN~1" || !e.IsDir {
		t.Errorf("entries[0] = %+v, want .fseventsd (FSEVEN~1)", e)
	}
	if c, err := d.ReadFile(".FSEVENTSD/fseventsd-uuid"); err != nil || len(c) != 36 {
		t.Errorf("ReadFile(.fseventsd/fseventsd-uuid) = %d bytes, %v; want 36", len(c), err)
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

// badSector is the fill pattern imaging tools write for unreadable sectors.
var badSector = bytes.Repeat([]byte("-=[BAD SECTOR]=-"), 32)

func TestBadSectors(t *testing.T) {
	raw := readImage(t, "720kb.img")

	// A file with an unreadable sector is corrupt. In 720kb.img data starts
	// at sector 43 (1 reserved + 2 FATs of 5 + 32 root directory sectors),
	// so HELLO.TXT (cluster 4) is sector 45.
	img := append([]byte{}, raw...)
	copy(img[45*512:], badSector)
	d := openImage(t, img)
	entries, _ := d.List()
	if e := entries[1]; e.Name != "HELLO.TXT" || !e.Corrupt || e.Error != "1 unreadable sectors" {
		t.Errorf("HELLO.TXT = %+v, want corrupt with 1 unreadable sector", e)
	}
	if w := d.Warnings(); len(w) != 1 || !strings.HasPrefix(w[0], "1 unreadable sectors in the filesystem") || strings.Contains(w[0], "listing may be incomplete") {
		t.Errorf("Warnings = %q", w)
	}

	// An unreadable root directory sector makes the listing unreliable.
	img = append([]byte{}, raw...)
	copy(img[20*512:], badSector)
	if w := openImage(t, img).Warnings(); len(w) != 1 || !strings.Contains(w[0], "1 of them in the boot sector, FAT or root directory") {
		t.Errorf("root directory bad sector: Warnings = %q", w)
	}

	// A standard floppy image may hold a smaller filesystem if every sector
	// after it is unreadable: a 720 KiB filesystem in a 1.44 MiB image.
	big := append(append([]byte{}, raw...), bytes.Repeat(badSector, 1440)...)
	d = openImage(t, big)
	if w := d.Warnings(); len(w) != 1 || w[0] != "the FAT12 filesystem covers only the first 1440 of 2880 sectors; the other 1440 sectors are unreadable" {
		t.Errorf("Warnings = %q", w)
	}
	if fm := d.Format(); fm.Image != "IMG (raw sector image; filesystem in the first 1440 of 2880 sectors)" || fm.Sectors != "18" {
		t.Errorf("Format = %+v", fm)
	}
	u, _ := d.Usage()
	if u.TotalSectors != 2880 || u.ExtraSectors != 1440 || u.InUseSectors+u.FreeSectors+u.ExtraSectors != 2880 || u.TotalBytes != int64(len(big)) || u.UnformattedDesc != "2 MB" {
		t.Errorf("Usage = %+v", u)
	}

	// A readable sector after the filesystem means the image does not match.
	copy(big[2000*512:2001*512], make([]byte, 512))
	if _, err := ibm.Open(bytes.NewReader(big), int64(len(big))); err == nil ||
		!strings.Contains(err.Error(), "image size 1474560 does not match the FAT filesystem size 737280") {
		t.Errorf("Open with a readable sector after the filesystem: error = %v, want size mismatch", err)
	}
}

// A file whose cluster chain ends before its directory size cannot be
// read: cat must fail rather than print a truncated file.
func TestShortChain(t *testing.T) {
	raw := append([]byte{}, readImage(t, "720kb.img")...)
	// HELLO.TXT is cluster 4 (12 bytes, one cluster); claim 2000 bytes.
	root := raw[11*512 : 43*512]
	for o := 0; o < len(root); o += 32 {
		if string(root[o:o+11]) == "HELLO   TXT" {
			root[o+28], root[o+29] = 0xD0, 0x07
		}
	}
	d := openImage(t, raw)
	if _, err := d.ReadFile("HELLO.TXT"); err == nil || !strings.Contains(err.Error(), "cluster chain holds 512 bytes, directory says 2000") {
		t.Errorf("ReadFile(HELLO.TXT) error = %v", err)
	}
	entries, _ := d.List()
	if e := entries[1]; !e.Corrupt || e.Error != "cluster chain too short: 1 clusters, need 4" {
		t.Errorf("HELLO.TXT = %+v, want corrupt", e)
	}
}

func TestSizeMismatch(t *testing.T) {
	img := readImage(t, "144mb.img")
	for name, data := range map[string][]byte{
		"extra byte":    append(append([]byte{}, img...), 0),
		"extra sector":  append(append([]byte{}, img...), make([]byte, 512)...),
		"missing bytes": img[:len(img)-512],
	} {
		if _, err := ibm.Open(bytes.NewReader(data), int64(len(data))); err == nil {
			t.Errorf("Open(%s) succeeded, want size mismatch error", name)
		}
	}
}
