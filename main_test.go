package main

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

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
	testFile := "test_data/Workbench v1.0 (1985)(Commodore).adf"
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
		{
			args:        []string{"ls", "test_data/JeepersCreepersPlus1-UNK.d64"},
			wantContain: []string{"Label:\n", "Contents:\n", "  JEEP.CREEPERS+\n", "170 KiB (174,848 bytes)", "250 KB (250,000 bytes)", "BAM:      2.8%     19 sectors", "Free:    82.4%    563 sectors"},
		},
		{
			args:        []string{"ls", "test_data/och0528v35.d64"},
			wantContain: []string{"Label:\n  \"OCH0528 V3.5 AT\" ST,2A\n", "Contents:\n", "  MENU\n", "  \"DISK MASTER\"\n", "BAM:      2.8%     19 sectors", "147 sectors", "517 sectors"},
		},
		{
			args:        []string{"ls", "-l", "test_data/JeepersCreepersPlus1-UNK.d64"},
			wantContain: []string{"Contents:\n  Type     Size Modified            Name", "Native\n", `"JEEP.CREEPERS+"`, "  -       25627", "JEEP.CREEPERS+", "Capacity:", "Usage:"},
		},
		{
			args:        []string{"ls", "-l", "test_data/flipandflop.prg"},
			wantContain: []string{"Contents:\n  Type     Size Modified            Name\n", "  -       25071", "flipandflop.prg"},
		},
		{
			args:        []string{"ls", "-l", "test_data/EF3 Adrian Toolset.crt"},
			wantContain: []string{"Contents:\n  Type     Size Modified            Name\n", "  -      622592", "\"ndefpack 1.0.1\""},
		},
		{
			args:        []string{"ls", "-l", "test_data/64 games 21.D64"},
			wantContain: []string{"Label:\n  \"64 GAMES 21\" GL,2A\n", "Contents:\n  Type     Size Modified            Name", "Native\n", `"V▂π🭻▂πππππππππππ"`, "  -        1798                     MENU", "  ?        8193", "Warning: broken directory on disk (track 18, sector 7: 23, READ ERROR (checksum error in data block))"},
		},
		{
			args:        []string{"ls", "-l", "test_data/ProDOS_2_4_3.po"},
			wantContain: []string{"Label:\n  /PRODOS.2.4.3\n", "Contents:\n  Type     Size Modified            Name\n", "  -       17128 2023-12-30 02:43:00 PRODOS", "  -         999 2023-12-30 02:43:00 README", "Formatted:   140 KiB (143,360 bytes)", "In-use:  90.7%    508 sectors"},
		},
		{
			args:        []string{"ls", "-l", "test_data/Apple DOS 3.1 Master.d13"},
			wantContain: []string{"Label:\n  \"DISK VOLUME 254\"\n", "Contents:\n  Type     Size Modified            Name", "Native\n", `\x08\x05\x0C\x0C\x0F HELLO`, "  -       10704                     APPLESOFT", "113 KiB (116,480 bytes)", "In-use:  39.3%    179 sectors", "Free:    60.7%    276 sectors"},
		},
		{
			args:        []string{"ls", "-l", "test_data/DOS 3.1 Master.nib"},
			wantContain: []string{"Label:\n  \"DISK VOLUME 254\"\n", "Contents:\n  Type     Size Modified            Name", "Native\n", `\x08\x05\x0C\x0C\x0F HELLO`, "  -       10704                     APPLESOFT", "113 KiB (116,480 bytes)", "In-use:  39.3%    179 sectors", "Free:    60.7%    276 sectors"},
		},
		{
			args:        []string{"ls", "-l", "test_data/DOS 3.2 System Master.woz"},
			wantContain: []string{"Label:\n  \"DISK VOLUME 254\"\n", "Contents:\n  Type     Size Modified            Name\n", "  -         138                     HELLO", "  -       13175                     APPLE-TREK", "  -        6629                     PINBALL", "113 KiB (116,480 bytes)", "In-use:  99.3%    452 sectors", "Free:     0.7%      3 sectors"},
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
		{"test_data/DOS 3.2 System Master.woz", []string{"Format:\n  Platform:   Apple II\n", "WOZ2 (bitstream image", "Filesystem: Apple DOS 3.2", "35 tracks, 1 head, 13 sectors/track, 256 bytes/sector"}},
		{"test_data/DOS 3.1 Master.nib", []string{"NIB (nibblized image", "Apple DOS 3.1"}},
		{"test_data/Apple DOS 3.1 Master.d13", []string{"D13 (raw sector image, 13-sector)", "13 sectors/track"}},
		{"test_data/ProDOS_2_4_3.po", []string{"PO (raw sector image, ProDOS sector order)", "Filesystem: ProDOS", "16 sectors/track"}},
		{"test_data/aptestdos.do", []string{"DO (raw sector image, DOS 3.3 sector order)", "Apple DOS 3.3"}},
		{"test_data/Workbench v1.0 (1985)(Commodore).adf", []string{"Platform:   Amiga", "Amiga OFS", "80 tracks, 2 heads, 11 sectors/track, 512 bytes/sector"}},
		{"test_data/och0528v35.d64", []string{"Platform:   Commodore 64", "D64 (1541 disk image, 35 tracks)", "35 tracks, 1 head, 17-21 sectors/track, 256 bytes/sector"}},
		{"test_data/dos.atr", []string{"Platform:   Atari 8-bit", "ATR (disk image", "Atari DOS 2.0S", "40 tracks, 1 head, 18 sectors/track, 128 bytes/sector"}},
		{"test_data/lpmud.img", []string{"Platform:   IBM PC", "IMG (raw sector image)", "Filesystem: FAT12", "80 tracks, 2 heads, 9 sectors/track, 512 bytes/sector"}},
		{"test_data/ZX81_Kong (Paul Farrow 2010) [V1-01].p", []string{"Platform:   Sinclair ZX81", "P (program file, memory image from $4009)"}},
		{"test_data/ZX81_Kong (Paul Farrow 2010) [V1-01].P81", []string{"Platform:   Sinclair ZX81", "P81 (named program file, 1 program)"}},
		{"test_data/trs80profileiii.img", []string{"Platform:   TRS-80 Model III", "IMG (raw sector image)", "Filesystem: TRSDOS 1.3", "40 tracks, 1 head, 18 sectors/track, 256 bytes/sector"}},
		{"test_data/mfgamania.img", []string{"Platform:   Atari 8-bit", "XFD (raw sector image, 128 byte sectors, all bits inverted)", "Atari DOS 2.0S"}},
		{"test_data/maxell.img", []string{"Platform:   IBM PC", "IMG (raw sector image; filesystem in the first 720 of 1440 sectors)", "80 tracks, 2 heads, 9 sectors/track, 512 bytes/sector"}},
		{"test_data/T80M4UPG.IMD", []string{"Platform:   TRS-80 Model 4", "IMD (ImageDisk 1.10, 250 kbps MFM)", "Filesystem: TRSDOS 6.2", "40 tracks, 1 head, 18 sectors/track, 256 bytes/sector"}},
		{"test_data/c64/ultima iv.d81", []string{"Platform:   Commodore 64", "D81 (1581 disk image, 80 tracks)", "80 tracks, 2 heads, 20 sectors/track, 256 bytes/sector"}},
		{"test_data/c64/40 tracks/the_sun.d64", []string{"D64 (1541 disk image, 40 tracks)", "Filesystem: CBM DOS (SpeedDOS 40 track BAM)"}},
		{"test_data/c64/force.g64", []string{"G64 (GCR-1541 bitstream image, 84 half-tracks)", "35 tracks, 1 head, 17-21 sectors/track"}},
		{"test_data/c64/lemmings.dfi", []string{"DFI (DreamLoad file archive, version 1.0, 14 tracks of 256 sectors)", "Filesystem: DreamLoad"}},
		{"test_data/c64/choppin_mall.t64", []string{"T64 (tape archive, version 1.0, 1 of 30 entries used)"}},
		{"test_data/c64/1942.tap", []string{"TAP (raw tape image, version 1, C64, PAL, 718070 pulses)"}},
		{"test_data/c64/war64.tcrt", []string{"TCRT (tapecart image, version 1, 2012 KiB flash)", "Filesystem: TapcrtFileSys"}},
		{"test_data/flipandflop.prg", []string{"PRG (program file, load address $0801)"}},
		{"test_data/EF3 Adrian Toolset.crt", []string{"CRT (cartridge image, version 1.0"}},
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
	var stdout, stderr bytes.Buffer
	code := run([]string{"cat", "test_data/och0528v35.d64", "MENU"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("run(cat och0528v35.d64 MENU) returned %d, want 0 (stderr: %s)", code, stderr.String())
	}
	if stdout.Len() != 95 {
		t.Errorf("stdout len = %d, want 95", stdout.Len())
	}

	stdout.Reset()
	stderr.Reset()
	code = run([]string{"cat", "test_data/och0528v35.d64", "nonexistent"}, &stdout, &stderr)
	if code != 1 {
		t.Errorf("run(cat nonexistent) returned %d, want 1", code)
	}

	stdout.Reset()
	stderr.Reset()
	code = run([]string{"cat", "test_data/och0528v35.d64"}, &stdout, &stderr)
	if code != 1 {
		t.Errorf("run(cat without path) returned %d, want 1", code)
	}
}

func TestRunDump(t *testing.T) {
	tests := []struct {
		file        string
		wantContain []string
	}{
		{
			file:        "test_data/Workbench v1.0 (1985)(Commodore).adf",
			wantContain: []string{"Amiga ADF Disk Image Dump", "Workbench", "Root Block: sector 880"},
		},
		{
			file:        "test_data/JeepersCreepersPlus1-UNK.d64",
			wantContain: []string{"Commodore 64 1541 Disk Image (D64) Dump", "JEEP.CREEPERS+", "101 sectors"},
		},
		{
			file:        "test_data/flipandflop.prg",
			wantContain: []string{"Commodore 64 PRG File Dump", "Load Address: 0x0801", "25071 bytes"},
		},
		{
			file:        "test_data/EF3 Adrian Toolset.crt",
			wantContain: []string{"Commodore 64 Cartridge (CRT) Dump", "ndefpack 1.0.1", "CHIP Packets (76)"},
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
	if code := run([]string{"ls", "-l", "test_data/dos.atr"}, &stdout, &stderr); code != 0 {
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
	if code := run([]string{"cat", "test_data/dos.atr", "AUTORUN.SYS"}, &stdout, &stderr); code != 0 || stdout.Len() != 88 {
		t.Errorf("cat AUTORUN.SYS = code %d, %d bytes; want 0, 88 bytes", code, stdout.Len())
	}
}

func TestRunLsIMG(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if code := run([]string{"ls", "-l", "test_data/lpmud.img"}, &stdout, &stderr); code != 0 {
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
	if code := run([]string{"cat", "test_data/lpmud.img", "DOSP11/LPMUD.ARJ"}, &stdout, &stderr); code != 0 || stdout.Len() != 264730 {
		t.Errorf("cat DOSP11/LPMUD.ARJ = code %d, %d bytes; want 0, 264730 bytes", code, stdout.Len())
	}
}

func TestRunLsSubdir(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if code := run([]string{"ls", "-l", "test_data/lpmud.img", "DOSP11"}, &stdout, &stderr); code != 0 {
		t.Fatalf("run(ls -l lpmud.img DOSP11) = %d (stderr: %s)", code, stderr.String())
	}
	want := "Contents of DOSP11/:\n  Type     Size Modified            Name\n  -      264730 1993-02-21 22:24:12 LPMUD.ARJ\n"
	if !strings.Contains(stdout.String(), want) {
		t.Errorf("stdout missing %q; got:\n%s", want, stdout.String())
	}

	stdout.Reset()
	if code := run([]string{"ls", "test_data/lpmud.img", "/.fseventsd/"}, &stdout, &stderr); code != 0 ||
		!strings.Contains(stdout.String(), "Contents of .fseventsd/:\n  fseventsd-uuid\n") {
		t.Errorf("ls lpmud.img /.fseventsd/ = code %d; got:\n%s", code, stdout.String())
	}

	for _, dir := range []string{"DOSP11/LPMUD.ARJ", "NOPE"} {
		stderr.Reset()
		if code := run([]string{"ls", "test_data/lpmud.img", dir}, &stdout, &stderr); code != 1 || !strings.Contains(stderr.String(), dir) {
			t.Errorf("ls lpmud.img %s = code %d, stderr %q; want error", dir, code, stderr.String())
		}
	}
}

func TestRunLsRecursive(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if code := run([]string{"ls", "-l", "-r", "test_data/lpmud.img"}, &stdout, &stderr); code != 0 {
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
	const path = "test_data/ZX81_Kong (Paul Farrow 2010) [V1-01].P81"
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
	if code := run([]string{"ls", "-l", "test_data/trs80profileiii.img"}, &stdout, &stderr); code != 0 {
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
	if code := run([]string{"cat", "test_data/trs80profileiii.img", "RM/CMD"}, &stdout, &stderr); code != 0 || stdout.Len() != 3384 {
		t.Errorf("cat RM/CMD = code %d, %d bytes; want 0, 3384 bytes", code, stdout.Len())
	}
}

func TestRunLsWarnings(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if code := run([]string{"ls", "test_data/maxell.img"}, &stdout, &stderr); code != 0 {
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
	raw, err := os.ReadFile("test_data/maxell.img")
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
	if code := run([]string{"ls", "test_data/lpmud.img"}, &stdout, &stderr); code != 0 || strings.Contains(stdout.String(), "Warning") || strings.Contains(stdout.String(), "Extra:") {
		t.Errorf("ls lpmud.img = code %d, want no warnings or extra sectors; got:\n%s", code, stdout.String())
	}
}

func TestRunLsIMD(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if code := run([]string{"ls", "-l", "test_data/T80M4UPG.IMD"}, &stdout, &stderr); code != 0 {
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
	if code := run([]string{"dump", "test_data/T80M4UPG.IMD"}, &stdout, &stderr); code != 0 {
		t.Fatalf("run(dump) = %d (stderr: %s)", code, stderr.String())
	}
	for _, w := range []string{"ImageDisk (IMD) Container", "Deleted data marks on cylinders: 20", "TRS-80 Model 4 TRSDOS 6.2 Disk Image Dump", "--0 MAKE621/JCL"} {
		if !strings.Contains(stdout.String(), w) {
			t.Errorf("dump missing %q; got:\n%s", w, stdout.String())
		}
	}

	stdout.Reset()
	if code := run([]string{"cat", "test_data/T80M4DOS.IMD", "BASIC/CMD"}, &stdout, &stderr); code != 0 || stdout.Len() != 21584 {
		t.Errorf("cat BASIC/CMD = code %d, %d bytes", code, stdout.Len())
	}
}

func TestRunLsC64Containers(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if code := run([]string{"ls", "-l", "-r", "test_data/c64/partitions/0912.d64.d81"}, &stdout, &stderr); code != 0 {
		t.Fatalf("ls -l -r 0912.d64.d81 = %d (stderr: %s)", code, stderr.String())
	}
	for _, w := range []string{
		"  d       61440                     \"CODING TOOLS/\"",
		"  ?        7112                     \"COPIES 1541+1581/BURSTNIBBLER 1.9\"",
		"Warning: broken directory on disk",
	} {
		if !strings.Contains(stdout.String(), w) {
			t.Errorf("stdout missing %q; got:\n%s", w, stdout.String())
		}
	}

	stdout.Reset()
	if code := run([]string{"ls", "test_data/c64/smashtv.g64"}, &stdout, &stderr); code != 0 {
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

	stdout.Reset()
	if code := run([]string{"cat", "test_data/c64/war64.tcrt", "Arkanoid"}, &stdout, &stderr); code != 0 || stdout.Len() != 34242 {
		t.Errorf("cat war64.tcrt Arkanoid = code %d, %d bytes", code, stdout.Len())
	}

	stderr.Reset()
	if code := run([]string{"ls", "test_data/c64/40 tracks/lotus esprit turbo challenge (1992)(gremlin) [s1].d64"}, &stdout, &stderr); code != 1 ||
		!strings.Contains(stderr.String(), "invalid D64 disk size: 808 bytes") {
		t.Errorf("ls of an HTML page saved as .d64 = code %d, stderr %q", code, stderr.String())
	}
}
