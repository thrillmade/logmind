"""logmind - AI decision logging system for development projects."""

__version__ = "0.6.7"

from logmind.core.logger import log
from logmind.decorators import log_choice, log_decision

__all__ = ["log", "log_decision", "log_choice"]
