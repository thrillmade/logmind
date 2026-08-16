# Building Custom Integrations

> **Historical — Python v0.6.x era (frozen).** The `BaseIntegration` API
> described below does not exist in the Go binary (v1.0+). It is kept for
> historical reference only; see [docs/plan.md](plan.md) for the current
> (Go) architecture.

This guide explains how to build a logmind integration for any AI framework.

## Overview

`BaseIntegration` in `logmind.integrations.base` provides the foundation. Subclass it, implement the framework-specific hooks, and call `self.log()` to record decisions.

## Quick Start

```python
from logmind.integrations.base import BaseIntegration

class MyFrameworkLogger(BaseIntegration):
    """Log decisions from MyFramework."""

    def on_decision(self, output: str) -> None:
        self.log(
            f"MyFramework chose: {output}",
            reasoning="Selected by MyFramework reasoning engine",
        )
```

Pass it into your framework's callback/hook system, and every decision is automatically logged to `docs/decisions-branches/<branch>.md` — the file for whatever branch you are on, the default branch included (SPEC §3.2).

## BaseIntegration API

```python
class BaseIntegration:
    def __init__(self, auto_commit: bool = True, auto_push: bool = True): ...

    def log(
        self,
        decision: str,
        reasoning: str | None = None,
        alternatives: list[str] | None = None,
        implications: list[str] | None = None,
    ) -> None: ...
```

| Parameter     | Type            | Description                                      |
|---------------|-----------------|--------------------------------------------------|
| `decision`    | `str`           | One-line summary of the decision (required)      |
| `reasoning`   | `str`           | Why this decision was made                       |
| `alternatives`| `list[str]`     | Other options that were considered               |
| `implications`| `list[str]`     | Downstream effects of this decision              |
| `auto_commit` | `bool`          | Commit to git after logging (default: `True`)    |
| `auto_push`   | `bool`          | Push after committing (default: `True`)          |

## Full Example: CrewAI Integration

```python
from logmind.integrations.base import BaseIntegration

class CrewAILogger(BaseIntegration):
    """Logs CrewAI agent task completions as decisions."""

    def on_task_end(self, task_name: str, output: str, agent_name: str) -> None:
        summary = output[:150] + "..." if len(output) > 150 else output
        self.log(
            f"Agent '{agent_name}' completed: {task_name}",
            reasoning=f"Output: {summary}",
            implications=[f"Task result passed to next agent or returned to user"],
        )

# Usage
logger = CrewAILogger(auto_commit=True, auto_push=False)

# Wire into CrewAI task callbacks
task.callback = lambda output: logger.on_task_end(
    task_name=task.description,
    output=str(output),
    agent_name=task.agent.role,
)
```

## Full Example: AutoGen Integration

```python
from logmind.integrations.base import BaseIntegration

class AutoGenLogger(BaseIntegration):
    """Logs AutoGen conversation decisions."""

    def on_message(self, sender: str, content: str) -> None:
        # Only log substantive decisions, not every message
        if len(content) < 50:
            return
        summary = content[:120] + "..." if len(content) > 120 else content
        self.log(
            f"{sender}: {summary}",
            reasoning="AutoGen agent message captured as decision log",
        )
```

## Pattern: Opt-in Logging

For frameworks with high message volume, use opt-in flags to avoid noise:

```python
class VerboseFrameworkLogger(BaseIntegration):
    def __init__(
        self,
        log_intermediate: bool = False,
        log_tool_use: bool = False,
        **kwargs,
    ):
        super().__init__(**kwargs)
        self.log_intermediate = log_intermediate
        self.log_tool_use = log_tool_use

    def on_intermediate_step(self, step: str) -> None:
        if self.log_intermediate:
            self.log(f"Intermediate: {step}")

    def on_tool_use(self, tool: str, result: str) -> None:
        if self.log_tool_use:
            self.log(f"Tool {tool}: {result[:100]}")
```

## Pattern: Context-Aware Logging

Include runtime context from the framework in your decisions:

```python
class ContextAwareLogger(BaseIntegration):
    def on_decision(self, decision: str, context: dict) -> None:
        self.log(
            decision,
            reasoning=context.get("reasoning", ""),
            alternatives=context.get("options_considered", []),
            implications=[f"Confidence: {context.get('confidence', 'unknown')}"],
        )
```

## Testing Your Integration

Use `unittest.mock.patch` to test without hitting git or the filesystem:

```python
from unittest.mock import patch
from my_integration import MyFrameworkLogger

def test_logs_on_decision():
    logger = MyFrameworkLogger(auto_commit=False, auto_push=False)

    with patch("logmind.integrations.base._log_decision") as mock_log:
        logger.on_decision("chose Redis over Memcached")

    mock_log.assert_called_once()
    args, kwargs = mock_log.call_args
    assert "Redis" in args[0]
```

## Publishing Your Integration

To share your integration with the community:

1. Create a package named `logmind-<framework>` (e.g., `logmind-crewai`)
2. Import from `logmind.integrations.base` — don't copy it
3. List `logmind` as a dependency in `pyproject.toml`
4. Open a PR or issue on [thrillmade/logmind](https://github.com/thrillmade/logmind) to link it

## Available Integrations

| Framework  | Package             | Import                                      |
|------------|---------------------|---------------------------------------------|
| LangChain  | `logmind[langchain]`| `from logmind.integrations import LangChainLogger` |
| Custom     | your package        | `from logmind.integrations.base import BaseIntegration` |
