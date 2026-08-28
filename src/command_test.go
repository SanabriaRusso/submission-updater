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
			got, _, err := runCommand(tc.command, tc.input)
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

func TestRunCommandCapturesStderr(t *testing.T) {
	_, _, err := runCommand("sh -c echo_boom_and_fail", "")
	if err == nil {
		t.Fatal("expected an error for a missing command")
	}

	// The verifier reports why it gave up on stderr and signals what went wrong
	// through the exit status; both have to survive into the returned error.
	_, _, err = runCommand("sh -c", "")
	if err == nil {
		t.Fatal("expected an error when sh -c is given no script")
	}
	if !strings.Contains(err.Error(), "exit status") {
		t.Errorf("error should carry the exit status, got: %v", err)
	}
	if !strings.Contains(err.Error(), "sh:") && !strings.Contains(err.Error(), "option") {
		t.Errorf("error should carry the stderr text, got: %v", err)
	}
}

func TestParseDelegationVerifyOutput(t *testing.T) {
	const (
		validA     = `{"submitted_at_date":"2026-08-27","submitter":"A","verified":true}`
		validB     = `{"submitted_at_date":"2026-08-27","submitter":"B","verified":true}`
		logLine    = `2026-08-27 10:00:00 INFO starting verification`
		jsonLog    = `{"level":"info","message":"loaded config"}`
		badRecord  = `{"submitted_at_date":12345}`
		truncated  = `{"submitted_at_date":"2026-08-27","submitter":`
		emptyLines = "\n\n   \n"
	)

	testCases := []struct {
		name          string
		data          string
		wantSubmitter []string
		wantMalformed int
	}{
		{
			name:          "only valid records",
			data:          validA + "\n" + validB,
			wantSubmitter: []string{"A", "B"},
		},
		{
			name:          "log lines are skipped quietly",
			data:          logLine + "\n" + validA + "\n" + jsonLog + "\n" + validB + emptyLines,
			wantSubmitter: []string{"A", "B"},
		},
		{
			name:          "a malformed record does not cost the valid ones",
			data:          validA + "\n" + badRecord + "\n" + validB,
			wantSubmitter: []string{"A", "B"},
			wantMalformed: 1,
		},
		{
			name:          "a truncated record is reported, not silently dropped",
			data:          validA + "\n" + truncated + "\n" + validB,
			wantSubmitter: []string{"A", "B"},
			wantMalformed: 1,
		},
		{
			name:          "nothing parseable",
			data:          badRecord,
			wantSubmitter: nil,
			wantMalformed: 1,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			submissions, malformed := parseDelegationVerifyOutput(tc.data)

			var got []string
			for _, sub := range submissions {
				got = append(got, sub.Submitter)
			}
			if strings.Join(got, ",") != strings.Join(tc.wantSubmitter, ",") {
				t.Errorf("submitters = %v, want %v", got, tc.wantSubmitter)
			}
			if len(malformed) != tc.wantMalformed {
				t.Errorf("malformed = %d (%v), want %d", len(malformed), malformed, tc.wantMalformed)
			}
		})
	}
}

// writeStubPrinter creates an executable that drains stdin and prints the given
// lines, standing in for the delegation-verify binary.
func writeStubPrinter(t *testing.T, lines ...string) string {
	t.Helper()

	script := "#!/bin/sh\ncat > /dev/null\n"
	for _, line := range lines {
		script += "cat <<'RECORD'\n" + line + "\nRECORD\n"
	}

	path := filepath.Join(t.TempDir(), "delegation-verify-stub")
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatalf("writing stub: %v", err)
	}
	return path
}

func testAppContext() *AppContext {
	return &AppContext{Log: logging.Logger("submission-updater-test")}
}

func TestRunDelegationVerifyCommandIsolatesBadRecords(t *testing.T) {
	// One node's unparseable record must not discard the records belonging to
	// every other node in the same batch.
	stub := writeStubPrinter(t,
		`{"submitted_at_date":"2026-08-27","submitter":"A","verified":true}`,
		`starting verification`,
		`{"submitted_at_date":12345}`,
		`{"submitted_at_date":"2026-08-27","submitter":"B","verified":true}`,
	)

	submissions, err := testAppContext().runDelegationVerifyCommand(stub, "", "[]")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(submissions) != 2 {
		t.Fatalf("got %d submissions, want 2 (%+v)", len(submissions), submissions)
	}
	if submissions[0].Submitter != "A" || submissions[1].Submitter != "B" {
		t.Errorf("got submitters %q and %q, want A and B", submissions[0].Submitter, submissions[1].Submitter)
	}
}

func TestRunDelegationVerifyCommandFailsWhenNothingParses(t *testing.T) {
	// Losing every record is the verifier misbehaving rather than one bad
	// submitter, and must not be mistaken for an empty batch.
	stub := writeStubPrinter(t, `{"submitted_at_date":12345}`)

	submissions, err := testAppContext().runDelegationVerifyCommand(stub, "", "[]")
	if err == nil {
		t.Fatalf("expected an error, got %d submissions", len(submissions))
	}
	if !strings.Contains(err.Error(), "could be parsed") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestRunDelegationVerifyCommandReportsVerifierStderr(t *testing.T) {
	path := filepath.Join(t.TempDir(), "failing-verify")
	script := "#!/bin/sh\ncat > /dev/null\necho '{\"error\":\"fail to read config file\"}' >&2\nexit 4\n"
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatalf("writing stub: %v", err)
	}

	_, err := testAppContext().runDelegationVerifyCommand(path, "", "[]")
	if err == nil {
		t.Fatal("expected an error")
	}
	if !strings.Contains(err.Error(), "fail to read config file") {
		t.Errorf("error should carry the verifier's stderr, got: %v", err)
	}
	if !strings.Contains(err.Error(), "exit status 4") {
		t.Errorf("error should carry the exit status, got: %v", err)
	}
}

func TestRunDelegationVerifyCommandFailsOnEmptyOutput(t *testing.T) {
	// A verifier that returns nothing for a non-empty batch has failed; treating
	// that as an empty batch would quietly update no submissions and exit 0.
	stub := writeStubPrinter(t)

	if _, err := testAppContext().runDelegationVerifyCommand(stub, "", "[]"); err == nil {
		t.Fatal("expected an error when the verifier returns no records")
	}
}

func TestRunCommandKeepsOutputWrittenBeforeFailure(t *testing.T) {
	// Records emitted before the process died belong to nodes that did nothing
	// wrong, so the caller must still be able to see them.
	path := filepath.Join(t.TempDir(), "dies-partway")
	script := "#!/bin/sh\ncat > /dev/null\necho 'partial record'\necho 'boom' >&2\nexit 2\n"
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatalf("writing stub: %v", err)
	}

	stdout, stderr, err := runCommand(path, "")
	if err == nil {
		t.Fatal("expected an error")
	}
	if !strings.Contains(stdout, "partial record") {
		t.Errorf("stdout written before the failure was dropped, got %q", stdout)
	}
	if !strings.Contains(stderr, "boom") {
		t.Errorf("stderr was dropped, got %q", stderr)
	}
}

func TestBoundedBufferKeepsTail(t *testing.T) {
	b := &boundedBuffer{max: 8}
	if n, err := b.Write([]byte("0123456789abcdef")); n != 16 || err != nil {
		t.Fatalf("Write returned (%d, %v), want (16, nil)", n, err)
	}
	if got := b.String(); got != "(truncated) ...89abcdef" {
		t.Errorf("String() = %q, want the last 8 bytes marked truncated", got)
	}

	small := &boundedBuffer{max: 8}
	small.Write([]byte("hi\n"))
	if got := small.String(); got != "hi" {
		t.Errorf("String() = %q, want %q unmarked", got, "hi")
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

func TestRunDualDelegationVerifySkipsEmptyPreForkPartition(t *testing.T) {
	cutover := time.Date(2026, 9, 3, 0, 0, 0, 0, time.UTC)
	dir := t.TempDir()
	preBin, preArgsFile := writeStubVerifier(t, dir, "delegation-verify-pre-fork", "pre-fork-stub")
	postBin, _ := writeStubVerifier(t, dir, "delegation-verify-post-fork", "post-fork-stub")
	appCtx := newTestAppContext(cutover, preBin, postBin)

	// all submissions are at/after the cutover, so the pre-fork stub must not be invoked
	submissions := []Submission{
		{ID: "post-1", SubmittedAtDate: "2026-09-03", SubmittedAt: cutover.Add(time.Hour), Submitter: "B62post1"},
	}

	verifiedSubmissions, err := appCtx.runDualDelegationVerify(submissions)
	if err != nil {
		t.Fatalf("runDualDelegationVerify() error = %v", err)
	}
	if len(verifiedSubmissions) != 1 || verifiedSubmissions[0].ValidationError != "post-fork-stub" {
		t.Errorf("runDualDelegationVerify() = %v, want single submission processed by post-fork-stub", verifiedSubmissions)
	}
	if _, err := os.Stat(preArgsFile); !os.IsNotExist(err) {
		t.Errorf("pre-fork stub was invoked for an empty partition")
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
