package main

import (
	"io"
	"os"
	"testing"
)

// The counter `deja search` prints through must say what it wraps, or the
// printer cannot tell it is writing to a terminal (#4620).
func TestCountingWriterUnwrapsToTheTerminal(t *testing.T) {
	c := &countingWriter{w: os.Stdout}
	var u interface{ Unwrap() io.Writer } = c
	if u.Unwrap() != os.Stdout {
		t.Fatalf("countingWriter unwraps to %v, want os.Stdout", u.Unwrap())
	}
}
