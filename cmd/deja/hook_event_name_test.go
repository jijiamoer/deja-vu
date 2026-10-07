package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/vshulcz/deja-vu/internal/index"
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
// Devin, and keep the SessionStart name for everybody else. The index holds
// a real session so every run produces a reply — an empty one fails loud,
// because the echo is exactly what this test exists to see.
func TestHookContextEchoesDevinEventOnly(t *testing.T) {
	tmp := hermeticEnv(t)
	root := filepath.Join(tmp, "claude")
	t.Setenv("DEJA_CLAUDE_ROOT", root)
	dir := filepath.Join(root, "-work-api")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	at := time.Now().Add(-2 * time.Hour).UTC().Format(time.RFC3339)
	line := fmt.Sprintf(`{"type":"user","sessionId":"mine","timestamp":%q,"cwd":"/work/api",`+
		`"message":{"role":"user","content":"the ledger cutover retry budget"}}`, at)
	if err := os.WriteFile(filepath.Join(dir, "mine.jsonl"), []byte(line+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	indexDir := index.DefaultDir()
	if err := index.Ensure(indexDir, "", true, nil); err != nil {
		t.Fatal(err)
	}

	for _, c := range []struct {
		name    string
		payload string
		want    string
	}{
		{"claude canonical", `{"hook_event_name":"SessionStart","session_id":"s1","cwd":"/work/api"}`, "SessionStart"},
		{"devin compaction", `{"hook_event_name":"PostCompaction","session_id":"s1","cwd":"/work/api"}`, "PostCompaction"},
		{"cursor camelCase", `{"hook_event_name":"sessionStart","session_id":"s1","cwd":"/work/api"}`, "SessionStart"},
		{"gemini dialect", `{"hook_event_name":"BeforeAgent","session_id":"s1","cwd":"/work/api"}`, "SessionStart"},
		{"no event named", `{"session_id":"s1","cwd":"/work/api"}`, "SessionStart"},
	} {
		out := hookContextFor(t, indexDir, c.payload)
		if out == "" {
			t.Fatalf("%s: the hook stayed silent with an indexed session in scope — the echo went unobserved", c.name)
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
// hosts that spell the event any other way — again with a command history
// seeded so silence cannot masquerade as a pass.
func TestHookToolEchoesDevinPostToolUseOnly(t *testing.T) {
	t.Setenv("DEJA_COMMAND_HINTS", "on")
	tmp := hermeticEnv(t)
	t.Setenv("DEJA_INDEX_DIR", filepath.Join(tmp, "index.db"))
	commandRunWithAnOutcome(t, "go test ./... -count=1", "a", "b")
	if _, err := captureRun(t, "index"); err != nil {
		t.Fatal(err)
	}

	for _, c := range []struct {
		name    string
		payload string
		want    string
	}{
		{"devin", `{"hook_event_name":"PostToolUse","tool_name":"Bash","tool_input":{"command":"go test ./... -count=1"},"session_id":"now-devin","cwd":"/work/app"}`, "PostToolUse"},
		{"claude", `{"hook_event_name":"PreToolUse","tool_name":"Bash","tool_input":{"command":"go test ./... -count=1"},"session_id":"now-claude","cwd":"/work/app"}`, "PreToolUse"},
		{"cursor camelCase", `{"hook_event_name":"postToolUse","tool_name":"Bash","tool_input":{"command":"go test ./... -count=1"},"session_id":"now-cursor","cwd":"/work/app"}`, "PreToolUse"},
	} {
		out := toolHookRun(t, c.payload)
		if out == "" {
			t.Fatalf("%s: a command run in two sessions produced no line — the echo went unobserved", c.name)
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
