package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

const (
	// Upper bound on how much of a rejected record we echo into the log.
	maxLoggedRecord = 512
	// Upper bound on how much of the verifier's stderr we retain.
	maxCapturedStderr = 4096
)

func (ctx *AppContext) runDelegationVerifyCommand(command, input string) ([]Submission, error) {
	cmd := ctx.buildVerifyCommand(command)

	submissions, err := ctx.runVerifyPass(cmd, command, input)
	if err != nil {
		return nil, err
	}

	if ctx.AppConfig.TolerateSokMismatch {
		submissions = ctx.retrySokMismatchesWithoutSnarkWork(cmd, command, input, submissions)
	}

	return submissions, nil
}

// buildVerifyCommand assembles the verifier invocation. The retry pass reuses
// it so a re-verification always runs against the same binary, flags and
// config file as the pass that produced the failure.
func (ctx *AppContext) buildVerifyCommand(command string) string {
	// Start building the command
	cmd := fmt.Sprintf("%v stdin", command)

	// Add --no-checks flag if needed
	if ctx.AppConfig.NoChecks {
		ctx.Log.Info("Note! Running with --no-checks flag. This will skip some checks.")
		cmd = fmt.Sprintf("%s --no-checks", cmd)
	}

	// Add --config-file flag if ConfigFile is specified
	if ctx.AppConfig.GenesisLedgerFile != "" {
		cmd = fmt.Sprintf("%s --config-file %s", cmd, ctx.AppConfig.GenesisLedgerFile)
	}

	return cmd
}

// runVerifyPass runs one verifier invocation over input and returns the records
// it emitted. command is the bare binary path, used for logging only.
func (ctx *AppContext) runVerifyPass(cmd, command, input string) ([]Submission, error) {
	out, stderr, err := runCommand(cmd, input)
	if err != nil {
		return nil, fmt.Errorf("error running %v: %w", command, err)
	}
	// Exit status 0 does not mean it had nothing to say.
	if stderr != "" {
		ctx.Log.Warnf("%v wrote to stderr: %s", command, stderr)
	}

	submissions, malformed := parseDelegationVerifyOutput(out)

	// A record we cannot parse costs us that one submission, never the rest of
	// the batch: submissions come from unrelated nodes and must not be able to
	// invalidate each other. A batch in which nothing parsed is a different
	// matter - that is the verifier misbehaving, not one bad submitter.
	for _, record := range malformed {
		ctx.Log.Errorf("Skipping unparseable record: %s", truncateHead(record, maxLoggedRecord))
	}
	if len(malformed) > 0 {
		ctx.Log.Errorf("Skipped %d unparseable record(s) out of %d returned by %v",
			len(malformed), len(malformed)+len(submissions), command)
	}
	// We are only called with a non-empty batch, so coming back with nothing is a
	// failure however it happened - a verifier that emitted nothing, or one whose
	// output we no longer recognise - and must not pass for an empty batch.
	if len(submissions) == 0 {
		if len(malformed) > 0 {
			return nil, fmt.Errorf("none of the %d records returned by %v could be parsed", len(malformed), command)
		}
		return nil, fmt.Errorf("%v returned no submission records", command)
	}

	return submissions, nil
}

// sokMismatchError is the verifier's message for a snark-work sok-digest
// mismatch. The verifier has always enforced the binding; mainnet never saw
// it fail only because pre-3.4.0 daemons never route uptime snark work
// through the zkApp-segment path that stamps the default sok digest into the
// statement (MinaProtocol/mina#19299) - a path the released Mesa daemons all
// carry. The binding prevents fee/prover misattribution in the snark pool,
// where work is paid; uptime snark work is never pooled or paid, so the
// mismatch can be waived via TOLERATE_SOK_MISMATCH until the daemon fix
// (MinaProtocol/mina#19313) is deployed fleet-wide.
const sokMismatchError = "sok message digest does not match the sok message"

// submissionKey identifies a submission across the verifier round trip. The
// verifier echoes the fields it was given, so these two are carried both on the
// batch we send and on the records we get back.
func submissionKey(submission Submission) string {
	return submission.SubmittedAt.UTC().Format(time.RFC3339Nano) + "|" + submission.Submitter
}

// retrySokMismatchesWithoutSnarkWork re-verifies submissions that failed only
// the snark-work sok-digest check, this time with the snark work stripped.
//
// Marking the record verified in place is not enough: delegation_verify
// short-circuits on the first error, so a sok-failed record comes back with no
// state_hash, parent, height or slot. The coordinator awards a point only when
// a submission's state_hash appears in its shortlist, and NULL state hashes are
// dropped before that comparison - so a record marked verified but carrying an
// empty payload still scores zero. Re-running the submission without snark work
// makes the verifier skip the snark-work path entirely and verify the block on
// its own, which produces a complete payload with a real state_hash. The block
// proof is still verified in full; only the pooled-work binding is skipped.
//
// The retry reuses the command of the pass that failed, so in dual-verifier
// deployments each partition retries against its own binary and config.
func (ctx *AppContext) retrySokMismatchesWithoutSnarkWork(cmd, command, input string, submissions []Submission) []Submission {
	failedIdx := make(map[string]int)
	for i, submission := range submissions {
		if strings.Contains(submission.ValidationError, sokMismatchError) {
			failedIdx[submissionKey(submission)] = i
		}
	}
	if len(failedIdx) == 0 {
		return submissions
	}

	// The verifier echoes back what it was given, but not the block itself, so
	// the retry batch has to be rebuilt from the submissions we sent.
	var originals []Submission
	if err := json.Unmarshal([]byte(input), &originals); err != nil {
		ctx.Log.Errorf("Cannot retry %d sok-mismatch submission(s): unreadable verifier input: %v", len(failedIdx), err)
		return submissions
	}

	var retryBatch []Submission
	for _, original := range originals {
		i, ok := failedIdx[submissionKey(original)]
		if !ok {
			continue
		}
		ctx.Log.Infof("Re-verifying sok-mismatch submission without snark work: submitter %s, block hash %s, original error: %s",
			submissions[i].Submitter, submissions[i].BlockHash, submissions[i].ValidationError)
		// Without snark work the verifier takes its "nothing to check" branch
		// for the snark-work statement and goes on to verify the block.
		original.SnarkWork = nil
		retryBatch = append(retryBatch, original)
	}
	if len(retryBatch) == 0 {
		ctx.Log.Errorf("Cannot retry %d sok-mismatch submission(s): none matched the submissions sent to %v", len(failedIdx), command)
		return submissions
	}

	retryJSON, err := json.Marshal(retryBatch)
	if err != nil {
		ctx.Log.Errorf("Cannot retry %d sok-mismatch submission(s): %v", len(retryBatch), err)
		return submissions
	}

	// A failed retry costs only the retried records, which are already failing.
	retried, err := ctx.runVerifyPass(cmd, command, string(retryJSON))
	if err != nil {
		ctx.Log.Errorf("Re-verification of %d sok-mismatch submission(s) failed, keeping the original results: %v", len(retryBatch), err)
		return submissions
	}

	verified := 0
	for _, record := range retried {
		i, ok := failedIdx[submissionKey(record)]
		if !ok {
			ctx.Log.Errorf("Re-verification returned an unrecognised record for submitter %s, ignoring it", record.Submitter)
			continue
		}
		// Keep the retried verdict either way: if it still fails, that failure
		// is the accurate one, and it is no longer the sok check.
		if record.ValidationError != "" || !record.Verified {
			ctx.Log.Errorf("Submission from %s still fails without snark work: %s",
				record.Submitter, record.ValidationError)
		} else {
			verified++
		}
		submissions[i] = record
	}
	ctx.Log.Infof("Re-verified %d sok-mismatch submission(s) without snark work; %d now verified", len(retryBatch), verified)

	return submissions
}

// Output from the delegation verification binary is expected to be newline-separated JSON
// objects, one per submission. When run with --config-file the binary also writes log lines
// to the same stream, so anything that is not a submission record is skipped quietly.
// Records that look like submissions but fail to unmarshal are returned separately so the
// caller can report them; they never abort the batch.
func parseDelegationVerifyOutput(data string) (submissions []Submission, malformed []string) {
	for _, record := range strings.Split(data, "\n") {
		record = strings.TrimSpace(record)
		if record == "" {
			continue
		}

		// Not JSON at all. Usually a log line from the verifier, but a record cut
		// short (the process died mid-write) also lands here, so anything naming
		// the key we expect on a submission is reported rather than dropped.
		var fields map[string]json.RawMessage
		if err := json.Unmarshal([]byte(record), &fields); err != nil {
			if strings.Contains(record, "submitted_at_date") {
				malformed = append(malformed, record)
			}
			continue
		}
		// JSON, but not a submission record. submitted_at_date is present on every
		// submission we send and is echoed back by the verifier.
		if _, ok := fields["submitted_at_date"]; !ok {
			continue
		}

		var submission Submission
		if err := json.Unmarshal([]byte(record), &submission); err != nil {
			malformed = append(malformed, record)
			continue
		}

		submissions = append(submissions, submission)
	}

	return submissions, malformed
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

	// Return whatever was written even on failure. A verifier that dies partway
	// through has already emitted complete records for the submissions it got to,
	// and those belong to nodes that did nothing wrong.
	if err := cmd.Run(); err != nil {
		if msg := errBuf.String(); msg != "" {
			return outBuf.String(), msg, fmt.Errorf("failed to run command: %w: %s", err, msg)
		}
		return outBuf.String(), "", fmt.Errorf("failed to run command: %w", err)
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

// truncateHead keeps the first max bytes of s.
func truncateHead(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "... (truncated)"
}
