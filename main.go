package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/fredsa/retro-disk-tool/cmd/rdt/pkg/disk"
)

const version = "0.1.0"

func printHelp(w io.Writer) {
	fmt.Fprintln(w, "Retro Disk Tool (rdt) - Inspect retro computer archive disk formats.")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Usage:")
	fmt.Fprintln(w, "  rdt -v | --version")
	fmt.Fprintln(w, "  rdt -h | --help")
	fmt.Fprintln(w, "  rdt ls [-l] <file>")
	fmt.Fprintln(w, "  rdt dump <file>")
	fmt.Fprintln(w, "  rdt cat <file> <path>")
}

func printLsHelp(w io.Writer) {
	fmt.Fprintln(w, "Usage:")
	fmt.Fprintln(w, "  rdt ls [-l] <file>")
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
	if lsFlags.NArg() != 1 {
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

	entries, err := reader.List()
	if err != nil {
		fmt.Fprintf(stderr, "Error listing files in %q: %v\n", diskPath, err)
		return 1
	}

	header := reader.Header()
	if header != nil {
		nameStr := formatFilename(header.Name)
		if header.ID != "" || header.DOSType != "" {
			if header.Name == "" {
				nameStr = `""`
			}
			fmt.Fprintf(stdout, "%s %s,%s\n", nameStr, header.ID, header.DOSType)
		} else if header.Name != "" {
			fmt.Fprintln(stdout, nameStr)
		}
		fmt.Fprintln(stdout)
	}

	for _, entry := range entries {
		if *longFlag {
			typeIndicator := "-"
			displayName := entry.Name
			if entry.IsDir {
				typeIndicator = "d"
				displayName += "/"
			}
			displayName = formatFilename(displayName)
			timeStr := "                   "
			if entry.ModTime != nil {
				timeStr = entry.ModTime.Format("2006-01-02 15:04:05")
			}
			fmt.Fprintf(stdout, "%s %8d %s %s\n", typeIndicator, entry.Size, timeStr, displayName)
		} else {
			fmt.Fprintln(stdout, formatFilename(entry.Name))
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

		fmt.Fprintln(stdout)
		fmt.Fprintf(stdout, "Capacity:\n")
		fmt.Fprintf(stdout, "  Formatted:   %7s (%s bytes)\n", usage.FormattedDesc, formatWithCommas(usage.FormattedCapacity))
		fmt.Fprintf(stdout, "  Unformatted: %7s (%s bytes)\n", usage.UnformattedDesc, formatWithCommas(usage.UnformattedCapacity))
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
	default:
		fmt.Fprintf(stderr, "Unknown option or command: %s\n\n", args[0])
		printHelp(stderr)
		return 1
	}
}

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

