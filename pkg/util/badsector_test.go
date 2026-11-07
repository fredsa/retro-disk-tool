package util

import (
	"bytes"
	"testing"
)

func TestIsBadSector(t *testing.T) {
	bad := bytes.Repeat(BadSectorMarker, 512/len(BadSectorMarker))
	if !IsBadSector(bad) {
		t.Error("IsBadSector(marker sector) = false")
	}
	partial := append([]byte{}, bad...)
	partial[300] = 0
	for name, sec := range map[string][]byte{"partial": partial, "zeros": make([]byte, 512), "short": BadSectorMarker[:8]} {
		if IsBadSector(sec) {
			t.Errorf("IsBadSector(%s) = true", name)
		}
	}
}

func TestFormatRanges(t *testing.T) {
	if got, want := FormatRanges([]int{7, 13, 84, 85, 86, 90}), "7, 13, 84-86, 90"; got != want {
		t.Errorf("FormatRanges = %q, want %q", got, want)
	}
	if got := FormatRanges(nil); got != "" {
		t.Errorf("FormatRanges(nil) = %q", got)
	}
}
