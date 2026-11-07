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
	img, err := openImage(t, "ProDOS_2_4_3.po")
	if err != nil {
		t.Fatalf("apple2.Open failed: %v", err)
	}

	hdr := img.Header()
	if hdr == nil || hdr.Name != "/PRODOS.2.4.3" {
		t.Errorf("Header() = %+v, want Name=/PRODOS.2.4.3", hdr)
	}

	entries, err := img.List()
	if err != nil {
		t.Fatalf("List() failed: %v", err)
	}
	if len(entries) != 17 {
		t.Fatalf("len(entries) = %d, want 17", len(entries))
	}
	if entries[0].Name != "VIEW.README" || entries[0].Size != 512 {
		t.Errorf("entries[0] = %+v, want VIEW.README (512 bytes)", entries[0])
	}
	if entries[16].Name != "README" || entries[16].Size != 999 {
		t.Errorf("entries[16] = %+v, want README (999 bytes)", entries[16])
	}

	usage, err := img.Usage()
	if err != nil {
		t.Fatalf("Usage() failed: %v", err)
	}
	if usage.TotalSectors != 560 || usage.InUseSectors != 508 || usage.FreeSectors != 52 {
		t.Errorf("Usage() = %+v, want Total=560, InUse=508, Free=52", usage)
	}

	readme, err := img.ReadFile("README")
	if err != nil {
		t.Fatalf("ReadFile(README) failed: %v", err)
	}
	if len(readme) != 999 {
		t.Errorf("len(README) = %d, want 999", len(readme))
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

func TestDOS32SystemMasterWOZ(t *testing.T) {
	img, err := openImage(t, "DOS 3.2 System Master.woz")
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
	if len(entries) != 14 {
		t.Fatalf("len(entries) = %d, want 14", len(entries))
	}
	if entries[0].Name != "HELLO" || entries[0].Size != 138 {
		t.Errorf("entries[0] = %+v, want HELLO (138 bytes)", entries[0])
	}
	if entries[13].Name != "PINBALL" || entries[13].Size != 6629 {
		t.Errorf("entries[13] = %+v, want PINBALL (6629 bytes)", entries[13])
	}

	usage, err := img.Usage()
	if err != nil {
		t.Fatalf("Usage() failed: %v", err)
	}
	if usage.TotalSectors != 455 || usage.InUseSectors != 452 || usage.FreeSectors != 3 {
		t.Errorf("Usage() = %+v, want Total=455, InUse=452, Free=3", usage)
	}

	content, err := img.ReadFile("HELLO")
	if err != nil {
		t.Fatalf("ReadFile(HELLO) failed: %v", err)
	}
	if len(content) != 138 {
		t.Errorf("len(HELLO) = %d, want 138", len(content))
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
