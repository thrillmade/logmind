"""Base class for logmind framework integrations."""

from typing import List, Optional

from logmind.core.logger import log as _log_decision


class BaseIntegration:
    """Base class for logmind framework integrations.

    Subclass this to create integrations with AI frameworks.
    Call self.log() to record decisions.

    Example::

        class MyFrameworkLogger(BaseIntegration):
            def on_decision(self, output):
                self.log(
                    f"Framework chose: {output}",
                    reasoning="Selected by MyFramework reasoning engine",
                )
    """

    def __init__(self, auto_commit: bool = True, auto_push: bool = True):
        self.auto_commit = auto_commit
        self.auto_push = auto_push

    def log(
        self,
        decision: str,
        reasoning: Optional[str] = None,
        alternatives: Optional[List[str]] = None,
        implications: Optional[List[str]] = None,
    ) -> None:
        """Log a decision through the logmind system."""
        _log_decision(
            decision,
            reasoning=reasoning,
            alternatives=alternatives,
            implications=implications,
            auto_commit=self.auto_commit,
            auto_push=self.auto_push,
        )
