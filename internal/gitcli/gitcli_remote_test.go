package gitcli

import (
	"os/exec"
	"testing"
)

func TestRemoteRepoName(t *testing.T) {
	// All these origin URL shapes must parse to the bare repo name.
	for _, url := range []string{
		"git@github.com:thrillmade/logmind.git",
		"https://github.com/thrillmade/logmind.git",
		"https://github.com/thrillmade/logmind",
		"ssh://git@github.com/thrillmade/logmind.git",
		"https://github.com/thrillmade/logmind/",
	} {
		dir := t.TempDir()
		git := func(args ...string) {
			cmd := exec.Command("git", args...)
			cmd.Dir = dir
			if out, err := cmd.CombinedOutput(); err != nil {
				t.Fatalf("git %v: %v\n%s", args, err, out)
			}
		}
		git("init")
		git("remote", "add", "origin", url)
		if got := RemoteRepoName(dir); got != "logmind" {
			t.Errorf("RemoteRepoName(%q) = %q; want logmind", url, got)
		}
	}

	// No origin remote → "" (resolveRootLabel falls back to the basename).
	dir := t.TempDir()
	cmd := exec.Command("git", "init")
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git init: %v\n%s", err, out)
	}
	if got := RemoteRepoName(dir); got != "" {
		t.Errorf("no-origin RemoteRepoName = %q; want empty", got)
	}
}
