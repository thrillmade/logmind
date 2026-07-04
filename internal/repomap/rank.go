// rank.go — importance ranking + token-budget packing for the repomap
// (token-killer Phase 2, R3). When a budget is set, the map can't carry every
// file, so it must carry the MOST IMPORTANT ones first. Ranking is deterministic
// (the caching invariant) and native (no external dep, no network).
package repomap

import (
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/thrillmade/logmind/internal/tokens"
)

// Rank orders files by importance, most-important first, deterministically:
//
//  1. decision-linked files first — the file's path appears in the repo's
//     decision docs / timeline (a logmind-native signal: the team logged a
//     decision that names this file, so it is load-bearing);
//  2. then higher intra-repo fan-in — imported by more of the repo's own files
//     (the centrality signal at the heart of a repomap);
//  3. path ascending as the final, always-decisive tiebreak.
//
// The input slice is not mutated. (This is first-order in-degree centrality,
// not iterated PageRank — a strong, cheap, deterministic proxy; iteration is a
// later refinement.)
func Rank(repoRoot string, files []FileSymbols) []FileSymbols {
	linked := decisionLinkedPaths(repoRoot, files)
	fanIn := importFanIn(repoRoot, files)
	out := make([]FileSymbols, len(files))
	copy(out, files)
	sort.SliceStable(out, func(i, j int) bool {
		if li, lj := linked[out[i].Path], linked[out[j].Path]; li != lj {
			return li // decision-linked ranks above unlinked
		}
		if fi, fj := fanIn[out[i].Path], fanIn[out[j].Path]; fi != fj {
			return fi > fj // higher fan-in first
		}
		return out[i].Path < out[j].Path
	})
	return out
}

// Pack greedily keeps whole files from the (already Rank-ordered) slice while
// the rendered skeleton stays within maxTokens (the §14.2 ceil(len/4) estimate),
// and reports how many trailing files were dropped. maxTokens <= 0 means "no
// budget" — keep everything. Never-worse (§14.5): the packed render is a subset
// of the full render, never larger.
func Pack(ranked []FileSymbols, maxTokens int) (kept []FileSymbols, omitted int) {
	if maxTokens <= 0 {
		return ranked, 0
	}
	// Reserve headroom so the "... (N files omitted)" marker itself fits.
	const markerReserve = 20
	budget := maxTokens - markerReserve
	used := tokens.Estimate(repomapHeader)
	kept = make([]FileSymbols, 0, len(ranked))
	for i, f := range ranked {
		cost := tokens.Estimate(fileBlock(f))
		if used+cost > budget {
			return kept, len(ranked) - i
		}
		used += cost
		kept = append(kept, f)
	}
	return kept, 0
}

// decisionLinkedPaths returns the set of file paths (from files) that are named
// in the repo's decision docs or timeline — a logmind-native importance signal.
// Deterministic: reads committed docs in a fixed order.
func decisionLinkedPaths(repoRoot string, files []FileSymbols) map[string]bool {
	var corpus strings.Builder
	for _, p := range []string{"decisions.md", "decisions-archive.md", "timeline.md"} {
		if data, err := os.ReadFile(filepath.Join(repoRoot, "docs", p)); err == nil {
			corpus.Write(data)
			corpus.WriteByte('\n')
		}
	}
	branches, _ := filepath.Glob(filepath.Join(repoRoot, "docs", "decisions-branches", "*.md"))
	sort.Strings(branches)
	for _, p := range branches {
		if data, err := os.ReadFile(p); err == nil {
			corpus.Write(data)
			corpus.WriteByte('\n')
		}
	}
	text := corpus.String()
	linked := make(map[string]bool)
	for _, f := range files {
		if strings.Contains(text, f.Path) {
			linked[f.Path] = true
		}
	}
	return linked
}

// importFanIn counts, per file, how many OTHER files in the repo import that
// file's package — an intra-repo centrality (fan-in) score. All-zero when the
// module path can't be resolved (no go.mod). Deterministic.
func importFanIn(repoRoot string, files []FileSymbols) map[string]int {
	fanIn := make(map[string]int, len(files))
	mod := moduleImportPath(repoRoot)
	if mod == "" {
		return fanIn
	}
	// Each file's own package import path, derived from its directory.
	pkgOf := make(map[string]string, len(files))
	for _, f := range files {
		if dir := filepath.ToSlash(filepath.Dir(f.Path)); dir == "." {
			pkgOf[f.Path] = mod
		} else {
			pkgOf[f.Path] = mod + "/" + dir
		}
	}
	// Count importers per package path (deduped within a file), intra-repo only.
	importers := make(map[string]int)
	for _, f := range files {
		seen := make(map[string]bool)
		for _, imp := range f.Imports {
			if (imp == mod || strings.HasPrefix(imp, mod+"/")) && !seen[imp] {
				importers[imp]++
				seen[imp] = true
			}
		}
	}
	for _, f := range files {
		fanIn[f.Path] = importers[pkgOf[f.Path]]
	}
	return fanIn
}

// moduleImportPath reads the module path from go.mod (the `module X` line), or
// "" when there is no go.mod / no module directive.
func moduleImportPath(repoRoot string) string {
	data, err := os.ReadFile(filepath.Join(repoRoot, "go.mod"))
	if err != nil {
		return ""
	}
	for _, line := range strings.Split(string(data), "\n") {
		if line = strings.TrimSpace(line); strings.HasPrefix(line, "module ") {
			return strings.TrimSpace(strings.TrimPrefix(line, "module "))
		}
	}
	return ""
}
