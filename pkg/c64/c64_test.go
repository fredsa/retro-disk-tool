package c64_test

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/fredsa/retro-disk-tool/cmd/rdt/internal/testimages"
	"github.com/fredsa/retro-disk-tool/cmd/rdt/pkg/c64"
)

// TestPRG uses a program extracted from a downloaded disk image.
func TestPRG(t *testing.T) {
	raw, err := os.ReadFile(testimages.Path(t, "c64", "m.u.l.e.d64"))
	if err != nil {
		t.Fatal(err)
	}
	d, err := c64.OpenD64(bytes.NewReader(raw), int64(len(raw)))
	if err != nil {
		t.Fatal(err)
	}
	content, err := d.ReadFile("M.U.L.E.")
	if err != nil {
		t.Fatal(err)
	}
	prgPath := filepath.Join(t.TempDir(), "mule.prg")
	if err := os.WriteFile(prgPath, content, 0o644); err != nil {
		t.Fatal(err)
	}

	prg, err := c64.OpenPRG(prgPath, bytes.NewReader(content), int64(len(content)))
	if err != nil {
		t.Fatalf("OpenPRG failed: %v", err)
	}
	entries, err := prg.List()
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}
	if len(entries) != 1 || entries[0].Name != "mule.prg" || entries[0].Size != 80 {
		t.Errorf("entries = %+v, want mule.prg (80 bytes)", entries)
	}
	if f := prg.Format(); f.Image != "PRG (program file, load address $0801)" {
		t.Errorf("Format() = %+v", f)
	}
	if data, err := prg.ReadFile("mule.prg"); err != nil || len(data) != 80 {
		t.Errorf("ReadFile = %d bytes, %v", len(data), err)
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
