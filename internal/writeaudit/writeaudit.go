// Package writeaudit is a build-time guard against one specific,
// repeatedly-reintroduced vulnerability: the stdlib's creating/truncating
// file primitives FOLLOW SYMLINKS.
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
// internal/atomicio.WriteFile is the safe primitive for the FINAL path
// component: it writes to a temp sibling (unpredictable name, O_EXCL) and
// os.Renames it onto the destination NAME, and refuses outright when that
// name is a symlink. (It does not, and cannot cheaply, defend a symlinked
// ANCESTOR directory — see atomicio.RefuseSymlink, which says so.)
//
// Fixing the call sites once is not enough — the next raw call arrives next
// month and looks exactly like ordinary Go. So Scan finds every remaining
// raw call site by parsing the tree, and TestNoUnauthorizedRawWriteFile
// pins the result against Allowlist below.
//
// # WHY A SOURCE SCAN AND NOT A BEHAVIOURAL TEST
//
// A behavioural test cannot cover this class, and that was measured rather
// than assumed. PR #297 removed a second AGENTS.md refresher — a loop that
// took a path back out of the inserter API and wrote it raw. Restoring that
// loop and re-running the outcome assertions leaves them GREEN: by the time
// the loop runs, EnsureAgentsMD has already refreshed the block, so
// FindOutdatedMarkerBlocks reports nothing and the second writer is a no-op
// on that path. Being unreachable on the happy path is exactly why its
// wrong-argument write survived untested for so long.
//
// The same holds for the symlink class this package is named after: a raw
// write is only observable when someone has planted a link, which no ordinary
// test does. So the thing pinned here is the PRIMITIVE, not an outcome — the
// set of ways to spell a path is unbounded, the set of ways to open a file
// for creation is small and enumerable.
//
// # ONE GUARD, NOT TWO
//
// This package is the single owner of the raw-write check for the whole
// module. A second scanner elsewhere with its own allowlist is a duplicate to
// fold in here, not a second opinion: two lists that mean the same thing
// diverge, and the first time they disagree somebody silences whichever one
// is louder. Extend bannedPaths and Allowlist instead.
//
// # SCOPE — what this catches, and what it cannot
//
// CAUGHT: os.WriteFile, io/ioutil.WriteFile, os.Create, os.Truncate, and
// os.OpenFile carrying O_CREATE or O_TRUNC (or flags this cannot statically
// resolve). The package is resolved from the file's import declarations, not
// from the identifier being spelled "os", so an alias (`import w "os"` ->
// w.Create) and a dot import (`import . "os"` -> a bare Create) are both
// caught, and so is a call inside a file that is behind a build tag this
// platform does not compile. A banned function referenced WITHOUT being
// called — `var raw = os.WriteFile`, or `w := os.WriteFile` followed by
// `w(...)` — is caught too, at the reference itself: laundering the
// primitive through a variable does not change what it does, and the call
// through that variable (`raw(...)`, `w(...)`) resolves to nothing this scan
// recognises, so the ONE place the danger is still visible is where the
// banned selector is named.
//
// NOT CAUGHT, and no amount of added patterns would change it: once an
// *os.File exists, anything can write through it — tmpl.Execute(f, …),
// json.NewEncoder(f).Encode(…), io.Copy(f, …), f.Write(b) — and shelling out
// (exec.Command("sh", "-c", "> f")) leaves Go entirely. A determined author
// gets a raw write past this guard in one line, and that is fine, because
// this is NOT a sandbox and cannot be made into one.
//
// Its job is the accidental reintroduction: the one-line "just write the
// file" that looks like ordinary Go, written by somebody who has never read
// the CVE this package is named after. That is the failure mode that has
// actually happened here, repeatedly.
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

// Exception is one allowlisted file: exactly which raw call sites it may
// contain, and why. Reason is mandatory — an allowlist without reasons is
// just a list of bugs nobody has read.
//
// Sites identifies each permitted call as "<enclosing func>:<call>", e.g.
// "runInstallHook:os.WriteFile". It is a multiset: two permitted calls to
// os.WriteFile inside the same function are two identical entries.
//
// WHY IDENTITIES AND NOT A COUNT. A count says "this file may contain N raw
// writes" and nothing about which ones — so moving the single judged-keep
// call out of the branch that justifies it and into a general-purpose
// `func RawWriteFile(path string, b []byte)` keeps the count at 1, greens
// the guard, and hands the whole module a laundered raw write. Naming the
// enclosing function makes that relocation a failure: the ledger says the
// exception lives in runInstallHook, and it no longer does.
//
// WHY NOT LINE NUMBERS. They would be exact and they would also churn on
// every unrelated edit above the call, so the allowlist would need editing
// in PRs that have nothing to do with it — and a ledger people re-baseline
// out of habit is a ledger nobody reads. The function name is stable under
// ordinary editing and unstable under exactly the move that matters.
type Exception struct {
	Sites  []string
	Reason string
}

// Allowlist enumerates every non-test file permitted to call a raw
// creating/truncating primitive directly, keyed by slash-separated
// repo-relative path.
//
// Two kinds of entry live here and they are NOT equivalent:
//
//   - A JUDGED KEEP — the raw call is the correct behaviour at that site.
//     It stays until the behaviour changes.
//   - A LANE HANDOFF — the site is vulnerable and is being converted in a
//     different pull request that owns the file. It is debt, and the entry
//     must be deleted when that PR lands.
//
// Deleting a converted site's entry is not optional housekeeping: the test
// fails on a STALE entry too (a listed site that is no longer there), so the
// ledger cannot quietly rot into permission-to-be-unsafe.
var Allowlist = map[string]Exception{
	"internal/atomicio/atomicio.go": {
		Sites: []string{"createTemp:os.OpenFile"},
		Reason: "JUDGED KEEP, and the reason the rest of this list can exist. atomicio is " +
			"the safe primitive every other site routes through, so it is the one place " +
			"that must touch a raw create. The call is O_CREATE|O_EXCL against an " +
			"unguessable random name in the destination's own directory: O_EXCL fails on " +
			"a symlink rather than following it, so there is nothing to follow. It takes " +
			"a mode so open(2) applies the umask, which os.CreateTemp (hardcoded 0600 + a " +
			"later chmod) cannot do.",
	},
	"internal/cli/filelock_unix.go": {
		Sites: []string{"acquireRepoLock:os.OpenFile"},
		Reason: "JUDGED KEEP. A repo lock must be the SAME INODE for every process that " +
			"flocks it, so it cannot be written via temp+rename — atomicio replaces the " +
			"name and gives every caller a private inode, which would silently disable " +
			"the lock rather than fail. The symlink hole is closed at the call site " +
			"instead: atomicio.RefuseSymlink(lockPath) runs immediately before this " +
			"OpenFile, so a planted link is refused with the same error content writes " +
			"give. No O_TRUNC — an existing lock file's bytes are never touched.",
	},
	"internal/cli/install_hook.go": {
		Sites: []string{"runInstallHook:os.WriteFile"},
		Reason: "JUDGED KEEP. The single remaining call is the append-onto-an-existing-hook " +
			"branch, reached only after os.ReadFile SUCCEEDED — so the dangling-symlink " +
			"attack cannot arrive there (it needs ErrNotExist), and git never checks " +
			"anything out into .git/, so a hostile repo cannot plant the link either. " +
			"Write-through is the INTENT: symlinking (or hardlinking) .git/hooks/pre-commit " +
			"at a shared script is a deliberate, common setup, and atomicio's one rule " +
			"would refuse that write outright or sever the link. The fresh-install branch " +
			"in the same file — the one ErrNotExist actually reaches — IS routed through " +
			"atomicio. See the call site for the full argument.",
	},

	// ---- LANE HANDOFFS: delete each entry as its PR lands. ----
	"internal/cli/init.go": {
		Sites: []string{
			"installWorkflowTemplates:os.WriteFile",
			"installWorkflowTemplates:os.WriteFile",
			"ensureGitignoreBlock:os.WriteFile",
			"logFirstDecision:os.WriteFile",
		},
		Reason: "LANE HANDOFF to PR #306, which owns internal/cli/init.go. Vulnerable; not converted here to avoid an edit collision.",
	},
	"internal/cli/self_update.go": {
		Sites:  []string{"runSelfUpdate:os.WriteFile"},
		Reason: "LANE HANDOFF to PR #306, which owns internal/cli/self_update.go. Vulnerable; not converted here to avoid an edit collision.",
	},
	"internal/gitattr/gitattr.go": {
		Sites:  []string{"ensureBlockWithLines:os.WriteFile", "RemoveBlock:os.WriteFile"},
		Reason: "LANE HANDOFF to PR #301, which owns internal/gitattr/gitattr.go. Vulnerable; not converted here to avoid an edit collision.",
	},
	"internal/inserter/inserter.go": {
		Sites: []string{
			"CreateAgentFile:os.WriteFile",
			"EnsureAgentsMD:os.WriteFile",
			"EnsureAgentsMD:os.WriteFile",
			"MigrateToAgentsMD:os.WriteFile",
			"MigrateToAgentsMD:os.WriteFile",
		},
		Reason: "LANE HANDOFF to PR #306, which is routing exactly these five sites through atomicio right now.",
	},
	"internal/inserter/dependabot.go": {
		Sites: []string{"EnsureDependabot:os.WriteFile", "EnsureDependabot:os.WriteFile"},
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
	Func string // enclosing function ("Type.Method" for methods, "<file>" at file scope)
	Call string // canonical, import-path-resolved, e.g. "os.WriteFile"
	Why  string // short note for the failure message, e.g. "O_CREATE"
}

// Site is the allowlist identity for this finding: "<func>:<call>".
func (f Finding) Site() string { return f.Func + ":" + f.Call }

// bannedPaths maps an IMPORT PATH to the function names within it that
// create or truncate through a symlink. Keyed by path, not by identifier, so
// the check survives `import w "os"` and `import . "os"`.
//
// ioutil.WriteFile is the deprecated alias for the same syscall sequence and
// is banned for the same reason. os.Create is O_RDWR|O_CREATE|O_TRUNC by
// definition. os.Truncate does not create, but it resolves the path and
// destroys the content at the far end of a link, which is the same class of
// damage. os.OpenFile is conditional — see openFileVerdict.
var bannedPaths = map[string]map[string]bool{
	"os":        {"WriteFile": true, "Create": true, "OpenFile": true, "Truncate": true},
	"io/ioutil": {"WriteFile": true},
}

// Scan parses every non-test .go file under root and reports each direct
// call to a banned primitive.
//
// It parses rather than greps on purpose. This very file, internal/atomicio,
// and a dozen call-site comments all mention "os.WriteFile" in prose to
// explain why it is dangerous; a regex would flag all of them and the guard
// would be turned off within a week. The AST walk sees calls only.
//
// Build tags are ignored: files are parsed unconditionally, so a raw call
// hidden behind `//go:build windows` is still reported on a mac.
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

		rel, rerr := filepath.Rel(root, path)
		if rerr != nil {
			return rerr
		}
		relSlash := filepath.ToSlash(rel)

		named, dotted := resolveImports(file)

		// Walk each declaration separately so the enclosing function name is
		// known without threading a stack through ast.Inspect.
		for _, decl := range file.Decls {
			fnName := "<file>"
			var body ast.Node = decl
			if fd, ok := decl.(*ast.FuncDecl); ok {
				fnName = funcName(fd)
				if fd.Body == nil {
					continue
				}
				body = fd.Body
			}

			// A manual parent stack, not plain ast.Inspect: the escape check
			// below (a banned selector used somewhere OTHER than a call's
			// Fun) needs to know each node's immediate parent, and
			// ast.Inspect's f(nil)-on-exit signal is the documented way to
			// track that without a second pass. See resolveExpr's doc for
			// why a call and a bare reference need different treatment.
			var stack []ast.Node
			ast.Inspect(body, func(n ast.Node) bool {
				if n == nil {
					if len(stack) > 0 {
						stack = stack[:len(stack)-1]
					}
					return true
				}
				var parent ast.Node
				if len(stack) > 0 {
					parent = stack[len(stack)-1]
				}
				stack = append(stack, n)

				switch node := n.(type) {
				case *ast.CallExpr:
					pkgPath, fn, pos, ok := resolveExpr(node.Fun, named, dotted)
					if !ok || !bannedPaths[pkgPath][fn] {
						return true
					}
					why := "creates or truncates through a symlink"
					if fn == "OpenFile" {
						banned, note := openFileVerdict(node)
						if !banned {
							return true
						}
						why = note
					}
					out = append(out, Finding{
						File: relSlash,
						Line: fset.Position(pos).Line,
						Func: fnName,
						Call: canonicalName(pkgPath) + "." + fn,
						Why:  why,
					})

				case *ast.SelectorExpr:
					// Already handled above when this selector IS the thing
					// being called (`os.WriteFile(...)`) — including the
					// os.OpenFile case where the verdict depends on the call's
					// flags argument, which only a *ast.CallExpr carries.
					if isCallFun(parent, node) {
						return true
					}
					pkgPath, fn, pos, ok := resolveExpr(node, named, dotted)
					if !ok || !bannedPaths[pkgPath][fn] {
						return true
					}
					out = append(out, Finding{
						File: relSlash,
						Line: fset.Position(pos).Line,
						Func: fnName,
						Call: canonicalName(pkgPath) + "." + fn,
						Why:  "referenced as a value, not called directly — the guard cannot see how or whether it will be invoked",
					})

				case *ast.Ident:
					// Two positions to exclude, both already resolved
					// elsewhere: the Sel half of a SelectorExpr just visited
					// above (re-checking the bare name "WriteFile" without
					// its qualifier would either duplicate that finding or,
					// worse, misfire on an unrelated dot-imported package
					// that happens to share the name), and a bare call.Fun
					// dot-import (`WriteFile(...)`), handled by the
					// *ast.CallExpr case.
					if isSelectorSel(parent, node) || isCallFun(parent, node) {
						return true
					}
					pkgPath, fn, pos, ok := resolveExpr(node, named, dotted)
					if !ok || !bannedPaths[pkgPath][fn] {
						return true
					}
					out = append(out, Finding{
						File: relSlash,
						Line: fset.Position(pos).Line,
						Func: fnName,
						Call: canonicalName(pkgPath) + "." + fn,
						Why:  "referenced as a value, not called directly — the guard cannot see how or whether it will be invoked",
					})
				}
				return true
			})
		}
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

// resolveExpr maps an expression back to (import path, function name) when
// it denotes one of the banned packages' functions, however it is spelled.
// It is applied both to a call's Fun (an ordinary call) and to any other
// expression position (a banned function referenced as a VALUE — see the
// HIGH-1 escape check in Scan) — the resolution rule is the same either way,
// only what the caller does with the answer differs.
//
// Two spellings reach the stdlib:
//
//	pkg.Fn       — an *ast.SelectorExpr whose X is an identifier bound to an
//	               import in THIS file. The binding is what matters, not the
//	               text: `import w "os"` makes w.Create an os.Create, and a
//	               local `var os fakeOS` in a file that does not import "os"
//	               is not one.
//	Fn           — a bare identifier, which resolves to the stdlib only when
//	               the file dot-imports the package.
func resolveExpr(expr ast.Expr, named map[string]string, dotted map[string]bool) (pkgPath, fn string, pos token.Pos, ok bool) {
	switch e := expr.(type) {
	case *ast.SelectorExpr:
		ident, isIdent := e.X.(*ast.Ident)
		if !isIdent {
			return "", "", 0, false
		}
		p, bound := named[ident.Name]
		if !bound {
			return "", "", 0, false
		}
		return p, e.Sel.Name, e.Sel.Pos(), true
	case *ast.Ident:
		// A dot import puts the package's exported names in file scope.
		// Attribute the reference to whichever dot-imported banned package
		// declares that name.
		for p := range dotted {
			if bannedPaths[p][e.Name] {
				return p, e.Name, e.Pos(), true
			}
		}
		return "", "", 0, false
	}
	return "", "", 0, false
}

// isCallFun reports whether n is exactly the callee of parent — i.e. n is
// being CALLED, not merely referenced. Used to keep the value-escape check
// in Scan from re-flagging (or double-flagging) an ordinary `os.WriteFile(...)`
// call, which the *ast.CallExpr case already handles with the extra
// os.OpenFile flag analysis that only a call carries.
func isCallFun(parent ast.Node, n ast.Expr) bool {
	call, ok := parent.(*ast.CallExpr)
	return ok && call.Fun == n
}

// isSelectorSel reports whether n is the Sel half of parent, e.g. the
// "WriteFile" identifier inside "os.WriteFile". Used to keep a dot-imported
// package name from colliding with an unrelated selector that merely shares
// the banned function's bare name (`someOtherThing.WriteFile`) — the
// *ast.SelectorExpr case already resolves that call correctly, by checking
// what X is actually bound to.
func isSelectorSel(parent ast.Node, n *ast.Ident) bool {
	sel, ok := parent.(*ast.SelectorExpr)
	return ok && sel.Sel == n
}

// openFileSafeFlags is the CLOSED set of flag-constant names that neither
// create nor truncate. It starts from the os package's own published
// constants (https://pkg.go.dev/os#pkg-constants) minus O_CREATE and
// O_TRUNC, which the switch in openFileVerdict below already bans by name
// before this set is even consulted, and adds two syscall/x/sys/unix flags
// that are not exported from os at all but are exactly the ones a
// symlink-safe open needs:
//
//   - O_NOFOLLOW refuses to follow a symlink at the final path component —
//     it IS the flag this package's own RefuseSymlink-style defense is
//     built around, at the syscall layer. Banning it because it is not
//     spelled "os.O_something" punished exactly the caller doing the most
//     defensive possible thing, and their only way out was an allowlist
//     entry that permanently exempts that file from every future edit —
//     training people around the guard instead of through it.
//   - O_CLOEXEC sets the close-on-exec bit on the returned descriptor. It is
//     a process/fd-table property, orthogonal to what ends up on disk: it
//     cannot create a file, cannot truncate one, and cannot make an
//     existing file's bytes visible or invisible to this program.
//
// Deliberately NOT added: O_TMPFILE. Despite the name-shape match with the
// rest of this set, O_TMPFILE creates an unnamed inode — it is a creating
// flag in every sense that matters here, just spelled differently than
// O_CREATE, and adding it would reopen exactly the hole this package exists
// to close.
//
// Membership is checked by exact name, matching how the banning half of
// this function already worked (openFileVerdict has never verified that an
// "os.O_CREATE" selector's X actually resolves to the os import — it bans
// on the name alone, which is deliberately over-inclusive). This whitelist
// makes the SAFE half symmetric with that: an identifier passes only when
// its name is a known non-creating flag, not merely when it happens to
// start with "O_". A look-alike — a local `const O_LOOKALIKE = 0`, an
// unrelated package's own O_-prefixed constant, a typo'd spelling — is not
// in this set and falls through to the unresolvable-is-banned rule.
var openFileSafeFlags = map[string]bool{
	"O_RDONLY":   true,
	"O_WRONLY":   true,
	"O_RDWR":     true,
	"O_APPEND":   true,
	"O_EXCL":     true,
	"O_SYNC":     true,
	"O_NOFOLLOW": true,
	"O_CLOEXEC":  true,
}

// openFileVerdict decides whether an os.OpenFile call creates or truncates.
//
// The flags argument is read syntactically: every identifier appearing in it
// is collected (os.O_CREATE, a dot-imported O_CREATE, syscall.O_CREAT, a
// local variable). O_CREATE/O_CREAT/O_TRUNC means banned. Every other
// identifier must match openFileSafeFlags BY NAME to pass — an identifier
// that does not (a look-alike "O_"-prefixed spelling included) means the
// flags cannot be verified safe, and this cannot tell — banned too,
// deliberately, because "the audit could not read it" must not be the quiet
// way past the audit.
func openFileVerdict(call *ast.CallExpr) (bool, string) {
	if len(call.Args) < 2 {
		return true, "unreadable os.OpenFile call"
	}
	var names []string
	ast.Inspect(call.Args[1], func(n ast.Node) bool {
		switch e := n.(type) {
		case *ast.SelectorExpr:
			names = append(names, e.Sel.Name)
			return false
		case *ast.Ident:
			names = append(names, e.Name)
		case *ast.BasicLit:
			// A bare literal 0 IS os.O_RDONLY — Go and POSIX both fix
			// O_RDONLY at 0 on every platform this ships to, so an
			// unadorned `0` flags argument is exactly as safe as spelling
			// out os.O_RDONLY and just as unable to create or truncate.
			// Accept it here — banning it would reproduce the same "guard
			// punishes the correct pattern" problem O_NOFOLLOW had, this
			// time against the plainest possible read-only open. Any
			// OTHER integer literal (a raw
			// platform-specific magic number) is deliberately left
			// unhandled: it falls through to the same names-is-empty /
			// unresolvable-is-banned path a look-alike identifier does,
			// because this scan cannot verify what an arbitrary number
			// means and "could not verify" must stay banned.
			if e.Kind == token.INT && e.Value == "0" {
				names = append(names, "O_RDONLY")
			}
			return false
		}
		return true
	})
	if len(names) == 0 {
		return true, "os.OpenFile flags could not be read statically"
	}
	for _, n := range names {
		switch n {
		case "O_CREATE", "O_CREAT":
			return true, "os.OpenFile with O_CREATE"
		case "O_TRUNC":
			return true, "os.OpenFile with O_TRUNC"
		}
	}
	for _, n := range names {
		if !openFileSafeFlags[n] {
			return true, fmt.Sprintf(
				"os.OpenFile flag %q is not a recognized non-creating flag and cannot be verified safe", n)
		}
	}
	return false, ""
}

// resolveImports returns (identifier -> import path) for banned packages
// imported under a name, and the set of banned packages imported with `.`.
// Blank imports (`_ "os"`) bind nothing and are skipped.
func resolveImports(file *ast.File) (named map[string]string, dotted map[string]bool) {
	named = map[string]string{}
	dotted = map[string]bool{}
	for _, imp := range file.Imports {
		p, err := strconv.Unquote(imp.Path.Value)
		if err != nil {
			continue
		}
		if _, banned := bannedPaths[p]; !banned {
			continue
		}
		local := p
		if i := strings.LastIndex(p, "/"); i >= 0 {
			local = p[i+1:]
		}
		if imp.Name != nil {
			switch imp.Name.Name {
			case ".":
				dotted[p] = true
				continue
			case "_":
				continue
			default:
				local = imp.Name.Name
			}
		}
		named[local] = p
	}
	return named, dotted
}

// canonicalName renders an import path the way a call site reads:
// "io/ioutil" -> "ioutil".
func canonicalName(pkgPath string) string {
	if i := strings.LastIndex(pkgPath, "/"); i >= 0 {
		return pkgPath[i+1:]
	}
	return pkgPath
}

// funcName renders a declaration's identity for the allowlist: "Fn" for a
// plain function, "Type.Method" for a method (pointer receivers included,
// without the star, because the star adds nothing here).
func funcName(fd *ast.FuncDecl) string {
	name := fd.Name.Name
	if fd.Recv == nil || len(fd.Recv.List) == 0 {
		return name
	}
	t := fd.Recv.List[0].Type
	if star, ok := t.(*ast.StarExpr); ok {
		t = star.X
	}
	if ident, ok := t.(*ast.Ident); ok {
		return ident.Name + "." + name
	}
	return name
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
