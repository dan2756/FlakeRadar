package main

import (
	"fmt"
	"io"

	"github.com/dan2756/flaky/internal/runner"
)

// progress draws one mark per finished run, "." for a pass and "F" for a
// failure, followed by a run counter, redrawing the line in place. It is
// only used when stderr is a terminal.
type progress struct {
	w     io.Writer
	total int
	width int // terminal columns
	marks []byte
}

func newProgress(w io.Writer, total, width int) *progress {
	if width <= 0 {
		width = 80
	}
	p := &progress{w: w, total: total, width: width}
	p.draw()
	return p
}

func (p *progress) add(r runner.Result) {
	mark := byte('.')
	if !r.Passed() {
		mark = 'F'
	}
	p.marks = append(p.marks, mark)
	p.draw()
}

func (p *progress) draw() {
	counter := fmt.Sprintf("  %d/%d", len(p.marks), p.total)
	marks := p.marks
	// Keep clear of the last column so the terminal doesn't wrap, and
	// show the most recent marks once they no longer fit.
	if room := p.width - len(counter) - 1; len(marks) > room {
		marks = marks[len(marks)-max(room, 0):]
	}
	fmt.Fprintf(p.w, "\r%s%s\x1b[K", marks, counter)
}

// finish leaves the final line on screen and moves past it.
func (p *progress) finish() {
	fmt.Fprintln(p.w)
}
