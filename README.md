# Retro Disk Tool (`rdt`)

Inspect retro computer archive disk formats.

# Supported formats

System   | Extension | Capacity | Medium  | Cyclinder | Heads | Sectors | Bytes per sector
---------|-----------|----------|---------|-----------|-------|---------|----------------
Amiga    | `ADF`     | 880 KiB  | DD 3.5" | 80        | 2     | 11      | 512


# Usage

```
rdt -v | --version
rdt -h | --help

rdt ls -l workbench.adf
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

