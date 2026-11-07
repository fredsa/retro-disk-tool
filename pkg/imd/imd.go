// Package imd decodes ImageDisk (IMD) floppy disk images into plain sector
// data. IMD files record each track's recording mode and sector layout, and
// each sector's data (possibly compressed to a single fill byte) along with
// its status (deleted data address mark, data error, unavailable).
package imd

import (
	"bytes"
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/fredsa/retro-disk-tool/cmd/rdt/pkg/util"
)

// Sector record types.
const (
	recUnavailable = 0
)

// modes describes the track recording modes.
var modes = []string{
	"500 kbps FM", "300 kbps FM", "250 kbps FM",
	"500 kbps MFM", "300 kbps MFM", "250 kbps MFM",
}

// Sector is one decoded sector.
type Sector struct {
	ID          int // sector number from the sector numbering map
	Data        []byte
	Deleted     bool // written with a deleted data address mark
	DataError   bool // read with a data (CRC) error
	Unavailable bool // could not be read; Data holds the bad sector marker
	Compressed  bool // stored as a single fill byte
}

// Track is one decoded track.
type Track struct {
	Mode       int
	Cylinder   int
	Head       int
	SectorSize int
	Sectors    []Sector // in the order recorded (interleave)
}

// Image is a decoded IMD file.
type Image struct {
	Header  string // e.g. "IMD 1.10: 15/01/2006  8:47:43"
	Comment string
	Tracks  []Track
}

// Geometry describes a uniform sector image.
type Geometry struct {
	Cylinders   int
	Heads       int
	Sectors     int // per track
	SectorSize  int
	FirstSector int // number of the first sector on each track (0 or 1)
	Mode        string
}

// Parse decodes an IMD file. Every byte of the file must be accounted for.
func Parse(r io.ReaderAt, size int64) (*Image, error) {
	raw := make([]byte, size)
	if _, err := r.ReadAt(raw, 0); err != nil {
		return nil, fmt.Errorf("failed to read IMD file: %w", err)
	}
	if !bytes.HasPrefix(raw, []byte("IMD ")) {
		return nil, fmt.Errorf("not an IMD file: missing \"IMD \" signature")
	}
	end := bytes.IndexByte(raw, 0x1A)
	if end < 0 {
		return nil, fmt.Errorf("invalid IMD file: header comment not terminated")
	}
	text := strings.ReplaceAll(string(raw[:end]), "\r\n", "\n")
	header, comment, _ := strings.Cut(text, "\n")
	img := &Image{Header: strings.TrimSpace(header), Comment: strings.TrimRight(comment, "\n")}

	p := end + 1
	need := func(n int) error {
		if p+n > len(raw) {
			return fmt.Errorf("truncated IMD file at offset %d (track %d)", p, len(img.Tracks))
		}
		return nil
	}
	for p < len(raw) {
		if err := need(5); err != nil {
			return nil, err
		}
		mode, cyl, head, nsec, sizeCode := int(raw[p]), int(raw[p+1]), int(raw[p+2]), int(raw[p+3]), int(raw[p+4])
		p += 5
		if mode >= len(modes) {
			return nil, fmt.Errorf("invalid IMD track mode %d at cylinder %d", mode, cyl)
		}
		if sizeCode > 6 {
			return nil, fmt.Errorf("invalid IMD sector size code %d at cylinder %d", sizeCode, cyl)
		}
		secSize := 128 << sizeCode
		if err := need(nsec); err != nil {
			return nil, err
		}
		ids := raw[p : p+nsec]
		p += nsec
		// Optional cylinder and head maps: sectors whose ID fields differ
		// from the physical position. Not needed to read the data.
		for _, flag := range []int{0x80, 0x40} {
			if head&flag != 0 {
				if err := need(nsec); err != nil {
					return nil, err
				}
				p += nsec
			}
		}
		t := Track{Mode: mode, Cylinder: cyl, Head: head & 0x3F, SectorSize: secSize}
		for i := 0; i < nsec; i++ {
			if err := need(1); err != nil {
				return nil, err
			}
			rec := int(raw[p])
			p++
			s := Sector{ID: int(ids[i])}
			switch {
			case rec == recUnavailable:
				s.Unavailable = true
				s.Data = bytes.Repeat(util.BadSectorMarker, (secSize+len(util.BadSectorMarker)-1)/len(util.BadSectorMarker))[:secSize]
			case rec <= 8:
				s.Compressed = rec%2 == 0
				s.Deleted = rec == 3 || rec == 4 || rec == 7 || rec == 8
				s.DataError = rec >= 5
				if s.Compressed {
					if err := need(1); err != nil {
						return nil, err
					}
					s.Data = bytes.Repeat(raw[p:p+1], secSize)
					p++
				} else {
					if err := need(secSize); err != nil {
						return nil, err
					}
					s.Data = raw[p : p+secSize]
					p += secSize
				}
			default:
				return nil, fmt.Errorf("invalid IMD sector record type %d at cylinder %d head %d", rec, cyl, t.Head)
			}
			t.Sectors = append(t.Sectors, s)
		}
		img.Tracks = append(img.Tracks, t)
	}
	if len(img.Tracks) == 0 {
		return nil, fmt.Errorf("IMD file has no tracks")
	}
	return img, nil
}

// Linear returns the sector data in cylinder, head, sector order, which
// requires every track to have the same layout: the same number and size of
// sectors, numbered contiguously, and every cylinder and head present once.
func (img *Image) Linear() ([]byte, *Geometry, error) {
	first := img.Tracks[0]
	g := &Geometry{Sectors: len(first.Sectors), SectorSize: first.SectorSize, Mode: modes[first.Mode], FirstSector: 256}
	tracks := map[[2]int]Track{}
	for _, t := range img.Tracks {
		if len(t.Sectors) != g.Sectors || t.SectorSize != g.SectorSize || modes[t.Mode] != g.Mode {
			return nil, nil, fmt.Errorf("non-uniform IMD track layout (cylinder %d head %d: %d x %d byte sectors, %s) is not supported",
				t.Cylinder, t.Head, len(t.Sectors), t.SectorSize, modes[t.Mode])
		}
		key := [2]int{t.Cylinder, t.Head}
		if _, dup := tracks[key]; dup {
			return nil, nil, fmt.Errorf("duplicate IMD track: cylinder %d head %d", t.Cylinder, t.Head)
		}
		tracks[key] = t
		g.Cylinders = max(g.Cylinders, t.Cylinder+1)
		g.Heads = max(g.Heads, t.Head+1)
		for _, s := range t.Sectors {
			g.FirstSector = min(g.FirstSector, s.ID)
		}
	}
	if g.Cylinders*g.Heads != len(img.Tracks) {
		return nil, nil, fmt.Errorf("incomplete IMD image: %d tracks for %d cylinders x %d heads", len(img.Tracks), g.Cylinders, g.Heads)
	}

	out := make([]byte, 0, len(img.Tracks)*g.Sectors*g.SectorSize)
	for c := 0; c < g.Cylinders; c++ {
		for h := 0; h < g.Heads; h++ {
			t := tracks[[2]int{c, h}]
			byID := make([][]byte, g.Sectors)
			for _, s := range t.Sectors {
				i := s.ID - g.FirstSector
				if i < 0 || i >= g.Sectors || byID[i] != nil {
					return nil, nil, fmt.Errorf("non-contiguous sector numbering on cylinder %d head %d", c, h)
				}
				byID[i] = s.Data
			}
			for _, d := range byID {
				out = append(out, d...)
			}
		}
	}
	return out, g, nil
}

// Stats counts sectors by status.
type Stats struct {
	Sectors, Compressed, Deleted, DataErrors, Unavailable int
	DeletedTracks                                         []int // cylinders with deleted data marks
}

// Stats counts sectors by status.
func (img *Image) Stats() Stats {
	var st Stats
	seen := map[int]bool{}
	for _, t := range img.Tracks {
		for _, s := range t.Sectors {
			st.Sectors++
			if s.Compressed {
				st.Compressed++
			}
			if s.Deleted {
				st.Deleted++
				if !seen[t.Cylinder] {
					seen[t.Cylinder] = true
					st.DeletedTracks = append(st.DeletedTracks, t.Cylinder)
				}
			}
			if s.DataError {
				st.DataErrors++
			}
			if s.Unavailable {
				st.Unavailable++
			}
		}
	}
	sort.Ints(st.DeletedTracks)
	return st
}

// Warnings describes sectors that could not be read correctly.
func (img *Image) Warnings() []string {
	st := img.Stats()
	var w []string
	if st.Unavailable > 0 {
		w = append(w, fmt.Sprintf("%d IMD sectors are unavailable (could not be read when imaged)", st.Unavailable))
	}
	if st.DataErrors > 0 {
		w = append(w, fmt.Sprintf("%d IMD sectors were read with data (CRC) errors; their contents may be wrong", st.DataErrors))
	}
	return w
}

// Dump writes a description of the IMD container.
func (img *Image) Dump(w io.Writer) {
	st := img.Stats()
	fmt.Fprintf(w, "ImageDisk (IMD) Container\n")
	fmt.Fprintf(w, "  Header: %s\n", img.Header)
	for _, line := range strings.Split(img.Comment, "\n") {
		fmt.Fprintf(w, "  Comment: %s\n", line)
	}
	fmt.Fprintf(w, "  Tracks: %d, Sectors: %d (%d compressed, %d deleted data marks, %d data errors, %d unavailable)\n",
		len(img.Tracks), st.Sectors, st.Compressed, st.Deleted, st.DataErrors, st.Unavailable)
	if len(st.DeletedTracks) > 0 {
		fmt.Fprintf(w, "  Deleted data marks on cylinders: %s\n", util.FormatRanges(st.DeletedTracks))
	}
	if len(img.Tracks) > 0 {
		t := img.Tracks[0]
		ids := make([]string, len(t.Sectors))
		for i, s := range t.Sectors {
			ids[i] = fmt.Sprintf("%d", s.ID)
		}
		fmt.Fprintf(w, "  Sector interleave (cylinder %d head %d): %s\n", t.Cylinder, t.Head, strings.Join(ids, " "))
	}
}
