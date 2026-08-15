// Package doctor implements `logmind doctor` — a read-only stack-status
// command that surfaces version drift between the installed logmind /
// clud-bug workflow templates, agent instruction blocks, git hooks, and
// merge-driver configuration.
//
// Mirrors src/logmind/core/doctor.py at v0.6.16, with deliberate scope
// trims documented in B6's PR description:
//
//   - Workflow drift (regen-timeline, check-doc-links, logmind-self-update,
//     check-decisions) is detected by reading the first-line marker
//     `# logmind-template-version: vN` from each installed file and
//     comparing it to the bundled template.
//   - AGENTS.md block-version drift uses the `<!-- logmind-block-version:
//     vN -->` marker. Both slim + full bundled markers are checked so
//     either install path reports `current`.
//   - Git hooks (post-merge + post-rewrite + commit-msg) drift uses the
//     embedded `# logmind-hook-version: <Version>` marker. Content-diff
//     fast-path is included (v0.6.14): markers can match while body
//     drifts when bytes were manually edited.
//   - Merge-driver state probes .gitattributes block presence + per-clone
//     git config keys.
//   - PATH probe (v0.6.16 carry-forward): `which logmind` resolves to a
//     binary whose `--version` should match the currently-running
//     process. Drift here is the tokenomics-recurrence root cause.
//
// Doctor makes no network calls. The honest "installed version" is the
// running binary's version.Version; the PATH probe above already catches
// a stale on-PATH binary, so no version signal is lost. (The legacy
// best-effort PyPI probe was removed once the Go-era workflows switched
// to thrillmade/setup-logmind and dropped pip-install pins.)
//
// Read-only by design: doctor never writes. The suggested action
// (`brew install thrillmade/tap/logmind` or curl-pipe-bash from
// logmind.dev, then `logmind init`) is printed, not run.
//
// Not implemented (out-of-scope for B6; no single follow-up wave owns
// these — B7 shipped distribution/Homebrew packaging and didn't touch
// doctor):
//
//   - clud-bug status probe (reads `.claude/skills/.clud-bug.json`).
//
//   - check_clud_bug_skill_usage_integration (v0.6.6 upload-step gate).
//
//   - LOGMIND_AUTO_REGEN_PAT secret probe (queries GitHub repo settings).
//
//   - check_stale_derived_docs_warning (a branch-drift warning). Still
//     unimplemented, and NOT obsolete. The (unconditional) L1 restore in
//     internal/cli/log.go's commitDecision makes the common case rare — but
//     it only discards an UNCOMMITTED regen before staging. It cannot
//     detect a branch whose derived docs ALREADY diverged from the
//     merge-base in a landed commit (an older binary regenerating on a
//     branch, a raw `git commit --no-verify`, or a hand edit). Those are
//     exactly the cases the CI gate exists to catch, and a local probe
//     would catch them sooner. Keep it on the list.
package doctor

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/thrillmade/logmind/internal/auto"
	"github.com/thrillmade/logmind/internal/claudehook"
	"github.com/thrillmade/logmind/internal/config"
	"github.com/thrillmade/logmind/internal/decisions"
	"github.com/thrillmade/logmind/internal/gitattr"
	"github.com/thrillmade/logmind/internal/hooks"
	"github.com/thrillmade/logmind/internal/templates"
	"github.com/thrillmade/logmind/internal/timeline"
	"github.com/thrillmade/logmind/internal/version"
)

// Workflow filename constants — matches src/logmind/core/doctor.py
// LOGMIND_WORKFLOWS / CLUD_BUG_WORKFLOWS.
var LogmindWorkflows = []string{
	"regen-timeline.yml",
	"check-doc-links.yml",
	"logmind-self-update.yml",
	"check-decisions.yml",
}

// Marker regexes match Python's compiled patterns at the source.
var (
	logmindMarkerRe       = regexp.MustCompile(`^# logmind-template-version:\s*(\S+)`)
	logmindBlockVersionRe = regexp.MustCompile(`<!--\s*logmind-block-version:\s*(\S+)\s*-->`)
	// logmindVersionLineRe parses the version out of a PATH binary's
	// `--version` output. It anchors on the REAL versionLine format emitted
	// by internal/cli/root.go — `logmind <ver> (spec <ver>)` — capturing the
	// first token after `logmind`. Issue #214: the previous pattern
	// (`version\s+(\S+)`) expected the literal word "version" from Click's
	// legacy Python `logmind, version X` output, so it NEVER matched a real
	// Go binary's line and every on-PATH Go logmind was mis-classified
	// markerless — leaving the PATH-drift row blind.
	// Accepts BOTH the Go `logmind <ver> (spec <ver>)` and the legacy Click
	// `logmind, version <ver>` shapes, so a stale Python binary on PATH is
	// still classified (stale/DRIFT), not silently degraded to markerless
	// (dual-review follow-up to #214).
	logmindVersionLineRe = regexp.MustCompile(`^logmind,?\s+(?:version\s+)?(\S+)`)
)

// WorkflowStatus mirrors the Python dataclass — one row per workflow,
// hook, or other on-disk artifact probe. Reported uniformly so the
// renderer doesn't care about category.
type WorkflowStatus struct {
	Name          string  `json:"name"`
	Installed     bool    `json:"installed"`
	Marker        *string `json:"marker"`
	BundledMarker *string `json:"bundled_marker"`
	Drift         string  `json:"drift"`
}

// ToolStatus aggregates per-tool fields (currently just `logmind`;
// clud-bug deferred). Mirrors Python ToolStatus.
type ToolStatus struct {
	Name             string            `json:"name"`
	InstalledVersion *string           `json:"installed_version"`
	LatestVersion    *string           `json:"latest_version"`
	Workflows        []WorkflowStatus  `json:"workflows"`
	Drift            string            `json:"drift"`
	Extras           map[string]string `json:"extras"`
}

// StatusReport is the doctor's top-level output. Mirrors Python
// StatusReport — including the JSON field names so external tooling
// (clud-bug, agent-skills) can keep parsing the same shape regardless
// of which logmind binary produced it.
type StatusReport struct {
	ProjectRoot string       `json:"project_root"`
	Tools       []ToolStatus `json:"tools"`
	Overall     string       `json:"overall"`
	NetworkUsed bool         `json:"network_used"`
	Suggestions []string     `json:"suggestions"`
	// SummariesNeeded is an ADVISORY list (main-canonical only) of branch
	// detail files missing a §1.6.3 timeline marker or still on a placeholder
	// headline (== the first decision's title). It is a graceful best-practice
	// nudge for the agent to enrich — it NEVER affects Overall (not drift).
	SummariesNeeded []string `json:"summaries_needed"`

	// SpecAdvisories is an ADVISORY list (H2 of the canonical-spec-file
	// feature) of context.spec_file hygiene notes: configured-but-missing,
	// configured-but-empty, configured-with-an-unsafe-path, or a nudge to
	// configure it when a conventional spec file already exists on disk but
	// spec_file is unset. Like SummariesNeeded, this NEVER affects Overall —
	// the spec fold-in is a nice-to-have, not a gate.
	SpecAdvisories []string `json:"spec_advisories"`

	// AutoAdvisories is an ADVISORY list (#241) about `.logmind/auto.yml`,
	// the standing directive `logmind auto <profile>` writes: a directive
	// that predates the current bundled policy, one written for a profile
	// this binary doesn't know, or one carrying no ownership marker. Empty
	// in the (common) case of a repo that never opted into `logmind auto`.
	//
	// Advisory for the same reason SpecAdvisories is: `auto` is an explicit
	// opt-in verb and its directive carries policy a human authored, so
	// nothing here is auto-fixable and none of it is drift. Re-running
	// `logmind auto <profile>` is the deliberate remediation, exactly as
	// `logmind init --spec` is for the spec nudge.
	AutoAdvisories []string `json:"auto_advisories"`
}

// ToJSON serialises the report with 2-space indent, matching Python's
// json.dumps(indent=2) output.
func (r *StatusReport) ToJSON() (string, error) {
	b, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// CollectStatus assembles the full StatusReport. project_root may be
// "" — when empty, the current working directory is used (matches
// Python's Path.cwd() default).
//
// The `offline` parameter is retained for call-site / signature
// compatibility but is now a no-op: doctor makes no network calls, so
// NetworkUsed is always false.
func CollectStatus(projectRoot string, offline bool) StatusReport {
	if projectRoot == "" {
		if cwd, err := os.Getwd(); err == nil {
			projectRoot = cwd
		}
	}
	tools := []ToolStatus{collectLogmindStatus(projectRoot)}

	overall := "OK"
	for _, t := range tools {
		if t.Drift == "stale" {
			overall = "DRIFT"
			break
		}
	}
	if overall == "OK" {
		for _, t := range tools {
			if t.Drift == "unknown" {
				overall = "UNKNOWN"
				break
			}
		}
	}

	var suggestions []string
	for _, t := range tools {
		if t.Drift != "stale" {
			continue
		}
		switch t.Name {
		case "logmind":
			// v1 Go binary distribution — no pip path. The renderer
			// emits each entry on its own indented line under
			// "Suggested:", so split the multi-line remediation into
			// three suggestions to preserve formatting.
			suggestions = append(suggestions,
				"brew install thrillmade/tap/logmind",
				"# or: curl -fsSL https://logmind.dev/install.sh | bash",
				"# then re-run: logmind init",
			)
		case "clud-bug":
			suggestions = append(suggestions, "npx clud-bug update")
		}
	}

	return StatusReport{
		ProjectRoot:     projectRoot,
		Tools:           tools,
		Overall:         overall,
		NetworkUsed:     false,
		Suggestions:     suggestions,
		SummariesNeeded: collectSummariesNeeded(projectRoot),
		SpecAdvisories:  collectSpecAdvisories(projectRoot),
		AutoAdvisories:  collectAutoAdvisories(projectRoot),
	}
}

// collectAutoAdvisories returns the ADVISORY list for `.logmind/auto.yml`
// — the standing directive `logmind auto <profile>` writes (#241).
//
// A repo that never ran `logmind auto` has no directive and gets nothing:
// absence is not drift, exactly as a missing workflow or hook is "not
// installed" rather than "stale". When a directive IS installed, three
// things are worth saying:
//
//   - its ownership marker predates (or postdates) the directive this
//     binary bundles — the policy it restates has moved on;
//   - it names a profile this binary does not know — a newer logmind, a
//     hand-written file, or a typo, and in every case not something to
//     guess at;
//   - it carries no marker at all, so it belongs to the user (SPEC §5.2)
//     and nothing will ever refresh it.
//
// Purely informational: like SpecAdvisories, it never feeds Overall.
// `doctor --fix` deliberately does not act on any of it — the directive
// carries policy a human authored (repo hard stops, the wake mechanism),
// and rewriting that from a template is exactly the failure the whole
// feature exists to prevent.
func collectAutoAdvisories(projectRoot string) []string {
	state := auto.Inspect(projectRoot)
	if !state.Present {
		return nil
	}
	const path = ".logmind/auto.yml"
	if state.Marker == "" {
		return []string{
			path + " carries no `# logmind-auto-version:` marker — it belongs to you, and logmind will " +
				"never refresh it. Move it aside and re-run `logmind auto <profile>` to adopt a managed directive.",
		}
	}
	profile, known := auto.Lookup(state.Profile)
	if !known {
		return []string{
			fmt.Sprintf("%s names profile %q, which this binary does not know (known: %s) — logmind will not "+
				"guess what it should contain. Upgrade logmind, or re-run `logmind auto <profile>`.",
				path, state.Profile, strings.Join(auto.Names(), ", ")),
		}
	}
	bundled, ok := auto.BundledMarker(profile)
	if !ok || bundled == state.Marker {
		return nil
	}
	return []string{
		fmt.Sprintf("%s is at %s; this binary bundles %s for profile %q — the policy it restates has changed. "+
			"Move it aside, re-run `logmind auto %s`, then re-apply your repo-specific slots.",
			path, state.Marker, bundled, profile.Name, profile.Name),
	}
}

// collectSummariesNeeded returns the advisory list of branch detail files
// that lack a §1.6.3 timeline marker, or whose headline is still the
// deterministic placeholder (== the first decision's title, i.e. nobody wrote
// a real one-sentence branch summary). Runs unconditionally (main-canonical is
// the sole timeline as of v2.0.0). Purely informational — it never feeds
// Overall (a graceful nudge, not drift).
func collectSummariesNeeded(projectRoot string) []string {
	files, err := decisions.ListBranchFiles(filepath.Join(projectRoot, "docs", "decisions-branches"))
	if err != nil {
		return nil
	}
	var needed []string
	for _, bf := range files {
		data, err := os.ReadFile(bf)
		if err != nil {
			continue
		}
		content := string(data)
		entries, _ := decisions.Iter(bf, io.Discard)
		if len(entries) == 0 {
			continue // no decisions → nothing to summarize (and --fix would skip it)
		}
		rel := "docs/decisions-branches/" + filepath.Base(bf)
		if !timeline.HasEntryBlocks(content) {
			needed = append(needed, rel+" — no summary (run `logmind doctor --fix` to backfill, then enrich)")
			continue
		}
		current, _ := timeline.CurrentHeadline(content)
		placeholder := timeline.HeadlineLine(entries[0].Date, entries[0].Title)
		if stripPRSuffix(current) == placeholder {
			needed = append(needed, rel+" — placeholder summary (enrich: logmind headline --file "+rel+" \"…\")")
		}
	}
	return needed
}

// specNudgeCandidates lists the conventional spec-file names probed for the
// unset-but-a-file-exists nudge, in priority order.
var specNudgeCandidates = []string{"SPEC.md", "spec.md", "docs/spec.md"}

// collectSpecAdvisories returns the ADVISORY list of context.spec_file
// hygiene notes (H2 of the canonical-spec-file feature):
//
//   - configured but the resolved file is missing
//   - configured but the file is empty/whitespace-only
//   - configured with an absolute or out-of-root path (the same path-safety
//     rule config.ResolveSpecFile enforces for the actual fold-in — this
//     advisory and the real behavior can never disagree)
//   - NUDGE: unset, but a conventional spec file (SPEC.md, spec.md, then
//     docs/spec.md) already exists on disk
//
// At most one condition can apply at a time (spec_file is a single scalar),
// but the return stays a slice for shape-parity with collectSummariesNeeded
// and room to grow. Purely informational — like SummariesNeeded, this NEVER
// affects Overall (a spec fold-in is a nice-to-have, not a gate).
//
// `doctor --fix` deliberately does NOT act on any of these: there is no
// honest mechanical fallback for "which candidate file did the user mean"
// or "what should the missing spec say" — see runDoctorFix's scope-boundary
// comment in the cli package. This function only reports; it never writes.
func collectSpecAdvisories(projectRoot string) []string {
	cfg, err := config.Load(projectRoot)
	if err != nil {
		return nil
	}
	rel := cfg.Context.SpecFile
	if rel == "" {
		for _, candidate := range specNudgeCandidates {
			if _, err := os.Stat(filepath.Join(projectRoot, filepath.FromSlash(candidate))); err == nil {
				return []string{
					candidate + " exists but context.spec_file is unset — set `context.spec_file: " + candidate +
						"` in .logmind/config.yml (or run `logmind init --spec`) to fold it into `logmind context`.",
				}
			}
		}
		return nil
	}

	if _, ok := config.ResolveSpecFile(projectRoot, cfg); !ok {
		reason := "an absolute path"
		if !filepath.IsAbs(rel) {
			reason = "a path that escapes the repo root"
		}
		return []string{
			fmt.Sprintf("context.spec_file (%s) is %s — logmind treats this as UNSET; use a repo-relative path that stays inside the repo.", rel, reason),
		}
	}

	path := filepath.Join(projectRoot, filepath.FromSlash(rel))
	data, err := os.ReadFile(path)
	if err != nil {
		return []string{
			"context.spec_file is configured (" + rel + ") but the file is missing — create it (e.g. `logmind init --spec`) or unset context.spec_file.",
		}
	}
	if strings.TrimSpace(string(data)) == "" {
		return []string{
			"context.spec_file (" + rel + ") is empty — add content, or unset context.spec_file.",
		}
	}
	return nil
}

var prSuffixRe = regexp.MustCompile(` \(#\d+\)$`)

// stripPRSuffix removes a trailing " (#NN)" PR suffix so a placeholder headline
// compares equal whether or not LOGMIND_PR was set when the marker was written.
func stripPRSuffix(s string) string {
	return prSuffixRe.ReplaceAllString(s, "")
}

// collectLogmindStatus builds the per-tool report for `logmind`. The
// "installed version" is the running binary's version.Version — doctor
// no longer parses pip-install pins or queries PyPI. The on-PATH probe
// (probePathResolution) still surfaces a stale binary resolved ahead of
// this one on PATH, so no version-drift signal is lost.
func collectLogmindStatus(projectRoot string) ToolStatus {
	running := version.Version

	var workflows []WorkflowStatus
	for _, name := range LogmindWorkflows {
		workflows = append(workflows, probeWorkflow(projectRoot, name, bundledLogmindMarker(name)))
	}
	workflows = append(workflows, probeAgentsMD(projectRoot))
	workflows = append(workflows, probeMergeDriverAttrs(projectRoot))
	workflows = append(workflows, probeMergeDriverConfig(projectRoot))
	workflows = append(workflows, probePostMergeHook(projectRoot))
	workflows = append(workflows, probePostRewriteHook(projectRoot))
	workflows = append(workflows, probeCommitMsgHook(projectRoot))
	// v2.0.0 L2a probe — the pin-preservation pre-commit hook. Sits right
	// after commit-msg (the other commit-time git hook) and right before
	// the Claude Code harness guard (L2b uses the same restore, different
	// layer) since all three are part of the same enforcement/pin story.
	workflows = append(workflows, probePreCommitHook(projectRoot))
	// v2.0.0 Layer 1 probe — the Claude Code harness's PreToolUse guard
	// entry in .claude/settings.json. Sits right after the commit-msg row
	// (Layer 2) since the two are the enforcement feature's matched pair.
	workflows = append(workflows, probeClaudePreToolUseHook(projectRoot))
	// v0.6.16 PATH-resolution probe — always appended; the probe itself
	// decides whether to report stale/current/missing.
	workflows = append(workflows, probePathResolution())

	drift := classifyLogmindDrift(workflows)
	return ToolStatus{
		Name:             "logmind",
		InstalledVersion: &running,
		LatestVersion:    nil,
		Workflows:        workflows,
		Drift:            drift,
		Extras:           map[string]string{},
	}
}

// StaleCount runs the same probe set collectLogmindStatus uses (workflow /
// hook / marker drift classification) and returns how many components are
// classified STALE — the one drift class that flips Overall to DRIFT.
// "missing" (never installed) and "markerless" (hand-edited, pre-marker)
// are deliberately excluded: both are benign by design elsewhere in this
// package (see classifyLogmindDrift), so a caller that only cares about
// "does this repo need `doctor --fix`" should only count the same signal
// doctor itself gates on.
//
// This is the FULL probe set — including probePathResolution (a PATH lookup
// + a live subprocess) and probeMergeDriverConfig (a `git config` shell-out)
// — meant for on-demand callers like `logmind doctor` itself, which can
// afford a subprocess or two. `logmind log`'s hot path cannot: see
// StaleCountFast below.
func StaleCount(projectRoot string) int {
	status := collectLogmindStatus(projectRoot)
	count := 0
	for _, wf := range status.Workflows {
		if wf.Drift == "stale" {
			count++
		}
	}
	return count
}

// collectLogmindStatusFast runs the FILE-READ-ONLY subset of
// collectLogmindStatus's probes — every probe here does nothing but stat/
// read a local file (or parse embedded template bytes already resident in
// the binary). No probe here forks a subprocess, so the whole set costs a
// handful of stat/read syscalls: single-digit milliseconds, safe to run on
// EVERY `logmind log` invocation.
//
// EXCLUDED, and why:
//
//   - probePathResolution: does `exec.LookPath("logmind")` followed by a
//     live `<path> --version` subprocess. A hung PATH binary — or a
//     daemonizing wrapper whose grandchild inherits the output pipe past
//     the parent's own exit — can stall this for seconds or hang it
//     outright, exactly the failure mode the hot path must never risk. See
//     probePathResolution's WaitDelay hardening below, which bounds this
//     for the on-demand `doctor` path (which CAN tolerate a bounded
//     subprocess wait; `logmind log` cannot tolerate any).
//   - probeMergeDriverConfig: shells out to `git config --get` (via
//     gitattr.DriverConfigured) — a subprocess, the same category of risk
//     as above even though `git` itself is normally well-behaved.
//
// Both excluded signals — on-PATH version drift and merge-driver config
// drift — still surface via an on-demand `logmind doctor` run (the full
// probe set, StaleCount above); they're simply not worth paying a
// subprocess for on every single `logmind log`.
func collectLogmindStatusFast(projectRoot string) []WorkflowStatus {
	var workflows []WorkflowStatus
	for _, name := range LogmindWorkflows {
		workflows = append(workflows, probeWorkflow(projectRoot, name, bundledLogmindMarker(name)))
	}
	workflows = append(workflows, probeAgentsMD(projectRoot))
	workflows = append(workflows, probeMergeDriverAttrs(projectRoot)) // file read only (.gitattributes) — safe
	workflows = append(workflows, probePostMergeHook(projectRoot))
	workflows = append(workflows, probePostRewriteHook(projectRoot))
	workflows = append(workflows, probeCommitMsgHook(projectRoot))
	workflows = append(workflows, probePreCommitHook(projectRoot))
	workflows = append(workflows, probeClaudePreToolUseHook(projectRoot))
	return workflows
}

// StaleCountFast is StaleCount's subprocess-free subset, built from
// collectLogmindStatusFast — see that function's doc comment for exactly
// which probes are excluded and why. This is what `logmind log`'s pulse
// advisory calls (internal/cli/pulse.go driftPulseLine): the hot-path
// budget is single-digit milliseconds, not "however long a PATH binary
// takes to answer `--version`, if it answers at all."
func StaleCountFast(projectRoot string) int {
	workflows := collectLogmindStatusFast(projectRoot)
	count := 0
	for _, wf := range workflows {
		if wf.Drift == "stale" {
			count++
		}
	}
	return count
}

// classifyLogmindDrift aggregates per-workflow drift into the tool-level
// drift. ANY stale workflow/hook/probe row flips the tool to "stale";
// remaining unknowns flip to "unknown"; otherwise "ok". (The version
// signal now lives entirely in the on-PATH probe row — there's no
// installed-vs-latest comparison since doctor makes no network calls.)
func classifyLogmindDrift(workflows []WorkflowStatus) string {
	for _, wf := range workflows {
		if wf.Drift == "stale" {
			return "stale"
		}
	}
	for _, wf := range workflows {
		if wf.Drift == "unknown" {
			return "unknown"
		}
	}
	return "ok"
}

func bundledLogmindMarker(workflowName string) *string {
	body := templates.Workflow(workflowName + ".template")
	first := firstLine(body)
	m := logmindMarkerRe.FindStringSubmatch(first)
	if len(m) < 2 {
		return nil
	}
	v := m[1]
	return &v
}

func firstLine(s string) string {
	if idx := strings.Index(s, "\n"); idx >= 0 {
		return s[:idx]
	}
	return s
}

func readWorkflow(projectRoot, name string) (string, bool) {
	data, err := os.ReadFile(filepath.Join(projectRoot, ".github", "workflows", name))
	if err != nil {
		return "", false
	}
	return string(data), true
}

func classifyMarker(marker, bundled *string) string {
	if marker == nil {
		return "markerless"
	}
	if bundled == nil {
		return "unknown"
	}
	if *marker == *bundled {
		return "current"
	}
	return "stale"
}

func probeWorkflow(projectRoot, name string, bundled *string) WorkflowStatus {
	content, ok := readWorkflow(projectRoot, name)
	if !ok {
		return WorkflowStatus{Name: name, Installed: false, Marker: nil, BundledMarker: bundled, Drift: "missing"}
	}
	first := firstLine(content)
	m := logmindMarkerRe.FindStringSubmatch(first)
	var marker *string
	if len(m) >= 2 {
		v := m[1]
		marker = &v
	}
	return WorkflowStatus{
		Name: name, Installed: true, Marker: marker,
		BundledMarker: bundled, Drift: classifyMarker(marker, bundled),
	}
}

// bundledAgentsMDBlockVersions returns (slim, full) — both bundled
// AGENTS.md template markers. Either match is acceptable as "current".
func bundledAgentsMDBlockVersions() (*string, *string) {
	read := func(text string) *string {
		m := logmindBlockVersionRe.FindStringSubmatch(text)
		if len(m) < 2 {
			return nil
		}
		v := m[1]
		return &v
	}
	return read(templates.AgentsSlimTemplate()), read(templates.AgentsTemplate())
}

func probeAgentsMD(projectRoot string) WorkflowStatus {
	slim, full := bundledAgentsMDBlockVersions()
	display := slim
	if display == nil {
		display = full
	}
	path := filepath.Join(projectRoot, "AGENTS.md")
	data, err := os.ReadFile(path)
	if err != nil {
		return WorkflowStatus{
			Name: "AGENTS.md", Installed: false, Marker: nil,
			BundledMarker: display, Drift: "missing",
		}
	}
	m := logmindBlockVersionRe.FindStringSubmatch(string(data))
	var marker *string
	if len(m) >= 2 {
		v := m[1]
		marker = &v
	}
	drift := "markerless"
	switch {
	case marker == nil:
		drift = "markerless"
	case slim == nil && full == nil:
		drift = "unknown"
	case (slim != nil && *marker == *slim) || (full != nil && *marker == *full):
		drift = "current"
	default:
		drift = "stale"
	}
	return WorkflowStatus{
		Name: "AGENTS.md", Installed: true, Marker: marker,
		BundledMarker: display, Drift: drift,
	}
}

func probeMergeDriverAttrs(projectRoot string) WorkflowStatus {
	present := "present"
	if gitattr.HasBlock(filepath.Join(projectRoot, ".gitattributes")) {
		return WorkflowStatus{
			Name: ".gitattributes (merge driver)", Installed: true,
			Marker: &present, BundledMarker: &present, Drift: "current",
		}
	}
	return WorkflowStatus{
		Name: ".gitattributes (merge driver)", Installed: false,
		Marker: nil, BundledMarker: &present, Drift: "missing",
	}
}

func probeMergeDriverConfig(projectRoot string) WorkflowStatus {
	configured := "configured"
	if _, err := os.Stat(filepath.Join(projectRoot, ".git")); err != nil {
		return WorkflowStatus{
			Name: "git config (merge driver)", Installed: false,
			Marker: nil, BundledMarker: nil, Drift: "missing",
		}
	}
	if gitattr.DriverConfigured(projectRoot) {
		return WorkflowStatus{
			Name: "git config (merge driver)", Installed: true,
			Marker: &configured, BundledMarker: &configured, Drift: "current",
		}
	}
	return WorkflowStatus{
		Name: "git config (merge driver)", Installed: false,
		Marker: nil, BundledMarker: &configured, Drift: "missing",
	}
}

// probeHook is the shared helper behind post-merge / post-rewrite /
// commit-msg probes. `installedBody` is the canonical body the current
// Go binary would write; content drift means installed bytes != bundled.
func probeHook(projectRoot, displayName, hookFile string, bundledBody string) WorkflowStatus {
	current := version.Version
	if _, err := os.Stat(filepath.Join(projectRoot, ".git")); err != nil {
		return WorkflowStatus{
			Name: displayName, Installed: false,
			Marker: nil, BundledMarker: nil, Drift: "missing",
		}
	}
	path := filepath.Join(projectRoot, ".git", "hooks", hookFile)
	if _, err := os.Stat(path); err != nil {
		return WorkflowStatus{
			Name: displayName, Installed: false,
			Marker: nil, BundledMarker: &current, Drift: "missing",
		}
	}
	hookVer, ok := hooks.ExtractVersion(path)
	if !ok {
		marker := "markerless (pre-v0.6.10)"
		return WorkflowStatus{
			Name: displayName, Installed: true,
			Marker: &marker, BundledMarker: &current, Drift: "markerless",
		}
	}
	if hookVer != current {
		return WorkflowStatus{
			Name: displayName, Installed: true,
			Marker: &hookVer, BundledMarker: &current, Drift: "stale",
		}
	}
	// Content-diff fast-path (v0.6.14).
	installedBody, err := os.ReadFile(path)
	if err == nil && string(installedBody) != bundledBody {
		marker := hookVer + " (content drift)"
		return WorkflowStatus{
			Name: displayName, Installed: true,
			Marker: &marker, BundledMarker: &current, Drift: "stale",
		}
	}
	return WorkflowStatus{
		Name: displayName, Installed: true,
		Marker: &hookVer, BundledMarker: &current, Drift: "current",
	}
}

func probePostMergeHook(projectRoot string) WorkflowStatus {
	return probeHook(projectRoot, "post-merge hook", "post-merge", hooks.BuildPostMergeBody())
}

func probePostRewriteHook(projectRoot string) WorkflowStatus {
	return probeHook(projectRoot, "post-rewrite hook", "post-rewrite", hooks.BuildPostRewriteBody())
}

// probeCommitMsgHook reports the v0.6.16 commit-msg hook state. When
// the hook isn't yet installed (older logmind installs), `missing` is
// the natural state — the next `logmind init` or `logmind self-update`
// will install it.
func probeCommitMsgHook(projectRoot string) WorkflowStatus {
	return probeHook(projectRoot, "commit-msg hook", "commit-msg", hooks.BuildCommitMsgBody())
}

// probePreCommitHook reports drift for L2a of the v2.0.0 derived-docs
// pin-preservation design — the `pre-commit` git hook installed by
// hooks.InstallPreCommit (see hooks.BuildPreCommitBody). Unlike the three
// probeHook-based probes above, a pre-commit hook file MAY ALREADY be owned
// by a totally different, older, opt-in logmind feature: `logmind
// install-hook`'s `check-decisions` body (internal/cli/install_hook.go),
// which carries no hooks.PreCommitMarker of its own. Neither that body nor
// a hand-written hook is drift — both are reported "foreign" (Installed:
// true) rather than "stale"/"markerless", which classifyLogmindDrift falls
// through to "ok" for, exactly like a missing hook: `logmind init` and
// `doctor --fix` both deliberately leave a foreign pre-commit hook alone
// (see hooks.installHook), so there is nothing to auto-fix here either.
//
// File-read only (stat + read, no subprocess) — safe for the fast path;
// see StaleCountFast's doc comment for why that matters.
func probePreCommitHook(projectRoot string) WorkflowStatus {
	const displayName = "pre-commit hook (derived-docs pin)"
	current := version.Version
	if _, err := os.Stat(filepath.Join(projectRoot, ".git")); err != nil {
		return WorkflowStatus{
			Name: displayName, Installed: false,
			Marker: nil, BundledMarker: nil, Drift: "missing",
		}
	}
	path := filepath.Join(projectRoot, ".git", "hooks", "pre-commit")
	data, err := os.ReadFile(path)
	if err != nil {
		return WorkflowStatus{
			Name: displayName, Installed: false,
			Marker: nil, BundledMarker: &current, Drift: "missing",
		}
	}
	if !strings.Contains(string(data), hooks.PreCommitMarker) {
		marker := "foreign pre-commit hook present — left alone"
		return WorkflowStatus{
			Name: displayName, Installed: true,
			Marker: &marker, BundledMarker: &current, Drift: "foreign",
		}
	}
	hookVer, ok := hooks.ExtractVersion(path)
	if !ok {
		marker := "markerless"
		return WorkflowStatus{
			Name: displayName, Installed: true,
			Marker: &marker, BundledMarker: &current, Drift: "markerless",
		}
	}
	if hookVer != current {
		return WorkflowStatus{
			Name: displayName, Installed: true,
			Marker: &hookVer, BundledMarker: &current, Drift: "stale",
		}
	}
	if string(data) != hooks.BuildPreCommitBody() {
		marker := hookVer + " (content drift)"
		return WorkflowStatus{
			Name: displayName, Installed: true,
			Marker: &marker, BundledMarker: &current, Drift: "stale",
		}
	}
	return WorkflowStatus{
		Name: displayName, Installed: true,
		Marker: &hookVer, BundledMarker: &current, Drift: "current",
	}
}

// probeClaudePreToolUseHook reports drift for Layer 1 of the v2.0.0
// commit-enforcement design — the Claude Code harness's PreToolUse guard
// entry in .claude/settings.json (installed/refreshed by
// internal/claudehook.EnsurePreToolUseGuard). Mirrors probeHook's shape
// and drift vocabulary exactly (missing / markerless / stale / current)
// so it participates in classifyLogmindDrift identically to the git-hook
// probes: only "stale" flips the tool (and therefore Overall) to DRIFT.
// "missing" stays benign here for the same reason it's benign for the
// git hooks — a repo that simply hasn't run `logmind init` yet (or has
// the claude agent disabled) isn't "drifted," it's "not installed," and
// `doctor --fix` / `logmind init` remain the remediation either way.
func probeClaudePreToolUseHook(projectRoot string) WorkflowStatus {
	current := version.Version
	const displayName = "Claude Code PreToolUse guard"
	state := claudehook.Inspect(projectRoot)
	if !state.SettingsPresent || !state.EntryPresent {
		return WorkflowStatus{
			Name: displayName, Installed: false,
			Marker: nil, BundledMarker: &current, Drift: "missing",
		}
	}
	if !state.HasMarker {
		marker := "markerless (pre-v2.0.0)"
		return WorkflowStatus{
			Name: displayName, Installed: true,
			Marker: &marker, BundledMarker: &current, Drift: "markerless",
		}
	}
	installed := state.Version
	if installed != current {
		return WorkflowStatus{
			Name: displayName, Installed: true,
			Marker: &installed, BundledMarker: &current, Drift: "stale",
		}
	}
	return WorkflowStatus{
		Name: displayName, Installed: true,
		Marker: &installed, BundledMarker: &current, Drift: "current",
	}
}

// probePathResolution implements the v0.6.16 PATH-resolution check.
//
// Returns a WorkflowStatus that mirrors Python's
// src/logmind/core/doctor._probe_path_resolution:
//
//   - drift="current"   when `which logmind`'s --version matches the
//     running binary.
//   - drift="stale"     when versions differ — marker shows both
//     versions + the conflicting path so the user
//     can act on it without invoking `which -a`.
//   - drift="missing"   when no logmind found on PATH (merge driver
//     shell-outs will fail).
//   - drift="markerless" when the PATH binary exists but its
//     --version is unreadable / unparseable.
//
// Errors are best-effort: every failure path produces a status row
// (no panics). This is the v0.6.16 carry-forward that bubbles up
// the tokenomics-recurrence root cause at doctor time.
func probePathResolution() WorkflowStatus {
	running := version.Version
	pathBin, err := exec.LookPath("logmind")
	if err != nil {
		return WorkflowStatus{
			Name: "logmind on PATH", Installed: false,
			Marker: nil, BundledMarker: &running, Drift: "missing",
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, pathBin, "--version")
	// WaitDelay bounds how long Wait (called internally by CombinedOutput)
	// keeps blocking on the command's I/O pipes AFTER ctx's timeout kills
	// the direct child. Without it, a wrapper script that daemonizes —
	// forks a grandchild and exits — leaves that grandchild holding the
	// stdout/stderr pipe open; CombinedOutput blocks reading from the pipe
	// until EOF, which never arrives, so the 5s context timeout (which only
	// SIGKILLs the direct child) doesn't actually bound this call. WaitDelay
	// (Go 1.20+) forces the pipe closed and Wait to return once this grace
	// period elapses after the context kill signal, capping the worst case
	// at ctx-timeout + WaitDelay instead of unbounded.
	cmd.WaitDelay = 2 * time.Second
	out, runErr := cmd.CombinedOutput()
	if runErr != nil && len(out) == 0 {
		marker := fmt.Sprintf("%s (cannot exec --version)", pathBin)
		return WorkflowStatus{
			Name: "logmind on PATH", Installed: true,
			Marker: &marker, BundledMarker: &running, Drift: "markerless",
		}
	}
	text := strings.TrimSpace(string(out))
	m := logmindVersionLineRe.FindStringSubmatch(text)
	if len(m) < 2 {
		marker := fmt.Sprintf("%s (no version parsed from %q)", pathBin, text)
		return WorkflowStatus{
			Name: "logmind on PATH", Installed: true,
			Marker: &marker, BundledMarker: &running, Drift: "markerless",
		}
	}
	pathVer := strings.TrimRight(m[1], ",")
	if pathVer == running {
		marker := fmt.Sprintf("%s (%s)", pathBin, pathVer)
		return WorkflowStatus{
			Name: "logmind on PATH", Installed: true,
			Marker: &marker, BundledMarker: &running, Drift: "current",
		}
	}
	marker := fmt.Sprintf(
		"%s (%s) — STALE; running binary is %s. `which -a logmind` shows full PATH order.",
		pathBin, pathVer, running,
	)
	return WorkflowStatus{
		Name: "logmind on PATH", Installed: true,
		Marker: &marker, BundledMarker: &running, Drift: "stale",
	}
}

// formatVersion renders an optional version pointer — the dereferenced
// value when set, else "unknown".
func formatVersion(v *string) string {
	if v != nil {
		return *v
	}
	return "unknown"
}

func formatDrift(drift string) string {
	switch drift {
	case "ok":
		return "current ✓"
	case "stale":
		return "STALE"
	case "unknown":
		return "unknown"
	case "current":
		return "current"
	case "markerless":
		return "markerless"
	case "missing":
		return "—"
	case "foreign":
		return "foreign (left alone)"
	}
	return drift
}

// RenderStatus formats a StatusReport for the default (non-JSON)
// output mode. Mirrors src/logmind/core/doctor.render_status.
func RenderStatus(r StatusReport) string {
	var lines []string

	// Stable iteration: tools were already inserted in order by the
	// collector; loop preserves it.
	for _, tool := range r.Tools {
		installed := formatVersion(tool.InstalledVersion)
		statusWord := formatDrift("ok")
		if tool.Drift != "ok" {
			statusWord = formatDrift(tool.Drift)
		}
		lines = append(lines, fmt.Sprintf(
			"%s %s · %s",
			tool.Name, installed, statusWord,
		))

		for _, wf := range tool.Workflows {
			if !wf.Installed && wf.Drift == "missing" {
				if wf.BundledMarker == nil {
					continue
				}
				lines = append(lines, fmt.Sprintf(
					"  %-28s —    not installed (latest: %s)",
					wf.Name, *wf.BundledMarker,
				))
				continue
			}
			marker := "—"
			if wf.Marker != nil {
				marker = *wf.Marker
			}
			driftWord := formatDrift(wf.Drift)
			if wf.Drift == "stale" && wf.BundledMarker != nil {
				driftWord = fmt.Sprintf("STALE (latest: %s)", *wf.BundledMarker)
			}
			lines = append(lines, fmt.Sprintf("  %-28s %-4s %s", wf.Name, marker, driftWord))
		}

		// Extras (stable order: alphabetical for now; Python preserves dict
		// insertion order, but the only extra we currently emit is
		// `strict_mode` from clud-bug — deferred — so this matches.)
		var extraKeys []string
		for k := range tool.Extras {
			extraKeys = append(extraKeys, k)
		}
		sort.Strings(extraKeys)
		for _, key := range extraKeys {
			label := strings.ReplaceAll(key, "_", " ")
			lines = append(lines, fmt.Sprintf("  %-28s %s", label, tool.Extras[key]))
		}

		lines = append(lines, "")
	}

	lines = append(lines, fmt.Sprintf("Stack status: %s", r.Overall))
	if len(r.Suggestions) > 0 {
		lines = append(lines, "Suggested:")
		for _, s := range r.Suggestions {
			lines = append(lines, fmt.Sprintf("  %s", s))
		}
	}
	if len(r.SummariesNeeded) > 0 {
		lines = append(lines, "")
		lines = append(lines, fmt.Sprintf("Branch summaries needing attention (%d) — enrich the important ones:", len(r.SummariesNeeded)))
		for _, s := range r.SummariesNeeded {
			lines = append(lines, fmt.Sprintf("  • %s", s))
		}
	}
	if len(r.SpecAdvisories) > 0 {
		lines = append(lines, "")
		lines = append(lines, fmt.Sprintf("Canonical spec file (%d):", len(r.SpecAdvisories)))
		for _, s := range r.SpecAdvisories {
			lines = append(lines, fmt.Sprintf("  • %s", s))
		}
	}
	if len(r.AutoAdvisories) > 0 {
		lines = append(lines, "")
		lines = append(lines, fmt.Sprintf("Unattended-operation directive (%d):", len(r.AutoAdvisories)))
		for _, s := range r.AutoAdvisories {
			lines = append(lines, fmt.Sprintf("  • %s", s))
		}
	}
	return strings.Join(lines, "\n")
}
