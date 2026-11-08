package util

import (
	"bytes"
	"fmt"
	"strings"
)

// BadSectorMarker is the fill pattern disk imaging tools write in place of
// sectors they could not read.
var BadSectorMarker = []byte("-=[BAD SECTOR]=-")

// BadSectorFill returns n bytes of the repeated bad sector marker, as written
// in place of a sector that could not be read.
func BadSectorFill(n int) []byte {
	m := len(BadSectorMarker)
	return bytes.Repeat(BadSectorMarker, (n+m-1)/m)[:n]
}

// IsBadSector reports whether sec consists of the repeated bad sector marker.
func IsBadSector(sec []byte) bool {
	n := len(BadSectorMarker)
	if len(sec) < n || len(sec)%n != 0 {
		return false
	}
	for i := 0; i < len(sec); i += n {
		if !bytes.Equal(sec[i:i+n], BadSectorMarker) {
			return false
		}
	}
	return true
}

// FormatRanges formats sorted numbers compactly, e.g. "7, 13, 84-85".
func FormatRanges(nums []int) string {
	var parts []string
	for i := 0; i < len(nums); {
		j := i
		for j+1 < len(nums) && nums[j+1] == nums[j]+1 {
			j++
		}
		if j > i {
			parts = append(parts, fmt.Sprintf("%d-%d", nums[i], nums[j]))
		} else {
			parts = append(parts, fmt.Sprintf("%d", nums[i]))
		}
		i = j + 1
	}
	return strings.Join(parts, ", ")
}
