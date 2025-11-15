"""Decorators for automatic decision logging."""

import functools
from typing import Any, Callable, List, Optional, TypeVar, Union

from logmind.core.logger import log as _log_impl

F = TypeVar("F", bound=Callable[..., Any])


def log_decision(
    decision: str,
    reasoning: Optional[str] = None,
    alternatives: Optional[Union[List[str], str]] = None,
    implications: Optional[Union[List[str], str]] = None,
    auto_commit: Optional[bool] = None,
    auto_push: Optional[bool] = None,
    docs_path: Optional[Any] = None,
) -> Callable[[F], F]:
    """
    Decorator to automatically log a decision when a function is called.

    The decision string can contain placeholders for function arguments using
    curly braces, e.g., "Use {method} authentication" where 'method' is a
    function parameter.

    Args:
        decision: Decision summary (can include {arg_name} placeholders)
        reasoning: Why this decision was made (can include placeholders)
        alternatives: Other options considered (can include placeholders)
        implications: What this decision means (can include placeholders)
        auto_commit: Whether to auto-commit. If None, uses config value.
        auto_push: Whether to auto-push. If None, uses config value.

    Returns:
        Decorated function that logs the decision when called

    Example:
        @log_decision(
            decision="Authenticate user with {method}",
            reasoning="Security checkpoint for {endpoint}",
            alternatives=["Basic auth", "API key"],
            implications=["User session created"]
        )
        def authenticate(method="oauth", endpoint="/api/data"):
            # Your auth code
            return True

        # When called:
        authenticate(method="oauth", endpoint="/api/users")
        # Logs: "Authenticate user with oauth"
        # Reasoning: "Security checkpoint for /api/users"
    """

    def decorator(func: F) -> F:
        @functools.wraps(func)
        def wrapper(*args: Any, **kwargs: Any) -> Any:
            # Get function signature to map args to names
            import inspect

            sig = inspect.signature(func)
            bound_args = sig.bind(*args, **kwargs)
            bound_args.apply_defaults()
            arg_dict = bound_args.arguments

            # Format decision string with actual argument values
            formatted_decision = decision.format(**arg_dict)

            # Format optional fields if they contain placeholders
            formatted_reasoning = reasoning
            if reasoning and "{" in reasoning:
                formatted_reasoning = reasoning.format(**arg_dict)

            # Handle alternatives (can be string or list)
            formatted_alternatives = alternatives
            if isinstance(alternatives, str) and "{" in alternatives:
                formatted_alternatives = alternatives.format(**arg_dict)
            elif isinstance(alternatives, list):
                formatted_alternatives = [
                    alt.format(**arg_dict) if "{" in alt else alt for alt in alternatives
                ]

            # Handle implications (can be string or list)
            formatted_implications = implications
            if isinstance(implications, str) and "{" in implications:
                formatted_implications = implications.format(**arg_dict)
            elif isinstance(implications, list):
                formatted_implications = [
                    impl.format(**arg_dict) if "{" in impl else impl
                    for impl in implications
                ]

            # Log the decision
            _log_impl(
                decision=formatted_decision,
                reasoning=formatted_reasoning,
                alternatives=formatted_alternatives,
                implications=formatted_implications,
                auto_commit=auto_commit,
                auto_push=auto_push,
                docs_path=docs_path,
            )

            # Call the original function
            return func(*args, **kwargs)

        return wrapper  # type: ignore

    return decorator


def log_choice(
    choices: dict,
    reasoning: Optional[str] = None,
    auto_commit: Optional[bool] = None,
    auto_push: Optional[bool] = None,
    docs_path: Optional[Any] = None,
) -> Callable[[F], F]:
    """
    Decorator to log a decision based on function return value.

    Useful for functions that make choices and return the choice.
    The choices dict maps return values to decision descriptions.

    Args:
        choices: Dict mapping return values to decision descriptions
        reasoning: Why this decision was made (can include {return_value})
        auto_commit: Whether to auto-commit. If None, uses config value.
        auto_push: Whether to auto-push. If None, uses config value.

    Returns:
        Decorated function that logs based on return value

    Example:
        @log_choice(
            choices={
                "redis": "Use Redis for caching",
                "memcached": "Use Memcached for caching",
                "memory": "Use in-memory dict for caching",
            },
            reasoning="Selected based on deployment environment"
        )
        def select_cache_backend():
            if is_production():
                return "redis"
            return "memory"

        # When called:
        backend = select_cache_backend()
        # Logs: "Use Redis for caching" (if production)
        # OR "Use in-memory dict for caching" (if not production)
    """

    def decorator(func: F) -> F:
        @functools.wraps(func)
        def wrapper(*args: Any, **kwargs: Any) -> Any:
            # Call the original function
            result = func(*args, **kwargs)

            # Get decision based on return value
            decision = choices.get(result, f"Unknown choice: {result}")

            # Format reasoning if it includes placeholder
            formatted_reasoning = reasoning
            if reasoning and "{return_value}" in reasoning:
                formatted_reasoning = reasoning.format(return_value=result)

            # Log the decision
            _log_impl(
                decision=decision,
                reasoning=formatted_reasoning,
                auto_commit=auto_commit,
                auto_push=auto_push,
                docs_path=docs_path,
            )

            return result

        return wrapper  # type: ignore

    return decorator
