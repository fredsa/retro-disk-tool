package main

import (
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/fredsa/retro-disk-tool/cmd/rdt/pkg/adf"
)

const version = "0.1.0"

func printHelp(w io.Writer) {
	fmt.Fprintln(w, "Retro Disk Tool (rdt) - Inspect retro computer archive disk formats.")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Usage:")
	fmt.Fprintln(w, "  rdt -v | --version")
	fmt.Fprintln(w, "  rdt -h | --help")
	fmt.Fprintln(w, "  rdt ls [-l] <file>")
}

func printLsHelp(w io.Writer) {
	fmt.Fprintln(w, "Usage:")
	fmt.Fprintln(w, "  rdt ls [-l] <file>")
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

	fs, err := adf.Open(f, fi.Size())
	if err != nil {
		fmt.Fprintf(stderr, "Error reading disk image %q: %v\n", diskPath, err)
		return 1
	}

	entries, err := fs.ListRoot()
	if err != nil {
		fmt.Fprintf(stderr, "Error listing files in %q: %v\n", diskPath, err)
		return 1
	}

	for _, entry := range entries {
		if *longFlag {
			typeIndicator := "-"
			displayName := entry.Name
			if entry.IsDir {
				typeIndicator = "d"
				displayName += "/"
			}
			timeStr := entry.ModTime.Format("2006-01-02 15:04:05")
			fmt.Fprintf(stdout, "%s %8d %s %s\n", typeIndicator, entry.Size, timeStr, displayName)
		} else {
			fmt.Fprintln(stdout, entry.Name)
		}
	}

	usage, err := fs.DiskUsage()
	if err != nil {
		fmt.Fprintf(stderr, "Warning: unable to calculate disk usage: %v\n", err)
		return 0
	}

	inUsePct := (float64(usage.InUseSectors) / float64(usage.TotalSectors)) * 100.0
	freePct := (float64(usage.FreeSectors) / float64(usage.TotalSectors)) * 100.0

	fmt.Fprintln(stdout)
	fmt.Fprintf(stdout, "Capacity:\n")
	fmt.Fprintf(stdout, "  Formatted:   %3d KiB (%s bytes)\n", 880, formatWithCommas(usage.FormattedCapacity))
	fmt.Fprintf(stdout, "  Unformatted: %3d MB  (%s bytes)\n", 1, formatWithCommas(usage.UnformattedCapacity))
	fmt.Fprintln(stdout)
	fmt.Fprintf(stdout, "Usage:\n")
	fmt.Fprintf(stdout, "  In-use: %5.1f%% %6s sectors (%s bytes)\n",
		inUsePct, formatWithCommas(usage.InUseSectors), formatWithCommas(usage.InUseBytes))
	fmt.Fprintf(stdout, "  Free:   %5.1f%% %6s sectors (%s bytes)\n",
		freePct, formatWithCommas(usage.FreeSectors), formatWithCommas(usage.FreeBytes))
	fmt.Fprintf(stdout, "  Total:  %5.1f%% %6s sectors (%s bytes)\n",
		100.0, formatWithCommas(usage.TotalSectors), formatWithCommas(usage.TotalBytes))

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
	default:
		fmt.Fprintf(stderr, "Unknown option or command: %s\n\n", args[0])
		printHelp(stderr)
		return 1
	}
}

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

