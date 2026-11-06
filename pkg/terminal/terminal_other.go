//go:build !windows

package terminal

import "os"

func consoleVT(f *os.File) (isConsole, enabled, enabledNow bool) {
	return false, false, false
}
