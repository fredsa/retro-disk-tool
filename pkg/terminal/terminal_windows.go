//go:build windows

package terminal

import (
	"os"
	"syscall"
	"unsafe"
)

const enableVirtualTerminalProcessing = 0x0004

var (
	kernel32       = syscall.NewLazyDLL("kernel32.dll")
	getConsoleMode = kernel32.NewProc("GetConsoleMode")
	setConsoleMode = kernel32.NewProc("SetConsoleMode")
)

// consoleVT reports whether f is a Windows console and whether VT processing
// is on, enabling it if necessary.
func consoleVT(f *os.File) (isConsole, enabled, enabledNow bool) {
	h := f.Fd()
	var mode uint32
	r, _, _ := getConsoleMode.Call(h, uintptr(unsafe.Pointer(&mode)))
	if r == 0 {
		return false, false, false
	}
	if mode&enableVirtualTerminalProcessing != 0 {
		return true, true, false
	}
	r, _, _ = setConsoleMode.Call(h, uintptr(mode|enableVirtualTerminalProcessing))
	if r != 0 {
		return true, true, true
	}
	return true, false, false
}
