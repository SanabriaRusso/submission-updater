package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	logging "github.com/ipfs/go-log/v2"
)

func TestParseForkCutoverConfig(t *testing.T) {
	dir := t.TempDir()
	executableBin := filepath.Join(dir, "delegation-verify-post-fork")
	if err := os.WriteFile(executableBin, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatalf("error writing executable bin fixture: %v", err)
	}
	nonExecutableBin := filepath.Join(dir, "delegation-verify-post-fork-no-exec")
	if err := os.WriteFile(nonExecutableBin, []byte("#!/bin/sh\n"), 0o644); err != nil {
		t.Fatalf("error writing non-executable bin fixture: %v", err)
	}
	configFile := filepath.Join(dir, "post-fork-ledger.json")
	if err := os.WriteFile(configFile, []byte("{}"), 0o644); err != nil {
		t.Fatalf("error writing config file fixture: %v", err)
	}
	missingPath := filepath.Join(dir, "does-not-exist")

	testCases := []struct {
		name               string
		cutover            string
		postForkBinPath    string
		postForkConfigPath string
		want               *time.Time
		wantErr            bool
	}{
		{
			name:               "cutover unset disables dual-verifier mode",
			cutover:            "",
			postForkBinPath:    "",
			postForkConfigPath: "",
			want:               nil,
			wantErr:            false,
		},
		{
			name:               "cutover unset skips validation of post-fork paths",
			cutover:            "",
			postForkBinPath:    missingPath,
			postForkConfigPath: missingPath,
			want:               nil,
			wantErr:            false,
		},
		{
			name:               "invalid RFC3339 cutover",
			cutover:            "2026-09-03 00:00:00",
			postForkBinPath:    executableBin,
			postForkConfigPath: configFile,
			want:               nil,
			wantErr:            true,
		},
		{
			name:               "cutover set without post-fork bin",
			cutover:            "2026-09-03T00:00:00Z",
			postForkBinPath:    "",
			postForkConfigPath: configFile,
			want:               nil,
			wantErr:            true,
		},
		{
			name:               "cutover set without post-fork config file",
			cutover:            "2026-09-03T00:00:00Z",
			postForkBinPath:    executableBin,
			postForkConfigPath: "",
			want:               nil,
			wantErr:            true,
		},
		{
			name:               "cutover set with nonexistent post-fork bin",
			cutover:            "2026-09-03T00:00:00Z",
			postForkBinPath:    missingPath,
			postForkConfigPath: configFile,
			want:               nil,
			wantErr:            true,
		},
		{
			name:               "cutover set with non-executable post-fork bin",
			cutover:            "2026-09-03T00:00:00Z",
			postForkBinPath:    nonExecutableBin,
			postForkConfigPath: configFile,
			want:               nil,
			wantErr:            true,
		},
		{
			name:               "cutover set with nonexistent post-fork config file",
			cutover:            "2026-09-03T00:00:00Z",
			postForkBinPath:    executableBin,
			postForkConfigPath: missingPath,
			want:               nil,
			wantErr:            true,
		},
		{
			name:               "valid cutover with executable bin and readable config file",
			cutover:            "2026-09-03T00:00:00Z",
			postForkBinPath:    executableBin,
			postForkConfigPath: configFile,
			want:               timePtr(time.Date(2026, 9, 3, 0, 0, 0, 0, time.UTC)),
			wantErr:            false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := parseForkCutoverConfig(tc.cutover, tc.postForkBinPath, tc.postForkConfigPath)
			if (err != nil) != tc.wantErr {
				t.Errorf("parseForkCutoverConfig(%q, %q, %q) error = %v, wantErr %v", tc.cutover, tc.postForkBinPath, tc.postForkConfigPath, err, tc.wantErr)
				return
			}
			if (got == nil) != (tc.want == nil) {
				t.Errorf("parseForkCutoverConfig(%q, %q, %q) = %v, want %v", tc.cutover, tc.postForkBinPath, tc.postForkConfigPath, got, tc.want)
				return
			}
			if got != nil && !got.Equal(*tc.want) {
				t.Errorf("parseForkCutoverConfig(%q, %q, %q) = %v, want %v", tc.cutover, tc.postForkBinPath, tc.postForkConfigPath, got, tc.want)
			}
		})
	}
}

func timePtr(t time.Time) *time.Time {
	return &t
}

func TestTolerateSokMismatchEnvParsing(t *testing.T) {
	log := logging.Logger("test")

	testCases := []struct {
		name  string
		value string
		want  bool
	}{
		{name: "unset defaults to off", value: "", want: false},
		{name: "explicitly disabled", value: "0", want: false},
		{name: "enabled", value: "1", want: true},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("TOLERATE_SOK_MISMATCH", tc.value)
			if got := boolEnvChecked("TOLERATE_SOK_MISMATCH", log); got != tc.want {
				t.Errorf("boolEnvChecked(TOLERATE_SOK_MISMATCH=%q) = %v, want %v", tc.value, got, tc.want)
			}
		})
	}
}
