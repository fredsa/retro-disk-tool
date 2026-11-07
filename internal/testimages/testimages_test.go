package testimages

import (
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestFetch(t *testing.T) {
	content := []byte("disk image bytes")
	sum := sha256.Sum256(content)
	requests, agent := 0, ""
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		agent = r.UserAgent()
		if r.URL.Path == "/missing" {
			http.NotFound(w, r)
			return
		}
		w.Write(content)
	}))
	defer srv.Close()

	img := Image{URL: srv.URL + "/disk", Size: int64(len(content)), SHA256: hex.EncodeToString(sum[:]), UserAgent: "MEATLOAF/"}
	path := filepath.Join(t.TempDir(), "sub", "disk.dsk")
	if err := fetchTo(path, img); err != nil {
		t.Fatalf("fetchTo failed: %v", err)
	}
	got, err := os.ReadFile(path)
	if err != nil || string(got) != string(content) {
		t.Fatalf("fetched %q, %v", got, err)
	}
	if agent != "MEATLOAF/" {
		t.Errorf("User-Agent = %q, want MEATLOAF/", agent)
	}
	if fi, _ := os.Stat(path); fi.Mode().Perm() != 0o644 {
		t.Errorf("mode = %v, want 0644", fi.Mode().Perm())
	}

	// An existing file is not downloaded again.
	if err := fetchTo(path, img); err != nil || requests != 1 {
		t.Errorf("second fetchTo: err %v, %d requests; want 1", err, requests)
	}

	for name, bad := range map[string]Image{
		"wrong size": {URL: img.URL, Size: img.Size + 1, SHA256: img.SHA256},
		"wrong hash": {URL: img.URL, Size: img.Size, SHA256: strings.Repeat("0", 64)},
		"not found":  {URL: srv.URL + "/missing", Size: img.Size, SHA256: img.SHA256},
	} {
		p := filepath.Join(t.TempDir(), "x")
		if err := fetchTo(p, bad); err == nil {
			t.Errorf("%s: fetchTo succeeded", name)
		}
		if _, err := os.Stat(p); err == nil {
			t.Errorf("%s: file was created", name)
		}
		if leftovers, _ := filepath.Glob(filepath.Join(filepath.Dir(p), ".download-*")); len(leftovers) != 0 {
			t.Errorf("%s: temporary files left: %v", name, leftovers)
		}
	}
}

func TestManifest(t *testing.T) {
	seen := map[string]bool{}
	for _, img := range Manifest {
		key := img.Dir + "/" + img.Name
		if seen[key] || img.Size <= 0 || len(img.SHA256) != 64 || !strings.HasPrefix(img.URL, "https://") {
			t.Errorf("bad manifest entry %+v", img)
		}
		seen[key] = true
	}
}

func TestMeatloafURL(t *testing.T) {
	img := meatloaf("error info/0587_cpx_upheav rips.d64", 1, strings.Repeat("0", 64))
	if want := "https://c64.meatloaf.cc/test/error%20info/0587_cpx_upheav%20rips.d64"; img.URL != want || img.UserAgent != "MEATLOAF/" {
		t.Errorf("URL %q, User-Agent %q; want %q, MEATLOAF/", img.URL, img.UserAgent, want)
	}
}
