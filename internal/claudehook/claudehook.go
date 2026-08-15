// Package claudehook installs and inspects the Claude Code harness's
// PreToolUse guard entry in `.claude/settings.json` — Layer 1 of
// logmind's two-layer commit-enforcement design (PR2/3 of the
// force-logmind-usage feature; see internal/guardcommit for the shared
// decision engine and internal/hooks for Layer 2, the git commit-msg
// hook).
//
// Why two layers: the git commit-msg hook (Layer 2) is the backstop that
// catches every `git commit` regardless of how it was invoked, but it
// fires AFTER Claude Code has already run the `git commit` Bash tool
// call — by then the agent has spent a turn on a commit that's about to
// be rejected. The PreToolUse hook (Layer 1) intercepts the Bash tool
// call itself, before it runs, so a non-compliant commit never executes
// at all. Both call the same `logmind guard-commit` binary
// (internal/cli/guard_commit.go) with a different `--layer` flag; this
// package only owns the INSTALLER for Layer 1's settings.json entry, not
// the decision logic.
//
// Ownership model: like the git hooks, we own exactly one thing inside
// `.claude/settings.json` — the PreToolUse hook entry whose command
// contains "logmind guard-commit" — and never touch anything else a user
// (or another tool) put in that file. EnsurePreToolUseGuard's merge
// algorithm decodes just enough of the JSON structure to find/patch that
// one entry, leaving every other key's VALUE untouched. The one
// unavoidable side effect: because encoding/json sorts Go map keys when
// marshaling, and this installer round-trips the whole document through
// a map-shaped decode/re-encode, an existing hand-formatted
// settings.json may see its top-level (and edited-object) keys
// re-ordered on first touch. That's semantically inert — JSON object key
// order carries no meaning — and mirrors the identical, already-accepted
// tradeoff in internal/config's YAML round-trip.
package claudehook

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/thrillmade/logmind/internal/atomicio"
	"github.com/thrillmade/logmind/internal/hooks"
	"github.com/thrillmade/logmind/internal/version"
)

// guardCommitMarker is the substring that identifies a logmind-owned
// PreToolUse hook command. Mirrors the ownership-detection role
// internal/hooks.PostMergeMarker etc. play for the git hooks, adapted to
// a single-line JSON string value instead of a shell-script comment.
const guardCommitMarker = "logmind guard-commit"

// rawObject is a JSON object decoded shallowly: every key's value stays
// as undecoded bytes so re-encoding it round-trips content this package
// doesn't understand (or doesn't need to touch) unchanged.
type rawObject = map[string]json.RawMessage

// SettingsPath returns the canonical `.claude/settings.json` path under
// repoRoot.
func SettingsPath(repoRoot string) string {
	return filepath.Join(repoRoot, ".claude", "settings.json")
}

// CanonicalCommand returns the exact command string this package installs
// into the PreToolUse Bash hook entry it owns:
//
//	logmind guard-commit --layer harness # logmind-hook-version: <ver>
//
// The trailing `# logmind-hook-version: X` reuses hooks.HookVersionPrefix
// verbatim as a shell comment — valid syntax in both bash AND
// PowerShell (Claude Code's PreToolUse `command` runs through the
// user's shell, which varies by OS) — giving doctor's drift probe the
// exact same marker-extraction contract the git hooks already use.
//
// Deliberately NOT prefixed with a `command -v logmind` guard, and the
// reason is now stronger than "no behavioral gain" (issue #298, SPEC
// §3.4's "Failing open MUST NOT be silent"): the bare name is the LOUD
// shape here, and the guard would be the silent one.
//
// Measured against Claude Code 2.1.233, driving a real headless session
// with this exact command string and no `logmind` on PATH: the shell
// exits 127, the Bash tool call still runs (fail-open holds — only exit
// 2 blocks), and Claude Code records a `hook_non_blocking_error`
// attachment carrying both `/bin/sh: logmind: command not found` AND
// this whole command string, version marker included. That names what
// it looked for, what it found, and which logmind installed the entry
// — §3.4's three requirements — and it reaches a human.
//
// A `command -v logmind >/dev/null || { echo ... >&2; exit 0; }` guard
// would have to exit 0 to stay fail-open, and an exit-0 PreToolUse
// hook's stderr is surfaced to NO ONE (same measurement: it produces a
// bare `hook_success` attachment). The "obvious" fix therefore trades a
// notice a human sees for one nobody does — on top of the
// cross-platform problem, since the command runs through bash on POSIX
// but PowerShell on Windows without Git Bash, where `command -v` is not
// syntax at all.
//
// What the bare name genuinely cannot catch is a logmind that IS on
// PATH and answers `guard-commit`, but is not the one that wrote this
// entry: that exits 0 and says nothing. Nothing on this line can fix
// that, and nothing on this line has to — the engine reports it from
// the inside, reading the marker above back out of settings.json. See
// harnessHookVersion in internal/cli/guard_commit.go.
//
// TestCanonicalCommand_MissingBinaryIsFailOpenAndLoud pins the measured
// behaviour rather than the shape of the string.
func CanonicalCommand() string {
	return "logmind guard-commit --layer harness " + hooks.HookVersionPrefix + version.Version
}

// canonicalHookEntry is the JSON shape of the single hook object this
// package owns inside a PreToolUse matcher group's `hooks[]` array.
// Field order is the struct's declaration order (type, if, command,
// timeout) — encoding/json preserves struct field order (unlike map key
// order, which it always sorts), so this matches the canonical shape
// documented in the PR spec byte-for-byte on a fresh install.
type canonicalHookEntry struct {
	Type    string `json:"type"`
	If      string `json:"if"`
	Command string `json:"command"`
	Timeout int    `json:"timeout"`
}

func canonicalHookRaw() json.RawMessage {
	h := canonicalHookEntry{
		Type:    "command",
		If:      "Bash(git *)",
		Command: CanonicalCommand(),
		Timeout: 10,
	}
	b, err := json.Marshal(h)
	if err != nil {
		// h is a fixed, always-marshalable literal; this can't happen.
		panic("claudehook: canonical hook entry failed to marshal: " + err.Error())
	}
	return b
}

// canonicalGroupRaw wraps canonicalHookRaw in a fresh `{"matcher":"Bash",
// "hooks":[...]}` group — used both for a from-scratch settings.json and
// for the "no existing Bash PreToolUse group" append case.
func canonicalGroupRaw() json.RawMessage {
	g := struct {
		Matcher string            `json:"matcher"`
		Hooks   []json.RawMessage `json:"hooks"`
	}{Matcher: "Bash", Hooks: []json.RawMessage{canonicalHookRaw()}}
	b, err := json.Marshal(g)
	if err != nil {
		panic("claudehook: canonical group failed to marshal: " + err.Error())
	}
	return b
}

// commandMarkerVersion extracts the `# logmind-hook-version: X` suffix
// from a hook command string (the single-line analog of
// hooks.ExtractVersion, which scans a multi-line shell script instead).
func commandMarkerVersion(command string) (string, bool) {
	idx := strings.Index(command, hooks.HookVersionPrefix)
	if idx == -1 {
		return "", false
	}
	return strings.TrimSpace(command[idx+len(hooks.HookVersionPrefix):]), true
}

// EnsurePreToolUseGuard installs or refreshes the logmind guard-commit
// PreToolUse hook entry in repoRoot/.claude/settings.json. Returns
// changed=true iff the file was created or modified.
//
// Behavior matrix (see the package doc for the ownership model):
//
//	no .claude/settings.json          → create it with just this hook. changed=true.
//	file exists, malformed JSON       → return a clear error; file untouched.
//	our entry present, marker current → no-op. changed=false.
//	our entry present, marker stale/absent → replace just that hook object. changed=true.
//	our entry absent, a Bash PreToolUse group exists → append to its hooks[]. changed=true.
//	our entry absent, no Bash PreToolUse group exists → append a new {matcher:"Bash",...} group. changed=true.
func EnsurePreToolUseGuard(repoRoot string) (changed bool, err error) {
	path := SettingsPath(repoRoot)
	data, err := os.ReadFile(path)
	if err != nil {
		if !errors.Is(err, fs.ErrNotExist) {
			return false, err
		}
		if err := writeFreshSettings(path); err != nil {
			return false, err
		}
		return true, nil
	}

	out, changed, err := mergeSettings(data)
	if err != nil {
		return false, fmt.Errorf("claudehook: %s: %w", path, err)
	}
	if !changed {
		return false, nil
	}
	// path already exists here (we're on the post-ReadFile-success branch) —
	// use the atomic writer so a crash mid-write can't leave the user's
	// settings.json truncated/corrupt.
	if err := atomicio.WriteFile(path, out, 0o644); err != nil {
		return false, err
	}
	return true, nil
}

// writeFreshSettings creates repoRoot/.claude/settings.json (and the
// .claude/ directory) from scratch, containing only the canonical
// PreToolUse guard.
func writeFreshSettings(path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	out, err := canonicalSettingsBytes()
	if err != nil {
		return err
	}
	return os.WriteFile(path, out, 0o644)
}

func canonicalSettingsBytes() ([]byte, error) {
	preToolUse, err := json.Marshal([]json.RawMessage{canonicalGroupRaw()})
	if err != nil {
		return nil, err
	}
	hooksObj, err := json.Marshal(rawObject{"PreToolUse": preToolUse})
	if err != nil {
		return nil, err
	}
	root, err := json.Marshal(rawObject{"hooks": hooksObj})
	if err != nil {
		return nil, err
	}
	out, err := reindent(root)
	if err != nil {
		return nil, err
	}
	return append(out, '\n'), nil
}

// decodedGroup is one entry of hooks.PreToolUse, decoded just enough to
// search/patch it. `raw` holds the ORIGINAL bytes exactly as read from
// disk (nil for a group this package creates from scratch); groups this
// package doesn't need to modify are re-emitted from `raw` verbatim so
// their key order survives untouched. `obj`/`matcherRaw` are only
// populated (and only consulted) for a group we DO end up touching.
type decodedGroup struct {
	raw        json.RawMessage
	obj        rawObject
	matcherRaw json.RawMessage
	matcher    string
	hooksArr   []json.RawMessage
	touched    bool
}

// mergeSettings applies the merge algorithm described on
// EnsurePreToolUseGuard to an already-read settings.json byte slice.
// Split out from EnsurePreToolUseGuard so the pure logic is unit
// testable without touching a filesystem.
func mergeSettings(data []byte) (out []byte, changed bool, err error) {
	root := rawObject{}
	if err := json.Unmarshal(data, &root); err != nil {
		return nil, false, fmt.Errorf("not valid JSON: %w", err)
	}

	hooksObj := rawObject{}
	if raw, ok := root["hooks"]; ok {
		if err := json.Unmarshal(raw, &hooksObj); err != nil {
			return nil, false, fmt.Errorf(".hooks is not a JSON object: %w", err)
		}
	}

	var preToolUseRaw []json.RawMessage
	if raw, ok := hooksObj["PreToolUse"]; ok {
		if err := json.Unmarshal(raw, &preToolUseRaw); err != nil {
			return nil, false, fmt.Errorf(".hooks.PreToolUse is not a JSON array: %w", err)
		}
	}

	groups := make([]decodedGroup, len(preToolUseRaw))
	for i, raw := range preToolUseRaw {
		obj := rawObject{}
		if err := json.Unmarshal(raw, &obj); err != nil {
			return nil, false, fmt.Errorf(".hooks.PreToolUse[%d] is not a JSON object: %w", i, err)
		}
		var matcher string
		matcherRaw, hasMatcher := obj["matcher"]
		if hasMatcher {
			_ = json.Unmarshal(matcherRaw, &matcher) // best-effort; non-string matchers just won't equal "Bash"
		}
		var hooksArr []json.RawMessage
		if h, ok := obj["hooks"]; ok {
			if err := json.Unmarshal(h, &hooksArr); err != nil {
				return nil, false, fmt.Errorf(".hooks.PreToolUse[%d].hooks is not a JSON array: %w", i, err)
			}
		}
		groups[i] = decodedGroup{raw: raw, obj: obj, matcherRaw: matcherRaw, matcher: matcher, hooksArr: hooksArr}
	}

	// Search every group's hooks[] (regardless of matcher) for an entry
	// whose command contains our marker — mirrors installHook's
	// marker-based ownership check.
	found := false
searchLoop:
	for gi := range groups {
		for hi, hraw := range groups[gi].hooksArr {
			hobj := rawObject{}
			if err := json.Unmarshal(hraw, &hobj); err != nil {
				continue // malformed hook entry; not ours, leave it alone
			}
			var command string
			if c, ok := hobj["command"]; ok {
				_ = json.Unmarshal(c, &command)
			}
			if !strings.Contains(command, guardCommitMarker) {
				continue
			}
			found = true
			if v, ok := commandMarkerVersion(command); ok && v == version.Version {
				// Already current — no-op for the whole document.
				return data, false, nil
			}
			// Stale (or pre-marker) — replace just this hook object.
			groups[gi].hooksArr[hi] = canonicalHookRaw()
			groups[gi].touched = true
			break searchLoop
		}
	}

	if !found {
		appended := false
		for gi := range groups {
			if groups[gi].matcher == "Bash" {
				groups[gi].hooksArr = append(groups[gi].hooksArr, canonicalHookRaw())
				groups[gi].touched = true
				appended = true
				break
			}
		}
		if !appended {
			groups = append(groups, decodedGroup{
				matcher:  "Bash",
				hooksArr: []json.RawMessage{canonicalHookRaw()},
				touched:  true,
			})
		}
	}

	newPreToolUse := make([]json.RawMessage, len(groups))
	for i, g := range groups {
		if !g.touched {
			// Completely untouched group: re-emit the ORIGINAL bytes
			// verbatim (not even re-decoded), so its key order survives.
			newPreToolUse[i] = g.raw
			continue
		}
		obj := g.obj
		if obj == nil {
			obj = rawObject{}
		}
		if g.matcherRaw != nil {
			obj["matcher"] = g.matcherRaw
		} else {
			mb, err := json.Marshal(g.matcher)
			if err != nil {
				return nil, false, err
			}
			obj["matcher"] = mb
		}
		hb, err := json.Marshal(g.hooksArr)
		if err != nil {
			return nil, false, err
		}
		obj["hooks"] = hb
		raw, err := json.Marshal(obj)
		if err != nil {
			return nil, false, err
		}
		newPreToolUse[i] = raw
	}

	preToolUseBytes, err := json.Marshal(newPreToolUse)
	if err != nil {
		return nil, false, err
	}
	hooksObj["PreToolUse"] = preToolUseBytes
	hooksBytes, err := json.Marshal(hooksObj)
	if err != nil {
		return nil, false, err
	}
	root["hooks"] = hooksBytes

	rootBytes, err := json.Marshal(root)
	if err != nil {
		return nil, false, err
	}
	out, err = reindent(rootBytes)
	if err != nil {
		return nil, false, err
	}
	return append(out, '\n'), true, nil
}

// reindent normalises compact JSON bytes to a canonical 2-space
// indentation, matching the "pretty" style the PR spec calls for.
func reindent(compact []byte) ([]byte, error) {
	var buf bytes.Buffer
	if err := json.Indent(&buf, compact, "", "  "); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// HookState is the result of inspecting a repo's .claude/settings.json
// for the logmind guard-commit PreToolUse entry. Read-only counterpart
// to EnsurePreToolUseGuard — used by `logmind doctor`'s drift probe.
type HookState struct {
	// SettingsPresent is true when .claude/settings.json exists and
	// parses as JSON.
	SettingsPresent bool
	// EntryPresent is true when a hook command containing
	// "logmind guard-commit" was found somewhere under hooks.PreToolUse.
	EntryPresent bool
	// Version is the embedded `# logmind-hook-version:` marker value, or
	// "" when EntryPresent is false or the entry carries no marker.
	Version string
	// HasMarker is true when EntryPresent is true AND the command carries
	// a `# logmind-hook-version:` marker.
	HasMarker bool
}

// Inspect reads repoRoot/.claude/settings.json and reports the current
// state of the logmind guard-commit PreToolUse entry. Best-effort and
// read-only: any I/O or parse error is reported as "absent" (mirrors
// internal/doctor's probeHook convention of treating unreadable state as
// "not installed" rather than failing the whole doctor run).
func Inspect(repoRoot string) HookState {
	data, err := os.ReadFile(SettingsPath(repoRoot))
	if err != nil {
		return HookState{}
	}
	command, ok := findGuardCommitCommand(data)
	if !ok {
		return HookState{SettingsPresent: true}
	}
	v, hasMarker := commandMarkerVersion(command)
	return HookState{
		SettingsPresent: true,
		EntryPresent:    true,
		Version:         v,
		HasMarker:       hasMarker,
	}
}

// findGuardCommitCommand walks hooks.PreToolUse[*].hooks[*] looking for
// the first command string containing our marker. Tolerant of any
// malformed structure along the way (returns "", false) — this is the
// read-only probe path, not the installer's stricter decode.
func findGuardCommitCommand(data []byte) (string, bool) {
	var root rawObject
	if err := json.Unmarshal(data, &root); err != nil {
		return "", false
	}
	hooksRaw, ok := root["hooks"]
	if !ok {
		return "", false
	}
	var hooksObj rawObject
	if err := json.Unmarshal(hooksRaw, &hooksObj); err != nil {
		return "", false
	}
	preRaw, ok := hooksObj["PreToolUse"]
	if !ok {
		return "", false
	}
	var groups []json.RawMessage
	if err := json.Unmarshal(preRaw, &groups); err != nil {
		return "", false
	}
	for _, graw := range groups {
		var gobj rawObject
		if err := json.Unmarshal(graw, &gobj); err != nil {
			continue
		}
		hraw, ok := gobj["hooks"]
		if !ok {
			continue
		}
		var hookList []json.RawMessage
		if err := json.Unmarshal(hraw, &hookList); err != nil {
			continue
		}
		for _, hh := range hookList {
			var hobj rawObject
			if err := json.Unmarshal(hh, &hobj); err != nil {
				continue
			}
			var command string
			if c, ok := hobj["command"]; ok {
				_ = json.Unmarshal(c, &command)
			}
			if strings.Contains(command, guardCommitMarker) {
				return command, true
			}
		}
	}
	return "", false
}
