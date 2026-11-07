package disk

import (
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"time"

	"github.com/fredsa/retro-disk-tool/cmd/rdt/pkg/adf"
	"github.com/fredsa/retro-disk-tool/cmd/rdt/pkg/apple2"
	"github.com/fredsa/retro-disk-tool/cmd/rdt/pkg/atari"
	"github.com/fredsa/retro-disk-tool/cmd/rdt/pkg/c64"
	"github.com/fredsa/retro-disk-tool/cmd/rdt/pkg/ibm"
	"github.com/fredsa/retro-disk-tool/cmd/rdt/pkg/trs80"
	"github.com/fredsa/retro-disk-tool/cmd/rdt/pkg/util"
	"github.com/fredsa/retro-disk-tool/cmd/rdt/pkg/zx81"
)

type Entry struct {
	Name    string
	IsDir   bool
	Size    uint32
	ModTime *time.Time
	Corrupt bool
	Error   string
	// Native is the platform specific rendering of Name (nil if the
	// platform has no distinct rendering).
	Native []util.Glyph
}

type Usage struct {
	FormattedCapacity   int64
	FormattedDesc       string
	UnformattedCapacity int64
	UnformattedDesc     string
	SectorSize          int
	TotalSectors        int
	InUseSectors        int
	FreeSectors         int
	BAMSectors          int // System / BAM sectors (e.g. 19 sectors on C64 Track 18)
	TotalBytes          int64
	InUseBytes          int64
	FreeBytes           int64
	BAMBytes            int64
	HasUsage            bool // false for non-disk single files like PRG / CRT
}

type DiskHeader struct {
	Name    string
	ID      string
	DOSType string
}

type Reader interface {
	Format() *util.FormatInfo
	Header() *DiskHeader
	List() ([]Entry, error)
	Usage() (*Usage, error)
	ReadFile(path string) ([]byte, error)
	Dump(w io.Writer) error
}

// DirLister is implemented by readers whose filesystems have subdirectories.
type DirLister interface {
	// ListDir returns the entries of the directory at path ("" for root).
	ListDir(path string) ([]Entry, error)
}

// Warner is implemented by readers that can report problems with an image,
// such as unreadable sectors.
type Warner interface {
	Warnings() []string
}

func Open(filename string, r io.ReaderAt, size int64) (Reader, error) {
	ext := strings.ToLower(filepath.Ext(filename))
	switch ext {
	case ".adf":
		fs, err := adf.Open(r, size)
		if err != nil {
			return nil, err
		}
		return &adfAdapter{fs: fs}, nil
	case ".d64":
		d, err := c64.OpenD64(r, size)
		if err != nil {
			return nil, err
		}
		return &d64Adapter{d: d}, nil
	case ".prg":
		p, err := c64.OpenPRG(filename, r, size)
		if err != nil {
			return nil, err
		}
		return &prgAdapter{p: p}, nil
	case ".crt":
		c, err := c64.OpenCRT(filename, r, size)
		if err != nil {
			return nil, err
		}
		return &crtAdapter{c: c}, nil
	case ".atr":
		a, err := atari.Open(r, size)
		if err != nil {
			return nil, err
		}
		return &atariAdapter{a: a}, nil
	case ".img", ".ima", ".vfd":
		d, err := ibm.Open(r, size)
		if err == nil {
			return &ibmAdapter{d: d}, nil
		}
		if ext != ".img" {
			return nil, err
		}
		// .img is also used for TRS-80 and Atari raw sector images.
		t, terr := trs80.Open(r, size)
		if terr == nil {
			return &trs80Adapter{d: t}, nil
		}
		a, aerr := atari.OpenXFD(r, size)
		if aerr == nil {
			return &atariAdapter{a: a}, nil
		}
		return nil, fmt.Errorf("unrecognized disk image: IBM PC: %v; TRS-80: %v; Atari: %v", err, terr, aerr)
	case ".xfd":
		a, err := atari.OpenXFD(r, size)
		if err != nil {
			return nil, err
		}
		return &atariAdapter{a: a}, nil
	case ".p", ".p81":
		z, err := zx81.Open(filename, r, size)
		if err != nil {
			return nil, err
		}
		return &zx81Adapter{f: z}, nil
	case ".po", ".do", ".dsk", ".d13", ".nib", ".woz":
		img, err := apple2.Open(filename, r, size)
		if err != nil {
			return nil, err
		}
		return &apple2Adapter{img: img}, nil
	default:
		return nil, fmt.Errorf("unsupported file extension: %s", ext)
	}
}

type adfAdapter struct {
	fs *adf.FileSystem
}

func (a *adfAdapter) Header() *DiskHeader {
	name := a.fs.DiskName()
	if name == "" {
		return nil
	}
	return &DiskHeader{
		Name: name,
	}
}

func (a *adfAdapter) List() ([]Entry, error) {
	entries, err := a.fs.ListRoot()
	if err != nil {
		return nil, err
	}
	res := make([]Entry, len(entries))
	for i, e := range entries {
		t := e.ModTime
		res[i] = Entry{
			Name:    e.Name,
			IsDir:   e.IsDir,
			Size:    e.Size,
			ModTime: &t,
		}
	}
	return res, nil
}

func (a *adfAdapter) Usage() (*Usage, error) {
	u, err := a.fs.DiskUsage()
	if err != nil {
		return nil, err
	}
	return &Usage{
		FormattedCapacity:   u.FormattedCapacity,
		FormattedDesc:       "880 KiB",
		UnformattedCapacity: u.UnformattedCapacity,
		UnformattedDesc:     "1 MB",
		SectorSize:          u.SectorSize,
		TotalSectors:        u.TotalSectors,
		InUseSectors:        u.InUseSectors,
		FreeSectors:         u.FreeSectors,
		TotalBytes:          u.TotalBytes,
		InUseBytes:          u.InUseBytes,
		FreeBytes:           u.FreeBytes,
		HasUsage:            true,
	}, nil
}

func (a *adfAdapter) ReadFile(path string) ([]byte, error) {
	return a.fs.ReadFile(path)
}

func (a *adfAdapter) Dump(w io.Writer) error {
	return a.fs.Dump(w)
}

type d64Adapter struct {
	d *c64.D64Disk
}

func (a *d64Adapter) Header() *DiskHeader {
	h := a.d.Header()
	if h == nil {
		return nil
	}
	return &DiskHeader{
		Name:    h.Name,
		ID:      h.ID,
		DOSType: h.DOSType,
	}
}

func (a *d64Adapter) List() ([]Entry, error) {
	compat, err := a.d.List()
	if err != nil {
		return nil, err
	}
	res := make([]Entry, len(compat))
	for i, e := range compat {
		res[i] = Entry{
			Name:    e.Name,
			IsDir:   e.IsDir,
			Size:    e.Size,
			ModTime: e.ModTime,
			Corrupt: e.Corrupt,
			Error:   e.Error,
			Native:  e.Native,
		}
	}
	return res, nil
}

func (a *d64Adapter) Usage() (*Usage, error) {
	u, err := a.d.Usage()
	if err != nil {
		return nil, err
	}
	return &Usage{
		FormattedCapacity:   u.FormattedCapacity,
		FormattedDesc:       u.FormattedDesc,
		UnformattedCapacity: u.UnformattedCapacity,
		UnformattedDesc:     u.UnformattedDesc,
		SectorSize:          u.SectorSize,
		TotalSectors:        u.TotalSectors,
		InUseSectors:        u.InUseSectors,
		FreeSectors:         u.FreeSectors,
		BAMSectors:          u.BAMSectors,
		TotalBytes:          u.TotalBytes,
		InUseBytes:          u.InUseBytes,
		FreeBytes:           u.FreeBytes,
		BAMBytes:            u.BAMBytes,
		HasUsage:            u.HasUsage,
	}, nil
}

func (a *d64Adapter) ReadFile(path string) ([]byte, error) {
	return a.d.ReadFile(path)
}

func (a *d64Adapter) Dump(w io.Writer) error {
	return a.d.Dump(w)
}

type prgAdapter struct {
	p *c64.PRGFile
}

func (a *prgAdapter) Header() *DiskHeader {
	return nil
}

func (a *prgAdapter) List() ([]Entry, error) {
	compat, err := a.p.List()
	if err != nil {
		return nil, err
	}
	res := make([]Entry, len(compat))
	for i, e := range compat {
		res[i] = Entry{
			Name:    e.Name,
			IsDir:   e.IsDir,
			Size:    e.Size,
			ModTime: e.ModTime,
		}
	}
	return res, nil
}

func (a *prgAdapter) Usage() (*Usage, error) {
	u, err := a.p.Usage()
	if err != nil {
		return nil, err
	}
	return &Usage{
		HasUsage: u.HasUsage,
	}, nil
}

func (a *prgAdapter) ReadFile(path string) ([]byte, error) {
	return a.p.ReadFile(path)
}

func (a *prgAdapter) Dump(w io.Writer) error {
	return a.p.Dump(w)
}

type crtAdapter struct {
	c *c64.CRTFile
}

func (a *crtAdapter) Header() *DiskHeader {
	return nil
}

func (a *crtAdapter) List() ([]Entry, error) {
	compat, err := a.c.List()
	if err != nil {
		return nil, err
	}
	res := make([]Entry, len(compat))
	for i, e := range compat {
		res[i] = Entry{
			Name:    e.Name,
			IsDir:   e.IsDir,
			Size:    e.Size,
			ModTime: e.ModTime,
		}
	}
	return res, nil
}

func (a *crtAdapter) Usage() (*Usage, error) {
	u, err := a.c.Usage()
	if err != nil {
		return nil, err
	}
	return &Usage{
		HasUsage: u.HasUsage,
	}, nil
}

func (a *crtAdapter) ReadFile(path string) ([]byte, error) {
	return a.c.ReadFile(path)
}

func (a *crtAdapter) Dump(w io.Writer) error {
	return a.c.Dump(w)
}

type apple2Adapter struct {
	img apple2.Image
}

func (a *apple2Adapter) Header() *DiskHeader {
	h := a.img.Header()
	if h == nil {
		return nil
	}
	return &DiskHeader{
		Name:    h.Name,
		ID:      h.ID,
		DOSType: h.DOSType,
	}
}

func (a *apple2Adapter) List() ([]Entry, error) {
	entries, err := a.img.List()
	if err != nil {
		return nil, err
	}
	res := make([]Entry, len(entries))
	for i, e := range entries {
		res[i] = Entry{
			Name:    e.Name,
			IsDir:   e.IsDir,
			Size:    e.Size,
			ModTime: e.ModTime,
			Corrupt: e.Corrupt,
			Error:   e.Error,
			Native:  e.Native,
		}
	}
	return res, nil
}

func (a *apple2Adapter) Usage() (*Usage, error) {
	u, err := a.img.Usage()
	if err != nil {
		return nil, err
	}
	return &Usage{
		FormattedCapacity:   u.FormattedCapacity,
		FormattedDesc:       u.FormattedDesc,
		UnformattedCapacity: u.UnformattedCapacity,
		UnformattedDesc:     u.UnformattedDesc,
		SectorSize:          u.SectorSize,
		TotalSectors:        u.TotalSectors,
		InUseSectors:        u.InUseSectors,
		FreeSectors:         u.FreeSectors,
		BAMSectors:          u.BAMSectors,
		TotalBytes:          u.TotalBytes,
		InUseBytes:          u.InUseBytes,
		FreeBytes:           u.FreeBytes,
		BAMBytes:            u.BAMBytes,
		HasUsage:            u.HasUsage,
	}, nil
}

func (a *apple2Adapter) ReadFile(path string) ([]byte, error) {
	return a.img.ReadFile(path)
}

func (a *apple2Adapter) Dump(w io.Writer) error {
	return a.img.Dump(w)
}

func (a *adfAdapter) Format() *util.FormatInfo    { return a.fs.Format() }
func (a *d64Adapter) Format() *util.FormatInfo    { return a.d.Format() }
func (a *prgAdapter) Format() *util.FormatInfo    { return a.p.Format() }
func (a *crtAdapter) Format() *util.FormatInfo    { return a.c.Format() }
func (a *apple2Adapter) Format() *util.FormatInfo { return a.img.Format() }

type atariAdapter struct {
	a *atari.Disk
}

func (a *atariAdapter) Format() *util.FormatInfo { return a.a.Format() }

func (a *atariAdapter) Header() *DiskHeader { return nil }

func (a *atariAdapter) List() ([]Entry, error) {
	entries, err := a.a.List()
	if err != nil {
		return nil, err
	}
	res := make([]Entry, len(entries))
	for i, e := range entries {
		res[i] = Entry{
			Name:    e.Name,
			Size:    e.Size,
			Corrupt: e.Corrupt,
			Error:   e.Error,
		}
	}
	return res, nil
}

func (a *atariAdapter) Usage() (*Usage, error) {
	u, err := a.a.Usage()
	if err != nil {
		return nil, err
	}
	return &Usage{
		FormattedCapacity: u.FormattedCapacity,
		FormattedDesc:     a.a.FormattedDesc(),
		SectorSize:        u.SectorSize,
		TotalSectors:      u.TotalSectors,
		InUseSectors:      u.InUseSectors,
		FreeSectors:       u.FreeSectors,
		TotalBytes:        u.TotalBytes,
		InUseBytes:        u.InUseBytes,
		FreeBytes:         u.FreeBytes,
		HasUsage:          true,
	}, nil
}

func (a *atariAdapter) ReadFile(path string) ([]byte, error) { return a.a.ReadFile(path) }

func (a *atariAdapter) Dump(w io.Writer) error { return a.a.Dump(w) }

type ibmAdapter struct {
	d *ibm.Disk
}

func (a *ibmAdapter) Format() *util.FormatInfo { return a.d.Format() }

func (a *ibmAdapter) Header() *DiskHeader {
	label := a.d.Label()
	if label == "" {
		return nil
	}
	return &DiskHeader{Name: label}
}

func (a *ibmAdapter) List() ([]Entry, error) { return a.ListDir("") }

func (a *ibmAdapter) ListDir(path string) ([]Entry, error) {
	entries, err := a.d.ListDir(path)
	if err != nil {
		return nil, err
	}
	res := make([]Entry, len(entries))
	for i, e := range entries {
		res[i] = Entry{
			Name:    e.Name,
			IsDir:   e.IsDir,
			Size:    e.Size,
			ModTime: e.ModTime,
			Corrupt: e.Corrupt,
			Error:   e.Error,
		}
	}
	return res, nil
}

func (a *ibmAdapter) Usage() (*Usage, error) {
	u, err := a.d.Usage()
	if err != nil {
		return nil, err
	}
	return &Usage{
		FormattedCapacity:   u.FormattedCapacity,
		FormattedDesc:       u.FormattedDesc,
		UnformattedCapacity: u.UnformattedCapacity,
		UnformattedDesc:     u.UnformattedDesc,
		SectorSize:          u.SectorSize,
		TotalSectors:        u.TotalSectors,
		InUseSectors:        u.InUseSectors,
		FreeSectors:         u.FreeSectors,
		TotalBytes:          u.TotalBytes,
		InUseBytes:          u.InUseBytes,
		FreeBytes:           u.FreeBytes,
		HasUsage:            true,
	}, nil
}

func (a *ibmAdapter) Warnings() []string { return a.d.Warnings() }

func (a *ibmAdapter) ReadFile(path string) ([]byte, error) { return a.d.ReadFile(path) }

func (a *ibmAdapter) Dump(w io.Writer) error { return a.d.Dump(w) }

type zx81Adapter struct {
	f *zx81.File
}

func (a *zx81Adapter) Format() *util.FormatInfo { return a.f.Format() }

func (a *zx81Adapter) Header() *DiskHeader { return nil }

func (a *zx81Adapter) List() ([]Entry, error) {
	entries, err := a.f.List()
	if err != nil {
		return nil, err
	}
	res := make([]Entry, len(entries))
	for i, e := range entries {
		res[i] = Entry{Name: e.Name, Size: e.Size, Native: e.Native}
	}
	return res, nil
}

func (a *zx81Adapter) Usage() (*Usage, error) { return &Usage{}, nil }

func (a *zx81Adapter) ReadFile(path string) ([]byte, error) { return a.f.ReadFile(path) }

func (a *zx81Adapter) Dump(w io.Writer) error { return a.f.Dump(w) }

type trs80Adapter struct {
	d *trs80.Disk
}

func (a *trs80Adapter) Format() *util.FormatInfo { return a.d.Format() }

func (a *trs80Adapter) Header() *DiskHeader {
	label := a.d.Label()
	if label == "" {
		return nil
	}
	return &DiskHeader{Name: label}
}

func (a *trs80Adapter) List() ([]Entry, error) {
	entries, err := a.d.List()
	if err != nil {
		return nil, err
	}
	res := make([]Entry, len(entries))
	for i, e := range entries {
		res[i] = Entry{
			Name:    e.Name,
			Size:    e.Size,
			Corrupt: e.Corrupt,
			Error:   e.Error,
		}
	}
	return res, nil
}

func (a *trs80Adapter) Usage() (*Usage, error) {
	u, err := a.d.Usage()
	if err != nil {
		return nil, err
	}
	return &Usage{
		FormattedCapacity:   u.FormattedCapacity,
		FormattedDesc:       u.FormattedDesc,
		UnformattedCapacity: u.UnformattedCapacity,
		UnformattedDesc:     u.UnformattedDesc,
		SectorSize:          u.SectorSize,
		TotalSectors:        u.TotalSectors,
		InUseSectors:        u.InUseSectors,
		FreeSectors:         u.FreeSectors,
		TotalBytes:          u.TotalBytes,
		InUseBytes:          u.InUseBytes,
		FreeBytes:           u.FreeBytes,
		HasUsage:            true,
	}, nil
}

func (a *trs80Adapter) ReadFile(path string) ([]byte, error) { return a.d.ReadFile(path) }

func (a *trs80Adapter) Dump(w io.Writer) error { return a.d.Dump(w) }
