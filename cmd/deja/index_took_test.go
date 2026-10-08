package main

import (
	"strings"
	"testing"
	"time"

	"github.com/vshulcz/deja-vu/internal/index"
)

// The index summary says how long the build took (#4630), at a precision that
// reads naturally for the size of the number.
func TestTookSuffix(t *testing.T) {
	for _, tc := range []struct {
		d    time.Duration
		want string
	}{
		{0, ""},
		{-time.Second, ""},
		{150 * time.Millisecond, " in 0.15s"},
		{3200 * time.Millisecond, " in 3.2s"},
		{42 * time.Second, " in 42s"},
		{125 * time.Second, " in 2m05s"},
	} {
		if got := tookSuffix(tc.d); got != tc.want {
			t.Errorf("tookSuffix(%v) = %q, want %q", tc.d, got, tc.want)
		}
	}
}

// The first-index greeting states the time when the build was timed, and
// claims none when it was not.
func TestFirstIndexInfoSaysHowLongItTook(t *testing.T) {
	b := index.BuildSummary{Initial: true, Sessions: 3, Messages: 12, Harnesses: 1,
		PerHarness: []index.HarnessCount{{Name: "claude", Sessions: 3, Messages: 12}}}

	timed := b
	timed.Took = 150 * time.Millisecond
	if got := strings.Join(firstIndexInfo(timed, ""), "\n"); !strings.Contains(got, "agent in 0.15s") {
		t.Errorf("timed build's summary does not say how long it took:\n%s", got)
	}
	if got := strings.Join(firstIndexInfo(b, ""), "\n"); strings.Contains(got, " in 0") {
		t.Errorf("untimed build's summary claims a duration:\n%s", got)
	}
}

// Every build that runs through withBuildProgress is timed, including the
// non-terminal path it returns early on: the first `deja search` or
// `deja warmup` builds the index there, and its greeting needs the time too.
func TestWithBuildProgressTimesEveryBuild(t *testing.T) {
	saved := index.LastBuild
	t.Cleanup(func() { index.LastBuild = saved })
	for _, sentinel := range []string{"", "1"} {
		t.Setenv("DEJA_WARMUP_SENTINEL", sentinel)
		index.LastBuild = index.BuildSummary{}
		err := withBuildProgress(func() error {
			time.Sleep(5 * time.Millisecond)
			index.LastBuild = index.BuildSummary{Initial: true, Messages: 1}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
		if index.LastBuild.Took < 5*time.Millisecond {
			t.Errorf("sentinel %q: Took = %v, want the build's wall time", sentinel, index.LastBuild.Took)
		}
	}
}
