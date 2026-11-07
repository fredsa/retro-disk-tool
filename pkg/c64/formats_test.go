package c64

import (
	"bytes"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/fredsa/retro-disk-tool/cmd/rdt/internal/testimages"
)

// readTest reads images/c64/<name>, downloading it first if it has a
// download source; the test is skipped if the image is not available.
func readTest(t *testing.T, name string) []byte {
	t.Helper()
	raw, err := os.ReadFile(testimages.Path(t, "c64", name))
	if err != nil {
		t.Fatalf("failed to read %s: %v", name, err)
	}
	return raw
}

func openWith[T any](t *testing.T, name string, open func(io.ReaderAt, int64) (T, error)) T {
	t.Helper()
	raw := readTest(t, name)
	v, err := open(bytes.NewReader(raw), int64(len(raw)))
	if err != nil {
		t.Fatalf("open %s: %v", name, err)
	}
	return v
}

func names(entries []DiskEntry) []string {
	var n []string
	for _, e := range entries {
		n = append(n, e.Name)
	}
	return n
}

func must[T any](v T, err error) T {
	if err != nil {
		panic(err)
	}
	return v
}

func findEntry(entries []DiskEntry, name string) *DiskEntry {
	for i := range entries {
		if entries[i].Name == name {
			return &entries[i]
		}
	}
	return nil
}

func checkUsage(t *testing.T, name string, u *DiskUsageInfo, total, free, bam, extra int) {
	t.Helper()
	if u.TotalSectors != total || u.FreeSectors != free || u.BAMSectors != bam || u.ExtraSectors != extra ||
		u.InUseSectors+u.FreeSectors+u.BAMSectors+u.ExtraSectors != u.TotalSectors || u.TotalBytes != int64(total)*256 {
		t.Errorf("%s: usage = %+v, want total %d free %d bam %d extra %d", name, u, total, free, bam, extra)
	}
}

func TestD64(t *testing.T) {
	d := openWith(t, "m.u.l.e.d64", OpenD64)
	if h := d.Header(); h.Name != "M.U.L.E." || h.ID != "88" || h.DOSType != "2A" {
		t.Errorf("header = %+v", h)
	}
	if n := names(must(d.ReadDirectory())); strings.Join(n, ",") != "M.U.L.E.,MULE.2,M" {
		t.Errorf("entries = %q", n)
	}
	u, _ := d.Usage()
	checkUsage(t, "m.u.l.e", u, 683, 395, 19, 0)
	if f := d.Format(); f.Image != "D64 (1541 disk image, 35 tracks)" || f.Comment != "" || u.UnformattedDesc != "250 KB" {
		t.Errorf("format = %+v", f)
	}
	if c, err := d.ReadFile("mule.2"); err != nil || len(c) != 32002 || c[0] != 0x00 || c[1] != 0x08 {
		t.Errorf("ReadFile(mule.2) = %d bytes, %v; want 32002 bytes loading at $0800", len(c), err)
	}
	// The loader: load address $0801, then BASIC "SYS 2088".
	if c, err := d.ReadFile("M.U.L.E."); err != nil || len(c) != 82 || !bytes.HasPrefix(c, []byte{0x01, 0x08, 0x26, 0x08, 0xC0, 0x07, 0x9E, ' ', '2', '0', '8', '8'}) {
		t.Errorf("ReadFile(M.U.L.E.) = % x, %v", c[:min(12, len(c))], err)
	}

	// Failed downloads (e.g. HTML error pages) are not disk images.
	html := []byte("<!DOCTYPE html>\n<html><title>404 Not Found</title></html>\n")
	if _, err := OpenD64(bytes.NewReader(html), int64(len(html))); err == nil {
		t.Error("HTML page opened as a D64")
	}
}

func TestD64FortyTwoTracks(t *testing.T) {
	for _, name := range []string{"42 tracks/scan42.d64", "error info/scan42.d64"} {
		d := openWith(t, name, OpenD64)
		if f := d.Format(); f.Tracks != 42 || f.Filesystem != "CBM DOS (SpeedDOS 40 track BAM)" {
			t.Errorf("%s: format = %+v", name, f)
		}
		// Tracks 41-42 are not in any BAM.
		u, _ := d.Usage()
		checkUsage(t, name, u, 802, 745, 19, 34)
		if n := names(must(d.ReadDirectory())); len(n) != 1 || n[0] != "SCANNER" {
			t.Errorf("%s: entries = %q", name, n)
		}
	}
}

// The scanner test disks mark one whole track with each error code 2-15
// ($0C-$0E are not defined error codes).
func TestErrorTable(t *testing.T) {
	for _, tc := range []struct {
		name   string
		tracks int
		image  string
	}{
		{"error info/scan35err.d64", 35, "D64 (1541 disk image, 35 tracks, with error table)"},
		{"error info/scan40err.d64", 40, "D64 (1541 disk image, 40 tracks, with error table)"},
		{"42 tracks/scan42err.d64", 42, "D64 (1541 disk image, 42 tracks, with error table)"},
		{"error info/scan42err.d64", 42, "D64 (1541 disk image, 42 tracks, with error table)"},
	} {
		d := openWith(t, tc.name, OpenD64)
		if f := d.Format(); f.Image != tc.image || f.Tracks != tc.tracks {
			t.Errorf("%s: format = %+v", tc.name, f)
		}
		if w := d.Warnings(); len(w) != 1 || !strings.HasPrefix(w[0], "294 sectors have read errors in the error table (2/0, 2/1,") {
			t.Errorf("%s: warnings = %q", tc.name, w)
		}
		for code := byte(2); code <= 15; code++ {
			if got := d.sectorErrorCode(int(code), 0); got != code {
				t.Errorf("%s: track %d error code $%02X, want $%02X", tc.name, code, got, code)
			}
		}
		if e := findEntry(must(d.ReadDirectory()), "SCANNER"); e == nil || e.Corrupt || e.Bytes != 828 {
			t.Errorf("%s: SCANNER = %+v", tc.name, e)
		}
	}
	if desc := errorDesc(0x0C); desc != "unknown error code $0C" {
		t.Errorf("errorDesc($0C) = %q", desc)
	}

	// A read error in a directory sector marks its entries as corrupt.
	raw := readTest(t, "error info/scan35err.d64")
	raw = append([]byte{}, raw...)
	raw[683*256+357+1] = 0x05 // error table entry for 18/1
	d, err := OpenD64(bytes.NewReader(raw), int64(len(raw)))
	if err != nil {
		t.Fatal(err)
	}
	if e := findEntry(must(d.ReadDirectory()), "SCANNER"); e == nil || !e.Corrupt || e.Error != "track 18, sector 1: 23, READ ERROR (checksum error in data block)" {
		t.Errorf("SCANNER with directory read error = %+v", e)
	}
}

func TestErrorInfoDisks(t *testing.T) {
	// A file whose chain reaches a sector with a read error.
	d := openWith(t, "error info/0006x.d64", OpenD64)
	entries := must(d.ReadDirectory())
	var corrupt []string
	for _, e := range entries {
		if e.Corrupt {
			corrupt = append(corrupt, e.Name+": "+e.Error)
		}
	}
	if len(entries) != 9 || len(corrupt) != 1 || !strings.HasSuffix(corrupt[0], ": track 13, sector 15: 23, READ ERROR (checksum error in data block)") {
		t.Errorf("0006x: %d entries, corrupt %q", len(entries), corrupt)
	}

	// An unreadable disk: every sector has "21, READ ERROR (no sync character)".
	d = openWith(t, "error info/0168.d64", OpenD64)
	w := d.Warnings()
	if len(w) != 3 || !strings.Contains(w[1], "the header/BAM sector (18/0) has a read error (21, READ ERROR (no sync character))") ||
		!strings.HasPrefix(w[2], "683 sectors have read errors") || len(must(d.ReadDirectory())) != 0 {
		t.Errorf("0168: warnings %q", w)
	}

	// 40 tracks without a BAM extension: tracks 36-40 are extra sectors.
	d = openWith(t, "error info/abacus-ada-b2b7ea.d64", OpenD64)
	u, _ := d.Usage()
	checkUsage(t, "abacus", u, 768, 143, 19, 85)
	if f := d.Format().Filesystem; f != "CBM DOS" || len(must(d.ReadDirectory())) != 21 {
		t.Errorf("abacus: filesystem %q", f)
	}

	// A D81 with an error table: files that reach bad sectors are corrupt.
	d81 := openWith(t, "error info/0923.d64.d81", OpenD81)
	if h := d81.Header(); h.Name != "NEUROMANCER" {
		t.Errorf("0923 header = %+v", h)
	}
	entries = must(d81.ReadDirectory())
	corrupt = nil
	for _, e := range entries {
		if e.Corrupt {
			corrupt = append(corrupt, e.Name+": "+e.Error)
		}
	}
	if len(entries) != 205 || strings.Join(corrupt, "; ") != "080B1: track 11, sector 0: 23, READ ERROR (checksum error in data block); "+
		"06002: track 11, sector 7: 23, READ ERROR (checksum error in data block)" {
		t.Errorf("0923: %d entries, corrupt %q", len(entries), corrupt)
	}
	if w := d81.Warnings(); len(w) != 1 || !strings.HasPrefix(w[0], "13 sectors have read errors in the error table (11/0,") {
		t.Errorf("0923 warnings = %q", w)
	}

	// Every error info disk opens and accounts for every sector.
	for _, name := range []string{"0027.d64", "0152.d64", "0180.d64", "0206_heroes_of_lance.d64", "0216_sceptre_of_b.d64", "0220.d64", "0587_cpx_upheav rips.d64"} {
		d := openWith(t, "error info/"+name, OpenD64)
		u, _ := d.Usage()
		if u.TotalSectors != 683 || len(d.Warnings()) == 0 {
			t.Errorf("%s: usage %+v, warnings %q", name, u, d.Warnings())
		}
	}
}

func TestBadListing(t *testing.T) {
	d := openWith(t, "bad listing/topsecretstuff.d64", OpenD64)
	if w := d.Warnings(); len(w) != 1 || !strings.Contains(w[0], "DOS version byte $14") {
		t.Errorf("warnings = %q", w)
	}
	entries := must(d.ReadDirectory())
	for name, want := range map[string]string{
		"VIEW BAM":    "invalid sector 152 for track 10",
		"TOP III":     "block chain ends after 17 of 37 blocks",
		"FAST FORMAT": "block chain ends after 2 of 10 blocks",
	} {
		if e := findEntry(entries, name); e == nil || !e.Corrupt || e.Error != want {
			t.Errorf("%s = %+v, want corrupt: %s", name, e, want)
		}
	}
	if e := findEntry(entries, "RTL-64"); e == nil || e.Corrupt || e.Bytes != 8171 {
		t.Errorf("RTL-64 = %+v", e)
	}
}

// Some disks hide a message in the unused part of the BAM sector.
func TestBAMMessage(t *testing.T) {
	for name, want := range map[string]string{
		"bam message/053_diskdevil5.d64":  "CHECK YOUR DIRECTORY ....",
		"bam message/06-expert_v41.d64":   "BETA VERSION EXPERT V4.1                  SN 66151122354",
		"bam message/60k_tool.d64":        "DDY: CENTRAX/SAMAR _NOE DAMIAN STUPIEN, SAPEROW 16B/8  42-612 T-GORY  POLAND !!!",
		"bam message/sb005157-df624d.d64": ".....WRITE       TO:  MICRO      MIX,PLK     123202C,4270 DORSTEN 11,WEST    GERMANY",
		"bam message/sb005230-c4fee2.d64": "*THIS DISK IS PREPARED WITH SUPER-DISK BY OLIVER REITZ TEL.06400/1801 HELLO",
	} {
		d := openWith(t, name, OpenD64)
		if got := d.BAMMessage(); got != want || d.Format().Comment != want {
			t.Errorf("%s: BAM message %q, want %q", name, got, want)
		}
	}
	if h := openWith(t, "bam message/053_diskdevil5.d64", OpenD64).Header(); h.Name != `NICHT M."*"LADEN` {
		t.Errorf("053_diskdevil5 name = %q", h.Name)
	}
}

func TestD81(t *testing.T) {
	d := openWith(t, "ultima v.d81", OpenD81)
	if h := d.Header(); h.Name != "ULTIMA V" || h.ID != "65" || h.DOSType != "3D" {
		t.Errorf("header = %+v", h)
	}
	if n := len(must(d.ReadDirectory())); n != 228 {
		t.Errorf("entries = %d, want 228", n)
	}
	u, _ := d.Usage()
	checkUsage(t, "ultima v", u, 3200, 524, 40, 0)
	if f := d.Format(); f.Tracks != 80 || f.Heads != 2 || f.Sectors != "20" || u.UnformattedDesc != "1 MB" {
		t.Errorf("format = %+v", f)
	}
}

func TestD81Partitions(t *testing.T) {
	// Partitions formatted as sub-directories, with an error table.
	d := openWith(t, "error info/0905.d64.d81", OpenD81)
	if f := d.Format().Image; f != "D81 (1581 disk image, 80 tracks, with error table)" {
		t.Errorf("image = %q", f)
	}
	var dirs []string
	for _, e := range must(d.ReadDirectory()) {
		if e.IsDir {
			dirs = append(dirs, e.Name)
		}
	}
	if strings.Join(dirs, ",") != "CODING TOOLS,COPIES 1541+1581,DIRECTORY+DRIVE!,GRAPHICS TOOLS !,PACKER+CRUNCHER!,WORK" {
		t.Errorf("sub-directories = %q", dirs)
	}
	sub, err := d.ListDir("COPIES 1541+1581")
	if err != nil {
		t.Fatal(err)
	}
	var burst *ListEntry
	for i := range sub {
		if sub[i].Name == "BURSTNIBBLER 1.9" {
			burst = &sub[i]
		}
	}
	if burst == nil || !burst.Corrupt || burst.Error != "block chain ends after 28 of 48 blocks; block chain leaves its partition (tracks 7-22) at track 1" {
		t.Errorf("BURSTNIBBLER = %+v", burst)
	}
	if c, err := d.ReadFile("COPIES 1541+1581/DREAM COPY V1.6+"); err != nil || len(c) != 4792 {
		t.Errorf("ReadFile in sub-directory = %d bytes, %v", len(c), err)
	}
	if w := d.Warnings(); len(w) != 1 || !strings.HasPrefix(w[0], "20 sectors have read errors in the error table (66/20,") {
		t.Errorf("warnings = %q", w)
	}
	if _, err := d.ListDir("NOPE"); err == nil {
		t.Error("ListDir(NOPE) succeeded")
	}
}

func TestDFI(t *testing.T) {
	d := openWith(t, "lemmings.dfi", OpenDFI)
	entries := must(d.ReadDirectory())
	if len(entries) != 123 || entries[0].Name != "!LEMMINGS" || entries[0].Bytes != 10303 {
		t.Errorf("entries: %d, first %+v", len(entries), entries[0])
	}
	for _, e := range entries {
		if e.Corrupt {
			t.Errorf("%s corrupt: %s", e.Name, e.Error)
		}
	}
	u, _ := d.Usage()
	checkUsage(t, "lemmings", u, 3584, 177, 0, 0)
	if f := d.Format(); f.Filesystem != "DreamLoad" || f.Tracks != 14 || f.Sectors != "256" {
		t.Errorf("format = %+v", f)
	}
	raw := readTest(t, "lemmings.dfi")
	if _, err := OpenDFI(bytes.NewReader(raw[:len(raw)-0x10000]), int64(len(raw)-0x10000)); err == nil {
		t.Error("DFI with a missing track opened")
	}
}

func TestT64(t *testing.T) {
	tf := openWith(t, "choppin_mall.t64", OpenT64)
	l, _ := tf.List()
	if len(l) != 1 || l[0].Name != "CHOPPIN' MALL" || l[0].Size != 26482 || tf.Header().Name != "215 1992" || len(tf.Warnings()) != 0 {
		t.Errorf("list = %+v, header %+v, warnings %q", l, tf.Header(), tf.Warnings())
	}
	c, err := tf.ReadFile("choppin' mall")
	if err != nil || len(c) != 26482 || c[0] != 0x01 || c[1] != 0x08 {
		t.Errorf("ReadFile = %d bytes, %v", len(c), err)
	}

	raw := readTest(t, "choppin_mall.t64")
	bad := append([]byte{}, raw...)
	bad[64+4], bad[64+5] = 0xC6, 0xC3 // a common bogus end address
	tf, err = OpenT64(bytes.NewReader(bad), int64(len(bad)))
	if err != nil || len(tf.Warnings()) != 1 || !strings.Contains(tf.Warnings()[0], "end address $C3C6") {
		t.Errorf("bogus end address: err %v, warnings %q", err, tf.Warnings())
	}
	gap := append([]byte{}, raw...)
	gap[64+8]++ // data no longer starts right after the directory
	if _, err := OpenT64(bytes.NewReader(gap), int64(len(gap))); err == nil {
		t.Error("T64 with a gap before the data opened")
	}
}

func TestTAP(t *testing.T) {
	tp := openWith(t, "1942.tap", OpenTAP)
	l, _ := tp.List()
	if len(l) != 1 || l[0].Name != "1942" || l[0].Size != 178 || l[0].Corrupt || len(tp.blocks) != 4 {
		t.Errorf("list = %+v, %d blocks", l, len(tp.blocks))
	}
	if w := tp.Warnings(); len(w) != 1 || !strings.Contains(w[0], "(97%) are not CBM ROM loader blocks") {
		t.Errorf("warnings = %q", w)
	}
	c, err := tp.ReadFile("1942")
	if err != nil || !bytes.HasPrefix(c, []byte{0x49, 0xCC, 0x0B, 0x08, 0x00, 0x00, 0x9E, '2', '0', '6', '1'}) {
		t.Errorf("ReadFile(1942) = % x, %v", c[:min(12, len(c))], err)
	}
	raw := readTest(t, "1942.tap")
	if _, err := OpenTAP(bytes.NewReader(raw[:len(raw)-1]), int64(len(raw)-1)); err == nil {
		t.Error("truncated TAP opened")
	}
}

// The tests below use local images without a download source; they are
// skipped when the images are absent.

func TestD64FortyTrackBAM(t *testing.T) {
	// The BAM entries for tracks 36-40 are at $B0, the only location that
	// agrees with the sectors the files use (the SpeedDOS and DolphinDOS
	// locations also hold self-consistent but wrong data).
	raw := readTest(t, "40 tracks/rocketsciencea.d64")
	d, err := OpenD64(bytes.NewReader(raw), int64(len(raw)))
	if err != nil {
		t.Fatal(err)
	}
	if f := d.Format(); f.Tracks != 40 || f.Filesystem != "CBM DOS (40 track BAM at $B0)" {
		t.Errorf("format = %+v", f)
	}
	u, _ := d.Usage()
	checkUsage(t, "rocketscience", u, 768, 57, 19, 0)
	// Without any extension, tracks 36-40 are outside the BAM.
	plain := append([]byte{}, raw...)
	copy(plain[0x16500+0xAC:0x16500+0xD4], make([]byte, 40))
	d, _ = OpenD64(bytes.NewReader(plain), int64(len(plain)))
	u, _ = d.Usage()
	if d.Format().Filesystem != "CBM DOS" || u.ExtraSectors != 85 || u.TotalSectors != 768 {
		t.Errorf("no extended BAM: format %+v usage %+v", d.Format(), u)
	}
	// the_sun.d64 uses the SpeedDOS location.
	if f := openWith(t, "40 tracks/the_sun.d64", OpenD64).Format().Filesystem; f != "CBM DOS (SpeedDOS 40 track BAM)" {
		t.Errorf("the_sun filesystem = %q", f)
	}
}

func TestG64(t *testing.T) {
	d := openWith(t, "force.g64", OpenG64)
	if n := len(must(d.ReadDirectory())); n != 12 || d.g64.decoded != 662 || d.g64.badSectors != 21 {
		t.Errorf("force: %d entries, %+v", n, d.g64)
	}
	if w := d.Warnings(); len(w) != 1 || !strings.HasPrefix(w[0], "21 sectors could not be decoded from the GCR data (1/0, 1/1") {
		t.Errorf("force warnings = %q", w)
	}
	// Copy protection data on tracks 36-42, which the BAM does not cover.
	d = openWith(t, "smashtv.g64", OpenG64)
	u, _ := d.Usage()
	checkUsage(t, "smashtv", u, 802, 187, 19, 119)
	d = openWith(t, "spirit_a_side_1-7a51b0.g64", OpenG64)
	if d.g64.decoded != 683 || d.g64.badSectors != 0 || len(d.g64.halfWithData) != 42 || len(d.Warnings()) != 0 {
		t.Errorf("spirit: %+v", d.g64)
	}
	raw := readTest(t, "force.g64")
	for name, data := range map[string][]byte{"extra byte": append(append([]byte{}, raw...), 0), "truncated": raw[:len(raw)-1]} {
		if _, err := OpenG64(bytes.NewReader(data), int64(len(data))); err == nil {
			t.Errorf("OpenG64(%s) succeeded", name)
		}
	}
}

func TestTCRT(t *testing.T) {
	tc := openWith(t, "war64.tcrt", OpenTCRT)
	l, _ := tc.List()
	if len(l) != 65 || l[0].Name != "bROWSER-p1x3lNET" || l[2].Name != "Arkanoid" || l[2].Size != 34242 || tc.Format().Filesystem != "TapcrtFileSys" {
		t.Errorf("list: %d entries, %+v %+v", len(l), l[0], l[2])
	}
	c, err := tc.ReadFile("ARKANOID")
	if err != nil || len(c) != 34242 || c[0] != 0x01 || c[1] != 0x08 {
		t.Errorf("ReadFile(ARKANOID) = %d bytes, %v", len(c), err)
	}
	tc = openWith(t, "dotc.tcrt", OpenTCRT)
	if l, _ := tc.List(); len(l) != 1 || l[0].Name != "DOTC+8[N0]" || l[0].Size != 849 || tc.Format().Filesystem != "" {
		t.Errorf("dotc list = %+v", l)
	}
	raw := readTest(t, "war64.tcrt")
	if _, err := OpenTCRT(bytes.NewReader(append(raw, 0)), int64(len(raw)+1)); err == nil {
		t.Error("TCRT with a trailing byte opened")
	}
}

func TestDirectoryStart(t *testing.T) {
	// The BAM's directory link (70/65) is a copy protection trick: like the
	// drive, the directory is read from 18/1.
	d := openWith(t, "autoswap/maniac-a.d64", OpenD64)
	n := names(must(d.ReadDirectory()))
	if len(n) != 12 || n[0] != "MANIAC MANSION" || n[2] != "MM1" {
		t.Errorf("maniac-a entries = %q", n)
	}
	if w := d.Warnings(); len(w) != 1 || w[0] != "the header's directory link (70/65) is not used by the drive; the directory starts at 18/1" {
		t.Errorf("maniac-a warnings = %q", w)
	}
	// A data disk without a BAM or directory.
	d = openWith(t, "autoswap/maniac-b.d64", OpenD64)
	if w := d.Warnings(); len(w) != 1 || !strings.Contains(w[0], "does not look like a 1541 BAM (DOS version byte $00)") {
		t.Errorf("maniac-b warnings = %q", w)
	}
}

func TestDFISmall(t *testing.T) {
	d := openWith(t, "skateordie.dfi", OpenDFI)
	if n := names(must(d.ReadDirectory())); strings.Join(n, ",") != "GET HIGH OR DIE!,SKATE OR DIE_WOW" {
		t.Errorf("skateordie entries = %q", n)
	}
}
