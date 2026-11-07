// Package testimages provides disk images for tests. Images committed to the
// repository live in images/<dir>/. Freely downloadable images are not kept
// in the repository: they are fetched into downloads/<dir>/ (gitignored) the
// first time a test needs them, and verified against a pinned size and
// SHA-256.
package testimages

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

// Image is a downloadable test image.
type Image struct {
	Dir       string // directory under images/
	Name      string // file name, possibly in a sub-directory ("42 tracks/scan42.d64")
	URL       string
	Size      int64
	SHA256    string
	UserAgent string // User-Agent header to send, if the server needs one
}

// meatloaf is an image from the Meatloaf project's C64 test collection,
// which asks clients to identify themselves as Meatloaf.
func meatloaf(name string, size int64, sha string) Image {
	return Image{Dir: "c64", Name: name, URL: "https://c64.meatloaf.cc/test/" + (&url.URL{Path: name}).EscapedPath(),
		Size: size, SHA256: sha, UserAgent: "MEATLOAF/"}
}

const appleOS = "https://mirrors.apple2.org.za/Apple%20II%20Documentation%20Project/Software/Operating%20Systems/"

// Manifest lists the downloadable images.
var Manifest = []Image{
	{Dir: "apple", Name: "Apple DOS 3.1 Master.nib", URL: appleOS + "Apple%20DOS/Disk%20Images/Apple%20DOS%203.1%20Master.nib",
		Size: 232960, SHA256: "d0fd3db1dd4991af20f824b3b576eb307b0abd5ae6d7be6b8107e3b60f4cf40c"},
	{Dir: "apple", Name: "Apple DOS 3.1 Master.d13", URL: appleOS + "Apple%20DOS/Disk%20Images/Apple%20DOS%203.1%20Master.d13",
		Size: 116480, SHA256: "39d0d8f8e28700db2db20d6bd76123f3e629c904942e34dc9d347d3fc3f682be"},
	{Dir: "apple", Name: "Apple DOS 3.3 August 1980.dsk", URL: appleOS + "Apple%20DOS/Disk%20Images/Apple%20DOS%203.3%20August%201980.dsk",
		Size: 143360, SHA256: "84bb9e1db42cc3d005657e4f61d5e908c2a89f8069e65a9cb22c5cc55bc47c76"},
	{Dir: "apple", Name: "ProDOS 2.4.2.dsk", URL: appleOS + "Apple%20ProDOS/Disk%20Images/ProDOS%202.4.2.dsk",
		Size: 143360, SHA256: "d1e6fab8d9a25acf6e10ffa15e4120a616898c03613ab87a8e12882b12102543"},
	{Dir: "apple", Name: "DOS 3.2 System Master.woz", URL: "https://mirrors.apple2.org.za/ftp.apple.asimov.net/images/masters/DOS%203.2%20System%20Master.woz",
		Size: 234496, SHA256: "aae07a4477e8c7a273fc1ca435bf31f3c87ca3fb9927c46922cd8e07c8a0ee33"},
	{Dir: "c64", Name: "EF3 Adrian Toolset.crt", URL: "https://github.com/misterblack1/ef3-c64-tools/raw/dfe7c04b262f9605113ab12b344e064a79ed4695/EF3%20Adrian%20Toolset.crt",
		Size: 623872, SHA256: "0be17c0212a0dcf1dc30645ba51a7b0291b960da91a00933c0756140603b30e1"},
	{Dir: "apple", Name: "Apple Pascal - Disk 0.po", URL: appleOS + "Apple%20Pascal/Disk%20Images/Apple%20Pascal%20-%20Disk%200.po",
		Size: 143360, SHA256: "ff5ce078485f98b1990a56c93843f153c1c4f85928ad1bf70e4eaebd354d22fc"},
	{Dir: "apple", Name: "CPAM 3.5.po", URL: appleOS + "Apple%20CPM/Disk%20Images/CPAM%203.5.po",
		Size: 819200, SHA256: "00cfd502c1e198cf7ebd563a8af264e0899fcdf58ce02ed1f8b5133bfdb26fe2"},
	meatloaf("m.u.l.e.d64", 174848, "29a645d6c9a276928aaa9c424ab336f7b5b7f07d1fa1bc42d81e4949b2e51b0b"),
	meatloaf("lemmings.dfi", 917504, "78822b9482deac1467997828218fd476a6214368f70f521137e55fe92e0007cc"),
	meatloaf("ultima v.d81", 819200, "c4ca219afa763be59b5a3b767b0cc24ba3c60a346a28e93dd2c8b4bda5190f83"),
	meatloaf("1942.tap", 718108, "53321aeef5e259d0e9e29198778b69f57dc48c8c27eb58b2addec55441a1e6e2"),
	meatloaf("choppin_mall.t64", 27504, "59ca4c8e3149c59e5a17646d4debc3b7f057f8b4f77b7860a7e6cdab9ad12270"),
	meatloaf("42 tracks/scan42.d64", 205312, "fbf2ce51ea62e911b64125c5a1ee13ee5d2b929bad688e73ca6635e9c327eaf6"),
	meatloaf("42 tracks/scan42err.d64", 206114, "1c067f390be458a12a2322fb3a78f6933cc2d18ec2ab14b248c99a97fac73c68"),
	meatloaf("bad listing/topsecretstuff.d64", 174848, "7d726aa916ab80d8ea7ae7ed83c1fceba166014e2eb1f5cc0ecb4efa155c9dd5"),
	meatloaf("bam message/053_diskdevil5.d64", 174848, "ea8e9d5c7b40ec83a35f685b6d44c4431b0f61fb0c4035bb2bb199bd26413b11"),
	meatloaf("bam message/06-expert_v41.d64", 174848, "3950efa32a6329aab51ba7a4eb8bb3b79cdde2d95d658ef7b843c8aec0f80a9e"),
	meatloaf("bam message/60k_tool.d64", 174848, "836e380925255e4fede27f2dcedf255f4d9956857a3c4bc604386d6efe3f3584"),
	meatloaf("bam message/sb005157-df624d.d64", 174848, "84ed2601caa119977b0a4eb23a4630812579ca64da3b7e17146ad7786abccd5b"),
	meatloaf("bam message/sb005230-c4fee2.d64", 174848, "fd8c1920015a3fbb6f79a92b02415a1b09a5a4c82032877d738b58d2c0c3e414"),
	meatloaf("error info/0006x.d64", 175531, "5037e7165315c15d2bcd04b84281babc4defaed1ad2e19c12a7879a2a2746161"),
	meatloaf("error info/0027.d64", 175531, "6a2ac905127f74df1fbc0de9cb55c62eee28120a212533d8d9d14f00d29824e0"),
	meatloaf("error info/0152.d64", 175531, "37c595197a0aa9f84d789e34f268ea8d5523534b6ee84b5b72142a19017060ad"),
	meatloaf("error info/0168.d64", 175531, "b373ec6ce47d7479b687648afaa0b522147f8ed0f6c04b7e93c1fbc80c5747e4"),
	meatloaf("error info/0180.d64", 175531, "724239e6b089bd56c54c21f3aa96143c821ca84cb5273ab370d842e492faca17"),
	meatloaf("error info/0206_heroes_of_lance.d64", 175531, "8249e8df56f2c05eff474d1f867a6ddbc848bce76c81948ccc9e78aedf48bfda"),
	meatloaf("error info/0216_sceptre_of_b.d64", 175531, "92c5657d91ebb2270fb591b8c55b613368bb11f487736d6fc3eaa9d8a7f1db78"),
	meatloaf("error info/0220.d64", 175531, "8336ac8626bd87c0b11df92b11290b0fee1d983e4d188417af39960f54c52fe9"),
	meatloaf("error info/0587_cpx_upheav rips.d64", 175531, "fb572cdf7eda6ddbdb12bece2a3c7baeb40c900420036516625f35a18e331452"),
	meatloaf("error info/0905.d64.d81", 822400, "08a425bd58afbe58f4c4e4ae41608092d2b18d6b18a5b6a80bab868d5319701d"),
	meatloaf("error info/0923.d64.d81", 822400, "7ee90ca327835c6bd7c8a038f9537df8b75f1478b9156c728636ec49564e6e85"),
	meatloaf("error info/abacus-ada-b2b7ea.d64", 197376, "59b2a66879bf39b226ec84c43d867fc9d05dafec31ebfb401c8c3fb78c449607"),
	meatloaf("error info/scan35err.d64", 175531, "7e26c40be0d3f14814d9eb3c56eb7239bb4a0915bd306df1565d606496c593de"),
	meatloaf("error info/scan40err.d64", 197376, "d359417cade4ce3daf523b87ce68eb4c9bd5a4c0535f3a8dab3b56cea3346ad1"),
	meatloaf("error info/scan42.d64", 205312, "fbf2ce51ea62e911b64125c5a1ee13ee5d2b929bad688e73ca6635e9c327eaf6"),
	meatloaf("error info/scan42err.d64", 206114, "1c067f390be458a12a2322fb3a78f6933cc2d18ec2ab14b248c99a97fac73c68"),
}

// repoRoot returns the repository's top directory.
func repoRoot() string {
	_, file, _, _ := runtime.Caller(0)
	return filepath.Join(filepath.Dir(file), "..", "..")
}

// Root returns the directory of images committed to the repository.
func Root() string { return filepath.Join(repoRoot(), "images") }

// DownloadRoot returns the directory downloaded images are stored in.
func DownloadRoot() string { return filepath.Join(repoRoot(), "downloads") }

// DownloadPath returns where a downloadable image is stored.
func DownloadPath(img Image) string { return filepath.Join(DownloadRoot(), img.Dir, img.Name) }

// Path returns the path of a test image. An image listed in the Manifest
// is downloads/<dir>/<name>, downloaded first if it is missing; any other
// image is images/<dir>/<name>. The test is skipped if the image is not
// available (the download failed, or a local image is absent).
func Path(t testing.TB, dir, name string) string {
	t.Helper()
	for _, img := range Manifest {
		if img.Dir == dir && img.Name == name {
			if err := Fetch(img); err != nil {
				t.Skipf("test image %s/%s is not available: %v", dir, name, err)
			}
			return DownloadPath(img)
		}
	}
	path := filepath.Join(Root(), dir, name)
	if _, err := os.Stat(path); err != nil {
		t.Skipf("test image images/%s/%s is not available locally and has no download source", dir, name)
	}
	return path
}

// Fetch downloads an image if it does not exist locally, verifying its size
// and SHA-256 before moving it into place.
func Fetch(img Image) error {
	return fetchTo(DownloadPath(img), img)
}

func fetchTo(path string, img Image) error {
	if _, err := os.Stat(path); err == nil {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	client := &http.Client{Timeout: 2 * time.Minute}
	req, err := http.NewRequest(http.MethodGet, img.URL, nil)
	if err != nil {
		return err
	}
	if img.UserAgent != "" {
		req.Header.Set("User-Agent", img.UserAgent)
	}
	resp, err := client.Do(req)
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
