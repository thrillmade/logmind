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

