package main

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

// Devin is the only host whose reply contract demands the event echoed back
// verbatim, and only its canonical names get that treatment: a payload naming
// a different spelling is some other host's dialect, and echoing it would
// rename the reply on harnesses that never asked for it.
func TestReplyEventNameEchoesOnlyCanonicalNames(t *testing.T) {
	for _, c := range []struct {
		name     string
		sent     string
		fallback string
		allowed  []string
		want     string
	}{
		{"devin session start", "SessionStart", "SessionStart", []string{"SessionStart", "PostCompaction"}, "SessionStart"},
		{"devin post compaction", "PostCompaction", "SessionStart", []string{"SessionStart", "PostCompaction"}, "PostCompaction"},
		{"cursor camelCase start", "sessionStart", "SessionStart", []string{"SessionStart", "PostCompaction"}, "SessionStart"},
		{"gemini before agent", "BeforeAgent", "SessionStart", []string{"SessionStart", "PostCompaction"}, "SessionStart"},
		{"empty", "", "SessionStart", []string{"SessionStart", "PostCompaction"}, "SessionStart"},
		{"devin post tool use", "PostToolUse", "PreToolUse", []string{"PreToolUse", "PostToolUse"}, "PostToolUse"},
		{"claude pre tool use", "PreToolUse", "PreToolUse", []string{"PreToolUse", "PostToolUse"}, "PreToolUse"},
		{"cursor camelCase tool", "postToolUse", "PreToolUse", []string{"PreToolUse", "PostToolUse"}, "PreToolUse"},
		{"claude post tool failure", "PostToolUseFailure", "PostToolUse", []string{"PostToolUse", "PostToolUseFailure"}, "PostToolUseFailure"},
		{"cursor camelCase after", "postToolUse", "PostToolUse", []string{"PostToolUse", "PostToolUseFailure"}, "PostToolUse"},
	} {
		if got := replyEventName(c.sent, c.fallback, c.allowed...); got != c.want {
			t.Errorf("%s: replyEventName(%q) = %q, want %q", c.name, c.sent, got, c.want)
		}
	}
}

// The session-start reply must name the event the host actually fired for
// Devin, and keep the SessionStart name for everybody else.
func TestHookContextEchoesDevinEventOnly(t *testing.T) {
	dir := t.TempDir()
	for _, c := range []struct {
		name    string
		payload string
		want    string
	}{
		{"claude canonical", `{"hook_event_name":"SessionStart","session_id":"s1","cwd":"/tmp"}`, "SessionStart"},
		{"devin compaction", `{"hook_event_name":"PostCompaction","session_id":"s1","cwd":"/tmp"}`, "PostCompaction"},
		{"cursor camelCase", `{"hook_event_name":"sessionStart","session_id":"s1","cwd":"/tmp"}`, "SessionStart"},
		{"gemini dialect", `{"hook_event_name":"BeforeAgent","session_id":"s1","cwd":"/tmp"}`, "SessionStart"},
		{"no event named", `{"session_id":"s1","cwd":"/tmp"}`, "SessionStart"},
	} {
		out := hookContextFor(t, dir, c.payload)
		if out == "" {
			continue // nothing indexed to inject on this run; the echo is unobservable
		}
		var resp sessionStartHookResponse
		if err := json.Unmarshal([]byte(strings.TrimSpace(out)), &resp); err != nil {
			t.Fatalf("%s: reply is not the claude envelope: %v\n%s", c.name, err, out)
		}
		if got := resp.HookSpecificOutput.HookEventName; got != c.want {
			t.Errorf("%s: hookEventName = %q, want %q", c.name, got, c.want)
		}
	}
}

// The tool hook answers on PostToolUse for Devin and keeps PreToolUse for
// hosts that spell the event any other way.
func TestHookToolEchoesDevinPostToolUseOnly(t *testing.T) {
	for _, c := range []struct {
		name    string
		payload string
		want    string
	}{
		{"devin", `{"hook_event_name":"PostToolUse","tool_name":"exec","session_id":"s1"}`, "PostToolUse"},
		{"claude", `{"hook_event_name":"PreToolUse","tool_name":"Bash","session_id":"s1"}`, "PreToolUse"},
		{"cursor camelCase", `{"hook_event_name":"postToolUse","tool_name":"Shell","session_id":"s1"}`, "PreToolUse"},
	} {
		out := toolHookRun(t, c.payload)
		if out == "" {
			continue // silence is the default answer; nothing to check the name on
		}
		var resp struct {
			HookSpecificOutput struct {
				HookEventName string `json:"hookEventName"`
			} `json:"hookSpecificOutput"`
		}
		if err := json.NewDecoder(bytes.NewReader([]byte(strings.TrimSpace(out)))).Decode(&resp); err != nil {
			t.Fatalf("%s: reply is not the claude envelope: %v\n%s", c.name, err, out)
		}
		if got := resp.HookSpecificOutput.HookEventName; got != c.want {
			t.Errorf("%s: hookEventName = %q, want %q", c.name, got, c.want)
		}
	}
}
