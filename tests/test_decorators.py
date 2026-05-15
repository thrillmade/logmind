"""Tests for decorators module."""

from pathlib import Path

import pytest

from logmind.decorators import log_choice, log_decision


@pytest.fixture
def init_logmind(temp_dir, git_repo):
    """Initialize logmind in test directory."""
    from logmind.core.logger import log_first_decision

    docs_path = temp_dir / "docs"
    docs_path.mkdir()
    log_first_decision(docs_path)
    return docs_path


def test_log_decision_basic(init_logmind):
    """Test basic @log_decision decorator."""

    @log_decision(
        decision="Test decision",
        reasoning="Test reasoning",
        auto_commit=False,
        docs_path=init_logmind,
    )
    def test_func():
        return "result"

    result = test_func()

    assert result == "result"

    # Check that decision was logged
    decisions_path = init_logmind / "decisions.md"
    content = decisions_path.read_text(encoding="utf-8")
    assert "Test decision" in content
    assert "Test reasoning" in content


def test_log_decision_with_args(init_logmind):
    """Test @log_decision with argument placeholders."""

    @log_decision(
        decision="Use {method} authentication",
        reasoning="Security for {endpoint}",
        auto_commit=False,
        docs_path=init_logmind,
    )
    def authenticate(method, endpoint):
        return True

    result = authenticate("oauth", "/api/data")

    assert result is True

    # Check formatted decision
    decisions_path = init_logmind / "decisions.md"
    content = decisions_path.read_text(encoding="utf-8")
    assert "Use oauth authentication" in content
    assert "Security for /api/data" in content


def test_log_decision_with_kwargs(init_logmind):
    """Test @log_decision with keyword arguments."""

    @log_decision(
        decision="Connect to {host}:{port}",
        auto_commit=False,
        docs_path=init_logmind,
    )
    def connect(host="localhost", port=5432):
        return f"{host}:{port}"

    result = connect(host="db.example.com", port=3306)

    assert result == "db.example.com:3306"

    decisions_path = init_logmind / "decisions.md"
    content = decisions_path.read_text(encoding="utf-8")
    assert "Connect to db.example.com:3306" in content


def test_log_decision_with_defaults(init_logmind):
    """Test @log_decision uses function defaults."""

    @log_decision(
        decision="Cache backend: {backend}",
        auto_commit=False,
        docs_path=init_logmind,
    )
    def setup_cache(backend="redis"):
        return backend

    # Call without arguments - should use default
    result = setup_cache()

    assert result == "redis"

    decisions_path = init_logmind / "decisions.md"
    content = decisions_path.read_text(encoding="utf-8")
    assert "Cache backend: redis" in content


def test_log_decision_with_alternatives(init_logmind):
    """Test @log_decision with alternatives list."""

    @log_decision(
        decision="Use {framework}",
        reasoning="Best for our use case",
        alternatives=["Django", "Flask", "Tornado"],
        auto_commit=False,
        docs_path=init_logmind,
    )
    def choose_framework(framework):
        return framework

    result = choose_framework("FastAPI")

    assert result == "FastAPI"

    decisions_path = init_logmind / "decisions.md"
    content = decisions_path.read_text(encoding="utf-8")
    assert "Use FastAPI" in content
    assert "Django" in content
    assert "Flask" in content


def test_log_decision_with_template_alternatives(init_logmind):
    """Test @log_decision with templated alternatives."""

    @log_decision(
        decision="Deploy to {env}",
        alternatives=["AWS {region}", "GCP", "Azure"],
        auto_commit=False,
        docs_path=init_logmind,
    )
    def deploy(env, region="us-east-1"):
        return env

    result = deploy("AWS", region="eu-west-1")

    assert result == "AWS"

    decisions_path = init_logmind / "decisions.md"
    content = decisions_path.read_text(encoding="utf-8")
    assert "Deploy to AWS" in content
    assert "AWS eu-west-1" in content  # Template filled


def test_log_decision_with_implications(init_logmind):
    """Test @log_decision with implications."""

    @log_decision(
        decision="Enable {feature}",
        implications=[
            "Users will see new UI",
            "Backend load may increase",
        ],
        auto_commit=False,
        docs_path=init_logmind,
    )
    def enable_feature(feature):
        return True

    result = enable_feature("dark_mode")

    assert result is True

    decisions_path = init_logmind / "decisions.md"
    content = decisions_path.read_text(encoding="utf-8")
    assert "Enable dark_mode" in content
    assert "Users will see new UI" in content
    assert "Backend load may increase" in content


def test_log_decision_preserves_function_metadata(init_logmind):
    """Test that decorator preserves function metadata."""

    @log_decision(decision="Test", auto_commit=False)
    def my_function():
        """My docstring."""
        return 42

    assert my_function.__name__ == "my_function"
    assert my_function.__doc__ == "My docstring."


def test_log_decision_with_multiple_calls(init_logmind):
    """Test that decorator logs each function call."""

    @log_decision(
        decision="Process {item}",
        auto_commit=False,
        docs_path=init_logmind,
    )
    def process(item):
        return f"processed-{item}"

    # Call multiple times
    process("item1")
    process("item2")
    process("item3")

    decisions_path = init_logmind / "decisions.md"
    content = decisions_path.read_text(encoding="utf-8")
    assert "Process item1" in content
    assert "Process item2" in content
    assert "Process item3" in content


def test_log_choice_basic(init_logmind):
    """Test basic @log_choice decorator."""

    @log_choice(
        choices={
            "redis": "Use Redis for caching",
            "memory": "Use in-memory dict for caching",
        },
        auto_commit=False,
        docs_path=init_logmind,
    )
    def select_cache():
        return "redis"

    result = select_cache()

    assert result == "redis"

    decisions_path = init_logmind / "decisions.md"
    content = decisions_path.read_text(encoding="utf-8")
    assert "Use Redis for caching" in content


def test_log_choice_with_reasoning(init_logmind):
    """Test @log_choice with reasoning template."""

    @log_choice(
        choices={
            "postgres": "Use PostgreSQL",
            "mysql": "Use MySQL",
        },
        reasoning="Selected {return_value} based on requirements",
        auto_commit=False,
        docs_path=init_logmind,
    )
    def select_db():
        return "postgres"

    result = select_db()

    assert result == "postgres"

    decisions_path = init_logmind / "decisions.md"
    content = decisions_path.read_text(encoding="utf-8")
    assert "Use PostgreSQL" in content
    assert "Selected postgres based on requirements" in content


def test_log_choice_unknown_choice(init_logmind):
    """Test @log_choice with value not in choices dict."""

    @log_choice(
        choices={
            "redis": "Use Redis",
        },
        auto_commit=False,
        docs_path=init_logmind,
    )
    def select_backend():
        return "memcached"  # Not in choices

    result = select_backend()

    assert result == "memcached"

    decisions_path = init_logmind / "decisions.md"
    content = decisions_path.read_text(encoding="utf-8")
    assert "Unknown choice: memcached" in content


def test_log_choice_preserves_return_value(init_logmind):
    """Test that @log_choice doesn't modify return value."""

    @log_choice(
        choices={
            42: "The answer",
            0: "Zero",
        },
        auto_commit=False,
        docs_path=init_logmind,
    )
    def compute():
        return 42

    result = compute()

    assert result == 42
    assert isinstance(result, int)


def test_log_decision_with_exceptions(init_logmind):
    """Test that decorator logs even if function raises exception."""

    @log_decision(
        decision="Attempt risky operation",
        auto_commit=False,
        docs_path=init_logmind,
    )
    def risky_function():
        raise ValueError("Something went wrong")

    with pytest.raises(ValueError, match="Something went wrong"):
        risky_function()

    # Decision should still be logged before exception
    decisions_path = init_logmind / "decisions.md"
    content = decisions_path.read_text(encoding="utf-8")
    assert "Attempt risky operation" in content


@pytest.mark.skip(reason="Git integration tested in test_git_handler.py")
def test_log_decision_respects_auto_commit_flag(init_logmind, git_repo):
    """Test that auto_commit parameter is respected."""
    import subprocess

    # Create initial commit so we have a HEAD
    subprocess.run(
        ["git", "add", "."],
        cwd=git_repo,
        capture_output=True,
    )
    subprocess.run(
        ["git", "commit", "-m", "Initial commit"],
        cwd=git_repo,
        capture_output=True,
    )

    # Get initial commit count
    result = subprocess.run(
        ["git", "rev-list", "--count", "HEAD"],
        cwd=git_repo,
        capture_output=True,
        text=True,
    )
    initial_commits = int(result.stdout.strip())

    @log_decision(
        decision="Test auto commit",
        auto_commit=True,  # Should commit
        docs_path=init_logmind,
    )
    def test_func():
        return True

    test_func()

    # Check commit count increased
    result = subprocess.run(
        ["git", "rev-list", "--count", "HEAD"],
        cwd=git_repo,
        capture_output=True,
        text=True,
    )
    final_commits = int(result.stdout.strip())

    assert final_commits > initial_commits


def test_log_decision_with_complex_types(init_logmind):
    """Test @log_decision with complex argument types."""

    @log_decision(
        decision="Process items from {source}",
        auto_commit=False,
        docs_path=init_logmind,
    )
    def process_data(items, source="database"):
        count = len(items)
        return count

    result = process_data([1, 2, 3], source="API")

    assert result == 3

    # Check that decision was logged with correct source
    decisions_path = init_logmind / "decisions.md"
    content = decisions_path.read_text(encoding="utf-8")
    assert "Process items from API" in content
