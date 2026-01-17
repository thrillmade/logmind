# Decision Log

This file contains the 20 most recent decisions. Older decisions are archived in [decisions-archive.md](decisions-archive.md).

---
## 2025-10-19 18:42 - The answer

---
## 2025-10-19 18:42 - Attempt risky operation

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
## 2025-10-19 20:28 - Add decorator documentation to README and update plan

**Reasoning:** Users need clear examples of how to use the new @log_decision and @log_choice decorators. README should showcase Phase 3 features.

**Implications:**
- README shows decorator usage examples
- Plan.md updated to show Phase 3 progress (33% complete)
- Clear path forward for remaining Phase 3 features

---
## 2025-10-19 20:46 - Prepare package for PyPI publishing

**Reasoning:** Package built and validated. Added LICENSE, MANIFEST.in, updated pyproject.toml with proper metadata, created CHANGELOG. Ready for upload to PyPI.

**Implications:**
- Built packages in dist/ folder (wheel and tar.gz)
- All twine checks pass
- Templates included correctly
- Package name: logmind v0.1.0

---
## 2025-10-20 07:07 - Add logmind-readme.md to docs folder for AI agents

**Reasoning:** AI agents need easy access to logmind documentation without navigating to repository root. Creates docs/logmind-readme.md during init by copying README.md content.

**Implications:**
- CLAUDE.md and other AI instruction files now link to docs/logmind-readme.md
- AI agents can read complete logmind instructions directly from docs folder
- Template files updated: logmind-section.md and CLAUDE.md.template

---
## 2025-10-20 07:12 - Make decisions-archive.md optional reference, not required reading

**Reasoning:** Archive is a searchable reference for historical context, not something AI agents must read upfront. Only recent decisions (docs/decisions.md) are required.

**Implications:**
- decisions-archive.md moved to 'Additional Reference' section
- Reduces cognitive load for AI agents - focus on recent 20 decisions
- Template files updated: logmind-section.md and CLAUDE.md.template

---
## 2026-01-17 14:28 - Add Cline and OpenAI Codex (AGENTS.md) to supported agents

**Reasoning:** Cline is a major VS Code agent using .clinerules; AGENTS.md is an emerging universal standard supported by OpenAI Codex, Cursor, Windsurf, and others under the Linux Foundation

**Alternatives considered:** Only add Cline, Only add AGENTS.md, Wait for more adoption

**Implications:**
- Now support 11 AI agents total
- AGENTS.md provides cross-tool compatibility

---
## 2026-01-17 14:28 - Implement logmind agents CLI command group with list, add, remove subcommands

**Reasoning:** Provides visibility into configured agents, easy setup without manual file creation, and project-level agent control

**Alternatives considered:** Single 'agents' command that does everything, Separate commands like 'list-agents', 'add-agent'

**Implications:**
- Users can manage agents via CLI instead of manually creating files
- Follows Click subcommand pattern for extensibility

---
## 2026-01-17 14:28 - Add --agents and --all-agents flags to init command

**Reasoning:** Allows explicit control over which agent files are created during initialization, supports CI/CD scripted setup

**Alternatives considered:** Auto-detect and create all found agents, Interactive prompt to select agents

**Implications:**
- logmind init --agents claude,cursor creates only specified files
- logmind init --all-agents creates all 11 agent files

---
## 2026-01-17 14:29 - Rename claude-md-insertion.md to ai-agent-files.md

**Reasoning:** Original name was Claude-specific but we now support 11 AI agents; generic name better reflects scope

**Alternatives considered:** Keep original name, Name it agent-instruction-files.md

**Implications:**
- Updated all doc links in README.md and logmind-readme.md

---
## 2026-01-17 14:30 - Use AGENT_REGISTRY dict as single source of truth for agent metadata

**Reasoning:** Centralizes agent definitions (file path, display name, is_json) in one place; makes adding new agents trivial

**Alternatives considered:** Separate config files per agent, Hardcoded if/elif chains

**Implications:**
- Adding a new agent requires only one dict entry plus template function update
- All agent functions derive from registry automatically

---
## 2026-01-17 16:00 - Implement config-driven agent file sync

**Reasoning:** User wants to configure agents in config.yml and have files auto-created/updated on any logmind command

**Alternatives considered:** Only sync on init, Require explicit sync command

**Implications:**
- sync_agent_files_from_config() added to inserter.py
- Called from log, show, search, and agents list commands

---
## 2026-01-17 16:05 - Enable both Claude and Cursor as default agents

**Reasoning:** Most users will want Claude Code and Cursor configured by default - the two most popular AI coding assistants

**Alternatives considered:** Only Claude enabled by default, No agents enabled by default

**Implications:**
- config.py DEFAULT_CONFIG updated: cursor: True
- config.yml.template updated: cursor: true
- Init command now creates both CLAUDE.md and .cursorrules by default

---
## 2026-01-17 16:05 - Add logmind update command for self-upgrade

**Reasoning:** Users need easy way to upgrade logmind without remembering pip commands

**Alternatives considered:** Only document pip upgrade in README, Add version check on every command

**Implications:**
- Runs pip install --upgrade logmind
- Shows before/after version comparison

---
## 2026-01-17 16:05 - Create Homebrew tap structure for distribution

**Reasoning:** Homebrew is preferred installation method for macOS/Linux CLI tools

**Alternatives considered:** Only distribute via PyPI, Submit directly to homebrew-core

**Implications:**
- Created homebrew-logmind/ with Formula/logmind.rb
- Custom tap allows faster iteration before homebrew-core submission

---
## 2026-01-17 16:06 - Merge plan archive into plan.md Development History section

**Reasoning:** Consolidate documentation - archive had valuable phase completion details that were missing from plan.md

**Alternatives considered:** Keep archive as separate file, Delete archive without merging

**Implications:**
- Added detailed Phase 1-3 checklists to plan.md
- Included test progression history (65 to 160+ tests)
- Deleted docs/plan-archive-2025-01.md

---
## 2026-01-17 16:28 - Update documentation to reflect Phase 3 completion

**Reasoning:** README.md and docs/logmind-readme.md were out of date - showed Phase 2 and missing agents/update commands

**Alternatives considered:** Leave docs as-is, Update only phase status

**Implications:**
- Users now see accurate feature status
- Quick Start examples match actual CLI commands

---
## 2026-01-17 18:23 - Add CLI tests for config commands

**Reasoning:** Config CLI commands (list, get, set) had no test coverage

**Alternatives considered:** Only test Python API, Skip CLI tests

**Implications:**
- 6 new tests cover list, get, set, error handling, nested keys, and type conversion

---
## 2026-01-17 18:24 - Add CLI tests for config commands

**Reasoning:** Config CLI commands (list, get, set) had no test coverage

**Alternatives considered:** Only test Python API, Skip CLI tests

**Implications:**
- 6 new tests cover list, get, set, error handling, nested keys, and type conversion

---
