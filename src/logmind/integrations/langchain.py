"""LangChain callback integration for logmind."""

from typing import Any, Dict, Optional

from logmind.integrations.base import BaseIntegration

_LANGCHAIN_AVAILABLE = False
try:
    from langchain_core.callbacks import BaseCallbackHandler

    _LANGCHAIN_AVAILABLE = True
except ImportError:
    try:
        from langchain.callbacks.base import BaseCallbackHandler  # type: ignore[no-redef]

        _LANGCHAIN_AVAILABLE = True
    except ImportError:

        class BaseCallbackHandler:  # type: ignore[no-redef]
            """Stub for when langchain is not installed."""

            pass


class LangChainLogger(BaseIntegration, BaseCallbackHandler):
    """LangChain callback handler that automatically logs agent decisions to logmind.

    Usage::

        from logmind.integrations import LangChainLogger

        chain = LLMChain(llm=llm, callbacks=[LangChainLogger()])

    Args:
        auto_commit: Whether to auto-commit logged decisions. Defaults to True.
        auto_push: Whether to auto-push after commit. Defaults to True.
        log_chain_end: Whether to log when chains complete. Defaults to False.
        log_tool_use: Whether to log individual tool uses. Defaults to False.
    """

    def __init__(
        self,
        auto_commit: bool = True,
        auto_push: bool = True,
        log_chain_end: bool = False,
        log_tool_use: bool = False,
    ):
        if not _LANGCHAIN_AVAILABLE:
            raise ImportError(
                "langchain is required for LangChainLogger. "
                "Install with: pip install logmind[langchain]"
            )
        BaseIntegration.__init__(self, auto_commit=auto_commit, auto_push=auto_push)
        self.log_chain_end = log_chain_end
        self.log_tool_use = log_tool_use

    def on_agent_finish(self, finish: Any, **kwargs: Any) -> None:
        """Log when an agent completes and produces a final answer."""
        output = finish.return_values.get("output", str(finish.return_values))
        summary = output[:150] + "..." if len(output) > 150 else output
        self.log(
            f"Agent decision: {summary}",
            reasoning="LangChain agent completed reasoning and produced output",
        )

    def on_chain_end(self, outputs: Dict[str, Any], **kwargs: Any) -> None:
        """Log when a chain completes (opt-in via log_chain_end=True)."""
        if not self.log_chain_end:
            return
        output_str = str(outputs)
        summary = output_str[:100] + "..." if len(output_str) > 100 else output_str
        self.log(f"Chain completed: {summary}")

    def on_tool_end(self, output: str, **kwargs: Any) -> None:
        """Log when a tool is used (opt-in via log_tool_use=True)."""
        if not self.log_tool_use:
            return
        summary = output[:100] + "..." if len(output) > 100 else output
        self.log(f"Tool result: {summary}")
