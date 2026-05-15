"""Tests for logmind.core.skill_install (Phase 10)."""

from __future__ import annotations

from types import SimpleNamespace

import pytest

from logmind.core import skill_install
from logmind.core.skill_install import (
    DEFAULT_SKILL_NAME,
    DEFAULT_SKILL_SOURCE,
    _build_install_argv,
    install_globally,
    is_skills_available,
)


def test_default_constants_match_published_skill():
    # v0.1.2 — collection layout at thrillmot/agent-skills (full URL form
    # so `skills add` recognizes it as a collection install).
    assert DEFAULT_SKILL_SOURCE == "https://github.com/thrillmot/agent-skills"
    assert DEFAULT_SKILL_NAME == "logmind"


def test_install_globally_passes_collection_url_and_skill_name(monkeypatch):
    """Default install argv must reference the agent-skills collection URL
    and pass --skill logmind so skills.sh picks the right entry."""
    monkeypatch.setattr(skill_install.shutil, "which", lambda name: "/usr/bin/skills")
    captured = {}
    def fake_run(argv, **kwargs):
        captured["argv"] = argv
        return SimpleNamespace(returncode=0, stdout="", stderr="")
    install_globally(runner=fake_run)
    assert "https://github.com/thrillmot/agent-skills" in captured["argv"]
    skill_idx = captured["argv"].index("--skill")
    assert captured["argv"][skill_idx + 1] == "logmind"


def test_build_install_argv_npx_mode():
    argv = _build_install_argv("owner/repo", "logmind", use_npx=True)
    assert argv == ["npx", "-y", "skills", "add", "-g", "owner/repo", "--skill", "logmind"]


def test_build_install_argv_native_mode():
    argv = _build_install_argv("owner/repo", "logmind", use_npx=False)
    assert argv == ["skills", "add", "-g", "owner/repo", "--skill", "logmind"]


def test_is_skills_available_true_when_skills_on_path(monkeypatch):
    monkeypatch.setattr(skill_install.shutil, "which",
                        lambda name: "/usr/local/bin/skills" if name == "skills" else None)
    assert is_skills_available() is True


def test_is_skills_available_falls_back_to_npx(monkeypatch):
    def which(name):
        return "/usr/local/bin/npx" if name == "npx" else None
    monkeypatch.setattr(skill_install.shutil, "which", which)
    assert is_skills_available() is True


def test_is_skills_available_false_when_neither_present(monkeypatch):
    monkeypatch.setattr(skill_install.shutil, "which", lambda name: None)
    assert is_skills_available() is False


def test_install_globally_uses_npx_when_skills_missing(monkeypatch):
    """If only npx is on PATH, install_globally should shell out via npx."""
    def which(name):
        return "/usr/local/bin/npx" if name == "npx" else None
    monkeypatch.setattr(skill_install.shutil, "which", which)

    captured = {}
    def fake_run(argv, **kwargs):
        captured["argv"] = argv
        return SimpleNamespace(returncode=0, stdout="ok", stderr="")

    rc, output = install_globally(runner=fake_run)
    assert rc == 0
    assert "ok" in output
    assert captured["argv"][0] == "npx"
    assert captured["argv"][:4] == ["npx", "-y", "skills", "add"]


def test_install_globally_uses_native_skills_when_available(monkeypatch):
    monkeypatch.setattr(skill_install.shutil, "which",
                        lambda name: "/usr/local/bin/skills" if name == "skills" else None)

    captured = {}
    def fake_run(argv, **kwargs):
        captured["argv"] = argv
        return SimpleNamespace(returncode=0, stdout="ok", stderr="")

    install_globally(runner=fake_run)
    assert captured["argv"][0] == "skills"


def test_install_globally_propagates_failure(monkeypatch):
    monkeypatch.setattr(skill_install.shutil, "which", lambda name: "/usr/bin/skills")
    def fake_run(argv, **kwargs):
        return SimpleNamespace(returncode=2, stdout="", stderr="boom")
    rc, output = install_globally(runner=fake_run)
    assert rc == 2
    assert "boom" in output


def test_install_globally_accepts_custom_source_and_skill(monkeypatch):
    monkeypatch.setattr(skill_install.shutil, "which", lambda name: "/usr/bin/skills")
    captured = {}
    def fake_run(argv, **kwargs):
        captured["argv"] = argv
        return SimpleNamespace(returncode=0, stdout="", stderr="")
    install_globally(source="other/repo", skill_name="other-name", runner=fake_run)
    assert "other/repo" in captured["argv"]
    assert "other-name" in captured["argv"]
