"""Configuration management for logmind."""

from pathlib import Path
from typing import Any, Dict, Optional

try:
    import yaml
    HAS_YAML = True
except ImportError:
    HAS_YAML = False


# Default configuration
DEFAULT_CONFIG = {
    "git": {
        "auto_commit": True,
        "auto_push": True,
        "commit_message_template": "logmind: {decision}",
    },
    "decisions": {
        "max_recent": 20,
        # When True (default), feature-branch decisions are routed to
        # docs/decisions-branches/<branch>.md instead of decisions.md.
        # The default branch always writes to decisions.md.
        "branch_aware": True,
    },
    "file_structure": {
        "auto_update": True,
        "ignore_patterns": [
            "__pycache__",
            ".git",
            "node_modules",
            "venv",
            ".venv",
            "env",
            ".env",
            "*.pyc",
            ".pytest_cache",
            ".mypy_cache",
            "dist",
            "build",
            "*.egg-info",
        ],
    },
    "agents": {
        "claude": True,
        "cursor": True,
        "copilot": False,
        "windsurf": False,
        "aider": False,
        "continue": False,
        "cody": False,
        "zed": False,
        "amazonq": False,
        "cline": False,
        "codex": False,
    },
}


class Config:
    """Configuration manager for logmind."""

    def __init__(self, config_path: Optional[Path] = None):
        """
        Initialize configuration.

        Args:
            config_path: Path to config file. Defaults to .logmind/config.yml
        """
        if config_path is None:
            config_path = Path.cwd() / ".logmind" / "config.yml"

        self.config_path = config_path
        self._config = self._load_config()

    def _load_config(self) -> Dict[str, Any]:
        """
        Load configuration from file or return defaults.

        Returns:
            Configuration dictionary
        """
        import copy

        if not self.config_path.exists():
            return copy.deepcopy(DEFAULT_CONFIG)

        if not HAS_YAML:
            # Fallback to defaults if PyYAML not available
            return copy.deepcopy(DEFAULT_CONFIG)

        try:
            with open(self.config_path, "r") as f:
                user_config = yaml.safe_load(f) or {}

            # Merge with defaults (user config overrides defaults)
            config = copy.deepcopy(DEFAULT_CONFIG)
            self._deep_update(config, user_config)
            return config

        except Exception:
            # If any error reading config, use defaults
            return copy.deepcopy(DEFAULT_CONFIG)

    def _deep_update(self, base: Dict, updates: Dict) -> None:
        """
        Recursively update nested dictionary.

        Args:
            base: Base dictionary to update
            updates: Updates to apply
        """
        for key, value in updates.items():
            if isinstance(value, dict) and key in base and isinstance(base[key], dict):
                self._deep_update(base[key], value)
            else:
                base[key] = value

    def get(self, key_path: str, default: Any = None) -> Any:
        """
        Get configuration value using dot notation.

        Args:
            key_path: Dot-separated path (e.g., "git.auto_commit")
            default: Default value if key not found

        Returns:
            Configuration value

        Example:
            config.get("git.auto_commit")  # Returns True/False
        """
        keys = key_path.split(".")
        value = self._config

        for key in keys:
            if isinstance(value, dict) and key in value:
                value = value[key]
            else:
                return default

        return value

    def set(self, key_path: str, value: Any, save: bool = True) -> None:
        """
        Set configuration value using dot notation.

        Args:
            key_path: Dot-separated path (e.g., "git.auto_commit")
            value: Value to set
            save: Whether to save to file after setting (default: True)
        """
        keys = key_path.split(".")
        config = self._config

        for key in keys[:-1]:
            if key not in config:
                config[key] = {}
            config = config[key]

        config[keys[-1]] = value

        if save:
            self.save()

    def save(self) -> None:
        """Save configuration to file."""
        if not HAS_YAML:
            raise RuntimeError("PyYAML is required to save configuration")

        self.config_path.parent.mkdir(parents=True, exist_ok=True)

        with open(self.config_path, "w") as f:
            yaml.dump(self._config, f, default_flow_style=False, sort_keys=False)

    @property
    def auto_commit(self) -> bool:
        """Whether to auto-commit after logging."""
        return self.get("git.auto_commit", True)

    @property
    def auto_push(self) -> bool:
        """Whether to auto-push after committing."""
        return self.get("git.auto_push", True)

    @property
    def commit_message_template(self) -> str:
        """Template for commit messages."""
        return self.get("git.commit_message_template", "logmind: {decision}")

    @property
    def max_recent_decisions(self) -> int:
        """Maximum number of recent decisions to keep."""
        return self.get("decisions.max_recent", 20)

    @property
    def branch_aware(self) -> bool:
        """Route feature-branch decisions to per-branch files."""
        return self.get("decisions.branch_aware", True)

    @property
    def auto_update_file_structure(self) -> bool:
        """Whether to auto-update file structure."""
        return self.get("file_structure.auto_update", True)

    @property
    def ignore_patterns(self) -> list:
        """Patterns to ignore in file structure."""
        return self.get("file_structure.ignore_patterns", [])

    @property
    def agents(self) -> Dict[str, bool]:
        """Agent configuration."""
        return self.get("agents", {})

    def get_enabled_agents(self) -> list:
        """Get list of enabled agent names."""
        agents = self.agents
        return [name for name, enabled in agents.items() if enabled]


def load_config(config_path: Optional[Path] = None) -> Config:
    """
    Load logmind configuration.

    Args:
        config_path: Path to config file. Defaults to .logmind/config.yml

    Returns:
        Config instance
    """
    return Config(config_path)
