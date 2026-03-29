package runner

import (
	"bytes"
	"strings"
	"testing"
)

func TestTailBuffer(t *testing.T) {
	b := newTailBuffer(10)
	b.Write([]byte("abc\n"))
	if got := string(b.Bytes()); got != "abc\n" || b.dropped != 0 {
		t.Errorf("under the limit: %q, dropped %d", got, b.dropped)
	}

	for _, line := range []string{"one\n", "two\n", "three\n", "four\n"} {
		b.Write([]byte(line))
	}
	// 23 bytes written; the last 10 are "hree\nfour\n", and the partial
	// "hree" line goes too.
	if got := string(b.Bytes()); got != "four\n" || b.dropped != 18 {
		t.Errorf("over the limit: %q, dropped %d; want %q, 18", got, b.dropped, "four\n")
	}
}

func TestTailBufferBigWrite(t *testing.T) {
	b := newTailBuffer(1000)
	data := bytes.Repeat([]byte("0123456789abcdef\n"), 1000) // 17000 bytes
	b.Write(data)
	got := b.Bytes()
	if len(got) > 1000 || !bytes.HasSuffix(data, got) || !strings.HasPrefix(string(got), "0123") {
		t.Errorf("kept %d bytes starting %q", len(got), got[:min(len(got), 10)])
	}
	if b.dropped+int64(len(got)) != int64(len(data)) {
		t.Errorf("dropped %d + kept %d != %d written", b.dropped, len(got), len(data))
	}
}

func TestTailBufferManySmallWrites(t *testing.T) {
	b := newTailBuffer(64)
	for range 10000 {
		b.Write([]byte("x"))
		if cap(b.buf) > 4*64 {
			t.Fatalf("buffer grew to %d bytes for a 64 byte limit", cap(b.buf))
		}
	}
	if got := b.Bytes(); len(got) != 64 || b.dropped != 10000-64 {
		t.Errorf("kept %d, dropped %d", len(got), b.dropped)
	}
}

func TestCollectorKeepsLowestFailure(t *testing.T) {
	out := func(s string) []byte { return []byte(s) }
	c := newCollector()
	// As runs might finish with -j: 3 fails, 1 passes, 2 fails, 5 fails.
	c.add(Result{Index: 3, ExitCode: 1, Output: out("three"), OutputDropped: 7})
	c.add(Result{Index: 1, Output: out("one")})
	c.add(Result{Index: 2, ExitCode: 1, Output: out("two")})
	c.add(Result{Index: 5, ExitCode: 1, Output: out("five")})

	want := map[int]string{2: "two"}
	for _, r := range c.results {
		if string(r.Output) != want[r.Index] {
			t.Errorf("run %d kept %q, want %q", r.Index, r.Output, want[r.Index])
		}
		if r.Index == 3 && r.OutputDropped != 0 {
			t.Errorf("run 3 still claims %d dropped bytes", r.OutputDropped)
		}
	}
	if len(c.results) != 4 {
		t.Errorf("got %d results, want 4", len(c.results))
	}
}
