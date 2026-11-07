package adf_test

import (
	"bytes"
	"encoding/binary"
	"math/rand"
	"os"
	"strings"
	"testing"

	"github.com/fredsa/retro-disk-tool/cmd/rdt/pkg/adf"
)

func TestOpenAndListRoot(t *testing.T) {
	const testFile = "../../images/Workbench v1.0 (1985)(Commodore).adf"
	f, err := os.Open(testFile)
	if err != nil {
		t.Fatalf("failed to open test file: %v", err)
	}
	defer f.Close()

	fi, err := f.Stat()
	if err != nil {
		t.Fatalf("failed to stat test file: %v", err)
	}

	fs, err := adf.Open(f, fi.Size())
	if err != nil {
		t.Fatalf("adf.Open failed: %v", err)
	}

	entries, err := fs.ListRoot()
	if err != nil {
		t.Fatalf("fs.ListRoot failed: %v", err)
	}

	if len(entries) == 0 {
		t.Fatalf("expected non-empty root directory")
	}

	entryMap := make(map[string]adf.Entry)
	for _, e := range entries {
		entryMap[e.Name] = e
	}

	// Verify some known entries on Workbench v1.0 disk
	expected := []struct {
		name  string
		isDir bool
		size  uint32
	}{
		{name: "c", isDir: true},
		{name: "devs", isDir: true},
		{name: "fonts", isDir: true},
		{name: "libs", isDir: true},
		{name: "s", isDir: true},
		{name: "Clock", isDir: false, size: 15800},
		{name: "Disk.info", isDir: false},
	}

	for _, exp := range expected {
		e, ok := entryMap[exp.name]
		if !ok {
			t.Errorf("entry %q not found in root listing", exp.name)
			continue
		}
		if e.IsDir != exp.isDir {
			t.Errorf("entry %q IsDir = %v, want %v", exp.name, e.IsDir, exp.isDir)
		}
		if !exp.isDir && exp.size != 0 && e.Size != exp.size {
			t.Errorf("entry %q Size = %d, want %d", exp.name, e.Size, exp.size)
		}
	}

	usage, err := fs.DiskUsage()
	if err != nil {
		t.Fatalf("fs.DiskUsage failed: %v", err)
	}

	if usage.TotalSectors != 1760 {
		t.Errorf("usage.TotalSectors = %d, want 1760", usage.TotalSectors)
	}
	if usage.FreeSectors != 462 {
		t.Errorf("usage.FreeSectors = %d, want 462", usage.FreeSectors)
	}
	if usage.InUseSectors != 1298 {
		t.Errorf("usage.InUseSectors = %d, want 1298", usage.InUseSectors)
	}
	if usage.FreeBytes != int64(usage.FreeSectors)*512 {
		t.Errorf("usage.FreeBytes = %d, want %d", usage.FreeBytes, usage.FreeSectors*512)
	}
	if usage.FormattedCapacity != 901120 {
		t.Errorf("usage.FormattedCapacity = %d, want 901120", usage.FormattedCapacity)
	}
	if usage.UnformattedCapacity != 1000000 {
		t.Errorf("usage.UnformattedCapacity = %d, want 1000000", usage.UnformattedCapacity)
	}

	clockContent, err := fs.ReadFile("Clock")
	if err != nil {
		t.Fatalf("fs.ReadFile(Clock) failed: %v", err)
	}
	if len(clockContent) != 15800 {
		t.Errorf("len(clockContent) = %d, want 15800", len(clockContent))
	}
}

// TestOFSDisk uses a small OFS disk with files, nested folders and icons.
func TestOFSDisk(t *testing.T) {
	const testFile = "../../images/amiga/ofs.adf"
	raw, err := os.ReadFile(testFile)
	if err != nil {
		t.Fatalf("failed to read test file: %v", err)
	}
	fs, err := adf.Open(bytes.NewReader(raw), int64(len(raw)))
	if err != nil {
		t.Fatalf("adf.Open failed: %v", err)
	}
	if name := fs.DiskName(); name != "Amiga" {
		t.Errorf("DiskName() = %q, want Amiga", name)
	}
	if f := fs.Format(); f.Filesystem != "Amiga OFS" || f.Tracks != 80 || f.Heads != 2 || f.Sectors != "11" || f.SectorSize != 512 {
		t.Errorf("Format() = %+v", f)
	}

	entries, err := fs.ListRoot()
	if err != nil {
		t.Fatalf("ListRoot failed: %v", err)
	}
	want := []struct {
		name     string
		dir      bool
		size     uint32
		modified string
	}{
		{".info", false, 42, "2026-11-07 14:26:58"},
		{"Disk.info", false, 306, "2026-11-07 14:00:43"},
		{"level1", true, 0, "2026-11-07 14:27:59"},
		{"level1.info", false, 346, "2026-11-07 14:22:13"},
		{"MyFolder", true, 0, "2026-11-07 14:02:45"},
		{"note", false, 21, "2026-11-07 14:23:59"},
		{"note.info", false, 376, "2026-11-07 14:23:59"},
		{"Trashcan", true, 0, "2026-11-07 14:27:02"},
		{"Trashcan.info", false, 466, "2026-11-07 14:00:43"},
	}
	if len(entries) != len(want) {
		t.Fatalf("len(entries) = %d, want %d: %+v", len(entries), len(want), entries)
	}
	for i, w := range want {
		e := entries[i]
		if e.Name != w.name || e.IsDir != w.dir || e.Size != w.size || e.ModTime.Format("2006-01-02 15:04:05") != w.modified {
			t.Errorf("entries[%d] = %+v, want %s (dir %v, %d bytes, %s)", i, e, w.name, w.dir, w.size, w.modified)
		}
	}

	// note is a single OFS data block of 21 bytes.
	note, err := fs.ReadFile("NOTE")
	if err != nil || string(note) != "test\x1a\x00d\x00\n\x01,\x00dnote-cat" {
		t.Errorf("ReadFile(NOTE) = %q, %v", note, err)
	}
	if _, err := fs.ReadFile("level1"); err == nil {
		t.Error("ReadFile(level1) succeeded, want directory error")
	}
	if _, err := fs.ReadFile("nope"); err == nil {
		t.Error("ReadFile(nope) succeeded, want not found error")
	}

	// Nested folders.
	for path, want := range map[string]string{
		"":              ".info Disk.info level1/ level1.info MyFolder/ note note.info Trashcan/ Trashcan.info",
		"level1":        ".info level2/ level2.info",
		"LEVEL1/level2": "",
		"MyFolder":      "",
		"Trashcan":      "note2 note2.info",
	} {
		entries, err := fs.ListDir(path)
		if err != nil {
			t.Errorf("ListDir(%q) failed: %v", path, err)
			continue
		}
		var got []string
		for _, e := range entries {
			n := e.Name
			if e.IsDir {
				n += "/"
			}
			got = append(got, n)
		}
		if strings.Join(got, " ") != want {
			t.Errorf("ListDir(%q) = %q, want %q", path, got, want)
		}
	}
	if c, err := fs.ReadFile("Trashcan/note2"); err != nil || len(c) != 21 {
		t.Errorf("ReadFile(Trashcan/note2) = %d bytes, %v", len(c), err)
	}
	for _, path := range []string{"note/x", "nope/x", "level1/nope"} {
		if _, err := fs.ListDir(path); err == nil {
			t.Errorf("ListDir(%q) succeeded, want error", path)
		}
	}

	u, err := fs.DiskUsage()
	if err != nil {
		t.Fatalf("DiskUsage failed: %v", err)
	}
	if u.TotalSectors != 1760 || u.InUseSectors != 28 || u.FreeSectors != 1732 || u.TotalBytes != int64(len(raw)) {
		t.Errorf("DiskUsage() = %+v, want Total=1760 InUse=28 Free=1732", u)
	}
}

// TestBuiltDisks reads OFS and FFS disks built by diskBuilder: nested
// folders, and a file large enough to need an extension block.
func TestBuiltDisks(t *testing.T) {
	big := make([]byte, 100*512+123)
	rand.New(rand.NewSource(1)).Read(big)
	for _, ffs := range []bool{false, true} {
		b := newDisk(ffs, "Built")
		a := b.dir(rootBlock, "a")
		sub := b.dir(a, "b")
		b.dir(sub, "c")
		b.file(rootBlock, "readme", []byte("hello amiga\n"))
		b.file(sub, "big", big)
		b.file(rootBlock, "empty", nil)
		raw := b.bytes()

		fs, err := adf.Open(bytes.NewReader(raw), int64(len(raw)))
		if err != nil {
			t.Fatalf("ffs=%v: Open failed: %v", ffs, err)
		}
		wantFS := map[bool]string{false: "Amiga OFS", true: "Amiga FFS"}[ffs]
		if f := fs.Format(); f.Filesystem != wantFS || fs.DiskName() != "Built" {
			t.Errorf("ffs=%v: Format %+v, name %q", ffs, f, fs.DiskName())
		}
		got, err := fs.ReadFile("A/B/big")
		if err != nil || !bytes.Equal(got, big) {
			t.Errorf("ffs=%v: ReadFile(A/B/big) = %d bytes, %v; want %d identical bytes", ffs, len(got), err, len(big))
		}
		if c, err := fs.ReadFile("readme"); err != nil || string(c) != "hello amiga\n" {
			t.Errorf("ffs=%v: ReadFile(readme) = %q, %v", ffs, c, err)
		}
		if c, err := fs.ReadFile("empty"); err != nil || len(c) != 0 {
			t.Errorf("ffs=%v: ReadFile(empty) = %q, %v", ffs, c, err)
		}
		if entries, err := fs.ListDir("a/b"); err != nil || len(entries) != 2 || entries[0].Name != "big" || entries[0].Size != uint32(len(big)) || !entries[1].IsDir {
			t.Errorf("ffs=%v: ListDir(a/b) = %+v, %v", ffs, entries, err)
		}
		u, err := fs.DiskUsage()
		if err != nil {
			t.Fatal(err)
		}
		// Boot blocks, root, bitmap and every allocated block are in use.
		if want := 2 + len(b.used); u.InUseSectors != want || u.InUseSectors+u.FreeSectors != 1760 {
			t.Errorf("ffs=%v: usage %+v, want %d in use", ffs, u, want)
		}
		var dump bytes.Buffer
		if err := fs.Dump(&dump); err != nil || !strings.Contains(dump.String(), "[FILE] a/b/big") || !strings.Contains(dump.String(), "[DIR ] a/b/c/") {
			t.Errorf("ffs=%v: dump:\n%s", ffs, dump.String())
		}
	}

	// A damaged OFS data block (wrong sequence number) is reported.
	b := newDisk(false, "Bad")
	h := b.file(rootBlock, "f", make([]byte, 1000))
	raw := b.bytes()
	second := binary.BigEndian.Uint32(raw[int(h)*512+24+70*4:])
	binary.BigEndian.PutUint32(raw[int(second)*512+8:], 7)
	fs, _ := adf.Open(bytes.NewReader(raw), int64(len(raw)))
	if _, err := fs.ReadFile("f"); err == nil || !strings.Contains(err.Error(), "invalid OFS data block") {
		t.Errorf("ReadFile with a bad data block: %v", err)
	}
}
