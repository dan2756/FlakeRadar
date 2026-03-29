package runner

import "bytes"

// maxOutput is how much of a run's output is kept: the last 1 MiB. That is
// plenty to see why a run failed, and it keeps go test -v at -j 16 from
// holding gigabytes.
const maxOutput = 1 << 20

// tailBuffer is an io.Writer that keeps only the last max bytes written.
type tailBuffer struct {
	max     int
	buf     []byte
	dropped int64 // bytes discarded from the front
}

func newTailBuffer(max int) *tailBuffer {
	return &tailBuffer{max: max}
}

func (t *tailBuffer) Write(p []byte) (int, error) {
	t.buf = append(t.buf, p...)
	// Let the buffer run to twice the limit before trimming, so a stream
	// of small writes doesn't move the whole buffer every time.
	if len(t.buf) > 2*t.max {
		t.trim()
	}
	return len(p), nil
}

func (t *tailBuffer) trim() {
	if over := len(t.buf) - t.max; over > 0 {
		t.dropped += int64(over)
		t.buf = append(t.buf[:0], t.buf[over:]...)
	}
}

// Bytes returns what is kept. If anything was dropped, it starts at the
// first complete line, and the partial line is counted as dropped too.
func (t *tailBuffer) Bytes() []byte {
	t.trim()
	if t.dropped > 0 {
		if i := bytes.IndexByte(t.buf, '\n'); i >= 0 {
			t.dropped += int64(i + 1)
			t.buf = t.buf[i+1:]
		}
	}
	return t.buf
}

// collector gathers results as runs finish. To keep memory bounded over
// thousands of runs, it holds on to output only for the lowest-numbered
// failed run seen so far.
type collector struct {
	results []Result
	kept    int // position in results of the failure whose output is kept, or -1
}

func newCollector() *collector {
	return &collector{kept: -1}
}

// add records r, minus its output unless it is now the first failure, and
// returns what was recorded.
func (c *collector) add(r Result) Result {
	switch {
	case r.Passed():
		r = withoutOutput(r)
	case c.kept < 0 || r.Index < c.results[c.kept].Index:
		if c.kept >= 0 {
			c.results[c.kept] = withoutOutput(c.results[c.kept])
		}
		c.kept = len(c.results)
	default:
		r = withoutOutput(r)
	}
	c.results = append(c.results, r)
	return r
}

func withoutOutput(r Result) Result {
	r.Output, r.OutputDropped = nil, 0
	return r
}
