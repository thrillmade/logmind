package decisions

import (
	"os"
	"path"
	"path/filepath"
	"strings"
)

// Layout owns WHERE a repository's decision record lives for the two sides
// that must never disagree about it — because when they do, the artefact is
// a decision logmind wrote that logmind's own gate refuses:
//
//   - `logmind log` BUILDS a path from it (resolveDecisionsPath,
//     internal/cli/log.go, which owns the routing RULE — which file, given
//     the branch and the config), and `logmind init` scaffolds into it;
//   - the commit gate TESTS a staged path against it (IsDecisionRel, called
//     by guardcommit.DecisionRecorded for both the commit-msg hook and the
//     §6.2 workflow).
//
// SCOPE, stated because "one owner" invites a wider reading than is true:
// the READ paths (Collect, ListSources, `timeline`, `search`, `context`,
// `pulse`) still join `filepath.Join(cwd, "docs")` themselves. That is not a
// second answer today — on a case-folding volume the literal join opens the
// same directory, and a nested project is read from its own cwd — but it is
// not routed here either. See the note above the layout constants in
// decisions.go for why that sweep is a separate change.
//
// A SHARED CONSTANT WAS NOT ENOUGH, and the gap was measured. The three
// names below have always been constants, but the gate consumed all three
// and the writer only two: it spelled the docs directory itself, as
// `filepath.Join(cwd, "docs")`. So the half neither shared was the half
// that broke, in both directions the join can be wrong:
//
//   - AGAINST WHAT ROOT. `cwd`, not the repository root git reports staged
//     paths against. `logmind init` below the git root exits 0, and
//     `logmind log` run there writes pkg/api/docs/decisions.md, which the
//     gate did not recognise.
//   - SPELLED HOW. `docs` as a literal, on a filesystem that folds case. A
//     repository that already had a `Docs/` directory took the write into
//     it; git stages `Docs/decisions.md`; the gate compared against
//     `docs/decisions.md`.
//
// Both configurations wrote the decision to disk, reported success, and
// produced an index the gate exited 65 on.
//
// WHAT LAYOUT DOES ABOUT EACH, which is not symmetric. The spelling it
// resolves once, through the filesystem, and both sides use the answer. The
// root it does NOT relocate: `logmind log` still resolves from its working
// directory, because moving the writer up to the git root would refuse the
// nested project outright rather than fix it. What changes is that the GATE
// can now recognise a record hanging off a nested logmind project, and
// nothing else — see IsDecisionRel.
type Layout struct {
	// Root is the absolute directory the docs tree hangs off. For the
	// writer that is its working directory; for the gate it is the
	// repository root, because that is what git reports paths against.
	Root string

	// docsName is DocsDirName as the filesystem under Root actually spells
	// it — `Docs` where a case-folding volume already had one. Unexported:
	// a caller that could set it could set it to something the filesystem
	// does not answer to, which is the state this type exists to remove.
	docsName string

	// foldCase records that Root's filesystem answered to a spelling other
	// than the one on disk. It is a property of the volume, not a policy
	// choice: where the filesystem cannot tell `Docs` from `docs`, neither
	// side of the gate can either.
	foldCase bool
}

// ResolveLayout resolves the decision-record layout for root.
//
// Cheap and non-failing by design — it is on `logmind log`'s hot path and
// on the commit gate's. At most three stats and one directory read, and
// every failure degrades to the literal `docs`, which is what the two
// sides did unconditionally before.
func ResolveLayout(root string) Layout {
	l := Layout{Root: root, docsName: DocsDirName}
	st, err := os.Stat(filepath.Join(root, DocsDirName))
	if err != nil || !st.IsDir() {
		return l
	}
	// WHICH entry did the literal `docs` actually resolve to? Asked of the
	// filesystem rather than inferred from the platform: os.SameFile is the
	// only thing that can say "these two names are one directory", and the
	// answer differs per VOLUME (a case-sensitive APFS image mounted on a
	// case-folding macOS) rather than per GOOS.
	if entries, err := os.ReadDir(root); err == nil {
		for _, e := range entries {
			if e.Name() == DocsDirName || !strings.EqualFold(e.Name(), DocsDirName) {
				continue
			}
			info, err := os.Stat(filepath.Join(root, e.Name()))
			if err != nil || !os.SameFile(st, info) {
				continue
			}
			l.docsName = e.Name()
			l.foldCase = true
			return l
		}
	}
	// The on-disk spelling IS `docs`, which says nothing about the volume —
	// and it still matters, because git may report the INDEX's spelling
	// rather than the working tree's when the two differ in case only. Ask
	// directly.
	if up, err := os.Stat(filepath.Join(root, strings.ToUpper(DocsDirName))); err == nil {
		l.foldCase = os.SameFile(st, up)
	}
	return l
}

// docs returns the docs directory's on-disk name, tolerating a zero Layout
// so a caller that never resolved one still gets the documented default
// rather than an empty path component.
func (l Layout) docs() string {
	if l.docsName == "" {
		return DocsDirName
	}
	return l.docsName
}

// Dir is the absolute docs directory — what `logmind log` writes under and
// what `logmind init` scaffolded.
func (l Layout) Dir() string { return filepath.Join(l.Root, l.docs()) }

// LegacyFile is the absolute branchless decision log: the file the routing
// rule falls back to in exactly the states where it cannot resolve a branch
// name, and the pre-§3.2 main log a repository that predates the collapse
// still carries.
func (l Layout) LegacyFile() string { return filepath.Join(l.Dir(), LegacyFileName) }

// BranchFile is the absolute per-branch decision log for an
// already-sanitized branch name (sanitizeBranchName owns that transform;
// this owns the path it lands in).
func (l Layout) BranchFile(sanitized string) string {
	return filepath.Join(l.Dir(), BranchDirName, sanitized+".md")
}

// IsDecisionRel reports whether rel — a path relative to Root, spelled the
// way git reports a staged file (forward slashes on every platform) — is
// one `logmind log` writes a decision to. It is the IMAGE of LegacyFile and
// BranchFile above, which is why it lives beside them.
//
// Two shapes, and a prefix rule:
//
//   - <docs>/decisions.md — the branchless log.
//   - <docs>/decisions-branches/<name>.md — one branch's log. Any <name>,
//     because the gate cannot know which branch it is judging; but a SINGLE
//     path component ending `.md`, because ListBranchFiles skips
//     subdirectories and a file under one is invisible to every read path.
//   - the prefix in front of <docs> is empty (the repository root, where
//     SPEC §3.2 puts the record) or names a NESTED logmind project — a
//     directory `logmind init` has run in, which is the only other place
//     `logmind log` can write.
//
// SCOPED, not suffixed, and the difference was a live gate hole. The
// predicate this replaces accepted a `/decisions.md` suffix in ANY
// directory: measured on the release candidate, a well-formed §3.1 entry at
// internal/x/decisions.md plus 302 lines of new Go cleared `guard-commit
// --layer git-hook` (exit 0, "allowed (decision-recorded)") where the
// identical index without that file was refused (exit 65). Three lines in a
// file no read path enumerates cleared all three enforcement surfaces — a
// cheaper bypass than the content-free pointer this release exists to
// close, because the decoy does not even have to look like logmind's own
// file. It stays closed here: internal/x/decisions.md has no <docs>
// component, and internal/x/docs/decisions.md has one but no logmind
// project behind it.
//
// docs/decisions-archive.md is deliberately NOT a shape. Nothing writes it
// in any state (NonBranchSources) — the read paths surface it where it
// lies, and a gate that honours a path logmind will never write is the same
// approximation, one filename narrower.
func (l Layout) IsDecisionRel(rel string) bool {
	prefix, ok := l.trimRecordSuffix(rel)
	if !ok {
		return false
	}
	if prefix == "" {
		return true
	}
	return l.nestedProject(prefix)
}

// trimRecordSuffix splits rel into the project root it hangs off and
// whether the remainder is one of the two shapes the writer produces.
//
// The DOCS component is matched under the volume's case rules and the two
// logmind-owned names with it: where the filesystem cannot distinguish
// `Docs` from `docs`, `filepath.Join` lands in whichever exists and git
// reports whichever is recorded, so insisting on one spelling would refuse
// logmind's own file.
//
// The `.md` SUFFIX is matched exactly, foldCase or not, and the asymmetry
// is deliberate: ListBranchFiles filters entries on a literal `.md`, so a
// branch file named `x.MD` is one no read path enumerates. Folding here
// would re-open the hole this predicate closes, one extension wider.
func (l Layout) trimRecordSuffix(rel string) (prefix string, ok bool) {
	rel = path.Clean(rel)
	if rel == "" || rel == "." || path.IsAbs(rel) {
		return "", false
	}
	parts := strings.Split(rel, "/")
	for _, p := range parts {
		// git reports paths from the repository root and never emits these,
		// but a caller that hand-built one must not be able to walk out of
		// the repository into a `.logmind` somewhere else on the machine.
		if p == ".." {
			return "", false
		}
	}
	n := len(parts)
	if n >= 3 && l.eq(parts[n-3], l.docs()) && l.eq(parts[n-2], BranchDirName) &&
		len(parts[n-1]) > len(".md") && strings.HasSuffix(parts[n-1], ".md") {
		return strings.Join(parts[:n-3], "/"), true
	}
	if n >= 2 && l.eq(parts[n-2], l.docs()) && l.eq(parts[n-1], LegacyFileName) {
		return strings.Join(parts[:n-2], "/"), true
	}
	return "", false
}

// eq compares one path component under the volume's case rules.
func (l Layout) eq(got, want string) bool {
	if l.foldCase {
		return strings.EqualFold(got, want)
	}
	return got == want
}

// nestedProject reports whether prefix names a logmind project INSIDE this
// repository — a directory `logmind init` has run in, and so the only place
// other than the repository root where `logmind log` can write.
//
// `.logmind/config.yml` is the evidence because it is what init writes and
// what config.Load reads: a directory that has one is a project whose
// `logmind log` writes a real decision record, and a directory that does
// not is somewhere a decision-shaped file was merely placed. The repository
// root does NOT need it — SPEC §3.2 puts the record there by definition,
// and a repo whose docs/ predates init would otherwise have its own log
// refused.
func (l Layout) nestedProject(prefix string) bool {
	dir := filepath.Join(l.Root, filepath.FromSlash(prefix))
	// The path config.Load joins (internal/config, Load).
	st, err := os.Stat(filepath.Join(dir, ".logmind", "config.yml"))
	return err == nil && !st.IsDir()
}
