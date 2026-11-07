package c64

import (
	"bytes"
	"io"
	"os"
	"strings"
	"testing"
)

const testDir = "../../images/c64/"

func readTest(t *testing.T, name string) []byte {
	t.Helper()
	raw, err := os.ReadFile(testDir + name)
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

func checkUsage(t *testing.T, name string, u *DiskUsageInfo, total, free, bam, extra int) {
	t.Helper()
	if u.TotalSectors != total || u.FreeSectors != free || u.BAMSectors != bam || u.ExtraSectors != extra ||
		u.InUseSectors+u.FreeSectors+u.BAMSectors+u.ExtraSectors != u.TotalSectors || u.TotalBytes != int64(total)*256 {
		t.Errorf("%s: usage = %+v, want total %d free %d bam %d extra %d", name, u, total, free, bam, extra)
	}
}

func TestD64Formats(t *testing.T) {
	d := openWith(t, "m.u.l.e.d64", OpenD64)
	if h := d.Header(); h.Name != "M.U.L.E." || h.ID != "88" || h.DOSType != "2A" {
		t.Errorf("header = %+v", h)
	}
	entries, _ := d.ReadDirectory()
	if len(entries) != 3 {
		t.Errorf("entries = %v", names(entries))
	}
	u, _ := d.Usage()
	checkUsage(t, "m.u.l.e", u, 683, 395, 19, 0)

	// 40 tracks: the BAM entries for tracks 36-40 are at $B0, the only
	// location that agrees with the sectors the files use (the SpeedDOS and
	// DolphinDOS locations also hold self-consistent but wrong data).
	raw := readTest(t, "40 tracks/rocketsciencea.d64")
	d, err := OpenD64(bytes.NewReader(raw), int64(len(raw)))
	if err != nil {
		t.Fatal(err)
	}
	if f := d.Format(); f.Tracks != 40 || f.Filesystem != "CBM DOS (40 track BAM at $B0)" {
		t.Errorf("format = %+v", f)
	}
	u, _ = d.Usage()
	checkUsage(t, "rocketscience", u, 768, 57, 19, 0)
	for _, e := range must(d.ReadDirectory()) {
		if e.Corrupt {
			t.Errorf("%s corrupt: %s", e.Name, e.Error)
		}
	}
	// Without any extension, tracks 36-40 are outside the BAM.
	plain := append([]byte{}, raw...)
	copy(plain[0x16500+0xAC:0x16500+0xD4], make([]byte, 40))
	d, _ = OpenD64(bytes.NewReader(plain), int64(len(plain)))
	u, _ = d.Usage()
	if d.Format().Filesystem != "CBM DOS" || u.ExtraSectors != 85 || u.TotalSectors != 768 {
		t.Errorf("no extended BAM: format %+v usage %+v", d.Format(), u)
	}
	// the_sun.d64 uses the SpeedDOS location.
	d = openWith(t, "40 tracks/the_sun.d64", OpenD64)
	if f := d.Format().Filesystem; f != "CBM DOS (SpeedDOS 40 track BAM)" {
		t.Errorf("the_sun filesystem = %q", f)
	}

	// Failed downloads (HTML pages) are not disk images.
	if _, err := OpenD64(bytes.NewReader(readTest(t, "40 tracks/lotus esprit turbo challenge (1992)(gremlin) [s1].d64")), 808); err == nil {
		t.Error("808 byte HTML file opened as a D64")
	}
}

func must[T any](v T, err error) T {
	if err != nil {
		panic(err)
	}
	return v
}

func TestD81(t *testing.T) {
	d := openWith(t, "ultima iv.d81", OpenD81)
	if h := d.Header(); h.Name != "ULTIMA IV" || h.DOSType != "3D" {
		t.Errorf("header = %+v", h)
	}
	if n := len(must(d.ReadDirectory())); n != 292 {
		t.Errorf("entries = %d, want 292", n)
	}
	u, _ := d.Usage()
	checkUsage(t, "ultima iv", u, 3200, 1235, 40, 0)
	if f := d.Format(); f.Tracks != 80 || f.Heads != 2 || f.Sectors != "20" || u.UnformattedDesc != "1 MB" {
		t.Errorf("format = %+v", f)
	}

	// Partitions formatted as sub-directories.
	d = openWith(t, "partitions/0912.d64.d81", OpenD81)
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
	if _, err := d.ListDir("NOPE"); err == nil {
		t.Error("ListDir(NOPE) succeeded")
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
	if c, err := d.ReadFile("SOUL CRYSTAL <S>"); err != nil || len(c) != 7503 || c[0] != 0x01 || c[1] != 0x08 {
		t.Errorf("ReadFile(SOUL CRYSTAL <S>) = %d bytes, %v", len(c), err)
	}

	raw := readTest(t, "force.g64")
	for name, data := range map[string][]byte{"extra byte": append(append([]byte{}, raw...), 0), "truncated": raw[:len(raw)-1]} {
		if _, err := OpenG64(bytes.NewReader(data), int64(len(data))); err == nil {
			t.Errorf("OpenG64(%s) succeeded", name)
		}
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
	d = openWith(t, "skateordie.dfi", OpenDFI)
	if n := names(must(d.ReadDirectory())); strings.Join(n, ",") != "GET HIGH OR DIE!,SKATE OR DIE_WOW" {
		t.Errorf("skateordie entries = %q", n)
	}
	raw := readTest(t, "skateordie.dfi")
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

	raw := readTest(t, "mancopter.t64")
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
	l, _ = tc.List()
	if len(l) != 1 || l[0].Name != "DOTC+8[N0]" || l[0].Size != 849 || tc.Format().Filesystem != "" {
		t.Errorf("dotc list = %+v", l)
	}
	raw := readTest(t, "dotc.tcrt")
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
