# First Decision Example

## What gets written to docs/decisions.md on init

After running `logmind init`, the `docs/decisions.md` file will contain:

```markdown
# Decision Log

This file contains the 20 most recent decisions. Older decisions are archived in [decisions-archive.md](decisions-archive.md).

---

## 2025-10-19 15:42 - Initialize logmind decision tracking

**Reasoning:** Starting structured decision logging for this project to maintain clear documentation of architectural choices and provide context for AI agents.

**Implications:**
- All significant decisions should now be logged using `logmind.log()`
- AI agents will have access to decision history via docs/decisions.md
- Git history will serve as an audit trail for all decisions

**Alternatives considered:** Manual decision documentation, ADR (Architecture Decision Records)

---

[Future decisions will be appended here]
```

## Why this matters

1. **Example for users:** Shows the format decisions should follow
2. **Context for AI:** Explains why logmind exists in this project
3. **Git history:** Creates first commit with proper logmind message format
4. **Timestamp:** Marks when decision tracking began
5. **Non-empty file:** Gives AI agents something to read immediately

## Template used

```python
from datetime import datetime

first_decision = {
    "decision": "Initialize logmind decision tracking",
    "reasoning": "Starting structured decision logging for this project to maintain clear documentation of architectural choices and provide context for AI agents.",
    "implications": [
        "All significant decisions should now be logged using `logmind.log()`",
        "AI agents will have access to decision history via docs/decisions.md",
        "Git history will serve as an audit trail for all decisions"
    ],
    "alternatives": ["Manual decision documentation", "ADR (Architecture Decision Records)"]
}
```

## Subsequent init calls

If `logmind init` is run again in a project that's already initialized:

```bash
$ logmind init

✓ docs/ already exists
✓ CLAUDE.md already has logmind instructions
✓ decisions.md already has entries

logmind is already initialized in this project.
```

It will **not** add another initialization decision.
