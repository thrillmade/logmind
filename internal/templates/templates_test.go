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

// TestAgentsTemplate_HasV8Marker pins the protocol-version marker. The
// `agents update --apply` workflow keys on this marker to decide
// whether an installed block is stale.
//
// The stale-binary-hardening / enforcement wave bumped v7→v8: the
// DO-NOT-git-commit blockquote now reflects BLOCKING enforcement (the
// commit-msg + Claude Code PreToolUse hooks), not a warn-only hook, and
// documents the `[skip-logmind]` / `LOGMIND_ALLOW_GIT_COMMIT=1` /
// `git.enforce_commits: false` carve-outs. The Slice-2 branch-summary
// wave bumped v6→v7: the full inline procedure carries the
// branch-summary (headline) convention.
func TestAgentsTemplate_HasV8Marker(t *testing.T) {
	body := AgentsTemplate()
	if !strings.Contains(body, "<!-- logmind-block-version: v8 -->") {
		t.Fatalf("full template missing v8 marker")
	}
	if !strings.Contains(body, "<!-- logmind-start -->") || !strings.Contains(body, "<!-- logmind-end -->") {
		t.Fatalf("full template missing start/end markers")
	}
	if !strings.Contains(body, "REQUIRED for substantive commits") {
		t.Fatalf("v8 template missing REQUIRED framing in heading")
	}
	if !strings.Contains(body, "DO NOT run `git add` / `git commit` / `git push`") {
		t.Fatalf("v8 template missing DO-NOT-git-commit blockquote")
	}
	// v8 delta: enforcement prose. BLOCK, not warn; the carve-outs.
	if !strings.Contains(body, "BLOCK") {
		t.Fatalf("v8 template missing BLOCK framing (must not just say the hook warns)")
	}
	if !strings.Contains(body, "[skip-logmind]") {
		t.Fatalf("v8 template missing the [skip-logmind] carve-out")
	}
	if !strings.Contains(body, "LOGMIND_ALLOW_GIT_COMMIT=1") {
		t.Fatalf("v8 template missing the LOGMIND_ALLOW_GIT_COMMIT=1 carve-out")
	}
	if !strings.Contains(body, "git.enforce_commits: false") {
		t.Fatalf("v8 template missing the git.enforce_commits: false per-repo off-ramp")
	}
	// v7 delta (retained): the branch-summary (headline) convention. Pin
	// the heading, both authoring forms, and the verbatim-into-timeline
	// promise so a future revert that drops the convention trips this test.
	if !strings.Contains(body, "Branch summary (headline)") {
		t.Fatalf("v8 template missing the branch-summary (headline) subsection")
	}
	if !strings.Contains(body, `logmind headline "<one sentence>"`) {
		t.Fatalf("v8 template missing the `logmind headline` authoring form")
	}
	if !strings.Contains(body, `logmind log "..." -H "<one sentence>"`) {
		t.Fatalf("v8 template missing the bundled `logmind log -H` authoring form")
	}
	if !strings.Contains(body, "copied verbatim into") {
		t.Fatalf("v8 template missing the verbatim-into-timeline promise")
	}
}

// TestAgentsSlimTemplate_HasV9PointerMarker pins the slim variant
// marker so the byte-identical-rewrite path can never confuse the two
// templates' marker versions.
//
// The stale-binary-hardening / enforcement wave bumped v8-pointer→v9-pointer:
// same enforcement-prose delta as the full template's v7→v8 bump (BLOCKS,
// not warns; documents the carve-outs), condensed to the slim flavour's
// tone/length. v0.6.16 bumped v7-pointer→v8-pointer: heading reframed as
// "REQUIRED for substantive commits" + DO-NOT-git-commit blockquote
// paired with the commit-msg hook.
func TestAgentsSlimTemplate_HasV9PointerMarker(t *testing.T) {
	body := AgentsSlimTemplate()
	if !strings.Contains(body, "<!-- logmind-block-version: v9-pointer -->") {
		t.Fatalf("slim template missing v9-pointer marker")
	}
	if !strings.Contains(body, "REQUIRED for substantive commits") {
		t.Fatalf("v9-pointer template missing REQUIRED framing in heading")
	}
	if !strings.Contains(body, "DO NOT run raw `git add` / `git commit` / `git push`") {
		t.Fatalf("v9-pointer template missing DO-NOT-git-commit blockquote")
	}
	// v9-pointer delta: enforcement prose. BLOCK, not warn; the carve-outs.
	if !strings.Contains(body, "BLOCK") {
		t.Fatalf("v9-pointer template missing BLOCK framing (must not just say the hook warns)")
	}
	if !strings.Contains(body, "[skip-logmind]") {
		t.Fatalf("v9-pointer template missing the [skip-logmind] carve-out")
	}
	if !strings.Contains(body, "LOGMIND_ALLOW_GIT_COMMIT=1") {
		t.Fatalf("v9-pointer template missing the LOGMIND_ALLOW_GIT_COMMIT=1 carve-out")
	}
	if !strings.Contains(body, "git.enforce_commits: false") {
		t.Fatalf("v9-pointer template missing the git.enforce_commits: false per-repo off-ramp")
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
// `pip install logmind==X.Y.Z` line. check-doc-links, regen-timeline,
// and logmind-self-update are all asserted here.
func TestWorkflowTemplates_UseSetupLogmindAction(t *testing.T) {
	for _, name := range []string{
		"check-doc-links.yml.template",
		"regen-timeline.yml.template",
		"logmind-self-update.yml.template",
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

// TestWorkflowTemplates_SetupLogmindStepsCarryToken pins the
// setup-logmind#4 fix: EVERY `uses: thrillmade/setup-logmind@…` step,
// in every workflow template, must pass `token: ${{ github.token }}`.
// Composite actions cannot default an input to `github.token` — that
// has to happen at each call site — so an anonymous setup-logmind call
// hits api.github.com unauthenticated and shared-runner IP ranges
// routinely blow through the 60-req/hour budget before logmind is even
// installed. This test walks every `uses:` occurrence individually (not
// just "the token string appears somewhere in the file") so a future
// setup-logmind call site added without `token:` trips it.
func TestWorkflowTemplates_SetupLogmindStepsCarryToken(t *testing.T) {
	const usesPrefix = "uses: thrillmade/setup-logmind@"
	for _, name := range []string{
		"check-doc-links.yml.template",
		"regen-timeline.yml.template",
		"logmind-self-update.yml.template",
	} {
		body := Workflow(name)
		lines := strings.Split(body, "\n")
		found := 0
		for i, line := range lines {
			if !strings.Contains(line, usesPrefix) {
				continue
			}
			found++
			// The `token:` input must appear within the next few lines
			// (the `with:` block immediately following this step).
			window := lines[i:]
			if len(window) > 6 {
				window = window[:6]
			}
			block := strings.Join(window, "\n")
			if !strings.Contains(block, "token: ${{ github.token }}") {
				t.Errorf("%s: setup-logmind step at line %d missing `token: ${{ github.token }}` (setup-logmind#4)", name, i+1)
			}
		}
		if found == 0 {
			t.Errorf("%s: expected at least one setup-logmind call site", name)
		}
	}
}

// TestRegenTimelineTemplate_V8_BlockingGate pins the v2.0.0
// derived-docs-on-main contract (replaces the Slice-1 advisory model): on a
// PR the `check-derived-docs` job is NON-mutating and BLOCKING — it fails
// (exit 1) if the branch edited docs/timeline.md or docs/file-structure.md,
// because those are derived, main-only artifacts and a branch edit is exactly
// what causes cross-PR merge conflicts. Regeneration moves to a SEPARATE
// main-only `regen-on-main` job. The required-check NAME stays
// `check-derived-docs` so branch-protection rulesets keep matching.
func TestRegenTimelineTemplate_V8_BlockingGate(t *testing.T) {
	body := Workflow("regen-timeline.yml.template")

	// Marker bump v7 → v8 (advisory model → blocking gate + main-only regen).
	if !strings.Contains(body, "# logmind-template-version: v8") {
		t.Errorf("regen-timeline template missing v8 marker")
	}
	// The required-check name MUST stay check-derived-docs (ruleset matching),
	// and the main regen is a distinct job.
	if !strings.Contains(body, "check-derived-docs") {
		t.Errorf("regen-timeline v8 must keep the check-derived-docs job name (ruleset matching)")
	}
	if !strings.Contains(body, "regen-on-main") {
		t.Errorf("regen-timeline v8 missing the separate regen-on-main job")
	}
	// The PR gate BLOCKS: a branch edit to a derived doc must `exit 1`. This is
	// the exact inversion of the old advisory contract and the structural
	// guarantee that a derived-doc conflict can never merge.
	if !strings.Contains(body, "exit 1") {
		t.Errorf("regen-timeline v8 PR gate must `exit 1` on a derived-doc edit (blocking)")
	}
	// The gate detects the edit via the PR's own file list (GitHub's 3-dot
	// diff = branch-vs-merge-base, fork-correct) and matches ONLY the two
	// derived docs as whole lines.
	if !strings.Contains(body, "gh pr diff") || !strings.Contains(body, "--name-only") {
		t.Errorf("regen-timeline v8 PR gate must use `gh pr diff --name-only`")
	}
	if !strings.Contains(body, `grep -qxE 'docs/(timeline|file-structure)\.md'`) {
		t.Errorf("regen-timeline v8 PR gate must match exactly the two derived docs as whole lines")
	}
	// Event-gated jobs: gate runs only on pull_request, regen only on push.
	if !strings.Contains(body, "github.event_name == 'pull_request'") ||
		!strings.Contains(body, "github.event_name == 'push'") {
		t.Errorf("regen-timeline v8 must event-gate the two jobs (pull_request gate / push regen)")
	}
	// The main regen commit carries the [skip-logmind] convention and pushes
	// via the explicit PAT URL (never a persisted/GITHUB_TOKEN credential).
	if !strings.Contains(body, "[skip-logmind]") {
		t.Errorf("regen-timeline v8 main regen commit missing the [skip-logmind] prefix")
	}
	if !strings.Contains(body, "x-access-token:${PAT}") {
		t.Errorf("regen-timeline v8 main push must use the explicit PAT URL")
	}
	if !strings.Contains(body, "persist-credentials: false") {
		t.Errorf("regen-timeline v8 regen-on-main checkout must set persist-credentials: false")
	}
	// The PR gate reads the PR file list via `gh pr diff`, which needs
	// pull-requests:read. Because specifying ANY permission zeroes the rest,
	// this MUST be explicit or the gate 403s and fails-closed on every PR.
	if !strings.Contains(body, "pull-requests: read") {
		t.Errorf("regen-timeline v8 must grant pull-requests: read (else gh pr diff 403s and blocks every PR)")
	}
	// No-PAT is a freshness-only gap, not a failure: warn + exit 0 (never blocks
	// the push event; the invariant guarantee lives in the PR gate, not here).
	if !strings.Contains(body, "::warning") || !strings.Contains(body, "exit 0") {
		t.Errorf("regen-timeline v8 regen-on-main must warn + exit 0 when no PAT (freshness-only)")
	}
}

// TestCheckDocLinksTemplate_V8_AdvisoryNoStrand pins the v1.2.0 advisory
// contract for the doc-link gate: like regen-timeline, it must NEVER
// hard-block a PR and must NEVER push with the default GITHUB_TOKEN (a
// GITHUB_TOKEN push moves the PR head SHA without re-triggering checks,
// stranding every required check). check-links is advisory (warn +
// exit 0); the dual-mode self-heal either PAT-pushes a Claude fix
// (mode A) or posts a deterministic PR comment (mode B).
func TestCheckDocLinksTemplate_V8_AdvisoryNoStrand(t *testing.T) {
	body := Workflow("check-doc-links.yml.template")

	// Marker bump v7 → v8 (setup-logmind#4: added `token: ${{ github.token }}`).
	if !strings.Contains(body, "# logmind-template-version: v8") {
		t.Errorf("check-doc-links template missing v8 marker")
	}

	// Advisory: the old `exit $rc` (re-raise the linkcheck exit) red-lit
	// the PR — it must be gone, replaced by an advisory warning + exit 0.
	if strings.Contains(body, "exit $rc") {
		t.Errorf("check-doc-links v8 must not `exit $rc` — check-links is advisory and must never block a PR")
	}
	if !strings.Contains(body, "::warning") {
		t.Errorf("check-doc-links v8 missing the advisory ::warning:: on broken links")
	}

	// No GITHUB_TOKEN push: the old `git push origin HEAD:<head-ref>`
	// (authenticated by GITHUB_TOKEN) stranded the PR. Any push must go
	// through the explicit PAT-credentialed URL.
	if strings.Contains(body, "git push origin") {
		t.Errorf("check-doc-links v8 must not `git push origin` (a GITHUB_TOKEN push strands the PR's required checks)")
	}
	if !strings.Contains(body, "x-access-token:${PAT}") {
		t.Errorf("check-doc-links v8 mode-A push must use the explicit PAT URL, not a GITHUB_TOKEN credential")
	}

	// Mode A is PAT-gated: without LOGMIND_AUTO_REGEN_PAT there is no safe
	// push, so mode A must require it (and fall through to mode B otherwise).
	if !strings.Contains(body, "LOGMIND_AUTO_REGEN_PAT") {
		t.Errorf("check-doc-links v8 missing the LOGMIND_AUTO_REGEN_PAT push gate")
	}
	if !strings.Contains(body, "env.PAT != ''") {
		t.Errorf("check-doc-links v8 mode-A must be gated on a configured PAT (env.PAT != '')")
	}

	// Workflow token stays read-only; the PAT does any push.
	if !strings.Contains(body, "contents: read") {
		t.Errorf("check-doc-links v8 must keep the workflow token read-only (contents: read)")
	}
	if strings.Contains(body, "contents: write") {
		t.Errorf("check-doc-links v8 must not grant contents: write (the PAT, not GITHUB_TOKEN, pushes)")
	}

	// No actor filter: a filtered required check strands as "Expected —
	// Waiting…". Bots/forks take the advisory / mode-B path.
	if strings.Contains(body, "github.actor != ") {
		t.Errorf("check-doc-links v8 must not filter a job by actor (a filtered required check hangs forever)")
	}

	// Fork PRs must reach the advisory path, not die at checkout.
	if !strings.Contains(body, "repository: ${{ github.event.pull_request.head.repo.full_name") {
		t.Errorf("check-doc-links v8 checkout must set repository to the PR head repo (else fork PRs fail at checkout)")
	}
	// No token persisted into .git; the PAT is injected only at push time.
	if !strings.Contains(body, "persist-credentials: false") {
		t.Errorf("check-doc-links v8 checkout must set persist-credentials: false")
	}

	// Dual-mode self-heal preserved: mode A (Claude auto-fix) + mode B
	// (deterministic PR comment), both keyed off the check-links report.
	if !strings.Contains(body, "Mode A") || !strings.Contains(body, "Mode B") {
		t.Errorf("check-doc-links v8 missing a self-heal mode (both mode A and mode B required)")
	}
	if !strings.Contains(body, "secrets.ANTHROPIC_API_KEY") || !strings.Contains(body, "ANTHROPIC_API_KEY != ''") {
		t.Errorf("check-doc-links v8 missing the mode-A ANTHROPIC_API_KEY gate")
	}
	if !strings.Contains(body, "gh pr comment") {
		t.Errorf("check-doc-links v8 missing `gh pr comment` (mode B deterministic path)")
	}
	if !strings.Contains(body, "logmind check-links --json") {
		t.Errorf("check-doc-links v8 missing `logmind check-links --json` invocation")
	}

	// Self-heal fires on pull_request when check-links reported issues
	// (keyed off the `failed` output, since check-links now always exits 0).
	if !strings.Contains(body, "github.event_name == 'pull_request'") {
		t.Errorf("check-doc-links v8 self-heal must be gated on pull_request events")
	}
	if !strings.Contains(body, "needs.check-links.outputs.failed != '0'") {
		t.Errorf("check-doc-links v8 self-heal must key off the check-links `failed` output")
	}
	// pull-requests:write is still needed for the mode-B comment.
	if !strings.Contains(body, "pull-requests: write") {
		t.Errorf("check-doc-links v8 missing pull-requests:write permission (mode B comment)")
	}

	// Both self-heal modes are best-effort advisory: a transient auto-fix
	// failure (mode A) or a read-only fork token 403 (mode B) MUST degrade
	// to a ::warning:: and never red-light the helper job. Guarding these is
	// what keeps the header's "forks take the mode-B path" promise honest.
	if !strings.Contains(body, "if ! python3") {
		t.Errorf("check-doc-links v8 mode A must guard the Claude auto-fix (a transient failure must not red the self-heal job)")
	}
	if !strings.Contains(body, "if ! gh pr comment") {
		t.Errorf("check-doc-links v8 mode B must guard `gh pr comment` (a fork 403 must not red the self-heal job)")
	}
}

// TestCheckDecisionsTemplate_V4_LivePRTitle pins the issue #212 fix: the
// `[skip-logmind]` override (a NORMATIVE escape hatch — protocol SPEC
// §15.3 / Appendix A.2) must be read from the LIVE PR title at run time,
// not from github.event.pull_request.title, which is frozen at trigger
// time and goes stale across a maintainer retitle+rerun.
func TestCheckDecisionsTemplate_V4_LivePRTitle(t *testing.T) {
	body := Workflow("check-decisions.yml.template")

	// Marker bump v3 → v4.
	if !strings.Contains(body, "# logmind-template-version: v4") {
		t.Errorf("check-decisions template missing v4 marker")
	}

	// The Enforce step's `if:` must no longer trust the frozen event
	// payload title for the skip-logmind check — that's the bug: a
	// re-run after a retitle replays the ORIGINAL payload.
	if strings.Contains(body, "contains(github.event.pull_request.title, '[skip-logmind]')") {
		t.Errorf("check-decisions v4 Enforce step must not gate on the frozen github.event.pull_request.title (issue #212)")
	}
	// Instead it must key off a step output resolved from the live title.
	if !strings.Contains(body, "steps.diff.outputs.skip_logmind != 'true'") {
		t.Errorf("check-decisions v4 Enforce step must gate on steps.diff.outputs.skip_logmind (resolved from the live PR title)")
	}

	// The live title must be fetched via `gh pr view` using the PR number
	// from the event (stable across reruns) — not the title.
	if !strings.Contains(body, `gh pr view "$PR_NUMBER" --json title -q .title`) {
		t.Errorf("check-decisions v4 missing the live-title fetch via `gh pr view \"$PR_NUMBER\" --json title -q .title`")
	}
	if !strings.Contains(body, "PR_NUMBER: ${{ github.event.pull_request.number }}") {
		t.Errorf("check-decisions v4 missing PR_NUMBER sourced from the event (stable across reruns, unlike the title)")
	}

	// A `gh` API hiccup must fail OPEN to the old behavior (the event
	// payload title), never crash the check.
	if !strings.Contains(body, `live_title="$EVENT_TITLE"`) {
		t.Errorf("check-decisions v4 missing the fail-open fallback to the event payload title on a `gh pr view` error")
	}
	if !strings.Contains(body, "EVENT_TITLE: ${{ github.event.pull_request.title }}") {
		t.Errorf("check-decisions v4 missing EVENT_TITLE (the fail-open fallback source)")
	}

	// `gh pr view` needs read access to pull request data; the workflow
	// token must grant it explicitly (the block already restricts
	// everything else to none).
	if !strings.Contains(body, "pull-requests: read") {
		t.Errorf("check-decisions v4 missing pull-requests: read permission (required for `gh pr view`)")
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

// TestSpecTemplate_HasExpectedSections pins the docs/spec.md skeleton
// (`logmind init --spec`): the explanatory HTML comment plus the four
// required sections. Any of these drifting would silently change what
// every newly-scaffolded spec.md looks like.
func TestSpecTemplate_HasExpectedSections(t *testing.T) {
	body := SpecTemplate()
	if !strings.HasPrefix(body, "<!--") {
		t.Fatalf("spec template should open with an explanatory HTML comment")
	}
	for _, want := range []string{
		"context.spec_file",
		"never regenerated",
		"# <Project> — Spec",
		"**Status:** Draft",
		"## What this project is building toward",
		"## Current contract",
		"## Open questions / not yet decided",
		"## Non-goals",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("spec template missing %q", want)
		}
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
