package terminal

import (
	"bytes"
	"testing"
)

func TestWarningLabel(t *testing.T) {
	if got := WarningLabel(false); got != "Warning:" {
		t.Errorf("WarningLabel(false) = %q", got)
	}
	if got := WarningLabel(true); got != "\x1b[93mWarning:\x1b[0m" {
		t.Errorf("WarningLabel(true) = %q", got)
	}
	// A buffer is not a terminal, so no colors.
	if Detect(&bytes.Buffer{}).ANSI {
		t.Error("Detect(buffer).ANSI = true")
	}
}
