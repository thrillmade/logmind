// block_downgrade_test.go — logmind#267 at the command surfaces: a repo
// whose AGENTS.md block was written by a NEWER binary keeps that block,
// and every command that could have rewritten it says so on stderr.
//
// The refusal has to reach the user through all four: an older binary is
// exactly what a staggered fleet rollout (#257) leaves lying around, and
// the pre-#267 failure was invisible in both directions — the old binary
// reported "Refreshed AGENTS.md logmind block to current template" while
// walking the block backwards.
package cli

import (
	"bytes"
	"fmt"
	"strings"
	"testing"

	"github.com/thrillmade/logmind/internal/inserter"
	"github.com/thrillmade/logmind/internal/templates"
)

// plantNewerBlock writes an AGENTS.md whose block carries a slim marker
// one generation ahead of the bundled one, wrapped in user prose so the
// byte-compare also proves the outer content survived. Returns
// (installedMarker, bundledMarker, exactBytesWritten).
func plantNewerBlock(t *testing.T) (string, string, string) {
	t.Helper()
	bundled := blockMarkerOf(t, templates.AgentsSlimTemplate())
	gen, ok := inserter.ParseMarkerGeneration(bundled)
	if !ok {
		t.Fatalf("bundled slim template carries no readable block-version marker; got %q", bundled)
	}
	installed := fmt.Sprintf("v%d-pointer", gen+1)
	content := "# AGENTS.md\n\nUser prose above.\n\n<!-- logmind-start -->" +
		"\n<!-- logmind-block-version: " + installed + " -->\n" +
		"## Decision logging\nbody written by a newer binary\n" +
		"<!-- logmind-end -->\n\nUser prose below.\n"
	writeRel(t, "AGENTS.md", content, 0o644)
	return installed, bundled, content
}

// blockMarkerOf pulls the `logmind-block-version` token out of a bundled
// template body.
func blockMarkerOf(t *testing.T, templateBody string) string {
	t.Helper()
	const open = "<!-- logmind-block-version: "
	i := strings.Index(templateBody, open)
	if i < 0 {
		t.Fatal("template body carries no logmind-block-version marker")
	}
	rest := templateBody[i+len(open):]
	j := strings.Index(rest, " -->")
	if j < 0 {
		t.Fatal("logmind-block-version marker is not closed")
	}
	return rest[:j]
}

// assertBlockRefusalReported checks the stderr note names the file, the
// installed marker, the direction, and what the binary ships.
func assertBlockRefusalReported(t *testing.T, stderr, installed, bundled string) {
	t.Helper()
	mustContain(t, stderr, "AGENTS.md")
	mustContain(t, stderr, "installed block-version "+installed)
	mustContain(t, stderr, "NEWER")
	mustContain(t, stderr, bundled)
}

// TestInitRefresh_RefusesBlockDowngrade — #267's reported scenario end to
// end through `logmind init` refresh mode.
func TestInitRefresh_RefusesBlockDowngrade(t *testing.T) {
	withTempCwd(t, func(_ string) {
		runQuiet(t, []string{"init", "--no-git"})
		installed, bundled, before := plantNewerBlock(t)

		out, errOut := runInitCapture(t, []string{"init", "--no-git"})

		if after := readRel(t, "AGENTS.md"); after != before {
			t.Errorf("init refresh downgraded a newer block:\n got: %q\nwant: %q", after, before)
		}
		if strings.Contains(out, "Refreshed AGENTS.md logmind block") {
			t.Errorf("a refused downgrade must not be reported as a refresh:\n%s", out)
		}
		assertBlockRefusalReported(t, errOut, installed, bundled)
	})
}

// TestInit_RefusesBlockDowngrade — the FIRST-init surface. A fresh init
// normally creates AGENTS.md, but it can meet one a newer binary already
// wrote (a half-migrated fleet), and that path reaches EnsureAgentsMD too.
func TestInit_RefusesBlockDowngrade(t *testing.T) {
	withTempCwd(t, func(_ string) {
		installed, bundled, before := plantNewerBlock(t)

		out, errOut := runInitCapture(t, []string{"init", "--no-git"})

		if after := readRel(t, "AGENTS.md"); after != before {
			t.Errorf("init downgraded a newer block:\n got: %q\nwant: %q", after, before)
		}
		if strings.Contains(out, "Refreshed AGENTS.md logmind block") {
			t.Errorf("a refused downgrade must not be reported as a refresh:\n%s", out)
		}
		assertBlockRefusalReported(t, errOut, installed, bundled)
	})
}

// TestDoctorFix_RefusesBlockDowngrade — the other applyRefresh caller.
func TestDoctorFix_RefusesBlockDowngrade(t *testing.T) {
	withTempCwd(t, func(_ string) {
		gitInitCwd(t)
		installed, bundled, before := plantNewerBlock(t)

		out, errOut := runDoctorFixCmd(t)

		if after := readRel(t, "AGENTS.md"); after != before {
			t.Errorf("doctor --fix downgraded a newer block:\n got: %q\nwant: %q", after, before)
		}
		mustContain(t, out, "ok doctor-fix")
		assertBlockRefusalReported(t, errOut, installed, bundled)
	})
}

// TestSelfUpdate_RefusesBlockDowngrade — the surface a STALE binary is
// most likely to be run from, so the one most likely to meet a block a
// newer binary wrote.
func TestSelfUpdate_RefusesBlockDowngrade(t *testing.T) {
	withTempCwd(t, func(_ string) {
		installed, bundled, before := plantNewerBlock(t)

		out, errOut := runInitCapture(t, []string{"self-update"})

		if after := readRel(t, "AGENTS.md"); after != before {
			t.Errorf("self-update downgraded a newer block:\n got: %q\nwant: %q", after, before)
		}
		if strings.Contains(out, "Refreshed AGENTS.md logmind block") {
			t.Errorf("a refused downgrade must not be reported as a refresh:\n%s", out)
		}
		if strings.Contains(out, "up to date") {
			t.Errorf("a block this binary cannot move forward is not \"up to date\":\n%s", out)
		}
		assertBlockRefusalReported(t, errOut, installed, bundled)
	})
}

// TestAgentsUpdate_RefusesBlockDowngrade — `agents update` always took the
// right ACTION (it skipped), but silently reported the ahead-of-binary
// block as "current". Skipping and saying so are both required.
func TestAgentsUpdate_RefusesBlockDowngrade(t *testing.T) {
	withTempCwd(t, func(dir string) {
		installed, bundled, before := plantNewerBlock(t)

		var stdout, stderr bytes.Buffer
		if err := runAgentsUpdate(dir, "1.0.0-dev", true, &stdout, &stderr); err != nil {
			t.Fatalf("runAgentsUpdate: %v", err)
		}

		if after := readRel(t, "AGENTS.md"); after != before {
			t.Errorf("agents update --apply downgraded a newer block:\n got: %q\nwant: %q", after, before)
		}
		if strings.Contains(stdout.String(), "is current") {
			t.Errorf("a block this binary cannot move forward is not \"current\":\n%s", stdout.String())
		}
		assertBlockRefusalReported(t, stderr.String(), installed, bundled)
	})
}

// TestAgentsMigrate_RefusesBlockDowngrade — migrate consolidates the
// per-agent files either way, but must not walk AGENTS.md's own block
// backwards while doing it.
func TestAgentsMigrate_RefusesBlockDowngrade(t *testing.T) {
	withTempCwd(t, func(dir string) {
		installed, bundled, before := plantNewerBlock(t)
		writeRel(t, "CLAUDE.md", "# CLAUDE\n\nUser notes worth keeping.\n", 0o644)

		var stdout, stderr bytes.Buffer
		if err := runAgentsMigrate(dir, true, &stdout, &stderr); err != nil {
			t.Fatalf("runAgentsMigrate: %v", err)
		}

		// The block itself is untouched; migrate's own append happens
		// OUTSIDE the markers, so the planted bytes must still be a prefix.
		after := readRel(t, "AGENTS.md")
		if !strings.HasPrefix(after, strings.TrimRight(before, "\n")) {
			t.Errorf("migrate rewrote the newer block:\n got: %q\nwant prefix: %q", after, before)
		}
		assertBlockRefusalReported(t, stderr.String(), installed, bundled)
	})
}

// TestDoctorFixJSON_RefusalSurvivesJSON — the note goes to stderr, so it
// reaches the user without polluting the JSON document on stdout.
func TestDoctorFixJSON_RefusalSurvivesJSON(t *testing.T) {
	withTempCwd(t, func(_ string) {
		gitInitCwd(t)
		installed, bundled, _ := plantNewerBlock(t)

		root := NewRootCmd()
		root.SetArgs([]string{"doctor", "--fix", "--offline", "--json"})
		var out, errOut bytes.Buffer
		root.SetOut(&out)
		root.SetErr(&errOut)
		if err := root.Execute(); err != nil {
			t.Fatalf("doctor --fix --json: %v\nstderr=%s", err, errOut.String())
		}

		if !strings.HasPrefix(strings.TrimSpace(out.String()), "{") {
			t.Errorf("stdout is not a bare JSON document:\n%s", out.String())
		}
		if strings.Contains(out.String(), "left unchanged") {
			t.Errorf("the refusal note must not land on stdout under --json:\n%s", out.String())
		}
		assertBlockRefusalReported(t, errOut.String(), installed, bundled)
	})
}

// TestInitRefresh_UnreadableMarkerRefusedWithItsOwnMessage — an
// unreadable id is not an ordering fact, so it gets the "refusing to
// guess" wording and names both bundled markers instead of claiming the
// block is newer.
func TestInitRefresh_UnreadableMarkerRefusedWithItsOwnMessage(t *testing.T) {
	withTempCwd(t, func(_ string) {
		runQuiet(t, []string{"init", "--no-git"})
		before := "# AGENTS.md\n\n<!-- logmind-start -->" +
			"\n<!-- logmind-block-version: vNEXT -->\nhand-edited block\n" +
			"<!-- logmind-end -->\n"
		writeRel(t, "AGENTS.md", before, 0o644)

		_, errOut := runInitCapture(t, []string{"init", "--no-git"})

		if after := readRel(t, "AGENTS.md"); after != before {
			t.Errorf("an unreadable id was overwritten:\n got: %q\nwant: %q", after, before)
		}
		mustContain(t, errOut, "unrecognised block-version marker")
		mustContain(t, errOut, "found vNEXT")
		mustContain(t, errOut, blockMarkerOf(t, templates.AgentsTemplate()))
		mustContain(t, errOut, blockMarkerOf(t, templates.AgentsSlimTemplate()))
		if strings.Contains(errOut, "NEWER") {
			t.Errorf("an unreadable id must not be reported as newer:\n%s", errOut)
		}
	})
}

// TestInitRefresh_CurrentBlockStillSilent — the guard must not turn every
// ordinary run into a note. A repo on the bundled block gets neither a
// refusal nor a refresh line.
func TestInitRefresh_CurrentBlockStillSilent(t *testing.T) {
	withTempCwd(t, func(_ string) {
		runQuiet(t, []string{"init", "--no-git"})
		before := readRel(t, "AGENTS.md")

		out, errOut := runInitCapture(t, []string{"init", "--no-git"})

		if after := readRel(t, "AGENTS.md"); after != before {
			t.Errorf("a current block was rewritten:\n got: %q\nwant: %q", after, before)
		}
		if strings.Contains(errOut, "left unchanged") {
			t.Errorf("a current block must not produce a refusal note:\n%s", errOut)
		}
		if strings.Contains(out, "Refreshed AGENTS.md logmind block") {
			t.Errorf("a current block must not report a refresh:\n%s", out)
		}
	})
}

// TestInit_FreshRepoStillGetsABlock — the ordinary create path. An
// UNRECOGNISED id means "leave it alone"; an ABSENT FILE still means
// "install one".
func TestInit_FreshRepoStillGetsABlock(t *testing.T) {
	withTempCwd(t, func(dir string) {
		runQuiet(t, []string{"init", "--no-git"})

		got := readRel(t, "AGENTS.md")
		if want := blockMarkerOf(t, templates.AgentsSlimTemplate()); !strings.Contains(got, want) {
			t.Errorf("a fresh repo must get the bundled slim block (%s); got:\n%s", want, got)
		}
		// And the freshly-installed block is not then reported as refused.
		var stdout, stderr bytes.Buffer
		if err := runAgentsUpdate(dir, "1.0.0-dev", false, &stdout, &stderr); err != nil {
			t.Fatalf("runAgentsUpdate: %v", err)
		}
		if stderr.Len() != 0 {
			t.Errorf("a freshly-installed block must not be refused:\n%s", stderr.String())
		}
		mustContain(t, stdout.String(), "is current")
	})
}

// TestReportAgentsBlockRefusal_NilIsSilent — the formatter is called
// unconditionally from six places; nil must write nothing at all.
func TestReportAgentsBlockRefusal_NilIsSilent(t *testing.T) {
	var buf bytes.Buffer
	reportAgentsBlockRefusal(&buf, nil)
	if buf.Len() != 0 {
		t.Errorf("nil refusal wrote %q; want nothing", buf.String())
	}
	// And a non-nil one always writes exactly one line.
	buf.Reset()
	reportAgentsBlockRefusal(&buf, &inserter.AgentsBlockRefusal{
		Path: "/repo/AGENTS.md", Installed: "v99-pointer", Bundled: "v9-pointer", Ahead: true,
	})
	if got := strings.Count(buf.String(), "\n"); got != 1 {
		t.Errorf("refusal wrote %d lines; want exactly 1:\n%s", got, buf.String())
	}
}
