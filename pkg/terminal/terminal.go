package terminal

import (
	"fmt"
	"io"
	"os"
	"runtime"
	"strings"

	"github.com/fredsa/retro-disk-tool/cmd/rdt/pkg/util"
)

const (
	Normal  = "\x1b[0m"
	Inverse = "\x1b[7m"
	// Flashing is rendered as inverse yellow because the blink (\x1b[5m) and
	// underline (\x1b[4m) attributes are ignored by Windows cmd, PowerShell
	// and Git Bash. Bright yellow (93) is used because Windows PowerShell's
	// default scheme remaps plain yellow (33) to its default text color.
	Flashing = "\x1b[7;93m"
	// Yellow is bright yellow, for the same reason.
	Yellow = "\x1b[93m"
)

// WarningLabel returns "Warning:", in yellow when ansi is true.
func WarningLabel(ansi bool) string {
	if ansi {
		return Yellow + "Warning:" + Normal
	}
	return "Warning:"
}

// Info describes what can be determined about the terminal attached to a writer.
type Info struct {
	GOOS, GOARCH string
	IsFile       bool // writer is an *os.File
	IsCharDevice bool // file is a character device (typical on Unix ttys)
	IsConsole    bool // Windows console handle
	VTEnabled    bool // Windows: VT processing enabled (possibly by us)
	VTEnabledNow bool // Windows: we had to enable VT processing ourselves
	Env          [][2]string
	Emulator     string // best guess at the terminal emulator
	ANSI         bool   // verdict: ANSI escape sequences are safe to emit
	Reason       string // why ANSI verdict was reached
}

var envVars = []string{
	"TERM", "COLORTERM", "TERM_PROGRAM", "TERM_PROGRAM_VERSION", "WT_SESSION",
	"MSYSTEM", "SHELL", "ComSpec", "PSModulePath", "NO_COLOR", "COLUMNS", "LINES",
}

// Detect gathers terminal information for w. On Windows it also tries to
// enable virtual terminal processing on the console.
func Detect(w io.Writer) *Info {
	info := &Info{GOOS: runtime.GOOS, GOARCH: runtime.GOARCH}
	for _, k := range envVars {
		v, ok := os.LookupEnv(k)
		if !ok {
			continue
		}
		if k == "PSModulePath" {
			v = "(set)"
		}
		info.Env = append(info.Env, [2]string{k, v})
	}

	f, ok := w.(*os.File)
	if ok {
		info.IsFile = true
		if fi, err := f.Stat(); err == nil {
			info.IsCharDevice = fi.Mode()&os.ModeCharDevice != 0
		}
		info.IsConsole, info.VTEnabled, info.VTEnabledNow = consoleVT(f)
	}

	term := os.Getenv("TERM")
	switch {
	case os.Getenv("WT_SESSION") != "":
		info.Emulator = "Windows Terminal"
	case os.Getenv("MSYSTEM") != "":
		info.Emulator = "Git Bash / MSYS2 (mintty)"
	case os.Getenv("TERM_PROGRAM") != "":
		info.Emulator = os.Getenv("TERM_PROGRAM")
	case info.IsConsole:
		info.Emulator = "Windows console (conhost)"
	case term != "":
		info.Emulator = term
	default:
		info.Emulator = "unknown"
	}

	termLooksOK := term != "" && term != "dumb"
	switch {
	case !info.IsFile:
		info.Reason = "output is not a file/terminal"
	case os.Getenv("NO_COLOR") != "":
		info.Reason = "NO_COLOR is set"
	case info.IsConsole && info.VTEnabled:
		info.ANSI, info.Reason = true, "Windows console with VT processing"
	case info.IsConsole:
		info.Reason = "Windows console without VT processing"
	case info.IsCharDevice && termLooksOK:
		info.ANSI, info.Reason = true, "tty with TERM="+term
	case os.Getenv("MSYSTEM") != "" && termLooksOK:
		info.ANSI, info.Reason = true, "MSYS/mintty pty with TERM="+term
	case term == "dumb":
		info.Reason = "TERM=dumb"
	default:
		info.Reason = "not a terminal (redirected or piped)"
	}
	return info
}

// Render converts glyphs to a string. When ansi is true, inverse and flashing
// glyphs are wrapped in the corresponding escape sequences.
func Render(g []util.Glyph, ansi bool) string {
	var sb strings.Builder
	cur := util.StyleNormal
	for _, x := range g {
		if ansi && x.Style != cur {
			sb.WriteString(Normal)
			switch x.Style {
			case util.StyleInverse:
				sb.WriteString(Inverse)
			case util.StyleFlashing:
				sb.WriteString(Flashing)
			}
			cur = x.Style
		}
		sb.WriteRune(x.R)
	}
	if ansi && cur != util.StyleNormal {
		sb.WriteString(Normal)
	}
	return sb.String()
}

// Print writes the report and a brief normal/inverse/flashing sample to w.
func Print(w io.Writer, info *Info) {
	fmt.Fprintf(w, "Terminal:\n")
	fmt.Fprintf(w, "  Platform:         %s/%s\n", info.GOOS, info.GOARCH)
	fmt.Fprintf(w, "  Emulator:         %s\n", info.Emulator)
	fmt.Fprintf(w, "  Output is file:   %v\n", info.IsFile)
	fmt.Fprintf(w, "  Character device: %v\n", info.IsCharDevice)
	if info.GOOS == "windows" {
		fmt.Fprintf(w, "  Windows console:  %v\n", info.IsConsole)
		if info.IsConsole {
			fmt.Fprintf(w, "  VT processing:    %v%s\n", info.VTEnabled, map[bool]string{true: " (enabled by rdt)"}[info.VTEnabledNow])
		}
	}
	fmt.Fprintf(w, "  ANSI supported:   %v (%s)\n", info.ANSI, info.Reason)

	fmt.Fprintf(w, "\nEnvironment:\n")
	if len(info.Env) == 0 {
		fmt.Fprintf(w, "  (none of the relevant variables are set)\n")
	}
	for _, kv := range info.Env {
		fmt.Fprintf(w, "  %-22s %s\n", kv[0]+":", kv[1])
	}

	fmt.Fprintf(w, "\nText samples:\n")
	if info.ANSI {
		fmt.Fprintf(w, "  Normal:   %s\n", "HELLO world")
		fmt.Fprintf(w, "  Inverse:  %sHELLO world%s\n", Inverse, Normal)
		fmt.Fprintf(w, "  Flashing: %sHELLO world%s  (inverse yellow; blink is unsupported in many terminals)\n", Flashing, Normal)
		fmt.Fprintf(w, "  Yellow:   %s something to check\n", WarningLabel(true))
	} else {
		fmt.Fprintf(w, "  Normal:   HELLO world\n")
		fmt.Fprintf(w, "  Inverse:  skipped (ANSI not available)\n")
		fmt.Fprintf(w, "  Flashing: skipped (ANSI not available)\n")
		fmt.Fprintf(w, "  Yellow:   skipped (ANSI not available)\n")
	}
}
