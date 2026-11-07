package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
	"unicode/utf8"

	"github.com/fredsa/retro-disk-tool/cmd/rdt/pkg/disk"
	"github.com/fredsa/retro-disk-tool/cmd/rdt/pkg/terminal"
	"github.com/fredsa/retro-disk-tool/cmd/rdt/pkg/util"
)

const version = "0.1.0"

func printHelp(w io.Writer) {
	fmt.Fprintln(w, "Retro Disk Tool (rdt) - Inspect retro computer archive disk formats.")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Usage:")
	fmt.Fprintln(w, "  rdt -v | --version")
	fmt.Fprintln(w, "  rdt -h | --help")
	fmt.Fprintln(w, "  rdt ls [-l] <file> [<dir>]")
	fmt.Fprintln(w, "  rdt dump <file>")
	fmt.Fprintln(w, "  rdt cat <file> <path>")
	fmt.Fprintln(w, "  rdt terminal")
}

func printLsHelp(w io.Writer) {
	fmt.Fprintln(w, "Usage:")
	fmt.Fprintln(w, "  rdt ls [-l] <file> [<dir>]")
}

func printDumpHelp(w io.Writer) {
	fmt.Fprintln(w, "Usage:")
	fmt.Fprintln(w, "  rdt dump <file>")
}

func printCatHelp(w io.Writer) {
	fmt.Fprintln(w, "Usage:")
	fmt.Fprintln(w, "  rdt cat <file> <path>")
}

func formatWithCommas[T int | int64 | uint | uint32 | uint64](n T) string {
	in := fmt.Sprintf("%d", n)
	var out []byte
	l := len(in)
	for i, c := range in {
		if i > 0 && (l-i)%3 == 0 {
			out = append(out, ',')
		}
		out = append(out, byte(c))
	}
	return string(out)
}

func formatFilename(name string) string {
	if strings.Contains(name, " ") {
		return fmt.Sprintf("%q", name)
	}
	return name
}

// nativeDiffers reports whether an entry's platform specific name rendering
// differs from its plain name (different text or any inverse / flashing).
func nativeDiffers(e disk.Entry) bool {
	if len(e.Native) == 0 {
		return false
	}
	for _, g := range e.Native {
		if g.Style != util.StyleNormal {
			return true
		}
	}
	return util.GlyphsString(e.Native) != e.Name
}

// printFormat prints the platform, image format and basic geometry.
func printFormat(w io.Writer, f *util.FormatInfo) {
	if f == nil {
		return
	}
	fmt.Fprintf(w, "Format:\n")
	fmt.Fprintf(w, "  Platform:   %s\n", f.Platform)
	fmt.Fprintf(w, "  Image:      %s\n", f.Image)
	if f.Filesystem != "" {
		fmt.Fprintf(w, "  Filesystem: %s\n", f.Filesystem)
	}
	if f.Tracks > 0 {
		heads := "heads"
		if f.Heads == 1 {
			heads = "head"
		}
		fmt.Fprintf(w, "  Geometry:   %d tracks, %d %s, %s sectors/track, %d bytes/sector\n",
			f.Tracks, f.Heads, heads, f.Sectors, f.SectorSize)
	}
	fmt.Fprintln(w)
}

func runLs(args []string, stdout, stderr io.Writer) int {
	lsFlags := flag.NewFlagSet("ls", flag.ContinueOnError)
	lsFlags.SetOutput(stderr)
	lsFlags.Usage = func() {
		printLsHelp(stderr)
	}
	longFlag := lsFlags.Bool("l", false, "use a long listing format")
	if err := lsFlags.Parse(args); err != nil {
		return 1
	}
	if lsFlags.NArg() < 1 || lsFlags.NArg() > 2 {
		printLsHelp(stderr)
		return 1
	}

	diskPath := lsFlags.Arg(0)
	f, err := os.Open(diskPath)
	if err != nil {
		fmt.Fprintf(stderr, "Error opening disk image %q: %v\n", diskPath, err)
		return 1
	}
	defer f.Close()

	fi, err := f.Stat()
	if err != nil {
		fmt.Fprintf(stderr, "Error getting file info for %q: %v\n", diskPath, err)
		return 1
	}

	reader, err := disk.Open(diskPath, f, fi.Size())
	if err != nil {
		fmt.Fprintf(stderr, "Error reading disk image %q: %v\n", diskPath, err)
		return 1
	}

	dirPath := strings.Trim(lsFlags.Arg(1), `/\`)
	var entries []disk.Entry
	if dirPath == "" {
		entries, err = reader.List()
	} else if dl, ok := reader.(disk.DirLister); ok {
		entries, err = dl.ListDir(dirPath)
	} else {
		err = fmt.Errorf("subdirectories are not supported for this disk format")
	}
	if err != nil {
		fmt.Fprintf(stderr, "Error listing files in %q: %v\n", diskPath, err)
		return 1
	}

	printFormat(stdout, reader.Format())

	header := reader.Header()
	if header != nil && (header.Name != "" || header.ID != "" || header.DOSType != "") {
		fmt.Fprintf(stdout, "Label:\n")
		nameStr := formatFilename(header.Name)
		if header.ID != "" || header.DOSType != "" {
			if header.Name == "" {
				nameStr = `""`
			}
			fmt.Fprintf(stdout, "  %s %s,%s\n", nameStr, header.ID, header.DOSType)
		} else if header.Name != "" {
			fmt.Fprintf(stdout, "  %s\n", nameStr)
		}
		fmt.Fprintln(stdout)
	}

	var corruptWarnings []string
	seenWarnings := make(map[string]bool)

	if dirPath == "" {
		fmt.Fprintf(stdout, "Contents:\n")
	} else {
		fmt.Fprintf(stdout, "Contents of %s/:\n", formatFilename(dirPath))
	}

	// Show an extra "Native" column when any entry has a platform specific
	// rendering of its name (e.g. Apple II inverse / flashing characters).
	displayNames := make([]string, len(entries))
	nameWidth := 0
	hasNative := false
	for i, entry := range entries {
		n := entry.Name
		if entry.IsDir {
			n += "/"
		}
		displayNames[i] = formatFilename(n)
		if w := utf8.RuneCountInString(displayNames[i]); w > nameWidth {
			nameWidth = w
		}
		if nativeDiffers(entry) {
			hasNative = true
		}
	}
	useANSI := false
	if *longFlag && hasNative {
		useANSI = terminal.Detect(stdout).ANSI
	}
	if *longFlag {
		if hasNative {
			if nameWidth < len("Name") {
				nameWidth = len("Name")
			}
			fmt.Fprintf(stdout, "  %-4s %8s %-19s %-*s %s\n", "Type", "Size", "Modified", nameWidth, "Name", "Native")
		} else {
			fmt.Fprintf(stdout, "  %-4s %8s %-19s %s\n", "Type", "Size", "Modified", "Name")
		}
	}

	for i, entry := range entries {
		if entry.Corrupt && entry.Error != "" {
			// Record unique directory sector errors for the summary warning
			for _, part := range strings.Split(entry.Error, "; ") {
				if strings.HasPrefix(part, "track ") && !seenWarnings[part] {
					seenWarnings[part] = true
					corruptWarnings = append(corruptWarnings, part)
				}
			}
		}
		if *longFlag {
			typeIndicator := "-"
			if entry.Corrupt {
				typeIndicator = "?"
			} else if entry.IsDir {
				typeIndicator = "d"
			}
			displayName := displayNames[i]
			timeStr := "                   "
			if entry.ModTime != nil {
				timeStr = entry.ModTime.Format("2006-01-02 15:04:05")
			}
			if hasNative {
				pad := strings.Repeat(" ", nameWidth-utf8.RuneCountInString(displayName))
				native := terminal.Render(entry.Native, useANSI)
				if len(entry.Native) == 0 {
					native = entry.Name
				}
				fmt.Fprintf(stdout, "  %-4s %8d %s %s%s %s\n", typeIndicator, entry.Size, timeStr, displayName, pad, native)
			} else {
				fmt.Fprintf(stdout, "  %-4s %8d %s %s\n", typeIndicator, entry.Size, timeStr, displayName)
			}
		} else {
			fmt.Fprintf(stdout, "  %s\n", formatFilename(entry.Name))
		}
	}

	hasCorrupt := false
	for _, entry := range entries {
		if entry.Corrupt {
			hasCorrupt = true
			break
		}
	}
	if *longFlag && hasCorrupt {
		fmt.Fprintln(stdout)
		if len(corruptWarnings) > 0 {
			fmt.Fprintf(stdout, "Warning: broken directory on disk (%s)\n", strings.Join(corruptWarnings, "; "))
		} else {
			fmt.Fprintf(stdout, "Warning: broken directory on disk (corrupted directory entries)\n")
		}
	}

	usage, err := reader.Usage()
	if err != nil {
		fmt.Fprintf(stderr, "Warning: unable to calculate disk usage: %v\n", err)
		return 0
	}

	if usage != nil && usage.HasUsage {
		inUsePct := (float64(usage.InUseSectors) / float64(usage.TotalSectors)) * 100.0
		freePct := (float64(usage.FreeSectors) / float64(usage.TotalSectors)) * 100.0

		descWidth := 7
		if len(usage.FormattedDesc) > descWidth {
			descWidth = len(usage.FormattedDesc)
		}
		if len(usage.UnformattedDesc) > descWidth {
			descWidth = len(usage.UnformattedDesc)
		}
		fmtStr := fmt.Sprintf("  %%-12s %%%ds (%%s bytes)\n", descWidth)

		fmt.Fprintln(stdout)
		fmt.Fprintf(stdout, "Capacity:\n")
		fmt.Fprintf(stdout, fmtStr, "Formatted:", usage.FormattedDesc, formatWithCommas(usage.FormattedCapacity))
		if usage.UnformattedDesc != "" {
			fmt.Fprintf(stdout, fmtStr, "Unformatted:", usage.UnformattedDesc, formatWithCommas(usage.UnformattedCapacity))
		}
		fmt.Fprintln(stdout)
		fmt.Fprintf(stdout, "Usage:\n")
		if usage.BAMSectors > 0 {
			bamPct := (float64(usage.BAMSectors) / float64(usage.TotalSectors)) * 100.0
			fmt.Fprintf(stdout, "  BAM:    %5.1f%% %6s sectors (%s bytes)\n",
				bamPct, formatWithCommas(usage.BAMSectors), formatWithCommas(usage.BAMBytes))
		}
		fmt.Fprintf(stdout, "  In-use: %5.1f%% %6s sectors (%s bytes)\n",
			inUsePct, formatWithCommas(usage.InUseSectors), formatWithCommas(usage.InUseBytes))
		fmt.Fprintf(stdout, "  Free:   %5.1f%% %6s sectors (%s bytes)\n",
			freePct, formatWithCommas(usage.FreeSectors), formatWithCommas(usage.FreeBytes))
		fmt.Fprintf(stdout, "  Total:  %5.1f%% %6s sectors (%s bytes)\n",
			100.0, formatWithCommas(usage.TotalSectors), formatWithCommas(usage.TotalBytes))
	}

	return 0
}

func runDump(args []string, stdout, stderr io.Writer) int {
	dumpFlags := flag.NewFlagSet("dump", flag.ContinueOnError)
	dumpFlags.SetOutput(stderr)
	dumpFlags.Usage = func() {
		printDumpHelp(stderr)
	}
	if err := dumpFlags.Parse(args); err != nil {
		return 1
	}
	if dumpFlags.NArg() != 1 {
		printDumpHelp(stderr)
		return 1
	}

	diskPath := dumpFlags.Arg(0)
	f, err := os.Open(diskPath)
	if err != nil {
		fmt.Fprintf(stderr, "Error opening file %q: %v\n", diskPath, err)
		return 1
	}
	defer f.Close()

	fi, err := f.Stat()
	if err != nil {
		fmt.Fprintf(stderr, "Error getting file info for %q: %v\n", diskPath, err)
		return 1
	}

	reader, err := disk.Open(diskPath, f, fi.Size())
	if err != nil {
		fmt.Fprintf(stderr, "Error inspecting %q: %v\n", diskPath, err)
		return 1
	}

	if err := reader.Dump(stdout); err != nil {
		fmt.Fprintf(stderr, "Error dumping %q: %v\n", diskPath, err)
		return 1
	}

	return 0
}

func runCat(args []string, stdout, stderr io.Writer) int {
	catFlags := flag.NewFlagSet("cat", flag.ContinueOnError)
	catFlags.SetOutput(stderr)
	catFlags.Usage = func() {
		printCatHelp(stderr)
	}
	if err := catFlags.Parse(args); err != nil {
		return 1
	}
	if catFlags.NArg() != 2 {
		printCatHelp(stderr)
		return 1
	}

	diskPath := catFlags.Arg(0)
	filePath := catFlags.Arg(1)

	f, err := os.Open(diskPath)
	if err != nil {
		fmt.Fprintf(stderr, "Error opening file %q: %v\n", diskPath, err)
		return 1
	}
	defer f.Close()

	fi, err := f.Stat()
	if err != nil {
		fmt.Fprintf(stderr, "Error getting file info for %q: %v\n", diskPath, err)
		return 1
	}

	reader, err := disk.Open(diskPath, f, fi.Size())
	if err != nil {
		fmt.Fprintf(stderr, "Error opening %q: %v\n", diskPath, err)
		return 1
	}

	content, err := reader.ReadFile(filePath)
	if err != nil {
		fmt.Fprintf(stderr, "Error reading %q in %q: %v\n", filePath, diskPath, err)
		return 1
	}

	if _, err := stdout.Write(content); err != nil {
		fmt.Fprintf(stderr, "Error writing content to stdout: %v\n", err)
		return 1
	}

	return 0
}

func run(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		printHelp(stderr)
		return 1
	}

	switch args[0] {
	case "-v", "--version":
		fmt.Fprintf(stdout, "rdt version %s\n", version)
		return 0
	case "-h", "--help":
		printHelp(stdout)
		return 0
	case "ls":
		return runLs(args[1:], stdout, stderr)
	case "dump":
		return runDump(args[1:], stdout, stderr)
	case "cat":
		return runCat(args[1:], stdout, stderr)
	case "terminal":
		terminal.Print(stdout, terminal.Detect(stdout))
		return 0
	default:
		fmt.Fprintf(stderr, "Unknown option or command: %s\n\n", args[0])
		printHelp(stderr)
		return 1
	}
}

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}
