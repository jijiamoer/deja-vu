package search

import (
	"bytes"
	"io"
	"os"
	"testing"
)

// The search output honoured NO_COLOR and ignored TERM=dumb, so a terminal
// that cannot render escapes got them anyway (#903).
func TestColorOKHonoursDumbTerminals(t *testing.T) {
	f, err := os.Open(os.DevNull)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = f.Close() })

	t.Setenv("NO_COLOR", "")
	t.Setenv("TERM", "xterm-256color")
	if !colorOK(f) {
		t.Fatal("a character device with a real TERM lost its colour")
	}
	t.Setenv("TERM", "dumb")
	if colorOK(f) {
		t.Error("TERM=dumb still coloured")
	}
	// And the older half of the rule, which nothing covered: removing it
	// passed the whole suite.
	t.Setenv("TERM", "xterm-256color")
	t.Setenv("NO_COLOR", "1")
	if colorOK(f) {
		t.Error("NO_COLOR stopped being honoured")
	}
}

type wrapped struct{ w io.Writer }

func (x wrapped) Write(p []byte) (int, error) { return x.w.Write(p) }
func (x wrapped) Unwrap() io.Writer           { return x.w }

// `deja search` prints through a counter so the log records what went out,
// and colorOK saw the counter rather than the terminal under it: every result
// came out plain in a terminal (#4620). A wrapper that says what it wraps is
// looked through; one that does not, or a buffer, still gets plain text.
func TestColorOKLooksThroughAWrapper(t *testing.T) {
	f, err := os.Open(os.DevNull)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = f.Close() })
	t.Setenv("NO_COLOR", "")
	t.Setenv("TERM", "xterm-256color")

	if !colorOK(wrapped{w: wrapped{w: f}}) {
		t.Error("a terminal behind two wrappers lost its colour")
	}
	if colorOK(wrapped{w: &bytes.Buffer{}}) {
		t.Error("a buffer behind a wrapper was coloured")
	}
	t.Setenv("NO_COLOR", "1")
	if colorOK(wrapped{w: f}) {
		t.Error("NO_COLOR was not honoured through a wrapper")
	}
}
