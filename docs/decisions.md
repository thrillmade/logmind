# Decision Log

Decisions are not kept in this file. Since SPEC §3.2 every decision lands in
`docs/decisions-branches/<branch>.md` — one file per branch, the default
branch included (`main.md`).

This file stays because a logmind release older than v2.0 uses its presence as
the "already initialised" sentinel. Without it, an older binary reads this
repository as a fresh one and re-runs the whole scaffold, overwriting
`.logmind/config.yml` and the settings in it. It is not written to, and it
holds no decisions of its own.

Read [timeline.md](timeline.md) for the chronological overview, or
`docs/decisions-branches/` for the record itself.
