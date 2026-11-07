# C64 test images

Most images in this folder are not stored in the repository. Images with a download source are fetched here by the
tests the first time they are needed (only if missing), verifying each file's size and SHA-256 (see
`internal/testimages`), or ahead of time with `go run ./internal/testimages/fetch`:

- `EF3 Adrian Toolset.crt`: from [misterblack1/ef3-c64-tools](https://github.com/misterblack1/ef3-c64-tools) at commit `dfe7c04`
