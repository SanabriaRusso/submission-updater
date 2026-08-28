package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
)

// Upper bound on how much of the verifier's stderr we retain.
const maxCapturedStderr = 4096

func (ctx *AppContext) runDelegationVerifyCommand(command, input string) ([]Submission, error) {
	var cmd string

	// Start building the command
	cmd = fmt.Sprintf("%v stdin", command)

	// Add --no-checks flag if needed
	if ctx.AppConfig.NoChecks {
		ctx.Log.Info("Note! Running with --no-checks flag. This will skip some checks.")
		cmd = fmt.Sprintf("%s --no-checks", cmd)
	}

	// Add --config-file flag if ConfigFile is specified
	if ctx.AppConfig.GenesisLedgerFile != "" {
		cmd = fmt.Sprintf("%s --config-file %s", cmd, ctx.AppConfig.GenesisLedgerFile)
	}

	out, stderr, err := runCommand(cmd, input)
	if err != nil {
		return nil, fmt.Errorf("error running %v: %w", command, err)
	}
	// Exit status 0 does not mean it had nothing to say.
	if stderr != "" {
		ctx.Log.Warnf("%v wrote to stderr: %s", command, stderr)
	}

	submissions, err := parseDelegationVerifyOutput(out)
	if err != nil {
		return nil, fmt.Errorf("error parsing submissions: %w", err)
	}

	return submissions, nil
}

// Output from the delegation verification binary is expected to be a newline-separated JSON array of Submission objects.
// We parse this into a slice of Submission objects.
func parseDelegationVerifyOutput(data string) ([]Submission, error) {
	var submissions []Submission

	// Split the input data into separate records based on newline.
	records := strings.Split(data, "\n")

	for _, record := range records {
		if record == "" {
			continue // Skip empty lines
		}
		// skip all lines that do not have submitted_at_date, which indicates optput is a submission
		// and not a log line (when using --config-file flag, the output will contain additional log lines as well)
		if !strings.Contains(record, "submitted_at_date") {
			continue
		}

		var submission Submission
		if err := json.Unmarshal([]byte(record), &submission); err != nil {
			return nil, err // Return error if any record fails to unmarshal
		}

		submissions = append(submissions, submission)
	}

	return submissions, nil
}

// runCommand returns the command's stdout and stderr. The verifier reports its
// failures on stderr and exits with a distinct status per failure kind, so dropping
// that stream leaves a bare exit code with no way to tell a bad config file from a
// bad submission. Stderr is returned even on success, since exiting 0 does not mean
// the command had nothing to report.
func runCommand(command, input string) (stdout string, stderr string, err error) {
	cmdParts := strings.Split(command, " ")
	cmd := exec.Command(cmdParts[0], cmdParts[1:]...)

	cmd.Stdin = bytes.NewBufferString(input)

	var outBuf bytes.Buffer
	errBuf := &boundedBuffer{max: maxCapturedStderr}
	cmd.Stdout = &outBuf
	cmd.Stderr = errBuf

	if err := cmd.Run(); err != nil {
		if msg := errBuf.String(); msg != "" {
			return "", msg, fmt.Errorf("failed to run command: %w: %s", err, msg)
		}
		return "", "", fmt.Errorf("failed to run command: %w", err)
	}

	return outBuf.String(), errBuf.String(), nil
}

// boundedBuffer keeps at most max bytes, dropping from the front. A command that
// floods stderr over a long batch must not grow our memory without limit, and the
// tail is the part worth keeping: the failure that made it give up is the last
// thing it writes.
type boundedBuffer struct {
	max       int
	buf       []byte
	truncated bool
}

func (b *boundedBuffer) Write(p []byte) (int, error) {
	written := len(p)

	if len(p) > b.max {
		p = p[len(p)-b.max:]
		b.truncated = true
	}
	b.buf = append(b.buf, p...)
	if len(b.buf) > b.max {
		b.buf = b.buf[len(b.buf)-b.max:]
		b.truncated = true
	}

	// Report the full length: a short write is an error to exec.Cmd.
	return written, nil
}

func (b *boundedBuffer) String() string {
	s := strings.TrimSpace(string(b.buf))
	if s != "" && b.truncated {
		return "(truncated) ..." + s
	}
	return s
}
