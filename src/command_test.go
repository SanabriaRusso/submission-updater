package main

import (
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
