# Homebrew Tap for logmind

This is a [Homebrew](https://brew.sh/) tap for installing [logmind](https://github.com/thrillmot/logmind).

## Installation

```bash
# Add the tap
brew tap thrillmot/logmind

# Install logmind
brew install logmind
```

## Usage

After installation, you can use logmind:

```bash
# Initialize in your project
cd your-project
logmind init

# Log a decision
logmind log "Use PostgreSQL for database" -r "Need ACID compliance"

# View decisions
logmind show

# Search decisions
logmind search "postgres"
```

## Updating

```bash
brew upgrade logmind
```

## Alternative: pipx

If you prefer Python package management:

```bash
pipx install logmind
```

## About

logmind is an AI decision logging system that tracks decisions made during development, maintains documentation, and provides context for AI agents.

See the [main repository](https://github.com/thrillmot/logmind) for full documentation.
