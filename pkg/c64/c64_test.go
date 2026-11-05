package c64_test

import (
	"os"
	"testing"

	"github.com/fredsa/retro-disk-tool/cmd/rdt/pkg/c64"
)

func TestD64(t *testing.T) {
	const d64Path = "../../test_data/JeepersCreepersPlus1-UNK.d64"
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

	usage, err := disk.Usage()
	if err != nil {
		t.Fatalf("Usage failed: %v", err)
	}
	if usage.TotalSectors != 683 {
		t.Errorf("TotalSectors = %d, want 683", usage.TotalSectors)
	}
	if usage.FreeSectors != 580 {
		t.Errorf("FreeSectors = %d, want 580", usage.FreeSectors)
	}
	if usage.InUseSectors != 103 {
		t.Errorf("InUseSectors = %d, want 103", usage.InUseSectors)
	}
}

func TestPRG(t *testing.T) {
	const prgPath = "../../test_data/flipandflop.prg"
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
	const crtPath = "../../test_data/EF3 Adrian Toolset.crt"
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
