package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/vshulcz/deja-vu/internal/sources"
)

// Kiro takes MCP servers in a global settings file, the same `mcpServers`
// shape every other client here uses:
//
//	~/.kiro/settings/mcp.json
//
// (kiro.dev's own docs and `kiro-cli mcp add --scope global`, which writes that
// file). The CLI and the IDE read the same one, which is why this is a single
// target rather than one per client the way the reader has two (#3651).
//
// One thing a user has to know and the installer cannot do for them: a custom
// agent — `~/.kiro/agents/<name>.json` — reads this file only when it sets
// `"includeMcpJson": true`. The note says so rather than leaving a reader
// wondering why recall is missing in their agent; the agent kiro-auto writes
// sets it.
func kiroMCPSettingsPath() string {
	return filepath.Join(sources.KiroConfigDir(), "settings", "mcp.json")
}

const kiroAgentNote = "a custom agent in ~/.kiro/agents/*.json reads the global MCP servers only with " +
	"`\"includeMcpJson\": true` in it"

// kiroSteeringPath is Kiro's user-level guidance channel: `~/.kiro/steering`
// is the global half of steering, loaded for every project, and kiro-cli scans
// it alongside the workspace one.
//
// It is not a skill, and the difference matters for what goes in it. Steering
// documents declare an inclusion mode, and the only mode measured as actually
// loaded by kiro-cli is `always` — `manual` is not loaded and cannot be invoked
// from a session, `fileMatch` was withheld (KiroCrew's steering reference,
// measured against kiro-cli 2.19.1). So this text rides in front of every turn
// whether it is wanted or not, which is why it is four lines naming the tool
// rather than the full skill deja writes where a skill is loaded on demand.
func kiroSteeringPath() string {
	return filepath.Join(sources.KiroConfigDir(), "steering", "deja.md")
}

func kiroSteering(exe string) string {
	return fmt.Sprintf(`---
inclusion: always
---

# Past sessions are searchable

This machine indexes every coding session it has, across agents, with deja-vu.
Before debugging an error or re-implementing something, call the deja tool with
mode recall and the user's own words — the specific tokens win.
Before your first edit in a task, and again before you call a change done or
ready to merge, call deja with mode recall and the task's key nouns (file,
package, feature, setting). A rule, a rejected option or a check it returns
outranks your defaults: follow it and say so in one line.
Outside a session: %s search -- "<query>".
`, exe)
}

func installKiro(exe string, uninstall bool) (installResult, error) {
	res, err := installMCPJSON(kiroMCPSettingsPath(), exe, uninstall)
	if err != nil {
		return res, err
	}
	steering, err := installTextFile(kiroSteeringPath(), kiroSteering(exe), uninstall)
	if err != nil {
		return installResult{}, err
	}
	if uninstall {
		return wroteAll(res, steering), nil
	}
	out := wroteAll(res, steering)
	out.Note = joinNotes(out.Note, kiroAgentNote)
	return out, nil
}

// kiro-cli runs the hooks of the agent a chat starts in: an agent file in
// `~/.kiro/agents` takes agentSpawn, userPromptSubmit, preToolUse, postToolUse
// and stop. Measured on 2.22.0, what an agentSpawn or userPromptSubmit hook
// prints goes in front of the model, the first for the whole conversation;
// what preToolUse and postToolUse print does not, so there is no pre-edit
// line here (#4304), and a failed command's fix pair goes out with the next
// prompt instead.
//
// The hooks go in an agent of deja's own rather than into the reader's: the
// built-in kiro_default takes no hooks from a file (a kiro_default.json beside
// it is ignored), and deja does not switch `chat.defaultAgent` for them, since
// that would trade the default agent's prompt for this one. So the agent runs
// when a chat is started in it, and doctor says which of the two it is.
func kiroAgentPath() string {
	return filepath.Join(sources.KiroConfigDir(), "agents", "deja.json")
}

const kiroAgentDescription = "Kiro's tools with deja-vu recall — written by deja install kiro-auto"

const kiroAgentHookTimeoutMs = 10000

func kiroAgentJSON(exe string) (string, error) {
	hook := func(args ...string) []map[string]any {
		return []map[string]any{{"command": hookRun(exe, args...), "timeout_ms": kiroAgentHookTimeoutMs}}
	}
	b, err := json.MarshalIndent(map[string]any{
		"name":           "deja",
		"description":    kiroAgentDescription,
		"tools":          []string{"*"},
		"includeMcpJson": true,
		"hooks": map[string]any{
			"agentSpawn":       hook("hook-context", "--plain"),
			"userPromptSubmit": hook("hook-prompt", "--plain"),
			// What postToolUse prints never reaches the model, so a failed
			// command's fix pair waits for the next userPromptSubmit
			// (hook_deferred.go).
			"postToolUse": hook("hook-tool-after", "--defer"),
		},
	}, "", "  ")
	return string(b) + "\n", err
}

const kiroAutoNote = "kiro-cli runs these hooks in the deja agent: `kiro-cli chat --agent deja`, " +
	"or `kiro-cli agent set-default deja` for every chat"

func installKiroAuto(exe string, uninstall bool) (installResult, error) {
	// The server and steering first: an agent of the reader's own named deja
	// costs only the agent file, not what `deja install kiro` gives them.
	base, err := installKiro(exe, uninstall)
	if err != nil {
		return base, err
	}
	path := kiroAgentPath()
	if b, err := os.ReadFile(path); err == nil && !strings.Contains(string(b), kiroAgentDescription) {
		if !uninstall {
			base.Note = joinNotes(base.Note, reportPath(path)+" is an agent deja did not write, so it was left as it is — rename it and run this again for the recall hooks")
		}
		return base, nil
	}
	body, err := kiroAgentJSON(hookExeFor(exe, uninstall))
	if err != nil {
		return installResult{}, err
	}
	agent, err := installTextFile(path, body, uninstall)
	if err != nil {
		return installResult{}, err
	}
	if uninstall && agent.Action == "removed" {
		if err := kiroForgetDefaultAgent(); err != nil {
			return installResult{}, err
		}
	}
	out := wroteAll(base, agent)
	if !uninstall {
		out.Note = joinNotes(out.Note, kiroAutoNote)
	}
	return out, nil
}

// kiroSettingsPath is kiro-cli's own settings file, where
// `kiro-cli agent set-default` writes `chat.defaultAgent`.
func kiroSettingsPath() string {
	return filepath.Join(sources.KiroConfigDir(), "settings", "cli.json")
}

// kiroDefaultAgent is the agent a plain `kiro-cli chat` starts in, "" for the
// built-in one.
func kiroDefaultAgent() string {
	name, _ := readJSONConfig(kiroSettingsPath())["chat.defaultAgent"].(string)
	return name
}

// kiroForgetDefaultAgent takes `chat.defaultAgent` out when it names deja's
// agent, which uninstall has just removed: left in, kiro-cli is pointed at an
// agent that is gone. Any other value is the reader's.
func kiroForgetDefaultAgent() error {
	if kiroDefaultAgent() != "deja" {
		return nil
	}
	path := kiroSettingsPath()
	old, err := readConfig(path)
	if err != nil {
		return err
	}
	root := map[string]any{}
	if err := json.Unmarshal([]byte(jsoncToJSON(string(old))), &root); err != nil {
		return nil
	}
	delete(root, "chat.defaultAgent")
	next, err := marshalConfigLike(old, root)
	if err != nil {
		return err
	}
	_, err = writeIfChanged(path, old, append(next, '\n'))
	return err
}
