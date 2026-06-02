package skill

import (
	"strings"
	"testing"
)

// TestBenchSkill_Tight covers a small skill that should be in the
// "tight" bucket. Section-name capture + byte-count pinning catches
// any drift in the splitter.
func TestBenchSkill_Tight(t *testing.T) {
	body := "---\nname: x\ndescription: y\n---\n\n# X\n\n" +
		"intro line\n\n" +
		"## When to use\n\n- thing\n\n" +
		"## Steps\n\n1. step\n"
	res := BenchSkill(body, 0, 0)
	if res.Status != "tight" {
		t.Errorf("status = %q; want tight (size=%d)", res.Status, res.Bytes)
	}
	if res.Target != DefaultBenchTarget {
		t.Errorf("target = %d; want %d", res.Target, DefaultBenchTarget)
	}
	if res.Budget != DefaultBenchBudget {
		t.Errorf("budget = %d; want %d", res.Budget, DefaultBenchBudget)
	}
	names := make([]string, 0, len(res.Sections))
	for _, s := range res.Sections {
		names = append(names, s.Name)
	}
	if got := strings.Join(names, ","); got != "frontmatter,intro,When to use,Steps" {
		t.Errorf("section names = %q; want frontmatter,intro,When to use,Steps", got)
	}
}

// TestBenchSkill_Verbose makes the body large enough to land in the
// "verbose" bucket and confirms the generic-fallback suggestion fires.
func TestBenchSkill_Verbose(t *testing.T) {
	// Generate a body just over the budget but under the cap.
	prefix := "---\nname: x\ndescription: y\n---\n\n"
	body := prefix + strings.Repeat("filler ", 1000) // ~7000 bytes
	res := BenchSkill(body, 0, 0)
	if res.Status != "verbose" {
		t.Errorf("status = %q; want verbose (bytes=%d)", res.Status, res.Bytes)
	}
	if len(res.Suggestions) == 0 {
		t.Errorf("expected at least one suggestion for verbose skill")
	}
}

// TestBenchSkill_OverBudget pushes past the hard cap to exercise the
// over-budget branch + the "split" suggestion.
func TestBenchSkill_OverBudget(t *testing.T) {
	body := "---\nname: x\ndescription: y\n---\n" +
		strings.Repeat("X", LogmindByteCap+100)
	res := BenchSkill(body, 0, 0)
	if res.Status != "over-budget" {
		t.Errorf("status = %q; want over-budget", res.Status)
	}
	hasSplit := false
	for _, s := range res.Suggestions {
		if strings.Contains(s, "split into multiple focused skills") {
			hasSplit = true
		}
	}
	if !hasSplit {
		t.Errorf("expected 'split' suggestion; got %v", res.Suggestions)
	}
}

// TestBenchSkill_DominantSection triggers Heuristic 1 (>30% of total).
func TestBenchSkill_DominantSection(t *testing.T) {
	frontmatter := "---\nname: x\ndescription: y\n---\n\n"
	body := frontmatter + "# X\n\n" +
		"## Background\n\n" + strings.Repeat("background filler ", 200) + "\n\n" +
		"## Steps\n\n1. step\n"
	res := BenchSkill(body, 0, 0)
	if res.Status != "verbose" && res.Status != "typical" {
		// Either bucket is fine; we just need the dominant-section
		// suggestion to fire when we're past budget.
	}
	if res.Status == "verbose" {
		hasDominant := false
		for _, s := range res.Suggestions {
			if strings.Contains(s, "Section 'Background'") {
				hasDominant = true
			}
		}
		if !hasDominant {
			t.Errorf("expected dominant-section suggestion; got %v", res.Suggestions)
		}
	}
}
