package trs80

import (
	"bytes"
	"testing"
)

func TestOpenRejects(t *testing.T) {
	for name, data := range map[string][]byte{
		"odd size": make([]byte, 1000),
		"blank":    make([]byte, 40*sectorsPerTrack*sectorSize),
	} {
		if _, err := Open(bytes.NewReader(data), int64(len(data))); err == nil {
			t.Errorf("Open(%s) succeeded, want error", name)
		}
	}
}
