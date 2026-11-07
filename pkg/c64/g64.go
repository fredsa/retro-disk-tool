package c64

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"sort"

	"github.com/fredsa/retro-disk-tool/cmd/rdt/pkg/util"
)

// gcrDecode maps 5 bit GCR codes to 4 bit values (-1 = invalid code).
var gcrDecode = func() [32]int {
	var t [32]int
	for i := range t {
		t[i] = -1
	}
	for v, code := range []int{0x0A, 0x0B, 0x12, 0x13, 0x0E, 0x0F, 0x16, 0x17, 0x09, 0x19, 0x1A, 0x1B, 0x0D, 0x1D, 0x1E, 0x15} {
		t[code] = v
	}
	return t
}()

// g64Info describes a G64 container and how its tracks decoded.
type g64Info struct {
	halfTracks   int
	maxTrackSize int
	withData     int // half-tracks with data
	halfWithData []int
	decoded      int // sectors decoded without errors
	badSectors   int // sectors missing or with checksum errors
	speedBlocks  int
}

// OpenG64 parses a G64 (GCR-1541) image and decodes its full tracks into a
// 1541 disk. Every byte of the file must belong to the header, the track
// and speed zone tables, or a track or speed zone block.
func OpenG64(r io.ReaderAt, size int64) (*CBMDisk, error) {
	if size < 12 {
		return nil, fmt.Errorf("G64 file too small: %d bytes", size)
	}
	raw := make([]byte, size)
	if _, err := r.ReadAt(raw, 0); err != nil {
		return nil, fmt.Errorf("failed to read G64 file: %w", err)
	}
	if !bytes.HasPrefix(raw, []byte("GCR-1541")) {
		return nil, fmt.Errorf("invalid G64 signature: %q", raw[:8])
	}
	if raw[8] != 0 {
		return nil, fmt.Errorf("unsupported G64 version %d", raw[8])
	}
	info := &g64Info{halfTracks: int(raw[9]), maxTrackSize: int(binary.LittleEndian.Uint16(raw[10:12]))}
	n := info.halfTracks
	tablesEnd := 12 + 8*n
	if n == 0 || n > 84 || tablesEnd > len(raw) {
		return nil, fmt.Errorf("invalid G64 track count %d", n)
	}

	type region struct{ start, end int }
	var regions []region
	tracks := make([][]byte, n)
	for i := 0; i < n; i++ {
		off := int(binary.LittleEndian.Uint32(raw[12+4*i:]))
		speed := int(binary.LittleEndian.Uint32(raw[12+4*n+4*i:]))
		if speed > 3 {
			// Offset of a speed zone block: 2 bits per track byte.
			end := speed + (info.maxTrackSize+3)/4
			if end > len(raw) {
				return nil, fmt.Errorf("G64 speed zone block for half-track %d beyond end of file", i)
			}
			regions = append(regions, region{speed, end})
			info.speedBlocks++
		}
		if off == 0 {
			continue
		}
		if off+2 > len(raw) {
			return nil, fmt.Errorf("G64 half-track %d beyond end of file", i)
		}
		length := int(binary.LittleEndian.Uint16(raw[off:]))
		end := off + 2 + info.maxTrackSize
		if length > info.maxTrackSize || end > len(raw) {
			return nil, fmt.Errorf("invalid G64 half-track %d: %d bytes at offset %d", i, length, off)
		}
		tracks[i] = raw[off+2 : off+2+length]
		regions = append(regions, region{off, end})
		info.withData++
		if i%2 == 1 {
			info.halfWithData = append(info.halfWithData, i)
		}
	}
	sort.Slice(regions, func(a, b int) bool { return regions[a].start < regions[b].start })
	pos := tablesEnd
	for _, rg := range regions {
		if rg.start != pos {
			return nil, fmt.Errorf("G64 layout does not account for bytes %d-%d", min(pos, rg.start), max(pos, rg.start)-1)
		}
		pos = rg.end
	}
	if pos != len(raw) {
		return nil, fmt.Errorf("G64 file has %d bytes after the last track", len(raw)-pos)
	}

	// Decode full tracks 1..42 (half-track index 2*(t-1)).
	type sec struct {
		data []byte
		code byte
	}
	decoded := map[int]map[int]sec{}
	maxTrack := 35
	for t := 1; t <= 42 && 2*(t-1) < n; t++ {
		secs := decodeGCRTrack(tracks[2*(t-1)], t)
		decoded[t] = map[int]sec{}
		for s, d := range secs {
			decoded[t][s] = sec{d.data, d.code}
			if t > 35 && d.code == 0x01 && s < d64SPT(t) {
				maxTrack = max(maxTrack, t)
			}
		}
	}
	if maxTrack > 35 {
		maxTrack = map[bool]int{true: 40, false: 42}[maxTrack <= 40]
	}

	l := d64Layout(maxTrack)
	data := make([]byte, 0, l.totalSectors()*256)
	errs := make([]byte, 0, l.totalSectors())
	bad := bytes.Repeat(util.BadSectorMarker, 256/len(util.BadSectorMarker))
	for t := 1; t <= maxTrack; t++ {
		for s := 0; s < d64SPT(t); s++ {
			d, ok := decoded[t][s]
			switch {
			case !ok:
				data, errs = append(data, bad...), append(errs, 0x02) // header block not found
				info.badSectors++
			default:
				data, errs = append(data, d.data...), append(errs, d.code)
				if d.code == 0x01 {
					info.decoded++
				} else {
					info.badSectors++
				}
			}
		}
	}
	d := newCBMDisk(data, errs, l, fmt.Sprintf("G64 (GCR-1541 bitstream image, %d half-tracks)", n))
	d.g64 = info
	d.errSource = "could not be decoded from the GCR data"
	return d, nil
}

type gcrSector struct {
	data []byte
	code byte // D64 error code: 0x01 OK, 0x04 no data block, 0x05 data checksum, 0x09 header checksum
}

// decodeGCRTrack finds the sectors of a 1541 track in its GCR bitstream.
func decodeGCRTrack(track []byte, trackNum int) map[int]gcrSector {
	out := map[int]gcrSector{}
	if len(track) == 0 {
		return out
	}
	nbits := len(track) * 8
	bit := func(i int) int {
		i %= nbits
		return int(track[i/8]>>(7-i%8)) & 1
	}
	// Block starts: the first bit after each sync (10 or more 1 bits).
	var starts []int
	ones := 0
	for i := 0; i < nbits+16; i++ {
		if bit(i) == 1 {
			ones++
			continue
		}
		if ones >= 10 {
			starts = append(starts, i)
		}
		ones = 0
	}
	decode := func(start, nbytes int) ([]byte, bool) {
		out := make([]byte, nbytes)
		for b := 0; b < nbytes; b++ {
			for half := 0; half < 2; half++ {
				code := 0
				for k := 0; k < 5; k++ {
					code = code<<1 | bit(start+b*10+half*5+k)
				}
				v := gcrDecode[code]
				if v < 0 {
					return out, false
				}
				out[b] |= byte(v) << (4 * (1 - half))
			}
		}
		return out, true
	}
	for k, st := range starts {
		hdr, ok := decode(st, 8)
		if !ok || hdr[0] != 0x08 {
			continue
		}
		s, t := int(hdr[2]), int(hdr[3])
		if t != trackNum || s >= d64SPT(trackNum) {
			continue
		}
		if prev, seen := out[s]; seen && prev.code == 0x01 {
			continue // already have a good copy
		}
		if hdr[1] != hdr[2]^hdr[3]^hdr[4]^hdr[5] {
			out[s] = gcrSector{data: bytes.Repeat(util.BadSectorMarker, 16), code: 0x09}
			continue
		}
		sector := gcrSector{data: bytes.Repeat(util.BadSectorMarker, 16), code: 0x04}
		if k+1 < len(starts) {
			if blk, ok := decode(starts[k+1], 258); ok && blk[0] == 0x07 {
				var sum byte
				for _, c := range blk[1:257] {
					sum ^= c
				}
				sector = gcrSector{data: blk[1:257], code: 0x01}
				if sum != blk[257] {
					sector.code = 0x05
				}
			}
		}
		out[s] = sector
	}
	return out
}

// dumpG64 describes the G64 container.
func (d *CBMDisk) dumpG64(w io.Writer) {
	g := d.g64
	fmt.Fprintf(w, "  G64: %d half-tracks (%d with data), max track size %d bytes, %d speed zone blocks\n",
		g.halfTracks, g.withData, g.maxTrackSize, g.speedBlocks)
	if len(g.halfWithData) > 0 {
		var ts []int
		for _, i := range g.halfWithData {
			ts = append(ts, i)
		}
		fmt.Fprintf(w, "  Half-tracks with data (index, track = index/2+1): %s\n", util.FormatRanges(ts))
	}
	fmt.Fprintf(w, "  GCR decoding: %d sectors decoded, %d missing or with checksum errors\n", g.decoded, g.badSectors)
}
