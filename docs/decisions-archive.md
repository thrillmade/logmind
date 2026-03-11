# Decision Archive

This file contains historical decisions that have been archived from [decisions.md](decisions.md).

Decisions are listed in reverse chronological order (newest first).

---
## 2025-10-20 07:12 - Make decisions-archive.md optional reference, not required reading

**Reasoning:** Archive is a searchable reference for historical context, not something AI agents must read upfront. Only recent decisions (docs/decisions.md) are required.

**Implications:**
- decisions-archive.md moved to 'Additional Reference' section
- Reduces cognitive load for AI agents - focus on recent 20 decisions
- Template files updated: logmind-section.md and CLAUDE.md.template

---

## 2025-10-20 07:07 - Add logmind-readme.md to docs folder for AI agents

**Reasoning:** AI agents need easy access to logmind documentation without navigating to repository root. Creates docs/logmind-readme.md during init by copying README.md content.

**Implications:**
- CLAUDE.md and other AI instruction files now link to docs/logmind-readme.md
- AI agents can read complete logmind instructions directly from docs folder
- Template files updated: logmind-section.md and CLAUDE.md.template

---

## 2025-10-19 20:46 - Prepare package for PyPI publishing

**Reasoning:** Package built and validated. Added LICENSE, MANIFEST.in, updated pyproject.toml with proper metadata, created CHANGELOG. Ready for upload to PyPI.

**Implications:**
- Built packages in dist/ folder (wheel and tar.gz)
- All twine checks pass
- Templates included correctly
- Package name: logmind v0.1.0

---

## 2025-10-19 20:28 - Add decorator documentation to README and update plan

**Reasoning:** Users need clear examples of how to use the new @log_decision and @log_choice decorators. README should showcase Phase 3 features.

**Implications:**
- README shows decorator usage examples
- Plan.md updated to show Phase 3 progress (33% complete)
- Clear path forward for remaining Phase 3 features

---

## 2025-10-19 20:26 - Implement Phase 3 decorators for automatic decision logging

**Reasoning:** Enable developers to log decisions automatically using @log_decision and @log_choice decorators. Supports template strings with function arguments, alternatives, implications, and all standard logmind features.

**Alternatives considered:** Manual logging only, Aspect-oriented programming approach

**Implications:**
- Users can decorate functions to auto-log decisions
- Template strings support {arg_name} placeholders
- @log_choice decorator for return-value-based decisions
- 15 comprehensive tests, 110 total tests passing

---

## 2025-10-19 18:42 - Attempt risky operation

---

## 2025-10-19 18:42 - The answer

---

## 2025-10-19 18:42 - Unknown choice: memcached

---

## 2025-10-19 18:42 - Use PostgreSQL

**Reasoning:** Selected postgres based on requirements

---

## 2025-10-19 18:42 - Use Redis for caching

---

## 2025-10-19 18:42 - Process item3

---

## 2025-10-19 18:42 - Process item2

---

## 2025-10-19 18:42 - Process item1

---

## 2025-10-19 18:42 - Enable dark_mode

**Implications:**
- Users will see new UI
- Backend load may increase

---

## 2025-10-19 18:42 - Deploy to AWS

**Alternatives considered:** AWS eu-west-1, GCP, Azure

---

## 2025-10-19 18:42 - Use FastAPI

**Reasoning:** Best for our use case

**Alternatives considered:** Django, Flask, Tornado

---

## 2025-10-19 18:42 - Cache backend: redis

---

## 2025-10-19 18:42 - Connect to db.example.com:3306

---

## 2025-10-19 18:42 - Use oauth authentication

**Reasoning:** Security for /api/data

---

## 2025-10-19 18:42 - Test decision

**Reasoning:** Test reasoning

---

## 2025-10-19 17:44 - Make logmind commit all changed files, not just docs

**Reasoning:** When logging a decision, users expect ALL their changes to be committed together with that decision - like a normal git workflow. Previously only docs/ files were committed.

**Alternatives considered:** Keep selective file commits

**Implications:**
- logmind now does 'git add .' before committing
- All changes committed together with decision
- More intuitive git workflow

---

## 2025-10-19 17:41 - Update README with pipx setup instructions and CLI examples

**Reasoning:** Contributors need clear setup instructions. README should show both Python API and CLI usage, and explain the pipx approach.

**Implications:**
- Clear onboarding for new contributors
- README shows complete feature set (log, show, search)
- Explains why pipx (CLI tool philosophy)

---

## 2025-10-19 17:41 - Use pipx for global logmind installation like standard CLI tools

**Reasoning:** logmind is a CLI tool like git/npm/docker - should be globally installed, not repo-local. Users expect to just type 'logmind' without prefixes or activation.

**Alternatives considered:** Repo-local venv with wrapper script, Makefile commands

**Implications:**
- Contributors run 'pipx install -e .' once after cloning
- Clean CLI experience: just type 'logmind'
- Works from any directory
- Need to document setup in README

---

## 2025-10-19 17:16 - Enhance CLAUDE.md templates with comprehensive CLI examples and search documentation

**Reasoning:** AI agents need clear, practical examples of both Python API and CLI commands. Previous templates only showed Python usage, but agents often work in shell contexts where CLI is more appropriate.

**Alternatives considered:** Keep only Python API examples, Create separate CLI-only documentation

**Implications:**
- All new logmind projects will get improved instructions with both API styles
- Agents will know about search, show, and log commands
- Better discoverability of logmind features for AI agents

---

## 2025-10-19 16:57 - Implement search command for decision logs

**Reasoning:** Users need to be able to search through past decisions to find relevant context quickly. Supports regex patterns, case-sensitive/insensitive search, and includes archived decisions by default.

**Alternatives considered:** grep-based approach, Full-text search engine

**Implications:**
- Search includes both decisions.md and decisions-archive.md
- Context lines shown around matches for better understanding
- CLI command: logmind search 'query'

---

## 2025-10-19 16:52 - Implement configuration system

**Reasoning:** Allow users to customize git behavior, commit messages, and archive settings without changing code

**Alternatives considered:** Command-line flags only, Environment variables

**Implications:**
- Users can disable auto-push if preferred
- Custom commit message templates supported
- Configurable max_recent_decisions threshold

---

## 2025-10-19 16:42 - Update CLAUDE.md instructions to be more urgent and directive

**Reasoning:** AI agents need clear requirements, not suggestions. Must emphasize that decision logging is mandatory.

**Implications:**
- AI agents will be more likely to actually log decisions
- Better compliance with decision tracking

---

## 2025-10-19 16:26 - Test Python API

**Reasoning:** Testing that import works

---

## 2025-10-19 16:26 - Use Click for CLI framework

**Reasoning:** Click provides excellent argument parsing and is widely used

**Alternatives considered:** argparse, Typer

**Implications:**
- Need to learn Click API

---

## 2025-10-19 16:25 - Initialize logmind decision tracking

**Reasoning:** Starting structured decision logging for this project to maintain clear documentation of architectural choices and provide context for AI agents.

**Alternatives considered:** Manual decision documentation, ADR (Architecture Decision Records)

**Implications:**
- All significant decisions should now be logged using `logmind.log()`
- AI agents will have access to decision history via docs/decisions.md
- Git history will serve as an audit trail for all decisions

---

