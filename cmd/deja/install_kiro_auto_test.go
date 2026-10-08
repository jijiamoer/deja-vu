package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// kiro-cli runs the hooks of the agent a chat starts in, and adds what an
// agentSpawn or userPromptSubmit hook prints to the model's context. Measured
// on 2.22.0: both outputs were in the request; preToolUse and postToolUse
// output was not (#4304).
func TestInstallKiroAutoWritesAnAgentWithRecallHooks(t *testing.T) {
	hermeticEnv(t)
	if _, err := captureRun(t, "install", "kiro-auto", "--no-index"); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(kiroTestDir(t), "agents", "deja.json")
	var agent struct {
		Name           string   `json:"name"`
		Tools          []string `json:"tools"`
		IncludeMcpJSON bool     `json:"includeMcpJson"`
		Hooks          map[string][]struct {
			Command   string `json:"command"`
			TimeoutMs int    `json:"timeout_ms"`
		} `json:"hooks"`
	}
	if err := json.Unmarshal([]byte(readFile(t, path)), &agent); err != nil {
		t.Fatalf("agent file is not JSON: %v", err)
	}
	if agent.Name != "deja" || !agent.IncludeMcpJSON || len(agent.Tools) != 1 || agent.Tools[0] != "*" {
		t.Errorf("agent is not a full-tool agent that reads the global MCP file: %+v", agent)
	}
	spawn, prompt := agent.Hooks["agentSpawn"], agent.Hooks["userPromptSubmit"]
	if len(spawn) != 1 || !strings.HasSuffix(spawn[0].Command, " hook-context --plain") {
		t.Errorf("agentSpawn does not run hook-context --plain: %+v", spawn)
	}
	if len(prompt) != 1 || !strings.HasSuffix(prompt[0].Command, " hook-prompt --plain") {
		t.Errorf("userPromptSubmit does not run hook-prompt --plain: %+v", prompt)
	}
	if spawn[0].TimeoutMs <= 0 || spawn[0].TimeoutMs > 30000 {
		t.Errorf("timeout_ms = %d", spawn[0].TimeoutMs)
	}
	// The MCP half is the kiro target's.
	if !strings.Contains(readFile(t, kiroMCPSettingsPath()), `"deja"`) {
		t.Error("kiro-auto did not write the MCP entry")
	}

	var row autoWiring
	for _, a := range autoWirings() {
		if a.name == "kiro" {
			row = a
		}
	}
	if row.name == "" {
		t.Fatal("doctor has no auto-recall row for kiro")
	}
	// kiro-cli starts kiro_default unless told otherwise, so an agent nobody
	// starts is installed, not wired.
	if state, _ := autoWiringState(row); state != "installed" {
		t.Errorf("doctor reads the fresh install as %q, want installed", state)
	}

	if _, err := captureRun(t, "uninstall", "kiro-auto"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Errorf("uninstall left the agent behind: %v", err)
	}
}

// The IDE and kiro-cli's V3 engine run ~/.kiro/hooks/*.json in every chat, in
// the v1 schema acp-server.js validates; on a 2.28 stand SessionStart and
// UserPromptSubmit stdout reached the model. The skill directory is loaded by
// both engines.
func TestInstallKiroAutoWritesTheGlobalHooksAndSkill(t *testing.T) {
	hermeticEnv(t)
	if _, err := captureRun(t, "install", "kiro-auto", "--no-index"); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(kiroTestDir(t), "hooks", "deja.json")
	var file struct {
		Version string `json:"version"`
		Hooks   []struct {
			Name    string `json:"name"`
			Trigger string `json:"trigger"`
			Action  struct {
				Type    string `json:"type"`
				Command string `json:"command"`
			} `json:"action"`
		} `json:"hooks"`
	}
	if err := json.Unmarshal([]byte(readFile(t, path)), &file); err != nil {
		t.Fatalf("hook file is not JSON: %v", err)
	}
	if file.Version != "v1" {
		t.Errorf("version = %q, want v1", file.Version)
	}
	want := map[string]string{
		"SessionStart":     " hook-context --plain",
		"UserPromptSubmit": " hook-prompt --plain",
		"SessionEnd":       " hook-session-end",
	}
	for _, h := range file.Hooks {
		suffix, ok := want[h.Trigger]
		if !ok || h.Action.Type != "command" || !strings.HasSuffix(h.Action.Command, suffix) {
			t.Errorf("unexpected hook %+v", h)
		}
		delete(want, h.Trigger)
	}
	if len(want) != 0 {
		t.Errorf("triggers missing: %v", want)
	}
	skill := filepath.Join(kiroTestDir(t), "skills", "deja-history", "SKILL.md")
	if _, err := os.Stat(skill); err != nil {
		t.Errorf("no skill: %v", err)
	}
	if _, err := captureRun(t, "uninstall", "kiro-auto"); err != nil {
		t.Fatal(err)
	}
	for _, p := range []string{path, skill} {
		if _, err := os.Stat(p); !os.IsNotExist(err) {
			t.Errorf("uninstall left %s: %v", p, err)
		}
	}
}

// An agent the reader named deja is theirs: install leaves it alone and still
// writes the MCP server and steering, which `deja install --auto` used to give
// that machine before kiro-auto existed.
func TestInstallKiroAutoLeavesTheReadersOwnDejaAgent(t *testing.T) {
	hermeticEnv(t)
	path := filepath.Join(kiroTestDir(t), "agents", "deja.json")
	theirs := `{"name":"deja","description":"mine","tools":["read"]}` + "\n"
	writeFileMkdir(t, path, theirs)
	out, err := captureRun(t, "install", "kiro-auto", "--no-index")
	if err != nil {
		t.Fatalf("install refused the whole target over their agent: %v", err)
	}
	if got := readFile(t, path); got != theirs {
		t.Errorf("their agent changed:\n%s", got)
	}
	if !strings.Contains(readFile(t, kiroMCPSettingsPath()), `"deja"`) {
		t.Error("no MCP entry beside their agent")
	}
	if _, err := os.Stat(kiroSteeringPath()); err != nil {
		t.Errorf("no steering file beside their agent: %v", err)
	}
	if !strings.Contains(out, "did not write") {
		t.Errorf("install did not say why the agent was skipped:\n%s", out)
	}
	if _, err := captureRun(t, "uninstall", "kiro-auto"); err != nil {
		t.Fatal(err)
	}
	if got := readFile(t, path); got != theirs {
		t.Errorf("uninstall took their agent:\n%s", got)
	}
}

func kiroTestDir(t *testing.T) string {
	t.Helper()
	return filepath.Join(os.Getenv("HOME"), ".kiro")
}

// The deja agent runs only when a chat starts in it. doctor says wired once it
// is kiro-cli's default, and uninstall takes the default back when it named
// deja's agent, so kiro-cli is not left pointing at a file that is gone.
func TestKiroAutoDefaultAgent(t *testing.T) {
	hermeticEnv(t)
	if _, err := captureRun(t, "install", "kiro-auto", "--no-index"); err != nil {
		t.Fatal(err)
	}
	var row autoWiring
	for _, a := range autoWirings() {
		if a.name == "kiro" {
			row = a
		}
	}
	var report bytes.Buffer
	doctorAutoRecall(&report)
	if !strings.Contains(report.String(), "kiro-cli agent set-default deja") {
		t.Errorf("doctor does not say how to make the agent run:\n%s", report.String())
	}
	settings := filepath.Join(kiroTestDir(t), "settings", "cli.json")
	writeFileMkdir(t, settings, "{\n  \"chat.defaultAgent\": \"deja\",\n  \"chat.enableThinking\": true\n}\n")
	if state, _ := autoWiringState(row); state != "wired" {
		t.Errorf("with deja as the default agent doctor says %q, want wired", state)
	}
	if _, err := captureRun(t, "uninstall", "kiro-auto"); err != nil {
		t.Fatal(err)
	}
	got := readFile(t, settings)
	if strings.Contains(got, "chat.defaultAgent") || !strings.Contains(got, "chat.enableThinking") {
		t.Errorf("uninstall did not take back only the default agent:\n%s", got)
	}

	// A default of the reader's own stays.
	if _, err := captureRun(t, "install", "kiro-auto", "--no-index"); err != nil {
		t.Fatal(err)
	}
	mine := "{\n  \"chat.defaultAgent\": \"mine\"\n}\n"
	writeFileMkdir(t, settings, mine)
	if _, err := captureRun(t, "uninstall", "kiro-auto"); err != nil {
		t.Fatal(err)
	}
	if got := readFile(t, settings); got != mine {
		t.Errorf("uninstall changed a default that was not deja's:\n%s", got)
	}
}
