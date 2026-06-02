package skill

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
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

		decisionCount := 0
		if name != "" && decisionText != "" {
			decisionCount = strings.Count(decisionText, name)
		}

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
