package main

import (
	"bytes"
	"strings"
	"testing"

	"github.com/dan2756/flaky/internal/runner"
)

func TestLastLines(t *testing.T) {
	tests := []struct {
		out       string
		n         int
		want      string
		wantTotal int
	}{
		{"", 5, "", 0},
		{"\n", 5, "", 0},
		{"a\nb\nc\n", 2, "b\nc", 3},
		{"a\nb\nc", 2, "b\nc", 3},
		{"a\nb\nc\n", 3, "a\nb\nc", 3},
		{"a\nb\nc\n", 10, "a\nb\nc", 3},
		{"a\nb\nc\n", 0, "a\nb\nc", 3},
		{"a\n\nc\n", 1, "c", 3},
		{"one line", 1, "one line", 1},
	}
	for _, tt := range tests {
		got, total := lastLines([]byte(tt.out), tt.n)
		if string(got) != tt.want || total != tt.wantTotal {
			t.Errorf("lastLines(%q, %d) = %q, %d; want %q, %d", tt.out, tt.n, got, total, tt.want, tt.wantTotal)
		}
	}
}

func TestPrefixWriter(t *testing.T) {
	var buf bytes.Buffer
	w := newPrefixWriter(&buf, "[3] ")
	for _, chunk := range []string{"hel", "lo\nwor", "ld\n", "\npartial"} {
		w.Write([]byte(chunk))
	}
	if got, want := buf.String(), "[3] hello\n[3] world\n[3] \n"; got != want {
		t.Errorf("before Close: %q, want %q", got, want)
	}
	w.Close()
	if got, want := buf.String(), "[3] hello\n[3] world\n[3] \n[3] partial\n"; got != want {
		t.Errorf("after Close: %q, want %q", got, want)
	}
}

func TestWriteOutput(t *testing.T) {
	var lines []string
	for i := 1; i <= 8; i++ {
		lines = append(lines, "line "+string(rune('0'+i)))
	}
	r := runner.Result{Index: 4, ExitCode: 1, Output: []byte(strings.Join(lines, "\n") + "\n")}

	var buf bytes.Buffer
	writeOutput(&buf, r, 3)
	want := "--- output of run 4 (exit 1), last 3 of 8 lines ---\nline 6\nline 7\nline 8\n\n"
	if buf.String() != want {
		t.Errorf("writeOutput = %q, want %q", buf.String(), want)
	}

	buf.Reset()
	writeOutput(&buf, runner.Result{Index: 2, TimedOut: true, ExitCode: -1}, 3)
	if want := "--- run 2 (timeout) printed nothing ---\n\n"; buf.String() != want {
		t.Errorf("writeOutput with no output = %q, want %q", buf.String(), want)
	}
}

func TestProgress(t *testing.T) {
	var buf bytes.Buffer
	p := newProgress(&buf, 3, 80)
	p.add(runner.Result{Index: 1})
	p.add(runner.Result{Index: 2, ExitCode: 1})
	p.add(runner.Result{Index: 3})
	p.finish()

	frames := strings.Split(strings.TrimSuffix(buf.String(), "\n"), "\r")[1:]
	want := []string{"  0/3", ".  1/3", ".F  2/3", ".F.  3/3"}
	if len(frames) != len(want) {
		t.Fatalf("got frames %q, want %d", frames, len(want))
	}
	for i, f := range frames {
		if got := strings.TrimSuffix(f, "\x1b[K"); got != want[i] {
			t.Errorf("frame %d = %q, want %q", i, got, want[i])
		}
	}
}

func TestProgressNarrowTerminal(t *testing.T) {
	var buf bytes.Buffer
	p := newProgress(&buf, 100, 20)
	for i := 1; i <= 30; i++ {
		p.add(runner.Result{Index: i})
	}
	frames := strings.Split(buf.String(), "\r")
	last := strings.TrimSuffix(frames[len(frames)-1], "\x1b[K")
	if len(last) >= 20 {
		t.Errorf("last frame %q is %d columns, want it to fit in 20", last, len(last))
	}
	if !strings.HasSuffix(last, "  30/100") {
		t.Errorf("last frame %q lost its counter", last)
	}
}
