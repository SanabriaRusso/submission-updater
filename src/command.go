package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
	"time"
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

	out, err := runCommand(cmd, input)
	if err != nil {
		return nil, fmt.Errorf("error running %v: %w", command, err)
	}

	submissions, err := parseDelegationVerifyOutput(out)
	if err != nil {
		return nil, fmt.Errorf("error parsing submissions: %w", err)
	}

	return submissions, nil
}

// verifySubmissions marshals the given submissions and runs them through the
// delegation verification binary at binPath with the given config file.
func (ctx *AppContext) verifySubmissions(submissions []Submission, binPath, configFile string) ([]Submission, error) {
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

// runDualDelegationVerify partitions submissions at the fork cutover time and
// runs each non-empty partition through its corresponding delegation-verify
// binary (pre-fork or post-fork), returning the merged results.
func (ctx *AppContext) runDualDelegationVerify(submissions []Submission) ([]Submission, error) {
	cfg := ctx.AppConfig
	preFork, postFork := partitionSubmissionsByCutover(submissions, *cfg.ForkCutoverTime)
	ctx.Log.Infof("Dual-verifier mode (cutover %v): %v pre-fork submissions, %v post-fork submissions",
		cfg.ForkCutoverTime.Format(time.RFC3339), len(preFork), len(postFork))

	var verifiedSubmissions []Submission
	if len(preFork) > 0 {
		verified, err := ctx.verifySubmissions(preFork, cfg.DelegationVerifyBinPath, cfg.GenesisLedgerFile)
		if err != nil {
			return nil, fmt.Errorf("error verifying pre-fork submissions: %w", err)
		}
		verifiedSubmissions = append(verifiedSubmissions, verified...)
	} else {
		ctx.Log.Info("No pre-fork submissions, skipping pre-fork verification run")
	}
	if len(postFork) > 0 {
		verified, err := ctx.verifySubmissions(postFork, cfg.DelegationVerifyBinPathPostFork, cfg.GenesisLedgerFilePostFork)
		if err != nil {
			return nil, fmt.Errorf("error verifying post-fork submissions: %w", err)
		}
		verifiedSubmissions = append(verifiedSubmissions, verified...)
	} else {
		ctx.Log.Info("No post-fork submissions, skipping post-fork verification run")
	}

	return verifiedSubmissions, nil
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

func runCommand(command, input string) (string, error) {
	cmdParts := strings.Split(command, " ")
	cmd := exec.Command(cmdParts[0], cmdParts[1:]...)

	cmd.Stdin = bytes.NewBufferString(input)

	// Run the command and capture its standard output.
	var stdout bytes.Buffer
	cmd.Stdout = &stdout

	err := cmd.Run()
	if err != nil {
		return "", fmt.Errorf("failed to run command: %w", err)
	}

	return stdout.String(), nil
}
