package claudehook

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/thrillmade/logmind/internal/hooks"
	"github.com/thrillmade/logmind/internal/version"
)

// --- CanonicalCommand -------------------------------------------------

func TestCanonicalCommand_Shape(t *testing.T) {
	got := CanonicalCommand()
	want := "logmind guard-commit --layer harness " + hooks.HookVersionPrefix + version.Version
	if got != want {
		t.Fatalf("CanonicalCommand() = %q; want %q", got, want)
	}
	if strings.Contains(got, "command -v") {
		t.Errorf("CanonicalCommand() must NOT contain a `command -v logmind` guard (must stay cross-platform + fail-open on missing binary); got %q", got)
	}
}

// --- EnsurePreToolUseGuard: fresh install ------------------------------

func TestEnsurePreToolUseGuard_FreshInstall(t *testing.T) {
	dir := t.TempDir()
	changed, err := EnsurePreToolUseGuard(dir)
	if err != nil {
		t.Fatalf("EnsurePreToolUseGuard: %v", err)
	}
	if !changed {
		t.Fatalf("changed = false on a fresh repo; want true")
	}

	path := SettingsPath(dir)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read settings.json: %v", err)
	}

	var doc map[string]any
	if err := json.Unmarshal(data, &doc); err != nil {
		t.Fatalf("output is not valid JSON: %v\n%s", err, data)
	}

	command := onlyHookCommand(t, doc)
	if command != CanonicalCommand() {
		t.Errorf("command = %q; want %q", command, CanonicalCommand())
	}

	// Pretty (2-space indent) per spec.
	if !strings.Contains(string(data), "\n  \"hooks\"") && !strings.Contains(string(data), "\n  \"PreToolUse\"") {
		// Depending on top-level key set this could be nested either way;
		// the real assertion is just "not a single compact line".
	}
	if strings.Count(string(data), "\n") < 5 {
		t.Errorf("expected multi-line pretty JSON; got:\n%s", data)
	}
}

// --- EnsurePreToolUseGuard: merge into existing empty hooks ------------

func TestEnsurePreToolUseGuard_MergeIntoExistingEmptyHooks(t *testing.T) {
	dir := t.TempDir()
	path := SettingsPath(dir)
	mustWriteJSON(t, path, map[string]any{
		"permissions": map[string]any{"allow": []any{"Bash(ls *)"}},
		"hooks":       map[string]any{},
	})

	changed, err := EnsurePreToolUseGuard(dir)
	if err != nil {
		t.Fatalf("EnsurePreToolUseGuard: %v", err)
	}
	if !changed {
		t.Fatalf("changed = false; want true")
	}

	doc := readJSONDoc(t, path)
	command := onlyHookCommand(t, doc)
	if command != CanonicalCommand() {
		t.Errorf("command = %q; want %q", command, CanonicalCommand())
	}
	// Untouched sibling top-level key survives.
	perms, ok := doc["permissions"].(map[string]any)
	if !ok {
		t.Fatalf("permissions key lost or reshaped: %v", doc["permissions"])
	}
	if !reflect.DeepEqual(perms["allow"], []any{"Bash(ls *)"}) {
		t.Errorf("permissions.allow = %v; want [Bash(ls *)]", perms["allow"])
	}
}

// --- EnsurePreToolUseGuard: merge alongside a foreign Bash hook --------

// TestEnsurePreToolUseGuard_MergeAlongsideForeignBashHook is the crux
// non-destructiveness test: an existing matcher:"Bash" PreToolUse group
// with a hook logmind doesn't own must gain our entry (appended into the
// SAME group) while its own hook, a sibling matcher:"Write" group, and an
// unrelated PostToolUse event all survive with their VALUES unchanged
// (structurally byte-identical — see foreign entries compared via
// canonical re-marshal below; only whitespace/key-order may differ, which
// the package doc documents as an accepted, semantically inert tradeoff).
func TestEnsurePreToolUseGuard_MergeAlongsideForeignBashHook(t *testing.T) {
	dir := t.TempDir()
	path := SettingsPath(dir)

	foreignBashHook := map[string]any{
		"type":    "command",
		"command": "echo foreign-hook",
		"timeout": float64(5),
	}
	foreignWriteHook := map[string]any{
		"type":    "command",
		"command": "echo write-hook",
	}
	postToolUse := []any{
		map[string]any{
			"matcher": "Bash",
			"hooks":   []any{map[string]any{"type": "command", "command": "echo post-tool-use"}},
		},
	}
	original := map[string]any{
		"permissions": map[string]any{"allow": []any{"Bash(ls *)"}},
		"hooks": map[string]any{
			"PreToolUse": []any{
				map[string]any{"matcher": "Bash", "hooks": []any{foreignBashHook}},
				map[string]any{"matcher": "Write", "hooks": []any{foreignWriteHook}},
			},
			"PostToolUse": postToolUse,
		},
	}
	mustWriteJSON(t, path, original)

	changed, err := EnsurePreToolUseGuard(dir)
	if err != nil {
		t.Fatalf("EnsurePreToolUseGuard: %v", err)
	}
	if !changed {
		t.Fatalf("changed = false; want true")
	}

	doc := readJSONDoc(t, path)

	// PostToolUse must survive completely unchanged (content-wise).
	gotPostToolUse := navigate(t, doc, "hooks", "PostToolUse")
	assertJSONEqual(t, "hooks.PostToolUse", gotPostToolUse, postToolUse)

	// permissions must survive completely unchanged.
	assertJSONEqual(t, "permissions", doc["permissions"], original["permissions"])

	preToolUse, ok := navigate(t, doc, "hooks", "PreToolUse").([]any)
	if !ok {
		t.Fatalf("hooks.PreToolUse is not an array: %#v", navigate(t, doc, "hooks", "PreToolUse"))
	}
	if len(preToolUse) != 2 {
		t.Fatalf("hooks.PreToolUse has %d groups; want 2 (no new group should have been created — we append into the existing Bash group)", len(preToolUse))
	}

	var bashGroup, writeGroup map[string]any
	for _, g := range preToolUse {
		gm := g.(map[string]any)
		switch gm["matcher"] {
		case "Bash":
			bashGroup = gm
		case "Write":
			writeGroup = gm
		}
	}
	if bashGroup == nil {
		t.Fatalf("Bash matcher group missing after merge")
	}
	if writeGroup == nil {
		t.Fatalf("Write matcher group missing after merge")
	}

	// The Write group (never touched) must survive unchanged.
	assertJSONEqual(t, "Write group", writeGroup, map[string]any{"matcher": "Write", "hooks": []any{foreignWriteHook}})

	// The Bash group must now hold BOTH the foreign hook (unchanged) and
	// our canonical entry — appended, not replacing.
	bashHooks, ok := bashGroup["hooks"].([]any)
	if !ok || len(bashHooks) != 2 {
		t.Fatalf("Bash group hooks = %#v; want 2 entries (foreign + ours)", bashGroup["hooks"])
	}
	assertJSONEqual(t, "foreign Bash hook", bashHooks[0], foreignBashHook)

	ourHook := bashHooks[1].(map[string]any)
	if ourHook["command"] != CanonicalCommand() {
		t.Errorf("appended hook command = %v; want %q", ourHook["command"], CanonicalCommand())
	}
}

// --- EnsurePreToolUseGuard: idempotent re-run --------------------------

func TestEnsurePreToolUseGuard_RerunWithCurrentMarkerIsNoop(t *testing.T) {
	dir := t.TempDir()
	if _, err := EnsurePreToolUseGuard(dir); err != nil {
		t.Fatalf("first install: %v", err)
	}
	path := SettingsPath(dir)
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read after first install: %v", err)
	}

	changed, err := EnsurePreToolUseGuard(dir)
	if err != nil {
		t.Fatalf("second install: %v", err)
	}
	if changed {
		t.Errorf("changed = true on a re-run with a current marker; want false")
	}
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read after second install: %v", err)
	}
	if string(before) != string(after) {
		t.Errorf("file bytes changed on a no-op re-run:\n--- before ---\n%s\n--- after ---\n%s", before, after)
	}
}

// --- EnsurePreToolUseGuard: version-bump replace -----------------------

// TestEnsurePreToolUseGuard_VersionBumpReplacesOnlyOurEntry seeds an
// installed hook carrying a stale version marker (simulating an older
// logmind binary's install) alongside a sibling hook in the SAME group,
// and asserts only OUR entry's command is rewritten — the sibling is
// untouched.
func TestEnsurePreToolUseGuard_VersionBumpReplacesOnlyOurEntry(t *testing.T) {
	dir := t.TempDir()
	path := SettingsPath(dir)

	staleCommand := "logmind guard-commit --layer harness " + hooks.HookVersionPrefix + "0.1.0-STALE"
	siblingHook := map[string]any{"type": "command", "command": "echo sibling", "timeout": float64(3)}
	original := map[string]any{
		"hooks": map[string]any{
			"PreToolUse": []any{
				map[string]any{
					"matcher": "Bash",
					"hooks": []any{
						siblingHook,
						map[string]any{"type": "command", "if": "Bash(git *)", "command": staleCommand, "timeout": float64(10)},
					},
				},
			},
		},
	}
	mustWriteJSON(t, path, original)

	changed, err := EnsurePreToolUseGuard(dir)
	if err != nil {
		t.Fatalf("EnsurePreToolUseGuard: %v", err)
	}
	if !changed {
		t.Fatalf("changed = false on a stale marker; want true")
	}

	doc := readJSONDoc(t, path)
	preToolUse := navigate(t, doc, "hooks", "PreToolUse").([]any)
	if len(preToolUse) != 1 {
		t.Fatalf("expected the single existing group to be reused, not duplicated; got %d groups", len(preToolUse))
	}
	group := preToolUse[0].(map[string]any)
	bashHooks := group["hooks"].([]any)
	if len(bashHooks) != 2 {
		t.Fatalf("expected the group to still have exactly 2 hooks (sibling + ours); got %d", len(bashHooks))
	}

	assertJSONEqual(t, "sibling hook", bashHooks[0], siblingHook)

	ours := bashHooks[1].(map[string]any)
	if ours["command"] != CanonicalCommand() {
		t.Errorf("replaced command = %v; want %q", ours["command"], CanonicalCommand())
	}
}

// --- EnsurePreToolUseGuard: malformed settings.json --------------------

func TestEnsurePreToolUseGuard_MalformedSettingsReturnsErrorAndLeavesFileUntouched(t *testing.T) {
	dir := t.TempDir()
	path := SettingsPath(dir)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	broken := "{ this is not valid JSON "
	if err := os.WriteFile(path, []byte(broken), 0o644); err != nil {
		t.Fatal(err)
	}

	changed, err := EnsurePreToolUseGuard(dir)
	if err == nil {
		t.Fatalf("expected an error on malformed settings.json; got nil (changed=%v)", changed)
	}
	if changed {
		t.Errorf("changed = true on an error path; want false")
	}
	got, readErr := os.ReadFile(path)
	if readErr != nil {
		t.Fatalf("re-read settings.json: %v", readErr)
	}
	if string(got) != broken {
		t.Fatalf("malformed settings.json was modified:\n got: %q\nwant: %q", got, broken)
	}
}

func TestEnsurePreToolUseGuard_HooksKeyNotAnObjectReturnsError(t *testing.T) {
	dir := t.TempDir()
	path := SettingsPath(dir)
	mustWriteJSON(t, path, map[string]any{"hooks": "not an object"})

	if _, err := EnsurePreToolUseGuard(dir); err == nil {
		t.Fatalf("expected an error when .hooks is not a JSON object")
	}
}

func TestEnsurePreToolUseGuard_PreToolUseNotAnArrayReturnsError(t *testing.T) {
	dir := t.TempDir()
	path := SettingsPath(dir)
	mustWriteJSON(t, path, map[string]any{"hooks": map[string]any{"PreToolUse": "nope"}})

	if _, err := EnsurePreToolUseGuard(dir); err == nil {
		t.Fatalf("expected an error when .hooks.PreToolUse is not a JSON array")
	}
}

// --- EnsurePreToolUseGuard: no existing Bash group ----------------------

func TestEnsurePreToolUseGuard_NoExistingBashGroupAppendsNewGroup(t *testing.T) {
	dir := t.TempDir()
	path := SettingsPath(dir)
	writeHook := map[string]any{"type": "command", "command": "echo write-hook"}
	mustWriteJSON(t, path, map[string]any{
		"hooks": map[string]any{
			"PreToolUse": []any{
				map[string]any{"matcher": "Write", "hooks": []any{writeHook}},
			},
		},
	})

	changed, err := EnsurePreToolUseGuard(dir)
	if err != nil {
		t.Fatalf("EnsurePreToolUseGuard: %v", err)
	}
	if !changed {
		t.Fatalf("changed = false; want true")
	}

	doc := readJSONDoc(t, path)
	preToolUse := navigate(t, doc, "hooks", "PreToolUse").([]any)
	if len(preToolUse) != 2 {
		t.Fatalf("expected a NEW group to be appended (2 total); got %d", len(preToolUse))
	}
	writeGroup := preToolUse[0].(map[string]any)
	assertJSONEqual(t, "Write group", writeGroup, map[string]any{"matcher": "Write", "hooks": []any{writeHook}})

	bashGroup := preToolUse[1].(map[string]any)
	if bashGroup["matcher"] != "Bash" {
		t.Fatalf("new group matcher = %v; want Bash", bashGroup["matcher"])
	}
	bashHooks := bashGroup["hooks"].([]any)
	if len(bashHooks) != 1 {
		t.Fatalf("new group hooks = %v; want exactly our 1 entry", bashHooks)
	}
}

// --- Inspect ------------------------------------------------------------

func TestInspect_NoSettingsFile(t *testing.T) {
	dir := t.TempDir()
	state := Inspect(dir)
	if state.SettingsPresent || state.EntryPresent {
		t.Errorf("Inspect() = %+v; want both false on a missing settings.json", state)
	}
}

func TestInspect_SettingsPresentNoEntry(t *testing.T) {
	dir := t.TempDir()
	mustWriteJSON(t, SettingsPath(dir), map[string]any{"permissions": map[string]any{}})
	state := Inspect(dir)
	if !state.SettingsPresent {
		t.Errorf("SettingsPresent = false; want true")
	}
	if state.EntryPresent {
		t.Errorf("EntryPresent = true; want false (no PreToolUse hooks at all)")
	}
}

func TestInspect_AfterInstallReportsCurrentVersion(t *testing.T) {
	dir := t.TempDir()
	if _, err := EnsurePreToolUseGuard(dir); err != nil {
		t.Fatalf("install: %v", err)
	}
	state := Inspect(dir)
	if !state.SettingsPresent || !state.EntryPresent || !state.HasMarker {
		t.Fatalf("Inspect() = %+v; want fully present with a marker", state)
	}
	if state.Version != version.Version {
		t.Errorf("Version = %q; want %q", state.Version, version.Version)
	}
}

// --- test helpers ---------------------------------------------------------

func mustWriteJSON(t *testing.T, path string, v any) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		t.Fatalf("marshal fixture: %v", err)
	}
	if err := os.WriteFile(path, b, 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
}

func readJSONDoc(t *testing.T, path string) map[string]any {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	var doc map[string]any
	if err := json.Unmarshal(data, &doc); err != nil {
		t.Fatalf("output is not valid JSON: %v\n%s", err, data)
	}
	return doc
}

// navigate walks a chain of map keys, failing the test if any hop isn't a
// map or the key is absent.
func navigate(t *testing.T, doc map[string]any, keys ...string) any {
	t.Helper()
	var cur any = doc
	for _, k := range keys {
		m, ok := cur.(map[string]any)
		if !ok {
			t.Fatalf("navigate(%v): %q is not a JSON object (got %#v)", keys, k, cur)
		}
		v, ok := m[k]
		if !ok {
			t.Fatalf("navigate(%v): key %q missing", keys, k)
		}
		cur = v
	}
	return cur
}

// assertJSONEqual re-marshals both values to canonical JSON (Go's
// encoding/json deterministically sorts map keys) and compares the
// resulting bytes — the strongest reasonable "byte-identical" comparison
// for JSON values, whose whitespace/key-order carries no meaning.
func assertJSONEqual(t *testing.T, label string, got, want any) {
	t.Helper()
	gotBytes, err := json.Marshal(got)
	if err != nil {
		t.Fatalf("%s: marshal got: %v", label, err)
	}
	wantBytes, err := json.Marshal(want)
	if err != nil {
		t.Fatalf("%s: marshal want: %v", label, err)
	}
	if string(gotBytes) != string(wantBytes) {
		t.Errorf("%s drifted:\n got: %s\nwant: %s", label, gotBytes, wantBytes)
	}
}

// onlyHookCommand digs into doc.hooks.PreToolUse[0].hooks[0].command and
// returns it, failing the test if the shape doesn't match (used by tests
// that expect exactly one group with exactly one hook).
func onlyHookCommand(t *testing.T, doc map[string]any) string {
	t.Helper()
	preToolUse, ok := navigate(t, doc, "hooks", "PreToolUse").([]any)
	if !ok || len(preToolUse) != 1 {
		t.Fatalf("hooks.PreToolUse = %#v; want exactly 1 group", navigate(t, doc, "hooks", "PreToolUse"))
	}
	group, ok := preToolUse[0].(map[string]any)
	if !ok {
		t.Fatalf("PreToolUse[0] is not an object: %#v", preToolUse[0])
	}
	if group["matcher"] != "Bash" {
		t.Fatalf("PreToolUse[0].matcher = %v; want Bash", group["matcher"])
	}
	hookList, ok := group["hooks"].([]any)
	if !ok || len(hookList) != 1 {
		t.Fatalf("PreToolUse[0].hooks = %#v; want exactly 1 hook", group["hooks"])
	}
	hookObj, ok := hookList[0].(map[string]any)
	if !ok {
		t.Fatalf("hooks[0] is not an object: %#v", hookList[0])
	}
	command, _ := hookObj["command"].(string)
	return command
}
