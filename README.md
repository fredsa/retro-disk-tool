# Retro Disk Tool (`rdt`)

Inspect retro computer archive disk formats.

# Supported formats

System   | Extension | Capacity | Medium  | Cyclinder | Heads | Sectors | Bytes per sector
---------|-----------|----------|---------|-----------|-------|---------|----------------
Amiga    | `ADF`     | 880 KiB  | DD 3.5" | 80        | 2     | 11      | 512
C64      | `D64`     | 170 KiB  | SS 5.25"| 35        | 1     | 17-21   | 256
C64      | `PRG`     | N/A      | File    | N/A       | N/A   | N/A     | N/A
C64      | `CRT`     | N/A      | Cart    | N/A       | N/A   | N/A     | N/A
Apple II | `D13`     | 113 KiB  | SS 5.25"| 35        | 1     | 13      | 256
Apple II | `DO`, `DSK` | 140 KiB | SS 5.25"| 35        | 1     | 16      | 256
Apple II | `PO`      | 140 KiB  | SS 5.25"| 35        | 1     | 16      | 256
Apple II | `NIB`     | 140 KiB  | SS 5.25"| 35        | 1     | 13 / 16 | 256
Apple II | `WOZ`     | 140 KiB  | SS 5.25"| 35        | 1     | 13 / 16 | 256
Atari 8-bit | `ATR` | 90 KiB   | SS 5.25"| 40        | 1     | 18      | 128
IBM PC   | `IMG`, `IMA`, `VFD` | 160 KiB - 2.88 MiB | 5.25" / 3.5" | 40 / 80 | 1 / 2 | 8 - 36 | 512

Apple II images may hold Apple DOS 3.x (13 or 16 sector) or ProDOS volumes. `NIB` and `WOZ` (v1 and v2)
images are decoded from their GCR nibble / bitstream data (5-and-3 and 6-and-2 encoding).

IBM PC images may hold FAT12, FAT16 or FAT32 volumes, including long file names, DOS 1.x floppies without a
BPB, and MBR partitioned hard disk images. `ls` and `cat` accept paths such as `DOS` and `DOS/README.TXT`, and
`dump` shows the full directory tree.


# Usage

```
rdt -v | --version
rdt -h | --help

rdt ls -l workbench.adf
rdt ls -l dos.img DOS
rdt dump workbench.adf
rdt cat workbench.adf Clock
rdt terminal
```

`ls` prints the disk image format (`Format:`), the disk label (`Label:`), and the file listing (`Contents:`).
An optional directory argument lists a subdirectory instead of the root, on formats with subdirectories (IBM PC).
With `-l` the listing has `Type`, `Size`, `Modified` and `Name` columns. For Apple II (DOS 3.x) and C64 disks, an
extra `Native` column shows the file name as the original computer displayed it, using Unicode and ANSI styling:

- Apple II: inverse and flashing characters (flashing is shown as inverse yellow, since many terminals ignore blink)
- C64: names in quote mode, with PETSCII graphics as Unicode and control codes as reverse video symbols

`terminal` prints what `rdt` can detect about your terminal (platform, emulator, ANSI support) and shows
sample normal, inverse and flashing text, which is useful for checking how `Native` names will look.

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

