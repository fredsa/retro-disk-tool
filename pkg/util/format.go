package util

// FormatInfo describes a disk image file format and its basic geometry.
type FormatInfo struct {
	Platform   string // e.g. "Apple II"
	Image      string // image/container format, e.g. "NIB (nibblized tracks)"
	Filesystem string // e.g. "Apple DOS 3.3" (empty if not applicable)
	Tracks     int    // tracks (cylinders); 0 for non-disk files
	Heads      int    // sides / heads
	Sectors    string // sectors per track, e.g. "16" or "17-21"
	SectorSize int    // bytes per sector
	Comment    string // image description stored in the file (may be multi-line)
	// ContentsNote, if set, is shown instead of the Contents section, for
	// disks without files (e.g. a bootable disk without a DOS).
	ContentsNote string
}

