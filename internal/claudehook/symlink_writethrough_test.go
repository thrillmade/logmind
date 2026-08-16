package claudehook

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// TestEnsurePreToolUseGuard_DanglingSymlinkAtSettings is the regression for
// writeFreshSettings.
//
// EnsurePreToolUseGuard branches on os.ReadFile(.claude/settings.json)
// returning fs.ErrNotExist and calls writeFreshSettings for the "no settings
// yet" case. A DANGLING symlink at that path produces exactly that error —
// ReadFile follows the link and finds nothing at the far end — so a bare
// os.WriteFile in writeFreshSettings followed the same link and dropped the
// hook config outside the repository. The overwrite branch a few lines above
// was already atomic; only the create branch was exposed, which is precisely
// the branch the exploit steers into.
//
// This is the package's own entry point, called verbatim by `logmind
// refresh`, `logmind init` and `logmind self-update` — those three commands
// pass repoRoot and nothing else, so there is no additional command-layer
// behaviour between them and this call.
func TestEnsurePreToolUseGuard_DanglingSymlinkAtSettings(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("unprivileged symlink creation is unreliable on Windows CI runners")
	}

	base := t.TempDir()
	repo := filepath.Join(base, "repo")
	loot := filepath.Join(base, "outside", "loot-settings.json")
	if err := os.MkdirAll(filepath.Join(base, "outside"), 0o755); err != nil {
		t.Fatalf("mkdir outside: %v", err)
	}

	settings := SettingsPath(repo)
	if err := os.MkdirAll(filepath.Dir(settings), 0o755); err != nil {
		t.Fatalf("mkdir .claude: %v", err)
	}
	if err := os.Symlink(loot, settings); err != nil {
		t.Fatalf("symlink: %v", err)
	}
	// Control: the trap must actually read as "absent", or the test never
	// reaches the create branch and would pass against the unfixed code.
	if _, err := os.ReadFile(settings); !os.IsNotExist(err) {
		t.Fatalf("planted link does not read as absent (ReadFile err = %v); test would be vacuous", err)
	}

	_, err := EnsurePreToolUseGuard(repo)

	if body, rerr := os.ReadFile(loot); rerr == nil {
		t.Fatalf("WROTE THROUGH THE SYMLINK: %s was created outside the repository, %d bytes:\n%s",
			loot, len(body), body)
	} else if !os.IsNotExist(rerr) {
		t.Fatalf("unexpected error reading %s: %v", loot, rerr)
	}

	// PR #300 turns this into an explicit refusal inside atomicio.WriteFile.
	// Either outcome is acceptable; a refusal must say why.
	if err != nil {
		if !strings.Contains(strings.ToLower(err.Error()), "symlink") &&
			!strings.Contains(strings.ToLower(err.Error()), "symbolic link") {
			t.Errorf("refused the write but the message does not name the problem: %v", err)
		}
		return
	}
	fi, lerr := os.Lstat(settings)
	if lerr != nil {
		t.Fatalf("succeeded but %s is missing: %v", settings, lerr)
	}
	if fi.Mode()&os.ModeSymlink != 0 {
		t.Errorf("%s is still a symlink; the next settings write will follow it off-tree", settings)
	}
}
