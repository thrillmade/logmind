← back to [docs/timeline.md](../timeline.md)

<!-- logmind-entry-start: 2026-08-15-inserter-make-a-block-body-where-a-whole-file-belongs-imposs -->
- **2026-08-15** — inserter: make a block body where a whole file belongs impossible to express, and collapse the two marker extractors to one
<!-- logmind-entry-end -->

## 2026-08-15 14:00 - inserter: make a block body where a whole file belongs impossible to express, and collapse the two marker extractors to one

**Reasoning:** Two issues turned out to be one class: a partial write landing on a whole artifact the user owns, against SPEC line 1101. Self-update passed a block body where ReplaceMarkerBlock's first parameter is the whole file, then wrote the result over all of AGENTS.md, destroying everything outside the markers. Doctor read the template marker from line one only while init scanned every line, so a marker displaced to line two was markerless to one and versioned to the other, and doctor --fix then overwrote the file it had misjudged.

**Alternatives considered:** Correct the argument at the call site and align the two regexes. Rejected: both defects existed because the shapes permitted them. ReplaceMarkerBlock is now unexported, since returning its input unchanged when markers are absent is a safe contract for a pure function and a data-loss contract for anything that writes the result. RefreshMarkerBlockFile owns the read, so no whole-file parameter is left to get wrong, and OutdatedMarkerEntry's OldBody field is deleted because a struct offering a path and a body side by side is what made the wrong call look plausible.

**Implications:**
- The self-update call site was deleted rather than repaired: FindOutdatedMarkerBlocks only ever reported AGENTS.md, which EnsureAgentsMD had already refreshed through the same classifier, making it a second refresher of one path against SPEC 1.1. Being unreachable is why its wrong argument survived untested. First-line-only extraction wins, because any-line makes ownership a substring search and a substring search over a user's file claims it; a workflow merely quoting the marker in a heredoc would be adopted and then overwritten. Displacement becomes its own reported state rather than collapsing into markerless. Four mutations, all compiled, all died.

---

## 2026-08-15 14:50 - refresh: name a remedy that works, and route every inserter write through the symlink-refusing primitive

**Reasoning:** The panel found the new refusal told the user to paste the bundled marker version as the file's first line, which is exactly the value installWorkflowTemplates treats as nothing to do, since it rewrites only on version inequality. The advice therefore guaranteed the file would never be refreshed again while doctor reported it current, and it was the only path the message offered. The working remedy, delete the file and re-run doctor --fix, was never mentioned. Verified end to end after the change: the file is left alone, the message names deletion, deleting and re-running regenerates the real gate body rather than the inert stub, and doctor then reports it current for a file that actually matches.

**Alternatives considered:** Make doctor verify content rather than marker version, so a file claiming the current version with arbitrary content cannot be believed. Rejected on the SPEC: section 5.2 defines drift purely as a marker that does not match the current version, and reserves content hashing for catalog-subscribed items through skills-lock.json, a different artifact class. Marker-only freshness is the deliberate take-this-file-back escape hatch, consistent across workflows, hooks and the AGENTS.md block, so the message was the whole bug.

**Implications:**
- Five raw writes in inserter now route through atomicio.WriteFile, which refuses to follow a symlink; the line numbers in the report I was handed were wrong, and re-deriving them by exhaustive search found exactly five rather than the five named. The underlying hazard was reproduced directly: a dangling symlink at a managed path makes os.ReadFile return not-exist, the caller concludes the file is absent, and a bare os.WriteFile then creates it wherever the link points. Eight mutations, all compiled, all died, including one that plants a second AGENTS.md writer in a different file under a different symbol, which the previous single-file substring test would have missed.

---

