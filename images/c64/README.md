# C64 test images

Most images in this folder are not stored in the repository. Images with a download source are fetched here by the
tests the first time they are needed (only if missing), verifying each file's size and SHA-256 (see
`internal/testimages`), or ahead of time with `go run ./internal/testimages/fetch`:

- `EF3 Adrian Toolset.crt`: from [misterblack1/ef3-c64-tools](https://github.com/misterblack1/ef3-c64-tools) at commit `dfe7c04`
- From the [Meatloaf](https://c64.meatloaf.cc/test/) C64 test collection (requested with `User-Agent: MEATLOAF/`):
  - `m.u.l.e.d64`, `lemmings.dfi`, `ultima v.d81`, `1942.tap`, `choppin_mall.t64`
  - `42 tracks/`: `scan42.d64`, `scan42err.d64`
  - `bad listing/`: `topsecretstuff.d64`
  - `bam message/`: `053_diskdevil5.d64`, `06-expert_v41.d64`, `60k_tool.d64`, `sb005157-df624d.d64`, `sb005230-c4fee2.d64`
  - `error info/`: `0006x.d64`, `0027.d64`, `0152.d64`, `0168.d64`, `0180.d64`, `0206_heroes_of_lance.d64`,
    `0216_sceptre_of_b.d64`, `0220.d64`, `0587_cpx_upheav rips.d64`, `0905.d64.d81`, `0923.d64.d81`,
    `abacus-ada-b2b7ea.d64`, `scan35err.d64`, `scan40err.d64`, `scan42.d64`, `scan42err.d64`

Tests that use other images in this folder (G64, TCRT and some D64 / DFI / T64 images) are skipped when the images are
absent.
