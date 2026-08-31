package main

import (
	"bytes"
	"encoding/json"
	"errors"
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

func (ctx *AppContext) runDelegationVerifyCommand(command, configFile, input string) ([]Submission, error) {
	var cmd string

	// Start building the command
	cmd = fmt.Sprintf("%v stdin", command)

	// Add --no-checks flag if needed
	if ctx.AppConfig.NoChecks {
		ctx.Log.Info("Note! Running with --no-checks flag. This will skip some checks.")
		cmd = fmt.Sprintf("%s --no-checks", cmd)
	}

	// Add --config-file flag if configFile is specified
	if configFile != "" {
		cmd = fmt.Sprintf("%s --config-file %s", cmd, configFile)
	}

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

	if ctx.AppConfig.TolerateSokMismatch {
		submissions = ctx.tolerateSokMismatches(submissions)
	}

	return submissions, nil
}

// verifySubmissions is the single entry point for verification: it routes the
// batch through the delegation-verify binary - or, in dual-verifier mode, the
// pre- and post-fork binaries by submitted_at timestamp - and returns whatever
// verified. In dual mode a failing partition does not discard the other
// partition's completed work: successes are accumulated and returned alongside
// the joined error, so the caller can bank what verified before failing the run.
func (ctx *AppContext) verifySubmissions(submissions []Submission) ([]Submission, error) {
	cfg := ctx.AppConfig
	if cfg.ForkCutoverTime == nil {
		return ctx.verifyBatch(submissions, cfg.DelegationVerifyBinPath, cfg.GenesisLedgerFile)
	}

	preFork, postFork := partitionSubmissionsByCutover(submissions, *cfg.ForkCutoverTime)
	ctx.Log.Infof("Dual-verifier mode (cutover %v): %v pre-fork submissions, %v post-fork submissions",
		cfg.ForkCutoverTime.Format(time.RFC3339), len(preFork), len(postFork))

	var verifiedSubmissions []Submission
	var errs []error
	if len(preFork) > 0 {
		verified, err := ctx.verifyBatch(preFork, cfg.DelegationVerifyBinPath, cfg.GenesisLedgerFile)
		if err != nil {
			errs = append(errs, fmt.Errorf("pre-fork: %w", err))
		} else {
			verifiedSubmissions = append(verifiedSubmissions, verified...)
		}
	} else {
		ctx.Log.Info("No pre-fork submissions, skipping pre-fork verification run")
	}
	if len(postFork) > 0 {
		verified, err := ctx.verifyBatch(postFork, cfg.DelegationVerifyBinPathPostFork, cfg.GenesisLedgerFilePostFork)
		if err != nil {
			errs = append(errs, fmt.Errorf("post-fork: %w", err))
		} else {
			verifiedSubmissions = append(verifiedSubmissions, verified...)
		}
	} else {
		ctx.Log.Info("No post-fork submissions, skipping post-fork verification run")
	}

	return verifiedSubmissions, errors.Join(errs...)
}

// verifyBatch marshals the given submissions and runs them through the
// delegation verification binary at binPath with the given config file.
func (ctx *AppContext) verifyBatch(submissions []Submission, binPath, configFile string) ([]Submission, error) {
	submissionsJSON, err := json.Marshal(submissions)
	if err != nil {
		return nil, fmt.Errorf("error marshaling submissions to JSON: %w", err)
	}
	return ctx.runDelegationVerifyCommand(binPath, configFile, string(submissionsJSON))
}

// partitionSubmissionsByCutover splits submissions into pre-fork (submitted_at
// before the cutover) and post-fork (submitted_at at or after the cutover).
func partitionSubmissionsByCutover(submissions []Submission, cutover time.Time) (preFork, postFork []Submission) {
	for _, submission := range submissions {
		if submission.SubmittedAt.Before(cutover) {
			preFork = append(preFork, submission)
		} else {
			postFork = append(postFork, submission)
		}
	}
	return preFork, postFork
}

// sokMismatchError is the verifier's message for a snark-work sok-digest
// mismatch. The verifier has always enforced the binding; mainnet never saw
// it fail only because pre-3.4.0 daemons never route uptime snark work
// through the zkApp-segment path that stamps the default sok digest into the
// statement (MinaProtocol/mina#19299) - a path the released Mesa daemons all
// carry. The binding prevents fee/prover misattribution in the snark pool,
// where work is paid; uptime snark work is never pooled or paid, so the
// mismatch can be tolerated explicitly via TOLERATE_SOK_MISMATCH until the
// daemon fix (MinaProtocol/mina#19313) is deployed fleet-wide.
const sokMismatchError = "sok message digest does not match the sok message"

// tolerateSokMismatches counts submissions failing only the sok-digest check
// as valid: the block proof itself has still verified, and the mismatch is a
// known artifact of the released daemon fleet. Records with any other
// validation error are left untouched.
func (ctx *AppContext) tolerateSokMismatches(submissions []Submission) []Submission {
	tolerated := 0
	for i, submission := range submissions {
		if !strings.Contains(submission.ValidationError, sokMismatchError) {
			continue
		}
		ctx.Log.Infof("Tolerating sok-mismatch submission: submitter %s, block hash %s, original error: %s",
			submission.Submitter, submission.BlockHash, submission.ValidationError)
		submissions[i].Verified = true
		submissions[i].ValidationError = ""
		tolerated++
	}
	ctx.Log.Infof("Tolerated %d sok-mismatch submission(s) of %d", tolerated, len(submissions))
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
