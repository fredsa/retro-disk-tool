package adf_test

import (
	"os"
	"testing"

	"github.com/fredsa/retro-disk-tool/cmd/rdt/pkg/adf"
)

func TestOpenAndListRoot(t *testing.T) {
	const testFile = "../../test_data/Workbench v1.0 (1985)(Commodore).adf"
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
}
