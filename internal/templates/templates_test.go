package templates

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// TestTemplates_ByteIdenticalToPython is the load-bearing parity gate:
// it diffs each embedded template byte-for-byte against the Python
// source-of-truth at src/logmind/templates/<name>. Any drift between
// the two trees fails the test immediately.
//
// Skipped when the Python tree isn't reachable (e.g., running tests
// from an extracted release tarball) — in that case we degrade to the
// other tests in this file which only validate marker presence.
func TestTemplates_ByteIdenticalToPython(t *testing.T) {
	pyDir := pythonTemplatesDir(t)
	if pyDir == "" {
		t.Skip("Python templates dir not reachable; skipping parity diff")
	}

	cases := []struct {
		name     string
		embedded string
	}{
		{"AGENTS.md.template", AgentsTemplate()},
		{"AGENTS.md.slim.template", AgentsSlimTemplate()},
		{"agent-stub.md", Stub()},
		{"logmind-section.md", LogmindSection()},
		{"CLAUDE.md.template", FullClaudeTemplate()},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			py, err := os.ReadFile(filepath.Join(pyDir, tc.name))
			if err != nil {
				t.Fatalf("read python source %s: %v", tc.name, err)
			}
			if string(py) != tc.embedded {
				t.Fatalf("%s drift between Go embed and Python source\n--- want (python) ---\n%s\n--- got (embed) ---\n%s",
					tc.name, py, tc.embedded)
			}
		})
	}
}

// TestAgentsTemplate_HasV6Marker pins the protocol-version marker. The
// `agents update --apply` workflow keys on this marker to decide
// whether an installed block is stale.
//
// v0.6.16 bumped v5→v6: heading reframed as "REQUIRED for substantive
// commits" with an explicit DO-NOT-git-commit blockquote that pairs
// with the commit-msg hook installed by `logmind init`.
func TestAgentsTemplate_HasV6Marker(t *testing.T) {
	body := AgentsTemplate()
	if !strings.Contains(body, "<!-- logmind-block-version: v6 -->") {
		t.Fatalf("full template missing v6 marker")
	}
	if !strings.Contains(body, "<!-- logmind-start -->") || !strings.Contains(body, "<!-- logmind-end -->") {
		t.Fatalf("full template missing start/end markers")
	}
	// v0.6.16: the REQUIRED framing + DO-NOT blockquote are the
	// user-visible delta over v5. Pin both so a future inadvertent
	// revert to the v5 prose trips this test.
	if !strings.Contains(body, "REQUIRED for substantive commits") {
		t.Fatalf("v6 template missing REQUIRED framing in heading")
	}
	if !strings.Contains(body, "DO NOT run `git add` / `git commit` / `git push`") {
		t.Fatalf("v6 template missing DO-NOT-git-commit blockquote")
	}
}

// TestAgentsSlimTemplate_HasV8PointerMarker pins the slim variant
// marker so the byte-identical-rewrite path can never confuse the two
// templates' marker versions.
//
// v0.6.16 bumped v7-pointer→v8-pointer: heading reframed as
// "REQUIRED for substantive commits" + DO-NOT-git-commit blockquote
// paired with the commit-msg hook.
func TestAgentsSlimTemplate_HasV8PointerMarker(t *testing.T) {
	body := AgentsSlimTemplate()
	if !strings.Contains(body, "<!-- logmind-block-version: v8-pointer -->") {
		t.Fatalf("slim template missing v8-pointer marker")
	}
	if !strings.Contains(body, "REQUIRED for substantive commits") {
		t.Fatalf("v8-pointer template missing REQUIRED framing in heading")
	}
	if !strings.Contains(body, "DO NOT run raw `git add` / `git commit` / `git push`") {
		t.Fatalf("v8-pointer template missing DO-NOT-git-commit blockquote")
	}
}

// TestStub_HasStubMarker confirms the stub identifier matches the
// LOGMIND_STUB_MARKER constant used by the inserter package.
func TestStub_HasStubMarker(t *testing.T) {
	body := Stub()
	if !strings.Contains(body, "<!-- logmind-stub:") {
		t.Fatalf("stub template missing stub marker")
	}
}

// TestDependabotTemplate_HasMarkerAndShape pins the v1.1.0 dependabot
// template content — the marker line, the `github-actions` ecosystem,
// and the thrillmade group with the `thrillmade/*` pattern. Any of
// those drifting would break the merge-detection regex in
// inserter/dependabot.go, so the test pins them together.
func TestDependabotTemplate_HasMarkerAndShape(t *testing.T) {
	body := DependabotTemplate()
	if !strings.Contains(body, "logmind-dependabot-marker: v1") {
		t.Fatalf("dependabot template missing v1 marker")
	}
	if !strings.Contains(body, `package-ecosystem: "github-actions"`) {
		t.Fatalf("dependabot template missing github-actions ecosystem")
	}
	if !strings.Contains(body, "thrillmade:") {
		t.Fatalf("dependabot template missing thrillmade group key")
	}
	if !strings.Contains(body, `- "thrillmade/*"`) {
		t.Fatalf("dependabot template missing thrillmade/* pattern")
	}
	if !strings.Contains(body, `interval: "daily"`) {
		t.Fatalf("dependabot template should default to daily — v1.1.0 design point so action pin tracks fresh logmind releases without manual ceremony")
	}
}

// TestWorkflowTemplates_UseSetupLogmindAction pins the v1.1.0
// distribution-lock change: workflow templates must use the new
// `thrillmade/setup-logmind@vX.Y.Z` action and NOT the legacy
// `pip install logmind==X.Y.Z` line. Both check-doc-links and
// regen-timeline are user-facing CI workflows under
// `# logmind-template-version:` markers; the self-update workflow
// uses the action too but is allowed to grep for the legacy pin (so
// pre-v1.1.0 installs can upgrade once). We only assert the consumer-
// CI workflows here.
func TestWorkflowTemplates_UseSetupLogmindAction(t *testing.T) {
	for _, name := range []string{
		"check-doc-links.yml.template",
		"regen-timeline.yml.template",
	} {
		body := Workflow(name)
		if !strings.Contains(body, "thrillmade/setup-logmind@v") {
			t.Errorf("%s missing setup-logmind action ref", name)
		}
		// Legacy curl-install / pip-install patterns must not appear
		// in the consumer-CI templates. They were replaced by the
		// action in v1.1.0 per the 2026-06-05 distribution lock.
		if strings.Contains(body, "pip install \"logmind==") || strings.Contains(body, "actions/setup-python") {
			t.Errorf("%s still contains the pre-v1.1.0 pip install / setup-python pattern", name)
		}
	}
}

// TestRegenTimelineTemplate_V5_Advisory pins the Slice-1 de-friction
// contract: the derived-doc gate must NEVER hard-block a PR and must NEVER
// push with the default GITHUB_TOKEN (a GITHUB_TOKEN push moves the PR head
// SHA without re-triggering checks, stranding every required check). On
// stale docs it either pushes via LOGMIND_AUTO_REGEN_PAT (which re-triggers
// checks) or emits an advisory ::warning:: and exits 0.
func TestRegenTimelineTemplate_V5_Advisory(t *testing.T) {
	body := Workflow("regen-timeline.yml.template")

	// Marker bump v4 → v5.
	if !strings.Contains(body, "# logmind-template-version: v5") {
		t.Errorf("regen-timeline template missing v5 marker")
	}
	// Advisory, never fail-fast: no literal `exit 1`. Staleness must not
	// red-light the PR (a real tool crash still fails the job via `set -e`).
	if strings.Contains(body, "exit 1") {
		t.Errorf("regen-timeline v5 must not `exit 1` — the gate is advisory and must never block a PR")
	}
	// The no-PAT / fork path must warn and pass.
	if !strings.Contains(body, "::warning") || !strings.Contains(body, "exit 0") {
		t.Errorf("regen-timeline v5 missing the advisory warn + exit 0 path")
	}
	// The PAT auto-push path is retained (the only path that pushes).
	if !strings.Contains(body, "LOGMIND_AUTO_REGEN_PAT") {
		t.Errorf("regen-timeline v5 missing the LOGMIND_AUTO_REGEN_PAT auto-push path")
	}
	// Regen commits carry the [skip-logmind] convention (SPEC §5.1/§5.2).
	if !strings.Contains(body, "[skip-logmind]") {
		t.Errorf("regen-timeline v5 PAT-push commit missing the [skip-logmind] prefix")
	}
	// The job must NOT filter itself out by actor: a filtered required check
	// strands as "Expected — Waiting…". Bots/forks take the advisory path.
	if strings.Contains(body, "github.actor != ") {
		t.Errorf("regen-timeline v5 must not filter the job by actor (a filtered required check hangs forever)")
	}
	// Fork PRs must reach the advisory path, not die at checkout: the
	// checkout MUST set `repository:` to the PR head repo (a fork's head_ref
	// does not exist on the base repo, so checkout would otherwise fail red).
	if !strings.Contains(body, "repository: ${{ github.event.pull_request.head.repo.full_name") {
		t.Errorf("regen-timeline v5 checkout must set repository to the PR head repo (else fork PRs fail at checkout)")
	}
	// The advisory diff display MUST be SIGPIPE/pipefail-guarded: a large
	// stale diff piped into `head` exits 141 under `set -euo pipefail` and
	// would abort the job before `exit 0` — re-blocking on big diffs.
	if !strings.Contains(body, "| head -80 || true") {
		t.Errorf("regen-timeline v5 advisory diff must be `|| true`-guarded against SIGPIPE/pipefail")
	}
	// GITHUB_TOKEN must never push: the workflow token stays read-only and
	// the push (if any) uses the explicit PAT-credentialed URL.
	if !strings.Contains(body, "contents: read") {
		t.Errorf("regen-timeline v5 must keep the workflow token read-only (contents: read)")
	}
	if !strings.Contains(body, "x-access-token:${PAT}") {
		t.Errorf("regen-timeline v5 PAT push must use the explicit PAT URL, not a persisted/GITHUB_TOKEN credential")
	}
	// The PAT/token must not be persisted into .git during regeneration/build.
	if !strings.Contains(body, "persist-credentials: false") {
		t.Errorf("regen-timeline v5 checkout must set persist-credentials: false")
	}
}

// TestCheckDocLinksTemplate_V5_DualModeSelfHeal pins the v1.2.0 Layer
// 3 self-heal shape (plan §8.7): the workflow template marker bumps
// to v5, AND it contains both the mode-A (ANTHROPIC_API_KEY auto-fix)
// and mode-B (deterministic PR comment) conditional branches. Either
// mode missing means the dual-mode promise is broken.
func TestCheckDocLinksTemplate_V5_DualModeSelfHeal(t *testing.T) {
	body := Workflow("check-doc-links.yml.template")

	// Marker bump v4 → v5.
	if !strings.Contains(body, "# logmind-template-version: v5") {
		t.Errorf("check-doc-links template missing v5 marker (plan §8.7 / Layer 3)")
	}

	// Mode A: ANTHROPIC_API_KEY-gated. Must mention the secret + run
	// some auto-fix machinery + push back to the PR head ref.
	if !strings.Contains(body, "secrets.ANTHROPIC_API_KEY") {
		t.Errorf("v5 template missing ANTHROPIC_API_KEY conditional (mode A)")
	}
	if !strings.Contains(body, "ANTHROPIC_API_KEY != ''") {
		t.Errorf("v5 template missing mode-A gate (env.ANTHROPIC_API_KEY != '')")
	}
	if !strings.Contains(body, "Mode A") {
		t.Errorf("v5 template missing mode-A label in step name/comment")
	}

	// Mode B: fallback when no key. Must post a PR comment via gh.
	if !strings.Contains(body, "ANTHROPIC_API_KEY == ''") {
		t.Errorf("v5 template missing mode-B gate (env.ANTHROPIC_API_KEY == '')")
	}
	if !strings.Contains(body, "Mode B") {
		t.Errorf("v5 template missing mode-B label in step name/comment")
	}
	if !strings.Contains(body, "gh pr comment") {
		t.Errorf("v5 template missing `gh pr comment` (mode B deterministic path)")
	}

	// Both modes share the `logmind check-links --json` machinery.
	if !strings.Contains(body, "logmind check-links --json") {
		t.Errorf("v5 template missing `logmind check-links --json` invocation")
	}

	// Self-heal job is gated on pull_request events to avoid pushing
	// to main on push triggers.
	if !strings.Contains(body, "github.event_name == 'pull_request'") {
		t.Errorf("v5 self-heal job must be gated on pull_request events")
	}

	// Permissions: needs contents:write (commit) + pull-requests:write
	// (comment). Without these the mode-A push and mode-B comment fail
	// silently.
	if !strings.Contains(body, "contents: write") {
		t.Errorf("v5 template missing contents:write permission")
	}
	if !strings.Contains(body, "pull-requests: write") {
		t.Errorf("v5 template missing pull-requests:write permission")
	}
}

// TestDecisionsBranchHeader_Shape pins the v1.2.0 backlink header
// template (plan §8.7 deliverable 3). The single-line shape +
// trailing blank line + relative ../timeline.md path are
// load-bearing: `logmind log` prepends this verbatim to a freshly-
// created branch decision file so timeline ↔ branch decision linking
// is bidirectional.
func TestDecisionsBranchHeader_Shape(t *testing.T) {
	body := DecisionsBranchHeader()
	want := "← back to [docs/timeline.md](../timeline.md)\n\n"
	if body != want {
		t.Errorf("decisions-branch-header drift:\n--- want ---\n%q\n--- got ---\n%q",
			want, body)
	}
}

// pythonTemplatesDir walks up from the running test file's location
// to find the repo root, then returns src/logmind/templates/. Returns
// "" when the Python tree is absent.
func pythonTemplatesDir(t *testing.T) string {
	t.Helper()
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatalf("runtime.Caller(0) failed")
	}
	dir := filepath.Dir(thisFile)
	for i := 0; i < 8; i++ {
		candidate := filepath.Join(dir, "src", "logmind", "templates")
		if info, err := os.Stat(candidate); err == nil && info.IsDir() {
			return candidate
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return ""
}
