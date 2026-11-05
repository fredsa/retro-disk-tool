package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestRunVersion(t *testing.T) {
	for _, arg := range []string{"-v", "--version"} {
		var stdout, stderr bytes.Buffer
		code := run([]string{arg}, &stdout, &stderr)
		if code != 0 {
			t.Errorf("run(%q) exited with %d, want 0", arg, code)
		}
		if !strings.Contains(stdout.String(), version) {
			t.Errorf("run(%q) stdout = %q, want version %q", arg, stdout.String(), version)
		}
	}
}

func TestRunHelp(t *testing.T) {
	for _, arg := range []string{"-h", "--help"} {
		var stdout, stderr bytes.Buffer
		code := run([]string{arg}, &stdout, &stderr)
		if code != 0 {
			t.Errorf("run(%q) exited with %d, want 0", arg, code)
		}
		if !strings.Contains(stdout.String(), "Usage:") {
			t.Errorf("run(%q) stdout = %q, want usage info", arg, stdout.String())
		}
	}
}

func TestRunLs(t *testing.T) {
	testFile := "test_data/Workbench v1.0 (1985)(Commodore).adf"
	tests := []struct {
		args        []string
		wantContain []string
	}{
		{
			args:        []string{"ls", testFile},
			wantContain: []string{"Clock", "c", "devs", "libs", "Capacity:", "Formatted:   880 KiB (901,120 bytes)", "Unformatted:    1 MB (1,000,000 bytes)", "Free:    26.2%    462 sectors (236,544 bytes)", "In-use:  73.8%  1,298 sectors (664,576 bytes)", "Total:  100.0%  1,760 sectors (901,120 bytes)"},
		},
		{
			args:        []string{"ls", "-l", testFile},
			wantContain: []string{"-    15800", "d        0", "Clock", "devs/", "Capacity:", "Usage:"},
		},
		{
			args:        []string{"ls", "test_data/JeepersCreepersPlus1-UNK.d64"},
			wantContain: []string{"JEEP.CREEPERS+", "170 KiB (174,848 bytes)", "250 KB (250,000 bytes)", "Free:    84.9%    580 sectors"},
		},
		{
			args:        []string{"ls", "-l", "test_data/JeepersCreepersPlus1-UNK.d64"},
			wantContain: []string{"-    25627", "JEEP.CREEPERS+", "Capacity:", "Usage:"},
		},
		{
			args:        []string{"ls", "-l", "test_data/flipandflop.prg"},
			wantContain: []string{"-    25071", "flipandflop.prg"},
		},
		{
			args:        []string{"ls", "-l", "test_data/EF3 Adrian Toolset.crt"},
			wantContain: []string{"-   622592", "\"ndefpack 1.0.1\""},
		},
	}
	for _, tt := range tests {
		var stdout, stderr bytes.Buffer
		code := run(tt.args, &stdout, &stderr)
		if code != 0 {
			t.Errorf("run(%v) exited with %d, want 0 (stderr: %s)", tt.args, code, stderr.String())
		}
		for _, w := range tt.wantContain {
			if !strings.Contains(stdout.String(), w) {
				t.Errorf("run(%v) stdout missing %q; got:\n%s", tt.args, w, stdout.String())
			}
		}
	}
}

func TestRunDump(t *testing.T) {
	tests := []struct {
		file        string
		wantContain []string
	}{
		{
			file:        "test_data/Workbench v1.0 (1985)(Commodore).adf",
			wantContain: []string{"Amiga ADF Disk Image Dump", "Workbench", "Root Block: sector 880"},
		},
		{
			file:        "test_data/JeepersCreepersPlus1-UNK.d64",
			wantContain: []string{"Commodore 64 1541 Disk Image (D64) Dump", "JEEP.CREEPERS+", "101 sectors"},
		},
		{
			file:        "test_data/flipandflop.prg",
			wantContain: []string{"Commodore 64 PRG File Dump", "Load Address: 0x0801", "25071 bytes"},
		},
		{
			file:        "test_data/EF3 Adrian Toolset.crt",
			wantContain: []string{"Commodore 64 Cartridge (CRT) Dump", "ndefpack 1.0.1", "CHIP Packets (76)"},
		},
	}
	for _, tt := range tests {
		var stdout, stderr bytes.Buffer
		code := run([]string{"dump", tt.file}, &stdout, &stderr)
		if code != 0 {
			t.Errorf("run(dump %s) returned %d, want 0 (stderr: %s)", tt.file, code, stderr.String())
		}
		for _, w := range tt.wantContain {
			if !strings.Contains(stdout.String(), w) {
				t.Errorf("run(dump %s) output missing %q", tt.file, w)
			}
		}
	}
}
