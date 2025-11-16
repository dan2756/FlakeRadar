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
	total int // 0 if there is no fixed number of runs
	width int // terminal columns
	done  int
	marks []byte // the most recent marks, no more than fit on a line
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
	p.done++
	p.marks = append(p.marks, mark)
	if len(p.marks) > p.width {
		p.marks = append(p.marks[:0], p.marks[len(p.marks)-p.width:]...)
	}
	p.draw()
}

func (p *progress) draw() {
	counter := fmt.Sprintf("  %d", p.done)
	if p.total > 0 {
		counter += fmt.Sprintf("/%d", p.total)
	}
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
