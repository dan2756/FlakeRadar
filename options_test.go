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
		{"-j", "0", "true"},
		{"--timeout", "-1s", "true"},
		{"--timeout", "30", "true"},
		{"--show-output", "-1", "true"},
		{"--threshold", "101", "true"},
		{"--threshold", "-5", "true"},
		{"--threshold", "most", "true"},
		{"-q", "-v", "true"},
		{"--warmup", "-1", "true"},
		{"-n", "5", "--warmup", "5", "true"},
		{"-n", "-1", "--until-fail", "true"},
	} {
		if _, err := parseArgs(args); err == nil {
			t.Errorf("parseArgs(%q) succeeded, want error", args)
		}
	}
}

func TestParseArgsRuns(t *testing.T) {
	tests := []struct {
		args []string
		want int
	}{
		{[]string{"true"}, 10},
		{[]string{"-n", "3", "true"}, 3},
		{[]string{"--runs", "7", "true"}, 7},
		{[]string{"--until-fail", "true"}, 0}, // no limit
		{[]string{"--until-fail", "-n", "500", "true"}, 500},
		{[]string{"--until-fail", "-n", "0", "true"}, 0},
		{[]string{"--until-fail", "--warmup", "3", "true"}, 0},
	}
	for _, tt := range tests {
		o, err := parseArgs(tt.args)
		if err != nil {
			t.Errorf("parseArgs(%q): %v", tt.args, err)
			continue
		}
		if o.runs != tt.want {
			t.Errorf("parseArgs(%q).runs = %d, want %d", tt.args, o.runs, tt.want)
		}
	}
}
