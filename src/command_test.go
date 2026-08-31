package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

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

// writeStubVerifier creates an executable that drains stdin and prints the given
// lines, standing in for the delegation-verify binary.
func writeStubVerifier(t *testing.T, lines ...string) string {
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
	stub := writeStubVerifier(t,
		`{"submitted_at_date":"2026-08-27","submitter":"A","verified":true}`,
		`starting verification`,
		`{"submitted_at_date":12345}`,
		`{"submitted_at_date":"2026-08-27","submitter":"B","verified":true}`,
	)

	submissions, err := testAppContext().runDelegationVerifyCommand(stub, "[]")
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
	stub := writeStubVerifier(t, `{"submitted_at_date":12345}`)

	submissions, err := testAppContext().runDelegationVerifyCommand(stub, "[]")
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

	_, err := testAppContext().runDelegationVerifyCommand(path, "[]")
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
	stub := writeStubVerifier(t)

	if _, err := testAppContext().runDelegationVerifyCommand(stub, "[]"); err == nil {
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

const (
	sokFailRecord   = `{"submitted_at_date":"2026-09-03","submitter":"A","block_hash":"hashA","verified":false,"validation_error":"sok message digest does not match the sok message"}`
	cleanRecord     = `{"submitted_at_date":"2026-09-03","submitter":"B","block_hash":"hashB","verified":true}`
	otherFailRecord = `{"submitted_at_date":"2026-09-03","submitter":"C","block_hash":"hashC","verified":false,"validation_error":"invalid block proof"}`
)

func TestRunDelegationVerifyCommandToleratesSokMismatch(t *testing.T) {
	// With TOLERATE_SOK_MISMATCH on, a record failing only the sok-digest
	// check is counted as valid; the clean record and the record failing for
	// any other reason are untouched.
	stub := writeStubVerifier(t, sokFailRecord, cleanRecord, otherFailRecord)

	appCtx := testAppContext()
	appCtx.AppConfig.TolerateSokMismatch = true

	submissions, err := appCtx.runDelegationVerifyCommand(stub, "[]")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(submissions) != 3 {
		t.Fatalf("got %d submissions, want 3 (%+v)", len(submissions), submissions)
	}
	if !submissions[0].Verified || submissions[0].ValidationError != "" {
		t.Errorf("sok-mismatch record should be tolerated, got verified=%v error=%q",
			submissions[0].Verified, submissions[0].ValidationError)
	}
	if !submissions[1].Verified || submissions[1].ValidationError != "" {
		t.Errorf("clean record should be untouched, got verified=%v error=%q",
			submissions[1].Verified, submissions[1].ValidationError)
	}
	if submissions[2].Verified || submissions[2].ValidationError != "invalid block proof" {
		t.Errorf("differently-failing record should be untouched, got verified=%v error=%q",
			submissions[2].Verified, submissions[2].ValidationError)
	}
}

func TestRunDelegationVerifyCommandKeepsSokMismatchWhenFlagOff(t *testing.T) {
	// With the flag off, behavior is unchanged: the sok-mismatch record keeps
	// its failure like any other.
	stub := writeStubVerifier(t, sokFailRecord, cleanRecord, otherFailRecord)

	submissions, err := testAppContext().runDelegationVerifyCommand(stub, "[]")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(submissions) != 3 {
		t.Fatalf("got %d submissions, want 3 (%+v)", len(submissions), submissions)
	}
	if submissions[0].Verified || !strings.Contains(submissions[0].ValidationError, sokMismatchError) {
		t.Errorf("sok-mismatch record should keep its failure, got verified=%v error=%q",
			submissions[0].Verified, submissions[0].ValidationError)
	}
	if !submissions[1].Verified || submissions[1].ValidationError != "" {
		t.Errorf("clean record should be untouched, got verified=%v error=%q",
			submissions[1].Verified, submissions[1].ValidationError)
	}
	if submissions[2].Verified || submissions[2].ValidationError != "invalid block proof" {
		t.Errorf("differently-failing record should be untouched, got verified=%v error=%q",
			submissions[2].Verified, submissions[2].ValidationError)
	}
}
