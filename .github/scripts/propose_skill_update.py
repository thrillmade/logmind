"""Generate a proposed SKILL.md update from a logmind CHANGELOG section.

Called by `.github/workflows/notify-agent-skills.yml` on every logmind
tag push (v0.4.0+). Reads:

  - CHANGELOG section for the released version (from --changelog)
  - Current skills/logmind/SKILL.md content (from --current-skill)

Writes:

  - --out-reasoning : Claude's structured explanation of what it changed
    (or why it judged the release skill-irrelevant). Always written —
    the human reviewer needs this context even when no SKILL.md edit
    was proposed.

  - --out-skill : the proposed new SKILL.md content. ONLY written when
    Claude returns a `<skill_md>` block (i.e. the changelog had
    user-facing changes worth surfacing to AI agents). Skipped when
    Claude emits the NO_SKILL_UPDATE_NEEDED sentinel, in which case
    the workflow leaves the on-disk SKILL.md untouched and the human
    sees the stub TODO file alone.

Exit code 0 always (sentinel and proposal both count as "success");
exit code 1 only on infrastructure failure (missing API key, network
error after retries, malformed response). The workflow falls back to
the v0.3.x issue-shaped notification on exit code 1.
"""

from __future__ import annotations

import argparse
import os
import re
import sys
from pathlib import Path

# `anthropic` is pip-installed inside the workflow's Python step.
# Local pytest mocks the client, so import-time dependency is OK to
# expect at runtime but allowable to defer in test environments.
try:
    from anthropic import Anthropic
except ImportError:  # pragma: no cover - exercised in CI install
    Anthropic = None  # type: ignore[assignment]


SENTINEL = "NO_SKILL_UPDATE_NEEDED"
MODEL = "claude-sonnet-4-6"

SYSTEM_PROMPT = """\
You are a documentation engineer keeping a project's SKILL.md (a
prompt-format guidance file consumed by AI coding agents) in sync
with its CHANGELOG.

You receive a CHANGELOG section for a new release and the current
SKILL.md. Decide whether the release introduces *user-visible*
behavior or guidance that should be reflected in the SKILL.md:

  - New CLI commands, flags, or behaviors → mention in skill
  - New defaults that change agent-facing behavior → update Don'ts/Do's
  - Removed features or renamed primitives → strike or replace
  - New "always do X" / "never do Y" guidance from the release notes
    → add to the appropriate section

Skill-IRRELEVANT releases (do NOT propose an edit):

  - Internal refactors with no behavior change
  - CI / workflow changes that don't affect users
  - Test additions, doc-internal cleanup, dependency bumps
  - Bug fixes where the bug was invisible to agents using the skill

Strict output format:

  - If skill update is warranted, emit EXACTLY two XML-tagged blocks:
      <reasoning>1-3 sentences explaining what changed in the CHANGELOG
      that warrants the SKILL.md edit, citing specific user-visible
      behavior.</reasoning>
      <skill_md>...full updated SKILL.md content...</skill_md>

  - If no skill update is warranted, emit EXACTLY this line on its
    own (no other content):
      NO_SKILL_UPDATE_NEEDED
    Then on the next line, a <reasoning> block explaining why the
    release is skill-irrelevant.

Preserve the SKILL.md frontmatter (the leading `---\\n...\\n---` block)
EXACTLY as-is — name, description, review_mode, anything else. Edit
only the body content.

Do not include any prose outside the XML blocks / sentinel line.
"""


def _user_message(changelog_section: str, current_skill: str, version: str) -> str:
    return (
        f"Release version: {version}\n\n"
        f"=== CHANGELOG SECTION (verbatim) ===\n\n"
        f"{changelog_section}\n\n"
        f"=== CURRENT skills/logmind/SKILL.md (verbatim) ===\n\n"
        f"{current_skill}\n"
    )


_REASONING_RE = re.compile(r"<reasoning>(.*?)</reasoning>", re.DOTALL)
_SKILL_RE = re.compile(r"<skill_md>(.*?)</skill_md>", re.DOTALL)


def parse_response(text: str) -> tuple[str | None, str]:
    """Return (proposed_skill_or_None, reasoning).

    proposed_skill is None when the model returned the sentinel.
    reasoning is always a string (empty if the model didn't include
    one, which the workflow then surfaces as 'no reasoning provided').
    """
    reasoning_match = _REASONING_RE.search(text)
    reasoning = reasoning_match.group(1).strip() if reasoning_match else ""

    # Sentinel takes precedence — even if a stray <skill_md> block
    # leaked through, treat NO_SKILL_UPDATE_NEEDED on its own line as
    # the canonical signal.
    if re.search(r"^\s*" + SENTINEL + r"\s*$", text, re.MULTILINE):
        return None, reasoning

    skill_match = _SKILL_RE.search(text)
    if not skill_match:
        # Malformed response — no sentinel AND no skill block. Treat
        # as "no update" with a flag in the reasoning so the human
        # reviewer can spot it.
        return None, (
            reasoning + "\n\n[malformed response — no <skill_md> block and no "
            "NO_SKILL_UPDATE_NEEDED sentinel; treating as no-op]"
        ).strip()

    return skill_match.group(1).strip() + "\n", reasoning


def call_claude(changelog_section: str, current_skill: str, version: str) -> str:
    """Run one Anthropic API call. Returns the raw response text.
    Raises on infrastructure failure (no key, no client, no content)."""
    if Anthropic is None:
        raise RuntimeError("anthropic package not installed — pip install anthropic")
    if not os.environ.get("ANTHROPIC_API_KEY"):
        raise RuntimeError("ANTHROPIC_API_KEY not set")

    client = Anthropic()
    msg = client.messages.create(
        model=MODEL,
        max_tokens=8192,
        system=SYSTEM_PROMPT,
        messages=[{"role": "user", "content": _user_message(changelog_section, current_skill, version)}],
    )

    # Concatenate text blocks (Anthropic SDK returns a list of content
    # blocks; for non-streaming we expect just one TextBlock).
    parts: list[str] = []
    for block in msg.content:
        text = getattr(block, "text", None)
        if text:
            parts.append(text)
    if not parts:
        raise RuntimeError("empty response from Anthropic API")
    return "".join(parts)


def main(argv: list[str] | None = None) -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--changelog", type=Path, required=True,
                        help="path to the extracted CHANGELOG section")
    parser.add_argument("--current-skill", type=Path, required=True,
                        help="path to the current SKILL.md content")
    parser.add_argument("--version", required=True,
                        help="release version tag (e.g. v0.4.0)")
    parser.add_argument("--out-reasoning", type=Path, required=True,
                        help="output path for Claude's reasoning blob")
    parser.add_argument("--out-skill", type=Path, required=True,
                        help="output path for the proposed SKILL.md (only "
                             "written if Claude proposed an update)")
    args = parser.parse_args(argv)

    changelog_section = args.changelog.read_text(encoding="utf-8")
    current_skill = args.current_skill.read_text(encoding="utf-8")

    response = call_claude(changelog_section, current_skill, args.version)
    proposed, reasoning = parse_response(response)

    args.out_reasoning.parent.mkdir(parents=True, exist_ok=True)
    args.out_reasoning.write_text(
        reasoning or "(no reasoning provided)",
        encoding="utf-8",
    )

    if proposed is not None:
        args.out_skill.parent.mkdir(parents=True, exist_ok=True)
        args.out_skill.write_text(proposed, encoding="utf-8")
        print(f"::notice::Proposed SKILL.md update written ({len(proposed)} chars).")
    else:
        print("::notice::No skill update needed (sentinel returned). "
              "Workflow will leave SKILL.md untouched.")
    return 0


if __name__ == "__main__":
    sys.exit(main())
