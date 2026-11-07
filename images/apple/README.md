# Apple II test images

These disk images are not stored in the repository. Tests download them here the first time they are needed (only if
missing), verifying each file's size and SHA-256 (see `internal/testimages`). To download them all ahead of time:

```
go run ./internal/testimages/fetch
```

Sources, from the [Apple II Documentation Project](https://mirrors.apple2.org.za/Apple%20II%20Documentation%20Project/Software/Operating%20Systems/):

- `Apple DOS/Disk Images/`: `Apple DOS 3.1 Master.nib`, `Apple DOS 3.1 Master.d13`, `Apple DOS 3.3 August 1980.dsk`
- `Apple ProDOS/Disk Images/`: `ProDOS 2.4.2.dsk`
- `Apple Pascal/Disk Images/`: `Apple Pascal - Disk 0.po`
- `Apple CPM/Disk Images/`: `CPAM 3.5.po`

From [4am's WOZ-A-Day collection](https://archive.org/details/wozaday) on archive.org:

- `Triple-Dump.woz`: [Triple-Dump (DOS 3.3)](https://archive.org/details/wozaday_Triple_Dump) (`00playable.woz`)
