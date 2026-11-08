# Retro Disk Tool (`rdt`)

Inspect retro computer archive disk formats.

# Supported formats

System   | Extension | Capacity | Medium  | Cyclinder | Heads | Sectors | Bytes per sector
---------|-----------|----------|---------|-----------|-------|---------|----------------
Amiga    | `ADF`     | 880 KiB  | DD 3.5" | 80        | 2     | 11      | 512
C64      | `D64`     | 170 KiB  | SS 5.25"| 35        | 1     | 17-21   | 256
C64      | `D64` (40 / 42 tracks) | 192 / 200 KiB | SS 5.25" | 40 / 42 | 1 | 17-21 | 256
C64      | `D81`     | 800 KiB  | DS 3.5" | 80        | 2     | 20      | 256
C64      | `G64`     | 170 KiB  | SS 5.25"| 35-42     | 1     | 17-21   | 256 (GCR)
C64      | `DFI`     | N/A      | File    | N/A       | N/A   | 256     | 256
C64      | `T64`     | N/A      | Tape    | N/A       | N/A   | N/A     | N/A
C64      | `TAP`     | N/A      | Tape    | N/A       | N/A   | N/A     | N/A
C64      | `PRG`     | N/A      | File    | N/A       | N/A   | N/A     | N/A
C64      | `CRT`     | N/A      | Cart    | N/A       | N/A   | N/A     | N/A
C64      | `TCRT`    | N/A      | Cart    | N/A       | N/A   | N/A     | N/A
Apple II | `D13`     | 113 KiB  | SS 5.25"| 35        | 1     | 13      | 256
Apple II | `DO`, `DSK` | 140 KiB | SS 5.25"| 35        | 1     | 16      | 256
Apple II | `PO`      | 140 KiB  | SS 5.25"| 35        | 1     | 16      | 256
Apple II | `NIB`     | 140 KiB  | SS 5.25"| 35        | 1     | 13 / 16 | 256
Apple II | `WOZ`     | 140 KiB  | SS 5.25"| 35        | 1     | 13 / 16 | 256
Atari 8-bit | `ATR` | 90 KiB   | SS 5.25"| 40        | 1     | 18      | 128
Atari 8-bit | `XFD`, `IMG` | 90 KiB | SS 5.25"| 40     | 1     | 18      | 128
ZX81     | `P`, `P81` | N/A    | File    | N/A       | N/A   | N/A     | N/A
TRS-80 Model III | `IMG` | 180 KiB | SS 5.25"| 40   | 1     | 18      | 256
TRS-80 Model 4 | `IMD` | 180 KiB | SS 5.25"| 40     | 1     | 18      | 256
IBM PC   | `IMG`, `IMA`, `VFD` | 160 KiB - 2.88 MiB | 5.25" / 3.5" | 40 / 80 | 1 / 2 | 8 - 36 | 512

Apple II images may hold Apple DOS 3.x (13 or 16 sector) or ProDOS volumes. `NIB` and `WOZ` (v1 and v2)
images are decoded from their GCR nibble / bitstream data (5-and-3 and 6-and-2 encoding).

C64 disk images (`D64`, `D81`, decoded `G64`, `DFI`) share one CBM DOS reader:
- `D64` images may have 35, 40 or 42 tracks and an error table. For 40 track images, the tracks 36-40 BAM extension
  (SpeedDOS, DolphinDOS or other locations) is chosen by checking which one agrees with the sectors the files use;
  tracks no BAM covers are reported as `Extra`.
- `D81` partitions formatted as sub-directories are listed as directories (`ls -r`, `cat DIR/NAME`).
- `G64` GCR bitstreams are decoded sector by sector (headers, data blocks, checksums); sectors that can't be decoded
  (e.g. copy protection) are reported.
- `DFI` is the DreamLoad file archive format.
- Files whose block chain is shorter than their directory entry, or leaves their partition, are marked as corrupt.

`T64` tape archives, `TAP` tape images (files saved with the standard ROM loader; turbo loader data is reported but
not decoded) and `TCRT` tapecart images (the TapcrtFileSys directory, or the fastload program) list their files;
`cat` returns programs with their 2 byte load address, like a `PRG` file.

ZX81 `P` files are memory images saved from the system variables at $4009; `P81` files add the program name and may
hold several programs. `cat` returns a program's `P` data, and `dump` shows its system variables and BASIC listing.

TRS-80 images may hold TRSDOS 1.3 (Model III) or LDOS family (LDOS 5, TRSDOS 6, LS-DOS 6) volumes; files are named
`NAME/EXT` (`cat` also accepts `NAME.EXT`).

`IMD` (ImageDisk) images are decoded into sectors (compressed, deleted data, data error and unavailable sectors are
handled) and then checked for a TRS-80 or IBM PC filesystem. Every track must have the same layout. `ls` shows the
comment stored in the image.

Atari images may hold Atari DOS 2.x (2.0S, 2.5, 2.0D) or Atari DOS 3 volumes. Atari raw (headerless) images are
recognized by size. Raw dumps of disks written by Atari drives, which store data
with all bits inverted, are detected and corrected automatically.

`IMG` files are checked for an IBM PC FAT volume, then TRS-80 TRSDOS 1.3, then Atari DOS 2.x.

Every byte of a file must be accounted for: a file whose size does not exactly match its format (for example
trailing bytes after a program or cartridge) is rejected rather than partially interpreted.

For IBM PC images, `ls` warns about sectors the imaging tool (e.g. Greaseweazle) could not read, which are filled
with `-=[BAD SECTOR]=-`; files containing unreadable sectors are marked as corrupt. A standard floppy image may hold
a smaller filesystem (e.g. a 360 KiB format on a 720 KiB disk) only if every sector after the filesystem is
unreadable; those sectors are reported as `Extra` in the usage, so the total always matches the file size.

IBM PC images may hold FAT12, FAT16 or FAT32 volumes, including long file names, DOS 1.x floppies without a
BPB, and MBR partitioned hard disk images. `ls` and `cat` accept paths such as `DOS` and `DOS/README.TXT`, and
`dump` shows the full directory tree.


# Usage

```
rdt -v | --version
rdt -h | --help

rdt ls -l workbench.adf
rdt ls -l dos.img DOS
rdt ls -l -r dos.img
rdt workbench.adf
rdt dump workbench.adf
rdt cat workbench.adf Clock
rdt terminal
```

`ls` prints the disk image format (`Format:`), the disk label (`Label:`), and the file listing (`Contents:`).
Running `rdt` with just a file (and optionally a directory) is the same as `rdt ls -l -r`.
An optional directory argument lists a subdirectory instead of the root, on formats with subdirectories (Amiga,
IBM PC, C64 D81 / DFI),
and `-r` lists subdirectories recursively, naming each entry by its path.
With `-l` the listing has `Type`, `Size`, `Modified` and `Name` columns. For Apple II (DOS 3.x) and C64 disks, an
extra `Native` column shows the file name as the original computer displayed it, using Unicode and ANSI styling:

- Apple II: inverse and flashing characters (flashing is shown as inverse yellow, since many terminals ignore blink)
- C64: names in quote mode, with PETSCII graphics as Unicode and control codes as reverse video symbols

`terminal` prints what `rdt` can detect about your terminal (platform, emulator, ANSI support) and shows
sample normal, inverse and flashing text, which is useful for checking how `Native` names will look.

# Testing

```
go test ./...
```

Test disk images committed to the repository live in `images/`. Freely downloadable images (listed in
`downloads/README.md`) are not part of the repository: tests download them into `downloads/` the first time they need
them, only if missing, and verify them by SHA-256; `go run ./internal/testimages/fetch` downloads them all ahead of
time.

# Installation

1. Download and install Go: https://go.dev/doc/install

2. Clone the repository

```
git clone https://git.com/fredsa/retro-disk-tool
```

3. Build the project

```
cd retro-disk-tool
go build
```

4. Install the tool

```
go install
```

5. Verify installation

```
rdt -v
```

