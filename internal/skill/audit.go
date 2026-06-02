package skill

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

// AuditRow mirrors Python's per-skill audit dict. Field tags pin the
// JSON shape so `logmind skill audit --json` stays byte-identical to
// Python's `json.dumps(enriched, indent=2)` output.
type AuditRow struct {
	Name          string `json:"name"`
	Path          string `json:"path"`
	Bytes         int    `json:"bytes"`
	LastModified  string `json:"last_modified"`
	DecisionCount int    `json:"decision_count"`
	// Status is filled by the cli layer (via Classify) when emitting JSON
	// output. The Python code injects it via dict-spread, but the field
	// only appears in JSON output — table output renders it inline. We
	// add the json tag with omitempty so the bare AuditRow shape stays
	// usable without polluting the table output path.
	Status string `json:"status,omitempty"`
}

// auditTightCap mirrors Python's _LOGMIND_SKILL_AUDIT_TIGHT_BYTES. Same
// value as DefaultBenchTarget; declared standalone so audit doesn't
// break if bench is ever extracted.
const auditTightCap = 2000

// AuditSkills walks `.claude/skills/*/SKILL.md` and emits one AuditRow
// per skill found. Empty list if the skills dir is absent or no
// SKILL.md files are present — matches Python's audit_skills behaviour.
func AuditSkills(repoRoot string) []AuditRow {
	skillsDir := SkillsDir(repoRoot)
	info, err := os.Stat(skillsDir)
	if err != nil || !info.IsDir() {
		return nil
	}

	decisionText := readDecisionCorpus(repoRoot)

	entries, err := os.ReadDir(skillsDir)
	if err != nil {
		return nil
	}
	// os.ReadDir is already sorted, but Python's
	// `sorted(skills_dir.iterdir())` uses str-sort over Path objects
	// which produces the same lex order on POSIX systems. Be explicit so
	// cross-platform stays predictable.
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Name() < entries[j].Name()
	})

	var out []AuditRow
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		skillSubdir := filepath.Join(skillsDir, e.Name())
		skillMD := filepath.Join(skillSubdir, "SKILL.md")
		st, err := os.Stat(skillMD)
		if err != nil || !st.Mode().IsRegular() {
			continue
		}
		relPath, err := filepath.Rel(repoRoot, skillMD)
		if err != nil {
			relPath = skillMD
		}
		// Python uses POSIX-style separators in the rel_path field.
		relPath = filepath.ToSlash(relPath)
		name := e.Name()

		lastMod := gitLastTouched(repoRoot, relPath)
		if lastMod == "" {
			lastMod = st.ModTime().Format("2006-01-02")
		}

		// decisionCount: count of whole-word skill-name occurrences in
		// the decision corpus. Diverges intentionally from Python's
		// `decision_text.count(name)` (substring match) per clud-bug
		// PR #124 review — the substring path inflates counts for
		// short skill names (e.g., a skill named "go" matches
		// "going" / "logo" / etc.) and makes the ghost classifier
		// (Classify uses DecisionCount==0 as the gate) effectively
		// useless for common-short-name skills.
		//
		// Parity impact: for normal kebab-slug skill names
		// (`clud-bug-collaboration`, `critical-issues-only`, …) the
		// whole-word and substring counts coincide because the slug
		// is itself a word boundary on both sides. The divergence is
		// strictly a correctness win for short-name skills; the v1.0
		// spec accepts this delta from v0.6.16.
		decisionCount := countWholeWord(decisionText, name)

		out = append(out, AuditRow{
			Name:          name,
			Path:          relPath,
			Bytes:         int(st.Size()),
			LastModified:  lastMod,
			DecisionCount: decisionCount,
		})
	}
	return out
}

// readDecisionCorpus reads docs/decisions.md +
// docs/decisions-branches/*.md and returns the concatenated text.
// Mirrors Python's decision_text concatenation in audit_skills.
func readDecisionCorpus(repoRoot string) string {
	docsDir := filepath.Join(repoRoot, "docs")
	var parts []string

	if data, err := os.ReadFile(filepath.Join(docsDir, "decisions.md")); err == nil {
		parts = append(parts, string(data))
	}

	branchesDir := filepath.Join(docsDir, "decisions-branches")
	if entries, err := os.ReadDir(branchesDir); err == nil {
		sort.Slice(entries, func(i, j int) bool { return entries[i].Name() < entries[j].Name() })
		for _, e := range entries {
			if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
				continue
			}
			if data, err := os.ReadFile(filepath.Join(branchesDir, e.Name())); err == nil {
				parts = append(parts, string(data))
			}
		}
	}
	return strings.Join(parts, "\n")
}

// countWholeWord returns the number of whole-word occurrences of name
// in corpus. Both sides are anchored on `\b` (Go regexp meaning: a
// position between a word and a non-word character), so name="go"
// counts "go" / "Go" but NOT "going" / "logo" / "go-getter".
//
// The compiled regexp lives per-call rather than cached because
// AuditSkills runs a handful of rows total — caching would shave
// microseconds at the cost of map-management complexity. If this ever
// becomes hot (1000+ skills), promote to a sync.Map keyed by name.
func countWholeWord(corpus, name string) int {
	if name == "" || corpus == "" {
		return 0
	}
	re, err := regexp.Compile(`\b` + regexp.QuoteMeta(name) + `\b`)
	if err != nil {
		// Quote-meta should always produce a valid pattern; if it
		// somehow doesn't, surfacing zero is safer than a panic that
		// nukes the audit command.
		return 0
	}
	return len(re.FindAllStringIndex(corpus, -1))
}

// gitLastTouched returns the ISO date of the most recent commit that
// touched relPath, or "" when git isn't available or relPath isn't
// tracked. Mirrors Python's _git_last_touched.
func gitLastTouched(repoRoot, relPath string) string {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "git", "log", "-1", "--format=%cs", "--", relPath)
	cmd.Dir = repoRoot
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// Classify applies the deterministic staleness thresholds to one row.
// Mirrors Python's classify_audit_row — `now` defaults to today's date.
//
//   - ghost: decision_count == 0 AND bytes > auditTightCap
//     (loaded into every context but author never iterates — candidate
//     for clud-bug usage --health to confirm + archive).
//   - aging: last_modified > 90 days ago.
//   - active: otherwise.
func Classify(row AuditRow, now time.Time) string {
	if now.IsZero() {
		now = time.Now()
	}
	if row.DecisionCount == 0 && row.Bytes > auditTightCap {
		return "ghost"
	}
	if row.LastModified != "" {
		lastDate, err := time.Parse("2006-01-02", row.LastModified)
		if err == nil {
			// Compare midnight-to-midnight in UTC to match Python's
			// date.today() arithmetic (which ignores tz/hours).
			lastDay := time.Date(lastDate.Year(), lastDate.Month(), lastDate.Day(), 0, 0, 0, 0, time.UTC)
			today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
			daysOld := int(today.Sub(lastDay).Hours() / 24)
			if daysOld > 90 {
				return "aging"
			}
		}
	}
	return "active"
}
