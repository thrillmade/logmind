"""Tests for framework integrations."""

import pytest
from pathlib import Path
from unittest.mock import MagicMock, patch


# ---------------------------------------------------------------------------
# BaseIntegration tests
# ---------------------------------------------------------------------------


class TestBaseIntegration:
    def test_log_calls_core_logger(self, docs_dir):
        """BaseIntegration.log() delegates to logmind.core.logger.log."""
        from logmind.integrations.base import BaseIntegration

        integration = BaseIntegration(auto_commit=False, auto_push=False)

        with patch("logmind.integrations.base._log_decision") as mock_log:
            integration.log(
                "Test decision",
                reasoning="Test reasoning",
                alternatives=["A", "B"],
                implications=["Impact"],
            )

        mock_log.assert_called_once_with(
            "Test decision",
            reasoning="Test reasoning",
            alternatives=["A", "B"],
            implications=["Impact"],
            auto_commit=False,
            auto_push=False,
        )

    def test_default_auto_commit_push(self):
        """BaseIntegration defaults auto_commit and auto_push to True."""
        from logmind.integrations.base import BaseIntegration

        integration = BaseIntegration()
        assert integration.auto_commit is True
        assert integration.auto_push is True

    def test_custom_auto_commit_push(self):
        """BaseIntegration respects custom auto_commit/auto_push values."""
        from logmind.integrations.base import BaseIntegration

        integration = BaseIntegration(auto_commit=False, auto_push=False)
        assert integration.auto_commit is False
        assert integration.auto_push is False

    def test_log_passes_commit_settings(self):
        """log() passes auto_commit and auto_push to the core logger."""
        from logmind.integrations.base import BaseIntegration

        integration = BaseIntegration(auto_commit=True, auto_push=False)

        with patch("logmind.integrations.base._log_decision") as mock_log:
            integration.log("Decision")

        _, kwargs = mock_log.call_args
        assert kwargs["auto_commit"] is True
        assert kwargs["auto_push"] is False

    def test_log_optional_fields_default_none(self):
        """log() passes None for omitted optional fields."""
        from logmind.integrations.base import BaseIntegration

        integration = BaseIntegration(auto_commit=False, auto_push=False)

        with patch("logmind.integrations.base._log_decision") as mock_log:
            integration.log("Minimal decision")

        mock_log.assert_called_once_with(
            "Minimal decision",
            reasoning=None,
            alternatives=None,
            implications=None,
            auto_commit=False,
            auto_push=False,
        )

    def test_subclass_can_override(self):
        """BaseIntegration can be subclassed and log() inherited."""
        from logmind.integrations.base import BaseIntegration

        class MyIntegration(BaseIntegration):
            def on_event(self, text):
                self.log(f"Event: {text}", reasoning="Custom handler")

        integration = MyIntegration(auto_commit=False, auto_push=False)

        with patch("logmind.integrations.base._log_decision") as mock_log:
            integration.on_event("something happened")

        mock_log.assert_called_once()
        args, kwargs = mock_log.call_args
        assert args[0] == "Event: something happened"
        assert kwargs["reasoning"] == "Custom handler"


# ---------------------------------------------------------------------------
# Helpers for LangChainLogger tests
# ---------------------------------------------------------------------------


class _MockAgentFinish:
    def __init__(self, output):
        self.return_values = {"output": output}
        self.log = output


def _make_logger(**kwargs):
    """Instantiate LangChainLogger with langchain availability mocked to True."""
    with patch("logmind.integrations.langchain._LANGCHAIN_AVAILABLE", True):
        from logmind.integrations.langchain import LangChainLogger

        return LangChainLogger(**kwargs)


# ---------------------------------------------------------------------------
# LangChainLogger tests
# ---------------------------------------------------------------------------


class TestLangChainLogger:
    def test_raises_import_error_when_langchain_missing(self):
        """LangChainLogger raises ImportError if langchain is not installed."""
        with patch("logmind.integrations.langchain._LANGCHAIN_AVAILABLE", False):
            from logmind.integrations.langchain import LangChainLogger

            with pytest.raises(ImportError, match="langchain is required"):
                LangChainLogger()

    def test_import_error_message_mentions_install(self):
        """ImportError message tells user how to install langchain."""
        with patch("logmind.integrations.langchain._LANGCHAIN_AVAILABLE", False):
            from logmind.integrations.langchain import LangChainLogger

            with pytest.raises(ImportError, match="pip install logmind\\[langchain\\]"):
                LangChainLogger()

    def test_defaults(self):
        """LangChainLogger defaults: auto_commit=True, auto_push=True, no chain/tool logging."""
        logger = _make_logger()
        assert logger.auto_commit is True
        assert logger.auto_push is True
        assert logger.log_chain_end is False
        assert logger.log_tool_use is False

    def test_custom_settings(self):
        """LangChainLogger accepts custom settings."""
        logger = _make_logger(
            auto_commit=False, auto_push=False, log_chain_end=True, log_tool_use=True
        )
        assert logger.auto_commit is False
        assert logger.auto_push is False
        assert logger.log_chain_end is True
        assert logger.log_tool_use is True

    def test_on_agent_finish_logs_decision(self):
        """on_agent_finish logs the agent output as a decision."""
        logger = _make_logger(auto_commit=False, auto_push=False)
        finish = _MockAgentFinish("Use PostgreSQL for the database")

        with patch.object(logger, "log") as mock_log:
            logger.on_agent_finish(finish)

        mock_log.assert_called_once()
        args, kwargs = mock_log.call_args
        assert "Use PostgreSQL for the database" in args[0]
        assert "Agent decision:" in args[0]

    def test_on_agent_finish_includes_reasoning(self):
        """on_agent_finish includes reasoning about LangChain agent."""
        logger = _make_logger(auto_commit=False, auto_push=False)
        finish = _MockAgentFinish("Deploy to production")

        with patch.object(logger, "log") as mock_log:
            logger.on_agent_finish(finish)

        _, kwargs = mock_log.call_args
        assert kwargs.get("reasoning") is not None
        assert "LangChain" in kwargs["reasoning"]

    def test_on_agent_finish_truncates_long_output(self):
        """on_agent_finish truncates outputs longer than 150 chars."""
        logger = _make_logger(auto_commit=False, auto_push=False)
        long_output = "x" * 300
        finish = _MockAgentFinish(long_output)

        with patch.object(logger, "log") as mock_log:
            logger.on_agent_finish(finish)

        args, _ = mock_log.call_args
        assert "..." in args[0]
        assert len(args[0]) < 300

    def test_on_agent_finish_short_output_not_truncated(self):
        """on_agent_finish does not truncate short outputs."""
        logger = _make_logger(auto_commit=False, auto_push=False)
        finish = _MockAgentFinish("Short answer")

        with patch.object(logger, "log") as mock_log:
            logger.on_agent_finish(finish)

        args, _ = mock_log.call_args
        assert "..." not in args[0]

    def test_on_agent_finish_missing_output_key(self):
        """on_agent_finish handles return_values without 'output' key."""
        logger = _make_logger(auto_commit=False, auto_push=False)
        finish = MagicMock()
        finish.return_values = {"result": "some result"}

        with patch.object(logger, "log") as mock_log:
            logger.on_agent_finish(finish)

        mock_log.assert_called_once()

    def test_on_chain_end_skipped_by_default(self):
        """on_chain_end does not log when log_chain_end=False (default)."""
        logger = _make_logger(auto_commit=False, auto_push=False)

        with patch.object(logger, "log") as mock_log:
            logger.on_chain_end({"output": "chain result"})

        mock_log.assert_not_called()

    def test_on_chain_end_logs_when_enabled(self):
        """on_chain_end logs when log_chain_end=True."""
        logger = _make_logger(auto_commit=False, auto_push=False, log_chain_end=True)

        with patch.object(logger, "log") as mock_log:
            logger.on_chain_end({"output": "chain result"})

        mock_log.assert_called_once()
        args, _ = mock_log.call_args
        assert "Chain completed:" in args[0]

    def test_on_chain_end_truncates_long_output(self):
        """on_chain_end truncates outputs longer than 100 chars."""
        logger = _make_logger(auto_commit=False, auto_push=False, log_chain_end=True)
        long_outputs = {"output": "y" * 200}

        with patch.object(logger, "log") as mock_log:
            logger.on_chain_end(long_outputs)

        args, _ = mock_log.call_args
        assert "..." in args[0]

    def test_on_tool_end_skipped_by_default(self):
        """on_tool_end does not log when log_tool_use=False (default)."""
        logger = _make_logger(auto_commit=False, auto_push=False)

        with patch.object(logger, "log") as mock_log:
            logger.on_tool_end("tool output")

        mock_log.assert_not_called()

    def test_on_tool_end_logs_when_enabled(self):
        """on_tool_end logs when log_tool_use=True."""
        logger = _make_logger(auto_commit=False, auto_push=False, log_tool_use=True)

        with patch.object(logger, "log") as mock_log:
            logger.on_tool_end("tool output")

        mock_log.assert_called_once()
        args, _ = mock_log.call_args
        assert "Tool result:" in args[0]
        assert "tool output" in args[0]

    def test_on_tool_end_truncates_long_output(self):
        """on_tool_end truncates outputs longer than 100 chars."""
        logger = _make_logger(auto_commit=False, auto_push=False, log_tool_use=True)

        with patch.object(logger, "log") as mock_log:
            logger.on_tool_end("z" * 200)

        args, _ = mock_log.call_args
        assert "..." in args[0]

    def test_on_tool_end_short_output_not_truncated(self):
        """on_tool_end does not truncate short outputs."""
        logger = _make_logger(auto_commit=False, auto_push=False, log_tool_use=True)

        with patch.object(logger, "log") as mock_log:
            logger.on_tool_end("short result")

        args, _ = mock_log.call_args
        assert "..." not in args[0]
        assert "short result" in args[0]
