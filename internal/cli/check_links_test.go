package cli

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

// TestCheckLinks_CleanRepo: README.md with no links → clean report.
func TestCheckLinks_CleanRepo(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte("# OK\n"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	var stdout bytes.Buffer
	if err := runCheckLinks(dir, &stdout); err != nil {
		t.Fatalf("runCheckLinks: %v", err)
	}
	checkGolden(t, "check_links_clean.golden", stdout.String())
}

// TestCheckLinks_BrokenLink: README.md → missing file → exit 1.
func TestCheckLinks_BrokenLink(t *testing.T) {
	dir := t.TempDir()
	body := "# Project\n[broken](does-not-exist.md)\n"
	if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte(body), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	var stdout bytes.Buffer
	err := runCheckLinks(dir, &stdout)
	if !errors.Is(err, ErrSilent) {
		t.Fatalf("runCheckLinks err = %v; want ErrSilent", err)
	}
	checkGolden(t, "check_links_broken.golden", stdout.String())
}

// TestCheckLinks_OrphanFile: orphaned docs/orphan.md → exit 1.
func TestCheckLinks_OrphanFile(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte("# OK\n"), 0o644); err != nil {
		t.Fatalf("write README: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "docs"), 0o755); err != nil {
		t.Fatalf("mkdir docs: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "docs", "orphan.md"), []byte("# Orphan\n"), 0o644); err != nil {
		t.Fatalf("write orphan: %v", err)
	}
	var stdout bytes.Buffer
	err := runCheckLinks(dir, &stdout)
	if !errors.Is(err, ErrSilent) {
		t.Fatalf("runCheckLinks err = %v; want ErrSilent", err)
	}
	checkGolden(t, "check_links_orphan.golden", stdout.String())
}
