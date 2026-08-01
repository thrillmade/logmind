<!-- Pointer, not a mirror. logmind's canonical forward-looking contract
     lives in the sibling protocol repo; context.spec_file is repo-relative
     by design (SPEC §1.5), so this committed pointer is the in-repo spec.

     Section numbers below are from the LIVE SPEC.md. SPEC 2.0 was a
     from-scratch rewrite and its numbering does NOT map to the predecessor,
     which is archived in the protocol repository (not this one) at
     thrillmade/protocol:docs/SPEC-0.7.2-archive.md — never reason from it. -->

# logmind — Spec

**Status:** Pointer

logmind's canonical, forward-looking contract is the SkDD protocol SPEC:
<https://github.com/thrillmade/protocol/blob/main/SPEC.md>

logmind implements **SPEC 2.0.0** (see `internal/version.SpecVersion`). The
authoritative statement of which parts is the binary's own declaration —
`logmind --version` prints the areas line required by §7.3:

```
logmind 2.0.0 (spec 2.0.0)
areas: orient, work, record, propagate, gates
```

In section terms that is §1 (orient — the instruction file, decision record,
derived views, cold-start payload), §2.7–2.8 (token discipline and truncation
markers), §3 (record — decisions, the history and the map, steering commits
through the log), §5.2's upward nomination path only, and the §6.2 checks
logmind's own templates install.

`review` and `versioning` are deliberately not claimed: logmind consumes a
review's output but performs none, and declaring a version is a §7.3
obligation rather than an implementation of §7.

See [docs/plan.md](plan.md) for architecture and the forward roadmap.
