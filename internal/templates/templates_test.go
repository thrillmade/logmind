package templates

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

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

// TestRegenTimelineTemplate_V10_UnconditionalBlockingGate pins the v2.0.0
// derived-docs-on-main contract now that v9's B6 per-repo adoption gate is
// removed: on a PR the `check-derived-docs` job is NON-mutating and
// BLOCKING — UNCONDITIONALLY, for every repo — it fails (exit 1) if the
// branch edited docs/timeline.md or docs/file-structure.md, because those
// are derived, main-only artifacts and a branch edit is exactly what causes
// cross-PR merge conflicts. Regeneration lives in a SEPARATE main-only
// `regen-on-main` job. The required-check NAME stays `check-derived-docs`
// so branch-protection rulesets keep matching.
//
// v10: removes v9's `derived_docs.mode: integration-point` opt-in — the
// invariant is now unconditional, with no per-repo escape hatch and no
// adoption signal left to read from anywhere, PR or base ref.
func TestRegenTimelineTemplate_V10_UnconditionalBlockingGate(t *testing.T) {
	body := Workflow("regen-timeline.yml.template")

	// Marker bump v9 → v10 (adoption gate removed; the invariant is now
	// unconditional).
	if !strings.Contains(body, "# logmind-template-version: v10") {
		t.Errorf("regen-timeline template missing v10 marker")
	}
	// The required-check name MUST stay check-derived-docs (ruleset matching),
	// and the main regen is a distinct job.
	if !strings.Contains(body, "check-derived-docs") {
		t.Errorf("regen-timeline v10 must keep the check-derived-docs job name (ruleset matching)")
	}
	if !strings.Contains(body, "regen-on-main") {
		t.Errorf("regen-timeline v10 missing the separate regen-on-main job")
	}
	// The PR gate BLOCKS UNCONDITIONALLY: a branch edit to a derived doc must
	// `exit 1`, for every repo — no adoption check gates it anymore.
	if !strings.Contains(body, "exit 1") {
		t.Errorf("regen-timeline v10 PR gate must `exit 1` on a derived-doc edit (blocking)")
	}
	// The gate detects the edit via the PR's own file list (GitHub's 3-dot
	// diff = branch-vs-merge-base, fork-correct) and matches ONLY the two
	// derived docs as whole lines.
	if !strings.Contains(body, "gh pr diff") || !strings.Contains(body, "--name-only") {
		t.Errorf("regen-timeline v10 PR gate must use `gh pr diff --name-only`")
	}
	if !strings.Contains(body, `grep -qxE 'docs/(timeline|file-structure)\.md'`) {
		t.Errorf("regen-timeline v10 PR gate must match exactly the two derived docs as whole lines")
	}
	// Event-gated jobs: gate runs only on pull_request, regen only on push.
	if !strings.Contains(body, "github.event_name == 'pull_request'") ||
		!strings.Contains(body, "github.event_name == 'push'") {
		t.Errorf("regen-timeline v10 must event-gate the two jobs (pull_request gate / push regen)")
	}
	// The main regen commit carries the [skip-logmind] convention and pushes
	// via the explicit PAT URL (never a persisted/GITHUB_TOKEN credential).
	if !strings.Contains(body, "[skip-logmind]") {
		t.Errorf("regen-timeline v10 main regen commit missing the [skip-logmind] prefix")
	}
	if !strings.Contains(body, "x-access-token:${PAT}") {
		t.Errorf("regen-timeline v10 main push must use the explicit PAT URL")
	}
	if !strings.Contains(body, "persist-credentials: false") {
		t.Errorf("regen-timeline v10 must set persist-credentials: false on regen-on-main's checkout")
	}
	// The PR gate reads the PR file list via `gh pr diff`, which needs
	// pull-requests:read. Because specifying ANY permission zeroes the rest,
	// this MUST be explicit or the gate 403s and fails-closed on every PR.
	if !strings.Contains(body, "pull-requests: read") {
		t.Errorf("regen-timeline v10 must grant pull-requests: read (else gh pr diff 403s and blocks every PR)")
	}
	// No-PAT is a freshness-only gap, not a failure: warn + exit 0 (never blocks
	// the push event; the invariant guarantee lives in the PR gate, not here).
	if !strings.Contains(body, "::warning") || !strings.Contains(body, "exit 0") {
		t.Errorf("regen-timeline v10 regen-on-main must warn + exit 0 when no PAT (freshness-only)")
	}
	// The push-rejection warning must not promise self-healing — a missing
	// ruleset bypass (GH013) is a POLICY refusal that repeats every cycle,
	// not a transient one-off; see docs/orchestrator-app.md.
	if !strings.Contains(body, "GH013") {
		t.Errorf("regen-timeline v10 push-rejection warning must name GH013 as the likely cause")
	}
	if !strings.Contains(body, "NOT self-heal") {
		t.Errorf("regen-timeline v10 warnings must not promise the staleness resolves itself on the next merge")
	}

	// v10 removes the entire adoption gate: no per-repo config read, no
	// base-ref fetch, no "hasn't adopted → pass" escape. Pin all three gone —
	// a regression here would silently reopen the v9 opt-out.
	if strings.Contains(body, ".logmind/config.yml") {
		t.Errorf("regen-timeline v10 PR gate must NOT read .logmind/config.yml — the invariant is unconditional, no adoption signal to check")
	}
	// A grep against a `mode: integration-point` line would be the OPERATIONAL
	// gate reappearing — the header comment's historical mention of the old
	// key name (explaining what v10 removed) is fine; a grep pattern isn't.
	if strings.Contains(body, `mode:[[:space:]]*"?integration-point"?`) {
		t.Errorf("regen-timeline v10 must not grep for an integration-point mode line — the opt-in gate is gone")
	}
	if strings.Contains(body, "has not adopted") {
		t.Errorf("regen-timeline v10 PR gate must not have a not-adopted pass-through message")
	}
	if strings.Contains(body, "pull_request.base.sha") || strings.Contains(body, "BASE_SHA") {
		t.Errorf("regen-timeline v10 must not read a base-ref adoption signal — there is nothing left to read")
	}

	// SECURITY — even with the adoption signal gone, this job must still
	// never check out the PR: a checkout on a pull_request event lands
	// refs/pull/N/merge (the PR's own content), and this job has no business
	// trusting anything from the PR beyond its file list (`gh pr diff`,
	// which talks to the API, not the workspace).
	prJob, _, found := strings.Cut(body, "  regen-on-main:")
	if !found {
		t.Fatalf("regen-timeline v10: could not locate the regen-on-main job boundary")
	}
	// Match the STEP INVOCATION, not the bare action name — the security
	// comment in the job body legitimately mentions `actions/checkout` while
	// explaining why there isn't one.
	if strings.Contains(prJob, "uses: actions/checkout") {
		t.Errorf("regen-timeline v10 PR gate must NOT check out the PR — a checkout on a pull_request event lands refs/pull/N/merge (the PR's own content)")
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

// repoRootFromCaller walks up from the test binary's working directory
// (which `go test` sets to the package source directory) to find the
// repo root, identified by go.mod.
func repoRootFromCaller(t *testing.T) string {
	t.Helper()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	dir := wd
	for i := 0; i < 8; i++ {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	t.Fatalf("could not locate go.mod walking up from %s", wd)
	return ""
}

// TestRegenTimelineWorkflow_LockstepWithTemplate is a REAL pair-diff
// lockstep test: before this, nothing actually diffed the installed
// .github/workflows/regen-timeline.yml against the template this repo
// hands to every other consumer repo
// (internal/templates/github/regen-timeline.yml.template) — that parity
// was maintained by hand, silently, which is how they drift.
//
// The two files are DELIBERATELY not byte-identical in exactly 3 ways:
//
//  1. The template's leading `# logmind-template-version: vN` marker
//     line — the installed workflow isn't scaffolded from a template
//     version, it IS the checked-in source of truth this repo runs on
//     itself.
//  2. The build mechanism: a consumer repo installs a released logmind
//     binary via `thrillmade/setup-logmind`; this repo instead builds
//     its own in-tree source (`actions/setup-go` + `make build` +
//     `./bin/logmind`) so its own CI always exercises the code under
//     review, never a stale release.
//  3. The credential block: a consumer repo configures a
//     LOGMIND_AUTO_REGEN_PAT secret directly; this repo mints a
//     short-lived `skdd-steward[bot]` GitHub App installation token
//     instead (see docs/orchestrator-app.md) — a different identity,
//     the same "degrade, never fail" push contract.
//
// Everything else — MOST IMPORTANTLY the check-derived-docs job, which
// is the actual PR-blocking enforcement logic — must be byte-identical,
// or logmind's own CI would be exercising different gate behavior than
// what every other repo installs. This test fails loudly if an edit to
// either file's gate logic isn't mirrored on the other side, and it
// fails loudly (pointing at the stale literal) if one of the 3
// documented differences itself changes shape, forcing a human to
// re-verify the divergence is still exactly what's documented above.
func TestRegenTimelineWorkflow_LockstepWithTemplate(t *testing.T) {
	repoRoot := repoRootFromCaller(t)
	wfBytes, err := os.ReadFile(filepath.Join(repoRoot, ".github", "workflows", "regen-timeline.yml"))
	if err != nil {
		t.Fatalf("read installed workflow: %v", err)
	}
	workflow := string(wfBytes)
	tmpl := Workflow("regen-timeline.yml.template")

	// Difference 1: strip the template's leading version-marker line.
	const markerPrefix = "# logmind-template-version: v"
	nl := strings.IndexByte(tmpl, '\n')
	if nl < 0 || !strings.HasPrefix(tmpl, markerPrefix) {
		t.Fatalf("template does not start with the expected %q marker line; got first line %q",
			markerPrefix, tmpl[:min(len(tmpl), 60)])
	}
	tmplBody := tmpl[nl+1:]

	// --- Job 1: check-derived-docs (the PR gate) + everything above
	// regen-on-main (name, header comments, on:, permissions:). This
	// slice carries NONE of the 3 documented differences, so it must be
	// byte-identical between the two files.
	const splitAnchor = "  regen-on-main:\n"
	tmplGateEnd := strings.Index(tmplBody, splitAnchor)
	wfGateEnd := strings.Index(workflow, splitAnchor)
	if tmplGateEnd < 0 {
		t.Fatalf("template missing %q — regen-timeline.yml.template structure drifted", splitAnchor)
	}
	if wfGateEnd < 0 {
		t.Fatalf("installed workflow missing %q — regen-timeline.yml structure drifted", splitAnchor)
	}
	tmplGate := tmplBody[:tmplGateEnd]
	wfGate := workflow[:wfGateEnd]
	if tmplGate != wfGate {
		t.Fatalf("regen-timeline.yml's header/check-derived-docs job (the PR gate) drifted from its "+
			"template — a gate-logic edit on one side wasn't mirrored on the other:\n"+
			"--- template ---\n%s\n--- installed workflow ---\n%s", tmplGate, wfGate)
	}

	// --- Job 2: regen-on-main. Walk a fixed sequence of anchors that
	// MUST appear, in order, identically in both files' regen-on-main
	// section. The gaps between anchors are exactly where the build
	// mechanism (gap 1), the credential env var (gap 2), and the rest of
	// the credential block through EOF (gap 3, unanchored) are allowed
	// to diverge. Anything else — an anchor going missing or moving —
	// means the surrounding structure itself changed on only one side.
	anchors := []string{
		splitAnchor +
			"    name: regen-on-main\n" +
			"    if: github.event_name == 'push'\n" +
			"    runs-on: ubuntu-latest\n" +
			"    steps:\n" +
			"      - uses: actions/checkout@v7\n" +
			"        with:\n" +
			"          fetch-depth: 0\n" +
			"          persist-credentials: false\n",
		// gap 1: build mechanism (setup-logmind vs setup-go+make build)
		// and the "Regenerate derived docs" step body (logmind vs
		// ./bin/logmind).
		"      - name: Commit + push if changed\n" +
			"        env:\n",
		// gap 2: the single credential env var line (PAT vs STEWARD_TOKEN).
		"        run: |\n" +
			"          set -euo pipefail\n" +
			"          if [ -z \"$(git status --porcelain -- docs/timeline.md docs/file-structure.md)\" ]; then\n" +
			"            echo \"Derived docs already current on main.\"\n" +
			"            exit 0\n" +
			"          fi\n",
		// gap 3 (unanchored, runs to EOF): the credential-missing check,
		// the git identity, the push-failure comment prose, and the
		// push URL's token variable — all keyed on which identity this
		// repo vs. a consumer repo pushes as.
	}
	requireAnchorsInOrder(t, "template", tmplBody[tmplGateEnd:], anchors)
	requireAnchorsInOrder(t, "installed workflow", workflow[wfGateEnd:], anchors)
}

// requireAnchorsInOrder asserts every string in anchors appears in body,
// in order, as an exact substring — t.Fatal with the first one that
// doesn't, naming which anchor and where the search gave up.
func requireAnchorsInOrder(t *testing.T, label, body string, anchors []string) {
	t.Helper()
	pos := 0
	for i, a := range anchors {
		idx := strings.Index(body[pos:], a)
		if idx < 0 {
			end := pos + 200
			if end > len(body) {
				end = len(body)
			}
			t.Fatalf("%s: regen-on-main anchor %d not found at/after offset %d "+
				"(regen-timeline.yml structure drifted from the lockstep test's anchors — "+
				"update TestRegenTimelineWorkflow_LockstepWithTemplate):\nwant substring:\n%q\n"+
				"--- body from offset %d ---\n%s", label, i, pos, a, pos, body[pos:end])
		}
		pos += idx + len(a)
	}
}
