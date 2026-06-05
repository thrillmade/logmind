// dependabot_test.go — exercises EnsureDependabot against the three
// reachable code paths (no-file / merge-into-existing / hands-off when
// the user already owns the ecosystem). v1.1.0 introduced this surface
// alongside the setup-logmind action pattern in the workflow templates.
//
// Each test runs in a fresh tmpdir so file writes never bleed across
// cases. We assert the on-disk shape (presence of the thrillmade group,
// preservation of pre-existing content) rather than diffing against a
// golden — golden parity for YAML would force us to commit to one
// indent style forever, and we'd rather leave the merge path tolerant.
package inserter

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestEnsureDependabot_CreatesWhenAbsent — fresh repo, no
// `.github/dependabot.yml`. Expect the bundled template to land
// verbatim under `.github/dependabot.yml` and the result code to be
// DependabotCreated.
func TestEnsureDependabot_CreatesWhenAbsent(t *testing.T) {
	dir := t.TempDir()

	result, err := EnsureDependabot(dir)
	if err != nil {
		t.Fatalf("EnsureDependabot: %v", err)
	}
	if result != DependabotCreated {
		t.Fatalf("expected DependabotCreated, got %v", result)
	}

	body := mustReadFile(t, filepath.Join(dir, ".github", "dependabot.yml"))
	if !strings.Contains(body, "package-ecosystem: \"github-actions\"") {
		t.Errorf("missing github-actions ecosystem entry; got:\n%s", body)
	}
	if !strings.Contains(body, "thrillmade:") {
		t.Errorf("missing thrillmade group; got:\n%s", body)
	}
	if !strings.Contains(body, "- \"thrillmade/*\"") {
		t.Errorf("missing thrillmade/* pattern; got:\n%s", body)
	}
}

// TestEnsureDependabot_IdempotentOnFreshInstall — running twice in a
// row hits the "already has thrillmade group" branch and produces
// DependabotUnchanged with no further writes.
func TestEnsureDependabot_IdempotentOnFreshInstall(t *testing.T) {
	dir := t.TempDir()
	if _, err := EnsureDependabot(dir); err != nil {
		t.Fatalf("first EnsureDependabot: %v", err)
	}
	firstSnapshot := mustReadFile(t, filepath.Join(dir, ".github", "dependabot.yml"))

	result, err := EnsureDependabot(dir)
	if err != nil {
		t.Fatalf("second EnsureDependabot: %v", err)
	}
	if result != DependabotUnchanged {
		t.Fatalf("expected DependabotUnchanged on second call, got %v", result)
	}
	secondSnapshot := mustReadFile(t, filepath.Join(dir, ".github", "dependabot.yml"))
	if firstSnapshot != secondSnapshot {
		t.Errorf("file modified on idempotent second call:\nfirst=\n%s\nsecond=\n%s", firstSnapshot, secondSnapshot)
	}
}

// TestEnsureDependabot_MergesIntoExistingFileWithoutEcosystem —
// consumer repo already has a dependabot.yml that pins gomod (or any
// other ecosystem) but NOT github-actions. We append the github-actions
// block + thrillmade group, preserve every existing byte above the
// append point, and return DependabotMerged.
func TestEnsureDependabot_MergesIntoExistingFileWithoutEcosystem(t *testing.T) {
	dir := t.TempDir()
	githubDir := filepath.Join(dir, ".github")
	if err := os.MkdirAll(githubDir, 0o755); err != nil {
		t.Fatal(err)
	}
	preExisting := `version: 2
updates:
  - package-ecosystem: "gomod"
    directory: "/"
    schedule:
      interval: "weekly"
`
	dependabotPath := filepath.Join(githubDir, "dependabot.yml")
	if err := os.WriteFile(dependabotPath, []byte(preExisting), 0o644); err != nil {
		t.Fatal(err)
	}

	result, err := EnsureDependabot(dir)
	if err != nil {
		t.Fatalf("EnsureDependabot: %v", err)
	}
	if result != DependabotMerged {
		t.Fatalf("expected DependabotMerged, got %v", result)
	}

	body := mustReadFile(t, dependabotPath)
	// Pre-existing entry preserved.
	if !strings.Contains(body, "gomod") {
		t.Errorf("merge dropped the user's gomod entry; got:\n%s", body)
	}
	// Our entry appended.
	if !strings.Contains(body, "package-ecosystem: \"github-actions\"") {
		t.Errorf("merge missing github-actions entry; got:\n%s", body)
	}
	if !strings.Contains(body, "thrillmade:") {
		t.Errorf("merge missing thrillmade group; got:\n%s", body)
	}
	// Sanity: only one `version: 2` line — we never re-write the
	// top-level keys when merging.
	if c := strings.Count(body, "version: 2"); c != 1 {
		t.Errorf("expected one `version: 2` line, got %d in:\n%s", c, body)
	}
}

// TestEnsureDependabot_MergeIsIdempotent — after a merge, the second
// call should detect the thrillmade group and return Unchanged without
// touching the file.
func TestEnsureDependabot_MergeIsIdempotent(t *testing.T) {
	dir := t.TempDir()
	githubDir := filepath.Join(dir, ".github")
	if err := os.MkdirAll(githubDir, 0o755); err != nil {
		t.Fatal(err)
	}
	preExisting := "version: 2\nupdates:\n  - package-ecosystem: \"gomod\"\n    directory: \"/\"\n    schedule:\n      interval: \"weekly\"\n"
	dependabotPath := filepath.Join(githubDir, "dependabot.yml")
	if err := os.WriteFile(dependabotPath, []byte(preExisting), 0o644); err != nil {
		t.Fatal(err)
	}

	if _, err := EnsureDependabot(dir); err != nil {
		t.Fatal(err)
	}
	postMerge := mustReadFile(t, dependabotPath)

	result, err := EnsureDependabot(dir)
	if err != nil {
		t.Fatal(err)
	}
	if result != DependabotUnchanged {
		t.Fatalf("expected DependabotUnchanged after merge, got %v", result)
	}
	postSecond := mustReadFile(t, dependabotPath)
	if postMerge != postSecond {
		t.Errorf("merge wasn't idempotent — file mutated on second call")
	}
}

// TestEnsureDependabot_LeavesUserGithubActionsBlockAlone — the
// consumer already has a github-actions ecosystem entry. We DON'T mutate
// it (Dependabot rejects duplicate ecosystem+directory pairs) and
// return DependabotExistingEcosystem so the CLI can surface a hint.
func TestEnsureDependabot_LeavesUserGithubActionsBlockAlone(t *testing.T) {
	dir := t.TempDir()
	githubDir := filepath.Join(dir, ".github")
	if err := os.MkdirAll(githubDir, 0o755); err != nil {
		t.Fatal(err)
	}
	preExisting := `version: 2
updates:
  - package-ecosystem: "github-actions"
    directory: "/"
    schedule:
      interval: "weekly"
`
	dependabotPath := filepath.Join(githubDir, "dependabot.yml")
	if err := os.WriteFile(dependabotPath, []byte(preExisting), 0o644); err != nil {
		t.Fatal(err)
	}

	result, err := EnsureDependabot(dir)
	if err != nil {
		t.Fatal(err)
	}
	if result != DependabotExistingEcosystem {
		t.Fatalf("expected DependabotExistingEcosystem, got %v", result)
	}
	body := mustReadFile(t, dependabotPath)
	if body != preExisting {
		t.Errorf("file modified when user already owned the ecosystem:\nwant=\n%s\ngot=\n%s", preExisting, body)
	}
}

// TestEnsureDependabot_HonoursExistingThrillmadeGroup — if a prior
// logmind init already merged in our block, re-running detects the
// `thrillmade:` group inside the ecosystem entry and returns Unchanged
// (NOT DependabotExistingEcosystem). The thrillmade group is the
// signature we use to distinguish "this is our entry" from "this is the
// user's entry".
func TestEnsureDependabot_HonoursExistingThrillmadeGroup(t *testing.T) {
	dir := t.TempDir()
	githubDir := filepath.Join(dir, ".github")
	if err := os.MkdirAll(githubDir, 0o755); err != nil {
		t.Fatal(err)
	}
	preExisting := `version: 2
updates:
  - package-ecosystem: "github-actions"
    directory: "/"
    schedule:
      interval: "daily"
    groups:
      thrillmade:
        patterns:
          - "thrillmade/*"
`
	dependabotPath := filepath.Join(githubDir, "dependabot.yml")
	if err := os.WriteFile(dependabotPath, []byte(preExisting), 0o644); err != nil {
		t.Fatal(err)
	}

	result, err := EnsureDependabot(dir)
	if err != nil {
		t.Fatal(err)
	}
	if result != DependabotUnchanged {
		t.Fatalf("expected DependabotUnchanged when thrillmade group already present, got %v", result)
	}
	body := mustReadFile(t, dependabotPath)
	if body != preExisting {
		t.Errorf("file modified despite thrillmade group already being present")
	}
}

// TestEnsureDependabot_MalformedFileSkipsMerge — if the existing file
// is missing the `updates:` key (malformed dependabot.yml that
// Dependabot itself would reject), we don't try to append to it. The
// safe fallback is to leave the file alone and let the user fix it.
func TestEnsureDependabot_MalformedFileSkipsMerge(t *testing.T) {
	dir := t.TempDir()
	githubDir := filepath.Join(dir, ".github")
	if err := os.MkdirAll(githubDir, 0o755); err != nil {
		t.Fatal(err)
	}
	garbage := "this is not yaml\n"
	dependabotPath := filepath.Join(githubDir, "dependabot.yml")
	if err := os.WriteFile(dependabotPath, []byte(garbage), 0o644); err != nil {
		t.Fatal(err)
	}

	result, err := EnsureDependabot(dir)
	if err != nil {
		t.Fatal(err)
	}
	if result != DependabotUnchanged {
		t.Fatalf("expected DependabotUnchanged on malformed file, got %v", result)
	}
	body := mustReadFile(t, dependabotPath)
	if body != garbage {
		t.Errorf("malformed file got modified; expected hands-off")
	}
}

// mustReadFile is a thin t.Helper wrapper. Fails the test on read
// error so the assertion code paths above stay focused on shape.
func mustReadFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(data)
}
