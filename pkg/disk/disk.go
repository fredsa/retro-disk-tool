package disk

import (
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"time"

	"github.com/fredsa/retro-disk-tool/cmd/rdt/pkg/adf"
	"github.com/fredsa/retro-disk-tool/cmd/rdt/pkg/c64"
)

type Entry struct {
	Name    string
	IsDir   bool
	Size    uint32
	ModTime *time.Time
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
	TotalBytes          int64
	InUseBytes          int64
	FreeBytes           int64
	HasUsage            bool // false for non-disk single files like PRG / CRT
}

type Reader interface {
	List() ([]Entry, error)
	Usage() (*Usage, error)
	Dump(w io.Writer) error
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
	default:
		return nil, fmt.Errorf("unsupported file extension: %s", ext)
	}
}

type adfAdapter struct {
	fs *adf.FileSystem
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

func (a *adfAdapter) Dump(w io.Writer) error {
	return a.fs.Dump(w)
}

type d64Adapter struct {
	d *c64.D64Disk
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
		TotalBytes:          u.TotalBytes,
		InUseBytes:          u.InUseBytes,
		FreeBytes:           u.FreeBytes,
		HasUsage:            u.HasUsage,
	}, nil
}

func (a *d64Adapter) Dump(w io.Writer) error {
	return a.d.Dump(w)
}

type prgAdapter struct {
	p *c64.PRGFile
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

func (a *prgAdapter) Dump(w io.Writer) error {
	return a.p.Dump(w)
}

type crtAdapter struct {
	c *c64.CRTFile
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

func (a *crtAdapter) Dump(w io.Writer) error {
	return a.c.Dump(w)
}
