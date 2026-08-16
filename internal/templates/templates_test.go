package templates

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
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

	// Marker bump v10 → v11 (push-refusal now fails the job; see v11 note
	// above). v11 → v12: the push credential is a degrading chain (see
	// TestRegenTimelineTemplate_V12_CredentialChainDegrades). v12 → v13: the
	// archive-gate change (see the v13 note in the template) had to move off
	// v12 after logmind#301 round 5 found it colliding with
	// fix/template-v12's unrelated, already in-flight v12. v13 is v12's body
	// PLUS the archive — the credential chain and the resolved default branch
	// the v12 tests below assert are still there, and are asserted there.
	if !strings.Contains(body, "# logmind-template-version: v13") {
		t.Errorf("regen-timeline template missing v13 marker")
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
	// All THREE derived docs of SPEC §3.3 — "the history, its archive, or
	// the map" — as whole lines. A gate that names only two leaves the
	// omitted one editable on a branch, undetected.
	if !strings.Contains(body, `grep -qxE 'docs/(timeline|timeline-archive|file-structure)\.md'`) {
		t.Errorf("regen-timeline PR gate must match exactly the three derived docs as whole lines")
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

// templateMarkerPins pins bundled workflow templates' declared
// `# logmind-template-version:` marker to a SHA256 of the marker's FULL
// body.
//
// logmind#301 round 5 BLOCK: feat/collapse-decision-layout (this branch)
// and fix/template-v12 (logmind#314) independently bumped
// regen-timeline.yml.template's marker from v11 to v12 with UNRELATED
// content. installWorkflowTemplates (internal/cli/init.go) refreshes an
// installed workflow only when the installed and bundled marker STRINGS
// differ — never when they merely match, even if the bytes underneath
// don't — so whichever v12 a repo installed first would have kept it
// forever, silently, even after the other v12 shipped upstream; `doctor`
// would have reported that repo current the whole time. This branch
// resolved the actual collision by moving to v13 (see the NOTE in the
// template itself); this table is what would have made it IMPOSSIBLE to
// ship silently in the first place.
//
// The collision's SECOND half is what the merge of #314 into this branch had
// to get right, and it is a distinct failure from the one above: taking this
// branch's v13 wholesale would have reverted #314's credential chain and
// resolved default branch while ADVERTISING a higher version number, and
// since installWorkflowTemplates rewrites only on marker inequality, every
// fleet repo would have taken that v13 and never seen v12's content again.
// v13's body is therefore v12's body PLUS the archive, and the digest below
// is over that merged body. TestRegenTimelineTemplate_V12_* still assert
// #314's content against the same file, which is what stops a future "newer
// version" from quietly deleting it.
//
// It works from inside ONE repo's tree, at test time, on the CI of
// whichever branch runs second: the first branch to land a marker's
// checksum here owns that number's content from then on. A second branch
// that independently reuses the same marker for different content fails
// THIS test the moment it merges/rebases past the first branch's entry —
// without ever needing to see the other branch's diff. If it instead
// edits the pin to match its own content, that edit shows up as a
// reviewable change to a line that used to mean something else: loud,
// not silent.
//
// What this does NOT catch: two branches that both introduce the SAME new
// marker and BOTH merge into dev before either ever rebases onto the
// other (a true simultaneous-merge race). Git still flags that case as a
// textual merge conflict on this table's entry — the same "loud, not
// silent" outcome — so the residual gap is narrower still: a collision
// where neither branch's own test run ever recorded a pin for the number
// it claims. Closing THAT would need something that can see both trees
// before either merges (e.g. a bot diffing open PRs' bundled markers
// against each other); nothing in this repo does that today, and nothing
// caught the actual v12/v12 collision this pins against — a human review
// pass did.
// The v13 digest was repinned once more inside #301 — the regen step now
// names docs/timeline-archive.md explicitly, because `logmind timeline
// --write` writes the one file it is given. "A SHIPPED marker's content must
// never change" is the rule, and v13 has not shipped: it is introduced by the
// same PR, so no repository holds it and a bump would reach nothing. Minting
// a v14 for an edit to v13's own unreleased body would leave a v13 that
// existed nowhere — which is the confusion the v12/v12 collision above WAS.
var templateMarkerPins = map[string]struct {
	marker string
	sha256 string
}{
	"regen-timeline.yml.template": {"v13", "c8d756f346fb5dca99a34f852ebe80f1b402013569f3a14aeee336c660baa8da"},
}

func TestWorkflowTemplateMarkers_PinnedToContent(t *testing.T) {
	for name, pin := range templateMarkerPins {
		body := Workflow(name)
		wantMarkerLine := "# logmind-template-version: " + pin.marker
		if !strings.HasPrefix(body, wantMarkerLine) {
			t.Errorf("%s: expected to start with %q — if this is a deliberate "+
				"version bump, update THIS entry's marker and sha256 in "+
				"templateMarkerPins to the new version (the map is keyed by "+
				"filename, so a second entry for %s is a compile error: "+
				"duplicate key)", name, wantMarkerLine, name)
			continue
		}
		sum := sha256.Sum256([]byte(body))
		got := hex.EncodeToString(sum[:])
		if got != pin.sha256 {
			t.Errorf("%s: content under marker %s changed (sha256 %s, want %s). "+
				"A shipped marker's content must never change silently: whether "+
				"this is a genuine content edit or a collision with another "+
				"branch that already claimed marker %s for different content, "+
				"bump the template's marker to a new, unclaimed version and "+
				"update THIS entry's marker and sha256 to match — the map is "+
				"keyed by filename, so a second entry for %s is a compile error.",
				name, pin.marker, got, pin.sha256, pin.marker, name)
		}
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

	// Marker bump v9 → v10 (the self-heal regenerates the archive too).
	// The marker is the ONLY thing doctor and logmind-self-update compare —
	// they never diff the body — so a content fix shipped without a bump
	// leaves every repo already running the old version on the old body,
	// with no drift row to say so.
	if !strings.Contains(body, "# logmind-template-version: v10") {
		t.Errorf("check-doc-links template missing v10 marker")
	}

	// The derived-doc self-heal must regenerate ALL THREE derived docs.
	// `logmind timeline --write` writes the one file it is given, so the
	// archive needs its own invocation: without it an archive-sourced
	// broken link survives every self-heal pass — the job re-runs, changes
	// nothing, and reports the same finding next time. linkcheck's own
	// remediation advice for that case is this exact command, so dropping
	// it leaves the workflow printing a fix it cannot itself run.
	for _, want := range []string{
		"logmind timeline --write docs/timeline.md",
		"logmind timeline --write docs/timeline-archive.md --half archive",
		"logmind file-structure --write docs/file-structure.md",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("check-doc-links self-heal does not run %q — a derived doc it omits is one it can never fix", want)
		}
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
// "May differ" is not "is unchecked", and the distinction is the whole
// lesson of the v11 divergence. Every one of the four is pinned to its own
// content: (1) is a required prefix, (2) is substituted before comparing so
// it costs no divergence at all, (3) is parsed key-by-key by
// requireAppCredentialEnvBlock, and (4) is pinned as two whole literals
// (wantTemplateBuildStanza / wantWorkflowBuildStanza).
//
// # What each region is read BY, region by region
//
// Two questions have to be answered separately, because they are different
// questions and answering only the first is how this test shipped a hole:
// "do the two copies agree" (a pair-diff) and "is what they agree on still
// what was reviewed" (a pin). A pair-diff cannot see an edit applied to
// BOTH files — that is agreement, as far as it can tell.
//
//	A  header + check-derived-docs  requireIdentical + digestRegionA
//	B  regen-on-main preamble       requireIdentical + digestRegionB
//	C  App-credential env block     requireAppCredentialEnvBlock (parsed key-by-key)
//	D  checkout stanza              requireIdentical + digestRegionD
//	E1 build mechanism (differs)    requireExactly, whole literal, refs validated
//	E2 regen step (differs)         requireExactly, whole literal
//	F  credential mechanism → EOF   requireIdentical + digestRegionF
//
// The ONE thing no digest covers is a whole-line `#` comment, and that
// exclusion is an argument rather than a convenience: YAML's comment syntax
// and — inside a `run:` block — the shell's are the same syntax, and
// neither executes. The claim is control-tested rather than asserted:
// inserting a payload as a comment leaves the PARSED YAML document
// byte-for-byte identical, so GitHub reads the same workflow either way.
// (A payload smuggled as comment TEXT still has to be un-commented by some
// line to run, and that line is not a comment, so it is inside a digest.)
// Excluding comments is also what keeps the header prose — which is most of
// what a version bump edits here — from churning four digests every time,
// because a digest people update reflexively is a digest that reads as
// true while nobody looks.
//
// Refs are handled on their own axis, by three owners with no overlap:
// pinActionRefs validates the SHAPE of every ref it collapses,
// TestWorkflowActionSurface_IsPinned validates every ref in every workflow
// in the repository (including the two templates that have no pair here),
// and requireIdentical catches a ref that moved on one side only. Before
// this, a normalisation erased every ref before every comparison, and
// `actions/setup-go@refs/pull/1/merge` — a one-sided edit — was green.
//
// # What this gate does and does not promise
//
// Stated plainly so the next reader does not over-read it: `regen-on-main`
// triggers on PUSH to the default branch, never on `pull_request`, so an
// edit to it does not run from a fork's PR. None of what this test catches
// is a one-click PR exploit. What it is: the thing that stops a malicious
// or accidental edit to a `contents: write` job — one whose `env` carries
// APP_ID/APP_PRIVATE_KEY and whose token pushes to the default branch —
// from reaching a human reviewer as an unremarkable green diff. That is a
// smaller claim than "prevents compromise" and a much larger one than
// nothing, and the fix is not downgraded on the strength of it: the same
// edit merged is code running with those credentials.
//
// Everything not listed as a permitted difference — the header, the
// check-derived-docs job (the actual enforcement logic), the checkout
// stanza, the regen step, and the whole credential mechanism through EOF —
// must be byte-identical, or logmind's own CI would be exercising different
// behaviour than what every other repo installs.
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
		regenAnchor  = "\n      - name: Regenerate derived docs\n"
		buildEnd     = "\n      # Rung 1: mint a short-lived (1 hour) App installation token"
		checkoutTail = "\n          persist-credentials: false\n"
	)

	// --- Region A: header + check-derived-docs (the PR gate). Carries none
	// of the permitted differences → byte-identical, AND digest-pinned so
	// the same edit made to both copies is not mistaken for agreement.
	tmplA, tmplRest := splitOnceOrFail(t, "template", tmplBody, jobAnchor)
	wfA, wfRest := splitOnceOrFail(t, "installed workflow", workflow, jobAnchor)
	requireIdentical(t, "header + check-derived-docs job (the PR gate)", tmplA, wfA)
	requireContentDigest(t, "region A — header + check-derived-docs job (the PR gate)", digestRegionA, tmplA)

	// --- Region B: regen-on-main's preamble through `env:`. This is the
	// prose that tells a repo owner the App rung is optional, so it must
	// read identically in both. It is also where a `defaults:` block would
	// sit — a job-level key, valid YAML, invisible to every step-level pin
	// below — which is why the digest covers it and why
	// TestWorkflowActionSurface_IsPinned bans the key outright.
	tmplB, tmplRest := splitOnceOrFail(t, "template", tmplRest, envAnchor)
	wfB, wfRest := splitOnceOrFail(t, "installed workflow", wfRest, envAnchor)
	requireIdentical(t, "regen-on-main preamble (through `env:`)", jobAnchor+tmplB, jobAnchor+wfB)
	requireContentDigest(t, "region B — regen-on-main preamble (through `env:`)", digestRegionB, jobAnchor+tmplB)

	// --- Region C (difference 2): the App-credential env block. The two
	// sides name different secrets; they must NOT differ in any other way.
	tmplC, tmplRest := splitOnceOrFail(t, "template", tmplRest, stepsAnchor)
	wfC, wfRest := splitOnceOrFail(t, "installed workflow", wfRest, stepsAnchor)
	requireAppCredentialEnvBlock(t, "template", tmplC)
	requireAppCredentialEnvBlock(t, "installed workflow", wfC)

	// --- Region D: the checkout stanza. Identical, and digest-pinned.
	tmplD, tmplRest := splitOnceOrFail(t, "template", tmplRest, checkoutTail)
	wfD, wfRest := splitOnceOrFail(t, "installed workflow", wfRest, checkoutTail)
	requireIdentical(t, "regen-on-main checkout stanza", stepsAnchor+tmplD, stepsAnchor+wfD)
	requireContentDigest(t, "region D — regen-on-main checkout stanza", digestRegionD,
		pinActionRefs(t, "regen-on-main checkout stanza", stepsAnchor+tmplD))

	// --- Region E1 (difference 4): how each side OBTAINS the logmind binary.
	//
	// This is the only region where the two files genuinely run different
	// mechanisms, and it used to be the one region where NOTHING was
	// checked: both halves were discarded, so any content at all could sit
	// in the gap. Two things went through it with the whole suite green — a
	// step running `curl -d "$(env | base64)" https://…` (the job `env` two
	// regions up holds APP_PRIVATE_KEY), and a regen redirected to /tmp,
	// which makes the push step report "already current" forever. That is
	// the same unanchored-gap defect that let the credential divergence
	// ship, which is what this test was written to stop.
	//
	// So each side is now pinned to its WHOLE content: "free to diverge"
	// means "free to be the other mechanism", never "unchecked". A ref is
	// collapsed to `@<ref>` only AFTER pinActionRefs has confirmed the
	// action is allowlisted and the ref is a release tag — `actions/setup-go@v7`
	// vs `@v8` is still "install Go", while `@refs/pull/1/merge` is
	// attacker-authored code inside the job holding APP_PRIVATE_KEY, and the
	// erasure that could not tell those apart is what this replaces.
	tmplE1, tmplRest := splitOnceOrFail(t, "template", tmplRest, regenAnchor)
	wfE1, wfRest := splitOnceOrFail(t, "installed workflow", wfRest, regenAnchor)
	requireExactly(t, "the template's build mechanism (a released binary via setup-logmind)",
		wantTemplateBuildStanza, pinActionRefs(t, "template build mechanism", tmplE1))
	requireExactly(t, "the installed workflow's build mechanism (this repo's own source)",
		wantWorkflowBuildStanza, pinActionRefs(t, "installed workflow build mechanism", wfE1))

	// --- Region E2: the regen step. NOT part of the permitted divergence —
	// both sides run the same two subcommands against the same two paths,
	// and only the binary they invoke differs (a PATH lookup vs the freshly
	// built ./bin/logmind). Pinned as whole content on each side rather than
	// diffed against the other, so neither the paths nor the flags can move
	// on one side alone.
	tmplE2, tmplRest := splitOnceOrFail(t, "template", tmplRest, buildEnd)
	wfE2, wfRest := splitOnceOrFail(t, "installed workflow", wfRest, buildEnd)
	requireExactly(t, "the template's regen step", fmt.Sprintf(wantRegenStep, "logmind"), tmplE2)
	requireExactly(t, "the installed workflow's regen step", fmt.Sprintf(wantRegenStep, "./bin/logmind"), wfE2)

	// --- Region F: the credential mechanism, through EOF. THE region this
	// test exists for. Byte-identical, and digest-pinned: "the two copies
	// agree" is not the same claim as "the bytes are the ones reviewed".
	requireIdentical(t, "regen-on-main credential mechanism (App token → PAT → GITHUB_TOKEN, and the push)",
		buildEnd+tmplRest, buildEnd+wfRest)
	requireContentDigest(t, "region F — regen-on-main credential mechanism, through EOF", digestRegionF,
		pinActionRefs(t, "regen-on-main credential mechanism", buildEnd+tmplRest))
}

// The content digests for the four regions the two copies must agree on.
//
// requireIdentical answers "do the two copies say the same thing"; these
// answer "is what they say still what was reviewed". They are separate
// questions and the second one had no owner: the same edit applied to BOTH
// files is, to a pair-diff, indistinguishable from agreement.
//
// Each digest covers the region with whole-line `#` comments removed and
// allowlisted action refs collapsed (see requireContentDigest for why, and
// for the argument that comments cannot carry a payload). A digest rather
// than a copy of the text: the workflow file is the one owner of its own
// content, and a literal here would be a hand-kept second copy that reads
// as true until one quietly isn't. The failure prints the current content
// so the change is reviewed, not just re-hashed.
// v13 (logmind#265/#301) moved A and F: the PR gate now names three derived
// docs rather than two, and the push step's `git status --porcelain` and
// `git add` name three paths rather than two. B and D are untouched by that
// change and their digests are unchanged, which is the evidence that the
// archive layered ONTO v12 rather than replacing it — a v13 that had reverted
// #314 would have moved B (the regen-on-main preamble) and D (the checkout
// stanza) as well.
const (
	digestRegionA = "75a4a82048f9b28379db76cd5630c3c7aa9918bba1341a149ff2e00b68c68c18"
	digestRegionB = "60c9f3ef2d00a0a58c4057f1c36637b1054658019a21f47ef3667059e4445346"
	digestRegionD = "7e2d788d136cdff688f698527cd505c1d70633f4134d4df2951fbff59b7fc612"
	digestRegionF = "bbe3a145536916b0c0ae850ae84e5e5e0662554d9d7059f79d9c7cd1844e117a"
)

// requireContentDigest pins a region's EXECUTABLE content to a digest.
//
// What is hashed, and why each exclusion is safe:
//
//   - Whole-line YAML comments are stripped first — and ONLY genuine YAML
//     comments, which is a narrower set than "lines starting with #". A `#`
//     line inside a `run: |` block scalar is NOT a comment to YAML: it is
//     part of the string GitHub hands to the shell, and stripping it would
//     leave every `run:` block with a hole an attacker could fill.
//     stripYAMLComments makes that distinction; stripCommentLines (used by
//     the banned-word tests elsewhere in this file) deliberately does not,
//     and using it here was a real gap caught by control-testing this very
//     claim rather than asserting it.
//     The exclusion that remains is provable: a comment outside a block
//     scalar contributes NO node to the parsed document, so GitHub reads
//     exactly the same workflow with or without it. Excluding those is what
//     keeps a header-prose edit — the bulk of every version bump here —
//     from churning four digests, which is what would train a reader to
//     update them without looking.
//   - Allowlisted action refs are collapsed to `@<ref>` by pinActionRefs,
//     which has already rejected any ref that is not a release tag. So a
//     Dependabot tag bump does not churn the digest, while the ref remains
//     read — by pinActionRefs here and by
//     TestWorkflowActionSurface_IsPinned across every workflow in the repo.
//
// Everything else — every key, every value, every line of every `run:`
// block — is inside the hash.
func requireContentDigest(t *testing.T, region, want, got string) {
	t.Helper()
	body := stripYAMLComments(got)
	sum := sha256.Sum256([]byte(body))
	have := hex.EncodeToString(sum[:])
	if have == want {
		return
	}
	t.Errorf("regen-timeline.yml: %s no longer hashes to its pinned content digest.\n"+
		"  pinned: %s\n"+
		"  actual: %s\n"+
		"This region must be byte-identical in the template and in this repo's own copy, so a "+
		"pair-diff cannot see an edit made to BOTH — which is what this digest is for. READ the "+
		"content below before touching the constant; if the change is intended, update the "+
		"digest to the actual value above in the same commit.\n"+
		"--- content hashed (whole-line comments stripped, allowlisted action refs collapsed) ---\n%s",
		region, want, have, body)
}

// The two build stanzas, in full. These are the ONLY bytes either file is
// allowed to carry between the checkout and the regen step — the region
// that used to be discarded unchecked.
//
// A consumer repo installs a RELEASED logmind; this repo builds its own
// in-tree source so its CI always exercises the code under review. That is
// the whole of the difference, and stating it as two literals is what makes
// anything else — an extra step, an added `env:`, a `run:` that was not
// there — a failure rather than a silence.
const (
	wantTemplateBuildStanza = `      - uses: thrillmade/setup-logmind@<ref>
        with:
          token: ${{ github.token }}`

	wantWorkflowBuildStanza = `      - uses: actions/setup-go@<ref>
        with:
          go-version: "1.22"
          cache: true
      - name: Build logmind from source
        run: make build`

	// wantRegenStep takes the binary each side invokes. Everything else —
	// the subcommands, the flags, the OUTPUT PATHS — is common, and a regen
	// pointed anywhere but docs/ is a job that reports "already current"
	// forever while the derived docs rot.
	//
	// The archive gets its OWN invocation, and that is load-bearing rather
	// than stylistic: `logmind timeline --write` writes the one file it is
	// given, so dropping this line does not "simplify" the step, it stops
	// docs/timeline-archive.md being regenerated at all — on the only branch
	// that ever regenerates it, with every run still green.
	wantRegenStep = `        run: |
          set -euo pipefail
          # Both halves of the SPEC §3.3 split, each named explicitly:
          # ` + "`--write`" + ` writes the file it is given and no other.
          %[1]s timeline --write docs/timeline.md
          %[1]s timeline --write docs/timeline-archive.md --half archive
          %[1]s file-structure --write docs/file-structure.md`
)

// ─────────────────────────────────────────────────────────────────────────
// The action surface: which third-party code any workflow in this repo, or
// any workflow logmind installs into someone else's repo, is allowed to run.
//
// This replaces a normalisation that erased every ref for every action
// (`uses: owner/repo@<anything>` → `@<ref>`) BEFORE comparing a pinned
// region against its literal. That made the ref unread by anything, so
// rewriting one step's ref — a one-sided edit, no mirror needed —
//
//	-      - uses: actions/setup-go@v7
//	+      - uses: actions/setup-go@refs/pull/1/merge
//
// left `go test ./internal/templates/...` green while pointing a step
// inside `regen-on-main` at attacker-authored code. That job carries
// APP_ID/APP_PRIVATE_KEY in its `env` and pushes to the default branch.
//
// The tolerance the erasure existed for is real and is KEPT, but stated
// instead of assumed: Dependabot bumps a release tag in place, and a tag
// bump is not mechanism drift. So a ref may float only within a shape —
// and only for an action named here, with a reason.
// ─────────────────────────────────────────────────────────────────────────

// refPolicy is what an action's ref is allowed to look like.
type refPolicy int

const (
	// refSemverTag: `v7`, `v1.0.1`, `v3` — a maintainer-published release
	// tag. This is the shape Dependabot writes, and the only shape that
	// floats. Notably NOT matched: a branch name, `refs/pull/N/merge`,
	// `refs/heads/anything`, or a bare 40-hex SHA of unknown provenance —
	// each of which is a way to point a pinned action at code the action's
	// maintainer never released.
	refSemverTag refPolicy = iota
	// refCommitSHA: a full 40-hex commit SHA. Required where the callee is
	// a whole reusable WORKFLOW rather than an action — it runs with this
	// repo's secrets, and a tag there is mutable by its owner.
	refCommitSHA
)

var (
	semverTagRe = regexp.MustCompile(`^v\d+(\.\d+){0,2}$`)
	commitSHARe = regexp.MustCompile(`^[0-9a-f]{40}$`)
)

// allowedActions is the whole set of third-party code any workflow in this
// repository — installed or shipped as a template — may run, with the ref
// shape each is allowed to carry and why. An action absent from this map is
// a failure, not a default-allow: adding one is a decision someone makes on
// purpose.
var allowedActions = map[string]struct {
	policy refPolicy
	reason string
}{
	"actions/checkout": {refSemverTag,
		"first-party GitHub action; Dependabot bumps the major tag in place"},
	"actions/setup-go": {refSemverTag,
		"first-party GitHub action; Dependabot bumps the major tag in place"},
	"actions/setup-python": {refSemverTag,
		"first-party GitHub action; Dependabot bumps the major tag in place"},
	"actions/create-github-app-token": {refSemverTag,
		"first-party GitHub action; mints the rung-1 credential, so its ref shape is load-bearing"},
	"actions/upload-artifact": {refSemverTag,
		"first-party GitHub action; Dependabot bumps the major tag in place"},
	"goreleaser/goreleaser-action": {refSemverTag,
		"release tooling, vendor-published tags; Dependabot bumps them"},
	"thrillmade/setup-logmind": {refSemverTag,
		"our own action; the exact pin is additionally held uniform across every " +
			"call site by TestWorkflowTemplates_SetupLogmindPinIsUniformAndCurrent"},
	"thrillmade/.github/.github/workflows/dependabot-auto-merge.yml": {refCommitSHA,
		"a reusable WORKFLOW, not an action: it runs with this repository's secrets, " +
			"and a tag pointing at it is mutable by whoever owns thrillmade/.github"},
}

// checkActionRef reports whether one `uses:` value is allowed, and why not
// when it is not. Split on the LAST `@` so an owner/repo/path callee (a
// reusable workflow) parses the same way an action does.
func checkActionRef(uses string) (action, ref string, err error) {
	at := strings.LastIndex(uses, "@")
	if at < 0 {
		return uses, "", fmt.Errorf("carries no `@ref` at all — an unpinned `uses:` resolves to " +
			"the callee's default branch, which its owner can move at any time")
	}
	action, ref = uses[:at], uses[at+1:]
	rule, ok := allowedActions[action]
	if !ok {
		return action, ref, fmt.Errorf("is not on the action allowlist in templates_test.go — " +
			"every piece of third-party code a logmind workflow runs is named there on purpose, " +
			"with the ref shape it may carry and the reason")
	}
	switch rule.policy {
	case refSemverTag:
		if !semverTagRe.MatchString(ref) {
			return action, ref, fmt.Errorf("ref %q is not a release tag (want `vN`, `vN.N`, or "+
				"`vN.N.N`). %s may float ONLY across release tags (%s); a branch, a "+
				"`refs/...` ref, or a raw SHA points the step at code the action's maintainer "+
				"never released", ref, action, rule.reason)
		}
	case refCommitSHA:
		if !commitSHARe.MatchString(ref) {
			return action, ref, fmt.Errorf("ref %q is not a full 40-hex commit SHA. %s must be "+
				"SHA-pinned: %s", ref, action, rule.reason)
		}
	}
	return action, ref, nil
}

// pinActionRefs validates every `uses:` line in a region and returns the
// region with policy-VALID refs collapsed to `@<ref>`, which is what lets a
// pinned literal survive a Dependabot tag bump without going blind to the
// ref.
//
// An invalid ref is reported here AND left verbatim in the returned string,
// so the literal comparison downstream fails too. Two independent reds for
// one edit is deliberate: this is the mechanism whose single point of
// failure shipped the hole.
func pinActionRefs(t *testing.T, label, s string) string {
	t.Helper()
	var out []string
	for _, line := range strings.Split(s, "\n") {
		trimmed := strings.TrimSpace(line)
		const marker = "uses: "
		idx := strings.Index(trimmed, marker)
		if idx != 0 && !strings.HasPrefix(trimmed, "- "+marker) {
			out = append(out, line)
			continue
		}
		uses := strings.TrimSpace(strings.TrimPrefix(strings.TrimPrefix(trimmed, "- "), marker))
		action, _, err := checkActionRef(uses)
		if err != nil {
			t.Errorf("%s: `uses: %s` %v", label, uses, err)
			out = append(out, line) // leave it verbatim → the literal compare fails too
			continue
		}
		out = append(out, strings.Replace(line, uses, action+"@<ref>", 1))
	}
	return strings.Join(out, "\n")
}

// TestWorkflowActionSurface_IsPinned is the CLASS guard for the ref hole,
// and it is what answers "what covers the two templates that have no
// lockstep pair".
//
// The lockstep test can only ever protect regen-timeline, because it is the
// only workflow this repo runs a near-copy of. `check-decisions.yml` and
// `check-doc-links.yml` have no pair at all — logmind runs deliberately
// different variants of both — so nothing diffs them against anything, and
// before this test `grep -rn "setup-go" --include='*_test.go' .` found no
// assertion pinning any of their action refs either.
//
// So the property is stated where it actually belongs: over EVERY workflow
// template logmind ships and EVERY workflow this repo runs, no matter which
// of them has a mirror copy. Both axes:
//
//   - every `uses:` names an action on the allowlist above, carrying a ref
//     of the shape that action is allowed to carry;
//   - no `defaults:` block exists anywhere in any of them (see below).
//
// It reads the PARSED YAML rather than the text, so the evasions that beat
// a `strings.Contains` scan — flow style (`{defaults: {run: {shell: …}}}`),
// a quoted key (`"defaults":`), an aliased anchor, odd spacing — are read
// as what GitHub would read, not as what a substring search would.
func TestWorkflowActionSurface_IsPinned(t *testing.T) {
	repoRoot := repoRootFromCaller(t)

	type file struct{ label, body string }
	var files []file
	for _, name := range ListWorkflowTemplates() {
		files = append(files, file{"template " + name, Workflow(name)})
	}
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
		files = append(files, file{".github/workflows/" + e.Name(), string(data)})
	}

	sawUses := 0
	for _, f := range files {
		var doc yaml.Node
		if err := yaml.Unmarshal([]byte(f.body), &doc); err != nil {
			t.Errorf("%s: does not parse as YAML (%v) — GitHub would not run it, and this test "+
				"cannot read it", f.label, err)
			continue
		}
		walkYAMLMappings(&doc, "", func(key string, val *yaml.Node, path string) {
			switch key {
			case "uses":
				sawUses++
				if _, _, err := checkActionRef(val.Value); err != nil {
					t.Errorf("%s at %s: `uses: %s` %v", f.label, path, val.Value, err)
				}
			case "defaults":
				// A `defaults:` block is BANNED outright rather than pinned.
				// It rewrites the meaning of every `run:` in the file without
				// touching any of them — `defaults: {run: {shell: <anything>}}`
				// makes each `run:` block an argument to a command of the
				// author's choosing — so whole-literal pinning of individual
				// steps buys nothing while one exists. No workflow here needs
				// one (measured: zero occurrences across every template and
				// every installed workflow), so the cheap, total rule is
				// available and this test takes it. If a real need ever
				// appears, delete this case and pin the block's content
				// explicitly — but do that on purpose.
				t.Errorf("%s at %s: a `defaults:` block is not allowed in a logmind workflow. It "+
					"rewrites what every `run:` in the file executes (`defaults.run.shell`) without "+
					"editing a single one of them, which defeats every step-level pin in this suite. "+
					"Pin it explicitly, or do not add it.", f.label, path)
			}
		})
	}

	// Controls: both halves of the search must be demonstrably live, or
	// every assertion above was vacuous.
	if len(files) < 2 {
		t.Fatalf("found %d workflow files — this test's file discovery is broken, not the tree", len(files))
	}
	if sawUses == 0 {
		t.Fatalf("found no `uses:` keys in %d workflow files — this test's YAML walk is broken, "+
			"not the tree", len(files))
	}
}

// bundledTemplateFingerprints binds each workflow template's MARKER VERSION
// to a digest of the exact bytes that marker names, one line per template.
//
// `installWorkflowTemplatesMode` rewrites an installed workflow only when
// the markers DIFFER (init.go: `if installedVer != bundledVer`). So a
// marker is not a label on a file — it is the identity of a specific
// content, fleet-wide, and two different contents wearing the same marker
// is a distribution bug with no symptom: every repo already holding vN
// keeps its vN forever, and `logmind doctor` reports it current while
// doing so.
//
// This happened. Two branches independently bumped
// regen-timeline.yml.template to v12 with different content, and the whole
// suite was green on both.
//
// What this test can and cannot do, stated exactly:
//
//   - It CANNOT see a sibling branch. Nothing in a single-repo `go test`
//     can — the other branch's blob is not in the working tree, and a test
//     that shelled out to `git` for it would be reading refs that a shallow
//     CI checkout may not have fetched. If a guard that genuinely observes
//     the collision is wanted, it belongs in CI as a cross-branch marker
//     check (compare the PR's bundled marker against every other open PR's,
//     via the API), not here. That is not built; this comment is not
//     claiming it is.
//   - It DOES convert the silent case into a loud one. Two branches that
//     both bump to v12 both edit the SAME line of this map to different
//     values, so the second merge conflicts in git rather than resolving
//     cleanly into one v12 with the other's content.
//   - It DOES catch the more common shape of the same bug directly: content
//     edited without the marker moving. That change ships to nobody, and
//     before this nothing said so.
//
// Update procedure: change a template → bump its marker → the test prints
// the new digest → paste it here, same commit.
var bundledTemplateFingerprints = map[string]string{
	"check-decisions.yml.template": "v6:5fbd605bfc774cae66e321405a634baa0f0d3e93a47ffa661623100219430559",
	// v10 = v9's body plus the archive half of the derived-doc self-heal:
	// `logmind timeline --write docs/timeline-archive.md --half archive`.
	// The marker moves WITH the content by the rule this map exists to
	// enforce — a content-only fix would have left every repo already
	// holding v9 running the two-of-three self-heal forever, with `doctor`
	// calling them current. See the v10 note in the template itself.
	"check-doc-links.yml.template":     "v10:9e6328e81ac0f56d538f41c2b1a304c6f9eb279f65eb5cd7ef109460a437ef8b",
	"logmind-self-update.yml.template": "v11:d4214fb3d201997b3089e8bdaf824ea513da27b0d40093d71d663016b6e903d9",
	// v13 = v12's body plus docs/timeline-archive.md in the PR gate and the
	// push step (logmind#265/#301). It is a SUPERSET of v12, not a
	// replacement: had the merge taken #301's v13 wholesale it would have
	// reverted this file to the v11 base under a higher marker, and the
	// rewrite-on-marker-inequality rule above is exactly what would have made
	// that permanent for every repo that took it.
	//
	// The digest moved once more inside #301, WITHOUT the marker: the regen
	// step now names docs/timeline-archive.md explicitly, because `logmind
	// timeline --write` writes the one file it is given. That is a repin, not
	// a skipped bump — v13 is introduced by this same PR and no repository
	// holds it, so there is no installed v13 for a bump to reach. A marker
	// only has to move when the content it names has already SHIPPED; moving
	// it for an edit to its own unreleased body would mint a v13 that never
	// existed anywhere, which is the confusion the v12 collision above was.
	"regen-timeline.yml.template": "v13:c8d756f346fb5dca99a34f852ebe80f1b402013569f3a14aeee336c660baa8da",
}

// TestWorkflowTemplateMarkers_MoveWithContent enforces the binding above.
func TestWorkflowTemplateMarkers_MoveWithContent(t *testing.T) {
	names := ListWorkflowTemplates()
	if len(names) == 0 {
		t.Fatalf("ListWorkflowTemplates returned nothing — this test's search is broken, not the tree")
	}
	seen := map[string]bool{}
	for _, name := range names {
		seen[name] = true
		body := Workflow(name)
		marker := ""
		if nl := strings.IndexByte(body, '\n'); nl > 0 {
			marker = strings.TrimSpace(strings.TrimPrefix(body[:nl], "# logmind-template-version:"))
		}
		if marker == "" || strings.HasPrefix(marker, "#") {
			t.Errorf("%s does not start with a `# logmind-template-version:` marker line — "+
				"`logmind init` keys every rewrite decision on that marker", name)
			continue
		}
		sum := sha256.Sum256([]byte(body))
		got := marker + ":" + hex.EncodeToString(sum[:])
		want, ok := bundledTemplateFingerprints[name]
		if !ok {
			t.Errorf("%s is a bundled workflow template with no entry in "+
				"bundledTemplateFingerprints. Add:\n\t%q: %q,", name, name, got)
			continue
		}
		if got != want {
			t.Errorf("%s: marker+content fingerprint moved.\n  pinned: %s\n  actual: %s\n"+
				"If the CONTENT changed, the MARKER must move with it — `logmind init` only "+
				"rewrites an installed workflow when the markers differ, so content shipped under "+
				"an unchanged marker reaches no repository that already holds that version, and "+
				"`logmind doctor` reports those repos current while they run the old bytes. "+
				"Bump the marker, then paste the actual value above into "+
				"bundledTemplateFingerprints in this same commit.", name, want, got)
		}
	}
	var stale []string
	for name := range bundledTemplateFingerprints {
		if !seen[name] {
			stale = append(stale, name)
		}
	}
	sort.Strings(stale)
	if len(stale) > 0 {
		t.Errorf("bundledTemplateFingerprints names templates that are no longer bundled: %v", stale)
	}
}

// blockScalarKeyRe matches a mapping key whose value is a block scalar —
// `run: |`, `script: >-`, `run: |2+`. Everything indented past that key is
// the scalar's CONTENT, not YAML.
var blockScalarKeyRe = regexp.MustCompile(`:\s*[|>][-+]?\d*[-+]?$`)

// stripYAMLComments removes whole-line comments that YAML actually treats
// as comments, and nothing else.
//
// The distinction is load-bearing and was very nearly got wrong here. Inside
// a `run: |` block, a line beginning with `#` is a SHELL comment, which
// means it is a YAML STRING — it is in the document, it is handed to the
// runner, and only the shell's own lexer makes it inert. Removing such a
// line from a content digest opens a hole in the middle of every `run:`
// block: the digest stops covering whatever sits there. So block-scalar
// content is hashed in full, `#` lines included.
//
// Outside a block scalar a `#` line produces no node at all, which is the
// property the digest exclusion rests on.
func stripYAMLComments(body string) string {
	var kept []string
	blockIndent := -1 // -1: not inside a block scalar
	for _, line := range strings.Split(body, "\n") {
		indent := len(line) - len(strings.TrimLeft(line, " "))
		trimmed := strings.TrimSpace(line)
		if blockIndent >= 0 {
			// A blank line does not end a block scalar, and anything
			// indented past the key is still its content.
			if trimmed == "" || indent > blockIndent {
				kept = append(kept, line)
				continue
			}
			blockIndent = -1
		}
		if strings.HasPrefix(trimmed, "#") {
			continue
		}
		if blockScalarKeyRe.MatchString(trimmed) {
			blockIndent = indent
		}
		kept = append(kept, line)
	}
	return strings.Join(kept, "\n")
}

// TestStripYAMLComments_KeepsBlockScalarContent is the unit control for the
// distinction above. Without it, a stripper that dropped every `#` line
// would pass every other test in this file while silently un-covering the
// inside of every `run:` block.
func TestStripYAMLComments_KeepsBlockScalarContent(t *testing.T) {
	const in = "# a real YAML comment\n" +
		"jobs:\n" +
		"  a:\n" +
		"    steps:\n" +
		"      # another real one\n" +
		"      - run: |\n" +
		"          set -e\n" +
		"          # NOT a comment to YAML — this is string content\n" +
		"          echo hi\n" +
		"      - name: next\n" +
		"        # real again: this is a mapping, not a scalar\n" +
		"        run: echo bye\n"
	got := stripYAMLComments(in)
	for _, gone := range []string{"a real YAML comment", "another real one", "real again"} {
		if strings.Contains(got, gone) {
			t.Errorf("stripYAMLComments kept the YAML comment %q", gone)
		}
	}
	if !strings.Contains(got, "# NOT a comment to YAML") {
		t.Errorf("stripYAMLComments dropped a `#` line from inside a `run: |` block. That line is "+
			"string content GitHub hands to the shell, so dropping it takes the inside of every "+
			"`run:` block out of the digest:\n%s", got)
	}
	// Control: the stripper must actually strip, or the assertions above
	// are satisfied by a function that returns its input.
	if got == in {
		t.Fatalf("stripYAMLComments returned its input unchanged — it strips nothing")
	}
}

// walkYAMLMappings calls fn for every (key, value) pair of every mapping in
// the document, at any depth, with a dotted path for the error message.
func walkYAMLMappings(n *yaml.Node, path string, fn func(key string, val *yaml.Node, path string)) {
	switch n.Kind {
	case yaml.DocumentNode:
		for _, c := range n.Content {
			walkYAMLMappings(c, path, fn)
		}
	case yaml.MappingNode:
		for i := 0; i+1 < len(n.Content); i += 2 {
			k, v := n.Content[i], n.Content[i+1]
			child := k.Value
			if path != "" {
				child = path + "." + k.Value
			}
			fn(k.Value, v, child)
			walkYAMLMappings(v, child, fn)
		}
	case yaml.SequenceNode:
		for i, c := range n.Content {
			walkYAMLMappings(c, fmt.Sprintf("%s[%d]", path, i), fn)
		}
	}
}

// requireExactly fails when a region is not byte-for-byte the content it is
// pinned to. Unlike requireIdentical (which compares the two files against
// each other) this compares ONE side against a literal, which is what a
// region the two files are allowed to differ in needs: there is no other
// copy to diff it against.
func requireExactly(t *testing.T, region, want, got string) {
	t.Helper()
	if want == got {
		return
	}
	t.Errorf("regen-timeline.yml: %s is not what it claims to be. This region is where the two "+
		"copies of this workflow are permitted to differ, so nothing else checks it — content that "+
		"appears here appears NOWHERE else in the suite.\n--- want ---\n%s\n--- got ---\n%s",
		region, want, got)
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
// secretsExprRe matches a value that is EXACTLY one `${{ secrets.NAME }}`
// expression, edge to edge.
var secretsExprRe = regexp.MustCompile(`^\$\{\{ secrets\.[A-Za-z_][A-Za-z0-9_]* \}\}$`)

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
		// WHOLE-value match, not prefix+suffix. `${{ secrets.X }}${{ evil }}`
		// satisfies "starts with `${{ secrets.` and ends with ` }}`" while
		// smuggling a second expression into the job env.
		if !secretsExprRe.MatchString(value) {
			t.Errorf("%s: App-credential env value for %q must be exactly one `${{ secrets.NAME }}` "+
				"expression and nothing else, got %q", label, key, value)
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

	// …and the same rule against logmind's OWN installed workflows, which
	// the loop above cannot see. This repo runs dogfood variants of two of
	// these templates (they build in-tree source instead of installing a
	// release), so nothing diffs them against anything — and one of them
	// carried a bare `branches: [main]` with no sentinel while the template
	// it mirrors had just gained one. Shipping a warning logmind does not
	// run itself is the same class as shipping a credential path it had
	// abandoned.
	//
	// The literal `main` is correct HERE and is not what is being checked:
	// this repo is not scaffolded, so its copy carries the rendered value.
	// What must exist is the sentinel that says so out loud when the value
	// goes stale.
	repoRoot := repoRootFromCaller(t)
	entries, err := os.ReadDir(filepath.Join(repoRoot, ".github", "workflows"))
	if err != nil {
		t.Fatalf("read .github/workflows: %v", err)
	}
	sawInstalledFilter := false
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".yml") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(repoRoot, ".github", "workflows", e.Name()))
		if err != nil {
			t.Fatalf("read %s: %v", e.Name(), err)
		}
		body := string(data)
		if !strings.Contains(stripCommentLines(body), "branches: [") {
			continue
		}
		sawInstalledFilter = true
		if !strings.Contains(body, "SCAFFOLDED_BRANCH: ") {
			t.Errorf(".github/workflows/%s filters on a hardcoded branch but carries no "+
				"SCAFFOLDED_BRANCH sentinel — renaming this repository's default branch would stop "+
				"it forever, silently. The templates logmind ships all carry one; the copies logmind "+
				"runs on itself must too.", e.Name())
		}
		if !strings.Contains(body, "DEFAULT_BRANCH: ${{ github.event.repository.default_branch }}") {
			t.Errorf(".github/workflows/%s carries a branch filter but never reads the live default "+
				"branch, so its sentinel has nothing to compare against", e.Name())
		}
		if !strings.Contains(body, `if [ "$SCAFFOLDED_BRANCH" != "$DEFAULT_BRANCH" ]; then`) {
			t.Errorf(".github/workflows/%s does not compare its trigger against the live default branch",
				e.Name())
		}
	}
	if !sawInstalledFilter {
		t.Fatalf("no workflow in .github/workflows/ contains a `branches: [` filter — this test's " +
			"installed-side search is broken, not the tree")
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
