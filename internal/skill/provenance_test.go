package skill

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestWriteProvenanceSkeleton: first call writes the file with the
// canonical YAML block + the skill name interpolated into the heading.
func TestWriteProvenanceSkeleton(t *testing.T) {
	dir := t.TempDir()
	skillDir := filepath.Join(dir, ".claude", "skills", "alpha")
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	skillMD := filepath.Join(skillDir, "SKILL.md")
	if err := os.WriteFile(skillMD, []byte("---\nname: alpha\n---\n"), 0o644); err != nil {
		t.Fatalf("write SKILL.md: %v", err)
	}
	if err := WriteProvenanceSkeleton(skillMD, "alpha"); err != nil {
		t.Fatalf("WriteProvenanceSkeleton: %v", err)
	}
	body, err := os.ReadFile(filepath.Join(skillDir, "PROVENANCE.md"))
	if err != nil {
		t.Fatalf("read PROVENANCE.md: %v", err)
	}
	if !strings.Contains(string(body), "# Provenance for skill: alpha") {
		t.Errorf("missing skill name heading; body:\n%s", body)
	}
	if !strings.Contains(string(body), "derived-from-decisions: []") {
		t.Errorf("missing derived-from-decisions key; body:\n%s", body)
	}
	if !strings.Contains(string(body), "cited-by-clud-bug: 0") {
		t.Errorf("missing cited-by-clud-bug counter; body:\n%s", body)
	}
}

// TestWriteProvenanceSkeleton_RefusesClobber: re-running on an
// existing PROVENANCE.md returns an error wrapping os.ErrExist.
func TestWriteProvenanceSkeleton_RefusesClobber(t *testing.T) {
	dir := t.TempDir()
	skillDir := filepath.Join(dir, ".claude", "skills", "alpha")
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	skillMD := filepath.Join(skillDir, "SKILL.md")
	if err := WriteProvenanceSkeleton(skillMD, "alpha"); err != nil {
		t.Fatalf("first write: %v", err)
	}
	err := WriteProvenanceSkeleton(skillMD, "alpha")
	if !errors.Is(err, os.ErrExist) {
		t.Fatalf("expected os.ErrExist; got %v", err)
	}
}
