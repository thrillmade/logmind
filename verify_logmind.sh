#!/bin/bash
# Quick verification script for logmind

set -e

echo "=== Creating test project ==="
TEST_DIR="/tmp/logmind-test-$(date +%s)"
mkdir -p "$TEST_DIR"
cd "$TEST_DIR"

echo "=== Initializing git ==="
git init
git config user.name "Test User"
git config user.email "test@test.com"

echo ""
echo "=== Running logmind init ==="
logmind init

echo ""
echo "=== Checking created files ==="
ls -la
echo ""
echo "--- docs/decisions.md ---"
cat docs/decisions.md
echo ""
echo "--- CLAUDE.md (first 30 lines) ---"
head -30 CLAUDE.md

echo ""
echo "=== Logging a test decision ==="
logmind log "Use PostgreSQL for database" \
    -r "Need ACID compliance and complex queries" \
    -a "MongoDB" \
    -a "MySQL" \
    -i "Need to set up connection pooling"

echo ""
echo "=== Showing all decisions ==="
logmind show

echo ""
echo "=== Git history ==="
git log --oneline

echo ""
echo "=== File structure ==="
cat docs/file-structure.md

echo ""
echo "✅ Verification complete! Test directory: $TEST_DIR"
echo "   (You can inspect or delete it manually)"
