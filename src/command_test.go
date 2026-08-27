package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	logging "github.com/ipfs/go-log/v2"
)

func TestRunCommand(t *testing.T) {
	testCases := []struct {
		name    string
		command string
		input   string
		want    string
		wantErr bool
	}{
		{
			name:    "without input",
			command: "echo -n",
			input:   "",
			want:    "",
			wantErr: false,
		},
		{
			name:    "with input",
			command: "cat",
			input:   "Hello",
			want:    "Hello",
			wantErr: false,
		},
		{
			name:    "invalid command",
			command: "nonexistentcommand",
			input:   "",
			want:    "",
			wantErr: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := runCommand(tc.command, tc.input)
			if (err != nil) != tc.wantErr {
				t.Errorf("runCommand(%q, %q) error = %v, wantErr %v", tc.command, tc.input, err, tc.wantErr)
				return
			}
			if !tc.wantErr && strings.TrimSpace(got) != tc.want {
				t.Errorf("runCommand(%q, %q) = %q, want %q", tc.command, tc.input, got, tc.want)
			}
		})
	}
}

func TestPartitionSubmissionsByCutover(t *testing.T) {
	cutover := time.Date(2026, 9, 3, 0, 0, 0, 0, time.UTC)
	submissions := []Submission{
		{ID: "before", SubmittedAt: cutover.Add(-time.Hour)},
		{ID: "just-before", SubmittedAt: cutover.Add(-time.Second)},
		{ID: "at-cutover", SubmittedAt: cutover},
		{ID: "after", SubmittedAt: cutover.Add(time.Hour)},
	}

	preFork, postFork := partitionSubmissionsByCutover(submissions, cutover)

	wantPreFork := []string{"before", "just-before"}
	wantPostFork := []string{"at-cutover", "after"}

	if len(preFork) != len(wantPreFork) {
		t.Fatalf("partitionSubmissionsByCutover() pre-fork size = %v, want %v", len(preFork), len(wantPreFork))
	}
	for i, sub := range preFork {
		if sub.ID != wantPreFork[i] {
			t.Errorf("partitionSubmissionsByCutover() pre-fork[%d] = %v, want %v", i, sub.ID, wantPreFork[i])
		}
	}
	if len(postFork) != len(wantPostFork) {
		t.Fatalf("partitionSubmissionsByCutover() post-fork size = %v, want %v", len(postFork), len(wantPostFork))
	}
	for i, sub := range postFork {
		if sub.ID != wantPostFork[i] {
			t.Errorf("partitionSubmissionsByCutover() post-fork[%d] = %v, want %v", i, sub.ID, wantPostFork[i])
		}
	}
}

// writeStubVerifier creates a stub delegation-verify executable that records
// the arguments it was invoked with and echoes each input submission back on
// its own line, tagging validation_error with the given marker so tests can
// tell which stub processed each submission.
func writeStubVerifier(t *testing.T, dir, name, marker string) (binPath, argsFile string) {
	t.Helper()
	binPath = filepath.Join(dir, name)
	argsFile = binPath + ".args"
	script := fmt.Sprintf(`#!/bin/sh
printf '%%s ' "$@" > %q
sed -e 's/^\[//' -e 's/\]$//' -e 's/"validation_error":""/"validation_error":"%s"/g' -e 's/},{/}\
{/g'
`, argsFile, marker)
	if err := os.WriteFile(binPath, []byte(script), 0o755); err != nil {
		t.Fatalf("error writing stub verifier %v: %v", name, err)
	}
	return binPath, argsFile
}

func newTestAppContext(cutover time.Time, preBin, postBin string) *AppContext {
	return &AppContext{
		AppConfig: AppConfig{
			DelegationVerifyBinPath:         preBin,
			GenesisLedgerFile:               "/config/pre-fork-ledger.json",
			ForkCutoverTime:                 &cutover,
			DelegationVerifyBinPathPostFork: postBin,
			GenesisLedgerFilePostFork:       "/config/post-fork-ledger.json",
		},
		Log: logging.Logger("test"),
	}
}

func TestRunDualDelegationVerify(t *testing.T) {
	cutover := time.Date(2026, 9, 3, 0, 0, 0, 0, time.UTC)
	dir := t.TempDir()
	preBin, preArgsFile := writeStubVerifier(t, dir, "delegation-verify-pre-fork", "pre-fork-stub")
	postBin, postArgsFile := writeStubVerifier(t, dir, "delegation-verify-post-fork", "post-fork-stub")
	appCtx := newTestAppContext(cutover, preBin, postBin)

	submissions := []Submission{
		{ID: "pre-1", SubmittedAtDate: "2026-09-02", SubmittedAt: cutover.Add(-time.Hour), Submitter: "B62pre1"},
		{ID: "pre-2", SubmittedAtDate: "2026-09-02", SubmittedAt: cutover.Add(-time.Second), Submitter: "B62pre2"},
		{ID: "at-cutover", SubmittedAtDate: "2026-09-03", SubmittedAt: cutover, Submitter: "B62boundary"},
		{ID: "post-1", SubmittedAtDate: "2026-09-03", SubmittedAt: cutover.Add(time.Hour), Submitter: "B62post1"},
	}

	verifiedSubmissions, err := appCtx.runDualDelegationVerify(submissions)
	if err != nil {
		t.Fatalf("runDualDelegationVerify() error = %v", err)
	}

	// each submission should be processed by exactly the right stub, and the
	// results of both runs should be merged
	wantMarkers := map[string]string{
		"pre-1":      "pre-fork-stub",
		"pre-2":      "pre-fork-stub",
		"at-cutover": "post-fork-stub",
		"post-1":     "post-fork-stub",
	}
	if len(verifiedSubmissions) != len(submissions) {
		t.Fatalf("runDualDelegationVerify() returned %v submissions, want %v", len(verifiedSubmissions), len(submissions))
	}
	seen := make(map[string]bool)
	for _, sub := range verifiedSubmissions {
		want, known := wantMarkers[sub.ID]
		if !known {
			t.Errorf("runDualDelegationVerify() returned unexpected submission %v", sub.ID)
			continue
		}
		if seen[sub.ID] {
			t.Errorf("runDualDelegationVerify() returned submission %v more than once", sub.ID)
		}
		seen[sub.ID] = true
		if sub.ValidationError != want {
			t.Errorf("submission %v processed by %q, want %q", sub.ID, sub.ValidationError, want)
		}
	}

	// each stub should be invoked with its own config file
	assertStubArgs(t, preArgsFile, "stdin --config-file /config/pre-fork-ledger.json")
	assertStubArgs(t, postArgsFile, "stdin --config-file /config/post-fork-ledger.json")
}

func TestRunDualDelegationVerifySkipsEmptyPartition(t *testing.T) {
	cutover := time.Date(2026, 9, 3, 0, 0, 0, 0, time.UTC)
	dir := t.TempDir()
	preBin, _ := writeStubVerifier(t, dir, "delegation-verify-pre-fork", "pre-fork-stub")
	postBin, postArgsFile := writeStubVerifier(t, dir, "delegation-verify-post-fork", "post-fork-stub")
	appCtx := newTestAppContext(cutover, preBin, postBin)

	// all submissions are pre-fork, so the post-fork stub must not be invoked
	submissions := []Submission{
		{ID: "pre-1", SubmittedAtDate: "2026-09-02", SubmittedAt: cutover.Add(-time.Hour), Submitter: "B62pre1"},
	}

	verifiedSubmissions, err := appCtx.runDualDelegationVerify(submissions)
	if err != nil {
		t.Fatalf("runDualDelegationVerify() error = %v", err)
	}
	if len(verifiedSubmissions) != 1 || verifiedSubmissions[0].ValidationError != "pre-fork-stub" {
		t.Errorf("runDualDelegationVerify() = %v, want single submission processed by pre-fork-stub", verifiedSubmissions)
	}
	if _, err := os.Stat(postArgsFile); !os.IsNotExist(err) {
		t.Errorf("post-fork stub was invoked for an empty partition")
	}
}

func assertStubArgs(t *testing.T, argsFile, want string) {
	t.Helper()
	args, err := os.ReadFile(argsFile)
	if err != nil {
		t.Fatalf("error reading stub args file %v: %v", argsFile, err)
	}
	if got := strings.TrimSpace(string(args)); got != want {
		t.Errorf("stub invoked with args %q, want %q", got, want)
	}
}
