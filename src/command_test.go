package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
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
