package cpm

import (
	"bytes"
	"encoding/binary"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/fredsa/retro-disk-tool/cmd/rdt/internal/testimages"
	"github.com/fredsa/retro-disk-tool/cmd/rdt/pkg/util"
)

func TestOpenColumbia320(t *testing.T) {
	path := testimages.Path(t, "cpm", "cpm86-1-2x40x8x512-ibm.320.img")
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	fi, err := f.Stat()
	if err != nil {
		t.Fatal(err)
	}

	d, err := Open(f, fi.Size())
	if err != nil {
		t.Fatalf("Open(%s): %v", path, err)
	}

	fmtInfo := d.Format()
	if fmtInfo.Platform != "IBM PC" || fmtInfo.Filesystem != "CP/M-86" ||
		fmtInfo.Tracks != 40 || fmtInfo.Heads != 2 || fmtInfo.Sectors != "8" || fmtInfo.SectorSize != 512 {
		t.Errorf("Format() = %+v", fmtInfo)
	}

	entries, err := d.List()
	if err != nil {
		t.Fatalf("List(): %v", err)
	}
	if len(entries) != 21 {
		t.Fatalf("len(entries) = %d, want 21", len(entries))
	}
	if entries[0].Name != "15:CPM.SYS" || entries[0].Size != 12416 || !entries[0].ReadOnly || !entries[0].System {
		t.Errorf("entries[0] = %+v, want 15:CPM.SYS (12416 bytes, R/O, SYS)", entries[0])
	}

	// DMEXTR.BAS resides on side 1 (block 0x64) under out-and-out track ordering.
	got, err := d.ReadFile("DMEXTR.BAS")
	if err != nil {
		t.Fatalf("ReadFile(DMEXTR.BAS): %v", err)
	}
	if len(got) != 1408 {
		t.Errorf("len(DMEXTR.BAS) = %d, want 1408", len(got))
	}
	if !strings.Contains(string(got), "DISPLAY MANAGER EXTERNAL FUNCTIONS") {
		t.Errorf("DMEXTR.BAS content unexpected: %q", string(got[:min(80, len(got))]))
	}

	// Case-insensitive lookup and non-zero user area lookup.
	sys, err := d.ReadFile("15:cpm.sys")
	if err != nil || len(sys) != 12416 {
		t.Errorf("ReadFile(15:cpm.sys) = len %d, err %v; want 12416, nil", len(sys), err)
	}

	u, err := d.Usage()
	if err != nil {
		t.Fatalf("Usage(): %v", err)
	}
	if u.TotalSectors != 640 || u.InUseSectors+u.FreeSectors != u.TotalSectors {
		t.Errorf("Usage() = %+v", u)
	}

	var dump bytes.Buffer
	if err := d.Dump(&dump); err != nil {
		t.Fatalf("Dump(): %v", err)
	}
	if !strings.Contains(dump.String(), "out-and-out") || !strings.Contains(dump.String(), "DMEXTR.BAS") {
		t.Errorf("Dump() missing expected details:\n%s", dump.String())
	}
}

func TestOpen800K5x1024(t *testing.T) {
	path := testimages.Path(t, "cpm", "cpm86-4-2x80x5x1024.img")
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	fi, err := f.Stat()
	if err != nil {
		t.Fatal(err)
	}

	d, err := Open(f, fi.Size())
	if err != nil {
		t.Fatalf("Open(%s): %v", path, err)
	}

	fmtInfo := d.Format()
	if fmtInfo.Tracks != 80 || fmtInfo.Heads != 2 || fmtInfo.Sectors != "5" || fmtInfo.SectorSize != 1024 {
		t.Errorf("Format() = %+v", fmtInfo)
	}

	entries, err := d.List()
	if err != nil {
		t.Fatalf("List(): %v", err)
	}
	if len(entries) == 0 || entries[0].Name != "7:RAIL.ASC" {
		t.Fatalf("entries[0] = %+v, want 7:RAIL.ASC", entries)
	}

	rail, err := d.ReadFile("7:RAIL.ASC")
	if err != nil {
		t.Fatalf("ReadFile(7:RAIL.ASC): %v", err)
	}
	if !strings.Contains(string(rail), `50 INPUT "ENTER X,Y,Z : "`) || !strings.Contains(string(rail), "390 END") {
		t.Errorf("7:RAIL.ASC content unexpected: %q", string(rail))
	}

	bat, err := d.ReadFile("BACKUP.BAT")
	if err != nil {
		t.Fatalf("ReadFile(BACKUP.BAT): %v", err)
	}
	if !strings.Contains(string(bat), "pip k:[g0]=*.*[g0ra") || !strings.Contains(string(bat), "pip k:[g15]=*.*[g15ra") {
		t.Errorf("BACKUP.BAT content unexpected: %q", string(bat))
	}

	u, err := d.Usage()
	if err != nil {
		t.Fatalf("Usage(): %v", err)
	}
	if u.TotalSectors != 800 || u.InUseSectors+u.FreeSectors != u.TotalSectors {
		t.Errorf("Usage() = %+v", u)
	}
}

// buildSyntheticImage constructs a CP/M image with the given spec and files.
type synthFile struct {
	user     int
	name     string // 8.3
	data     []byte
	readOnly bool
	system   bool
	archive  bool
	exactBC  bool       // use CP/M 3 S1 exact byte count
	modTime  *time.Time // CP/M 3 SFCB timestamp
}

func buildSyntheticImage(t *testing.T, spec Spec, label string, files []synthFile) []byte {
	t.Helper()
	imgSize := spec.Cylinders * spec.Heads * spec.SectorsPerTrack * spec.SectorSize
	img := make([]byte, imgSize)
	// Fill with 0xE5 (standard CP/M formatted state).
	for i := range img {
		img[i] = 0xE5
	}
	// Write non-trivial boot sector + optional ID byte.
	if spec.SectorSize == 512 && imgSize >= 512 {
		img[0] = 0xEB
		img[1] = 0xFE
		if spec.HasIDByte {
			img[511] = spec.IDByte
		}
	}

	d := newDisk(img, spec, "test")
	writeBlock := func(blk int, buf []byte) {
		secsPerBlock := spec.BlockSize / spec.SectorSize
		for s := 0; s < secsPerBlock; s++ {
			phys := d.physSectorIndex(blk*secsPerBlock + s)
			off := phys * spec.SectorSize
			copy(img[off:off+spec.SectorSize], buf[s*spec.SectorSize:(s+1)*spec.SectorSize])
		}
	}

	dirBuf := make([]byte, spec.dirBlocks()*spec.BlockSize)
	for i := range dirBuf {
		dirBuf[i] = 0xE5
	}

	hasStamps := false
	for _, f := range files {
		if f.modTime != nil {
			hasStamps = true
		}
	}

	slot := 0
	nextSlot := func() int {
		if hasStamps && slot%4 == 3 {
			// Reserve every 4th slot for SFCB timestamps.
			sfcb := dirBuf[slot*32 : (slot+1)*32]
			for i := range sfcb {
				sfcb[i] = 0
			}
			sfcb[0] = userStamp
			slot++
		}
		s := slot
		slot++
		return s
	}

	if label != "" {
		s := nextSlot()
		e := dirBuf[s*32 : (s+1)*32]
		e[0] = userLabel
		for i := 1; i < 12; i++ {
			e[i] = ' '
		}
		copy(e[1:12], label)
		e[12] = 0x61
		for i := 13; i < 32; i++ {
			e[i] = 0
		}
	}

	nextBlk := spec.dirBlocks()
	use16 := spec.use16BitPtrs()
	ptrs := spec.ptrsPerExtent()
	exm := spec.extentMask()
	bytesPerEntry := ptrs * spec.BlockSize

	for _, f := range files {
		base, ext, _ := strings.Cut(f.name, ".")
		totalRecs := (len(f.data) + recordSize - 1) / recordSize
		if totalRecs == 0 {
			totalRecs = 1
		}
		nEntries := (len(f.data) + bytesPerEntry - 1) / bytesPerEntry
		if nEntries == 0 {
			nEntries = 1
		}

		for ei := 0; ei < nEntries; ei++ {
			s := nextSlot()
			e := dirBuf[s*32 : (s+1)*32]
			for i := range e {
				e[i] = 0
			}
			e[0] = byte(f.user)
			for i := 1; i < 12; i++ {
				e[i] = ' '
			}
			copy(e[1:9], base)
			copy(e[9:12], ext)
			if f.readOnly {
				e[9] |= 0x80
			}
			if f.system {
				e[10] |= 0x80
			}
			if f.archive {
				e[11] |= 0x80
			}

			chunkStart := ei * bytesPerEntry
			chunkEnd := min(len(f.data), chunkStart+bytesPerEntry)
			chunk := f.data[chunkStart:chunkEnd]
			chunkRecs := (len(chunk) + recordSize - 1) / recordSize
			if len(f.data) == 0 {
				chunkRecs = 0
			}

			// Compute logical extent number and RC for this directory entry.
			subExt := 0
			rc := chunkRecs
			if rc > 128 {
				subExt = (rc - 1) / 128
				rc -= subExt * 128
			}
			extNum := ei*(exm+1) + subExt
			e[12] = byte(extNum & 0x1F)
			e[14] = byte((extNum >> 5) & 0x3F)
			e[15] = byte(rc)
			if f.exactBC && ei == nEntries-1 && len(f.data)%recordSize != 0 {
				e[13] = byte(len(f.data) % recordSize)
			}

			// Allocate blocks for this entry.
			nBlks := (len(chunk) + spec.BlockSize - 1) / spec.BlockSize
			for bi := 0; bi < nBlks; bi++ {
				blkNum := nextBlk
				nextBlk++
				blkBuf := make([]byte, spec.BlockSize)
				bStart := bi * spec.BlockSize
				bEnd := min(len(chunk), bStart+spec.BlockSize)
				copy(blkBuf, chunk[bStart:bEnd])
				writeBlock(blkNum, blkBuf)
				if use16 {
					binary.LittleEndian.PutUint16(e[16+bi*2:18+bi*2], uint16(blkNum))
				} else {
					e[16+bi] = byte(blkNum)
				}
			}

			if ei == 0 && f.modTime != nil {
				sfcbSlot := (s/4)*4 + 3
				sfcb := dirBuf[sfcbSlot*32 : (sfcbSlot+1)*32]
				if sfcb[0] != userStamp {
					for i := range sfcb {
						sfcb[i] = 0
					}
					sfcb[0] = userStamp
				}
				epoch := time.Date(1978, 1, 1, 0, 0, 0, 0, time.UTC)
				days := uint16(f.modTime.Sub(epoch)/(24*time.Hour)) + 1
				toBCD := func(v int) byte { return byte((v/10)<<4 | (v % 10)) }
				sub := sfcb[1+(s%4)*10 : 1+(s%4+1)*10]
				binary.LittleEndian.PutUint16(sub[4:6], days)
				sub[6] = toBCD(f.modTime.Hour())
				sub[7] = toBCD(f.modTime.Minute())
			}
		}
	}

	for b := 0; b < spec.dirBlocks(); b++ {
		writeBlock(b, dirBuf[b*spec.BlockSize:(b+1)*spec.BlockSize])
	}
	return img
}

func TestSyntheticFormats(t *testing.T) {
	ts := time.Date(1985, 6, 15, 14, 30, 0, 0, time.UTC)
	bigPayload := bytes.Repeat([]byte("CPM86-MULTI-EXTENT-DATA!"), 2000) // 48,000 bytes (> 32 KiB)

	tests := []struct {
		name   string
		spec   Spec
		label  string
		files  []synthFile
		expect string
	}{
		{
			name:  "160K SS",
			spec:  standardFormats[0],
			files: []synthFile{{name: "HELLO.TXT", data: []byte("Hello CP/M-86 160K!\r\n")}},
		},
		{
			name: "320K DS OutBack multi-extent",
			spec: standardFormats[1],
			files: []synthFile{
				{name: "BIG.DAT", data: bigPayload, readOnly: true},
				{user: 3, name: "NOTE.TXT", data: []byte("user 3 file"), exactBC: true, modTime: &ts},
			},
			label: "CPM86DISK",
		},
		{
			name: "720K 144FEAT (16-bit ptrs, track straddle)",
			spec: standardFormats[8],
			files: []synthFile{
				{name: "LARGE.BIN", data: bigPayload},
				{name: "SMALL.TXT", data: []byte("720K test"), exactBC: true},
			},
		},
		{
			name: "1.44M 144FEAT",
			spec: standardFormats[14],
			files: []synthFile{
				{name: "HDFILE.CMD", data: bigPayload, system: true, archive: true},
			},
		},
		{
			name: "8-inch SSSD skew 6",
			spec: standardFormats[16],
			files: []synthFile{
				{name: "CPM8.TXT", data: bytes.Repeat([]byte("8-inch SSSD!"), 200)},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			raw := buildSyntheticImage(t, tt.spec, tt.label, tt.files)
			d, err := Open(bytes.NewReader(raw), int64(len(raw)))
			if err != nil {
				t.Fatalf("Open() error: %v", err)
			}
			if d.Label() != tt.label {
				t.Errorf("Label() = %q, want %q", d.Label(), tt.label)
			}
			entries, err := d.List()
			if err != nil {
				t.Fatalf("List() error: %v", err)
			}
			if len(entries) != len(tt.files) {
				t.Fatalf("len(entries) = %d, want %d", len(entries), len(tt.files))
			}
			for i, f := range tt.files {
				e := entries[i]
				got, err := d.ReadFile(e.Name)
				if err != nil {
					t.Fatalf("ReadFile(%s): %v", e.Name, err)
				}
				if f.exactBC {
					if !bytes.Equal(got, f.data) {
						t.Errorf("ReadFile(%s) mismatch: got %d bytes, want %d", e.Name, len(got), len(f.data))
					}
				} else {
					if !bytes.HasPrefix(got, f.data) {
						t.Errorf("ReadFile(%s) does not start with expected payload", e.Name)
					}
				}
				if f.modTime != nil {
					if e.ModTime == nil || !e.ModTime.Equal(*f.modTime) {
						t.Errorf("ModTime = %v, want %v", e.ModTime, f.modTime)
					}
				}
			}
		})
	}
}

func TestMBRPartition0xDB(t *testing.T) {
	// Build a minimal 512 KiB hard disk image with an MBR type 0xDB partition.
	const (
		totalSecs = 1024
		partLBA   = 16
		spt       = 16 // 64 CP/M records/track
		off       = 2  // 2 tracks = 32 sectors (directory starts at LBA 32)
		blockSize = 2048
		dsm       = 247 // (1024 - 32) * 512 / 2048 = 248 blocks
		drm       = 63
	)
	raw := make([]byte, totalSecs*512)
	for i := range raw {
		raw[i] = 0xE5
	}
	// MBR signature and partition 1 entry (type 0xDB).
	raw[510], raw[511] = 0x55, 0xAA
	p1 := raw[446 : 446+16]
	p1[4] = 0xDB
	binary.LittleEndian.PutUint32(p1[8:12], partLBA)
	binary.LittleEndian.PutUint32(p1[12:16], totalSecs-partLBA)

	// Digital Research hard disk label at partLBA + 3, with 15-byte DPB at offset 41.
	lbl := raw[(partLBA+3)*512 : (partLBA+4)*512]
	for i := range lbl {
		lbl[i] = 0
	}
	dpb := lbl[41 : 41+15]
	binary.LittleEndian.PutUint16(dpb[0:2], spt*4)
	dpb[2] = 4  // BSH for 2K
	dpb[3] = 15 // BLM
	dpb[4] = 1  // EXM
	binary.LittleEndian.PutUint16(dpb[5:7], dsm)
	binary.LittleEndian.PutUint16(dpb[7:9], drm)
	dpb[9] = 0x80
	binary.LittleEndian.PutUint16(dpb[13:15], off)
	// Adjust last word so 256-word little-endian checksum is 0.
	var sum uint16
	for o := 0; o < 510; o += 2 {
		sum += binary.LittleEndian.Uint16(lbl[o : o+2])
	}
	binary.LittleEndian.PutUint16(lbl[510:512], uint16(0-sum))

	// Write a directory entry at LBA 32 (block 0) pointing to block 1 (LBA 36).
	dir := raw[32*512 : 32*512+32]
	for i := range dir {
		dir[i] = 0
	}
	dir[0] = 0
	copy(dir[1:9], "README  ")
	copy(dir[9:12], "TXT")
	dir[13] = 12 // exact byte count
	dir[15] = 1  // 1 record
	dir[16] = 1  // block 1
	copy(raw[36*512:], "CP/M-86 HDD!")

	d, err := Open(bytes.NewReader(raw), int64(len(raw)))
	if err != nil {
		t.Fatalf("Open(MBR 0xDB) error: %v", err)
	}
	got, err := d.ReadFile("README.TXT")
	if err != nil || string(got) != "CP/M-86 HDD!" {
		t.Errorf("ReadFile(README.TXT) = %q, %v; want \"CP/M-86 HDD!\", nil", string(got), err)
	}
}

func TestBadSectorWarning(t *testing.T) {
	spec := standardFormats[0] // 160K
	raw := buildSyntheticImage(t, spec, "", []synthFile{
		{name: "BAD.TXT", data: []byte("corrupted file")},
	})
	// Overwrite block 2 (first data block: boottrk=1 -> 8 sectors, dir=2 blocks -> 4 sectors => sector 12)
	copy(raw[12*512:13*512], util.BadSectorFill(512))

	d, err := Open(bytes.NewReader(raw), int64(len(raw)))
	if err != nil {
		t.Fatalf("Open() error: %v", err)
	}
	entries, _ := d.List()
	if len(entries) != 1 || !entries[0].Corrupt {
		t.Errorf("entries = %+v, want Corrupt=true", entries)
	}
	if w := d.Warnings(); len(w) != 1 || !strings.Contains(w[0], "12") {
		t.Errorf("Warnings() = %v, want warning mentioning sector 12", w)
	}
}

func TestOpenRejects(t *testing.T) {
	allE5 := bytes.Repeat([]byte{0xE5}, 320*1024)
	for name, data := range map[string][]byte{
		"too small":   make([]byte, 100),
		"odd size":    make([]byte, 10000),
		"blank 320K":  make([]byte, 320*1024),
		"all 0xE5":    allE5,
		"blank 1440K": make([]byte, 1440*1024),
	} {
		if _, err := Open(bytes.NewReader(data), int64(len(data))); err == nil {
			t.Errorf("Open(%s) succeeded, want error", name)
		}
	}
}
