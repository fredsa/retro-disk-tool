package c64_test

import (
	"os"
	"strings"
	"testing"

	"github.com/fredsa/retro-disk-tool/cmd/rdt/internal/testimages"
	"github.com/fredsa/retro-disk-tool/cmd/rdt/pkg/c64"
)

func TestD64(t *testing.T) {
	const d64Path = "../../images/JeepersCreepersPlus1-UNK.d64"
	f, err := os.Open(d64Path)
	if err != nil {
		t.Fatalf("failed to open d64: %v", err)
	}
	defer f.Close()

	fi, err := f.Stat()
	if err != nil {
		t.Fatalf("failed to stat d64: %v", err)
	}

	disk, err := c64.OpenD64(f, fi.Size())
	if err != nil {
		t.Fatalf("OpenD64 failed: %v", err)
	}

	entries, err := disk.ReadDirectory()
	if err != nil {
		t.Fatalf("ReadDirectory failed: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}
	if entries[0].Name != "JEEP.CREEPERS+" {
		t.Errorf("entry name = %q, want JEEP.CREEPERS+", entries[0].Name)
	}
	if entries[0].Type != "PRG" {
		t.Errorf("entry type = %q, want PRG", entries[0].Type)
	}
	if entries[0].Sectors != 101 {
		t.Errorf("entry sectors = %d, want 101", entries[0].Sectors)
	}
	if entries[0].Bytes != 25627 {
		t.Errorf("entry bytes = %d, want 25627", entries[0].Bytes)
	}
	if entries[0].Corrupt {
		t.Errorf("entry Corrupt = true, want false")
	}

	usage, err := disk.Usage()
	if err != nil {
		t.Fatalf("Usage failed: %v", err)
	}
	if usage.TotalSectors != 683 {
		t.Errorf("TotalSectors = %d, want 683", usage.TotalSectors)
	}
	if usage.FreeSectors != 563 {
		t.Errorf("FreeSectors = %d, want 563", usage.FreeSectors)
	}
	if usage.BAMSectors != 19 {
		t.Errorf("BAMSectors = %d, want 19", usage.BAMSectors)
	}
	if usage.InUseSectors != 101 {
		t.Errorf("InUseSectors = %d, want 101", usage.InUseSectors)
	}
}

func TestD64CorruptDirectory(t *testing.T) {
	const d64Path = "../../images/64 games 21.D64"
	f, err := os.Open(d64Path)
	if err != nil {
		t.Fatalf("failed to open d64: %v", err)
	}
	defer f.Close()

	fi, err := f.Stat()
	if err != nil {
		t.Fatalf("failed to stat d64: %v", err)
	}

	disk, err := c64.OpenD64(f, fi.Size())
	if err != nil {
		t.Fatalf("OpenD64 failed: %v", err)
	}

	entries, err := disk.ReadDirectory()
	if err != nil {
		t.Fatalf("ReadDirectory failed: %v", err)
	}
	if len(entries) != 22 {
		t.Fatalf("expected 22 entries, got %d", len(entries))
	}

	// First 16 entries (in sectors 18/1 and 18/4) are valid
	for i := 0; i < 16; i++ {
		if entries[i].Corrupt {
			t.Errorf("entry %d (%q) unexpectedly marked corrupt: %s", i, entries[i].Name, entries[i].Error)
		}
	}

	// Remaining 6 entries (in sector 18/7, which has error code 0x05 = 23, READ ERROR) are corrupt
	for i := 16; i < len(entries); i++ {
		if !entries[i].Corrupt {
			t.Errorf("entry %d (%q) expected Corrupt=true, got false", i, entries[i].Name)
		}
		if !strings.Contains(entries[i].Error, "track 18, sector 7: 23, READ ERROR") {
			t.Errorf("entry %d Error = %q, want track 18, sector 7: 23, READ ERROR", i, entries[i].Error)
		}
	}
}

func TestD64Header(t *testing.T) {
	const d64Path = "../../images/och0528v35.d64"
	f, err := os.Open(d64Path)
	if err != nil {
		t.Fatalf("failed to open d64: %v", err)
	}
	defer f.Close()

	fi, err := f.Stat()
	if err != nil {
		t.Fatalf("failed to stat d64: %v", err)
	}

	disk, err := c64.OpenD64(f, fi.Size())
	if err != nil {
		t.Fatalf("OpenD64 failed: %v", err)
	}

	hdr := disk.Header()
	if hdr == nil {
		t.Fatalf("disk.Header() returned nil")
	}
	if hdr.Name != "OCH0528 V3.5 AT" {
		t.Errorf("hdr.Name = %q, want %q", hdr.Name, "OCH0528 V3.5 AT")
	}
	if hdr.ID != "ST" {
		t.Errorf("hdr.ID = %q, want %q", hdr.ID, "ST")
	}
	if hdr.DOSType != "2A" {
		t.Errorf("hdr.DOSType = %q, want %q", hdr.DOSType, "2A")
	}

	entries, err := disk.ReadDirectory()
	if err != nil {
		t.Fatalf("ReadDirectory failed: %v", err)
	}
	if len(entries) != 7 {
		t.Fatalf("expected 7 entries, got %d", len(entries))
	}

	usage, err := disk.Usage()
	if err != nil {
		t.Fatalf("Usage failed: %v", err)
	}
	// On och0528v35.d64: 517 free user blocks (683 total, 166 in-use including directory track 18)
	if usage.FreeSectors != 517 {
		t.Errorf("FreeSectors = %d, want 517", usage.FreeSectors)
	}
	if usage.BAMSectors != 19 {
		t.Errorf("BAMSectors = %d, want 19", usage.BAMSectors)
	}
	if usage.InUseSectors != 147 {
		t.Errorf("InUseSectors = %d, want 147", usage.InUseSectors)
	}

	content, err := disk.ReadFile("MENU")
	if err != nil {
		t.Fatalf("ReadFile(MENU) failed: %v", err)
	}
	if len(content) != 95 {
		t.Errorf("len(MENU content) = %d, want 95", len(content))
	}
}

func TestPRG(t *testing.T) {
	const prgPath = "../../images/flipandflop.prg"
	f, err := os.Open(prgPath)
	if err != nil {
		t.Fatalf("failed to open prg: %v", err)
	}
	defer f.Close()

	fi, err := f.Stat()
	if err != nil {
		t.Fatalf("failed to stat prg: %v", err)
	}

	prg, err := c64.OpenPRG(prgPath, f, fi.Size())
	if err != nil {
		t.Fatalf("OpenPRG failed: %v", err)
	}

	entries, err := prg.List()
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}
	if entries[0].Name != "flipandflop.prg" {
		t.Errorf("entry name = %q, want flipandflop.prg", entries[0].Name)
	}
	if entries[0].Size != 25071 {
		t.Errorf("entry size = %d, want 25071", entries[0].Size)
	}
}

func TestCRT(t *testing.T) {
	crtPath := testimages.Path(t, "c64", "EF3 Adrian Toolset.crt")
	f, err := os.Open(crtPath)
	if err != nil {
		t.Fatalf("failed to open crt: %v", err)
	}
	defer f.Close()

	fi, err := f.Stat()
	if err != nil {
		t.Fatalf("failed to stat crt: %v", err)
	}

	crt, err := c64.OpenCRT(crtPath, f, fi.Size())
	if err != nil {
		t.Fatalf("OpenCRT failed: %v", err)
	}

	entries, err := crt.List()
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}
	if entries[0].Name != "ndefpack 1.0.1" {
		t.Errorf("entry name = %q, want ndefpack 1.0.1", entries[0].Name)
	}
	if entries[0].Size != 622592 {
		t.Errorf("entry size = %d, want 622592", entries[0].Size)
	}
}
