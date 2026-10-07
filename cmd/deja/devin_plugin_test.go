package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The Devin plugin is what a `devin plugins install` gets instead of
// `deja install devin-auto`, so it has to carry the same wiring: the same
// events, matchers and hook subcommands, a server that starts, and files the
// manifest points at that exist. Its version moves with the other plugins.
func TestDevinPluginMatchesTheInstaller(t *testing.T) {
	var m struct {
		Name       string `json:"name"`
		Version    string `json:"version"`
		Skills     string `json:"skills"`
		MCPServers map[string]struct {
			Command string   `json:"command"`
			Args    []string `json:"args"`
		} `json:"mcpServers"`
	}
	if err := json.Unmarshal(repoFile(t, "devin-plugin/.devin-plugin/plugin.json"), &m); err != nil {
		t.Fatal(err)
	}
	var claude struct {
		Version string `json:"version"`
	}
	if err := json.Unmarshal(repoFile(t, "claude-plugin/.claude-plugin/plugin.json"), &claude); err != nil {
		t.Fatal(err)
	}
	if m.Name != "deja-vu" || m.Version == "" || m.Version != claude.Version {
		t.Fatalf("name %q version %q, want deja-vu at %q", m.Name, m.Version, claude.Version)
	}
	server, ok := m.MCPServers["deja"]
	if !ok || server.Command != "deja" || len(server.Args) != 1 || server.Args[0] != "mcp" {
		t.Fatalf("mcpServers.deja = %+v, want the one-token deja mcp server", m.MCPServers)
	}

	var hooks map[string][]struct {
		Matcher string `json:"matcher"`
		Hooks   []struct {
			Type    string `json:"type"`
			Command string `json:"command"`
			Timeout int    `json:"timeout"`
		} `json:"hooks"`
	}
	if err := json.Unmarshal(repoFile(t, "devin-plugin/hooks.json"), &hooks); err != nil {
		t.Fatal(err)
	}
	want := map[string][]string{}
	for _, w := range devinHookWiring {
		want[w.Event] = append(want[w.Event], w.Matcher+":"+w.Sub)
	}
	for event, wiring := range want {
		entries := hooks[event]
		if len(entries) != len(wiring) {
			t.Fatalf("%s: plugin wires %d entries, the installer %d", event, len(entries), len(wiring))
		}
		for i, pair := range wiring {
			parts := strings.SplitN(pair, ":", 2)
			matcher, sub := parts[0], parts[1]
			if entries[i].Matcher != matcher || len(entries[i].Hooks) != 1 {
				t.Fatalf("%s[%d]: entry %+v, want matcher %q", event, i, entries[i], matcher)
			}
			h := entries[i].Hooks[0]
			if h.Type != "command" || h.Timeout != 60 {
				t.Fatalf("%s[%d]: hook %+v", event, i, h)
			}
			if !strings.HasSuffix(h.Command, "exec deja "+sub) {
				t.Fatalf("%s[%d]: command %q does not run deja %s", event, i, h.Command, sub)
			}
			// The plugin stands down when `deja install devin-auto` has already
			// wired ~/.config/devin/config.json — every session would otherwise
			// get the digest twice — and it stays silent without the binary.
			if !strings.Contains(h.Command, "config.json") || !strings.Contains(h.Command, "command -v deja") {
				t.Fatalf("%s[%d]: command %q has no stand-down or binary guard", event, i, h.Command)
			}
		}
	}
	for event := range hooks {
		if _, ok := want[event]; !ok {
			t.Errorf("plugin wires %s, which the installer never does", event)
		}
	}
	// The opening digest is the moment to tell the user the binary is missing;
	// the other events stay quiet.
	for _, event := range []string{"SessionStart", "PostCompaction"} {
		if !strings.Contains(hooks[event][0].Hooks[0].Command, "systemMessage") {
			t.Errorf("%s hook does not say how to get the binary", event)
		}
	}

	root := filepath.Join("..", "..", "devin-plugin")
	for _, rel := range []string{
		"skills/deja-history/SKILL.md", "README.md", "LICENSE", "hooks.json",
	} {
		info, err := os.Stat(filepath.Join(root, filepath.FromSlash(rel)))
		if err != nil || !info.Mode().IsRegular() {
			t.Errorf("devin-plugin/%s: %v", rel, err)
		}
	}
	if m.Skills != "./skills/" {
		t.Errorf("skills field = %q, want the bundle's skills dir", m.Skills)
	}
}
