package main

import (
	"bytes"
	"fmt"
	"io"
	"sync"

	"github.com/dan2756/flaky/internal/runner"
)

// lockedWriter serializes writes from runs that finish lines at the same
// time.
type lockedWriter struct {
	mu sync.Mutex
	w  io.Writer
}

func (l *lockedWriter) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.w.Write(p)
}

// prefixWriter passes output on a line at a time, each line starting with
// a prefix, so that concurrent runs streaming to the same place stay
// readable. A partial last line is held until it is finished or Close, or
// until it reaches maxLine bytes, which are then passed on as a line of
// their own.
type prefixWriter struct {
	w      io.Writer
	prefix []byte
	buf    []byte
}

// maxLine bounds how much of an unfinished line prefixWriter holds, so
// output with few or no newlines can't pile up in memory.
const maxLine = 64 << 10

func newPrefixWriter(w io.Writer, prefix string) *prefixWriter {
	return &prefixWriter{w: w, prefix: []byte(prefix)}
}

func (p *prefixWriter) Write(b []byte) (int, error) {
	n := len(b)
	for {
		i := bytes.IndexByte(b, '\n')
		if i < 0 {
			break
		}
		p.emit(append(p.buf, b[:i+1]...))
		p.buf = p.buf[:0]
		b = b[i+1:]
	}
	p.buf = append(p.buf, b...)
	for len(p.buf) >= maxLine {
		p.emit(append(p.buf[:maxLine:maxLine], '\n'))
		p.buf = p.buf[maxLine:]
	}
	// Errors writing the copy are not the command's problem; never let
	// them fail the run.
	return n, nil
}

func (p *prefixWriter) Close() error {
	if len(p.buf) > 0 {
		p.emit(append(p.buf, '\n'))
		p.buf = nil
	}
	return nil
}

func (p *prefixWriter) emit(line []byte) {
	out := make([]byte, 0, len(p.prefix)+len(line))
	out = append(out, p.prefix...)
	out = append(out, line...)
	p.w.Write(out)
}

// lastLines returns the last n lines of out, or all of them if n is 0,
// along with the number of lines out has in total.
func lastLines(out []byte, n int) (tail []byte, total int) {
	out = bytes.TrimSuffix(out, []byte("\n"))
	if len(out) == 0 {
		return nil, 0
	}
	total = bytes.Count(out, []byte("\n")) + 1
	if n <= 0 || n >= total {
		return out, total
	}
	i := len(out)
	for range n {
		i = bytes.LastIndexByte(out[:i], '\n')
	}
	return out[i+1:], total
}

// writeOutput shows what a failed run printed, cut to its last maxLines
// lines (0 means no limit).
func writeOutput(w io.Writer, r runner.Result, maxLines int) {
	tail, total := lastLines(r.Output, maxLines)
	if total == 0 {
		fmt.Fprintf(w, "--- run %d (%s) printed nothing ---\n\n", r.Index, status(r))
		return
	}
	header := fmt.Sprintf("--- output of run %d (%s)", r.Index, status(r))
	switch shown := bytes.Count(tail, []byte("\n")) + 1; {
	case r.OutputDropped > 0:
		// Only the end of the output was kept, so the total is unknown.
		header += fmt.Sprintf(", last %d lines; earlier output was dropped", shown)
	case shown < total:
		header += fmt.Sprintf(", last %d of %d lines", shown, total)
	}
	fmt.Fprintf(w, "%s ---\n%s\n\n", header, tail)
}
