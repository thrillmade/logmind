# Decision Log

This file contains the 20 most recent decisions. Older decisions are archived in [decisions-archive.md](decisions-archive.md).

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
## 2026-01-19 21:26 - Fix agents_remove to push after commit

**Reasoning:** Bug found by bugbot: agents_remove used raw subprocess for git add/commit but never pushed to remote, unlike agents_add which uses commit_and_push(push=True)

**Alternatives considered:** Keep local-only commits (rejected: inconsistent with agents_add behavior), Add separate git push subprocess call (rejected: commit_and_push already exists)

**Implications:**
- agents_remove now behaves consistently with agents_add
- All changes are pushed to remote automatically

---
## 2026-03-10 23:35 - Add LangChain callback integration and BaseIntegration pattern

**Reasoning:** First framework integration as specified in Phase 4 roadmap - enables zero-friction decision logging from LangChain agent runs

**Alternatives considered:** Require langchain as mandatory dependency, Use monkey-patching instead of inheritance

**Implications:**
- langchain is now an optional dependency: pip install logmind[langchain]
- Users subclass BaseIntegration to build custom integrations
- 22 new tests added

---
## 2026-03-10 23:40 - Add check-decisions and install-hook CLI commands

**Reasoning:** Implements git pre-commit hook support from roadmap — enforces decision logging at commit time

**Alternatives considered:** External hook script with no CLI support, Only a check command with no install helper

**Implications:**
- logmind check-decisions exits 1 when >20 lines staged without decisions.md update
- logmind install-hook creates or appends to .git/hooks/pre-commit
- 15 new tests added

---
## 2026-03-10 23:46 - Add decision templates with --template flag and logmind templates command

**Reasoning:** Roadmap item: pre-built templates for common patterns speed up logging and enforce consistent structure

**Alternatives considered:** Config-file-based templates, Interactive prompts

**Implications:**
- 7 built-in templates: database, api, architecture, security, performance, library, deployment
- logmind log --template <name> pre-fills reasoning, alternatives, implications; explicit flags override
- logmind templates lists all available templates
- 23 new tests added

---
## 2026-03-11 00:00 - Add analytics dashboard with logmind stats command

**Reasoning:** Roadmap item: visualize decision patterns, frequency, and trends without adding heavy dependencies

**Alternatives considered:** External visualization library (matplotlib, rich), Web dashboard

**Implications:**
- logmind stats shows total counts, monthly ASCII bar chart, velocity trend, and top keywords
- analytics.py parses decisions from both decisions.md and archive
- No new dependencies — pure Python with ASCII bar chart
- 37 new tests added

---
## 2026-03-11 00:14 - Add multi-project aggregation with logmind aggregate command

**Reasoning:** Roadmap item: view and search decisions across multiple repos in one place

**Alternatives considered:** Centralized database for cross-project storage, Separate aggregation service

**Implications:**
- logmind aggregate <path1> <path2> shows unified feed sorted newest-first
- logmind aggregate --summary shows per-project decision counts
- --limit, --no-archive flags control output scope
- 26 new tests added

---
