package apple2_test

import (
	"os"
	"strings"
	"testing"

	"github.com/fredsa/retro-disk-tool/cmd/rdt/internal/testimages"
	"github.com/fredsa/retro-disk-tool/cmd/rdt/pkg/apple2"
)

// openImage opens images/apple/<name>, downloading it first if needed.
func openImage(t *testing.T, name string) (apple2.Image, error) {
	t.Helper()
	path := testimages.Path(t, "apple", name)
	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("failed to open %s: %v", path, err)
	}
	t.Cleanup(func() { f.Close() })
	fi, err := f.Stat()
	if err != nil {
		t.Fatalf("failed to stat %s: %v", path, err)
	}
	return apple2.Open(path, f, fi.Size())
}

func TestProDOS(t *testing.T) {
	// ProDOS on a 140K disk stored in DOS 3.3 sector order.
	img, err := openImage(t, "ProDOS 2.4.2.dsk")
	if err != nil {
		t.Fatalf("apple2.Open failed: %v", err)
	}
	if f := img.Format(); f.Filesystem != "ProDOS" || !strings.HasPrefix(f.Image, "DSK (raw sector image, DOS 3.3 sector order)") {
		t.Errorf("Format() = %+v", f)
	}

	hdr := img.Header()
	if hdr == nil || hdr.Name != "/PRODOS.2.4.2" {
		t.Errorf("Header() = %+v, want Name=/PRODOS.2.4.2", hdr)
	}

	entries, err := img.List()
	if err != nil {
		t.Fatalf("List() failed: %v", err)
	}
	if len(entries) != 19 {
		t.Fatalf("len(entries) = %d, want 19", len(entries))
	}
	if entries[0].Name != "VIEW.README" || entries[0].Size != 512 {
		t.Errorf("entries[0] = %+v, want VIEW.README (512 bytes)", entries[0])
	}
	if e := entries[18]; e.Name != "PRODOS" || e.Size != 17128 || e.ModTime == nil || e.ModTime.Format("2006-01-02 15:04") != "2018-01-18 07:00" {
		t.Errorf("entries[18] = %+v, want PRODOS (17128 bytes, 2018-01-18 07:00)", e)
	}

	usage, err := img.Usage()
	if err != nil {
		t.Fatalf("Usage() failed: %v", err)
	}
	if usage.TotalSectors != 560 || usage.InUseSectors != 560 || usage.FreeSectors != 0 {
		t.Errorf("Usage() = %+v, want Total=560, InUse=560, Free=0", usage)
	}

	// README is Apple II text (high bit set).
	readme, err := img.ReadFile("README")
	if err != nil {
		t.Fatalf("ReadFile(README) failed: %v", err)
	}
	text := make([]byte, len(readme))
	for i, b := range readme {
		text[i] = b & 0x7F
	}
	if len(readme) != 497 || !strings.Contains(string(text), "ProDOS 2.4.2 is a minor update.") {
		t.Errorf("README = %d bytes %q, want 497 bytes", len(readme), text)
	}
}

func TestDOS31MasterD13(t *testing.T) {
	img, err := openImage(t, "Apple DOS 3.1 Master.d13")
	if err != nil {
		t.Fatalf("apple2.Open failed: %v", err)
	}

	hdr := img.Header()
	if hdr == nil || hdr.Name != "DISK VOLUME 254" {
		t.Errorf("Header() = %+v, want Name=DISK VOLUME 254", hdr)
	}

	entries, err := img.List()
	if err != nil {
		t.Fatalf("List() failed: %v", err)
	}
	if len(entries) != 8 {
		t.Fatalf("len(entries) = %d, want 8", len(entries))
	}
	// First entry is inverse "HELLO" (\x08\x05\x0C\x0C\x0F)
	if entries[0].Name != `\x08\x05\x0C\x0C\x0F` || entries[0].Size != 161 {
		t.Errorf("entries[0] = %+v, want \\x08\\x05\\x0C\\x0C\\x0F (161 bytes)", entries[0])
	}
	if entries[1].Name != "APPLESOFT" || entries[1].Size != 10704 {
		t.Errorf("entries[1] = %+v, want APPLESOFT (10704 bytes)", entries[1])
	}

	usage, err := img.Usage()
	if err != nil {
		t.Fatalf("Usage() failed: %v", err)
	}
	if usage.TotalSectors != 455 || usage.InUseSectors != 179 || usage.FreeSectors != 276 {
		t.Errorf("Usage() = %+v, want Total=455, InUse=179, Free=276", usage)
	}

	content, err := img.ReadFile("COPY")
	if err != nil {
		t.Fatalf("ReadFile(COPY) failed: %v", err)
	}
	if len(content) != 1407 {
		t.Errorf("len(COPY) = %d, want 1407", len(content))
	}
}

func TestDOS31MasterNIB(t *testing.T) {
	img, err := openImage(t, "Apple DOS 3.1 Master.nib")
	if err != nil {
		t.Fatalf("apple2.Open failed: %v", err)
	}

	hdr := img.Header()
	if hdr == nil || hdr.Name != "DISK VOLUME 254" {
		t.Errorf("Header() = %+v, want Name=DISK VOLUME 254", hdr)
	}

	entries, err := img.List()
	if err != nil {
		t.Fatalf("List() failed: %v", err)
	}
	if len(entries) != 8 {
		t.Fatalf("len(entries) = %d, want 8", len(entries))
	}
	if entries[1].Name != "APPLESOFT" || entries[1].Size != 10704 {
		t.Errorf("entries[1] = %+v, want APPLESOFT (10704 bytes)", entries[1])
	}

	usage, err := img.Usage()
	if err != nil {
		t.Fatalf("Usage() failed: %v", err)
	}
	if usage.TotalSectors != 455 || usage.InUseSectors != 179 || usage.FreeSectors != 276 {
		t.Errorf("Usage() = %+v, want Total=455, InUse=179, Free=276", usage)
	}

	content, err := img.ReadFile("COPY")
	if err != nil {
		t.Fatalf("ReadFile(COPY) failed: %v", err)
	}
	if len(content) != 1407 {
		t.Errorf("len(COPY) = %d, want 1407", len(content))
	}
}

func TestWOZ(t *testing.T) {
	// A WOZ2 bitstream image of a DOS 3.3 disk (Triple-Dump).
	img, err := openImage(t, "Triple-Dump.woz")
	if err != nil {
		t.Fatalf("apple2.Open failed: %v", err)
	}
	if f := img.Format(); f.Image != `WOZ2 (bitstream image, created by "Applesauce v1.38")` || f.Filesystem != "Apple DOS 3.3" || f.Sectors != "16" {
		t.Errorf("Format() = %+v", f)
	}
	entries, err := img.List()
	if err != nil {
		t.Fatalf("List() failed: %v", err)
	}
	if len(entries) != 86 {
		t.Fatalf("len(entries) = %d, want 86", len(entries))
	}
	if entries[0].Name != "STARTUP" || entries[0].Size != 2013 {
		t.Errorf("entries[0] = %+v, want STARTUP (2013 bytes)", entries[0])
	}
	for _, e := range entries {
		if e.Corrupt {
			t.Errorf("%s is corrupt: %s", e.Name, e.Error)
		}
	}
	usage, err := img.Usage()
	if err != nil {
		t.Fatalf("Usage() failed: %v", err)
	}
	if usage.TotalSectors != 560 || usage.InUseSectors != 529 || usage.FreeSectors != 31 {
		t.Errorf("Usage() = %+v, want Total=560, InUse=529, Free=31", usage)
	}

	// STARTUP is an Applesoft program: a 2 byte length, then lines linked
	// by their memory addresses (from $0801) up to a zero link. Decoding
	// errors would break the chain.
	prog, err := img.ReadFile("STARTUP")
	if err != nil {
		t.Fatalf("ReadFile(STARTUP) failed: %v", err)
	}
	n := int(prog[0]) | int(prog[1])<<8
	p, lines, last := 2, 0, -1
	for p+4 <= len(prog) {
		link := int(prog[p]) | int(prog[p+1])<<8
		if link == 0 {
			break
		}
		num := int(prog[p+2]) | int(prog[p+3])<<8
		if num <= last {
			t.Fatalf("line %d follows line %d", num, last)
		}
		last, lines = num, lines+1
		p = link - 0x801 + 2
	}
	if lines != 53 || last != 530 || p != n {
		t.Errorf("STARTUP: %d lines, last %d, program ends at %d; want 53 lines, last 530, ending at its length %d", lines, last, p, n)
	}
}

func TestDOS33August1980(t *testing.T) {
	img, err := openImage(t, "Apple DOS 3.3 August 1980.dsk")
	if err != nil {
		t.Fatalf("apple2.Open failed: %v", err)
	}
	if f := img.Format(); f.Filesystem != "Apple DOS 3.3" || !strings.HasPrefix(f.Image, "DSK (raw sector image, DOS 3.3 sector order)") || f.Sectors != "16" {
		t.Errorf("Format() = %+v", f)
	}
	if hdr := img.Header(); hdr == nil || hdr.Name != "DISK VOLUME 254" {
		t.Errorf("Header() = %+v, want Name=DISK VOLUME 254", hdr)
	}
	entries, err := img.List()
	if err != nil {
		t.Fatalf("List() failed: %v", err)
	}
	if len(entries) != 27 {
		t.Fatalf("len(entries) = %d, want 27", len(entries))
	}
	for i, w := range []struct {
		name string
		size uint32
	}{{"HELLO", 1139}, {"APPLE PROMS", 382}, {"INTBASIC", 12292}, {"RETRIEVE TEXT", 333}} {
		found := false
		for _, e := range entries {
			if e.Name == w.name {
				found = true
				if e.Size != w.size || e.Corrupt {
					t.Errorf("%d: %s = %+v, want %d bytes", i, w.name, e, w.size)
				}
			}
		}
		if !found {
			t.Errorf("%s not listed", w.name)
		}
	}
	usage, err := img.Usage()
	if err != nil {
		t.Fatalf("Usage() failed: %v", err)
	}
	if usage.TotalSectors != 560 || usage.InUseSectors != 511 || usage.FreeSectors != 49 {
		t.Errorf("Usage() = %+v, want Total=560, InUse=511, Free=49", usage)
	}
	if content, err := img.ReadFile("HELLO"); err != nil || len(content) != 1139 {
		t.Errorf("ReadFile(HELLO) = %d bytes, %v; want 1139", len(content), err)
	}
}

// Apple Pascal and CP/M (CP/AM) disks are not supported yet: the Pascal
// filesystem is not recognized and 800 KiB images are rejected by size.
func TestUnsupportedFilesystems(t *testing.T) {
	for _, tc := range []struct{ name, want string }{
		{"Apple Pascal - Disk 0.po", "unrecognized Apple II 140K disk filesystem"},
		{"CPAM 3.5.po", "invalid Apple II disk size: 819200 bytes"},
	} {
		if _, err := openImage(t, tc.name); err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("open %s: error %v, want %q", tc.name, err, tc.want)
		}
	}
}
