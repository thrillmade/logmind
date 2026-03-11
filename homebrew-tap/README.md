# homebrew-logmind

Homebrew tap for [logmind](https://github.com/thrillmot/logmind) — AI decision logging system.

## Install

```bash
brew tap thrillmot/logmind
brew install logmind
```

## Usage

```bash
logmind init          # Initialize in your project
logmind log "Use PostgreSQL" -r "Need ACID compliance"
logmind show          # View recent decisions
logmind stats         # View analytics
```

## Update

```bash
brew upgrade logmind
```

## Uninstall

```bash
brew uninstall logmind
brew untap thrillmot/logmind
```

## Publishing a new version

1. Build and publish to PyPI: `python -m build && twine upload dist/*`
2. Get the SHA256 of the new tarball from PyPI
3. Update `Formula/logmind.rb` with the new `url`, `sha256`, and version
4. Commit and push this tap repository
