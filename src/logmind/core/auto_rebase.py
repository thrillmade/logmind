"""v0.5.14 — deterministic auto-rebase when the gap is timeline.md only.

User-requested feature (2026-05-30): _"auto rebase must must be very
deterministic and safe, only the timeline md file"_.

Saves ~5-8 agent turns per DIRTY incident (Phase D tokenomics
scenario): when a PR batch lands out-of-order, the trailing branch
goes DIRTY because main's timeline.md has new entries. The recovery
is mechanical (rebase + regen timeline.md + force-with-lease push)
and entirely contained in a derived doc — exactly the kind of action
to automate behind a strong opt-in flag.

**Hard safety guards** (all must hold or auto-rebase REFUSES to act):

1. ``auto_rebase: true`` is explicit (opt-in; default OFF)
2. Branch is NOT the default branch (no rebasing main onto main)
3. Branch is behind ``origin/<default>`` (otherwise nothing to do)
4. The gap touches **exactly** ``docs/timeline.md`` and nothing else —
   not even ``docs/file-structure.md``, not any code, not any other
   derived doc. Single-file scope is the safety lever.
5. ``--force-with-lease`` (never ``--force``) so a concurrent push
   aborts safely
6. Any unexpected rebase conflict triggers ``git rebase --abort`` +
   bail out (no half-applied state)

Returns a small report so the caller can emit user-visible logging
about what happened (or didn't).
"""

from __future__ import annotations

import subprocess
from dataclasses import dataclass
from pathlib import Path
from typing import Optional


# The single file the gap is allowed to touch. Anything else aborts.
_ALLOWED_GAP_FILE = "docs/timeline.md"


@dataclass
class AutoRebaseResult:
    """Outcome of an auto-rebase attempt. Always non-None even when no-op."""

    fired: bool                # True iff the rebase + push actually ran
    reason: str                # human-readable explanation
    commits_behind: int = 0    # how many commits the branch was behind
    pushed: bool = False       # True iff the force-with-lease push succeeded


def maybe_auto_rebase(
    repo_root: Path,
    branch: str,
    default_branch_name: str,
    *,
    enabled: bool,
) -> AutoRebaseResult:
    """Attempt a deterministic timeline-md-only auto-rebase.

    Returns an :class:`AutoRebaseResult` describing what happened.
    Never raises — every failure mode returns a fired=False result
    with a reason string the caller can surface as a warning.

    Caller is responsible for:
      - deciding whether to print the result (most cases the reason
        is only interesting when ``fired=True``)
      - any post-rebase regeneration (the function handles the
        rebase-time regen for timeline.md but the broader logmind log
        flow continues from there)
    """
    if not enabled:
        return AutoRebaseResult(fired=False, reason="auto_rebase disabled in config")

    if branch == default_branch_name:
        return AutoRebaseResult(
            fired=False, reason=f"on default branch '{branch}' — nothing to rebase onto"
        )

    upstream = f"origin/{default_branch_name}"

    # Fetch — best-effort, abort on failure (offline / no remote).
    try:
        subprocess.run(
            ["git", "-C", str(repo_root), "fetch", "origin", default_branch_name],
            capture_output=True,
            check=True,
        )
    except (subprocess.CalledProcessError, FileNotFoundError, OSError) as e:
        return AutoRebaseResult(fired=False, reason=f"fetch failed: {e}")

    # Verify the upstream ref now exists. If not, abort cleanly.
    try:
        subprocess.run(
            ["git", "-C", str(repo_root), "rev-parse", "--verify", "--quiet", upstream],
            capture_output=True,
            check=True,
        )
    except (subprocess.CalledProcessError, FileNotFoundError, OSError):
        return AutoRebaseResult(fired=False, reason=f"no upstream ref {upstream}")

    # How far behind?
    try:
        behind = int(subprocess.run(
            ["git", "-C", str(repo_root), "rev-list", "--count",
             f"{branch}..{upstream}"],
            capture_output=True,
            text=True,
            check=True,
        ).stdout.strip() or "0")
    except (subprocess.CalledProcessError, FileNotFoundError, OSError, ValueError):
        return AutoRebaseResult(fired=False, reason="unable to count commits behind")

    if behind == 0:
        return AutoRebaseResult(
            fired=False, reason="branch up-to-date with upstream", commits_behind=0
        )

    # SAFETY GATE: only allow auto-rebase when the gap touches EXACTLY
    # docs/timeline.md and nothing else. Any other file in the gap
    # means we don't know how to resolve safely, and we bail out so
    # the user/agent does the manual rebase.
    try:
        files_in_gap_raw = subprocess.run(
            ["git", "-C", str(repo_root), "log",
             f"{branch}..{upstream}", "--name-only", "--format="],
            capture_output=True,
            text=True,
            check=True,
        ).stdout
    except (subprocess.CalledProcessError, FileNotFoundError, OSError):
        return AutoRebaseResult(
            fired=False, reason="unable to list files in upstream gap",
            commits_behind=behind,
        )

    files_in_gap = {line for line in files_in_gap_raw.splitlines() if line.strip()}
    if files_in_gap != {_ALLOWED_GAP_FILE}:
        # The deterministic-safety gate. We do NOT extend this to
        # file-structure.md or any other derived doc in v0.5.14.
        # Narrow scope = trusted scope.
        other_files = sorted(files_in_gap - {_ALLOWED_GAP_FILE})
        return AutoRebaseResult(
            fired=False,
            reason=(
                f"upstream gap touches files beyond {_ALLOWED_GAP_FILE}: "
                f"{', '.join(other_files) if other_files else '(empty diff?)'}"
            ),
            commits_behind=behind,
        )

    # OK, safe to rebase. Run it.
    rebase = subprocess.run(
        ["git", "-C", str(repo_root), "rebase", upstream],
        capture_output=True,
        text=True,
        check=False,
    )

    if rebase.returncode != 0:
        # Did rebase produce conflicts? If yes, check they're ONLY in
        # docs/timeline.md before we attempt regen-and-continue.
        conflicted = subprocess.run(
            ["git", "-C", str(repo_root), "diff", "--name-only", "--diff-filter=U"],
            capture_output=True,
            text=True,
            check=False,
        ).stdout.splitlines()
        conflicted_set = {line for line in conflicted if line.strip()}

        if conflicted_set != {_ALLOWED_GAP_FILE}:
            # Unexpected conflict surface — abort safely.
            subprocess.run(
                ["git", "-C", str(repo_root), "rebase", "--abort"],
                capture_output=True,
                check=False,
            )
            return AutoRebaseResult(
                fired=False,
                reason=(
                    f"rebase produced unexpected conflicts in {sorted(conflicted_set)}; "
                    f"aborted safely"
                ),
                commits_behind=behind,
            )

        # Conflict is exactly timeline.md. Regenerate from sources +
        # continue. Loop on conflict per commit since multi-commit
        # branches may hit timeline.md conflicts on EACH replay step.
        max_iterations = 50  # paranoid cap; real branches have <10 commits
        for _ in range(max_iterations):
            # Regenerate timeline.md from current decision files.
            regen = subprocess.run(
                ["logmind", "timeline", "--write", "docs/timeline.md"],
                cwd=repo_root,
                capture_output=True,
                check=False,
            )
            if regen.returncode != 0:
                subprocess.run(
                    ["git", "-C", str(repo_root), "rebase", "--abort"],
                    capture_output=True,
                    check=False,
                )
                return AutoRebaseResult(
                    fired=False,
                    reason="timeline regen failed mid-rebase; aborted safely",
                    commits_behind=behind,
                )

            subprocess.run(
                ["git", "-C", str(repo_root), "add", "docs/timeline.md"],
                capture_output=True,
                check=False,
            )

            cont = subprocess.run(
                ["git", "-C", str(repo_root), "rebase", "--continue"],
                capture_output=True,
                text=True,
                check=False,
                env={"GIT_EDITOR": "true"},  # don't open editor for continue
            )
            if cont.returncode == 0:
                break  # rebase complete

            # Still conflicted? Verify it's still timeline.md only.
            still_conflicted = set(subprocess.run(
                ["git", "-C", str(repo_root), "diff", "--name-only", "--diff-filter=U"],
                capture_output=True,
                text=True,
                check=False,
            ).stdout.splitlines())
            still_conflicted.discard("")
            if still_conflicted != {_ALLOWED_GAP_FILE}:
                subprocess.run(
                    ["git", "-C", str(repo_root), "rebase", "--abort"],
                    capture_output=True,
                    check=False,
                )
                return AutoRebaseResult(
                    fired=False,
                    reason=(
                        f"rebase continuation surfaced unexpected conflicts in "
                        f"{sorted(still_conflicted)}; aborted safely"
                    ),
                    commits_behind=behind,
                )
        else:
            # Hit the iteration cap (shouldn't happen on real branches).
            subprocess.run(
                ["git", "-C", str(repo_root), "rebase", "--abort"],
                capture_output=True,
                check=False,
            )
            return AutoRebaseResult(
                fired=False,
                reason=f"rebase exceeded {max_iterations} iterations; aborted",
                commits_behind=behind,
            )

    # Rebase complete. Now force-with-lease push. Failure here means
    # someone else pushed between our fetch + push — abort safely
    # (the user/agent should retry rather than us looping).
    push = subprocess.run(
        ["git", "-C", str(repo_root), "push", "--force-with-lease",
         "origin", branch],
        capture_output=True,
        text=True,
        check=False,
    )
    if push.returncode != 0:
        return AutoRebaseResult(
            fired=False,
            reason=(
                f"rebase succeeded locally but force-with-lease push failed "
                f"(concurrent push?): {push.stderr.strip()}"
            ),
            commits_behind=behind,
        )

    return AutoRebaseResult(
        fired=True,
        reason=f"rebased onto {upstream} (gap was {_ALLOWED_GAP_FILE} only)",
        commits_behind=behind,
        pushed=True,
    )
