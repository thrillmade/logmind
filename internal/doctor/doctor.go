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
//   - PyPI probe is best-effort (2s timeout, swallows all errors).
//
// Read-only by design: doctor never writes. The suggested action
// (`pip install --upgrade logmind && logmind init`) is printed, not run.
//
// Deferred (out-of-scope for B6, tracked for B7 follow-up):
//
//   - clud-bug status probe (reads `.claude/skills/.clud-bug.json`).
//   - check_stale_derived_docs_warning (Phase-D divergence detection).
//   - check_clud_bug_skill_usage_integration (v0.6.6 upload-step gate).
//   - LOGMIND_AUTO_REGEN_PAT secret probe (queries GitHub repo settings).
package doctor

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"time"

	"github.com/thrillmade/logmind/internal/gitattr"
	"github.com/thrillmade/logmind/internal/hooks"
	"github.com/thrillmade/logmind/internal/templates"
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
	logmindPinRe          = regexp.MustCompile(`pip install\s+["']?logmind==([\d.]+)["']?`)
	logmindMarkerRe       = regexp.MustCompile(`^# logmind-template-version:\s*(\S+)`)
	logmindBlockVersionRe = regexp.MustCompile(`<!--\s*logmind-block-version:\s*(\S+)\s*-->`)
	logmindVersionLineRe  = regexp.MustCompile(`version\s+(\S+)`)
)

// WorkflowStatus mirrors the Python dataclass — one row per workflow,
// hook, or other on-disk artifact probe. Reported uniformly so the
// renderer doesn't care about category.
type WorkflowStatus struct {
	Name           string  `json:"name"`
	Installed      bool    `json:"installed"`
	Marker         *string `json:"marker"`
	BundledMarker  *string `json:"bundled_marker"`
	Drift          string  `json:"drift"`
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

// httpGetJSON is a best-effort JSON GET — returns nil on any failure
// (timeout, network, parse, status>=400). Mirrors Python's
// _http_get_json semantics: never raises.
func httpGetJSON(url string, timeout time.Duration) map[string]any {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil
	}
	req.Header.Set("User-Agent", "logmind-doctor")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return nil
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil
	}
	var out map[string]any
	if err := json.Unmarshal(body, &out); err != nil {
		return nil
	}
	return out
}

// CollectStatus assembles the full StatusReport. project_root may be
// "" — when empty, the current working directory is used (matches
// Python's Path.cwd() default).
func CollectStatus(projectRoot string, offline bool) StatusReport {
	if projectRoot == "" {
		if cwd, err := os.Getwd(); err == nil {
			projectRoot = cwd
		}
	}
	tools := []ToolStatus{collectLogmindStatus(projectRoot, offline)}

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
			suggestions = append(suggestions, "pip install --upgrade logmind && logmind init")
		case "clud-bug":
			suggestions = append(suggestions, "npx clud-bug update")
		}
	}

	return StatusReport{
		ProjectRoot: projectRoot,
		Tools:       tools,
		Overall:     overall,
		NetworkUsed: !offline,
		Suggestions: suggestions,
	}
}

// collectLogmindStatus builds the per-tool report for `logmind`.
func collectLogmindStatus(projectRoot string, offline bool) ToolStatus {
	installed := logmindInstalledVersion(projectRoot)
	var latest *string
	if !offline {
		if data := httpGetJSON("https://pypi.org/pypi/logmind/json", 2*time.Second); data != nil {
			if info, ok := data["info"].(map[string]any); ok {
				if v, ok := info["version"].(string); ok {
					latest = &v
				}
			}
		}
	}

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
	// v0.6.16 PATH-resolution probe — always appended; the probe itself
	// decides whether to report stale/current/missing.
	workflows = append(workflows, probePathResolution())

	drift := classifyLogmindDrift(installed, latest, workflows, projectRoot)
	return ToolStatus{
		Name:             "logmind",
		InstalledVersion: installed,
		LatestVersion:    latest,
		Workflows:        workflows,
		Drift:            drift,
		Extras:           map[string]string{},
	}
}

// classifyLogmindDrift aggregates per-workflow drift into the tool-level
// drift. ANY stale workflow OR (installed != latest) flips the tool to
// "stale"; remaining unknowns flip to "unknown"; otherwise "ok". Mirrors
// the Python aggregation in collect_logmind_status.
func classifyLogmindDrift(installed, latest *string, workflows []WorkflowStatus, projectRoot string) string {
	// Version mismatch (when both known) — stale.
	if installed != nil && latest != nil && *installed != *latest {
		return "stale"
	}
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

func logmindInstalledVersion(projectRoot string) *string {
	content, err := os.ReadFile(filepath.Join(projectRoot, ".github", "workflows", "regen-timeline.yml"))
	if err != nil {
		return nil
	}
	m := logmindPinRe.FindStringSubmatch(string(content))
	if len(m) < 2 {
		return nil
	}
	v := m[1]
	return &v
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

// hookInstalledVersion returns the embedded `# logmind-hook-version:`
// marker from a hook body, or nil when the marker line is missing.
// Mirrors src/logmind/core/gitattributes.installed_<*>_hook_version.
func hookInstalledVersion(path string) (string, bool) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", false
	}
	for _, line := range strings.Split(string(data), "\n") {
		if rest, ok := strings.CutPrefix(line, hooks.HookVersionPrefix); ok {
			return strings.TrimSpace(rest), true
		}
	}
	return "", false
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
	hookVer, ok := hookInstalledVersion(path)
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

// probePathResolution implements the v0.6.16 PATH-resolution check.
//
// Returns a WorkflowStatus that mirrors Python's
// src/logmind/core/doctor._probe_path_resolution:
//
//   - drift="current"   when `which logmind`'s --version matches the
//                       running binary.
//   - drift="stale"     when versions differ — marker shows both
//                       versions + the conflicting path so the user
//                       can act on it without invoking `which -a`.
//   - drift="missing"   when no logmind found on PATH (merge driver
//                       shell-outs will fail).
//   - drift="markerless" when the PATH binary exists but its
//                       --version is unreadable / unparseable.
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

// formatVersion renders an optional version pointer with the same
// Python `?` / `(offline)` placeholder for the "latest" column when the
// PyPI probe couldn't return a value.
func formatVersion(v *string, offline bool) string {
	if v != nil {
		return *v
	}
	if offline {
		return "(offline)"
	}
	return "?"
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
	}
	return drift
}

// RenderStatus formats a StatusReport for the default (non-JSON)
// output mode. Mirrors src/logmind/core/doctor.render_status.
func RenderStatus(r StatusReport) string {
	offline := !r.NetworkUsed
	var lines []string

	// Stable iteration: tools were already inserted in order by the
	// collector; loop preserves it.
	for _, tool := range r.Tools {
		installed := formatVersion(tool.InstalledVersion, false)
		latest := formatVersion(tool.LatestVersion, offline)
		if tool.InstalledVersion == nil && tool.Name == "logmind" {
			installed = "(dev install)"
		}
		statusWord := formatDrift("ok")
		if tool.Drift != "ok" {
			statusWord = formatDrift(tool.Drift)
		}
		lines = append(lines, fmt.Sprintf(
			"%s %s installed · %s latest · %s",
			tool.Name, installed, latest, statusWord,
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
	return strings.Join(lines, "\n")
}

// runtimeOS is exported for tests that want to swap behaviour by GOOS.
// Currently unused but kept as a hook for future Windows-only branches.
func runtimeOS() string { return runtime.GOOS }
