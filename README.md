# Retro Disk Tool (`rdt`)

Inspect retro computer archive disk formats.

# Supported formats

System   | Extension | Capacity | Medium  | Cyclinder | Heads | Sectors | Bytes per sector
---------|-----------|----------|---------|-----------|-------|---------|----------------
Amiga    | `ADF`     | 880 KiB  | DD 3.5" | 80        | 2     | 11      | 512
C64      | `D64`     | 170 KiB  | SS 5.25"| 35        | 1     | 17-21   | 256
C64      | `PRG`     | N/A      | File    | N/A       | N/A   | N/A     | N/A
C64      | `CRT`     | N/A      | Cart    | N/A       | N/A   | N/A     | N/A


# Usage

```
rdt -v | --version
rdt -h | --help

rdt ls -l workbench.adf
rdt dump workbench.adf
```

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

