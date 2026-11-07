// Package testimages provides disk images for tests. Images that are freely
// downloadable are not kept in the repository: they are fetched into
// images/<dir>/ the first time a test needs them, and verified against a
// pinned size and SHA-256.
package testimages

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

// Image is a downloadable test image.
type Image struct {
	Dir    string // directory under images/
	Name   string
	URL    string
	Size   int64
	SHA256 string
}

const appleOS = "https://mirrors.apple2.org.za/Apple%20II%20Documentation%20Project/Software/Operating%20Systems/"

// Manifest lists the downloadable images.
var Manifest = []Image{
	{"apple", "Apple DOS 3.1 Master.nib", appleOS + "Apple%20DOS/Disk%20Images/Apple%20DOS%203.1%20Master.nib",
		232960, "d0fd3db1dd4991af20f824b3b576eb307b0abd5ae6d7be6b8107e3b60f4cf40c"},
	{"apple", "Apple DOS 3.1 Master.d13", appleOS + "Apple%20DOS/Disk%20Images/Apple%20DOS%203.1%20Master.d13",
		116480, "39d0d8f8e28700db2db20d6bd76123f3e629c904942e34dc9d347d3fc3f682be"},
	{"apple", "Apple DOS 3.3 August 1980.dsk", appleOS + "Apple%20DOS/Disk%20Images/Apple%20DOS%203.3%20August%201980.dsk",
		143360, "84bb9e1db42cc3d005657e4f61d5e908c2a89f8069e65a9cb22c5cc55bc47c76"},
	{"apple", "ProDOS 2.4.2.dsk", appleOS + "Apple%20ProDOS/Disk%20Images/ProDOS%202.4.2.dsk",
		143360, "d1e6fab8d9a25acf6e10ffa15e4120a616898c03613ab87a8e12882b12102543"},
	{"apple", "Apple Pascal - Disk 0.po", appleOS + "Apple%20Pascal/Disk%20Images/Apple%20Pascal%20-%20Disk%200.po",
		143360, "ff5ce078485f98b1990a56c93843f153c1c4f85928ad1bf70e4eaebd354d22fc"},
	{"apple", "CPAM 3.5.po", appleOS + "Apple%20CPM/Disk%20Images/CPAM%203.5.po",
		819200, "00cfd502c1e198cf7ebd563a8af264e0899fcdf58ce02ed1f8b5133bfdb26fe2"},
}

// Root returns the repository's images directory.
func Root() string {
	_, file, _, _ := runtime.Caller(0)
	return filepath.Join(filepath.Dir(file), "..", "..", "images")
}

// Path returns the path of images/<dir>/<name>. An image listed in the
// Manifest is downloaded first if it is missing; the test is skipped if the
// image is not available (not downloadable, or the download failed).
func Path(t testing.TB, dir, name string) string {
	t.Helper()
	path := filepath.Join(Root(), dir, name)
	if _, err := os.Stat(path); err == nil {
		return path
	}
	for _, img := range Manifest {
		if img.Dir == dir && img.Name == name {
			if err := Fetch(img); err != nil {
				t.Skipf("test image %s/%s is not available: %v", dir, name, err)
			}
			return path
		}
	}
	t.Skipf("test image %s/%s is not available locally and has no download source", dir, name)
	return ""
}

// Fetch downloads an image if it does not exist locally, verifying its size
// and SHA-256 before moving it into place.
func Fetch(img Image) error {
	return fetchTo(filepath.Join(Root(), img.Dir, img.Name), img)
}

func fetchTo(path string, img Image) error {
	if _, err := os.Stat(path); err == nil {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	client := &http.Client{Timeout: 2 * time.Minute}
	resp, err := client.Get(img.URL)
	if err != nil {
		return fmt.Errorf("download failed: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download failed: %s", resp.Status)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, img.Size+1))
	if err != nil {
		return fmt.Errorf("download failed: %w", err)
	}
	sum := sha256.Sum256(data)
	if int64(len(data)) != img.Size || hex.EncodeToString(sum[:]) != img.SHA256 {
		return fmt.Errorf("downloaded %d bytes with SHA-256 %x, want %d bytes with SHA-256 %s", len(data), sum, img.Size, img.SHA256)
	}
	// Write to a temporary file and rename, so concurrent test processes
	// never see a partial image.
	tmp, err := os.CreateTemp(filepath.Dir(path), ".download-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if err := tmp.Chmod(0o644); err != nil {
		tmp.Close()
		return err
	}
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}
