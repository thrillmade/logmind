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
// logmind-self-update and check-decisions are all asserted here.
func TestWorkflowTemplates_UseSetupLogmindAction(t *testing.T) {
	for _, name := range []string{
		"check-doc-links.yml.template",
		"regen-timeline.yml.template",
		"logmind-self-update.yml.template",
		"check-decisions.yml.template",
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
		"check-decisions.yml.template",
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
//
// v11 (logmind#262): regen-on-main's push step stops conflating "no
// credential" with "push refused". SPEC-2 §3.3 requires three outcomes
// that MUST NOT look alike — nothing-to-push and no-credential still exit
// 0 (the latter with a `::warning`, since a merge that was otherwise fine
// should not fail over a missing secret), but a push that was attempted
// and refused now fails the job (`::error` + `exit 1`): "a refused write
// is the job not doing its job", and reporting it as success is how a
// regenerator stayed green for eleven days while doing nothing.
func TestRegenTimelineTemplate_V10_UnconditionalBlockingGate(t *testing.T) {
	body := Workflow("regen-timeline.yml.template")

	// Marker bump v11 → v12 (the push credential is now a degrading chain;
	// see TestRegenTimelineTemplate_V12_CredentialChainDegrades).
	if !strings.Contains(body, "# logmind-template-version: v12") {
		t.Errorf("regen-timeline template missing v12 marker")
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
	// via an explicit credentialed URL built from the chain's chosen rung —
	// never a persisted credential.
	if !strings.Contains(body, "[skip-logmind]") {
		t.Errorf("regen-timeline v10 main regen commit missing the [skip-logmind] prefix")
	}
	if !strings.Contains(body, "x-access-token:${TOKEN}") {
		t.Errorf("regen-timeline v12 main push must use the explicit credentialed URL built from the chain's chosen rung")
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
	// No credential AT ALL is a freshness-only gap, not a failure: warn +
	// exit 0 (never blocks the push event; the invariant guarantee lives in
	// the PR gate, not here). Scoped to the no-credential branch
	// specifically — SPEC-2 §3.3 requires the push-refusal branch below to
	// look visibly different, not merely for "::warning" to appear somewhere
	// in the file. v12 keeps this branch even though rung 3 (GITHUB_TOKEN)
	// makes it nearly unreachable: a workflow whose token was scoped away
	// must still degrade rather than fail.
	_, noCredBlock, found := strings.Cut(body, "\n          else\n")
	if !found {
		t.Fatalf("regen-timeline v12 missing the no-credential fall-through of the chain")
	}
	noCredBlock, _, found = strings.Cut(noCredBlock, "fi\n")
	if !found {
		t.Fatalf("regen-timeline v12: could not find the end of the no-credential block")
	}
	if !strings.Contains(noCredBlock, "::warning") || !strings.Contains(noCredBlock, "exit 0") {
		t.Errorf("regen-timeline v12 regen-on-main must warn + exit 0 when NO credential of any kind is available (freshness-only)")
	}
	if strings.Contains(noCredBlock, "::error") || strings.Contains(noCredBlock, "exit 1") {
		t.Errorf("regen-timeline v12 no-credential branch must degrade, not fail — it MUST NOT look like the push-refusal case")
	}

	// SPEC-2 §3.3 (logmind#262): "A push that was attempted and refused is
	// a failure, and MUST be reported as one." This is a DIFFERENT outcome
	// from the no-credential case above and must fail the job loudly
	// (::error + exit 1), never degrade quietly (::warning + exit 0) —
	// that conflation is exactly what let the regen job stay green for
	// eleven days while doing nothing.
	_, pushBlock, found := strings.Cut(body, `if ! git push "https://x-access-token:${TOKEN}@github.com/${GITHUB_REPOSITORY}.git" "HEAD:${DEFAULT_BRANCH}"; then`)
	if !found {
		t.Fatalf("regen-timeline v12 missing the push-refusal check")
	}
	if !strings.Contains(pushBlock, "::error") {
		t.Errorf("regen-timeline v12 push-refusal must be reported with ::error — it MUST NOT look like the no-credential ::warning case")
	}
	if !strings.Contains(pushBlock, "exit 1") {
		t.Errorf("regen-timeline v12 push-refusal must fail the job (exit 1), not degrade (exit 0)")
	}
	if strings.Contains(pushBlock, "::warning") {
		t.Errorf("regen-timeline v12 push-refusal must not also emit a ::warning — the two outcomes must be visibly distinct")
	}
	// The push-rejection error must not promise self-healing — a missing
	// ruleset bypass (GH013) is a POLICY refusal that repeats every cycle,
	// not a transient one-off; see docs/orchestrator-app.md.
	if !strings.Contains(body, "GH013") {
		t.Errorf("regen-timeline v12 push-rejection error must name GH013 as the likely cause")
	}
	if !strings.Contains(body, "NOT self-heal") {
		t.Errorf("regen-timeline v12 messaging must not promise the staleness resolves itself on the next merge")
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

	// Marker bump v8 → v9 (setup-logmind action pin v1.0.0 → v1.0.1).
	if !strings.Contains(body, "# logmind-template-version: v9") {
		t.Errorf("check-doc-links template missing v9 marker")
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

// TestCheckDecisionsTemplate_V5_CallsTheVerb pins the v5 shape: the
// workflow INVOKES `logmind check-decisions --base/--head` instead of
// reimplementing the decision rule in bash. SPEC §3.4: "Both interception
// points and the gate MUST use this one list. Two lists that mean the
// same thing are two lists that will disagree" — v4's bash was that
// second list, and each assertion below pins one of the five ways it had
// already drifted from the verb.
func TestCheckDecisionsTemplate_V5_CallsTheVerb(t *testing.T) {
	body := Workflow("check-decisions.yml.template")
	// The must-NOT-contain assertions below run against the workflow with
	// whole-line comments removed, so they bind what the job DOES rather
	// than what its header says about what v4 used to do — the header
	// names every removed mechanism, and naming them is the point.
	active := stripCommentLines(body)

	// Marker bump v5 → v6 (setup-logmind action pin v1.0.0 → v1.0.1).
	if !strings.Contains(body, "# logmind-template-version: v6") {
		t.Errorf("check-decisions template missing v6 marker")
	}

	// The one thing this workflow does: call the verb over the PR's range.
	if !strings.Contains(body, `logmind check-decisions --base "$BASE_SHA" --head "$HEAD_SHA"`) {
		t.Errorf("check-decisions v5 must invoke `logmind check-decisions --base \"$BASE_SHA\" --head \"$HEAD_SHA\"` — the gate does not reimplement the rule")
	}
	if !strings.Contains(body, "BASE_SHA: ${{ github.event.pull_request.base.sha }}") ||
		!strings.Contains(body, "HEAD_SHA: ${{ github.event.pull_request.head.sha }}") {
		t.Errorf("check-decisions v5 missing the BASE_SHA/HEAD_SHA env pair the verb's range is built from")
	}

	// Drift 1: `*.md` in the exclusion list. §3.4 forbids it outright —
	// "Excluding markdown wholesale switches the rule off in the
	// repositories where writing is the work." Drift 5: the exclusion
	// list itself, which now lives only in guardcommit.IsExcludedPath.
	if strings.Contains(active, "docs/*|.logmind/*") || strings.Contains(active, "|*.md)") {
		t.Errorf("check-decisions v5 must not carry its own exclusion `case` — one list, in the verb (SPEC §3.4)")
	}

	// Drift 2: the PR-title read. §3.4: "The gate has no self-service
	// escape, and MUST NOT be given one." Not relocated — deleted.
	for _, banned := range []string{
		"gh pr view",
		"skip_logmind",
		"[skip-logmind]",
		"github.event.pull_request.title",
		"EVENT_TITLE",
	} {
		if strings.Contains(active, banned) {
			t.Errorf("check-decisions v5 must not reference %q — the gate has no self-service escape (SPEC §3.4)", banned)
		}
	}
	// ...and with the title fetch gone, so is the permission it needed.
	if strings.Contains(active, "pull-requests:") {
		t.Errorf("check-decisions v5 must not request pull-requests permission — nothing here reads PR data via the API")
	}

	// Drift 3: a path match standing in for §3.1 shape. §3.4: the gate
	// "MUST NOT be satisfied by the decision file merely appearing in the
	// diff."
	if strings.Contains(active, "git diff --name-only") || strings.Contains(active, "decision_touched") {
		t.Errorf("check-decisions v5 must not decide on a decision-file path match — the verb checks §3.1 shape")
	}

	// Drift 4: a hardcoded threshold. It is git.commit_line_threshold,
	// which the verb reads; the workflow must not pass or restate it.
	if strings.Contains(active, "THRESHOLD") || strings.Contains(active, "--threshold") {
		t.Errorf("check-decisions v5 must not hardcode or pass a threshold — it is git.commit_line_threshold, read by the verb")
	}

	// SPEC §6.3: a gate is never satisfiable by the change it judges. The
	// binary comes from the pinned release action; a PR that could rebuild
	// it from its own checkout would be rewriting its own gate.
	if !strings.Contains(active, "uses: thrillmade/setup-logmind@v") {
		t.Errorf("check-decisions v5 must install the binary via the pinned thrillmade/setup-logmind action (SPEC §6.3)")
	}
	for _, banned := range []string{"setup-go", "make build", "go build", "./bin/logmind"} {
		if strings.Contains(active, banned) {
			t.Errorf("check-decisions v5 must not build logmind from the PR's checkout (%q) — SPEC §6.3", banned)
		}
	}

	// §6.3 again, on the OTHER input: the threshold lives in the
	// repository's configuration, which MUST be read from the base ref.
	// A default pull_request checkout lands the merge ref, and a PR could
	// then raise the threshold it is judged against in the same diff.
	if !strings.Contains(active, "ref: ${{ github.event.pull_request.base.sha }}") {
		t.Errorf("check-decisions v5 must check out the BASE ref, so config comes from the base (SPEC §6.3)")
	}

	// The failure path stays actionable: it names the command to run.
	if !strings.Contains(active, `logmind log \"summary\" -r \"reasoning\" -a \"alternative\" -i \"implication\"`) {
		t.Errorf("check-decisions v5 failure annotation must tell the author what to run")
	}
}

// stripCommentLines drops every whole-line `#` comment (YAML's and, inside
// a `run:` block, the shell's — they are the same syntax and neither one
// executes). What survives is what the workflow actually does.
func stripCommentLines(body string) string {
	var kept []string
	for _, line := range strings.Split(body, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "#") {
			continue
		}
		kept = append(kept, line)
	}
	return strings.Join(kept, "\n")
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
// v12 narrowed the permitted divergence from three ways to two, and the
// one it removed is the one that mattered. Until v12 the entire
// credential block was free to differ: the template shipped a raw
// LOGMIND_AUTO_REGEN_PAT while this repo had already moved to a
// short-lived GitHub App installation token, and NOTHING failed, because
// the anchor list stopped before that block. `logmind init` was therefore
// deploying to the whole fleet the credential path this repo had
// abandoned. The mechanism is now identical on both sides — one degrading
// chain (App token → PAT → GITHUB_TOKEN) — and this test diffs it
// BYTE-FOR-BYTE, so the next edit to it on one side and not the other
// fails here instead of shipping.
//
// What may still differ, and only this:
//
//  1. The template's leading `# logmind-template-version: vN` marker
//     line — the installed workflow isn't scaffolded from a template
//     version, it IS the checked-in source of truth this repo runs on
//     itself.
//  2. The scaffold-time default-branch placeholder
//     (__LOGMIND_DEFAULT_BRANCH__), which `logmind init` renders into each
//     consumer repo. This repo is not scaffolded, so its copy carries the
//     rendered value; the test renders the template the same way before
//     comparing, so this costs no permitted divergence at all.
//  3. Two lines: the names of the secrets holding the OPTIONAL GitHub App
//     credentials (a consumer repo uses LOGMIND_APP_ID /
//     LOGMIND_APP_PRIVATE_KEY; this repo's org secrets keep their legacy
//     THRILLMADE_ORCHESTRATOR_* names — see docs/orchestrator-app.md).
//     Names only: the keys, their order, and every line of logic that
//     reads them are identical, and this test proves it.
//  4. The build mechanism: a consumer repo installs a released logmind
//     binary via `thrillmade/setup-logmind`; this repo instead builds
//     its own in-tree source (`actions/setup-go` + `make build` +
//     `./bin/logmind`) so its own CI always exercises the code under
//     review, never a stale release.
//
// Everything else — the header, the check-derived-docs job (the actual
// PR-blocking enforcement logic), the checkout stanza, and the whole
// credential mechanism through EOF — must be byte-identical, or logmind's
// own CI would be exercising different behaviour than what every other
// repo installs.
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

	// Difference 2: the scaffold-time default-branch placeholder. The
	// installed file is this repo's own copy, so it carries the RENDERED
	// value where the template carries the placeholder. Substituting it
	// here — rather than carving out another region the pair is allowed to
	// differ in — keeps every region below byte-identical, which is the
	// stronger property. The guard above it is what stops this substitution
	// from becoming a way to hide a re-hardcoded `main`: if the placeholder
	// ever disappears from the template, this fails loudly rather than
	// quietly comparing two hardcoded copies.
	const branchPlaceholder = "__LOGMIND_DEFAULT_BRANCH__"
	if !strings.Contains(tmplBody, branchPlaceholder) {
		t.Fatalf("regen-timeline template no longer contains %s — the default branch has been "+
			"hardcoded again, which is the defect this placeholder exists to prevent", branchPlaceholder)
	}
	tmplBody = strings.ReplaceAll(tmplBody, branchPlaceholder, "main")

	// The four boundaries that carve both files into the same regions. Each
	// must appear exactly once in each file, or the structure this test
	// reasons about has changed on one side only.
	// Each carries a leading newline so the indentation is part of the
	// match: bare "    env:\n" is a substring of the push step's
	// "        env:\n" and would silently pick the wrong boundary.
	const (
		jobAnchor    = "\n  regen-on-main:\n"
		envAnchor    = "\n    env:\n"
		stepsAnchor  = "\n    steps:\n"
		buildEnd     = "\n      # Rung 1: mint a short-lived (1 hour) App installation token"
		checkoutTail = "\n          persist-credentials: false\n"
	)

	// --- Region A: header + check-derived-docs (the PR gate). Carries none
	// of the permitted differences → byte-identical.
	tmplA, tmplRest := splitOnceOrFail(t, "template", tmplBody, jobAnchor)
	wfA, wfRest := splitOnceOrFail(t, "installed workflow", workflow, jobAnchor)
	requireIdentical(t, "header + check-derived-docs job (the PR gate)", tmplA, wfA)

	// --- Region B: regen-on-main's preamble through `env:`. This is the
	// prose that tells a repo owner the App rung is optional, so it must
	// read identically in both.
	tmplB, tmplRest := splitOnceOrFail(t, "template", tmplRest, envAnchor)
	wfB, wfRest := splitOnceOrFail(t, "installed workflow", wfRest, envAnchor)
	requireIdentical(t, "regen-on-main preamble (through `env:`)", jobAnchor+tmplB, jobAnchor+wfB)

	// --- Region C (difference 2): the App-credential env block. The two
	// sides name different secrets; they must NOT differ in any other way.
	tmplC, tmplRest := splitOnceOrFail(t, "template", tmplRest, stepsAnchor)
	wfC, wfRest := splitOnceOrFail(t, "installed workflow", wfRest, stepsAnchor)
	requireAppCredentialEnvBlock(t, "template", tmplC)
	requireAppCredentialEnvBlock(t, "installed workflow", wfC)

	// --- Region D: the checkout stanza. Identical.
	tmplD, tmplRest := splitOnceOrFail(t, "template", tmplRest, checkoutTail)
	wfD, wfRest := splitOnceOrFail(t, "installed workflow", wfRest, checkoutTail)
	requireIdentical(t, "regen-on-main checkout stanza", stepsAnchor+tmplD, stepsAnchor+wfD)

	// --- Region E (difference 3): the build mechanism. Free to diverge —
	// but each side must still be the mechanism it claims to be, or a
	// silent swap here would go unnoticed.
	_, tmplRest = splitOnceOrFail(t, "template", tmplRest, buildEnd)
	_, wfRest = splitOnceOrFail(t, "installed workflow", wfRest, buildEnd)

	// --- Region F: the credential mechanism, through EOF. THE region this
	// test exists for. Byte-identical.
	requireIdentical(t, "regen-on-main credential mechanism (App token → PAT → GITHUB_TOKEN, and the push)",
		buildEnd+tmplRest, buildEnd+wfRest)
}

// splitOnceOrFail cuts body at sep, requiring sep to appear EXACTLY once.
// Returns the part before sep and the part after. A second occurrence
// means the region boundaries no longer carve the file the way this test
// assumes, which is itself a drift worth failing on.
func splitOnceOrFail(t *testing.T, label, body, sep string) (before, after string) {
	t.Helper()
	if n := strings.Count(body, sep); n != 1 {
		t.Fatalf("%s: expected exactly 1 occurrence of the region boundary %q in the remaining body, got %d — "+
			"regen-timeline's structure drifted from the lockstep test's regions "+
			"(update TestRegenTimelineWorkflow_LockstepWithTemplate)", label, sep, n)
	}
	before, after, _ = strings.Cut(body, sep)
	return before, after
}

// requireIdentical fails with a readable pair-diff when two regions that
// MUST be byte-identical are not.
func requireIdentical(t *testing.T, region, tmpl, workflow string) {
	t.Helper()
	if tmpl == workflow {
		return
	}
	t.Fatalf("regen-timeline.yml's %s drifted from its template — an edit on one side "+
		"wasn't mirrored on the other:\n--- template ---\n%s\n--- installed workflow ---\n%s",
		region, tmpl, workflow)
}

// requireAppCredentialEnvBlock pins the ONE region the pair is allowed to
// differ in beyond the build mechanism: the job-level env block naming the
// optional GitHub App credentials. Both sides must declare exactly APP_ID
// and APP_PRIVATE_KEY, in that order, each sourced from a `secrets.*`
// expression — the secret NAMES may differ, nothing else may. Without this
// the "two lines" claim in the doc comment above would be unenforced, and
// a whole mechanism could reappear inside this gap, which is exactly how
// v11 shipped a PAT the repo had stopped using.
func requireAppCredentialEnvBlock(t *testing.T, label, block string) {
	t.Helper()
	var keys []string
	for _, line := range strings.Split(block, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		key, value, ok := strings.Cut(trimmed, ": ")
		if !ok {
			t.Errorf("%s: unparseable line in the App-credential env block: %q", label, line)
			continue
		}
		if !strings.HasPrefix(value, "${{ secrets.") || !strings.HasSuffix(value, " }}") {
			t.Errorf("%s: App-credential env value for %q must be a `${{ secrets.* }}` expression, got %q",
				label, key, value)
		}
		keys = append(keys, key)
	}
	want := []string{"APP_ID", "APP_PRIVATE_KEY"}
	if len(keys) != len(want) {
		t.Fatalf("%s: App-credential env block must declare exactly %v, got %v — "+
			"anything else here is mechanism escaping the region this pair is allowed to differ in",
			label, want, keys)
	}
	for i := range want {
		if keys[i] != want[i] {
			t.Errorf("%s: App-credential env block key %d is %q, want %q", label, i, keys[i], want[i])
		}
	}
}

// TestRegenTimelineTemplate_V12_CredentialChainDegrades pins the v12
// credential contract as BEHAVIOUR rather than as one secret's name.
//
// The ruling this encodes: logmind must work standalone. It is not a
// thrillmade-only tool and must not require an organization or a GitHub
// App, so the push credential degrades — App installation token if App
// credentials are configured, else a PAT if one is, else the workflow's
// own GITHUB_TOKEN — and it MUST NEVER silently do nothing. The App rung
// is an optimization (it is the only identity that can be granted a
// ruleset bypass), never a dependency: nothing may break when the App
// secrets are absent, which is the common case for every repo outside
// thrillmade.
//
// v11's shape — one hardcoded `secrets.LOGMIND_AUTO_REGEN_PAT`, with "no
// PAT" meaning "warn and do nothing" — is asserted GONE, since that is
// the regression this guards.
func TestRegenTimelineTemplate_V12_CredentialChainDegrades(t *testing.T) {
	body := Workflow("regen-timeline.yml.template")

	// Rung 1 — the App token, and it must be OPTIONAL on both axes: the
	// step is skipped when no App credentials are configured, AND a
	// configured-but-broken App degrades instead of failing the job.
	appStep, _, found := strings.Cut(body, "\n      - name: Commit + push if changed\n")
	if !found {
		t.Fatalf("regen-timeline v12: could not locate the push step")
	}
	_, appStep, found = strings.Cut(appStep, "\n      - name: Mint GitHub App installation token")
	if !found {
		t.Fatalf("regen-timeline v12 missing the App-token rung of the credential chain")
	}
	if !strings.Contains(appStep, "uses: actions/create-github-app-token@v") {
		t.Errorf("regen-timeline v12 App rung must mint the token via actions/create-github-app-token")
	}
	if !strings.Contains(appStep, "if: env.APP_ID != '' && env.APP_PRIVATE_KEY != ''") {
		t.Errorf("regen-timeline v12 App rung must be skipped when no App credentials are configured — " +
			"the App is an optimization, not a dependency, and an unconfigured repo must not see a failing step")
	}
	if !strings.Contains(appStep, "continue-on-error: true") {
		t.Errorf("regen-timeline v12 App rung must be continue-on-error — a misconfigured or expired App " +
			"must degrade to the next rung, never fail the job")
	}
	// A shipped template must not hardcode one organization's App
	// installation: `owner`/`repositories` default to the current repo.
	for _, banned := range []string{"owner:", "repositories:"} {
		if strings.Contains(appStep, banned) {
			t.Errorf("regen-timeline v12 App rung must not pin %q — it defaults to the current repository, "+
				"and a shipped template has no business naming someone else's org", banned)
		}
	}

	// The three rungs are wired into the push step's env, and only there.
	push := body[strings.Index(body, "\n      - name: Commit + push if changed\n"):]
	for _, want := range []string{
		"APP_TOKEN: ${{ steps.app_token.outputs.token }}",
		"PAT: ${{ secrets.LOGMIND_AUTO_REGEN_PAT }}",
		"DEFAULT_TOKEN: ${{ github.token }}",
	} {
		if !strings.Contains(push, want) {
			t.Errorf("regen-timeline v12 push step missing credential-chain input %q", want)
		}
	}

	// ORDER is the contract, not merely presence: App, then PAT, then
	// GITHUB_TOKEN. A chain that tried GITHUB_TOKEN first would push as an
	// identity that can never hold a ruleset bypass and would fail forever
	// on a protected branch while an App token sat unused.
	rungs := []string{
		`if [ -n "${APP_TOKEN:-}" ]; then`,
		`elif [ -n "${PAT:-}" ]; then`,
		`elif [ -n "${DEFAULT_TOKEN:-}" ]; then`,
	}
	pos := 0
	for i, r := range rungs {
		idx := strings.Index(push[pos:], r)
		if idx < 0 {
			t.Fatalf("regen-timeline v12 credential chain: rung %d (%q) missing or out of order — "+
				"the order App → PAT → GITHUB_TOKEN is the contract", i+1, r)
		}
		pos += idx + len(r)
	}

	// Rung 3 always exists, so "configured nothing" ATTEMPTS the push. A
	// template that only ever warned when no PAT was configured is exactly
	// the v11 defect: never silently do nothing.
	if strings.Contains(body, `if [ -z "${PAT:-}" ]; then`) {
		t.Errorf("regen-timeline v12 must not gate the whole push on a PAT being configured — " +
			"that is v11's shape, where a repo with no PAT warned and did nothing")
	}

	// The push uses whichever rung won, and the failure message names it,
	// so a red run says which identity was refused rather than assuming a
	// PAT.
	if !strings.Contains(push, `git push "https://x-access-token:${TOKEN}@github.com/${GITHUB_REPOSITORY}.git" "HEAD:${DEFAULT_BRANCH}"`) {
		t.Errorf("regen-timeline v12 push must authenticate with ${TOKEN} — the rung the chain selected")
	}
	if !strings.Contains(push, "$RUNG") {
		t.Errorf("regen-timeline v12 must report WHICH rung it used — a refusal that does not name the " +
			"identity it tried is not actionable")
	}
}

// TestRegenTimelineTemplate_V12_DefaultBranchIsResolvedNotHardcoded pins
// the second half of the standalone ruling: logmind must assume nothing
// about how a repository is set up, and "the default branch is called
// main" is that assumption in the same shape as the abandoned credential
// path.
//
// v11 hardcoded `main` in four operative places — the push refspec
// (`HEAD:main`), the `on: push:` filter, the PR gate's remediation
// command (`git checkout origin/main -- …`), and the GH013 ruleset error.
// A repo on `master` or `trunk` therefore either pushed the regen to a
// branch it does not have, or never ran the regen at all.
//
// Everything evaluated at RUNTIME now reads
// `github.event.repository.default_branch` (verified present on both the
// push and pull_request webhook payloads). The `on:` filter is the one
// thing that cannot — GitHub evaluates no context there — so it is a
// scaffold-time placeholder, and the gate warns when the two drift.
func TestRegenTimelineTemplate_V12_DefaultBranchIsResolvedNotHardcoded(t *testing.T) {
	body := Workflow("regen-timeline.yml.template")

	// No literal `main` may survive anywhere the workflow ACTS on it. Run
	// against the workflow with whole-line comments stripped, the same way
	// the check-decisions test does: the v12 header names every hardcoded
	// form it removed, and naming them is the point — what must not come
	// back is the form that EXECUTES. The job id/name also keep the word,
	// since the decision log refers to them by it.
	active := stripCommentLines(body)
	for _, banned := range []string{
		"HEAD:main",                // the push refspec — the wrong-ref write
		"origin/main",              // the PR gate's remediation command
		"branches: [main]",         // the trigger
		"branches: [main, master]", // a longer guess is still a guess
	} {
		if strings.Contains(active, banned) {
			t.Errorf("regen-timeline v12 still hardcodes the default branch as %q — "+
				"a repo on master/trunk is broken by this", banned)
		}
	}

	// The trigger is scaffolded, not guessed. `logmind init` substitutes
	// the repository's real default branch here.
	if !strings.Contains(body, "branches: [__LOGMIND_DEFAULT_BRANCH__]") {
		t.Errorf("regen-timeline v12 `on: push:` filter must be the scaffold placeholder " +
			"`branches: [__LOGMIND_DEFAULT_BRANCH__]` — an `on:` filter cannot take an expression, " +
			"so this is the only way it is not a hardcoded guess")
	}

	// Runtime reads the live value, in both jobs.
	if !strings.Contains(body, "DEFAULT_BRANCH: ${{ github.event.repository.default_branch }}") {
		t.Errorf("regen-timeline v12 must resolve the default branch from the event payload")
	}
	// …and it reaches the script through env, never `${{ }}` inside `run:`
	// — a branch name is attacker-influenceable text and `run:` is a
	// script-injection site.
	if strings.Contains(body, "${{ github.event.repository.default_branch }}\n          run:") ||
		strings.Contains(body, "HEAD:${{") {
		t.Errorf("regen-timeline v12 must not interpolate the branch name directly into a `run:` block")
	}

	// The regen job is gated on the live value, so a stale scaffolded
	// trigger can cost a run but can never cause a push to a non-default
	// branch.
	if !strings.Contains(body, "github.ref_name == github.event.repository.default_branch") {
		t.Errorf("regen-timeline v12 regen job must additionally gate on the pushed ref actually " +
			"BEING the default branch — the trigger is a scaffold-time literal and may be stale")
	}

	// A stale scaffolded trigger is otherwise silent: the job simply never
	// fires. The PR gate — which runs in every repo on every PR — compares
	// the two and says so.
	if !strings.Contains(body, "SCAFFOLDED_BRANCH: __LOGMIND_DEFAULT_BRANCH__") {
		t.Errorf("regen-timeline v12 PR gate must carry the scaffolded branch so it can detect drift")
	}
	if !strings.Contains(body, `if [ "$SCAFFOLDED_BRANCH" != "$DEFAULT_BRANCH" ]; then`) {
		t.Errorf("regen-timeline v12 PR gate must compare the scaffolded trigger against the live " +
			"default branch — a renamed default branch otherwise silently stops the regen forever")
	}
	driftBlock, _, found := strings.Cut(body, "changed=$(gh pr diff")
	if !found {
		t.Fatalf("regen-timeline v12: could not locate the PR gate's diff call")
	}
	_, driftBlock, _ = strings.Cut(driftBlock, `if [ "$SCAFFOLDED_BRANCH" != "$DEFAULT_BRANCH" ]; then`)
	if !strings.Contains(driftBlock, "::warning") {
		t.Errorf("regen-timeline v12 trigger-drift must surface as a ::warning")
	}
	if strings.Contains(driftBlock, "exit 1") {
		t.Errorf("regen-timeline v12 trigger-drift must NOT fail the gate — it is a freshness problem " +
			"in this repo's own configuration, not a defect in the PR being judged")
	}
	if !strings.Contains(driftBlock, "logmind init --refresh") {
		t.Errorf("regen-timeline v12 trigger-drift warning must name the command that fixes it")
	}
}

// TestWorkflowTemplates_NoTemplateGuessesTheDefaultBranch is the CLASS
// guard for the branch-name assumption, as opposed to the site guard above.
//
// The site guard was written for regen-timeline, where the assumption did
// real damage (a push refspec pointing at a branch the repo may not have).
// check-doc-links carried the same class in a milder form — `on: push:
// branches: [main, master]`, a two-name guess that silently dropped
// default-branch link checking in a repo called anything else. Fixing the
// site the defect was noticed at and leaving the other is how the class
// survives, so this walks EVERY workflow template that filters on a branch
// and requires the scaffold placeholder rather than any literal.
//
// Measured before removing `master`: all 8 repositories running these
// workflows (the 7 consumers plus logmind) default to `main`, so the second
// name was covering nothing. The asymmetry was the tell — regen-timeline,
// the workflow that WRITES, shipped `[main]` alone while the advisory
// read-only check shipped `[main, master]`. A deliberate hedge would have
// gone the other way round.
func TestWorkflowTemplates_NoTemplateGuessesTheDefaultBranch(t *testing.T) {
	const placeholder = "__LOGMIND_DEFAULT_BRANCH__"
	// Every literal branch filter this repo has ever shipped, plus the
	// obvious next guesses. A `branches:` line naming ANY of them is the
	// assumption coming back.
	guesses := []string{
		"branches: [main]",
		"branches: [master]",
		"branches: [main, master]",
		"branches: [trunk]",
		"branches: [develop]",
	}
	sawAFilter := false
	for _, name := range []string{
		"check-doc-links.yml.template",
		"regen-timeline.yml.template",
		"logmind-self-update.yml.template",
		"check-decisions.yml.template",
	} {
		body := Workflow(name)
		// Comments stripped: each template's header names the literal it
		// replaced, and naming it is the point.
		active := stripCommentLines(body)
		if !strings.Contains(active, "branches:") {
			continue // no branch filter at all (check-decisions is PR-only)
		}
		sawAFilter = true
		for _, guess := range guesses {
			if strings.Contains(active, guess) {
				t.Errorf("%s hardcodes a branch filter (%q) — an `on:` filter cannot take an "+
					"expression, so it must be the %s scaffold placeholder or it is a guess "+
					"about how someone else's repository is set up", name, guess, placeholder)
			}
		}
		if !strings.Contains(active, "branches: ["+placeholder+"]") {
			t.Errorf("%s has an `on:` branch filter that is not the scaffold placeholder %s",
				name, placeholder)
		}
		// A scaffolded trigger can go stale when the default branch is
		// renamed, and a check that stops running reports nothing at all.
		// Every template that filters on a branch must therefore also carry
		// the drift sentinel.
		if !strings.Contains(body, "SCAFFOLDED_BRANCH: "+placeholder) {
			t.Errorf("%s filters on a scaffolded branch but carries no SCAFFOLDED_BRANCH sentinel — "+
				"a default-branch rename would silently stop it forever", name)
		}
		if !strings.Contains(body, `if [ "$SCAFFOLDED_BRANCH" != "$DEFAULT_BRANCH" ]; then`) {
			t.Errorf("%s does not compare its scaffolded trigger against the live default branch", name)
		}
		if !strings.Contains(body, "DEFAULT_BRANCH: ${{ github.event.repository.default_branch }}") {
			t.Errorf("%s must read the live default branch from the event payload", name)
		}
		if !strings.Contains(body, "logmind init --refresh") {
			t.Errorf("%s drift warning must name the command that fixes it", name)
		}
	}
	// Control: if the loop above never found a branch filter, every
	// assertion in it was vacuous and this test proves nothing.
	if !sawAFilter {
		t.Fatalf("no workflow template contains a `branches:` filter — this test's search is " +
			"broken, not the tree")
	}
}

// TestRegenTimelineWorkflows_ShareOneCredentialChain is the class guard
// the v11 defect asks for, stated in terms of what went wrong rather than
// of any one file's bytes: the shipped template and the workflow logmind
// itself runs MUST NOT diverge on the credential MECHANISM. v11 did
// exactly that — the template pushed with a raw PAT while this repo minted
// a GitHub App token — and every existing test passed, because each file
// was only ever checked against itself.
//
// TestRegenTimelineWorkflow_LockstepWithTemplate proves the two are
// byte-identical through that region. This one is deliberately independent
// of it: it asserts the same chain, rung by rung, is present in BOTH
// files, so the property survives even if the region boundaries above are
// ever renegotiated.
func TestRegenTimelineWorkflows_ShareOneCredentialChain(t *testing.T) {
	repoRoot := repoRootFromCaller(t)
	wfBytes, err := os.ReadFile(filepath.Join(repoRoot, ".github", "workflows", "regen-timeline.yml"))
	if err != nil {
		t.Fatalf("read installed workflow: %v", err)
	}
	// Every rung of the chain, the outcome split, and the push itself.
	// Neither file may implement a credential path the other does not.
	chain := []string{
		"uses: actions/create-github-app-token@v",
		"if: env.APP_ID != '' && env.APP_PRIVATE_KEY != ''",
		"continue-on-error: true",
		"APP_TOKEN: ${{ steps.app_token.outputs.token }}",
		"PAT: ${{ secrets.LOGMIND_AUTO_REGEN_PAT }}",
		"DEFAULT_TOKEN: ${{ github.token }}",
		`if [ -n "${APP_TOKEN:-}" ]; then`,
		`elif [ -n "${PAT:-}" ]; then`,
		`elif [ -n "${DEFAULT_TOKEN:-}" ]; then`,
		`git push "https://x-access-token:${TOKEN}@github.com/${GITHUB_REPOSITORY}.git" "HEAD:${DEFAULT_BRANCH}"`,
	}
	for _, side := range []struct {
		label string
		body  string
	}{
		{"shipped template", Workflow("regen-timeline.yml.template")},
		{"logmind's own .github/workflows/regen-timeline.yml", string(wfBytes)},
	} {
		for _, want := range chain {
			if !strings.Contains(side.body, want) {
				t.Errorf("%s is missing credential-chain element %q — the shipped template and the "+
					"workflow logmind runs on itself must not diverge on the credential mechanism",
					side.label, want)
			}
		}
	}
}

// TestSelfUpdateWorkflow_ByteIdenticalToTemplate answers the one-owner
// question for logmind-self-update: unlike regen-timeline (which
// deliberately builds from in-tree source here), this repo runs the
// SCAFFOLDED workflow — `.github/workflows/logmind-self-update.yml` is an
// installed copy of the template, marker line and all. So there is nothing
// to permit: it must be byte-identical, and this test is the owner of that
// fact.
//
// It had silently drifted on two action pins — `actions/checkout` (v6 in
// the template, v7 installed) and `thrillmade/setup-logmind` (v1.0.0 vs
// v1.0.1) — because the installed copy was hand-edited without the
// template following, and the marker was never bumped, so `logmind doctor`
// reported the pair "current" while they differed.
func TestSelfUpdateWorkflow_ByteIdenticalToTemplate(t *testing.T) {
	repoRoot := repoRootFromCaller(t)
	installed, err := os.ReadFile(filepath.Join(repoRoot, ".github", "workflows", "logmind-self-update.yml"))
	if err != nil {
		t.Fatalf("read installed workflow: %v", err)
	}
	tmpl := Workflow("logmind-self-update.yml.template")
	if string(installed) == tmpl {
		return
	}
	t.Errorf("`.github/workflows/logmind-self-update.yml` is not byte-identical to " +
		"internal/templates/github/logmind-self-update.yml.template. This repo runs the scaffolded " +
		"workflow, so the two are one artifact with one owner — copy the template over the installed " +
		"file (and bump the template's marker so every consumer repo picks the change up too).")
	// Point at the first differing line rather than dumping both files.
	instLines := strings.Split(string(installed), "\n")
	tmplLines := strings.Split(tmpl, "\n")
	for i := 0; i < len(instLines) || i < len(tmplLines); i++ {
		var a, b string
		if i < len(tmplLines) {
			a = tmplLines[i]
		}
		if i < len(instLines) {
			b = instLines[i]
		}
		if a != b {
			t.Errorf("first divergence at line %d:\n  template:  %q\n  installed: %q", i+1, a, b)
			return
		}
	}
}

// TestWorkflowTemplates_SetupLogmindPinIsUniformAndCurrent pins the
// action-pin axis. Two failures it would have caught:
//
//   - the templates shipping `thrillmade/setup-logmind@v1.0.0` while every
//     repo already running these workflows had been moved to v1.0.1 by
//     Dependabot — a freshly scaffolded repo installed a pin older than the
//     fleet's on day one, and nothing said so;
//   - the pin drifting between templates, so which workflow you looked at
//     determined which version of the action you thought was current.
//
// The assertion is a property, not a literal: every setup-logmind call
// site in every template, AND every one in this repo's own
// `.github/workflows`, must agree on ONE version. Dependabot bumps the
// installed copies; this test is what makes a Dependabot bump that landed
// on the workflows but not the templates visible.
func TestWorkflowTemplates_SetupLogmindPinIsUniformAndCurrent(t *testing.T) {
	const usesPrefix = "uses: thrillmade/setup-logmind@"
	pins := map[string][]string{}
	collect := func(label, body string) {
		for _, line := range strings.Split(body, "\n") {
			_, after, ok := strings.Cut(line, usesPrefix)
			if !ok {
				continue
			}
			pin := strings.Fields(after)[0]
			pins[pin] = append(pins[pin], label)
		}
	}
	for _, name := range []string{
		"check-doc-links.yml.template",
		"regen-timeline.yml.template",
		"logmind-self-update.yml.template",
		"check-decisions.yml.template",
	} {
		collect("template "+name, Workflow(name))
	}
	repoRoot := repoRootFromCaller(t)
	entries, err := os.ReadDir(filepath.Join(repoRoot, ".github", "workflows"))
	if err != nil {
		t.Fatalf("read .github/workflows: %v", err)
	}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".yml") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(repoRoot, ".github", "workflows", e.Name()))
		if err != nil {
			t.Fatalf("read %s: %v", e.Name(), err)
		}
		collect(".github/workflows/"+e.Name(), string(data))
	}
	if len(pins) == 0 {
		t.Fatalf("found no `%s` call sites at all — this test's search is broken, not the tree", usesPrefix)
	}
	if len(pins) > 1 {
		t.Errorf("the `thrillmade/setup-logmind` pin is not uniform — %d different versions are in use, "+
			"so a freshly scaffolded repo gets a different action version than the ones already running "+
			"these workflows:", len(pins))
		for pin, sites := range pins {
			t.Errorf("  %s ← %v", pin, sites)
		}
	}
}
