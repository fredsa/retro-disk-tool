// Command fetch downloads all test images that are missing into downloads/.
//
//	go run ./internal/testimages/fetch
package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/fredsa/retro-disk-tool/cmd/rdt/internal/testimages"
)

func main() {
	failed := false
	for _, img := range testimages.Manifest {
		path := filepath.Join("downloads", img.Dir, img.Name)
		if err := testimages.Fetch(img); err != nil {
			fmt.Fprintf(os.Stderr, "%s: %v\n", path, err)
			failed = true
			continue
		}
		fmt.Println(path)
	}
	if failed {
		os.Exit(1)
	}
}
