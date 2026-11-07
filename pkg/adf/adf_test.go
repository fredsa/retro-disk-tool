package adf_test

import (
	"bytes"
	"os"
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

// TestSimpleADF uses a small OFS disk with files, folders and icons.
func TestSimpleADF(t *testing.T) {
	const testFile = "../../images/amiga/simple.adf"
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

	u, err := fs.DiskUsage()
	if err != nil {
		t.Fatalf("DiskUsage failed: %v", err)
	}
	if u.TotalSectors != 1760 || u.InUseSectors != 28 || u.FreeSectors != 1732 || u.TotalBytes != int64(len(raw)) {
		t.Errorf("DiskUsage() = %+v, want Total=1760 InUse=28 Free=1732", u)
	}
}
