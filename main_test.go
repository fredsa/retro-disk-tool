package main

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/fredsa/retro-disk-tool/cmd/rdt/internal/testimages"
	"github.com/fredsa/retro-disk-tool/cmd/rdt/pkg/disk"
)

func TestRunVersion(t *testing.T) {
	for _, arg := range []string{"-v", "--version"} {
		var stdout, stderr bytes.Buffer
		code := run([]string{arg}, &stdout, &stderr)
		if code != 0 {
			t.Errorf("run(%q) exited with %d, want 0", arg, code)
		}
		if !strings.Contains(stdout.String(), version) {
			t.Errorf("run(%q) stdout = %q, want version %q", arg, stdout.String(), version)
		}
	}
}

func TestRunHelp(t *testing.T) {
	for _, arg := range []string{"-h", "--help"} {
		var stdout, stderr bytes.Buffer
		code := run([]string{arg}, &stdout, &stderr)
		if code != 0 {
			t.Errorf("run(%q) exited with %d, want 0", arg, code)
		}
		if !strings.Contains(stdout.String(), "Usage:") {
			t.Errorf("run(%q) stdout = %q, want usage info", arg, stdout.String())
		}
	}
}

func TestRunLs(t *testing.T) {
	testFile := "images/Workbench v1.0 (1985)(Commodore).adf"
	tests := []struct {
		args        []string
		wantContain []string
	}{
		{
			args:        []string{"ls", testFile},
			wantContain: []string{"Label:\n  Workbench\n", "Contents:\n", "  Clock\n", "  c\n", "  devs\n", "  libs\n", "Capacity:", "Formatted:   880 KiB (901,120 bytes)", "Unformatted:    1 MB (1,000,000 bytes)", "Free:    26.2%    462 sectors (236,544 bytes)", "In-use:  73.8%  1,298 sectors (664,576 bytes)", "Total:  100.0%  1,760 sectors (901,120 bytes)"},
		},
		{
			args:        []string{"ls", "-l", testFile},
			wantContain: []string{"Label:\n  Workbench\n", "Contents:\n  Type     Size Modified            Name\n", "  -       15800", "  d           0", "Clock", "devs/", "Capacity:", "Usage:"},
		},
	}
	for _, tt := range tests {
		var stdout, stderr bytes.Buffer
		code := run(tt.args, &stdout, &stderr)
		if code != 0 {
			t.Errorf("run(%v) exited with %d, want 0 (stderr: %s)", tt.args, code, stderr.String())
		}
		for _, w := range tt.wantContain {
			if !strings.Contains(stdout.String(), w) {
				t.Errorf("run(%v) stdout missing %q; got:\n%s", tt.args, w, stdout.String())
			}
		}
	}
}

func TestRunTerminal(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if code := run([]string{"terminal"}, &stdout, &stderr); code != 0 {
		t.Fatalf("run(terminal) = %d, want 0", code)
	}
	for _, w := range []string{"Terminal:", "Platform:", "ANSI supported:   false", "Normal:", "Inverse:", "Flashing:"} {
		if !strings.Contains(stdout.String(), w) {
			t.Errorf("terminal output missing %q; got:\n%s", w, stdout.String())
		}
	}
}

func TestRunLsFormat(t *testing.T) {
	tests := []struct {
		file        string
		wantContain []string
	}{
		{"images/Workbench v1.0 (1985)(Commodore).adf", []string{"Platform:   Amiga", "Amiga OFS", "80 tracks, 2 heads, 11 sectors/track, 512 bytes/sector"}},
		{"images/dos.atr", []string{"Platform:   Atari 8-bit", "ATR (disk image", "Atari DOS 2.0S", "40 tracks, 1 head, 18 sectors/track, 128 bytes/sector"}},
		{"images/lpmud.img", []string{"Platform:   IBM PC", "IMG (raw sector image)", "Filesystem: FAT12", "80 tracks, 2 heads, 9 sectors/track, 512 bytes/sector"}},
		{"images/ZX81_Kong (Paul Farrow 2010) [V1-01].p", []string{"Platform:   Sinclair ZX81", "P (program file, memory image from $4009)"}},
		{"images/ZX81_Kong (Paul Farrow 2010) [V1-01].P81", []string{"Platform:   Sinclair ZX81", "P81 (named program file, 1 program)"}},
		{"images/trs80profileiii.img", []string{"Platform:   TRS-80 Model III", "IMG (raw sector image)", "Filesystem: TRSDOS 1.3", "40 tracks, 1 head, 18 sectors/track, 256 bytes/sector"}},
		{"images/mfgamania.img", []string{"Platform:   Atari 8-bit", "XFD (raw sector image, 128 byte sectors, all bits inverted)", "Atari DOS 2.0S"}},
		{"images/maxell.img", []string{"Platform:   IBM PC", "IMG (raw sector image; filesystem in the first 720 of 1440 sectors)", "80 tracks, 2 heads, 9 sectors/track, 512 bytes/sector"}},
		{"images/T80M4UPG.IMD", []string{"Platform:   TRS-80 Model 4", "IMD (ImageDisk 1.10, 250 kbps MFM)", "Filesystem: TRSDOS 6.2", "40 tracks, 1 head, 18 sectors/track, 256 bytes/sector"}},
	}
	for _, tt := range tests {
		var stdout, stderr bytes.Buffer
		if code := run([]string{"ls", tt.file}, &stdout, &stderr); code != 0 {
			t.Errorf("run(ls %s) = %d (stderr: %s)", tt.file, code, stderr.String())
			continue
		}
		if !strings.HasPrefix(stdout.String(), "Format:\n") {
			t.Errorf("ls %s output should start with Format:, got:\n%s", tt.file, stdout.String())
		}
		for _, w := range tt.wantContain {
			if !strings.Contains(stdout.String(), w) {
				t.Errorf("ls %s stdout missing %q; got:\n%s", tt.file, w, stdout.String())
			}
		}
	}
}

func TestRunCat(t *testing.T) {
	path := testimages.Path(t, "c64", "m.u.l.e.d64")
	var stdout, stderr bytes.Buffer
	if code := run([]string{"cat", path, "M.U.L.E."}, &stdout, &stderr); code != 0 {
		t.Fatalf("run(cat m.u.l.e.d64 M.U.L.E.) returned %d, want 0 (stderr: %s)", code, stderr.String())
	}
	if stdout.Len() != 82 || !bytes.HasPrefix(stdout.Bytes(), []byte{0x01, 0x08}) {
		t.Errorf("stdout = %d bytes, want 82 bytes loading at $0801", stdout.Len())
	}

	stdout.Reset()
	stderr.Reset()
	if code := run([]string{"cat", path, "nonexistent"}, &stdout, &stderr); code != 1 {
		t.Errorf("run(cat nonexistent) returned %d, want 1", code)
	}

	stdout.Reset()
	stderr.Reset()
	if code := run([]string{"cat", path}, &stdout, &stderr); code != 1 {
		t.Errorf("run(cat without path) returned %d, want 1", code)
	}
}

func TestRunDump(t *testing.T) {
	tests := []struct {
		file        string
		wantContain []string
	}{
		{
			file:        "images/Workbench v1.0 (1985)(Commodore).adf",
			wantContain: []string{"Amiga ADF Disk Image Dump", "Workbench", "Root Block: sector 880"},
		},
	}
	for _, tt := range tests {
		var stdout, stderr bytes.Buffer
		code := run([]string{"dump", tt.file}, &stdout, &stderr)
		if code != 0 {
			t.Errorf("run(dump %s) returned %d, want 0 (stderr: %s)", tt.file, code, stderr.String())
		}
		for _, w := range tt.wantContain {
			if !strings.Contains(stdout.String(), w) {
				t.Errorf("run(dump %s) output missing %q", tt.file, w)
			}
		}
	}
}

func TestRunLsATR(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if code := run([]string{"ls", "-l", "images/dos.atr"}, &stdout, &stderr); code != 0 {
		t.Fatalf("run(ls -l dos.atr) = %d (stderr: %s)", code, stderr.String())
	}
	for _, w := range []string{
		"Contents:\n  Type     Size Modified            Name\n",
		"  -        4875                     DOS.SYS",
		"  -        5126                     DUP.SYS",
		"  -          88                     AUTORUN.SYS",
		"Formatted:    90 KiB (92,160 bytes)",
		"In-use:  13.2%     95 sectors",
		"Free:    86.8%    625 sectors",
	} {
		if !strings.Contains(stdout.String(), w) {
			t.Errorf("stdout missing %q; got:\n%s", w, stdout.String())
		}
	}
	if strings.Contains(stdout.String(), "Unformatted") {
		t.Errorf("ATR output should not report unformatted capacity; got:\n%s", stdout.String())
	}

	stdout.Reset()
	if code := run([]string{"cat", "images/dos.atr", "AUTORUN.SYS"}, &stdout, &stderr); code != 0 || stdout.Len() != 88 {
		t.Errorf("cat AUTORUN.SYS = code %d, %d bytes; want 0, 88 bytes", code, stdout.Len())
	}
}

func TestRunLsIMG(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if code := run([]string{"ls", "-l", "images/lpmud.img"}, &stdout, &stderr); code != 0 {
		t.Fatalf("run(ls -l lpmud.img) = %d (stderr: %s)", code, stderr.String())
	}
	for _, w := range []string{
		"  d           0 1993-02-21 22:25:26 DOSP11/\n",
		"  d           0 2026-01-24 09:57:46 .fseventsd/\n",
		"Formatted:   720 KiB (737,280 bytes)",
		"Unformatted:    1 MB (1,000,000 bytes)",
		"In-use:  37.6%    542 sectors",
		"Free:    62.4%    898 sectors",
	} {
		if !strings.Contains(stdout.String(), w) {
			t.Errorf("stdout missing %q; got:\n%s", w, stdout.String())
		}
	}

	stdout.Reset()
	if code := run([]string{"cat", "images/lpmud.img", "DOSP11/LPMUD.ARJ"}, &stdout, &stderr); code != 0 || stdout.Len() != 264730 {
		t.Errorf("cat DOSP11/LPMUD.ARJ = code %d, %d bytes; want 0, 264730 bytes", code, stdout.Len())
	}
}

func TestRunLsSubdir(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if code := run([]string{"ls", "-l", "images/lpmud.img", "DOSP11"}, &stdout, &stderr); code != 0 {
		t.Fatalf("run(ls -l lpmud.img DOSP11) = %d (stderr: %s)", code, stderr.String())
	}
	want := "Contents of DOSP11/:\n  Type     Size Modified            Name\n  -      264730 1993-02-21 22:24:12 LPMUD.ARJ\n"
	if !strings.Contains(stdout.String(), want) {
		t.Errorf("stdout missing %q; got:\n%s", want, stdout.String())
	}

	stdout.Reset()
	if code := run([]string{"ls", "images/lpmud.img", "/.fseventsd/"}, &stdout, &stderr); code != 0 ||
		!strings.Contains(stdout.String(), "Contents of .fseventsd/:\n  fseventsd-uuid\n") {
		t.Errorf("ls lpmud.img /.fseventsd/ = code %d; got:\n%s", code, stdout.String())
	}

	for _, dir := range []string{"DOSP11/LPMUD.ARJ", "NOPE"} {
		stderr.Reset()
		if code := run([]string{"ls", "images/lpmud.img", dir}, &stdout, &stderr); code != 1 || !strings.Contains(stderr.String(), dir) {
			t.Errorf("ls lpmud.img %s = code %d, stderr %q; want error", dir, code, stderr.String())
		}
	}
}

func TestRunLsRecursive(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if code := run([]string{"ls", "-l", "-r", "images/lpmud.img"}, &stdout, &stderr); code != 0 {
		t.Fatalf("run(ls -l -r lpmud.img) = %d (stderr: %s)", code, stderr.String())
	}
	want := "Contents:\n  Type     Size Modified            Name\n" +
		"  d           0 1993-02-21 22:25:26 DOSP11/\n" +
		"  -      264730 1993-02-21 22:24:12 DOSP11/LPMUD.ARJ\n" +
		"  d           0 2026-01-24 09:57:46 .fseventsd/\n" +
		"  -          36 2026-01-24 09:58:06 .fseventsd/fseventsd-uuid\n"
	if !strings.Contains(stdout.String(), want) {
		t.Errorf("stdout missing %q; got:\n%s", want, stdout.String())
	}
}

// fakeDirs is a DirLister over an in-memory directory tree.
type fakeDirs map[string][]disk.Entry

func (f fakeDirs) ListDir(path string) ([]disk.Entry, error) {
	if e, ok := f[path]; ok {
		return e, nil
	}
	return nil, fmt.Errorf("%q not found", path)
}

func TestListRecursive(t *testing.T) {
	dirs := fakeDirs{
		"A":   {{Name: "B", IsDir: true}, {Name: "f2"}},
		"A/B": {{Name: "f3"}},
		"X/A": {{Name: "f4"}},
	}
	got, err := listRecursive(dirs, "", []disk.Entry{{Name: "A", IsDir: true}, {Name: "f1"}}, 0)
	if err != nil {
		t.Fatalf("listRecursive error: %v", err)
	}
	var names []string
	for _, e := range got {
		names = append(names, e.Name)
	}
	if w := "A A/B A/B/f3 A/f2 f1"; strings.Join(names, " ") != w {
		t.Errorf("names = %v, want %s", names, w)
	}

	// Paths passed to ListDir are relative to the starting directory.
	got, err = listRecursive(dirs, "X", []disk.Entry{{Name: "A", IsDir: true}}, 0)
	if err != nil || len(got) != 2 || got[1].Name != "A/f4" {
		t.Errorf("listRecursive(X) = %+v, %v; want A, A/f4", got, err)
	}

	// A directory containing itself must not recurse forever.
	cyclic := fakeDirs{}
	path := "d"
	for i := 0; i < maxDirDepth+2; i++ {
		cyclic[path] = []disk.Entry{{Name: "d", IsDir: true}}
		path += "/d"
	}
	if _, err := listRecursive(cyclic, "", []disk.Entry{{Name: "d", IsDir: true}}, 0); err == nil || !strings.Contains(err.Error(), "too deeply") {
		t.Errorf("listRecursive(cyclic) error = %v, want nested too deeply", err)
	}
}

func TestRunLsP81(t *testing.T) {
	const path = "images/ZX81_Kong (Paul Farrow 2010) [V1-01].P81"
	var stdout, stderr bytes.Buffer
	if code := run([]string{"ls", "-l", path}, &stdout, &stderr); code != 0 {
		t.Fatalf("run(ls -l %s) = %d (stderr: %s)", path, code, stderr.String())
	}
	if w := "  -       12513                     \"ZX81 KONG\"\n"; !strings.Contains(stdout.String(), w) {
		t.Errorf("stdout missing %q; got:\n%s", w, stdout.String())
	}
	if strings.Contains(stdout.String(), "Usage:") {
		t.Errorf("program file should not report disk usage; got:\n%s", stdout.String())
	}

	stdout.Reset()
	if code := run([]string{"cat", path, "ZX81 KONG"}, &stdout, &stderr); code != 0 || stdout.Len() != 12513 {
		t.Errorf("cat ZX81 KONG = code %d, %d bytes; want 0, 12513 bytes", code, stdout.Len())
	}
}

func TestRunLsTRS80(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if code := run([]string{"ls", "-l", "images/trs80profileiii.img"}, &stdout, &stderr); code != 0 {
		t.Fatalf("run(ls -l trs80profileiii.img) = %d (stderr: %s)", code, stderr.String())
	}
	for _, w := range []string{
		"Label:\n  TRSDOS\n",
		"  -        5120                     BASIC/CMD\n",
		"  -        9195                     EFCD/CMD\n",
		"Formatted:   180 KiB (184,320 bytes)",
		"In-use:  58.8%    423 sectors",
	} {
		if !strings.Contains(stdout.String(), w) {
			t.Errorf("stdout missing %q; got:\n%s", w, stdout.String())
		}
	}

	stdout.Reset()
	if code := run([]string{"cat", "images/trs80profileiii.img", "RM/CMD"}, &stdout, &stderr); code != 0 || stdout.Len() != 3384 {
		t.Errorf("cat RM/CMD = code %d, %d bytes; want 0, 3384 bytes", code, stdout.Len())
	}
}

func TestRunLsWarnings(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if code := run([]string{"ls", "images/maxell.img"}, &stdout, &stderr); code != 0 {
		t.Fatalf("run(ls maxell.img) = %d (stderr: %s)", code, stderr.String())
	}
	for _, w := range []string{
		"\nWarning: the FAT12 filesystem covers only the first 720 of 1440 sectors; the other 720 sectors are unreadable\n",
		"Warning: 274 unreadable sectors in the filesystem",
		"Formatted:   720 KiB (737,280 bytes)",
		"  In-use:   0.8%     12 sectors (6,144 bytes)\n" +
			"  Free:    49.2%    708 sectors (362,496 bytes)\n" +
			"  Extra:   50.0%    720 sectors (368,640 bytes)\n" +
			"  Total:  100.0%  1,440 sectors (737,280 bytes)\n",
	} {
		if !strings.Contains(stdout.String(), w) {
			t.Errorf("stdout missing %q; got:\n%s", w, stdout.String())
		}
	}

	// A 360 KiB filesystem with readable data after it is rejected.
	raw, err := os.ReadFile("images/maxell.img")
	if err != nil {
		t.Fatal(err)
	}
	copy(raw[1000*512:1001*512], make([]byte, 512))
	path := filepath.Join(t.TempDir(), "partial.img")
	if err := os.WriteFile(path, raw, 0o644); err != nil {
		t.Fatal(err)
	}
	stderr.Reset()
	if code := run([]string{"ls", path}, &stdout, &stderr); code != 1 ||
		!strings.Contains(stderr.String(), "unrecognized disk image:\n  IBM PC: image size 737280 does not match the FAT filesystem size 368640\n  TRS-80: ") {
		t.Errorf("ls partial.img = code %d, stderr %q; want size mismatch error", code, stderr.String())
	}

	stdout.Reset()
	if code := run([]string{"ls", "images/lpmud.img"}, &stdout, &stderr); code != 0 || strings.Contains(stdout.String(), "Warning") || strings.Contains(stdout.String(), "Extra:") {
		t.Errorf("ls lpmud.img = code %d, want no warnings or extra sectors; got:\n%s", code, stdout.String())
	}
}

func TestRunLsIMD(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if code := run([]string{"ls", "-l", "images/T80M4UPG.IMD"}, &stdout, &stderr); code != 0 {
		t.Fatalf("run(ls -l T80M4UPG.IMD) = %d (stderr: %s)", code, stderr.String())
	}
	for _, w := range []string{
		"  Geometry:   40 tracks, 1 head, 18 sectors/track, 256 bytes/sector\n" +
			"  Comment:    700-2298 MODEL 4/4P\n" +
			"              TRSDOS UPGRADE\n" +
			"              VER. 06.02.00 to 06.02.01\n\n",
		"Label:\n  UPGRADE6\n",
		"  -        1280                     BOOT/SYS\n",
		"  -         503 1985-06-02          MAKE621/JCL\n",
		"  -        8821 1985-06-02          MAKE621A/JCL\n",
		"  Total:  100.0%    720 sectors (184,320 bytes)",
	} {
		if !strings.Contains(stdout.String(), w) {
			t.Errorf("stdout missing %q; got:\n%s", w, stdout.String())
		}
	}

	stdout.Reset()
	if code := run([]string{"dump", "images/T80M4UPG.IMD"}, &stdout, &stderr); code != 0 {
		t.Fatalf("run(dump) = %d (stderr: %s)", code, stderr.String())
	}
	for _, w := range []string{"ImageDisk (IMD) Container", "Deleted data marks on cylinders: 20", "TRS-80 Model 4 TRSDOS 6.2 Disk Image Dump", "--0 MAKE621/JCL"} {
		if !strings.Contains(stdout.String(), w) {
			t.Errorf("dump missing %q; got:\n%s", w, stdout.String())
		}
	}

	stdout.Reset()
	if code := run([]string{"cat", "images/T80M4DOS.IMD", "BASIC/CMD"}, &stdout, &stderr); code != 0 || stdout.Len() != 21584 {
		t.Errorf("cat BASIC/CMD = code %d, %d bytes", code, stdout.Len())
	}
}

func TestRunLsC64Containers(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if code := run([]string{"ls", "-l", "-r", testimages.Path(t, "c64", "error info/0905.d64.d81")}, &stdout, &stderr); code != 0 {
		t.Fatalf("ls -l -r 0905.d64.d81 = %d (stderr: %s)", code, stderr.String())
	}
	for _, w := range []string{
		"  d       61440                     \"CODING TOOLS/\"",
		"  ?        7112                     \"COPIES 1541+1581/BURSTNIBBLER 1.9\"",
		"Warning: broken directory on disk",
		"Warning: 20 sectors have read errors in the error table (66/20,",
	} {
		if !strings.Contains(stdout.String(), w) {
			t.Errorf("stdout missing %q; got:\n%s", w, stdout.String())
		}
	}

	// A failed download (an HTML page) saved as .d64 is rejected.
	html := filepath.Join(t.TempDir(), "lotus.d64")
	if err := os.WriteFile(html, []byte("<!DOCTYPE html>\n<title>404 Not Found</title>\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	stderr.Reset()
	if code := run([]string{"ls", html}, &stdout, &stderr); code != 1 || !strings.Contains(stderr.String(), "invalid D64 disk size") {
		t.Errorf("ls of an HTML page saved as .d64 = code %d, stderr %q", code, stderr.String())
	}

	// Local images without a download source.
	t.Run("smashtv.g64", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		if code := run([]string{"ls", testimages.Path(t, "c64", "smashtv.g64")}, &stdout, &stderr); code != 0 {
			t.Fatalf("ls smashtv.g64 = %d (stderr: %s)", code, stderr.String())
		}
		for _, w := range []string{
			"Warning: 2 sectors could not be decoded from the GCR data (1/5, 6/18)",
			"  Extra:   14.8%    119 sectors (30,464 bytes)",
			"  Total:  100.0%    802 sectors (205,312 bytes)",
		} {
			if !strings.Contains(stdout.String(), w) {
				t.Errorf("stdout missing %q; got:\n%s", w, stdout.String())
			}
		}
	})
	t.Run("war64.tcrt", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		if code := run([]string{"cat", testimages.Path(t, "c64", "war64.tcrt"), "Arkanoid"}, &stdout, &stderr); code != 0 || stdout.Len() != 34242 {
			t.Errorf("cat war64.tcrt Arkanoid = code %d, %d bytes", code, stdout.Len())
		}
	})
}

// TestRunLsApple lists Apple II images from images/apple, downloading them
// first if needed.
func TestRunLsApple(t *testing.T) {
	tests := []struct {
		name        string
		wantContain []string
	}{
		{"Apple DOS 3.1 Master.d13", []string{"D13 (raw sector image, 13-sector)", "Filesystem: Apple DOS 3.1", "35 tracks, 1 head, 13 sectors/track, 256 bytes/sector",
			"Label:\n  \"DISK VOLUME 254\"\n", "Contents:\n  Type     Size Modified            Name", "Native\n", `\x08\x05\x0C\x0C\x0F HELLO`,
			"  -       10704                     APPLESOFT", "113 KiB (116,480 bytes)", "In-use:  39.3%    179 sectors", "Free:    60.7%    276 sectors"}},
		{"Apple DOS 3.1 Master.nib", []string{"NIB (nibblized image", "Filesystem: Apple DOS 3.1",
			"Label:\n  \"DISK VOLUME 254\"\n", `\x08\x05\x0C\x0C\x0F HELLO`, "  -       10704                     APPLESOFT",
			"113 KiB (116,480 bytes)", "In-use:  39.3%    179 sectors", "Free:    60.7%    276 sectors"}},
		{"Apple DOS 3.3 August 1980.dsk", []string{"Format:\n  Platform:   Apple II\n", "DSK (raw sector image, DOS 3.3 sector order)", "Filesystem: Apple DOS 3.3",
			"35 tracks, 1 head, 16 sectors/track, 256 bytes/sector", "  -        1139                     HELLO", "  -         333                     \"RETRIEVE TEXT\"",
			"Formatted:   140 KiB (143,360 bytes)", "In-use:  91.2%    511 sectors", "Free:     8.8%     49 sectors"}},
		{"DOS 3.2 System Master.woz", []string{`WOZ2 (bitstream image, created by "Applesauce v1.1")`, "Filesystem: Apple DOS 3.2",
			"35 tracks, 1 head, 13 sectors/track, 256 bytes/sector", "Label:\n  \"DISK VOLUME 254\"\n", "  -         138                     HELLO",
			"  -       13175                     APPLE-TREK", "  -        6629                     PINBALL", "113 KiB (116,480 bytes)",
			"In-use:  99.3%    452 sectors", "Free:     0.7%      3 sectors"}},
		{"ProDOS 2.4.2.dsk", []string{"DSK (raw sector image, DOS 3.3 sector order)", "Filesystem: ProDOS", "16 sectors/track",
			"Label:\n  /PRODOS.2.4.2\n", "  -       17128 2018-01-18 07:00:00 PRODOS", "  -         497 2018-01-18 06:54:00 README",
			"Formatted:   140 KiB (143,360 bytes)", "In-use: 100.0%    560 sectors"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := testimages.Path(t, "apple", tt.name)
			var stdout, stderr bytes.Buffer
			if code := run([]string{"ls", "-l", path}, &stdout, &stderr); code != 0 {
				t.Fatalf("ls -l %s = %d (stderr: %s)", tt.name, code, stderr.String())
			}
			for _, w := range tt.wantContain {
				if !strings.Contains(stdout.String(), w) {
					t.Errorf("ls -l %s stdout missing %q; got:\n%s", tt.name, w, stdout.String())
				}
			}
		})
	}

	// Not supported yet: Apple Pascal and CP/AM (800 KiB) disks.
	for name, want := range map[string]string{
		"Apple Pascal - Disk 0.po": "unrecognized Apple II 140K disk filesystem",
		"CPAM 3.5.po":              "invalid Apple II disk size: 819200 bytes",
	} {
		t.Run(name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			if code := run([]string{"ls", testimages.Path(t, "apple", name)}, &stdout, &stderr); code != 1 || !strings.Contains(stderr.String(), want) {
				t.Errorf("ls %s = code %d, stderr %q; want error %q", name, code, stderr.String(), want)
			}
		})
	}

	var stdout, stderr bytes.Buffer
	if code := run([]string{"cat", testimages.Path(t, "apple", "Apple DOS 3.1 Master.d13"), "COPY"}, &stdout, &stderr); code != 0 || stdout.Len() != 1407 {
		t.Errorf("cat COPY = code %d, %d bytes; want 1407", code, stdout.Len())
	}
}

// TestRunCRT uses an EasyFlash cartridge image downloaded into images/c64.
func TestRunCRT(t *testing.T) {
	path := testimages.Path(t, "c64", "EF3 Adrian Toolset.crt")
	for _, tc := range []struct {
		args        []string
		wantContain []string
	}{
		{[]string{"ls", "-l", path}, []string{"Format:\n  Platform:   Commodore 64\n", "CRT (cartridge image, version 1.0, hardware type 32, 76 CHIP packets)",
			"Contents:\n  Type     Size Modified            Name\n", "  -      622592", "\"ndefpack 1.0.1\""}},
		{[]string{"dump", path}, []string{"Commodore 64 Cartridge (CRT) Dump", "ndefpack 1.0.1", "CHIP Packets (76)"}},
	} {
		var stdout, stderr bytes.Buffer
		if code := run(tc.args, &stdout, &stderr); code != 0 {
			t.Fatalf("run(%s) = %d (stderr: %s)", tc.args[0], code, stderr.String())
		}
		for _, w := range tc.wantContain {
			if !strings.Contains(stdout.String(), w) {
				t.Errorf("run(%s) stdout missing %q; got:\n%s", tc.args[0], w, stdout.String())
			}
		}
	}
}

// TestRunLsC64 lists C64 images from images/c64, downloading them first if
// they have a download source (others are skipped when absent).
func TestRunLsC64(t *testing.T) {
	tests := []struct {
		name        string
		args        []string // ls flags
		wantContain []string
	}{
		{"m.u.l.e.d64", []string{"-l"}, []string{"Platform:   Commodore 64", "D64 (1541 disk image, 35 tracks)", "35 tracks, 1 head, 17-21 sectors/track, 256 bytes/sector",
			"Label:\n  M.U.L.E. 88,2A\n", "Contents:\n  Type     Size Modified            Name     Native\n", "  -          82                     M.U.L.E. \"M.U.L.E.\"",
			"170 KiB (174,848 bytes)", "250 KB (250,000 bytes)", "BAM:      2.8%     19 sectors", "Free:    57.8%    395 sectors"}},
		{"m.u.l.e.d64", nil, []string{"Contents:\n  M.U.L.E.\n  MULE.2\n  M\n"}},
		{"42 tracks/scan42.d64", nil, []string{"D64 (1541 disk image, 42 tracks)", "Filesystem: CBM DOS (SpeedDOS 40 track BAM)",
			"  Extra:    4.2%     34 sectors (8,704 bytes)", "  Total:  100.0%    802 sectors (205,312 bytes)"}},
		{"error info/0006x.d64", []string{"-l"}, []string{"D64 (1541 disk image, 35 tracks, with error table)",
			"Warning: broken directory on disk (track 13, sector 15: 23, READ ERROR (checksum error in data block))",
			"Warning: 1 sectors have read errors in the error table (13/15)"}},
		{"bam message/06-expert_v41.d64", nil, []string{"  Comment:    BETA VERSION EXPERT V4.1                  SN 66151122354\n"}},
		{"ultima v.d81", nil, []string{"D81 (1581 disk image, 80 tracks)", "80 tracks, 2 heads, 20 sectors/track, 256 bytes/sector", "Label:\n  \"ULTIMA V\" 65,3D\n"}},
		{"lemmings.dfi", nil, []string{"DFI (DreamLoad file archive, version 1.0, 14 tracks of 256 sectors)", "Filesystem: DreamLoad"}},
		{"choppin_mall.t64", nil, []string{"T64 (tape archive, version 1.0, 1 of 30 entries used)"}},
		{"1942.tap", nil, []string{"TAP (raw tape image, version 1, C64, PAL, 718070 pulses)"}},
		// Local images without a download source.
		{"40 tracks/the_sun.d64", nil, []string{"D64 (1541 disk image, 40 tracks)", "Filesystem: CBM DOS (SpeedDOS 40 track BAM)"}},
		{"force.g64", nil, []string{"G64 (GCR-1541 bitstream image, 84 half-tracks)", "35 tracks, 1 head, 17-21 sectors/track"}},
		{"war64.tcrt", nil, []string{"TCRT (tapecart image, version 1, 2012 KiB flash)", "Filesystem: TapcrtFileSys"}},
	}
	for _, tt := range tests {
		t.Run(strings.Join(append(tt.args, tt.name), " "), func(t *testing.T) {
			args := append(append([]string{"ls"}, tt.args...), testimages.Path(t, "c64", tt.name))
			var stdout, stderr bytes.Buffer
			if code := run(args, &stdout, &stderr); code != 0 {
				t.Fatalf("ls %s = %d (stderr: %s)", tt.name, code, stderr.String())
			}
			for _, w := range tt.wantContain {
				if !strings.Contains(stdout.String(), w) {
					t.Errorf("ls %s stdout missing %q; got:\n%s", tt.name, w, stdout.String())
				}
			}
		})
	}

	// A PRG file, extracted from a disk image.
	var prg, stderr bytes.Buffer
	if code := run([]string{"cat", testimages.Path(t, "c64", "m.u.l.e.d64"), "M.U.L.E."}, &prg, &stderr); code != 0 {
		t.Fatalf("cat M.U.L.E. = %d (stderr: %s)", code, stderr.String())
	}
	path := filepath.Join(t.TempDir(), "mule.prg")
	if err := os.WriteFile(path, prg.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		cmd  string
		want []string
	}{
		{"ls", []string{"PRG (program file, load address $0801)", "  mule.prg\n"}},
		{"dump", []string{"Commodore 64 PRG File Dump", "Load Address: 0x0801", "80 bytes"}},
	} {
		var stdout bytes.Buffer
		if code := run([]string{tc.cmd, path}, &stdout, &stderr); code != 0 {
			t.Fatalf("%s mule.prg = %d (stderr: %s)", tc.cmd, code, stderr.String())
		}
		for _, w := range tc.want {
			if !strings.Contains(stdout.String(), w) {
				t.Errorf("%s mule.prg stdout missing %q; got:\n%s", tc.cmd, w, stdout.String())
			}
		}
	}
}

func TestRunLsADF(t *testing.T) {
	const path = "images/amiga/ofs.adf"
	var stdout, stderr bytes.Buffer
	if code := run([]string{"ls", "-l", path}, &stdout, &stderr); code != 0 {
		t.Fatalf("ls -l ofs.adf = %d (stderr: %s)", code, stderr.String())
	}
	for _, w := range []string{
		"Platform:   Amiga\n  Image:      ADF (Amiga Disk File, raw sector image)\n  Filesystem: Amiga OFS\n",
		"Label:\n  Amiga\n",
		"Contents:\n  Type     Size Modified            Name\n  -          42 2026-11-07 14:26:58 .info\n",
		"  d           0 2026-11-07 14:27:59 level1/\n",
		"  -          21 2026-11-07 14:23:59 note\n",
		"Formatted:   880 KiB (901,120 bytes)",
		"In-use:   1.6%     28 sectors (14,336 bytes)",
		"Free:    98.4%  1,732 sectors (886,784 bytes)",
	} {
		if !strings.Contains(stdout.String(), w) {
			t.Errorf("stdout missing %q; got:\n%s", w, stdout.String())
		}
	}

	stdout.Reset()
	if code := run([]string{"ls", "-l", "-r", path}, &stdout, &stderr); code != 0 {
		t.Fatalf("ls -l -r ofs.adf = %d (stderr: %s)", code, stderr.String())
	}
	for _, w := range []string{
		"  d           0 2026-11-07 14:27:25 level1/level2/\n",
		"  -          21 2026-11-07 14:26:53 Trashcan/note2\n",
	} {
		if !strings.Contains(stdout.String(), w) {
			t.Errorf("ls -r stdout missing %q; got:\n%s", w, stdout.String())
		}
	}
	stdout.Reset()
	if code := run([]string{"cat", path, "Trashcan/note2"}, &stdout, &stderr); code != 0 || stdout.Len() != 21 {
		t.Errorf("cat Trashcan/note2 = code %d, %d bytes", code, stdout.Len())
	}

	stdout.Reset()
	if code := run([]string{"cat", path, "note"}, &stdout, &stderr); code != 0 || stdout.String() != "test\x1a\x00d\x00\n\x01,\x00dnote-cat" {
		t.Errorf("cat note = code %d, %q", code, stdout.String())
	}
	stdout.Reset()
	if code := run([]string{"dump", path}, &stdout, &stderr); code != 0 || !strings.Contains(stdout.String(), "Root Entries (9):") {
		t.Errorf("dump = code %d; got:\n%s", code, stdout.String())
	}
}
