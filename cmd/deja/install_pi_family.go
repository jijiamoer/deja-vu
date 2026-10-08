package main

import (
	"path/filepath"

	"github.com/vshulcz/deja-vu/internal/sources"
)

// Kimchi and gajae-code are pi descendants, and they kept pi's wiring as well
// as its transcript: the MCP server goes in `mcp.json` in the agent directory.
// Both were read from the tools' own source rather than assumed from the
// lineage (#3651):
//
//   - Kimchi: `join(getAgentDir(), "mcp.json")` in
//     src/extensions/mcp-adapter/config.ts. The agent dir is always
//     ~/.config/kimchi/harness: entry.ts sets KIMCHI_CODING_AGENT_DIR itself.
//   - gajae-code: `~/.gjc/agent/mcp.json` for the user scope, in its
//     docs/customization.md surface table, which also gives it native skills
//     at `~/.gjc/agent/skills/<name>/SKILL.md`.
//
// gjc's directory hooks stay unwired for the reason they always were — a
// TypeScript module whose only documented return is `{block, reason}` has no
// channel for adding context — but its extension loader is pi's, and that is
// where recall goes. Read out of gjc 0.17.2 rather than assumed:
//
//   - `loadExtensionModules` in src/discovery/builtin.ts discovers modules in
//     `<agent dir>/extensions`, native `.gjc`/`.pi` only, and its own
//     `customize doctor` calls the file "a trusted filesystem extension module
//     discovered for session-start loading".
//   - The events in src/extensibility/extensions/types.ts are pi's:
//     `session_start`, `before_agent_start`, `context`, `tool_result`,
//     `session_compact`, with the same result shapes. 0.18.7 also has
//     `session_before_compact`, `session.compacting`, whose returned context
//     reaches the summarizer, and `session_shutdown`
//     (extensibility/extensions/types.ts:1208-1213); all fire on a stand. The
//     capture runs at `session_compact`, from the session file.
//
// Verified rather than inferred: with the extension in place, a print-mode
// session against a mock provider carried deja's `<deja-recall>` block into
// the request on both the OpenAI and Anthropic paths.

// Senpi (OmO Native) was read-only for want of anything to read its config
// surface against: the registry had every one of its capabilities as `unknown`,
// because no package or documentation for it was found. There is a package —
// `@code-yeongyu/senpi` — and installing it answers all five at once, each one
// on senpi's own screen:
//
//   - `<agent>/mcp.json` with `mcpServers` is loaded: the palette lists
//     `mcp:deja:deja`.
//   - skills load from `~/.agents/skills`, the shared directory, and from
//     `<agent>/skills`. Both at once is a fault it announces —
//     `"deja-history" collision: ✓ <agent>/skills ✗ ~/.agents/skills (skipped)`
//     — the same trap pi has (#3657), so senpi takes the shared skill only.
//   - pi's extension loads unchanged, and with it the `/deja` command.
//   - a session started with the extension records what `deja hook-context`
//     returned as `{"type":"custom_message","customType":"deja-recall"}`, which
//     is auto-recall arriving.
//   - `--session <path|id>`, `--resume` and `--fork` are in its own help.
//
// Senpi's first run moves `~/.pi/agent` to `~/.senpi/agent` — it is a rename of
// pi's whole directory, sessions and config and extensions — which is worth
// knowing before wiring both.
func senpiMCPPath() string {
	return filepath.Join(sources.SenpiConfigDir(), "mcp.json")
}

func installSenpi(exe string, uninstall bool) (installResult, error) {
	return installMCPJSON(senpiMCPPath(), exe, uninstall)
}

// installSenpiAuto adds the extension, which is both the injection point and
// the command.
func installSenpiAuto(exe string, uninstall bool) (installResult, error) {
	mcp, err := installSenpi(exe, uninstall)
	if err != nil {
		return installResult{}, err
	}
	ext, err := installPiShapedExtension(sources.SenpiConfigDir(), exe, uninstall)
	if err != nil {
		return installResult{}, err
	}
	return wroteAll(mcp, ext), nil
}

func kimchiMCPPath() string {
	return filepath.Join(sources.KimchiConfigDir(), "mcp.json")
}

// kimchiSkillPath is Kimchi's native skill directory. It does not list
// ~/.agents/skills: on a 0.1.99 stand a probe skill there never reached the
// model while the one here did.
func kimchiSkillPath() string {
	return filepath.Join(sources.KimchiConfigDir(), "skills", "deja-history", "SKILL.md")
}

func installKimchi(exe string, uninstall bool) (installResult, error) {
	res, err := installMCPJSON(kimchiMCPPath(), exe, uninstall)
	if err != nil {
		return res, err
	}
	skill, err := installSkillFile(kimchiSkillPath(), uninstall)
	if err != nil {
		return installResult{}, err
	}
	return wroteAll(res, skill), nil
}

// installKimchiAuto adds pi's extension. Kimchi 0.1.99 loads it from the
// harness directory without a trust prompt, and on a stand what its
// before_agent_start, context and tool_result handlers returned reached the
// model; session_before_compact, session_compact and session_shutdown fired.
// The extension also registers /deja.
func installKimchiAuto(exe string, uninstall bool) (installResult, error) {
	base, err := installKimchi(exe, uninstall)
	if err != nil {
		return installResult{}, err
	}
	ext, err := installPiShapedExtension(sources.KimchiConfigDir(), exe, uninstall)
	if err != nil {
		return installResult{}, err
	}
	return wroteAll(base, ext), nil
}

func gjcMCPPath() string {
	return filepath.Join(sources.GjcConfigDir(), "mcp.json")
}

// gjcSkillPath is gjc's native skill location. Claude's and Codex's skill
// directories are import candidates in gjc rather than things it loads, so
// writing there would leave a skill no session reads.
func gjcSkillPath() string {
	return filepath.Join(sources.GjcConfigDir(), "skills", "deja-history", "SKILL.md")
}

func installGjc(exe string, uninstall bool) (installResult, error) {
	res, err := installMCPJSON(gjcMCPPath(), exe, uninstall)
	if err != nil {
		return res, err
	}
	skill, skillErr := installSkillFile(gjcSkillPath(), uninstall)
	if skillErr != nil {
		return installResult{}, skillErr
	}
	command, cmdErr := installCommandFile("gjc", exe, uninstall)
	if cmdErr != nil {
		return installResult{}, cmdErr
	}
	return wroteAll(res, skill, command), nil
}

// installGjcAuto adds the extension, which is where recall arrives without
// being asked. The server and the rest go first, as in every other -auto
// target: `deja install --auto` installs the -auto target alone, and an
// extension with no tool behind it leaves the model unable to follow up on
// what it was just told.
func installGjcAuto(exe string, uninstall bool) (installResult, error) {
	base, err := installGjc(exe, uninstall)
	if err != nil {
		return installResult{}, err
	}
	ext, err := installPiShapedExtension(sources.GjcConfigDir(), exe, uninstall)
	if err != nil {
		return installResult{}, err
	}
	return wroteAll(base, ext), nil
}
