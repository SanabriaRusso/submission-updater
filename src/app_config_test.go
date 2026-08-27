package main

import (
	"testing"
	"time"
)

func TestParseForkCutoverConfig(t *testing.T) {
	testCases := []struct {
		name            string
		cutover         string
		postForkBinPath string
		want            *time.Time
		wantErr         bool
	}{
		{
			name:            "cutover unset disables dual-verifier mode",
			cutover:         "",
			postForkBinPath: "",
			want:            nil,
			wantErr:         false,
		},
		{
			name:            "cutover unset with post-fork bin still disables dual-verifier mode",
			cutover:         "",
			postForkBinPath: "/bin/delegation-verify-post-fork",
			want:            nil,
			wantErr:         false,
		},
		{
			name:            "invalid RFC3339 cutover",
			cutover:         "2026-09-03 00:00:00",
			postForkBinPath: "/bin/delegation-verify-post-fork",
			want:            nil,
			wantErr:         true,
		},
		{
			name:            "cutover set without post-fork bin",
			cutover:         "2026-09-03T00:00:00Z",
			postForkBinPath: "",
			want:            nil,
			wantErr:         true,
		},
		{
			name:            "valid cutover with post-fork bin",
			cutover:         "2026-09-03T00:00:00Z",
			postForkBinPath: "/bin/delegation-verify-post-fork",
			want:            timePtr(time.Date(2026, 9, 3, 0, 0, 0, 0, time.UTC)),
			wantErr:         false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := parseForkCutoverConfig(tc.cutover, tc.postForkBinPath)
			if (err != nil) != tc.wantErr {
				t.Errorf("parseForkCutoverConfig(%q, %q) error = %v, wantErr %v", tc.cutover, tc.postForkBinPath, err, tc.wantErr)
				return
			}
			if (got == nil) != (tc.want == nil) {
				t.Errorf("parseForkCutoverConfig(%q, %q) = %v, want %v", tc.cutover, tc.postForkBinPath, got, tc.want)
				return
			}
			if got != nil && !got.Equal(*tc.want) {
				t.Errorf("parseForkCutoverConfig(%q, %q) = %v, want %v", tc.cutover, tc.postForkBinPath, got, tc.want)
			}
		})
	}
}

func timePtr(t time.Time) *time.Time {
	return &t
}
