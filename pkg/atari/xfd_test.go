package atari_test

import (
	"bytes"
	"testing"

	"github.com/fredsa/retro-disk-tool/cmd/rdt/pkg/atari"
)

func TestOpenXFDRejects(t *testing.T) {
	for name, data := range map[string][]byte{
		"odd size": make([]byte, 1000),
		"blank":    make([]byte, 720*128),
	} {
		if _, err := atari.OpenXFD(bytes.NewReader(data), int64(len(data))); err == nil {
			t.Errorf("OpenXFD(%s) succeeded, want error", name)
		}
	}
}
