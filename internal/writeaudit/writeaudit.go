// Package writeaudit is a build-time guard against one specific,
// repeatedly-reintroduced vulnerability: os.WriteFile FOLLOWS SYMLINKS.
//
// The exploitable shape is everywhere in a tool like logmind, which runs
// inside repositories its user did not write:
//
//	if _, err := os.Stat(p); errors.Is(err, fs.ErrNotExist) {
//	        os.WriteFile(p, body, 0o644)   // <- writes THROUGH a link
//	}
//
// A DANGLING symlink at p makes os.Stat (and os.ReadFile) report
// fs.ErrNotExist — they follow the link and find nothing at the far end.
// The code concludes "absent, safe to create" and the subsequent
// os.WriteFile opens the same link with O_CREAT|O_TRUNC, resolves it, and
// writes the payload wherever it points. Outside the repository. Possibly
// outside the user's home.
//
// internal/atomicio.WriteFile is the safe primitive: it writes to an
// os.CreateTemp sibling (unpredictable name, O_EXCL) and os.Renames it onto
// the destination NAME. Rename replaces the name itself rather than
// resolving it, so a symlink sitting on the destination is swapped out, not
// written through.
//
// Fixing the call sites once is not enough — the 24th raw call arrives next
// month and looks exactly like ordinary Go. So Scan finds every remaining
// raw call site by parsing the tree, and TestNoUnauthorizedRawWriteFile
// pins the result against Allowlist below. Adding a raw os.WriteFile to a
// non-test file turns that test red, and the only way to green it is to
// either route through atomicio or add an entry here with a written reason.
package writeaudit

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

// Exception is one allowlisted file: how many raw call sites it may
// contain, and why. Reason is mandatory — an allowlist without reasons is
// just a list of bugs nobody has read.
type Exception struct {
	Count  int
	Reason string
}

// Allowlist enumerates every non-test file permitted to call os.WriteFile
// directly, keyed by slash-separated repo-relative path.
//
// Two kinds of entry live here and they are NOT equivalent:
//
//   - A JUDGED KEEP — write-through is the correct behaviour at that site.
//     It stays until the behaviour changes.
//   - A LANE HANDOFF — the site is vulnerable and is being converted in a
//     different pull request that owns the file. It is debt, and the entry
//     must be deleted when that PR lands.
//
// Deleting a converted site's entry is not optional housekeeping: the test
// fails on a STALE entry too (allowed count higher than actual), so the
// ledger cannot quietly rot into permission-to-be-unsafe.
var Allowlist = map[string]Exception{
	"internal/cli/install_hook.go": {
		Count: 1,
		Reason: "JUDGED KEEP. The single remaining call is the append-onto-an-" +
			"existing-hook branch, reached only after os.ReadFile SUCCEEDED — so " +
			"the dangling-symlink attack cannot arrive there (it needs ErrNotExist). " +
			"Write-through is wanted: symlinking .git/hooks/pre-commit at a shared " +
			"script is a deliberate, common setup, and atomicio's rename would " +
			"silently detach it and force the mode to 0o755, breaking the " +
			"mode-preservation contract documented at that call site. The fresh-" +
			"install branch in the same file — the one ErrNotExist actually reaches " +
			"— IS routed through atomicio.",
	},

	// ---- LANE HANDOFFS: delete each entry as its PR lands. ----
	"internal/cli/init.go": {
		Count:  5,
		Reason: "LANE HANDOFF to PR #306, which owns internal/cli/init.go. Vulnerable; not converted here to avoid an edit collision.",
	},
	"internal/cli/self_update.go": {
		Count:  1,
		Reason: "LANE HANDOFF to PR #306, which owns internal/cli/self_update.go. Vulnerable; not converted here to avoid an edit collision.",
	},
	"internal/gitattr/gitattr.go": {
		Count:  2,
		Reason: "LANE HANDOFF to PR #301, which owns internal/gitattr/gitattr.go. Vulnerable; not converted here to avoid an edit collision.",
	},
	"internal/inserter/inserter.go": {
		Count:  5,
		Reason: "LANE HANDOFF to PR #306, which is routing exactly these five sites (283/367/402/812/827) through atomicio right now.",
	},
	"internal/inserter/dependabot.go": {
		Count: 2,
		Reason: "UNOWNED GAP, flagged rather than fixed. Lives in internal/inserter/, " +
			"a directory PR #306 holds, but is NOT among the five inserter.go sites " +
			"that PR enumerated — so no lane has claimed it. Both calls write " +
			".github/dependabot.yml and are vulnerable. Needs an owner.",
	},
}

// Finding is one raw call site.
type Finding struct {
	File string // slash-separated, repo-relative
	Line int
	Call string // e.g. "os.WriteFile"
}

// bannedSelectors maps package name -> function names that follow symlinks
// and must not be called directly. ioutil.WriteFile is the deprecated alias
// for the same syscall sequence and is banned for the same reason — it is
// the obvious way to slip past a ban on the os. spelling alone.
var bannedSelectors = map[string][]string{
	"os":     {"WriteFile"},
	"ioutil": {"WriteFile"},
}

// Scan parses every non-test .go file under root and reports each direct
// call to a banned write function.
//
// It parses rather than greps on purpose. This very file, internal/atomicio,
// and a dozen call-site comments all mention "os.WriteFile" in prose to
// explain why it is dangerous; a regex would flag all of them and the guard
// would be turned off within a week. The AST walk sees calls only.
func Scan(root string) ([]Finding, error) {
	var out []Finding
	fset := token.NewFileSet()

	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			switch d.Name() {
			case ".git", "vendor", "node_modules", "testdata":
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}

		file, perr := parser.ParseFile(fset, path, nil, parser.SkipObjectResolution)
		if perr != nil {
			return fmt.Errorf("parse %s: %w", path, perr)
		}

		// Only treat `os.X` as the standard library when "os" is really
		// imported under that name in THIS file — a local variable or an
		// aliased import named os would otherwise produce a false positive.
		live := importedNames(file)

		rel, rerr := filepath.Rel(root, path)
		if rerr != nil {
			return rerr
		}
		relSlash := filepath.ToSlash(rel)

		ast.Inspect(file, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			sel, ok := call.Fun.(*ast.SelectorExpr)
			if !ok {
				return true
			}
			pkg, ok := sel.X.(*ast.Ident)
			if !ok {
				return true
			}
			if !live[pkg.Name] {
				return true
			}
			for _, fn := range bannedSelectors[pkg.Name] {
				if sel.Sel.Name == fn {
					out = append(out, Finding{
						File: relSlash,
						Line: fset.Position(sel.Pos()).Line,
						Call: pkg.Name + "." + fn,
					})
				}
			}
			return true
		})
		return nil
	})
	if err != nil {
		return nil, err
	}

	sort.Slice(out, func(i, j int) bool {
		if out[i].File != out[j].File {
			return out[i].File < out[j].File
		}
		return out[i].Line < out[j].Line
	})
	return out, nil
}

// importedNames returns the set of banned package names that this file
// actually imports under their own name (unaliased, or aliased back to
// themselves).
func importedNames(file *ast.File) map[string]bool {
	live := map[string]bool{}
	for _, imp := range file.Imports {
		p, err := strconv.Unquote(imp.Path.Value)
		if err != nil {
			continue
		}
		name := p
		if i := strings.LastIndex(p, "/"); i >= 0 {
			name = p[i+1:]
		}
		if imp.Name != nil {
			name = imp.Name.Name
		}
		if _, banned := bannedSelectors[name]; banned {
			live[name] = true
		}
	}
	return live
}

// FindRepoRoot walks up from start until it finds the directory holding
// go.mod.
func FindRepoRoot(start string) (string, error) {
	dir, err := filepath.Abs(start)
	if err != nil {
		return "", err
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", fmt.Errorf("no go.mod found at or above %s", start)
		}
		dir = parent
	}
}
