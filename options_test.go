package main

import (
	"slices"
	"testing"
)

func TestCommandArgv(t *testing.T) {
	tests := []struct {
		args        []string
		wantArgv    []string
		wantDisplay string
	}{
		{
			args:        []string{"go", "test", "./pkg/..."},
			wantArgv:    []string{"go", "test", "./pkg/..."},
			wantDisplay: "go test ./pkg/...",
		},
		{
			args:        []string{"./script.sh"},
			wantArgv:    []string{"./script.sh"},
			wantDisplay: "./script.sh",
		},
		{
			args:        []string{"go test ./... | tee log"},
			wantArgv:    []string{"sh", "-c", "go test ./... | tee log"},
			wantDisplay: "go test ./... | tee log",
		},
		{
			args:        []string{"go", "test", "-run", "TestFoo.*", "./..."},
			wantArgv:    []string{"go", "test", "-run", "TestFoo.*", "./..."},
			wantDisplay: "go test -run 'TestFoo.*' ./...",
		},
		{
			args:        []string{"echo", "it's", ""},
			wantArgv:    []string{"echo", "it's", ""},
			wantDisplay: `echo 'it'\''s' ''`,
		},
	}
	for _, tt := range tests {
		argv, display := commandArgv(tt.args)
		if !slices.Equal(argv, tt.wantArgv) {
			t.Errorf("commandArgv(%q) argv = %q, want %q", tt.args, argv, tt.wantArgv)
		}
		if display != tt.wantDisplay {
			t.Errorf("commandArgv(%q) display = %q, want %q", tt.args, display, tt.wantDisplay)
		}
	}
}

func TestParseArgsErrors(t *testing.T) {
	for _, args := range [][]string{
		{},
		{"-n", "0", "true"},
		{"-n", "x", "true"},
		{"--nope", "true"},
	} {
		if _, err := parseArgs(args); err == nil {
			t.Errorf("parseArgs(%q) succeeded, want error", args)
		}
	}
}
